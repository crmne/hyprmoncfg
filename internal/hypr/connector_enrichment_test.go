package hypr

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestMonitorsBoundsBlockedConnectorEnrichmentAndRecovers(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	var calls, active, maximum atomic.Int32
	enricher := newMonitorConnectorEnricher(func(monitors []Monitor) {
		n := active.Add(1)
		defer active.Add(-1)
		for old := maximum.Load(); n > old && !maximum.CompareAndSwap(old, n); old = maximum.Load() {
		}
		if calls.Add(1) == 1 {
			close(started)
			<-release
		}
		for i := range monitors {
			monitors[i].ConnectorPath = "mst:" + monitors[i].Name
		}
	})
	client, payloadPath := monitorEnrichmentFixture(t, duplicateMonitors(), enricher)
	type outcome struct {
		monitors []Monitor
		err      error
	}
	results := make(chan outcome, 9)
	query := func(client *Client, timeout time.Duration) {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		monitors, err := client.Monitors(ctx)
		results <- outcome{monitors, err}
	}
	go query(client, 150*time.Millisecond)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first enrichment did not start")
	}
	// Multiple clients share the same production coordinator, not one worker
	// per IPC connection. Inject that same arrangement without touching DRM.
	secondClient := &Client{hyprctl: client.hyprctl, connectorEnricher: enricher}
	for range 8 {
		go query(secondClient, 60*time.Millisecond)
	}
	for range 9 {
		select {
		case result := <-results:
			if !errors.Is(result.err, context.DeadlineExceeded) || result.monitors != nil {
				t.Fatalf("blocked query returned snapshot or wrong error: %+v", result)
			}
		case <-time.After(time.Second):
			t.Fatal("blocked DRM enrichment escaped the caller deadline")
		}
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("started %d blocked probes; want exactly one", got)
	}
	// Distinct identities need no connector disambiguation, including while a
	// previous driver call remains stuck. This read must not wait for its gate.
	unique := []Monitor{{Name: "DP-unique", Make: "Maker", Model: "Panel", Serial: "unique"}}
	writeMonitorFixture(t, payloadPath, unique)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	gotUnique, err := secondClient.Monitors(ctx)
	if err != nil || !reflect.DeepEqual(gotUnique, unique) || calls.Load() != 1 {
		t.Fatalf("unique identities did not bypass occupied DRM gate: %+v, %v, probes=%d", gotUnique, err, calls.Load())
	}
	// The first result is obsolete: the next read must enrich its own fresh
	// topology, never consume the canceled caller's snapshot as a cache hit.
	fresh := duplicateMonitors()
	fresh[0].Name, fresh[1].Name = "DP-new-1", "DP-new-2"
	writeMonitorFixture(t, payloadPath, fresh)
	unblock()
	gotFresh, err := client.Monitors(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for i := range fresh {
		if gotFresh[i].Name != fresh[i].Name || gotFresh[i].ConnectorPath != "mst:"+fresh[i].Name {
			t.Fatalf("recovered query returned stale or unenriched identity: %+v", gotFresh)
		}
	}
	if !reflect.DeepEqual(gotUnique, unique) {
		t.Fatalf("late worker mutated previously returned snapshot: %+v", gotUnique)
	}
	if calls.Load() != 2 || maximum.Load() != 1 {
		t.Fatalf("probes=%d, maximum simultaneous=%d; want 2 and 1", calls.Load(), maximum.Load())
	}
}

func TestConnectorEnrichmentOwnsCanceledSnapshot(t *testing.T) {
	started, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	enricher := newMonitorConnectorEnricher(func(monitors []Monitor) {
		close(started)
		<-release
		monitors[0].ConnectorPath = "late-path"
		monitors[0].AvailableModes[0] = "late-mode"
		close(finished)
	})
	input := duplicateMonitors()
	want := duplicateMonitors()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		got, err := enricher.enrich(ctx, input)
		if got != nil {
			done <- errors.New("returned an incomplete snapshot")
			return
		}
		done <- err
	}()
	<-started
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled enrichment returned %v", err)
	}
	unblock()
	<-finished
	if !reflect.DeepEqual(input, want) {
		t.Fatalf("late worker mutated caller-owned snapshot: %+v", input)
	}
}

func TestConnectorEnrichmentSkipsUniqueAndExpiredRequests(t *testing.T) {
	var probes int
	enricher := newMonitorConnectorEnricher(func([]Monitor) { probes++ })
	unique := duplicateMonitors()
	unique[0].Serial, unique[1].Serial = "one", "two"
	got, err := enricher.enrich(context.Background(), unique)
	if err != nil || !reflect.DeepEqual(got, unique) || probes != 0 {
		t.Fatalf("distinct serials require no DRM: %+v, %v, probes=%d", got, err, probes)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got, err := enricher.enrich(ctx, duplicateMonitors()); got != nil || !errors.Is(err, context.Canceled) || probes != 0 {
		t.Fatalf("expired request started enrichment: %+v, %v, probes=%d", got, err, probes)
	}
}

func TestConnectorEnrichmentOnlyProbesAmbiguousRoles(t *testing.T) {
	enricher := newMonitorConnectorEnricher(func(monitors []Monitor) {
		if len(monitors) != 2 {
			t.Errorf("DRM received %d roles; want only two ambiguous roles", len(monitors))
		}
		for i := range monitors {
			monitors[i].ConnectorPath = "mst:519-" + monitors[i].Name
		}
	})
	input := append(duplicateMonitors(), Monitor{Name: "eDP-1", Serial: "unique"})
	got, err := enricher.enrich(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	counts := MonitorMatchCounts(got)
	for i := range 2 {
		if got[i].ConnectorPath == "" || MonitorOutputKey(got[i], counts) == MonitorOutputKey(input[i], counts) {
			t.Errorf("ambiguous role lost its stable connector identity: %+v", got[i])
		}
	}
	if got[2].ConnectorPath != "" || MonitorOutputKey(got[2], counts) != MonitorOutputKey(input[2], counts) {
		t.Errorf("unique role changed identity: %+v", got[2])
	}
}

func duplicateMonitors() []Monitor {
	return []Monitor{
		{Name: "DP-1", Make: "Maker", Model: "Panel", AvailableModes: []string{"1920x1080@60.00Hz"}},
		{Name: "DP-2", Make: "Maker", Model: "Panel", AvailableModes: []string{"1920x1080@60.00Hz"}},
	}
}

func monitorEnrichmentFixture(t *testing.T, monitors []Monitor, enricher *monitorConnectorEnricher) (*Client, string) {
	t.Helper()
	dir := t.TempDir()
	hyprctl := filepath.Join(dir, "hyprctl")
	if err := os.WriteFile(hyprctl, []byte("#!/bin/sh\ncat \"${0%/*}/monitors.json\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	payload := filepath.Join(dir, "monitors.json")
	writeMonitorFixture(t, payload, monitors)
	t.Setenv("HYPRLAND_INSTANCE_SIGNATURE", "fixture")
	return &Client{hyprctl: hyprctl, connectorEnricher: enricher}, payload
}

func writeMonitorFixture(t *testing.T, path string, monitors []Monitor) {
	t.Helper()
	payload, err := json.Marshal(monitors)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
}

package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/ipc"
	"github.com/crmne/hyprmoncfg/internal/lid"
	"github.com/crmne/hyprmoncfg/internal/orientation"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

// dpmsWakeCount counts wake dispatches recorded by the fake compositor.
func dpmsWakeCount(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	return strings.Count(string(data), "dispatch dpms on")
}

// Ordinary sleep and deliberately disabled outputs must not request a wake.
func TestWakeIncompleteRespectsExplicitWakeAndDisabledOutputs(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "eDP-1", Width: 1920, Height: 1080, Scale: 1, DPMSStatus: true},
		{Name: "DP-1", Width: 1920, Height: 1080, Scale: 1, DPMSStatus: false},
	}
	svc := &Service{}
	target := profile.FromMonitors("Desk", monitors)
	if svc.wakeIncomplete(target, monitors) {
		t.Fatal("ordinary display sleep requested a wake")
	}
	svc.wakeRequested.Store(true)
	if !svc.wakeIncomplete(target, monitors) {
		t.Fatal("partial wake counted as complete")
	}
	for i := range target.Outputs {
		if target.Outputs[i].Name == "DP-1" {
			target.Outputs[i].Enabled = false
		}
	}
	if svc.wakeIncomplete(target, monitors) {
		t.Fatal("deliberately disabled display requested a wake")
	}
}

// Completing a wake must also prevent later intentional sleep from reviving it.
func TestRunStopsPartialWakeRetriesOnceExternalWakes(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "eDP-1", Make: "Example", Model: "Laptop", Width: 1920, Height: 1080, RefreshRate: 60, Scale: 1, DPMSStatus: true},
		{Name: "DP-1", Make: "Example", Model: "Desk", Width: 1920, Height: 1080, RefreshRate: 60, X: 1920, Scale: 1, DPMSStatus: true},
	}
	env := newRunTestEnvConfigured(t, monitors, func(cfg *Config) { cfg.PollInterval = 20 * time.Millisecond; cfg.WakeSettle = 200 * time.Millisecond })
	defer env.stop()
	waitFor(t, time.Second, func() bool { return env.logs.contains("applied profile: Home") }, "startup apply")
	env.suspendEvents <- true
	monitors[1].DPMSStatus = false
	if err := writeMonitorState(env.monitorStatePath, monitors); err != nil {
		t.Fatal(err)
	}
	env.suspendEvents <- false
	waitFor(t, time.Second, func() bool { return dpmsWakeCount(env.logPath) == 2 }, "first partial wake retry")
	monitors[1].DPMSStatus = true
	if err := writeMonitorState(env.monitorStatePath, monitors); err != nil {
		t.Fatal(err)
	}
	time.Sleep(3 * env.svc.cfg.WakeSettle)
	if got := dpmsWakeCount(env.logPath); got != 2 {
		t.Fatalf("wake continued after recovery: %d", got)
	}
	// A later, intentional external sleep must not revive the old request.
	monitors[1].DPMSStatus = false
	if err := writeMonitorState(env.monitorStatePath, monitors); err != nil {
		t.Fatal(err)
	}
	time.Sleep(3 * env.svc.cfg.WakeSettle)
	if got := dpmsWakeCount(env.logPath); got != 2 {
		t.Fatalf("later sleep repeated an expired wake: %d", got)
	}
}

// A stuck external uses only the wake budget, without reloading the awake desktop.
func TestRunRetriesPartialWakeWithoutReloadingOrWakingForever(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "eDP-1", Make: "Example", Model: "Laptop", Width: 1920, Height: 1080, RefreshRate: 60, Scale: 1, DPMSStatus: true},
		{Name: "DP-1", Make: "Example", Model: "Desk", Width: 1920, Height: 1080, RefreshRate: 60, X: 1920, Scale: 1, DPMSStatus: true},
	}
	env := newRunTestEnvConfigured(t, monitors, func(cfg *Config) { cfg.PollInterval = 20 * time.Millisecond })
	defer env.stop()
	defer func() {
		if t.Failed() {
			t.Log(env.logs.all())
		}
	}()
	waitFor(t, time.Second, func() bool { return env.logs.contains("applied profile: Home") }, "startup apply")
	env.suspendEvents <- true
	// The laptop wakes but the dock ignores the initial wake request. Neither
	// a valid mode nor one awake panel proves the other output has woken.
	monitors[1].DPMSStatus = false
	if err := writeMonitorState(env.monitorStatePath, monitors); err != nil {
		t.Fatal(err)
	}
	env.suspendEvents <- false
	waitFor(t, 2*time.Second, func() bool { return dpmsWakeCount(env.logPath) == 1+wakeRetryLimit }, "partial wake retries")
	time.Sleep(4 * env.svc.cfg.WakeSettle)
	if got := dpmsWakeCount(env.logPath); got != 1+wakeRetryLimit {
		t.Fatalf("unbounded wake requests: %d", got)
	}
	if got := reloadCount(env.logPath); got != 1 {
		t.Fatalf("partial wake caused %d reloads, want startup only", got)
	}
	if env.logs.contains("display sleep detected; pausing automatic switching") {
		t.Fatal("one sleeping output put the awake desktop to sleep")
	}
}

// Right after resume, displays still reporting DPMS off are a wake that did
// not take. Repeat it a bounded number of times before calling it sleep.
func TestRunRepeatsAWakeThatDidNotTakeThenRespectsSleep(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "DP-1", Description: "Dell U2720Q", Make: "Dell", Model: "U2720Q", Serial: "B1", Width: 3840, Height: 2160, RefreshRate: 60, Scale: 2, DPMSStatus: true},
	}
	env := newRunTestEnv(t, monitors)
	defer env.stop()
	defer func() {
		if t.Failed() {
			t.Logf("daemon logs:\n%s", env.logs.all())
		}
	}()
	waitFor(t, time.Second, func() bool { return env.logs.contains("applied profile: Home") }, "startup apply")

	env.suspendEvents <- true
	asleep := append([]hypr.Monitor(nil), monitors...)
	asleep[0].DPMSStatus = false
	if err := writeMonitorState(env.monitorStatePath, asleep); err != nil {
		t.Fatal(err)
	}
	env.suspendEvents <- false

	// The fake compositor ignores every wake, as a stuck one would.
	waitFor(t, 2*time.Second, func() bool {
		return strings.Count(env.logs.all(), "displays still asleep after a wake request; waking them again") == wakeRetryLimit
	}, "bounded wake retries")
	waitFor(t, time.Second, func() bool { return env.logs.contains("display sleep detected; pausing automatic switching") }, "sleep respected after retries")
	if got := dpmsWakeCount(env.logPath); got != 1+wakeRetryLimit {
		t.Fatalf("wake dispatches = %d, want %d", got, 1+wakeRetryLimit)
	}
	time.Sleep(3 * env.svc.cfg.WakeSettle)
	if got := dpmsWakeCount(env.logPath); got != 1+wakeRetryLimit {
		t.Fatalf("kept waking displays after giving up: %d dispatches", got)
	}
}

// The clamshell case from #59: the panel was switched off with the lid shut,
// and after opening the lid the external display stays asleep. The saved
// profile is what brings the panel back, so the sleep guard must let it
// through instead of waiting for the external to wake.
func TestRunRestoresTheLaptopPanelWhenTheExternalStaysAsleepAfterLidOpen(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "eDP-1", Description: "Framework Panel", Make: "Framework", Model: "Panel", Serial: "A1", Width: 2880, Height: 1800, RefreshRate: 120, Scale: 1.5, DPMSStatus: true},
		{Name: "DP-1", Description: "Dell U2720Q", Make: "Dell", Model: "U2720Q", Serial: "B1", Width: 3840, Height: 2160, RefreshRate: 60, X: 2880, Scale: 2, DPMSStatus: true},
	}
	env := newRunTestEnv(t, monitors)
	defer env.stop()
	defer func() {
		if t.Failed() {
			t.Logf("daemon logs:\n%s", env.logs.all())
		}
	}()
	waitFor(t, time.Second, func() bool { return env.logs.contains("applied profile: Home") }, "startup apply")

	clamshell := append([]hypr.Monitor(nil), monitors...)
	clamshell[0].Disabled = true
	if err := writeMonitorState(env.monitorStatePath+".next", clamshell); err != nil {
		t.Fatal(err)
	}
	env.setLid(lid.Closed)
	env.lidStates <- lid.Closed
	waitFor(t, time.Second, func() bool { return env.logs.contains("lid closed: forced internal outputs off") }, "clamshell apply")
	waitFor(t, time.Second, func() bool { return reloadCount(env.logPath) == 2 }, "clamshell reload")
	// Let that apply finish verifying before the displays change under it.
	waitFor(t, 2*time.Second, func() bool {
		return strings.Count(env.logs.all(), "automatic reconciliation completed") >= 2
	}, "clamshell reconciliation")

	// The external falls asleep and stays asleep through the lid opening.
	stuck := append([]hypr.Monitor(nil), clamshell...)
	stuck[1].DPMSStatus = false
	if err := writeMonitorState(env.monitorStatePath, stuck); err != nil {
		t.Fatal(err)
	}
	restored := append([]hypr.Monitor(nil), monitors...)
	restored[1].DPMSStatus = false
	if err := writeMonitorState(env.monitorStatePath+".next", restored); err != nil {
		t.Fatal(err)
	}
	env.setLid(lid.Open)
	env.lidStates <- lid.Open

	waitFor(t, 2*time.Second, func() bool { return reloadCount(env.logPath) == 3 }, "panel restored despite the sleeping external")
	if env.logs.contains("automatic switching deferred while displays sleep: lid:open") {
		t.Fatal("the lid-open reconciliation was deferred as display sleep")
	}
}

// An in-flight resume wake must not turn a successful IPC operation into an
// error. The background path still needs its sentinel to schedule wake retries.
func TestProfileAutoDuringPartialWakeDoesNotExposeRetryState(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "eDP-1", Make: "Example", Model: "Panel", Width: 1920, Height: 1080, RefreshRate: 60, Scale: 1, DPMSStatus: true},
		{Name: "DP-1", Make: "Example", Model: "External", Width: 1920, Height: 1080, RefreshRate: 60, Scale: 1, X: 1920, DPMSStatus: false},
	}
	data, err := json.Marshal(monitors)
	if err != nil {
		t.Fatal(err)
	}
	env := newApplyBestTestEnvWithMonitors(t, string(data), string(data))
	if err := env.store.Save(profile.FromMonitors("Desk", monitors)); err != nil {
		t.Fatal(err)
	}
	svc := New(env.client, env.store, Config{MonitorsConf: env.monitorsConfPath, HyprConfig: env.hyprlandConfigPath, ConfigDir: filepath.Dir(env.monitorsConfPath)})
	svc.wakeRequested.Store(true)
	svc.setManualOverride(profile.MonitorSetHash(monitors), profile.FromMonitors("Manual", monitors))
	if err := svc.SetProfileAuto(ipc.ProfileAutoParams{Enabled: true}); err != nil {
		t.Fatalf("successful profile selection returned an error to IPC: %v", err)
	}
	if _, pinned := svc.manualOverride(profile.MonitorSetHash(monitors)); pinned {
		t.Fatal("automatic selection did not clear manual override")
	}
	if !svc.wakeRequested.Load() {
		t.Fatal("IPC operation cleared the pending wake request")
	}
	if err := svc.tryApplyBest(context.Background()); !errors.Is(err, errIncompleteWake) {
		t.Fatalf("background retry state = %v, want incomplete wake", err)
	}
}

// Changing rotation must preserve wake recovery without reporting it as a failure.
func TestRotationDuringPartialWakeDoesNotLogFailure(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "eDP-1", Make: "Example", Model: "Panel", Width: 1920, Height: 1080, RefreshRate: 60, Scale: 1, DPMSStatus: true},
		{Name: "DP-1", Make: "Example", Model: "External", Width: 1920, Height: 1080, RefreshRate: 60, Scale: 1, X: 1920, DPMSStatus: false},
	}
	data, err := json.Marshal(monitors)
	if err != nil {
		t.Fatal(err)
	}
	env := newApplyBestTestEnvWithMonitors(t, string(data), string(data))
	if err := env.store.Save(profile.FromMonitors("Desk", monitors)); err != nil {
		t.Fatal(err)
	}
	var logs []string
	svc := New(env.client, env.store, Config{
		MonitorsConf: env.monitorsConfPath, HyprConfig: env.hyprlandConfigPath, ConfigDir: filepath.Dir(env.monitorsConfPath),
		WatchOrientation: func(context.Context) <-chan orientation.Reading { return nil },
		Logf:             func(format string, args ...any) { logs = append(logs, fmt.Sprintf(format, args...)) },
	})
	svc.rotation.observe(orientation.Reading{Available: true, Orientation: "left-up"})
	svc.wakeRequested.Store(true)
	if err := svc.SetRotation(ipc.RotationParams{Reversed: boolPtr(true)}); err != nil {
		t.Fatal(err)
	}
	if svc.rotation.turn() != 3 {
		t.Fatal("rotation did not change")
	}
	if strings.Contains(strings.Join(logs, "\n"), "rotate after a rotation change:") {
		t.Fatalf("pending wake logged as failure: %v", logs)
	}
	if !svc.wakeRequested.Load() {
		t.Fatal("rotation cleared the pending wake")
	}
	if err := svc.tryApplyBest(context.Background()); !errors.Is(err, errIncompleteWake) {
		t.Fatalf("background retry state = %v, want incomplete wake", err)
	}
}

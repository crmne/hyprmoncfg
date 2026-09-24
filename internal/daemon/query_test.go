package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/crmne/hyprmoncfg/internal/appstatus"
	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/ipc"
	"github.com/crmne/hyprmoncfg/internal/lid"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

func TestApplyBestBoundsEngineReadsAndCanRetry(t *testing.T) {
	for _, tc := range []struct {
		operation string
		call      int
	}{
		{"version", 1},
		{"version", 2},        // SupportsMonitorV2 performs its own version read.
		{"workspacerules", 2}, // The daemon's first read succeeds; the engine's stalls.
		{"workspaces", 1},
		{"monitors", 2}, // Post-reload validation, after the config has been written.
	} {
		t.Run(fmt.Sprintf("%s-%d", tc.operation, tc.call), func(t *testing.T) {
			env := newApplyBestTestEnvWithMonitors(t, applyBestDualBeforeJSON, applyBestDualBeforeJSON)
			if err := env.store.Save(profile.FromMonitors("Desk", applyBestDualMonitors())); err != nil {
				t.Fatal(err)
			}
			helper := filepath.Join(filepath.Dir(env.logPath), "hyprctl")
			source, err := os.ReadFile(helper)
			if err != nil {
				t.Fatal(err)
			}
			// Stall just one selected read. exec replaces the shell so canceled
			// subprocesses cannot leave a sleep process holding the output pipe.
			stall := fmt.Sprintf(`
if [[ "${1-}" == "-j" && "${2-}" == %q ]]; then
  count=0
  if [[ -f "$HYPRCTL_LOG.count" ]]; then read -r count < "$HYPRCTL_LOG.count"; fi
  count=$((count + 1))
  printf '%%s\n' "$count" > "$HYPRCTL_LOG.count"
  if [[ "$count" == %d ]]; then exec sleep 20; fi
fi
`, tc.operation, tc.call)
			anchor := `printf '%s\n' "$*" >> "$HYPRCTL_LOG"`
			if !strings.Contains(string(source), anchor) {
				t.Fatal("fake compositor script changed")
			}
			if err := os.WriteFile(helper, []byte(strings.Replace(string(source), anchor, anchor+stall, 1)), 0755); err != nil {
				t.Fatal(err)
			}
			logs := &logRecorder{}
			svc := New(env.client, env.store, Config{
				QueryTimeout: 40 * time.Millisecond,
				MonitorsConf: env.monitorsConfPath, HyprConfig: env.hyprlandConfigPath, Logf: logs.logf,
			})
			before := readMonitorsConf(t, env)
			started := time.Now()
			err = svc.applyBest(context.Background())
			if !errors.Is(err, ipc.ErrCompositorBusy) || time.Since(started) > 500*time.Millisecond {
				t.Fatalf("engine read was not bounded and retryable: elapsed=%s error=%v", time.Since(started), err)
			}
			if readMonitorsConf(t, env) != before {
				t.Fatal("timed-out apply did not preserve/restore the previous config")
			}
			if !logs.contains("apply query operation=") {
				t.Fatal("failure did not exercise an apply-engine query")
			}
			if err := svc.applyBest(context.Background()); err != nil {
				t.Fatalf("retry after the read recovered: %v", err)
			}
			if !logs.contains("applied profile: Desk") {
				t.Fatal("recovered compositor did not complete the apply")
			}
		})
	}
}

func TestCompositorQueriesAreBoundedAndLogLatency(t *testing.T) {
	dir := t.TempDir()
	// The process is replaced, so cancellation cannot leave a child sleep
	// around. No real compositor or display command is ever invoked.
	if err := os.WriteFile(filepath.Join(dir, "hyprctl"), []byte("#!/bin/bash\nexec sleep 20\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	t.Setenv("HYPRLAND_INSTANCE_SIGNATURE", "isolated-test")
	client, err := hypr.NewClient()
	if err != nil {
		t.Fatal(err)
	}
	logs := &logRecorder{}
	svc := New(client, profile.NewStore(dir), Config{QueryTimeout: 35 * time.Millisecond, Logf: logs.logf})
	for _, query := range []string{"monitors", "editor", "status"} {
		started := time.Now()
		var err error
		switch query {
		case "monitors":
			_, err = svc.queryMonitors(context.Background())
		case "editor":
			_, err = svc.EditorState()
		case "status":
			_, err = svc.Status()
		}
		if !errors.Is(err, ipc.ErrCompositorBusy) || time.Since(started) > 300*time.Millisecond {
			t.Fatalf("%s query not bounded: elapsed=%s err=%v", query, time.Since(started), err)
		}
	}
	if !logs.contains("compositor query operation=monitors elapsed=") {
		t.Fatalf("missing timing diagnostics: %s", logs.all())
	}
}

func TestUnfamiliarSetupBuildsTemporaryLayout(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "eDP-1", Make: "Example", Model: "Laptop", Width: 1920, Height: 1080, RefreshRate: 60, Scale: 1, DPMSStatus: true},
		{Name: "DP-1", Make: "Example", Model: "New display", Width: 1920, Height: 1080, RefreshRate: 60, Scale: 1, DPMSStatus: true},
	}
	before, _ := json.Marshal(monitors)
	monitors[1].X = 1920
	after, _ := json.Marshal(monitors)
	env := newApplyBestTestEnvWithMonitors(t, string(before), string(after))
	svc := New(env.client, env.store, Config{MonitorsConf: env.monitorsConfPath, HyprConfig: env.hyprlandConfigPath})
	svc.readLid = func(context.Context) (lid.State, error) { return lid.Open, nil }
	if err := svc.applyBest(context.Background()); err != nil {
		t.Fatal(err)
	}
	if rendered := readMonitorsConf(t, env); !strings.Contains(rendered, "position = 1920x0") || strings.Contains(rendered, "disable") {
		t.Fatalf("unfamiliar displays were not extended into a usable layout: %s", rendered)
	}
	if profiles, err := env.store.List(); err != nil || len(profiles) != 0 {
		t.Fatalf("automatic extension saved an unsolicited profile: %+v (%v)", profiles, err)
	}
}

func TestAutomaticApplyDoesNotWaitForInteractiveWriter(t *testing.T) {
	svc := New(nil, nil, Config{})
	svc.writeMu.Lock()
	defer svc.writeMu.Unlock()
	started := time.Now()
	if err := svc.tryApplyBest(context.Background()); !errors.Is(err, errWriterBusy) {
		t.Fatalf("unexpected result: %v", err)
	}
	if time.Since(started) > 20*time.Millisecond {
		t.Fatal("automatic apply blocked behind interactive writer")
	}
}

func TestHardwareSnapshotsRejectReplacementDuringWorkspaceRead(t *testing.T) {
	for _, operation := range []string{"status", "editor"} {
		t.Run(operation, func(t *testing.T) {
			monitors := applyBestDualMonitors()
			before, _ := json.Marshal(monitors)
			replacement := append([]hypr.Monitor(nil), monitors...)
			replacement[1].Serial = "replacement-on-the-same-connector"
			after, _ := json.Marshal(replacement)
			env := newApplyBestTestEnvWithMonitors(t, string(before), string(after))
			saved := profile.FromMonitors("Template", monitors)
			if err := env.store.Save(saved); err != nil {
				t.Fatal(err)
			}
			helper := filepath.Join(filepath.Dir(env.logPath), "hyprctl")
			script, err := os.ReadFile(helper)
			if err != nil {
				t.Fatal(err)
			}
			query := `if [[ "${1-}" == "-j" && "${2-}" == "workspacerules" ]]; then`
			script = []byte(strings.Replace(string(script), query, query+"\n  touch \"$HYPRCTL_STATE\"", 1))
			if err := os.WriteFile(helper, script, 0755); err != nil {
				t.Fatal(err)
			}
			svc := New(env.client, env.store, Config{})
			switch operation {
			case "status":
				_, err = svc.Status()
			case "editor":
				_, err = svc.EditorState()
			}
			if !errors.Is(err, ipc.ErrCompositorBusy) {
				t.Fatalf("%s returned an internally mixed hardware snapshot: %v", operation, err)
			}
			// A later retry can use the settled replacement without another event.
			document, err := svc.EditorState()
			if err != nil || document.MonitorSetHash != appstatus.HardwareSnapshotHash(replacement) {
				t.Fatalf("settled snapshot did not recover: hash=%q error=%v", document.MonitorSetHash, err)
			}
		})
	}
}

func TestRunConsumesMonitorEventsWhileCompositorReadIsBlocked(t *testing.T) {
	runtimeDir, err := os.MkdirTemp("", "hmc-events-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(runtimeDir)
	socketDir := filepath.Join(runtimeDir, "hypr", "sig-test")
	if err := os.MkdirAll(socketDir, 0700); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", filepath.Join(socketDir, ".socket2.sock"))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			accepted <- conn
		}
	}()
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	monitors := []hypr.Monitor{{Name: "eDP-1", Make: "Example", Model: "Laptop", Width: 1920, Height: 1080, RefreshRate: 60, Scale: 1, DPMSStatus: true}}
	env := newRunTestEnv(t, monitors)
	defer env.stop()
	waitFor(t, time.Second, func() bool { return env.logs.contains("applied profile: Home") }, "startup reconciliation completion")
	var conn net.Conn
	select {
	case conn = <-accepted:
	case <-time.After(time.Second):
		t.Fatal("event listener not connected")
	}
	defer conn.Close()
	helper := filepath.Join(filepath.Dir(env.logPath), "hyprctl")
	source, err := os.ReadFile(helper)
	if err != nil {
		t.Fatal(err)
	}
	replacement := strings.Replace(string(source), `  cat "$HYPRCTL_MONITORS"`, `  printf 'slow-monitor\n' >> "$HYPRCTL_LOG"
  exec sleep 20`, 1)
	if err := os.WriteFile(helper+".next", []byte(replacement), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(helper+".next", helper); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Write([]byte("monitoradded>>DP-1\n")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, time.Second, func() bool { return hyprctlLogContains(env.logPath, "slow-monitor") }, "blocked compositor probe")
	if _, err := conn.Write([]byte("monitoradded>>DP-2\n")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 150*time.Millisecond, func() bool { return env.logs.contains("monitor event received: monitoradded connector=DP-2") }, "event consumed before slow read timeout")
}

func TestRunPollDoesNotWaitForInteractiveWriter(t *testing.T) {
	monitors := []hypr.Monitor{{Name: "eDP-1", Make: "Example", Model: "Laptop", Width: 1920, Height: 1080, RefreshRate: 60, Scale: 1, DPMSStatus: true}}
	env := newRunTestEnvConfigured(t, monitors, func(cfg *Config) { cfg.PollInterval = 10 * time.Millisecond })
	waitFor(t, time.Second, func() bool { return env.logs.contains("applied profile: Home") }, "startup completion")
	env.svc.writeMu.Lock()
	// Allow several successful poll responses to reach the locked writer.
	time.Sleep(60 * time.Millisecond)
	stopped := make(chan struct{})
	go func() { env.stop(); close(stopped) }()
	select {
	case <-stopped:
		env.svc.writeMu.Unlock()
	case <-time.After(250 * time.Millisecond):
		env.svc.writeMu.Unlock()
		<-stopped
		t.Fatal("poll blocked cancellation behind interactive writer")
	}
}

func TestRunRetriesTransientWorkspaceTimeoutWithoutNewMonitorEvent(t *testing.T) {
	monitors := []hypr.Monitor{{Name: "eDP-1", Make: "Example", Model: "Laptop", Width: 1920, Height: 1080, RefreshRate: 60, Scale: 1, DPMSStatus: true}}
	env := newRunTestEnvConfigured(t, monitors, func(cfg *Config) {
		cfg.QueryTimeout = 35 * time.Millisecond
		cfg.Debounce = 20 * time.Millisecond
		helper := filepath.Join(filepath.Dir(os.Getenv("HYPRCTL_LOG")), "hyprctl")
		source, err := os.ReadFile(helper)
		if err != nil {
			t.Fatal(err)
		}
		marker := helper + ".first-workspace"
		if err := os.WriteFile(marker, []byte("1"), 0600); err != nil {
			t.Fatal(err)
		}
		before := `if [[ "${1-}" == "-j" && "${2-}" == "workspacerules" ]]; then`
		after := before + "\n  if [[ -f '" + marker + "' ]]; then rm '" + marker + "'; exec sleep 20; fi"
		if err := os.WriteFile(helper, []byte(strings.Replace(string(source), before, after, 1)), 0755); err != nil {
			t.Fatal(err)
		}
	})
	defer env.stop()
	waitFor(t, time.Second, func() bool { return env.logs.contains("applied profile: Home") }, "automatic retry after workspace read timeout")
	if !env.logs.contains("compositor query operation=workspace-rules") {
		t.Fatal("fixture did not exercise the query timeout")
	}
}

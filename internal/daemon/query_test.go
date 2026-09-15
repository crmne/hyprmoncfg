package daemon

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/ipc"
	"github.com/crmne/hyprmoncfg/internal/lid"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

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

func TestUnfamiliarSetupSkipsWorkspaceReadAndAllWrites(t *testing.T) {
	env := newApplyBestTestEnvWithMonitors(t, applyBestDualBeforeJSON, applyBestDualBeforeJSON)
	monitors := applyBestDualMonitors()
	other := hypr.Monitor{Name: "DP-9", Make: "Other", Model: "Desk", Width: 1920, Height: 1080, Scale: 1}
	if err := env.store.Save(profile.FromMonitors("Other", []hypr.Monitor{monitors[0], other})); err != nil {
		t.Fatal(err)
	}
	svc := New(env.client, env.store, Config{MonitorsConf: env.monitorsConfPath, HyprConfig: env.hyprlandConfigPath})
	svc.readLid = func(context.Context) (lid.State, error) { return lid.Open, nil }
	before := readMonitorsConf(t, env)
	if err := svc.applyBest(context.Background()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(env.logPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "workspacerules") || strings.Contains(string(data), "reload") || readMonitorsConf(t, env) != before {
		t.Fatalf("unknown fast path read workspaces or wrote displays: %s", data)
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

func TestReuseProfileReturnsDraftWithoutChangingConfigOrStore(t *testing.T) {
	env := newApplyBestTestEnvWithMonitors(t, applyBestDualBeforeJSON, applyBestDualBeforeJSON)
	monitors := applyBestDualMonitors()
	saved := profile.FromMonitors("Template", monitors)
	saved.Exec = "must-not-run"
	if err := env.store.Save(saved); err != nil {
		t.Fatal(err)
	}
	before := readMonitorsConf(t, env)
	svc := New(env.client, env.store, Config{MonitorsConf: env.monitorsConfPath, HyprConfig: env.hyprlandConfigPath})
	editor, err := svc.EditorState()
	if err != nil {
		t.Fatal(err)
	}
	if len(editor.Capabilities) != 1 || editor.Capabilities[0] != "reuse_profile" {
		t.Fatalf("capability missing: %+v", editor.Capabilities)
	}
	mapping := map[string]string{}
	for _, output := range saved.Outputs {
		mapping[output.Key] = output.Key
	}
	draft, err := svc.ReuseProfile(ipc.ReuseParams{Name: saved.Name, Mapping: mapping})
	if err != nil {
		t.Fatal(err)
	}
	if draft.Profile.Name != "" || draft.Profile.Exec != "" || len(draft.Profile.Outputs) != len(monitors) {
		t.Fatalf("invalid draft: %+v", draft)
	}
	after, err := env.store.Load(saved.Name)
	if err != nil || after.Exec != "must-not-run" || readMonitorsConf(t, env) != before || svc.pending != nil {
		t.Fatalf("reuse changed state: %v", err)
	}
	data, _ := os.ReadFile(env.logPath)
	if strings.Contains(string(data), "reload") || strings.Contains(string(data), "must-not-run") {
		t.Fatalf("reuse applied commands: %s", data)
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

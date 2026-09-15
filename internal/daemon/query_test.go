package daemon

import (
	"context"
	"encoding/json"
	"errors"
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
	status, err := svc.Status()
	if err != nil || draft.MonitorSetHash == "" || draft.MonitorSetHash != editor.MonitorSetHash || draft.MonitorSetHash != status.MonitorSetHash {
		t.Fatalf("status/editor/reuse hardware snapshots differ: status=%q editor=%q reuse=%q error=%v", status.MonitorSetHash, editor.MonitorSetHash, draft.MonitorSetHash, err)
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

func TestHardwareSnapshotsRejectReplacementDuringWorkspaceRead(t *testing.T) {
	for _, operation := range []string{"status", "editor", "reuse"} {
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
			case "reuse":
				mapping := map[string]string{}
				for _, output := range saved.Outputs {
					mapping[output.Key] = output.Key
				}
				_, err = svc.ReuseProfile(ipc.ReuseParams{Name: saved.Name, Mapping: mapping})
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

func TestReuseProfileRecoversCurrentProfileCalibration(t *testing.T) {
	monitors := applyBestDualMonitors()
	data, _ := json.Marshal(monitors)
	env := newApplyBestTestEnvWithMonitors(t, string(data), string(data))
	current := profile.FromMonitors("Current", monitors)
	for i := range current.Outputs {
		current.Outputs[i].ICC = "/current/" + current.Outputs[i].Name + ".icc"
	}
	template := profile.FromMonitors("Template", monitors)
	for i := range template.Outputs {
		template.Outputs[i].Serial = "foreign-" + template.Outputs[i].Name
		template.Outputs[i].ICC = "/foreign/calibration.icc"
	}
	template.Normalize()
	for _, saved := range []profile.Profile{current, template} {
		if err := env.store.Save(saved); err != nil {
			t.Fatal(err)
		}
	}
	mapping := map[string]string{}
	for i, output := range template.Outputs {
		mapping[output.Key] = current.Outputs[i].Key
	}
	svc := New(env.client, env.store, Config{})
	draft, err := svc.ReuseProfile(ipc.ReuseParams{Name: template.Name, Mapping: mapping})
	if err != nil {
		t.Fatal(err)
	}
	for _, output := range draft.Profile.Outputs {
		stored, _ := current.OutputByKey(output.Key)
		if output.ICC != stored.ICC {
			t.Fatalf("daemon failed to recover target calibration: %+v", output)
		}
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

package daemon

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/crmne/hyprmoncfg/internal/config"
	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/lid"
	"github.com/crmne/hyprmoncfg/internal/profile"
	"github.com/crmne/hyprmoncfg/internal/render"
)

func TestOpenInternalRecoveryPreservesChoiceAndGeometry(t *testing.T) {
	p := profile.Profile{Name: "desk", Exec: "must-not-run", Outputs: []profile.OutputConfig{
		{Key: "edp-1", Name: "eDP-1", Enabled: true, Width: 3456, Height: 2234, Mode: "3456x2234@120", Scale: 2, X: 1068, Y: -1117, MirrorOf: "hdmi"},
		{Key: "hdmi", Name: "HDMI-A-1", Enabled: true},
	}}
	for _, state := range []lid.State{lid.Closed, lid.Unknown} {
		if got := openInternalProfile(p, state); len(got.Outputs) != 0 {
			t.Fatalf("recovered panel with lid %s", state)
		}
	}
	got := openInternalProfile(p, lid.Open)
	if len(got.Outputs) != 1 || got.Exec != "" || got.Outputs[0].MirrorOf != "" {
		t.Fatalf("unsafe recovery profile: %+v", got)
	}
	code, err := render.RenderConfig(got, render.ProfileMonitors(got), render.Options{Format: config.HyprConfigLua, UseMonitorV2: true})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(code, "HDMI") || !strings.Contains(code, `position = "1068x-1117"`) || !strings.Contains(code, "scale = 2") {
		t.Fatalf("incorrect panel recovery: %s", code)
	}
	p.Outputs[0].Enabled = false
	if len(openInternalProfile(p, lid.Open).Outputs) != 0 {
		t.Fatal("overrode deliberately disabled panel")
	}
}

func TestWakeUsesLastKnownHardwareIdentity(t *testing.T) {
	monitors := []hypr.Monitor{{Name: "eDP-2", Make: "BOE", Model: "Panel", Serial: "123", Width: 2880, Height: 1800, Scale: 1.5, DPMSStatus: true}}
	p := profile.FromMonitors("desk", monitors)
	p.Outputs[0].Name = "old-connector"
	store := profile.NewStore(t.TempDir())
	if err := store.Save(p); err != nil {
		t.Fatal(err)
	}
	svc := New(nil, store, Config{})
	svc.lastProfile = p
	svc.applied = rememberApplied(p, monitors)
	svc.writeMu.Lock()
	recovered := openInternalProfile(svc.wakeProfile(), lid.Open)
	svc.writeMu.Unlock()
	if len(recovered.Outputs) != 1 || recovered.Outputs[0].Name != "eDP-2" {
		t.Fatalf("lost hardware mapping: %+v", recovered)
	}
	if svc.lastProfile.Outputs[0].Name != "old-connector" {
		t.Fatal("mutated selected profile while resolving recovery")
	}
}

// Reproduce the observed failure: after suspend, the internal output remains
// clamshell-disabled and the only enabled output reports DPMS off. A regular
// idle observation must not veto the explicit request to recover from sleep.
func TestResumeRecoversDisabledPanelDespiteSleepingHDMI(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "eDP-1", Width: 2880, Height: 1800, RefreshRate: 120, Scale: 1.5, DPMSStatus: true},
		{Name: "HDMI-A-1", Width: 2560, Height: 1440, RefreshRate: 60, X: 1920, Scale: 1, DPMSStatus: true},
	}
	env := newRunTestEnv(t, monitors)
	defer env.stop()
	waitFor(t, time.Second, func() bool { return env.logs.contains("applied profile") }, "startup apply")
	env.suspendEvents <- true
	broken := append([]hypr.Monitor(nil), monitors...)
	broken[0].Disabled = true
	broken[1].DPMSStatus = false
	if err := writeMonitorState(env.monitorStatePath, broken); err != nil {
		t.Fatal(err)
	}
	if err := writeMonitorState(env.monitorStatePath+".next", monitors); err != nil {
		t.Fatal(err)
	}
	env.setLid(lid.Open)
	env.suspendEvents <- false
	waitFor(t, 2*time.Second, func() bool { return env.logs.contains("display wake recovery complete") }, "recovery of sleeping outputs")
	state, err := env.svc.client.Monitors(context.Background())
	if err != nil || len(state) != 2 || state[0].Disabled || !state[1].DPMSStatus {
		t.Fatalf("outputs were not restored: %+v (%v)", state, err)
	}
	data, _ := os.ReadFile(env.logPath)
	commands := string(data)
	restore := strings.Index(commands, "keyword monitor eDP-1")
	wake := strings.Index(commands, "dispatch dpms on")
	if restore < 0 || wake < restore {
		t.Fatalf("internal panel must be restored before global DPMS: %s", commands)
	}
	if env.logs.contains("display sleep detected") {
		t.Fatalf("failed wake mistaken for idle: %s", env.logs.all())
	}
	before := strings.Count(commands, "dispatch dpms on")
	time.Sleep(4 * env.svc.cfg.WakeSettle)
	data, _ = os.ReadFile(env.logPath)
	if strings.Count(string(data), "dispatch dpms on") != before {
		t.Fatal("continued waking displays after recovery")
	}
}

func TestWakeRetriesStopOnExpirySuspendLidCloseAndUnmanage(t *testing.T) {
	for _, action := range []string{"expiry", "suspend", "lid-close", "unmanage"} {
		t.Run(action, func(t *testing.T) {
			base := t.TempDir()
			monitors := []hypr.Monitor{{Name: "eDP-1", Width: 2880, Height: 1800, RefreshRate: 120, Scale: 1.5, DPMSStatus: true}}
			env := newRunTestEnv(t, monitors, func(cfg *Config) { cfg.ConfigDir = base; cfg.WakeRecoveryTimeout = 500 * time.Millisecond })
			defer env.stop()
			waitFor(t, time.Second, func() bool { return env.logs.contains("applied profile") }, "startup apply")
			if err := os.WriteFile(env.monitorStatePath+".fail", nil, 0600); err != nil {
				t.Fatal(err)
			}
			env.suspendEvents <- false
			waitFor(t, time.Second, func() bool { return env.logs.contains("apply failed") }, "failed resume")
			switch action {
			case "suspend":
				env.suspendEvents <- true
				waitFor(t, time.Second, func() bool { return env.logs.contains("stopped display wake recovery") }, "suspend cancellation")
			case "lid-close":
				env.setLid(lid.Closed)
				env.lidStates <- lid.Closed
				waitFor(t, time.Second, func() bool { return env.logs.contains("triggered: lid:closed") }, "closed lid")
			case "unmanage":
				if err := config.SetManaged(base, false); err != nil {
					t.Fatal(err)
				}
				fallthrough
			case "expiry":
				waitFor(t, time.Second, func() bool { return env.logs.contains("recovery window expired") }, "bounded recovery")
			}
			before, _ := os.ReadFile(env.logPath)
			time.Sleep(4 * env.svc.cfg.WakeSettle)
			after, _ := os.ReadFile(env.logPath)
			if strings.Count(string(before), "dispatch dpms on") != strings.Count(string(after), "dispatch dpms on") {
				t.Fatal("continued waking after " + action)
			}
		})
	}
}

func TestWakePreservesActivePreviewAndUnmanagedConfiguration(t *testing.T) {
	for _, managed := range []bool{true, false} {
		t.Run(map[bool]string{true: "preview", false: "unmanaged"}[managed], func(t *testing.T) {
			dir := t.TempDir()
			log := filepath.Join(dir, "commands")
			// Every display operation is captured. This fixture cannot access the desktop.
			if err := os.WriteFile(filepath.Join(dir, "hyprctl"), []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$WAKE_COMMAND_LOG\"\ncase \"$*\" in *version*) echo '{\"version\":\"0.56.2\"}';; esac\n"), 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("WAKE_COMMAND_LOG", log)
			t.Setenv("HYPRLAND_INSTANCE_SIGNATURE", "wake-test")
			root := filepath.Join(dir, "hyprland.lua")
			if err := os.WriteFile(root, nil, 0600); err != nil {
				t.Fatal(err)
			}
			client, err := hypr.NewClient()
			if err != nil {
				t.Fatal(err)
			}
			store := profile.NewStore(dir)
			svc := New(client, store, Config{ConfigDir: dir, HyprConfig: root, MonitorsConf: filepath.Join(dir, "monitors.lua")})
			svc.lidState = lid.Open
			svc.lastProfile = profile.Profile{Name: "old", Outputs: []profile.OutputConfig{{Key: "edp-1", Name: "eDP-1", Enabled: true, Width: 2880, Height: 1800, Scale: 1.5}}}
			if managed {
				svc.pending = &pendingTransaction{}
			} else if err := config.SetManaged(dir, false); err != nil {
				t.Fatal(err)
			}
			svc.wakeDisplays(context.Background())
			data, _ := os.ReadFile(log)
			if strings.Contains(string(data), "eval") || strings.Contains(string(data), "keyword") {
				t.Fatalf("overwrote display ownership: %s", data)
			}
			if !managed && len(data) > 0 {
				t.Fatalf("issued display commands while unmanaged: %s", data)
			}
			if managed && !strings.Contains(string(data), "dpms") {
				t.Fatal("did not wake the active preview")
			}
		})
	}
}

func TestResumeRetriesAfterIPCFailureWithoutAnotherHardwareEvent(t *testing.T) {
	monitors := []hypr.Monitor{{Name: "eDP-1", Width: 2880, Height: 1800, RefreshRate: 120, Scale: 1.5, DPMSStatus: true}}
	env := newRunTestEnv(t, monitors)
	defer env.stop()
	waitFor(t, time.Second, func() bool { return env.logs.contains("applied profile") }, "startup apply")
	if err := os.WriteFile(env.monitorStatePath+".fail", nil, 0600); err != nil {
		t.Fatal(err)
	}
	env.suspendEvents <- false
	waitFor(t, time.Second, func() bool { return env.logs.contains("apply failed") }, "initial IPC failure")
	if !hyprctlLogContains(env.logPath, "keyword monitor eDP-1") {
		t.Fatal("panel recovery waited for broken monitor query")
	}
	if err := os.Remove(env.monitorStatePath + ".fail"); err != nil {
		t.Fatal(err)
	}
	// No lid event, monitor event, or regular poll follows. Only the retry can recover.
	waitFor(t, 2*time.Second, func() bool { return env.logs.contains("display wake recovery complete") }, "independent retry")
}

func TestWakeKeepsDetectedLuaDialectWhenIPCIsUnavailable(t *testing.T) {
	dir := t.TempDir()
	hyprDir := filepath.Join(dir, "hypr")
	if err := os.MkdirAll(hyprDir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"hyprland.lua", "hyprland.conf"} {
		if err := os.WriteFile(filepath.Join(hyprDir, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HYPRLAND_CONFIG", "")
	t.Setenv("HYPRMONCFG_MONITORS_CONF", "")
	t.Setenv("HYPRLAND_INSTANCE_SIGNATURE", "test-wake")
	t.Setenv("WAKE_TEST_DIR", dir)
	script := `#!/bin/bash
for ((i=1; i<=$#; i++)); do
  case ${!i} in
    version)
      [[ ! -e "$WAKE_TEST_DIR/fail" ]] || exit 1
      echo '{"version":"0.56.2"}'
      exit 0
      ;;
    eval)
      ((i++))
      code=${!i}
      # hyprctl parses an argument starting with -- as an option, even after eval.
      [[ $code != -* ]] || exit 2
      printf '%s' "$code" > "$WAKE_TEST_DIR/eval"
      echo ok
      exit 0
      ;;
    keyword) exit 3 ;;
  esac
done
exit 4
`
	if err := os.WriteFile(filepath.Join(dir, "hyprctl"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	client, err := hypr.NewClient()
	if err != nil {
		t.Fatal(err)
	}
	svc := New(client, profile.NewStore(t.TempDir()), Config{})
	if _, err := svc.resolveHyprConfig(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "fail"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	svc.lidState = lid.Open
	svc.lastProfile = profile.Profile{Outputs: []profile.OutputConfig{{Name: "eDP-1", Enabled: true, Width: 2880, Height: 1800, Scale: 1.5}}}
	svc.writeMu.Lock()
	err = svc.restoreOpenInternal(context.Background())
	svc.writeMu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	code, err := os.ReadFile(filepath.Join(dir, "eval"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(code), "do\n") || !strings.Contains(string(code), `output = "eDP-1"`) {
		t.Fatalf("incorrect Lua recovery command: %s", code)
	}
}

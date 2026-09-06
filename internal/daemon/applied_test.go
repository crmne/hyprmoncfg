package daemon

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/lid"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

func wakeMonitors() []hypr.Monitor {
	return []hypr.Monitor{
		{Name: "eDP-1", Make: "Framework", Model: "Panel", Width: 2256, Height: 1504, RefreshRate: 60,
			Scale: 1.56667, DPMSStatus: true, ColorManagementPreset: "srgb", CurrentFormat: "XRGB8888"},
		{Name: "DP-4", Make: "HP", Model: "E223", Serial: "left", Width: 1920, Height: 1080, RefreshRate: 60,
			X: 1440, Scale: 1, DPMSStatus: true},
		{Name: "DP-5", Make: "HP", Model: "E223", Serial: "right", Width: 1920, Height: 1080, RefreshRate: 60,
			X: 3360, Scale: 1, DPMSStatus: true},
	}
}

func TestAppliedStateIgnoresReadbackNoiseButNotConfigurationChanges(t *testing.T) {
	monitors := wakeMonitors()
	requested := profile.FromMonitors("Dock", monitors)
	requested.Outputs[0].CM = "auto"
	state := rememberApplied(requested, monitors)

	monitors[0].Scale = 1.57
	monitors[0].VRR = 1
	monitors[0].CurrentFormat = "ARGB8888"
	if !state.matches(requested, monitors, nil) {
		t.Fatal("rounded scale, VRR activity, or equivalent buffer format invalidated the applied state")
	}
	for _, edit := range []struct {
		name string
		edit func(*profile.Profile)
	}{
		{"ICC", func(p *profile.Profile) { p.Outputs[0].ICC = "/display.icc" }},
		{"VRR mode", func(p *profile.Profile) { p.Outputs[0].VRR = 2 }},
		{"HDR metadata", func(p *profile.Profile) { p.Outputs[0].MaxLuminance = 1000 }},
		{"hook", func(p *profile.Profile) { p.Exec = "true" }},
		{"profile identity", func(p *profile.Profile) { p.Name = "Another dock" }},
	} {
		t.Run(edit.name, func(t *testing.T) {
			changed := requested
			changed.Outputs = append([]profile.OutputConfig(nil), requested.Outputs...)
			edit.edit(&changed)
			if state.matches(changed, monitors, nil) {
				t.Fatal("a changed request was ignored because the readback looked the same")
			}
		})
	}
	monitors[0].X = 100
	if state.matches(requested, monitors, nil) {
		t.Fatal("real layout drift was ignored")
	}
}

func TestAppliedStateRetainsPreciseScaleAfterRoundedFirstReadback(t *testing.T) {
	monitors := wakeMonitors()
	requested := profile.FromMonitors("Dock", monitors)
	monitors[0].Scale = 1.57
	state := rememberApplied(requested, monitors)
	monitors[0].Scale = 1.56667
	if !state.matches(requested, monitors, nil) {
		t.Fatal("more precise readback caused another apply")
	}
}

func TestApplyBestDoesNotReloadForPostWakeReadbackChanges(t *testing.T) {
	monitors := wakeMonitors()
	data, _ := json.Marshal(monitors)
	env := newApplyBestTestEnvWithMonitors(t, string(data), string(data))
	requested := profile.FromMonitors("Dock", monitors)
	if err := env.store.Save(requested); err != nil {
		t.Fatal(err)
	}
	svc := New(env.client, env.store, Config{MonitorsConf: env.monitorsConfPath, HyprConfig: env.hyprlandConfigPath})
	if err := svc.applyBest(context.Background()); err != nil {
		t.Fatal(err)
	}
	readback := filepath.Join(t.TempDir(), "monitors.json")
	t.Setenv("HYPRCTL_MONITORS_OVERRIDE", readback)
	for i := 0; i < 6; i++ {
		monitors[0].VRR = hypr.VRRMode(i % 2)
		monitors[0].Scale = []float64{1.56667, 1.57}[i%2]
		monitors[0].CurrentFormat = []string{"XRGB8888", "ARGB8888"}[i%2]
		if err := writeMonitorState(readback, monitors); err != nil {
			t.Fatal(err)
		}
		if err := svc.applyBest(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	log, err := os.ReadFile(env.logPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(log), "\nreload\n"); got != 1 {
		t.Fatalf("got %d reloads, want the initial apply only:\n%s", got, log)
	}
	// Config-only edits must still be applied with identical live monitor data.
	requested.Outputs[0].VRR = 2
	if err := env.store.Save(requested); err != nil {
		t.Fatal(err)
	}
	if err := svc.applyBest(context.Background()); err != nil {
		t.Fatal(err)
	}
	log, _ = os.ReadFile(env.logPath)
	if got := strings.Count(string(log), "\nreload\n"); got != 2 {
		t.Fatalf("changed VRR configuration did not reload: %d", got)
	}
}

func TestApplyBestPreservesRecognizedAutoColorLayout(t *testing.T) {
	monitors := wakeMonitors()[:2]
	monitors[1].Disabled = true
	data, _ := json.Marshal(monitors)
	env := newApplyBestTestEnvWithMonitors(t, string(data), string(data))
	desk := profile.FromMonitors("Desk", monitors)
	desk.Outputs[0].CM = "auto"
	mirror := profile.FromMonitors("A mirror", monitors)
	mirror.Outputs[1].Enabled = true
	mirror.Outputs[1].MirrorOf = mirror.Outputs[0].Key
	for _, saved := range []profile.Profile{desk, mirror} {
		if err := env.store.Save(saved); err != nil {
			t.Fatal(err)
		}
	}
	svc := New(env.client, env.store, Config{MonitorsConf: env.monitorsConfPath, HyprConfig: env.hyprlandConfigPath})
	for i := 0; i < 2; i++ {
		if err := svc.applyBest(context.Background()); err != nil {
			t.Fatal(err)
		}
		if svc.lastProfile.Name != "Desk" {
			t.Fatalf("replaced a recognized layout with %q", svc.lastProfile.Name)
		}
	}
}

func TestOpeningTheLidRematchesInsteadOfKeepingTheClosedLayout(t *testing.T) {
	open := wakeMonitors()[:2]
	closed := append([]hypr.Monitor(nil), open...)
	closed[0].Disabled = true
	before, _ := json.Marshal(closed)
	after, _ := json.Marshal(open)
	env := newApplyBestTestEnvWithMonitors(t, string(before), string(after))
	full := profile.FromMonitors("Open desk", open)
	clamshell := profile.FromMonitors("Closed desk", closed)
	for _, saved := range []profile.Profile{full, clamshell} {
		if err := env.store.Save(saved); err != nil {
			t.Fatal(err)
		}
	}
	svc := New(env.client, env.store, Config{MonitorsConf: env.monitorsConfPath, HyprConfig: env.hyprlandConfigPath})
	svc.lastProfile, svc.lastMonitorSet = clamshell, profile.MonitorSetHash(closed)
	svc.applied = rememberApplied(clamshell, closed)
	svc.lastLidState, svc.lidState = lid.Closed, lid.Open
	if err := svc.applyBest(context.Background()); err != nil {
		t.Fatal(err)
	}
	if svc.lastProfile.Name != full.Name {
		t.Fatalf("opening the lid kept %q", svc.lastProfile.Name)
	}
}

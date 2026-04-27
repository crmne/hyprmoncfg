package daemon

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

func TestInternalOnlyFallbackProfileEnablesInternalWhenAllOutputsDisabled(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "eDP-1", Make: "Framework", Model: "Panel", Serial: "A1", Width: 2880, Height: 1800, RefreshRate: 120, X: 3840, Scale: 1.5, Disabled: true},
	}

	got, ok := internalOnlyFallbackProfile(monitors)
	if !ok {
		t.Fatal("expected fallback profile")
	}
	if len(got.Outputs) != 1 {
		t.Fatalf("expected 1 output, got %d", len(got.Outputs))
	}

	output := got.Outputs[0]
	if !output.Enabled {
		t.Fatal("expected internal output to be enabled")
	}
	if output.Mode != "2880x1800@120.00Hz" || output.Width != 2880 || output.Height != 1800 || output.Refresh != 120 {
		t.Fatalf("unexpected fallback mode: %+v", output)
	}
	if output.X != 0 || output.Y != 0 || output.Scale != 1.5 || output.MirrorOf != "" {
		t.Fatalf("unexpected fallback placement: %+v", output)
	}
	if got.Workspaces.Enabled {
		t.Fatalf("expected fallback workspace settings to be disabled: %+v", got.Workspaces)
	}
}

func TestInternalOnlyFallbackProfileLeavesExternalOutputsDisabled(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "DP-1", Make: "Dell", Model: "U2720Q", Serial: "B1", Disabled: true},
		{Name: "eDP-1", Make: "Framework", Model: "Panel", Serial: "A1", Disabled: true},
	}

	got, ok := internalOnlyFallbackProfile(monitors)
	if !ok {
		t.Fatal("expected fallback profile")
	}

	for _, output := range got.Outputs {
		if output.Name == "DP-1" && output.Enabled {
			t.Fatalf("expected external output to stay disabled: %+v", output)
		}
		if output.Name == "eDP-1" && !output.Enabled {
			t.Fatalf("expected internal output to be enabled: %+v", output)
		}
	}
}

func TestInternalOnlyFallbackProfileDoesNotOverrideEnabledOutput(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "DP-1", Make: "Dell", Model: "U2720Q", Serial: "B1"},
		{Name: "eDP-1", Make: "Framework", Model: "Panel", Serial: "A1", Disabled: true},
	}

	if _, ok := internalOnlyFallbackProfile(monitors); ok {
		t.Fatal("did not expect fallback while an output is enabled")
	}
}

func TestInternalOnlyFallbackProfileRequiresInternalOutput(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "DP-1", Make: "Dell", Model: "U2720Q", Serial: "B1", Disabled: true},
	}

	if _, ok := internalOnlyFallbackProfile(monitors); ok {
		t.Fatal("did not expect fallback without an internal output")
	}
}

func TestApplyBestUsesInternalFallbackWhenNoProfilesAndAllOutputsDisabled(t *testing.T) {
	env := newApplyBestTestEnv(t, `[{"id":1,"name":"eDP-1","description":"Framework Panel","make":"Framework","model":"Panel","serial":"A1","width":2880,"height":1800,"refreshRate":120,"x":0,"y":0,"scale":1.5,"transform":0,"disabled":false,"mirrorOf":""}]`)

	svc := New(env.client, env.store, Config{
		MonitorsConf: env.monitorsConfPath,
		HyprConfig:   env.hyprlandConfigPath,
	})
	if err := svc.applyBest(context.Background()); err != nil {
		t.Fatalf("applyBest returned error: %v", err)
	}

	rendered := readMonitorsConf(t, env)
	for _, want := range []string{
		"output = desc:Framework Panel",
		"mode = 2880x1800@120.00",
		"position = 0x0",
		"scale = 1.5",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered config missing %q:\n%s", want, rendered)
		}
	}

	logBytes, err := os.ReadFile(env.logPath)
	if err != nil {
		t.Fatalf("read hyprctl log: %v", err)
	}
	if !strings.Contains(string(logBytes), "reload") {
		t.Fatalf("expected applyBest to reload Hyprland, got:\n%s", logBytes)
	}
}

func TestApplyBestUsesSavedProfileBeforeInternalFallback(t *testing.T) {
	env := newApplyBestTestEnv(t, `[{"id":1,"name":"eDP-1","description":"Framework Panel","make":"Framework","model":"Panel","serial":"A1","width":2880,"height":1800,"refreshRate":120,"x":100,"y":200,"scale":2,"transform":0,"disabled":false,"mirrorOf":""}]`)
	mon := hypr.Monitor{Name: "eDP-1", Description: "Framework Panel", Make: "Framework", Model: "Panel", Serial: "A1"}

	if err := env.store.Save(profile.New("saved-internal", []profile.OutputConfig{{
		Key:     mon.HardwareKey(),
		Name:    mon.Name,
		Enabled: true,
		Width:   2880,
		Height:  1800,
		Refresh: 120,
		X:       100,
		Y:       200,
		Scale:   2,
	}})); err != nil {
		t.Fatalf("save profile: %v", err)
	}

	svc := New(env.client, env.store, Config{
		MonitorsConf: env.monitorsConfPath,
		HyprConfig:   env.hyprlandConfigPath,
	})
	if err := svc.applyBest(context.Background()); err != nil {
		t.Fatalf("applyBest returned error: %v", err)
	}

	rendered := readMonitorsConf(t, env)
	for _, want := range []string{
		"output = desc:Framework Panel",
		"mode = 2880x1800@120.00",
		"position = 100x200",
		"scale = 2",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered config missing %q:\n%s", want, rendered)
		}
	}
}

type applyBestTestEnv struct {
	client             *hypr.Client
	store              *profile.Store
	logPath            string
	monitorsConfPath   string
	hyprlandConfigPath string
}

func newApplyBestTestEnv(t *testing.T, afterApplyMonitorsJSON string) applyBestTestEnv {
	t.Helper()

	dir := t.TempDir()
	logPath := filepath.Join(dir, "hyprctl.log")
	statePath := filepath.Join(dir, "applied")
	monitorsConfPath := filepath.Join(dir, "monitors.conf")
	hyprlandConfigPath := filepath.Join(dir, "hyprland.conf")
	hyprctlPath := filepath.Join(dir, "hyprctl")
	beforeApplyMonitorsJSON := `[{"id":1,"name":"eDP-1","description":"Framework Panel","make":"Framework","model":"Panel","serial":"A1","width":2880,"height":1800,"refreshRate":120,"x":3840,"y":0,"scale":1.5,"transform":0,"disabled":true,"mirrorOf":""}]`

	fakeHyprctlScript := `#!/bin/bash
set -eu

if [[ "${1-}" == "--instance" ]]; then
  shift 2
fi

printf '%s\n' "$*" >> "$HYPRCTL_LOG"

if [[ "${1-}" == "-j" && "${2-}" == "version" ]]; then
  printf '{"version":"0.54.0"}'
  exit 0
fi

if [[ "${1-}" == "-j" && "${2-}" == "monitors" && "${3-}" == "all" ]]; then
  if [[ -f "$HYPRCTL_STATE" ]]; then
    printf '%s' '` + afterApplyMonitorsJSON + `'
  else
    printf '%s' '` + beforeApplyMonitorsJSON + `'
  fi
  exit 0
fi

if [[ "${1-}" == "-j" && "${2-}" == "workspacerules" ]]; then
  printf '[]'
  exit 0
fi

if [[ "${1-}" == "-j" && "${2-}" == "workspaces" ]]; then
  printf '[]'
  exit 0
fi

if [[ "${1-}" == "reload" ]]; then
  touch "$HYPRCTL_STATE"
  exit 0
fi

echo "unexpected args: $*" >&2
exit 1
`

	if err := os.WriteFile(monitorsConfPath, []byte("# initial\n"), 0o644); err != nil {
		t.Fatalf("write monitors.conf: %v", err)
	}
	if err := os.WriteFile(hyprlandConfigPath, []byte("source = "+monitorsConfPath+"\n"), 0o644); err != nil {
		t.Fatalf("write hyprland.conf: %v", err)
	}
	if err := os.WriteFile(hyprctlPath, []byte(fakeHyprctlScript), 0o755); err != nil {
		t.Fatalf("write fake hyprctl: %v", err)
	}

	t.Setenv("HYPRCTL_LOG", logPath)
	t.Setenv("HYPRCTL_STATE", statePath)
	t.Setenv("HYPRLAND_INSTANCE_SIGNATURE", "sig-test")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	client, err := hypr.NewClient()
	if err != nil {
		t.Fatalf("new hypr client: %v", err)
	}

	return applyBestTestEnv{
		client:             client,
		store:              profile.NewStore(dir),
		logPath:            logPath,
		monitorsConfPath:   monitorsConfPath,
		hyprlandConfigPath: hyprlandConfigPath,
	}
}

func readMonitorsConf(t *testing.T, env applyBestTestEnv) string {
	t.Helper()

	rendered, err := os.ReadFile(env.monitorsConfPath)
	if err != nil {
		t.Fatalf("read monitors.conf: %v", err)
	}
	return string(rendered)
}

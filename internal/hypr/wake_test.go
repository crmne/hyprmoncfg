package hypr

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWakeDisplaysTargetsOnlyEnabledOutputs(t *testing.T) {
	for _, tc := range []struct {
		name        string
		luaDispatch bool
		want        string
	}{
		{name: "legacy", want: "dispatch dpms on DP-1"},
		{name: "lua", luaDispatch: true, want: `dispatch hl.dsp.dpms({ action = "enable", monitor = "DP-1" })`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logPath := filepath.Join(t.TempDir(), "hyprctl.log")
			hyprctlPath := filepath.Join(t.TempDir(), "hyprctl")
			script := `#!/bin/sh
printf '%s\n' "$*" >> "` + logPath + `"
`
			if err := os.WriteFile(hyprctlPath, []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("HYPRLAND_INSTANCE_SIGNATURE", "sig-test")

			monitors := []Monitor{
				{Name: "eDP-1", Disabled: true},
				{Name: "DP-1"},
				{Name: "HDMI-A-1", Disabled: true},
			}
			client := &Client{hyprctl: hyprctlPath}
			if err := client.WakeDisplays(context.Background(), monitors, tc.luaDispatch); err != nil {
				t.Fatal(err)
			}

			calls, err := os.ReadFile(logPath)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(calls), tc.want) {
				t.Fatalf("missing targeted wake %q in calls:\n%s", tc.want, calls)
			}
			if strings.Contains(string(calls), "eDP-1") || strings.Contains(string(calls), "HDMI-A-1") {
				t.Fatalf("woke a disabled output:\n%s", calls)
			}
		})
	}
}

func TestWakeDisplaysDoesNothingWhenAllOutputsAreDisabled(t *testing.T) {
	hyprctlPath := filepath.Join(t.TempDir(), "hyprctl")
	if err := os.WriteFile(hyprctlPath, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HYPRLAND_INSTANCE_SIGNATURE", "sig-test")

	client := &Client{hyprctl: hyprctlPath}
	if err := client.WakeDisplays(context.Background(), []Monitor{{Name: "eDP-1", Disabled: true}}, false); err != nil {
		t.Fatalf("disabled outputs should not be woken: %v", err)
	}
}

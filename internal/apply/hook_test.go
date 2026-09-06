package apply

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

func TestPostApplyHookUsesTheSelectedCompositorWithoutSessionEnvironment(t *testing.T) {
	dir := t.TempDir()
	result := filepath.Join(dir, "dispatch")
	script := `#!/bin/sh
set -eu
if [ "$1" = "-j" ] && [ "$2" = "instances" ]; then
  printf '[{"instance":"selected-session","wl_socket":"wayland-9"}]'
else
  test "$HYPRLAND_INSTANCE_SIGNATURE" = selected-session
  test "$1" = dispatch
  printf '%s' "$2" > "$HYPRMONCFG_TEST_DISPATCH"
fi
`
	if err := os.WriteFile(filepath.Join(dir, "hyprctl"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("HYPRLAND_INSTANCE_SIGNATURE", "")
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("DISPLAY", "")
	t.Setenv("HYPRMONCFG_TEST_DISPATCH", result)
	client, err := hypr.NewClient()
	if err != nil {
		t.Fatal(err)
	}
	engine := Engine{Client: client}
	// The documented Exec recipe delegates DISPLAY to the selected compositor.
	p := profile.Profile{Exec: `hyprctl dispatch 'hl.dsp.exec_cmd("xrandr --output HDMI-A-1 --primary")'`}
	if err := engine.PostApply(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(result); err != nil || string(got) != `hl.dsp.exec_cmd("xrandr --output HDMI-A-1 --primary")` {
		t.Fatalf("wrong hook dispatch: %s (%v)", got, err)
	}
}

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

func TestWakeKeepsLuaSyntaxWhenHyprlandIsTooBusyToAnswer(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("HYPRLAND_CONFIG", "")
	t.Setenv("HYPRMONCFG_MONITORS_CONF", "")
	if err := os.MkdirAll(filepath.Join(home, ".config/hypr"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".config/hypr/hyprland.lua"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(dir, "hyprctl.log")
	busy := filepath.Join(dir, "busy")
	script := `#!/bin/bash
if [[ "${1-}" == "--instance" ]]; then shift 2; fi
printf '%s\n' "$*" >> "` + log + `"
if [[ "${1-}" == "-j" && "${2-}" == "version" ]]; then
  [[ -f "` + busy + `" ]] && exit 1
  printf '{"version":"0.56.2"}'
  exit 0
fi
printf 'ok'
`
	if err := os.WriteFile(filepath.Join(dir, "hyprctl"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("HYPRLAND_INSTANCE_SIGNATURE", "sig-test")
	client, err := hypr.NewClient()
	if err != nil {
		t.Fatal(err)
	}
	svc := New(client, profile.NewStore(filepath.Join(dir, "profiles")), Config{})

	svc.wakeDisplays(context.Background())
	if err := os.WriteFile(busy, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(log, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	svc.wakeDisplays(context.Background())

	calls, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(calls), "hl.dsp.dpms") || strings.Contains(string(calls), "dispatch dpms on") {
		t.Fatalf("a busy Lua-mode Hyprland should still get the Lua wake command, got:\n%s", calls)
	}
}

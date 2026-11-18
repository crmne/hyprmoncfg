package daemon

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crmne/hyprmoncfg/internal/config"
	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

func TestWakeKeepsLuaSyntaxWhenHyprlandIsTooBusyToAnswer(t *testing.T) {
	for _, tc := range []struct {
		name             string
		unmanagedStartup bool
	}{
		{name: "previous-wake"},
		{name: "unmanaged-startup", unmanagedStartup: true},
	} {
		t.Run(tc.name, func(t *testing.T) {

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
			cfg := Config{ConfigDir: t.TempDir()}
			if tc.unmanagedStartup {
				if err := config.SetManaged(cfg.ConfigDir, false); err != nil {
					t.Fatal(err)
				}
			}
			svc := New(client, profile.NewStore(filepath.Join(dir, "profiles")), cfg)

			if tc.unmanagedStartup {
				svc.ensureConfigInclude(context.Background())
			} else {
				svc.wakeDisplays(context.Background())
			}
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

		})
	}
}

// luaDispatchRejection is what Hyprland 0.56.2 printed for issue #125.
const luaDispatchRejection = `error: [string "return hl.dispatch(dpms on)"]:1: ')' expected near 'on'
  → Note: dispatch in lua is a shorthand for hl.dispatch(...), your syntax might need to be updated.`

// newBusyWakeService builds a daemon whose Hyprland never answers the version
// query, so no dialect is ever detected. When lua is true, hyprctl rejects
// classic dispatches the way Hyprland 0.56 does on a Lua config.
func newBusyWakeService(t *testing.T, lua bool, hyprConfig string) (*Service, string) {
	t.Helper()
	dir := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("HYPRLAND_CONFIG", "")
	t.Setenv("HYPRMONCFG_MONITORS_CONF", "")
	log := filepath.Join(dir, "hyprctl.log")
	rejection := filepath.Join(dir, "rejection")
	if err := os.WriteFile(rejection, []byte(luaDispatchRejection), 0o644); err != nil {
		t.Fatal(err)
	}
	reject := ""
	if lua {
		reject = `if [[ "${1-}" == "dispatch" && "${2-}" != hl.* ]]; then cat "` + rejection + `"; exit 7; fi
`
	}
	script := `#!/bin/bash
if [[ "${1-}" == "--instance" ]]; then shift 2; fi
printf '%s\n' "$*" >> "` + log + `"
if [[ "${1-}" == "-j" && "${2-}" == "version" ]]; then exit 1; fi
` + reject + `printf 'ok'
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
	svc := New(client, profile.NewStore(filepath.Join(dir, "profiles")), Config{ConfigDir: t.TempDir(), HyprConfig: hyprConfig})
	return svc, log
}

// wakeDispatches returns the dispatches hyprctl received and clears the log.
func wakeDispatches(t *testing.T, log string) []string {
	t.Helper()
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	var dispatches []string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if strings.HasPrefix(line, "dispatch ") {
			dispatches = append(dispatches, line)
		}
	}
	if err := os.WriteFile(log, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	return dispatches
}

func TestWakeRetriesWithLuaWhenNoDialectWasEverDetected(t *testing.T) {
	svc, log := newBusyWakeService(t, true, "")
	svc.ensureConfigInclude(context.Background())

	svc.wakeDisplays(context.Background())
	got := wakeDispatches(t, log)
	if len(got) != 2 || got[0] != "dispatch dpms on" || !strings.Contains(got[1], "hl.dsp.dpms") {
		t.Fatalf("a rejected legacy wake should be resent in Lua syntax, got %q", got)
	}

	svc.wakeDisplays(context.Background())
	got = wakeDispatches(t, log)
	if len(got) != 1 || !strings.Contains(got[0], "hl.dsp.dpms") {
		t.Fatalf("the next wake should remember the Lua dialect, got %q", got)
	}
}

func TestLegacyWakeIsNotRetriedWithLua(t *testing.T) {
	svc, log := newBusyWakeService(t, false, "")
	svc.wakeDisplays(context.Background())
	if got := wakeDispatches(t, log); len(got) != 1 || got[0] != "dispatch dpms on" {
		t.Fatalf("a legacy Hyprland should get exactly one classic wake, got %q", got)
	}
}

func TestExplicitLuaRootSetsTheWakeDialectWithoutAVersion(t *testing.T) {
	root := filepath.Join(t.TempDir(), "hyprland.lua")
	if err := os.WriteFile(root, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	svc, log := newBusyWakeService(t, true, root)
	svc.ensureConfigInclude(context.Background())
	svc.wakeDisplays(context.Background())
	if got := wakeDispatches(t, log); len(got) != 1 || !strings.Contains(got[0], "hl.dsp.dpms") {
		t.Fatalf("an explicit .lua root should pick the Lua wake directly, got %q", got)
	}
}

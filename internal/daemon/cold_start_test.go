package daemon

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crmne/hyprmoncfg/internal/config"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

func TestColdStartNeutralizesPersistedClamshellRules(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("HYPRMONCFG_MONITORS_CONF", "")
	path, err := config.HyprlandGeneratedPath(config.HyprConfigLua)
	if err != nil {
		t.Fatalf("resolve generated path: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create config dir: %v", err)
	}
	stale := config.GeneratedLuaHeader + "\nhl.monitor({ output = \"eDP-1\", disabled = true })\n"
	if err := os.WriteFile(path, []byte(stale), 0o644); err != nil {
		t.Fatalf("write stale config: %v", err)
	}

	var logs []string
	svc := New(nil, profile.NewStore(t.TempDir()), Config{
		Logf: func(format string, args ...any) { logs = append(logs, format) },
	})
	svc.hasRunningHyprland = func(context.Context) (bool, error) { return false, nil }
	svc.neutralizeColdStartConfig(context.Background())

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read neutral config: %v", err)
	}
	if string(got) != config.GeneratedLuaHeader+"\n" {
		t.Fatalf("cold-start config = %q", got)
	}
	if len(logs) != 1 || !strings.Contains(logs[0], "neutralized stale monitor rules") {
		t.Fatalf("cold-start logs = %v", logs)
	}
}

func TestColdStartLeavesLayoutAloneWhenHyprlandIsRunning(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hyprmoncfg-monitors.lua")
	stale := config.GeneratedLuaHeader + "\nhl.monitor({ output = \"eDP-1\", disabled = true })\n"
	if err := os.WriteFile(path, []byte(stale), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	svc := New(nil, profile.NewStore(t.TempDir()), Config{MonitorsConf: path})
	svc.hasRunningHyprland = func(context.Context) (bool, error) { return true, nil }
	svc.neutralizeColdStartConfig(context.Background())

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if string(got) != stale {
		t.Fatalf("live-session config changed: %q", got)
	}
}

func TestColdStartLeavesLayoutAloneWhenManagementIsOff(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hyprmoncfg-monitors.lua")
	stale := config.GeneratedLuaHeader + "\nhl.monitor({ output = \"eDP-1\", disabled = true })\n"
	if err := os.WriteFile(path, []byte(stale), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	configDir := t.TempDir()
	if err := config.SetManaged(configDir, false); err != nil {
		t.Fatalf("disable management: %v", err)
	}

	called := false
	svc := New(nil, profile.NewStore(t.TempDir()), Config{ConfigDir: configDir, MonitorsConf: path})
	svc.hasRunningHyprland = func(context.Context) (bool, error) {
		called = true
		return false, nil
	}
	svc.neutralizeColdStartConfig(context.Background())
	if called {
		t.Fatal("queried Hyprland while monitor management was off")
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if string(got) != stale {
		t.Fatalf("unmanaged config changed: %q", got)
	}
}

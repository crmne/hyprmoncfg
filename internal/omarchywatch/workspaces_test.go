package omarchywatch

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/crmne/hyprmoncfg/internal/profile"
)

func TestDetectPerMonitorWorkspacesNeedsOmarchysModule(t *testing.T) {
	home := t.TempDir()
	omarchy := filepath.Join(t.TempDir(), "omarchy")
	env := map[string]string{"OMARCHY_PATH": omarchy, "XDG_STATE_HOME": filepath.Join(home, "state")}
	getenv := func(name string) string { return env[name] }

	if got := detectPerMonitorWorkspaces(home, getenv); got != nil {
		t.Fatal("found Omarchy's per-monitor workspaces without its module")
	}
	module := filepath.Join(omarchy, perMonitorWorkspacesModule)
	if err := os.MkdirAll(filepath.Dir(module), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(module, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	got := detectPerMonitorWorkspaces(home, getenv)
	if got == nil {
		t.Fatal("Omarchy's module not found")
	}
	want := []string{filepath.Join(home, "state", "omarchy/monitors/workspaces"), filepath.Join(home, "state", "omarchy/monitor-workspaces")}
	if len(got.statePaths) != 2 || got.statePaths[0] != want[0] || got.statePaths[1] != want[1] {
		t.Fatalf("state paths %v, want %v", got.statePaths, want)
	}
}

func TestPerMonitorWorkspaceBlocksFollowOmarchy(t *testing.T) {
	state := filepath.Join(t.TempDir(), "workspaces")
	// The laptop panel and an LG monitor Omarchy has seen before, and a
	// second identical LG that once took block 3.
	content := "0\teDP-1\tLaptop panel\n1\t\tLG 16MR70 A\n3\tDP-2\tTwin #2\n2\t\tTwin\n"
	if err := os.WriteFile(state, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	w := &PerMonitorWorkspaces{statePaths: []string{filepath.Join(t.TempDir(), "missing"), state}}

	blocks := w.Blocks([]profile.OutputConfig{
		{Key: "lg", Name: "DP-1", Description: "LG 16MR70 A"},
		{Key: "laptop", Name: "eDP-1", Description: "Laptop panel"},
	})
	if blocks["laptop"] != 0 || blocks["lg"] != 1 {
		t.Fatalf("saved blocks not kept: %v", blocks)
	}

	// The twin on DP-2 keeps its own identity; the other gets the plain one.
	blocks = w.Blocks([]profile.OutputConfig{
		{Key: "a", Name: "DP-1", Description: "Twin"},
		{Key: "b", Name: "DP-2", Description: "Twin"},
	})
	if blocks["a"] != 2 || blocks["b"] != 3 {
		t.Fatalf("identical monitors mixed up: %v", blocks)
	}

	// New monitors take the lowest free blocks, the laptop panel first.
	empty := &PerMonitorWorkspaces{statePaths: []string{filepath.Join(t.TempDir(), "missing")}}
	blocks = empty.Blocks([]profile.OutputConfig{
		{Key: "tv", Name: "HDMI-A-1", Description: "TV"},
		{Key: "laptop", Name: "eDP-1", Description: "Laptop panel"},
	})
	if blocks["laptop"] != 0 || blocks["tv"] != 1 {
		t.Fatalf("new monitors numbered wrong: %v", blocks)
	}
	blocks = w.Blocks([]profile.OutputConfig{{Key: "tv", Name: "HDMI-A-1", Description: "TV"}})
	if blocks["tv"] != 4 {
		t.Fatalf("a new monitor must skip taken blocks: %v", blocks)
	}
	if data, _ := os.ReadFile(state); string(data) != content {
		t.Fatal("Omarchy's state file was written")
	}
}

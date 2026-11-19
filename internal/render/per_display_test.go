package render

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crmne/hyprmoncfg/internal/config"
	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

func perDisplayFixture() (profile.Profile, []hypr.Monitor) {
	monitors := []hypr.Monitor{
		{ID: 0, Name: "eDP-1", Description: "Laptop panel", Width: 2880, Height: 1800, RefreshRate: 120, Scale: 2},
		{ID: 1, Name: "DP-1", Description: "Desk display", X: 1440, Width: 2560, Height: 1440, RefreshRate: 60, Scale: 1},
	}
	p := profile.FromMonitors("Desk", monitors)
	keys := map[string]string{}
	for _, output := range p.Outputs {
		keys[output.Name] = output.Key
	}
	p.Workspaces = profile.WorkspaceSettings{
		Enabled:      true,
		Strategy:     profile.WorkspaceStrategyPerDisplay,
		MonitorOrder: []string{keys["eDP-1"], keys["DP-1"]},
	}
	return p, monitors
}

func TestPerDisplayLuaHandsOffToOmarchyAndRunsItselfElsewhere(t *testing.T) {
	p, monitors := perDisplayFixture()
	got, err := RenderConfig(p, monitors, Options{Format: config.HyprConfigLua, UseMonitorV2: true})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "o.workspaces.reconcile = function() end") {
		t.Fatalf("per-display must not take Omarchy's workspaces over\n%s", got)
	}
	for _, want := range []string{
		`if type(o) == "table" and type(o.workspaces) == "table" and type(o.workspaces.reconcile) == "function" then`,
		`local blocks = { ["desc:Laptop panel"] = 0, ["desc:Desk display"] = 1 }`,
		`  hl.workspace_rule({ workspace = "11", monitor = "desc:Desk display", default = true, persistent = true })`,
		`  hl.workspace_rule({ workspace = "20", monitor = "desc:Desk display" })`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q\n%s", want, got)
		}
	}

	lua, err := exec.LookPath("lua")
	if err != nil {
		t.Skip("Lua interpreter is required for the generated-config runtime check")
	}
	section := got[strings.Index(got, "-- hyprmoncfg gives each display"):]
	section = section[:strings.Index(section, "\nend\n")+len("\nend\n")]
	script := `
local binds, unbinds, calls, rules = {}, {}, {}, {}
local active = { name = "DP-1", description = "Desk display" }
local function dispatch(kind)
  return function(spec) return function() calls[#calls + 1] = { kind = kind, spec = spec } end end
end
hl = {
  bind = function(key, fn, opts) binds[key] = fn; assert(opts.description) end,
  unbind = function(key) unbinds[key] = true end,
  workspace_rule = function(spec) rules[#rules + 1] = spec end,
  get_active_monitor = function() return active end,
  dispatch = function(fn) fn() end,
  dsp = { focus = dispatch("focus"), window = { move = dispatch("window") } },
}
-- Omarchy: nothing changes.
o = { workspaces = { reconcile = function() end } }
` + section + `
assert(next(binds) == nil and next(unbinds) == nil and #rules == 0, "Omarchy's per-monitor workspaces were touched")
-- Plain Hyprland: rules and keys come from this file.
o = nil
` + section + `
assert(#rules == 20, "expected ten rules per display, got " .. #rules)
assert(unbinds["SUPER + 1"] and unbinds["SUPER + code:10"] and unbinds["SUPER + SHIFT + 0"], "old bindings left to fire too")
binds["SUPER + code:12"]()
assert(calls[#calls].kind == "focus" and calls[#calls].spec.workspace == "13", "Super + 3 on the second display")
binds["SUPER + SHIFT + code:19"]()
assert(calls[#calls].kind == "window" and calls[#calls].spec.workspace == "20" and calls[#calls].spec.follow == true)
active = { name = "eDP-1", description = "Laptop panel" }
binds["SUPER + code:10"]()
assert(calls[#calls].spec.workspace == "1", "Super + 1 on the laptop panel")
active = { name = "HDMI-A-1", description = "Unplanned" }
binds["SUPER + code:14"]()
assert(calls[#calls].spec.workspace == "5", "a display without a block keeps plain numbers")
`
	path := filepath.Join(t.TempDir(), "per_display.lua")
	if err := os.WriteFile(path, []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(lua, path).CombinedOutput(); err != nil {
		t.Fatalf("generated per-display section failed: %v\n%s", err, output)
	}
}

// The generated file's own branch ignores Omarchy's blocks: it only runs
// where Omarchy's module is missing, so it follows the planner's order.
func TestPerDisplayLuaUsesThePlannerOrderEvenWithADelegate(t *testing.T) {
	profile.SetPerDisplayDelegate("omarchy", func(outputs []profile.OutputConfig) map[string]int {
		blocks := map[string]int{}
		for i, output := range outputs {
			blocks[output.Key] = len(outputs) - 1 - i
		}
		return blocks
	})
	t.Cleanup(func() { profile.SetPerDisplayDelegate("", nil) })

	p, monitors := perDisplayFixture()
	got, err := RenderConfig(p, monitors, Options{Format: config.HyprConfigLua, UseMonitorV2: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `local blocks = { ["desc:Laptop panel"] = 0, ["desc:Desk display"] = 1 }`) {
		t.Fatalf("own branch followed the delegate's blocks\n%s", got)
	}
}

func TestPerDisplayHyprlangBindsKeysToTheWorkspaceCommand(t *testing.T) {
	p, monitors := perDisplayFixture()
	got, err := RenderConfig(p, monitors, Options{Format: config.HyprConfigLegacy, UseMonitorV2: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"workspace = 11, monitor:desc:Desk display, default:true, persistent:true",
		"unbind = SUPER, 1\nunbind = SUPER, code:10\nbind = SUPER, code:10, exec, hyprmoncfg workspace focus 1",
		"unbind = SUPER SHIFT, 0\nunbind = SUPER SHIFT, code:19\nbind = SUPER SHIFT, code:19, exec, hyprmoncfg workspace move 10",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q\n%s", want, got)
		}
	}

	p.Workspaces.Strategy = profile.WorkspaceStrategySequential
	got, err = RenderConfig(p, monitors, Options{Format: config.HyprConfigLegacy, UseMonitorV2: true})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "hyprmoncfg workspace") {
		t.Fatalf("only per-display binds the number keys\n%s", got)
	}
}

// With fewer workspaces per display, Omarchy keeps running them but loses
// the keys past the count, and the file's own branch binds only that many.
func TestPerDisplayLuaHonoursTheWorkspaceCount(t *testing.T) {
	p, monitors := perDisplayFixture()
	p.Workspaces.SlotsPerDisplay = 4
	got, err := RenderConfig(p, monitors, Options{Format: config.HyprConfigLua, UseMonitorV2: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "local slots = 4\n") {
		t.Fatalf("missing slot count\n%s", got)
	}
	if strings.Contains(got, `workspace = "15"`) || !strings.Contains(got, `workspace = "14"`) {
		t.Fatalf("expected rules for slots 1-4 only\n%s", got)
	}

	lua, err := exec.LookPath("lua")
	if err != nil {
		t.Skip("Lua interpreter is required for the generated-config runtime check")
	}
	section := got[strings.Index(got, "-- hyprmoncfg gives each display"):]
	section = section[:strings.Index(section, "\nend\n")+len("\nend\n")]
	script := `
local binds, unbinds, rules = {}, {}, {}
hl = {
  bind = function(key, fn, opts) binds[key] = fn end,
  unbind = function(key) unbinds[key] = true end,
  workspace_rule = function(spec) rules[#rules + 1] = spec end,
  get_active_monitor = function() return { name = "DP-1", description = "Desk display" } end,
  dispatch = function(fn) fn() end,
  dsp = { focus = function() return function() end end, window = { move = function() return function() end end } },
}
o = { workspaces = { reconcile = function() end } }
` + section + `
assert(next(binds) == nil and #rules == 0, "Omarchy stays in charge")
assert(not unbinds["SUPER + code:13"], "Super + 4 must stay Omarchy's")
assert(unbinds["SUPER + code:14"] and unbinds["SUPER + SHIFT + code:19"] and unbinds["SUPER + SHIFT + ALT + code:16"], "keys past the count must go")
assert(not unbinds["SUPER + 5"], "Omarchy binds by key code; key symbols stay")
binds, unbinds, rules = {}, {}, {}
o = nil
` + section + `
assert(#rules == 8, "expected four rules per display, got " .. #rules)
assert(binds["SUPER + code:13"] and binds["SUPER + SHIFT + ALT + code:13"], "Super + 4 bound")
assert(not binds["SUPER + code:14"] and unbinds["SUPER + code:14"] and unbinds["SUPER + 5"], "Super + 5 unbound and left free")
`
	path := filepath.Join(t.TempDir(), "per_display_count.lua")
	if err := os.WriteFile(path, []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(lua, path).CombinedOutput(); err != nil {
		t.Fatalf("generated per-display section failed: %v\n%s", err, output)
	}
}

func TestPerDisplayHyprlangHonoursTheWorkspaceCount(t *testing.T) {
	p, monitors := perDisplayFixture()
	p.Workspaces.SlotsPerDisplay = 3
	got, err := RenderConfig(p, monitors, Options{Format: config.HyprConfigLegacy, UseMonitorV2: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"bind = SUPER SHIFT ALT, code:12, exec, hyprmoncfg workspace move-silent 3",
		"unbind = SUPER, 4\nunbind = SUPER, code:13\nunbind = SUPER SHIFT, 4",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q\n%s", want, got)
		}
	}
	if strings.Contains(got, "workspace focus 4") || strings.Contains(got, "workspace = 14,") {
		t.Fatalf("slots past the count must not be bound or placed\n%s", got)
	}
}

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

func TestOmarchyWorkspaceOwnershipFollowsPlanner(t *testing.T) {
	monitors := []hypr.Monitor{{Name: "DP-1", Description: "Test display", Width: 1920, Height: 1080, RefreshRate: 60, Scale: 1}}
	p := profile.FromMonitors("Desk", monitors)
	for _, enabled := range []bool{true, false} {
		p.Workspaces.Enabled = enabled
		for _, format := range []config.HyprConfigFormat{config.HyprConfigLua, config.HyprConfigLegacy} {
			got, err := RenderConfig(p, monitors, Options{Format: format, UseMonitorV2: true})
			if err != nil {
				t.Fatal(err)
			}
			want := enabled && format == config.HyprConfigLua
			if strings.Contains(got, "o.workspaces.reconcile = function() end") != want {
				t.Fatalf("ownership block: enabled=%v format=%v\n%s", enabled, format, got)
			}
		}
	}
}

func TestOmarchyMonitorPanelHandoffIsLuaOnly(t *testing.T) {
	monitors := []hypr.Monitor{{Name: "DP-1", Description: "Test display", Width: 1920, Height: 1080, RefreshRate: 60, Scale: 1}}
	p := profile.FromMonitors("Desk", monitors)
	for _, enabled := range []bool{true, false} {
		p.Workspaces.Enabled = enabled
		for _, format := range []config.HyprConfigFormat{config.HyprConfigLua, config.HyprConfigLegacy} {
			got, err := RenderConfig(p, monitors, Options{Format: format, UseMonitorV2: true})
			if err != nil {
				t.Fatal(err)
			}
			want := format == config.HyprConfigLua
			if strings.Contains(got, "omarchy_monitor_panel_settings = false") != want {
				t.Fatalf("panel handoff: planner=%v format=%v\n%s", enabled, format, got)
			}
		}
	}
}

func TestOmarchyGDKScaleFollowsLowestActiveScale(t *testing.T) {
	cases := []struct {
		name string
		outs []matchedOutput
		want string
	}{
		{"rounds", []matchedOutput{{config: profile.OutputConfig{Enabled: true, Scale: 1.6}}}, `hl.env("GDK_SCALE", "2")`},
		{"lowest wins", []matchedOutput{{config: profile.OutputConfig{Enabled: true, Scale: 2}}, {config: profile.OutputConfig{Enabled: true, Scale: 1.25}}}, `hl.env("GDK_SCALE", "1")`},
		{"skips off and mirrored", []matchedOutput{
			{config: profile.OutputConfig{Enabled: false, Scale: 1}},
			{config: profile.OutputConfig{Enabled: true, Scale: 1, MirrorOf: "a"}},
			{config: profile.OutputConfig{Enabled: true, Scale: 2}},
		}, `hl.env("GDK_SCALE", "2")`},
	}
	for _, tc := range cases {
		if got := omarchyGDKScaleSection(tc.outs, false); !strings.Contains(got, tc.want) {
			t.Fatalf("%s: got %q", tc.name, got)
		}
	}
	if got := omarchyGDKScaleSection(nil, false); got != "" {
		t.Fatalf("no outputs: %q", got)
	}
	if got := omarchyGDKScaleSection(cases[0].outs, true); got != "" {
		t.Fatalf("XWayland scales X11 clients itself, GDK_SCALE must stay untouched: %q", got)
	}
}

func TestOmarchyGDKScaleYieldsToXWaylandScaling(t *testing.T) {
	monitors := []hypr.Monitor{{Name: "eDP-1", Description: "Laptop panel", Width: 2880, Height: 1920, RefreshRate: 120, Scale: 2}}
	p := profile.FromMonitors("Laptop", monitors)
	for _, xwaylandScalesX11 := range []bool{false, true} {
		got, err := RenderConfig(p, monitors, Options{Format: config.HyprConfigLua, UseMonitorV2: true, XWaylandScalesX11: xwaylandScalesX11})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(got, `hl.env("GDK_SCALE"`) == xwaylandScalesX11 {
			t.Fatalf("xwaylandScalesX11=%v\n%s", xwaylandScalesX11, got)
		}
	}
}

func TestOmarchyWorkspaceHandoffRuntime(t *testing.T) {
	lua, err := exec.LookPath("lua")
	if err != nil {
		t.Skip("Lua interpreter is required for the generated-config runtime check")
	}
	path := filepath.Join(t.TempDir(), "handoff.lua")
	script := `
local bindings, events, calls = {}, {}, {}
local original = { enabled = true, reconcile = function() error("Omarchy reassigned global workspaces") end }
o = { workspaces = original, bind_expressions = {}, rebind = function(key, _, fn) bindings[key] = fn end }
for _, event in ipairs({"start", "reload", "plug", "unplug"}) do
  events[event] = function() original.reconcile() end
end
local function dispatch(kind)
  return function(spec) return function() calls[#calls + 1] = { kind = kind, spec = spec } end end
end
hl = { dispatch = function(fn) fn() end, dsp = { focus = dispatch("focus"), window = { move = dispatch("window") }, workspace = { move = dispatch("workspace") } } }
` + omarchyGlobalWorkspaces + `
assert(o.workspaces == original and original.enabled == false)
for _, fn in pairs(events) do fn() end
local function run(key, kind, workspace, follow)
  assert(bindings[key], key)
  assert(o.bind_expressions[bindings[key]], "keybindings menu lost its action")
  bindings[key]()
  local call = calls[#calls]
  assert(call.kind == kind and call.spec.workspace == workspace, key .. " resolved incorrectly")
  assert(call.spec.monitor == nil, "a number shortcut must follow the global workspace rule")
  if follow ~= nil then assert(call.spec.follow == follow) end
end
run("SUPER + code:11", "focus", "2")
run("SUPER + SHIFT + code:13", "window", "4", true)
run("SUPER + SHIFT + ALT + code:15", "window", "6", false)
run("SUPER + TAB", "focus", "e+1")
run("SUPER + SHIFT + TAB", "focus", "e-1")
run("SUPER + CTRL + TAB", "focus", "previous")
bindings["SUPER + SHIFT + ALT + UP"]()
assert(calls[#calls].kind == "workspace" and calls[#calls].spec.monitor == "u")
assert(#calls == 7, "unexpected monitor reassignment or workspace renumbering")
-- The same generated file must remain portable to plain Hyprland and older Omarchy.
o, hl = nil, nil
` + omarchyGlobalWorkspaces + `
o = { rebind = function() error("older Omarchy bindings changed") end }
` + omarchyGlobalWorkspaces
	if err := os.WriteFile(path, []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(lua, path).CombinedOutput(); err != nil {
		t.Fatalf("generated handoff failed: %v\n%s", err, output)
	}
}

package render

import (
	"fmt"
	"strings"
	"testing"

	"github.com/crmne/hyprmoncfg/internal/config"
	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

func TestDisconnectedWakePreparationRequiresExplicitOptIn(t *testing.T) {
	p := profile.Profile{Name: "Desk", DisableUnknownOutputs: true, Outputs: []profile.OutputConfig{
		{Description: "Test Display", Enabled: true, Mode: "1920x1080@60", Scale: 1},
	}}
	for _, format := range []config.HyprConfigFormat{config.HyprConfigLegacy, config.HyprConfigLua} {
		opts := Options{Format: format, UseMonitorV2: true}
		if _, err := RenderConfig(p, nil, opts); err == nil {
			t.Fatal("ordinary apply must reject an absent display")
		}
		opts.PrepareDisconnected = true
		got, err := RenderConfig(p, nil, opts)
		if err != nil || !strings.Contains(got, "desc:Test Display") {
			t.Fatalf("prepare wake rule: %v\n%s", err, got)
		}
		if _, err := RenderConfig(p, []hypr.Monitor{{Name: "DP-9", Description: "Other"}}, opts); err == nil {
			t.Fatal("preparation must not bypass matching with connected hardware")
		}
	}
}

func TestHyprlangEscapePreservesLiteralSyntax(t *testing.T) {
	got := hyprlangEscape(`desc:Dell #HASH {{raw}} {{{overlap}} path\{panel}`)
	want := `desc:Dell ##HASH \{\{raw}} \{\{\{overlap}} path\\{panel}`
	if got != want {
		t.Fatalf("hyprlangEscape() = %q, want %q", got, want)
	}
}

func TestHyprlangEscapeNormalizesLineBreaks(t *testing.T) {
	got := hyprlangEscape("left\nright\r\nthird\rfourth")
	want := "left right third fourth"
	if got != want {
		t.Fatalf("hyprlangEscape() = %q, want %q", got, want)
	}
}

func TestLegacyHyprlangSelectorFallsBackForUnsafeDescSyntax(t *testing.T) {
	monitor := hypr.Monitor{Name: "DP-1"}

	tests := []string{
		"desc:Dell $PANEL",
		"desc:Dell, Inc.",
		"desc:Dell\nPanel",
	}
	for _, selector := range tests {
		if got := legacyHyprlangSelector(selector, monitor); got != "DP-1" {
			t.Fatalf("legacyHyprlangSelector(%q) = %q, want DP-1", selector, got)
		}
	}

	safe := "desc:Dell #HASH {{raw}}"
	if got := legacyHyprlangSelector(safe, monitor); got != safe {
		t.Fatalf("legacyHyprlangSelector(%q) = %q, want unchanged selector", safe, got)
	}
}

func TestLuaQuoteEscapesLuaStringSyntax(t *testing.T) {
	got := luaQuote("desc:Dell #$,\n\"\\\x01")
	want := `"desc:Dell #$,\n\"\\\001"`
	if got != want {
		t.Fatalf("luaQuote() = %q, want %q", got, want)
	}
}

func TestLiveOutputRuleRendersOneEnabledOutput(t *testing.T) {
	panel := hypr.Monitor{Name: "eDP-1", Description: "BOE Panel", Make: "BOE", Model: "Panel", Serial: "I1"}
	hdmi := hypr.Monitor{Name: "HDMI-A-1", Description: "HKC 27E1QA", Make: "HKC", Model: "27E1QA", Serial: "E1"}
	monitors := []hypr.Monitor{panel, hdmi}
	p := profile.New("casa", []profile.OutputConfig{
		{Key: panel.HardwareKey(), Name: panel.Name, Enabled: true, Width: 1920, Height: 1080, Refresh: 144, X: 2560, Scale: 1.6},
		{Key: hdmi.HardwareKey(), Name: hdmi.Name, Enabled: false},
	})
	lua, ok := LiveOutputRule(p, monitors, "eDP-1", Options{Format: config.HyprConfigLua, UseMonitorV2: true})
	if !ok || !strings.HasPrefix(lua, "hl.monitor({") || !strings.Contains(lua, `output = "desc:BOE Panel"`) ||
		!strings.Contains(lua, `mode = "1920x1080@144.00"`) || !strings.Contains(lua, `position = "2560x0"`) || !strings.Contains(lua, "scale = 1.6") {
		t.Fatalf("unexpected Lua rule (ok=%v):\n%s", ok, lua)
	}
	if keyword, ok := LiveOutputRule(p, monitors, "eDP-1", Options{Format: config.HyprConfigLegacy}); !ok || keyword != "eDP-1,1920x1080@144.00,2560x0,1.6,transform,0,vrr,0" {
		t.Fatalf("unexpected keyword rule (ok=%v): %s", ok, keyword)
	}
	if _, ok := LiveOutputRule(p, monitors, "HDMI-A-1", Options{Format: config.HyprConfigLua}); ok {
		t.Fatal("a disabled output has no live rule to set")
	}
}

func TestRenderTurnsTouchInputWithARotatingPanel(t *testing.T) {
	panel := hypr.Monitor{Name: "eDP-1", Description: "BOE 0x0BCA", Make: "BOE", Model: "0x0BCA", Width: 1920, Height: 1200, RefreshRate: 60, Scale: 1}
	p := profile.FromMonitors("Tablet", []hypr.Monitor{panel})
	p.Outputs[0].RotationAuto = true
	p.Outputs[0].Transform = 1
	lua, err := RenderConfig(p, []hypr.Monitor{panel}, Options{Format: config.HyprConfigLua, UseMonitorV2: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(lua, "hl.config({ input = { touchdevice = { transform = 1 }, tablet = { transform = 1 } } })") {
		t.Fatalf("Lua config does not turn touch input:\n%s", lua)
	}
	legacy, err := RenderConfig(p, []hypr.Monitor{panel}, Options{Format: config.HyprConfigLegacy})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(legacy, "touchdevice {\n    transform = 1\n  }\n  tablet {\n    transform = 1\n  }") {
		t.Fatalf("legacy config does not turn touch input:\n%s", legacy)
	}
	p.Outputs[0].RotationAuto = false
	if fixed, _ := RenderConfig(p, []hypr.Monitor{panel}, Options{Format: config.HyprConfigLua, UseMonitorV2: true}); !strings.Contains(fixed, "touchdevice = { transform = 1 }") {
		t.Fatalf("a fixed panel must keep touch aligned:\n%s", fixed)
	}
}

func TestFixedPanelTouchTransformsAcrossFormats(t *testing.T) {
	for _, name := range []string{"DSI-1", "eDP-1", "DP-1"} {
		for transform := 0; transform < 8; transform++ {
			for _, format := range []config.HyprConfigFormat{config.HyprConfigLua, config.HyprConfigLegacy} {
				panel := hypr.Monitor{Name: name, Width: 1920, Height: 1200, RefreshRate: 60, Scale: 1}
				p := profile.FromMonitors("Tablet", []hypr.Monitor{panel})
				p.Outputs[0].Transform = transform
				rendered, err := RenderConfig(p, []hypr.Monitor{panel}, Options{Format: format})
				if err != nil {
					t.Fatal(err)
				}
				if panel.IsInternal() {
					want := fmt.Sprintf("touchdevice {\n    transform = %d", transform)
					if format == config.HyprConfigLua {
						want = fmt.Sprintf("touchdevice = { transform = %d }", transform)
					}
					if !strings.Contains(rendered, want) {
						t.Fatalf("touch transform mismatch: %s", rendered)
					}
				}
				if strings.Contains(rendered, "touchdevice") != panel.IsInternal() {
					t.Fatalf("%s transform %d: %s", name, transform, rendered)
				}
				p.Outputs[0].Enabled = false
				rendered, err = RenderConfig(p, []hypr.Monitor{panel}, Options{Format: format})
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(rendered, "touchdevice") {
					t.Fatal("disabled panel took ownership of touch")
				}
			}
		}
	}
}

package render

import (
	"strings"
	"testing"

	"github.com/crmne/hyprmoncfg/internal/config"
	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

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

func TestAutoRotationRendersTheLiveTransform(t *testing.T) {
	// A generated rule always states a transform: leaving it out means
	// transform 0, not "leave the rotation alone". So an auto output must
	// render the display's live rotation, or every reload pins it to normal.
	panel := hypr.Monitor{Name: "DSI-1", Description: "NYM Panel", Make: "NYM", Model: "Panel", Serial: "I1",
		Width: 1200, Height: 1920, RefreshRate: 50, Scale: 1, Transform: 3}
	monitors := []hypr.Monitor{panel}
	p := profile.New("Tablet", []profile.OutputConfig{
		{Key: panel.HardwareKey(), Name: panel.Name, Enabled: true, Width: 1200, Height: 1920,
			Refresh: 50, X: 0, Scale: 1, Transform: 1, RotationAuto: true},
	})

	// The stored Transform is deliberately stale (1) to prove auto ignores it
	// in favour of the live value (3).
	if keyword, ok := LiveOutputRule(p, monitors, "DSI-1", Options{Format: config.HyprConfigLegacy}); !ok ||
		!strings.Contains(keyword, ",transform,3,") {
		t.Fatalf("auto keyword rule should carry the live transform, got (ok=%v): %s", ok, keyword)
	}

	lua, ok := LiveOutputRule(p, monitors, "DSI-1", Options{Format: config.HyprConfigLua, UseMonitorV2: true})
	if !ok || !strings.Contains(lua, "transform = 3,") {
		t.Fatalf("auto Lua rule should carry the live transform (ok=%v):\n%s", ok, lua)
	}

	rendered, err := RenderConfig(p, monitors, Options{Format: config.HyprConfigLua, UseMonitorV2: true})
	if err != nil {
		t.Fatalf("RenderConfig: %v", err)
	}
	if !strings.Contains(rendered, "transform = 3,") || strings.Contains(rendered, "transform = 1,") {
		t.Fatalf("rendered config should carry the live transform, not the stored one:\n%s", rendered)
	}
}

func TestExplicitRotationStillRendersItsStoredValue(t *testing.T) {
	panel := hypr.Monitor{Name: "DSI-1", Description: "NYM Panel", Make: "NYM", Model: "Panel", Serial: "I1",
		Width: 1200, Height: 1920, RefreshRate: 50, Scale: 1, Transform: 3}
	monitors := []hypr.Monitor{panel}
	p := profile.New("Tablet", []profile.OutputConfig{
		{Key: panel.HardwareKey(), Name: panel.Name, Enabled: true, Width: 1200, Height: 1920,
			Refresh: 50, X: 0, Scale: 1, Transform: 1},
	})
	if keyword, ok := LiveOutputRule(p, monitors, "DSI-1", Options{Format: config.HyprConfigLegacy}); !ok ||
		!strings.Contains(keyword, ",transform,1,") {
		t.Fatalf("an explicit rotation should render its stored value, got (ok=%v): %s", ok, keyword)
	}
}

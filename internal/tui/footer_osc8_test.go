package tui

import (
	"strings"
	"testing"

	"github.com/crmne/hyprmoncfg/internal/buildinfo"
)

func TestDecorateFooterBarContainsOSC8Links(t *testing.T) {
	prevVersion := buildinfo.Version
	buildinfo.Version = "1.0.1"
	defer func() { buildinfo.Version = prevVersion }()

	m := Model{
		styles: newStyles(),
		mode:   modeMain,
		tab:    tabLayout,
		width:  160,
		height: 30,
	}

	footer := m.renderFooterBar()
	decorated := m.decorateFooterBar(footer)

	for _, want := range []struct{ label, url string }{
		{"v1.0.1", "releases"},
		{"Ask", "discussions"},
		{"Donate", "sponsors"},
	} {
		if !strings.Contains(decorated, want.url) {
			t.Errorf("missing %s link URL (%s)", want.label, want.url)
		}
	}

	if !strings.Contains(decorated, "\x07") {
		t.Error("missing BEL terminator in OSC8 links")
	}
}

func TestRenderMainPreservesOSC8Links(t *testing.T) {
	prevVersion := buildinfo.Version
	buildinfo.Version = "1.0.1"
	defer func() { buildinfo.Version = prevVersion }()

	m := Model{
		styles:      newStyles(),
		mode:        modeMain,
		tab:         tabLayout,
		layoutFocus: layoutFocusInspector,
		width:       160,
		height:      30,
		editOutputs: []editableOutput{{
			Key:       "test|monitor",
			Name:      "DP-1",
			Enabled:   true,
			Modes:     []string{"1920x1080@60Hz"},
			ModeIndex: 0,
			Width:     1920,
			Height:    1080,
			Refresh:   60,
			Scale:     1,
		}},
	}

	view := m.renderMain()

	for _, want := range []struct{ label, url string }{
		{"version", "releases"},
		{"Ask", "discussions"},
		{"Donate", "sponsors"},
	} {
		if !strings.Contains(view, want.url) {
			t.Errorf("OSC8 %s link URL (%s) lost after renderMain", want.label, want.url)
		}
	}

	for _, want := range []string{"v1.0.1", "Ask", "Donate"} {
		if !strings.Contains(view, want) {
			t.Errorf("visible text %q lost after renderMain", want)
		}
	}
}

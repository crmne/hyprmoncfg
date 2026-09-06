package apply

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crmne/hyprmoncfg/internal/config"
	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

var monitors = []hypr.Monitor{
	{Name: "DP-1", Description: "Microstep MPG321UR-QD", Make: "Microstep", Model: "MPG321UR-QD", Serial: "A1", Width: 3840, Height: 2160, RefreshRate: 143.99, Scale: 1, VRR: hypr.VRRMode(1)},
	{Name: "eDP-1", Description: "Samsung Display Corp. ATNA60CL10-0", Make: "Samsung Display Corp.", Model: "ATNA60CL10-0", Serial: "B2", Width: 2880, Height: 1800, RefreshRate: 120, X: 3840, Scale: 1},
}

func newTestProfile() profile.Profile {
	return profile.New("desk", []profile.OutputConfig{
		{Key: monitors[0].HardwareKey(), Name: monitors[0].Name, Enabled: true, Width: 3840, Height: 2160, Refresh: 143.99, X: 0, Y: 0, Scale: 1, VRR: 1},
		{Key: monitors[1].HardwareKey(), Name: monitors[1].Name, Enabled: true, Width: 2880, Height: 1800, Refresh: 120, X: 3840, Y: 0, Scale: 1},
	})
}

func TestCommandsForProfile(t *testing.T) {
	mon := hypr.Monitor{Name: "DP-1", Make: "Dell", Model: "U2720Q", Serial: "A1"}
	p := profile.New("desk", []profile.OutputConfig{{
		Key:       mon.HardwareKey(),
		Name:      "DP-1",
		Enabled:   true,
		Width:     2560,
		Height:    1440,
		Refresh:   144,
		X:         0,
		Y:         0,
		Scale:     1,
		Transform: 0,
	}})

	cmds, err := CommandsForProfile(p, []hypr.Monitor{mon})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
	if !strings.Contains(cmds[0], "DP-1,2560x1440@144") {
		t.Fatalf("unexpected command: %s", cmds[0])
	}
}

func TestCommandsForProfileDisable(t *testing.T) {
	mon := hypr.Monitor{Name: "HDMI-A-1", Make: "LG", Model: "27GP850", Serial: "B2"}
	p := profile.New("dock", []profile.OutputConfig{{
		Key:     mon.HardwareKey(),
		Name:    mon.Name,
		Enabled: false,
	}})

	cmds, err := CommandsForProfile(p, []hypr.Monitor{mon})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cmds) != 1 || cmds[0] != "HDMI-A-1,disable" {
		t.Fatalf("unexpected disable command: %+v", cmds)
	}
}

func TestCommandsForProfileDisablesConnectedMonitorMissingFromProfile(t *testing.T) {
	laptop := hypr.Monitor{Name: "eDP-1", Make: "Samsung", Model: "Panel", Serial: "A1"}
	external := hypr.Monitor{Name: "DP-1", Make: "Microstep", Model: "MPG321UR-QD", Serial: "B2"}
	p := profile.New("laptop", []profile.OutputConfig{{
		Key: laptop.HardwareKey(), Name: laptop.Name, Enabled: true,
		Width: 2880, Height: 1800, Refresh: 120, Scale: 1.5,
	}})

	commands, err := CommandsForProfile(p, []hypr.Monitor{laptop, external})
	if err != nil {
		t.Fatal(err)
	}
	if len(commands) != 2 || commands[1] != "DP-1,disable" {
		t.Fatalf("expected omitted external monitor to be explicitly disabled, got %+v", commands)
	}
}

func TestKeywordifyMonitorCommandsLeavesWorkspaceAndDispatchCommandsUntouched(t *testing.T) {
	commands := []string{
		"DP-1,2560x1440@144,0x0,1",
		"keyword workspace 1, monitor:desc:Dell U2720Q, default:true",
		"dispatch moveworkspacetomonitor 1 DP-1",
	}

	got := keywordifyMonitorCommands(commands)
	want := []string{
		"keyword monitor DP-1,2560x1440@144,0x0,1",
		"keyword workspace 1, monitor:desc:Dell U2720Q, default:true",
		"dispatch moveworkspacetomonitor 1 DP-1",
	}

	if len(got) != len(want) {
		t.Fatalf("expected %d commands, got %d", len(want), len(got))
	}
	for idx := range want {
		if got[idx] != want[idx] {
			t.Fatalf("command %d mismatch: got %q want %q", idx, got[idx], want[idx])
		}
	}
}

func TestWorkspaceCommandsForProfileIncludeKeywordAndDispatch(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "DP-1", Description: "Microstep MPG321UR-QD", Make: "Microstep", Model: "MPG321UR-QD", Serial: "A1"},
		{Name: "eDP-1", Description: "Samsung Display Corp. ATNA60CL10-0", Make: "Samsung Display Corp.", Model: "ATNA60CL10-0", Serial: "B2"},
	}
	p := profile.New("desk", []profile.OutputConfig{
		{Key: monitors[0].HardwareKey(), Name: monitors[0].Name, Enabled: true, Scale: 1},
		{Key: monitors[1].HardwareKey(), Name: monitors[1].Name, Enabled: true, Scale: 1},
	})
	p.Workspaces = profile.WorkspaceSettings{
		Enabled:  true,
		Strategy: profile.WorkspaceStrategyManual,
		Rules: []profile.WorkspaceRule{
			{Workspace: "1", OutputKey: monitors[0].HardwareKey(), OutputName: monitors[0].Name, Default: true, Persistent: true},
			{Workspace: "2", OutputKey: monitors[1].HardwareKey(), OutputName: monitors[1].Name, Default: true, Persistent: true},
		},
	}

	got := WorkspaceCommandsForProfile(p, monitors)
	want := []string{
		"keyword workspace 1, monitor:desc:Microstep MPG321UR-QD, default:true, persistent:true",
		"dispatch moveworkspacetomonitor 1 DP-1",
		"keyword workspace 2, monitor:desc:Samsung Display Corp. ATNA60CL10-0, default:true, persistent:true",
		"dispatch moveworkspacetomonitor 2 eDP-1",
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d workspace commands, got %d: %v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("workspace command %d mismatch: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestWorkspaceCommandsForProfileUseLuaDispatcherForLuaConfig(t *testing.T) {
	p := profile.New("desk", []profile.OutputConfig{
		{Key: monitors[0].HardwareKey(), Name: monitors[0].Name, Enabled: true, Scale: 1},
	})
	p.Workspaces = profile.WorkspaceSettings{
		Enabled:  true,
		Strategy: profile.WorkspaceStrategyManual,
		Rules: []profile.WorkspaceRule{
			{Workspace: "dev docs", OutputKey: monitors[0].HardwareKey(), OutputName: monitors[0].Name},
		},
	}

	got := workspaceCommandsForProfile(p, monitors, true)
	want := []string{
		`dispatch hl.dsp.workspace.move({ workspace = "dev docs", monitor = "DP-1" })`,
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d workspace commands, got %d: %v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("workspace command %d mismatch: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestSnapshotStateUsesLuaDispatcherForWorkspacePlacement(t *testing.T) {
	commands := snapshotState(monitors, []hypr.WorkspaceRule{
		{WorkspaceString: "2", Monitor: "DP-1"},
	}, []hypr.WorkspaceState{
		{Name: "2", Monitor: "DP-1"},
		{Name: "special:scratchpad", Monitor: "DP-1"},
	}, true)
	want := []string{`dispatch hl.dsp.workspace.move({ workspace = "2", monitor = "DP-1" })`}
	if len(commands) != len(want) || commands[0] != want[0] {
		t.Fatalf("unexpected Lua snapshot commands: got %v want %v", commands, want)
	}
}

func TestCommandsForProfileResolveDuplicateMonitorsByConnector(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "DP-5", Description: "VIE C24PULSE 0x01010101", Make: "VIE", Model: "C24PULSE", Serial: "0x01010101"},
		{Name: "DP-6", Description: "VIE C24PULSE 0x01010101", Make: "VIE", Model: "C24PULSE", Serial: "0x01010101"},
	}
	legacyKey := monitors[0].HardwareKey()
	p := profile.New("desk", []profile.OutputConfig{
		{Key: legacyKey, Name: "DP-5", Enabled: true, Width: 1920, Height: 1080, Refresh: 75, X: 0, Y: 0, Scale: 1},
		{Key: legacyKey, Name: "DP-6", Enabled: true, Width: 1920, Height: 1080, Refresh: 75, X: 1920, Y: 0, Scale: 1},
	})

	cmds, err := CommandsForProfile(p, monitors)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cmds) != 2 {
		t.Fatalf("expected 2 commands, got %d: %v", len(cmds), cmds)
	}
	if !strings.HasPrefix(cmds[0], "DP-5,") {
		t.Fatalf("expected first command to target DP-5, got %q", cmds[0])
	}
	if !strings.HasPrefix(cmds[1], "DP-6,") {
		t.Fatalf("expected second command to target DP-6, got %q", cmds[1])
	}
}

func TestValidateLayoutRejectsOverlaps(t *testing.T) {
	outputs := []profile.OutputConfig{
		{
			Key:     "left",
			Name:    "DP-1",
			Enabled: true,
			Width:   2560,
			Height:  1440,
			X:       0,
			Y:       0,
			Scale:   1,
		},
		{
			Key:     "right",
			Name:    "eDP-1",
			Enabled: true,
			Width:   1920,
			Height:  1200,
			X:       2500,
			Y:       0,
			Scale:   1,
		},
	}

	if err := ValidateLayout(outputs); err == nil {
		t.Fatal("expected overlap validation error")
	}
}

func TestRenderHyprlandConfigUsesMonitorV2WithWorkspaceRules(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "DP-1", Description: "Microstep MPG321UR-QD", Make: "Microstep", Model: "MPG321UR-QD"},
	}
	p := profile.New("desk", []profile.OutputConfig{{
		Key:       monitors[0].HardwareKey(),
		Name:      "DP-1",
		Enabled:   true,
		Width:     3840,
		Height:    2160,
		Refresh:   143.99,
		X:         3720,
		Y:         951,
		Scale:     1.33,
		VRR:       1,
		Transform: 0,
		Bitdepth:  10,
		CM:        "wide",
	}})
	p.Workspaces = profile.WorkspaceSettings{
		Enabled:       true,
		Strategy:      profile.WorkspaceStrategySequential,
		MaxWorkspaces: 3,
		GroupSize:     3,
		MonitorOrder:  []string{monitors[0].HardwareKey()},
	}

	rendered, err := RenderHyprlandConfig(p, monitors, true)
	if err != nil {
		t.Fatalf("unexpected render error: %v", err)
	}

	for _, want := range []string{
		"# Generated by hyprmoncfg",
		"monitorv2 {",
		"output = desc:Microstep MPG321UR-QD",
		"position = 3720x951",
		"scale = 1.33",
		"vrr = 1",
		"bitdepth = 10",
		"cm = wide",
		"workspace = 1, monitor:desc:Microstep MPG321UR-QD, default:true, persistent:true",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered config missing %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "persistent:true\n\nworkspace = 2") {
		t.Fatalf("expected workspace rules to be contiguous without blank lines, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "persistent:true\nworkspace = 2, monitor:desc:Microstep MPG321UR-QD") {
		t.Fatalf("expected workspace rules to be separated by a single newline, got:\n%s", rendered)
	}
}

func TestRenderLuaConfigUsesMonitorV2WithWorkspaceRules(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "DP-1", Description: "Microstep MPG321UR-QD", Make: "Microstep", Model: "MPG321UR-QD"},
	}
	p := profile.New("desk", []profile.OutputConfig{{
		Key:      monitors[0].HardwareKey(),
		Name:     "DP-1",
		Enabled:  true,
		Width:    3840,
		Height:   2160,
		Refresh:  143.99,
		X:        3720,
		Y:        951,
		Scale:    1.33,
		VRR:      1,
		Bitdepth: 10,
		CM:       "wide",
	}})
	p.Workspaces = profile.WorkspaceSettings{
		Enabled:       true,
		Strategy:      profile.WorkspaceStrategySequential,
		MaxWorkspaces: 1,
		GroupSize:     1,
		MonitorOrder:  []string{monitors[0].HardwareKey()},
	}

	rendered, err := RenderConfig(p, monitors, RenderOptions{Format: config.HyprConfigLua, UseMonitorV2: true})
	if err != nil {
		t.Fatalf("unexpected render error: %v", err)
	}

	for _, want := range []string{
		"-- Generated by hyprmoncfg",
		"hl.monitor({",
		`output = "desc:Microstep MPG321UR-QD",`,
		`mode = "3840x2160@143.99",`,
		`position = "3720x951",`,
		"scale = 1.33,",
		"vrr = 1,",
		"bitdepth = 10,",
		`cm = "wide",`,
		`hl.workspace_rule({ workspace = "1", monitor = "desc:Microstep MPG321UR-QD", default = true, persistent = true })`,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered lua config missing %q:\n%s", want, rendered)
		}
	}
}

func TestRenderLuaConfigDisablesConnectedMonitorMissingFromProfile(t *testing.T) {
	laptop := hypr.Monitor{Name: "eDP-1", Description: "Samsung Panel", Make: "Samsung", Model: "Panel", Serial: "A1"}
	external := hypr.Monitor{Name: "DP-1", Description: "Microstep MPG321UR-QD", Make: "Microstep", Model: "MPG321UR-QD", Serial: "B2"}
	p := profile.New("laptop", []profile.OutputConfig{{
		Key: laptop.HardwareKey(), Name: laptop.Name, Enabled: true,
		Width: 2880, Height: 1800, Refresh: 120, Scale: 1.5,
	}})

	rendered, err := RenderConfig(p, []hypr.Monitor{laptop, external}, RenderOptions{Format: config.HyprConfigLua, UseMonitorV2: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`output = "desc:Microstep MPG321UR-QD",`, "disabled = true,"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected generated config to override wildcard monitor defaults with %q:\n%s", want, rendered)
		}
	}
}

func TestValidateAppliedProfileRejectsEnabledMonitorMissingFromProfile(t *testing.T) {
	laptop := hypr.Monitor{Name: "eDP-1", Make: "Samsung", Model: "Panel", Serial: "A1", Width: 2880, Height: 1800, RefreshRate: 120, Scale: 1.5}
	external := hypr.Monitor{Name: "DP-1", Make: "Microstep", Model: "MPG321UR-QD", Serial: "B2", Width: 3840, Height: 2160, RefreshRate: 144, Scale: 1}
	p := profile.New("laptop", []profile.OutputConfig{{
		Key: laptop.HardwareKey(), Name: laptop.Name, Enabled: true,
		Width: 2880, Height: 1800, Refresh: 120, Scale: 1.5,
	}})

	if err := ValidateAppliedProfile(p, []hypr.Monitor{laptop, external}, []hypr.Monitor{laptop, external}); err == nil {
		t.Fatal("expected an omitted external monitor left enabled to fail validation")
	}
	external.Disabled = true
	if err := ValidateAppliedProfile(p, []hypr.Monitor{laptop, external}, []hypr.Monitor{laptop, external}); err != nil {
		t.Fatalf("expected the explicitly disabled external monitor to pass validation: %v", err)
	}
}

func TestRenderLuaConfigQuotesDescriptionSelector(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "DP-1", Description: `Dell, Inc. #A $B "{{raw}}" \Panel`, Make: "Dell", Model: "Panel", Serial: "A1"},
	}
	p := profile.New("desk", []profile.OutputConfig{{
		Key:      monitors[0].HardwareKey(),
		MatchKey: monitors[0].HardwareKey(),
		Name:     "DP-1",
		Enabled:  true,
		Width:    2560,
		Height:   1440,
		Refresh:  60,
		Scale:    1,
	}})

	rendered, err := RenderConfig(p, monitors, RenderOptions{Format: config.HyprConfigLua, UseMonitorV2: true})
	if err != nil {
		t.Fatalf("unexpected render error: %v", err)
	}
	want := `output = "desc:Dell, Inc. #A $B \"{{raw}}\" \\Panel",`
	if !strings.Contains(rendered, want) {
		t.Fatalf("expected lua render to contain %q, got:\n%s", want, rendered)
	}
	if strings.Contains(rendered, `output = "DP-1"`) {
		t.Fatalf("expected lua render to keep desc selector, got:\n%s", rendered)
	}
}

func TestRenderHyprlandConfigUsesConnectorsForAmbiguousDuplicateMonitors(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "DP-5", Description: "VIE C24PULSE 0x01010101", Make: "VIE", Model: "C24PULSE", Serial: "0x01010101"},
		{Name: "DP-6", Description: "VIE C24PULSE 0x01010101", Make: "VIE", Model: "C24PULSE", Serial: "0x01010101"},
	}
	legacyKey := monitors[0].HardwareKey()
	p := profile.New("desk", []profile.OutputConfig{
		{Key: legacyKey, Name: "DP-5", Enabled: true, Width: 1920, Height: 1080, Refresh: 75, X: 0, Y: 0, Scale: 1},
		{Key: legacyKey, Name: "DP-6", Enabled: true, Width: 1920, Height: 1080, Refresh: 75, X: 1920, Y: 0, Scale: 1},
	})
	p.Workspaces = profile.WorkspaceSettings{
		Enabled:  true,
		Strategy: profile.WorkspaceStrategyManual,
		Rules: []profile.WorkspaceRule{
			{Workspace: "1", OutputKey: legacyKey, OutputName: "DP-5", Default: true, Persistent: true},
			{Workspace: "2", OutputKey: legacyKey, OutputName: "DP-6", Default: true, Persistent: true},
		},
	}
	p.Normalize()

	rendered, err := RenderHyprlandConfig(p, monitors, true)
	if err != nil {
		t.Fatalf("unexpected render error: %v", err)
	}
	for _, want := range []string{
		"output = DP-5",
		"output = DP-6",
		"workspace = 1, monitor:DP-5, default:true, persistent:true",
		"workspace = 2, monitor:DP-6, default:true, persistent:true",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected duplicate-monitor render to contain %q, got:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "output = desc:VIE C24PULSE 0x01010101") {
		t.Fatalf("expected duplicate monitors to avoid desc selectors, got:\n%s", rendered)
	}
}

func TestRenderHyprlandConfigEscapesDescriptionCommentMarker(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "DP-1", Description: "Dell Inc. Dell S2716DG #ASMy+EjOdybd {{raw}}", Make: "Dell Inc.", Model: "Dell S2716DG", Serial: "#ASMy+EjOdybd"},
		{Name: "DP-2", Description: "Dell Inc. Dell S2716DG #ASO9n2jmng3d", Make: "Dell Inc.", Model: "Dell S2716DG", Serial: "#ASO9n2jmng3d"},
	}
	p := profile.New("desk", []profile.OutputConfig{
		{Key: monitors[0].HardwareKey(), MatchKey: monitors[0].HardwareKey(), Name: "DP-1", Enabled: true, Width: 2560, Height: 1440, Refresh: 143.96, X: 0, Y: 0, Scale: 1},
		{Key: monitors[1].HardwareKey(), MatchKey: monitors[1].HardwareKey(), Name: "DP-2", Enabled: true, Width: 2560, Height: 1440, Refresh: 143.96, X: 2560, Y: 0, Scale: 1},
	})
	p.Workspaces = profile.WorkspaceSettings{
		Enabled:  true,
		Strategy: profile.WorkspaceStrategyManual,
		Rules: []profile.WorkspaceRule{
			{Workspace: "1", OutputKey: monitors[0].HardwareKey(), OutputName: "DP-1", Default: true, Persistent: true},
			{Workspace: "2", OutputKey: monitors[1].HardwareKey(), OutputName: "DP-2", Default: true, Persistent: true},
		},
	}

	rendered, err := RenderHyprlandConfig(p, monitors, true)
	if err != nil {
		t.Fatalf("unexpected render error: %v", err)
	}
	for _, want := range []string{
		`output = desc:Dell Inc. Dell S2716DG ##ASMy+EjOdybd \{\{raw}}`,
		"output = desc:Dell Inc. Dell S2716DG ##ASO9n2jmng3d",
		`workspace = 1, monitor:desc:Dell Inc. Dell S2716DG ##ASMy+EjOdybd \{\{raw}}, default:true, persistent:true`,
		"workspace = 2, monitor:desc:Dell Inc. Dell S2716DG ##ASO9n2jmng3d, default:true, persistent:true",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected escaped render to contain %q, got:\n%s", want, rendered)
		}
	}
	for _, raw := range []string{
		"desc:Dell Inc. Dell S2716DG #ASMy+EjOdybd",
		"desc:Dell Inc. Dell S2716DG #ASO9n2jmng3d",
		" {{raw}}",
	} {
		if strings.Contains(rendered, raw) {
			t.Fatalf("expected raw comment marker %q to be escaped, got:\n%s", raw, rendered)
		}
	}
}

func TestRenderHyprlandConfigFallsBackToConnectorForUnsafeHyprlangDescSelector(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "DP-1", Description: "Dell, Inc. $PANEL", Make: "Dell", Model: "S2716DG", Serial: "A1"},
	}
	p := profile.New("desk", []profile.OutputConfig{{
		Key:      monitors[0].HardwareKey(),
		MatchKey: monitors[0].HardwareKey(),
		Name:     "DP-1",
		Enabled:  true,
		Width:    2560,
		Height:   1440,
		Refresh:  143.96,
		Scale:    1,
	}})
	p.Workspaces = profile.WorkspaceSettings{
		Enabled:  true,
		Strategy: profile.WorkspaceStrategyManual,
		Rules: []profile.WorkspaceRule{
			{Workspace: "1", OutputKey: monitors[0].HardwareKey(), OutputName: "DP-1", Default: true, Persistent: true},
		},
	}

	rendered, err := RenderHyprlandConfig(p, monitors, true)
	if err != nil {
		t.Fatalf("unexpected render error: %v", err)
	}
	for _, want := range []string{
		"output = DP-1",
		"workspace = 1, monitor:DP-1, default:true, persistent:true",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected connector fallback render to contain %q, got:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "desc:Dell, Inc. $PANEL") {
		t.Fatalf("expected unsafe desc selector to be avoided, got:\n%s", rendered)
	}
}

func TestCommandsForProfileMirror(t *testing.T) {
	primary := hypr.Monitor{Name: "DP-1", Make: "Dell", Model: "U2720Q", Serial: "A1"}
	secondary := hypr.Monitor{Name: "HDMI-A-1", Make: "LG", Model: "27GP850", Serial: "B2"}
	p := profile.New("mirror", []profile.OutputConfig{
		{
			Key:     primary.HardwareKey(),
			Name:    "DP-1",
			Enabled: true,
			Width:   2560,
			Height:  1440,
			Refresh: 144,
			Scale:   1,
		},
		{
			Key:      secondary.HardwareKey(),
			Name:     "HDMI-A-1",
			Enabled:  true,
			Width:    1920,
			Height:   1080,
			Refresh:  60,
			Scale:    1,
			MirrorOf: primary.HardwareKey(),
		},
	})

	cmds, err := CommandsForProfile(p, []hypr.Monitor{primary, secondary})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cmds) != 2 {
		t.Fatalf("expected 2 commands, got %d: %v", len(cmds), cmds)
	}
	if !strings.Contains(cmds[1], "1920x1080@60") {
		t.Fatalf("expected mirror command to include resolution, got: %s", cmds[1])
	}
	if !strings.Contains(cmds[1], "mirror,DP-1") {
		t.Fatalf("expected mirror command targeting DP-1, got: %s", cmds[1])
	}
}

func TestRenderHyprlandConfigMirrorV1(t *testing.T) {
	primary := hypr.Monitor{Name: "DP-1", Description: "Dell U2720Q", Make: "Dell", Model: "U2720Q", Serial: "A1"}
	secondary := hypr.Monitor{Name: "HDMI-A-1", Description: "LG 27GP850", Make: "LG", Model: "27GP850", Serial: "B2"}
	p := profile.New("mirror", []profile.OutputConfig{
		{
			Key:     primary.HardwareKey(),
			Name:    "DP-1",
			Enabled: true,
			Width:   2560,
			Height:  1440,
			Refresh: 144,
			Scale:   1,
		},
		{
			Key:      secondary.HardwareKey(),
			Name:     "HDMI-A-1",
			Enabled:  true,
			Width:    1920,
			Height:   1080,
			Refresh:  60,
			Scale:    1,
			MirrorOf: primary.HardwareKey(),
		},
	})

	rendered, err := RenderHyprlandConfig(p, []hypr.Monitor{primary, secondary}, false)
	if err != nil {
		t.Fatalf("unexpected render error: %v", err)
	}
	for _, want := range []string{"1920x1080@60", "mirror,desc:Dell U2720Q"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected v1 mirror line to contain %q, got:\n%s", want, rendered)
		}
	}
}

func TestRenderHyprlandConfigMirrorV2(t *testing.T) {
	primary := hypr.Monitor{Name: "DP-1", Description: "Dell U2720Q", Make: "Dell", Model: "U2720Q", Serial: "A1"}
	secondary := hypr.Monitor{Name: "HDMI-A-1", Description: "LG 27GP850", Make: "LG", Model: "27GP850", Serial: "B2"}
	p := profile.New("mirror", []profile.OutputConfig{
		{
			Key:     primary.HardwareKey(),
			Name:    "DP-1",
			Enabled: true,
			Width:   2560,
			Height:  1440,
			Refresh: 144,
			Scale:   1,
		},
		{
			Key:      secondary.HardwareKey(),
			Name:     "HDMI-A-1",
			Enabled:  true,
			Width:    1920,
			Height:   1080,
			Refresh:  60,
			Scale:    1,
			MirrorOf: primary.HardwareKey(),
		},
	})

	rendered, err := RenderHyprlandConfig(p, []hypr.Monitor{primary, secondary}, true)
	if err != nil {
		t.Fatalf("unexpected render error: %v", err)
	}
	// The mirror block should contain mode, position, scale, AND mirror.
	mirrorIdx := strings.Index(rendered, "desc:LG 27GP850")
	if mirrorIdx < 0 {
		t.Fatalf("mirror monitor block not found in:\n%s", rendered)
	}
	mirrorBlock := rendered[mirrorIdx:]
	endIdx := strings.Index(mirrorBlock, "}")
	if endIdx >= 0 {
		mirrorBlock = mirrorBlock[:endIdx]
	}
	for _, want := range []string{"mode = 1920x1080@60", "position =", "scale =", "mirror = desc:Dell U2720Q"} {
		if !strings.Contains(mirrorBlock, want) {
			t.Fatalf("mirror v2 block should contain %q, got:\n%s", want, rendered)
		}
	}
}

func TestValidateLayoutSkipsMirroredMonitors(t *testing.T) {
	outputs := []profile.OutputConfig{
		{
			Key:     "primary",
			Name:    "DP-1",
			Enabled: true,
			Width:   2560,
			Height:  1440,
			X:       0,
			Y:       0,
			Scale:   1,
		},
		{
			Key:      "secondary",
			Name:     "HDMI-A-1",
			Enabled:  true,
			Width:    2560,
			Height:   1440,
			X:        0,
			Y:        0,
			Scale:    1,
			MirrorOf: "primary",
		},
	}

	if err := ValidateLayout(outputs); err != nil {
		t.Fatalf("mirrored monitor should not trigger overlap: %v", err)
	}
}

func TestSnapshotCommandsMirror(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "DP-1", Width: 2560, Height: 1440, RefreshRate: 144, Scale: 1},
		{Name: "HDMI-A-1", Width: 1920, Height: 1080, RefreshRate: 60, MirrorOf: "DP-1", Scale: 1},
	}

	cmds := SnapshotCommands(monitors)
	if len(cmds) != 2 {
		t.Fatalf("expected 2 commands, got %d: %v", len(cmds), cmds)
	}
	if !strings.Contains(cmds[1], "1920x1080@60") {
		t.Fatalf("expected snapshot mirror command to include resolution, got: %s", cmds[1])
	}
	if !strings.Contains(cmds[1], "mirror,DP-1") {
		t.Fatalf("expected snapshot mirror command, got: %s", cmds[1])
	}
}

func TestEngineApplyAndRevertReplayWorkspacePlacement(t *testing.T) {
	p := newTestProfile()
	p.Workspaces = profile.WorkspaceSettings{
		Enabled:  true,
		Strategy: profile.WorkspaceStrategyManual,
		Rules: []profile.WorkspaceRule{
			{Workspace: "1", OutputKey: monitors[0].HardwareKey(), OutputName: monitors[0].Name, Default: true, Persistent: true},
			{Workspace: "2", OutputKey: monitors[1].HardwareKey(), OutputName: monitors[1].Name, Default: true, Persistent: true},
		},
	}

	engine, logPath, err := initTestEngine(t)
	if err != nil {
		t.Fatalf("init test engine: %v", err)
	}

	ctx := context.Background()
	snapshot, err := engine.Apply(ctx, p, monitors)
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if err := engine.Revert(ctx, snapshot); err != nil {
		t.Fatalf("revert failed: %v", err)
	}

	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read hyprctl log: %v", err)
	}
	log := string(logBytes)

	for _, want := range []string{
		"reload",
		"BATCH:keyword workspace 1, monitor:desc:Microstep MPG321UR-QD, default:true, persistent:true ; dispatch moveworkspacetomonitor 1 DP-1 ; keyword workspace 2, monitor:desc:Samsung Display Corp. ATNA60CL10-0, default:true, persistent:true ; dispatch moveworkspacetomonitor 2 eDP-1",
		"BATCH:keyword monitor DP-1,3840x2160@143.99,0x0,1,transform,0,vrr,1 ; keyword monitor eDP-1,2880x1800@120.00,3840x0,1,transform,0,vrr,0 ; keyword workspace 1, monitor:desc:Samsung Display Corp. ATNA60CL10-0, default:true, persistent:true ; keyword workspace 2, monitor:desc:Microstep MPG321UR-QD, default:true, persistent:true ; dispatch moveworkspacetomonitor 1 eDP-1 ; dispatch moveworkspacetomonitor 2 DP-1",
	} {
		if !strings.Contains(log, want) {
			t.Fatalf("expected hyprctl log to contain %q, got:\n%s", want, log)
		}
	}
}

func TestEngineApplyWaitsForDeferredMonitorReload(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "hyprctl.log")
	statePath := filepath.Join(dir, "monitor-query-count")
	stalePath := filepath.Join(dir, "stale-monitors.json")
	appliedPath := filepath.Join(dir, "applied-monitors.json")
	monitorsConfPath := filepath.Join(dir, "monitors.conf")
	hyprlandConfigPath := filepath.Join(dir, "hyprland.conf")
	hyprctlPath := filepath.Join(dir, "hyprctl")

	targetMonitors := append([]hypr.Monitor(nil), monitors...)
	targetMonitors[0].Width = 2560
	targetMonitors[0].Height = 1440
	targetMonitors[0].RefreshRate = 60

	writeJSON := func(path string, value any) {
		t.Helper()
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("marshal %s: %v", path, err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	writeJSON(stalePath, monitors)
	writeJSON(appliedPath, targetMonitors)

	hyprctlScript := fmt.Sprintf(`#!/usr/bin/env bash
set -eu
STATE=%q
STALE=%q
APPLIED=%q
if [ "${1-}" = "--instance" ]; then
  shift 2
fi
cmd1="${1-}"
cmd2="${2-}"
cmd3="${3-}"
printf '%%s\n' "$*" >> "$HYPRCTL_LOG"

if [ "$cmd1" = "-j" ] && [ "$cmd2" = "version" ]; then
  printf '{"version":"0.54.0"}'
  exit 0
fi

if [ "$cmd1" = "-j" ] && [ "$cmd2" = "monitors" ] && [ "$cmd3" = "all" ]; then
  count=0
  if [ -f "$STATE" ]; then
    count=$(cat "$STATE")
  fi
  count=$((count + 1))
  printf '%%s' "$count" > "$STATE"
  if [ "$count" -eq 1 ]; then
    cat "$STALE"
  else
    cat "$APPLIED"
  fi
  exit 0
fi

if [ "$cmd1" = "-j" ] && [ "$cmd2" = "workspacerules" ]; then
  printf '[]'
  exit 0
fi

if [ "$cmd1" = "-j" ] && [ "$cmd2" = "workspaces" ]; then
  printf '[]'
  exit 0
fi

if [ "$cmd1" = "reload" ]; then
  exit 0
fi

if [ "$cmd1" = "--batch" ]; then
  exit 0
fi

echo "unexpected args: $*" >&2
exit 1
`, statePath, stalePath, appliedPath)
	if err := os.WriteFile(hyprctlPath, []byte(hyprctlScript), 0o755); err != nil {
		t.Fatalf("write fake hyprctl: %v", err)
	}
	if err := os.WriteFile(monitorsConfPath, []byte(config.GeneratedLegacyHeader+"\n# initial\n"), 0o644); err != nil {
		t.Fatalf("write monitors.conf: %v", err)
	}
	if err := os.WriteFile(hyprlandConfigPath, []byte("source = "+monitorsConfPath+"\n"), 0o644); err != nil {
		t.Fatalf("write hyprland.conf: %v", err)
	}

	t.Setenv("HYPRCTL_LOG", logPath)
	t.Setenv("HYPRLAND_INSTANCE_SIGNATURE", "sig-test")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	client, err := hypr.NewClient()
	if err != nil {
		t.Fatalf("new hypr client: %v", err)
	}
	engine := Engine{Client: client, MonitorsConfPath: monitorsConfPath, HyprlandConfigPath: hyprlandConfigPath}

	p := newTestProfile()
	p.Outputs[0].Width = 2560
	p.Outputs[0].Height = 1440
	p.Outputs[0].Refresh = 60

	if _, err := engine.Apply(context.Background(), p, monitors, ApplyModeNonInteractive); err != nil {
		t.Fatalf("apply should wait for deferred monitor reload: %v", err)
	}

	countBytes, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read monitor query count: %v", err)
	}
	if got := strings.TrimSpace(string(countBytes)); got != "2" {
		t.Fatalf("expected two monitor validation queries, got %s", got)
	}
}

func TestEnginePostApply(t *testing.T) {
	engine, logPath, err := initTestEngine(t)
	if err != nil {
		t.Fatalf("init test engine: %v", err)
	}

	dir := t.TempDir()
	execPath := filepath.Join(dir, "post_apply.sh")

	execScript := `#!/bin/bash
	OUT_FILE="$1"

	if [ -z "$OUT_FILE" ]; then
	  exit 1
	fi

	touch "$OUT_FILE"`

	if err := os.WriteFile(execPath, []byte(execScript), 0o755); err != nil {
		t.Fatalf("write exec script: %v", err)
	}

	ctx := context.Background()

	testcases := []struct {
		ExecPath        string
		Name            string
		Mode            applyMode
		OutFile         string
		ShouldTouchFile bool
		VerifyResult    func()
	}{
		{
			Name:    "should_not_call_post_exec_when_interactive",
			Mode:    ApplyModeInteractive,
			OutFile: filepath.Join(dir, "8d0f0878-6240-4deb-ac28-b5f2f251a606"),
		},
		{
			Name:            "should_call_post_exec_when_noninteractive",
			Mode:            ApplyModeNonInteractive,
			OutFile:         filepath.Join(dir, "b33b65c7-2c39-4df6-b56b-3ff64c816ff6"),
			ShouldTouchFile: true,
		},
		{
			Name:     "should_handle_space_only_exec",
			ExecPath: "   ",
			Mode:     ApplyModeNonInteractive,
			OutFile:  filepath.Join(dir, "50000812-ca77-4f76-a1ed-7f7659c65fe3"),
		},
		{
			Name: "should_continue_on_post_apply_fail",
			Mode: ApplyModeNonInteractive,
			VerifyResult: func() {
				logBytes, err := os.ReadFile(logPath)
				if err != nil {
					t.Fatalf("read hyprctl log: %v", err)
				}
				log := string(logBytes)

				want := "post apply:"

				t.Log(log)

				if !strings.Contains(log, want) {
					t.Fatalf("Expected log to contain string \"%s\"", want)
				}
			},
		},
	}

	p := newTestProfile()

	for _, tc := range testcases {
		t.Run(tc.Name, func(st *testing.T) {

			if tc.ExecPath == "" {
				p.Exec = fmt.Sprintf("%s %s", execPath, tc.OutFile)
			} else {
				p.Exec = tc.ExecPath
			}

			_, err := engine.Apply(ctx, p, monitors, tc.Mode)
			if err != nil {
				st.Fatalf("%s: apply: %v", tc.Name, err)
			}

			var fileExists bool

			_, err = os.Stat(tc.OutFile)
			if err != nil {
				if !errors.Is(err, os.ErrNotExist) {
					st.Fatalf("stat file: %v", err)
				}
			} else {
				fileExists = true
			}

			if fileExists != tc.ShouldTouchFile {
				st.Fatalf("expected file to exist: %v. File exists: %v.", tc.ShouldTouchFile, fileExists)
			}

			if tc.VerifyResult != nil {
				tc.VerifyResult()
			}
		})
	}
}

func TestEnginePostApplyExpandsHomePath(t *testing.T) {
	engine, _, err := initTestEngine(t)
	if err != nil {
		t.Fatalf("init test engine: %v", err)
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	execPath := filepath.Join(home, "post-apply.sh")
	outPath := filepath.Join(home, "post-apply.out")
	execScript := `#!/bin/sh
touch "$1"`
	if err := os.WriteFile(execPath, []byte(execScript), 0o755); err != nil {
		t.Fatalf("write exec script: %v", err)
	}

	p := newTestProfile()
	p.Exec = "~/post-apply.sh " + outPath
	if err := engine.PostApply(context.Background(), p); err != nil {
		t.Fatalf("post apply: %v", err)
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Fatalf("expected post-apply script to create output file: %v", err)
	}
}

func TestValidatePostApplyExec(t *testing.T) {
	dir := t.TempDir()
	execPath := filepath.Join(dir, "ok.sh")
	if err := os.WriteFile(execPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write executable script: %v", err)
	}
	nonExecPath := filepath.Join(dir, "not-ok.sh")
	if err := os.WriteFile(nonExecPath, []byte("#!/bin/sh\nexit 0\n"), 0o644); err != nil {
		t.Fatalf("write non-executable script: %v", err)
	}

	for _, tc := range []struct {
		name    string
		command string
		wantErr string
	}{
		{name: "empty", command: ""},
		{name: "executable path", command: execPath},
		{name: "path with args", command: fmt.Sprintf("%s --flag value", execPath)},
		{name: "non executable path", command: nonExecPath, wantErr: "not executable"},
		{name: "missing path", command: filepath.Join(dir, "missing.sh"), wantErr: "does not exist"},
		{name: "missing path command", command: "definitely-missing-hyprmoncfg-test-command", wantErr: "not executable or was not found in PATH"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidatePostApplyExec(tc.command)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("validate exec: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestEngineApplyPostApplyFailureWithNilLogger(t *testing.T) {
	engine, _, err := initTestEngine(t)
	if err != nil {
		t.Fatalf("init test engine: %v", err)
	}
	engine.Logf = nil

	p := newTestProfile()
	p.Exec = "/definitely/missing/post-apply-script"

	if _, err := engine.Apply(context.Background(), p, monitors, ApplyModeNonInteractive); err != nil {
		t.Fatalf("apply should not fail when post-apply logging is unset: %v", err)
	}
}

func TestAddLuaExecutionProbe(t *testing.T) {
	rendered, probe, err := addLuaExecutionProbe("-- generated\nhl.monitor({})\n")
	if err != nil {
		t.Fatalf("add probe: %v", err)
	}
	if !strings.HasPrefix(probe, luaProbePrefix) {
		t.Fatalf("expected probe prefix %q, got %q", luaProbePrefix, probe)
	}
	if want := "_G." + probe + " = true\n"; !strings.HasSuffix(rendered, want) {
		t.Fatalf("expected rendered config to end with %q, got:\n%s", want, rendered)
	}
}

func TestEngineApplyWritesLuaMonitorConfigWhenHyprlandLuaIsActive(t *testing.T) {
	engine, monitorsLuaPath, logPath := initLuaProbeTestEngine(t, "ok", nil)

	if _, err := engine.Apply(context.Background(), newTestProfile(), monitors, ApplyModeNonInteractive); err != nil {
		t.Fatalf("apply lua config: %v", err)
	}

	renderedBytes, err := os.ReadFile(monitorsLuaPath)
	if err != nil {
		t.Fatalf("read generated lua config: %v", err)
	}
	rendered := string(renderedBytes)
	if !strings.Contains(rendered, "hl.monitor({") {
		t.Fatalf("expected the generated lua config to contain monitor rules, got:\n%s", rendered)
	}
	if strings.Contains(rendered, luaProbePrefix) {
		t.Fatalf("expected the temporary execution probe to be removed, got:\n%s", rendered)
	}
	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read hyprctl log: %v", err)
	}
	if !strings.Contains(string(logBytes), "eval assert(_G."+luaProbePrefix) {
		t.Fatalf("expected Hyprland to query a generated execution probe, got:\n%s", logBytes)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(monitorsLuaPath), "monitors.conf")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected monitors.conf not to be written, stat error: %v", err)
	}
}

func TestEngineApplyRollsBackLuaConfigWhenProbeFails(t *testing.T) {
	tests := []struct {
		name    string
		initial []byte
	}{
		{name: "restore existing generated config", initial: []byte(config.GeneratedLuaHeader + "\n-- initial\n")},
		{name: "remove newly created config"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine, monitorsLuaPath, logPath := initLuaProbeTestEngine(t, "error: hyprmoncfg generated monitor config did not run", tt.initial)

			_, err := engine.Apply(context.Background(), newTestProfile(), monitors, ApplyModeNonInteractive)
			if err == nil {
				t.Fatal("expected missing Lua execution probe to fail apply")
			}
			if !strings.Contains(err.Error(), "did not run when Hyprland reloaded") {
				t.Fatalf("expected execution error, got %v", err)
			}
			if want := fmt.Sprintf("dofile(%q)", monitorsLuaPath); !strings.Contains(err.Error(), want) {
				t.Fatalf("expected absolute dofile suggestion %q, got %v", want, err)
			}

			if tt.initial == nil {
				if _, statErr := os.Stat(monitorsLuaPath); !errors.Is(statErr, os.ErrNotExist) {
					t.Fatalf("expected failed first apply to remove the generated config, stat error: %v", statErr)
				}
			} else {
				restored, readErr := os.ReadFile(monitorsLuaPath)
				if readErr != nil {
					t.Fatalf("read restored config: %v", readErr)
				}
				if string(restored) != string(tt.initial) {
					t.Fatalf("expected the original config to be restored, got:\n%s", restored)
				}
			}

			logBytes, readErr := os.ReadFile(logPath)
			if readErr != nil {
				t.Fatalf("read hyprctl log: %v", readErr)
			}
			reloads := 0
			for _, line := range strings.Split(strings.TrimSpace(string(logBytes)), "\n") {
				if line == "reload" {
					reloads++
				}
			}
			if reloads != 2 {
				t.Fatalf("expected apply and rollback reloads, got %d:\n%s", reloads, logBytes)
			}
			if strings.Contains(string(logBytes), "--batch") {
				t.Fatalf("expected probe failure before live commands, got:\n%s", logBytes)
			}
		})
	}
}

func initLuaProbeTestEngine(t *testing.T, evalReply string, initialTarget []byte) (Engine, string, string) {
	t.Helper()
	dir := t.TempDir()
	xdg := filepath.Join(dir, "xdg")
	hyprDir := filepath.Join(xdg, "hypr")
	if err := os.MkdirAll(hyprDir, 0o755); err != nil {
		t.Fatalf("mkdir hypr dir: %v", err)
	}
	hyprlandLuaPath := filepath.Join(hyprDir, "hyprland.lua")
	monitorsLuaPath := filepath.Join(hyprDir, "hyprmoncfg-monitors.lua")
	rootConfig := `dofile((os.getenv("OMARCHY_PATH") or "/usr/share/omarchy") .. "/default/hypr/bootstrap.lua")
pcall(require, "hypr.monitors")
`
	if err := os.WriteFile(hyprlandLuaPath, []byte(rootConfig), 0o644); err != nil {
		t.Fatalf("write hyprland.lua: %v", err)
	}
	if initialTarget != nil {
		if err := os.WriteFile(monitorsLuaPath, initialTarget, 0o644); err != nil {
			t.Fatalf("write generated lua config: %v", err)
		}
	}

	logPath := filepath.Join(dir, "hyprctl.log")
	hyprctlPath := filepath.Join(dir, "hyprctl")
	hyprctlScript := `#!/usr/bin/env bash
set -eu
if [ "${1-}" = "--instance" ]; then
  shift 2
fi
cmd1="${1-}"
cmd2="${2-}"
cmd3="${3-}"
printf '%s\n' "$*" >> "$HYPRCTL_LOG"

if [ "$cmd1" = "-j" ] && [ "$cmd2" = "version" ]; then
  printf '{"version":"0.55.0"}'
  exit 0
fi

if [ "$cmd1" = "-j" ] && [ "$cmd2" = "monitors" ] && [ "$cmd3" = "all" ]; then
  printf '%s' '[{"id":1,"name":"DP-1","description":"Microstep MPG321UR-QD","make":"Microstep","model":"MPG321UR-QD","serial":"A1","width":3840,"height":2160,"refreshRate":143.99,"x":0,"y":0,"scale":1,"transform":0,"focused":true,"dpmsStatus":true,"vrr":true,"disabled":false,"mirrorOf":"","activeWorkspace":{"id":1,"name":"1"}},{"id":2,"name":"eDP-1","description":"Samsung Display Corp. ATNA60CL10-0","make":"Samsung Display Corp.","model":"ATNA60CL10-0","serial":"B2","width":2880,"height":1800,"refreshRate":120,"x":3840,"y":0,"scale":1,"transform":0,"focused":false,"dpmsStatus":true,"vrr":false,"disabled":false,"mirrorOf":"","activeWorkspace":{"id":2,"name":"2"}}]'
  exit 0
fi

if [ "$cmd1" = "-j" ] && [ "$cmd2" = "workspacerules" ]; then
  printf '[]'
  exit 0
fi

if [ "$cmd1" = "-j" ] && [ "$cmd2" = "workspaces" ]; then
  printf '[]'
  exit 0
fi

if [ "$cmd1" = "reload" ]; then
  exit 0
fi

if [ "$cmd1" = "eval" ]; then
  printf '%s' "$HYPRMONCFG_TEST_EVAL_REPLY"
  exit 0
fi

if [ "$cmd1" = "--batch" ]; then
  exit 0
fi

echo "unexpected args: $*" >&2
exit 1
`
	if err := os.WriteFile(hyprctlPath, []byte(hyprctlScript), 0o755); err != nil {
		t.Fatalf("write fake hyprctl: %v", err)
	}

	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("HYPRLAND_INSTANCE_SIGNATURE", "sig-test")
	t.Setenv("HYPRCTL_LOG", logPath)
	t.Setenv("HYPRMONCFG_TEST_EVAL_REPLY", evalReply)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	client, err := hypr.NewClient()
	if err != nil {
		t.Fatalf("new hypr client: %v", err)
	}
	return Engine{Client: client}, monitorsLuaPath, logPath
}

func initTestEngine(t *testing.T) (engine *Engine, logPath string, err error) {
	dir := t.TempDir()
	logPath = filepath.Join(dir, "hyprctl.log")
	monitorsConfPath := filepath.Join(dir, "monitors.conf")
	hyprlandConfigPath := filepath.Join(dir, "hyprland.conf")
	hyprctlPath := filepath.Join(dir, "hyprctl")

	hyprctlScript := `#!/usr/bin/env bash
set -eu
if [ "${1-}" = "--instance" ]; then
  shift 2
fi
cmd1="${1-}"
cmd2="${2-}"
cmd3="${3-}"
printf '%s\n' "$*" >> "$HYPRCTL_LOG"

if [ "$cmd1" = "-j" ] && [ "$cmd2" = "version" ]; then
  printf '{"version":"0.54.0"}'
  exit 0
fi

if [ "$cmd1" = "-j" ] && [ "$cmd2" = "monitors" ] && [ "$cmd3" = "all" ]; then
  printf '%s' '[{"id":1,"name":"DP-1","description":"Microstep MPG321UR-QD","make":"Microstep","model":"MPG321UR-QD","serial":"A1","width":3840,"height":2160,"refreshRate":143.99,"x":0,"y":0,"scale":1,"transform":0,"focused":true,"dpmsStatus":true,"vrr":true,"disabled":false,"mirrorOf":"","activeWorkspace":{"id":1,"name":"1"}},{"id":2,"name":"eDP-1","description":"Samsung Display Corp. ATNA60CL10-0","make":"Samsung Display Corp.","model":"ATNA60CL10-0","serial":"B2","width":2880,"height":1800,"refreshRate":120,"x":3840,"y":0,"scale":1,"transform":0,"focused":false,"dpmsStatus":true,"vrr":false,"disabled":false,"mirrorOf":"","activeWorkspace":{"id":2,"name":"2"}}]'
  exit 0
fi

if [ "$cmd1" = "-j" ] && [ "$cmd2" = "workspacerules" ]; then
  printf '%s' '[{"workspaceString":"1","monitor":"desc:Samsung Display Corp. ATNA60CL10-0","default":true,"persistent":true},{"workspaceString":"2","monitor":"desc:Microstep MPG321UR-QD","default":true,"persistent":true}]'
  exit 0
fi

if [ "$cmd1" = "-j" ] && [ "$cmd2" = "workspaces" ]; then
  printf '%s' '[{"id":1,"name":"1","monitor":"eDP-1","monitorID":2,"windows":1,"ispersistent":true},{"id":2,"name":"2","monitor":"DP-1","monitorID":1,"windows":1,"ispersistent":true}]'
  exit 0
fi

if [ "$cmd1" = "reload" ]; then
  exit 0
fi

if [ "$cmd1" = "--batch" ]; then
  printf 'BATCH:%s\n' "$2" >> "$HYPRCTL_LOG"
  exit 0
fi

echo "unexpected args: $*" >&2
exit 1
`

	if err := os.WriteFile(monitorsConfPath, []byte(config.GeneratedLegacyHeader+"\n# initial\n"), 0o644); err != nil {
		t.Fatalf("write monitors.conf: %v", err)
	}
	if err := os.WriteFile(hyprlandConfigPath, []byte("source = "+monitorsConfPath+"\n"), 0o644); err != nil {
		t.Fatalf("write hyprland.conf: %v", err)
	}
	if err := os.WriteFile(hyprctlPath, []byte(hyprctlScript), 0o755); err != nil {
		t.Fatalf("write fake hyprctl: %v", err)
	}

	t.Setenv("HYPRCTL_LOG", logPath)
	t.Setenv("HYPRLAND_INSTANCE_SIGNATURE", "sig-test")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	client, err := hypr.NewClient()
	if err != nil {
		err = fmt.Errorf("new hypr client: %w", err)
		return
	}

	monitors := []hypr.Monitor{
		{Name: "DP-1", Description: "Microstep MPG321UR-QD", Make: "Microstep", Model: "MPG321UR-QD", Serial: "A1", Width: 3840, Height: 2160, RefreshRate: 143.99, Scale: 1, VRR: 1},
		{Name: "eDP-1", Description: "Samsung Display Corp. ATNA60CL10-0", Make: "Samsung Display Corp.", Model: "ATNA60CL10-0", Serial: "B2", Width: 2880, Height: 1800, RefreshRate: 120, X: 3840, Scale: 1},
	}
	p := profile.New("desk", []profile.OutputConfig{
		{Key: monitors[0].HardwareKey(), Name: monitors[0].Name, Enabled: true, Width: 3840, Height: 2160, Refresh: 143.99, X: 0, Y: 0, Scale: 1, VRR: 1},
		{Key: monitors[1].HardwareKey(), Name: monitors[1].Name, Enabled: true, Width: 2880, Height: 1800, Refresh: 120, X: 3840, Y: 0, Scale: 1},
	})
	p.Workspaces = profile.WorkspaceSettings{
		Enabled:  true,
		Strategy: profile.WorkspaceStrategyManual,
		Rules: []profile.WorkspaceRule{
			{Workspace: "1", OutputKey: monitors[0].HardwareKey(), OutputName: monitors[0].Name, Default: true, Persistent: true},
			{Workspace: "2", OutputKey: monitors[1].HardwareKey(), OutputName: monitors[1].Name, Default: true, Persistent: true},
		},
	}
	logf := func(format string, args ...any) {
		msg := fmt.Sprintf(format, args...)
		file, err := os.OpenFile(logPath, os.O_RDWR, 0o644)
		if err != nil {
			t.Fatalf("open log file: %v", err)
		}
		defer file.Close()

		_, err = file.WriteString(msg)
		if err != nil {
			t.Fatalf("write to log file: %v", err)
		}
	}

	engine = &Engine{
		Client:             client,
		MonitorsConfPath:   monitorsConfPath,
		HyprlandConfigPath: hyprlandConfigPath,
		Logf:               logf,
	}

	return
}

func TestVRRModeUnmarshal(t *testing.T) {
	tests := []struct {
		json string
		want int
	}{
		{`{"vrr": false}`, 0},
		{`{"vrr": true}`, 1},
		{`{"vrr": 0}`, 0},
		{`{"vrr": 1}`, 1},
		{`{"vrr": 2}`, 2},
	}
	for _, tt := range tests {
		var m hypr.Monitor
		if err := json.Unmarshal([]byte(tt.json), &m); err != nil {
			t.Errorf("Unmarshal(%s) error: %v", tt.json, err)
			continue
		}
		if got := int(m.VRR); got != tt.want {
			t.Errorf("Unmarshal(%s).VRR = %d, want %d", tt.json, got, tt.want)
		}
	}
}

func TestMonitorBitdepthParsing(t *testing.T) {
	tests := []struct {
		format string
		want   int
	}{
		{"XBGR2101010", 10},
		{"XRGB2101010", 10},
		{"ABGR2101010", 10},
		{"XBGR16161616F", 16},
		{"XRGB8888", 8},
		{"", 8},
	}
	for _, tt := range tests {
		m := hypr.Monitor{CurrentFormat: tt.format}
		if got := m.Bitdepth(); got != tt.want {
			t.Errorf("Bitdepth(%q) = %d, want %d", tt.format, got, tt.want)
		}
	}
}

func testMonitor(name, desc, make_, model, serial string) hypr.Monitor {
	return hypr.Monitor{Name: name, Description: desc, Make: make_, Model: model, Serial: serial}
}

func testRenderV2(t *testing.T, outputs []profile.OutputConfig, monitors []hypr.Monitor) string {
	t.Helper()
	p := profile.New(t.Name(), outputs)
	rendered, err := RenderHyprlandConfig(p, monitors, true)
	if err != nil {
		t.Fatalf("unexpected render error: %v", err)
	}
	return rendered
}

func TestRenderMonitorV2BlockWithExtraFields(t *testing.T) {
	mon := testMonitor("HDMI-A-1", "LG Electronics LG TV SSCR2 0x01010101", "LG Electronics", "LG TV SSCR2", "0x01010101")
	rendered := testRenderV2(t, []profile.OutputConfig{{
		Key:             mon.HardwareKey(),
		Name:            "HDMI-A-1",
		Enabled:         true,
		Width:           3840,
		Height:          2160,
		Refresh:         143.99,
		Scale:           1.25,
		VRR:             2,
		Bitdepth:        10,
		CM:              "wide",
		SDRBrightness:   1.2,
		SDRSaturation:   0.98,
		SDRMinLuminance: 0.005,
		SDRMaxLuminance: 400,
		MinLuminance:    0,
		MaxLuminance:    800,
	}}, []hypr.Monitor{mon})

	for _, want := range []string{
		"vrr = 2",
		"bitdepth = 10",
		"cm = wide",
		"sdrbrightness = 1.2",
		"sdrsaturation = 0.98",
		"sdr_min_luminance = 0.005",
		"sdr_max_luminance = 400",
		"min_luminance = 0",
		"max_luminance = 800",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered config missing %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "transform =") {
		t.Fatalf("rendered config should not contain default transform:\n%s", rendered)
	}
}

func TestRenderMonitorV2BlockDefaultsOmitted(t *testing.T) {
	mon := testMonitor("DP-1", "Dell U2720Q", "Dell", "U2720Q", "A1")
	rendered := testRenderV2(t, []profile.OutputConfig{{
		Key:           mon.HardwareKey(),
		Name:          "DP-1",
		Enabled:       true,
		Width:         2560,
		Height:        1440,
		Refresh:       60,
		Scale:         1,
		Bitdepth:      8,
		CM:            "srgb",
		SDRBrightness: 1.0,
		SDRSaturation: 1.0,
	}}, []hypr.Monitor{mon})

	for _, unwanted := range []string{
		"bitdepth",
		"cm =",
		"vrr",
		"sdrbrightness",
		"sdrsaturation",
		"sdr_min_luminance",
		"sdr_max_luminance",
		"min_luminance",
		"max_luminance",
		"max_avg_luminance",
		"supports_wide_color",
		"supports_hdr",
		"sdr_eotf",
		"icc",
	} {
		if strings.Contains(rendered, unwanted) {
			t.Fatalf("rendered config should not contain default %q:\n%s", unwanted, rendered)
		}
	}
}

func TestRenderMonitorV2BlockLuminancePairs(t *testing.T) {
	mon := testMonitor("DP-1", "Dell U2720Q", "Dell", "U2720Q", "A1")
	base := profile.OutputConfig{
		Key: mon.HardwareKey(), Name: "DP-1", Enabled: true,
		Width: 3840, Height: 2160, Refresh: 144, Scale: 1.25,
	}

	t.Run("both EDID luminance", func(t *testing.T) {
		out := base
		out.MinLuminance = 0
		out.MaxLuminance = 800
		rendered := testRenderV2(t, []profile.OutputConfig{out}, []hypr.Monitor{mon})
		for _, want := range []string{"min_luminance = 0", "max_luminance = 800"} {
			if !strings.Contains(rendered, want) {
				t.Fatalf("missing %q:\n%s", want, rendered)
			}
		}
	})

	t.Run("both SDR luminance", func(t *testing.T) {
		out := base
		out.SDRMinLuminance = 0.005
		out.SDRMaxLuminance = 400
		rendered := testRenderV2(t, []profile.OutputConfig{out}, []hypr.Monitor{mon})
		for _, want := range []string{"sdr_min_luminance = 0.005", "sdr_max_luminance = 400"} {
			if !strings.Contains(rendered, want) {
				t.Fatalf("missing %q:\n%s", want, rendered)
			}
		}
	})

	t.Run("only SDR min luminance", func(t *testing.T) {
		out := base
		out.SDRMinLuminance = 0.005
		rendered := testRenderV2(t, []profile.OutputConfig{out}, []hypr.Monitor{mon})
		if !strings.Contains(rendered, "sdr_min_luminance = 0.005") {
			t.Fatalf("expected sdr_min_luminance when only min is set:\n%s", rendered)
		}
	})

	t.Run("only EDID min luminance", func(t *testing.T) {
		out := base
		out.MinLuminance = 0.005
		rendered := testRenderV2(t, []profile.OutputConfig{out}, []hypr.Monitor{mon})
		if !strings.Contains(rendered, "min_luminance = 0.005") {
			t.Fatalf("expected min_luminance when only min is set:\n%s", rendered)
		}
	})
}

func TestCommandForOutputV1ExtraFields(t *testing.T) {
	mon := testMonitor("DP-1", "", "Dell", "U2720Q", "A1")
	p := profile.New("v1", []profile.OutputConfig{{
		Key:          mon.HardwareKey(),
		Name:         "DP-1",
		Enabled:      true,
		Width:        2560,
		Height:       1440,
		Refresh:      144,
		Scale:        1,
		VRR:          2,
		Bitdepth:     10,
		CM:           "wide",
		SDREOTF:      "srgb",
		ICC:          "/usr/share/color/icc/test.icc",
		MaxLuminance: 800,
	}})

	cmds, err := CommandsForProfile(p, []hypr.Monitor{mon})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
	for _, want := range []string{"vrr,2", "bitdepth,10", "cm,wide", "sdr_eotf,1", "icc,/usr/share/color/icc/test.icc"} {
		if !strings.Contains(cmds[0], want) {
			t.Fatalf("v1 command missing %q: %s", want, cmds[0])
		}
	}
	for _, unwanted := range []string{"min_luminance", "max_luminance", "sdr_min_luminance", "max_avg_luminance", "supports_wide_color", "supports_hdr"} {
		if strings.Contains(cmds[0], unwanted) {
			t.Fatalf("v1 command should not contain %q: %s", unwanted, cmds[0])
		}
	}
}

func TestFromStateCopiesExtraFields(t *testing.T) {
	monitors := []hypr.Monitor{{
		Name: "HDMI-A-1", Make: "LG", Model: "TV", Serial: "123",
		Width: 3840, Height: 2160, RefreshRate: 144, Scale: 1.25,
		VRR: 1, CurrentFormat: "XBGR2101010",
		ColorManagementPreset: "wide",
		SDRBrightness:         1.2,
		SDRSaturation:         0.98,
		SDRMinLuminance:       0.005,
		SDRMaxLuminance:       400,
	}}
	p := profile.FromMonitors("test", monitors)

	out := p.Outputs[0]
	if out.VRR != 1 {
		t.Errorf("VRR = %d, want 1", out.VRR)
	}
	if out.Bitdepth != 10 {
		t.Errorf("Bitdepth = %d, want 10", out.Bitdepth)
	}
	if out.CM != "wide" {
		t.Errorf("CM = %q, want %q", out.CM, "wide")
	}
	if out.SDRBrightness != 1.2 {
		t.Errorf("SDRBrightness = %f, want 1.2", out.SDRBrightness)
	}
	if out.SDRSaturation != 0.98 {
		t.Errorf("SDRSaturation = %f, want 0.98", out.SDRSaturation)
	}
	if out.SDRMinLuminance != 0.005 {
		t.Errorf("SDRMinLuminance = %f, want 0.005", out.SDRMinLuminance)
	}
	if out.SDRMaxLuminance != 400 {
		t.Errorf("SDRMaxLuminance = %d, want 400", out.SDRMaxLuminance)
	}
}

func TestProfileValidateBitdepth(t *testing.T) {
	valid := []int{0, 8, 10}
	for _, bd := range valid {
		p := profile.New("test", []profile.OutputConfig{{
			Key: "test", Enabled: true, Scale: 1, Bitdepth: bd,
		}})
		if err := p.Validate(); err != nil {
			t.Errorf("bitdepth %d should be valid, got: %v", bd, err)
		}
	}

	// 16 is rejected on purpose. Hyprland's parser only asks whether the value
	// is "10", so a 16 there means 10-bit off and the display quietly runs at 8.
	invalid := []int{4, 12, 16, 24, -1}
	for _, bd := range invalid {
		p := profile.New("test", []profile.OutputConfig{{
			Key: "test", Enabled: true, Scale: 1, Bitdepth: bd,
		}})
		if err := p.Validate(); err == nil {
			t.Errorf("bitdepth %d should be invalid", bd)
		}
	}
}

func TestRenderMonitorV2BlockNewEDIDFields(t *testing.T) {
	mon := testMonitor("DP-1", "Dell U2720Q", "Dell", "U2720Q", "A1")
	rendered := testRenderV2(t, []profile.OutputConfig{{
		Key: mon.HardwareKey(), Name: "DP-1", Enabled: true,
		Width: 2560, Height: 1440, Refresh: 144, Scale: 1,
		SupportsWideColor: -1,
		SupportsHDR:       1,
		MaxAvgLuminance:   350,
		SDREOTF:           "gamma22",
		ICC:               "/usr/share/color/icc/test.icc",
	}}, []hypr.Monitor{mon})

	for _, want := range []string{
		"supports_wide_color = -1",
		"supports_hdr = 1",
		"max_avg_luminance = 350",
		"sdr_eotf = gamma22",
		"icc = /usr/share/color/icc/test.icc",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("missing %q:\n%s", want, rendered)
		}
	}
}

func TestRenderMonitorV2MinLuminanceFloat(t *testing.T) {
	mon := testMonitor("DP-1", "Dell U2720Q", "Dell", "U2720Q", "A1")
	rendered := testRenderV2(t, []profile.OutputConfig{{
		Key: mon.HardwareKey(), Name: "DP-1", Enabled: true,
		Width: 2560, Height: 1440, Refresh: 144, Scale: 1,
		MinLuminance: 0.005,
		MaxLuminance: 800,
	}}, []hypr.Monitor{mon})

	if !strings.Contains(rendered, "min_luminance = 0.005") {
		t.Fatalf("expected float min_luminance:\n%s", rendered)
	}
}

func TestApplyNeverLeavesTheConfigWithoutMonitorRules(t *testing.T) {
	// The upgrade path: the old generated file still holds the live rules, and
	// the new one does not exist yet. Nothing in between may leave the config
	// naming a file that is not there, or with no monitor rules at all.
	engine, generatedPath, _ := initLuaProbeTestEngine(t, "ok", nil)
	hyprDir := filepath.Dir(generatedPath)
	legacyPath := filepath.Join(hyprDir, "monitors.lua")

	legacy := config.GeneratedLuaHeader + "\nhl.monitor({ output = \"desc:Old\", scale = 1 })\n"
	if err := os.WriteFile(legacyPath, []byte(legacy), 0o644); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}

	rootPath := filepath.Join(hyprDir, "hyprland.lua")
	root, err := os.ReadFile(rootPath)
	if err != nil {
		t.Fatalf("read root config: %v", err)
	}
	if strings.Contains(string(root), "hyprmoncfg-monitors") {
		t.Fatal("expected no include before the first apply")
	}
	// An include added before the file exists breaks the next reload, which is
	// what makes a daemon restart flash the wrong layout.
	if _, err := os.Stat(generatedPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected the generated file to be absent, stat error: %v", err)
	}

	if _, err := engine.Apply(context.Background(), newTestProfile(), monitors, ApplyModeNonInteractive); err != nil {
		t.Fatalf("apply: %v", err)
	}

	root, err = os.ReadFile(rootPath)
	if err != nil {
		t.Fatalf("read root config: %v", err)
	}
	if !strings.Contains(string(root), "hyprmoncfg-monitors") {
		t.Fatalf("expected the include after the apply, got:\n%s", root)
	}
	if _, err := os.Stat(generatedPath); err != nil {
		t.Fatalf("expected the generated file to exist: %v", err)
	}

	retired, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatalf("read retired config: %v", err)
	}
	if strings.Contains(string(retired), "desc:Old") {
		t.Fatalf("expected the old rules to be retired once the new file works, got:\n%s", retired)
	}
	if !strings.Contains(string(retired), "no longer writes this file") {
		t.Fatalf("expected a note pointing at the new file, got:\n%s", retired)
	}
}

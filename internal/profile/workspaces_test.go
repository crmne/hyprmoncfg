package profile

import (
	"testing"

	"github.com/crmne/hyprmoncfg/internal/hypr"
)

func TestGeneratedSequentialWorkspaceRules(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "DP-1", Make: "Dell", Model: "U2720Q", Serial: "A1", X: 0},
		{Name: "HDMI-A-1", Make: "LG", Model: "27GP850", Serial: "B2", X: 1000},
	}
	prof := New("desk", []OutputConfig{
		{Key: monitors[0].HardwareKey(), Name: monitors[0].Name, Enabled: true, Scale: 1, Mode: "2560x1440@144.00Hz"},
		{Key: monitors[1].HardwareKey(), Name: monitors[1].Name, Enabled: true, Scale: 1, Mode: "2560x1440@144.00Hz"},
	})
	prof.Workspaces = WorkspaceSettings{
		Enabled:       true,
		Strategy:      WorkspaceStrategySequential,
		MaxWorkspaces: 6,
		GroupSize:     3,
		MonitorOrder:  []string{monitors[0].HardwareKey(), monitors[1].HardwareKey()},
	}

	rules := ResolveWorkspaceRules(prof, monitors)
	if len(rules) != 6 {
		t.Fatalf("expected 6 rules, got %d", len(rules))
	}
	if rules[0].OutputName != "DP-1" || rules[3].OutputName != "HDMI-A-1" {
		t.Fatalf("unexpected sequential assignment: %+v", rules)
	}
	if !rules[0].Default || !rules[3].Default {
		t.Fatalf("expected first workspace per monitor to be default")
	}
}

func TestGeneratedInterleaveWorkspaceRules(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "DP-1", Make: "Dell", Model: "U2720Q", Serial: "A1", X: 0},
		{Name: "HDMI-A-1", Make: "LG", Model: "27GP850", Serial: "B2", X: 1000},
	}
	prof := New("desk", []OutputConfig{
		{Key: monitors[0].HardwareKey(), Name: monitors[0].Name, Enabled: true, Scale: 1, Mode: "2560x1440@144.00Hz"},
		{Key: monitors[1].HardwareKey(), Name: monitors[1].Name, Enabled: true, Scale: 1, Mode: "2560x1440@144.00Hz"},
	})
	prof.Workspaces = WorkspaceSettings{
		Enabled:       true,
		Strategy:      WorkspaceStrategyInterleave,
		MaxWorkspaces: 4,
		GroupSize:     3,
		MonitorOrder:  []string{monitors[0].HardwareKey(), monitors[1].HardwareKey()},
	}

	rules := ResolveWorkspaceRules(prof, monitors)
	if len(rules) != 4 {
		t.Fatalf("expected 4 rules, got %d", len(rules))
	}
	if rules[0].OutputName != "DP-1" || rules[1].OutputName != "HDMI-A-1" || rules[2].OutputName != "DP-1" {
		t.Fatalf("unexpected interleave assignment: %+v", rules)
	}
}

func TestSequentialWorkspaceRulesSkipsMirroredMonitors(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "DP-1", Make: "Dell", Model: "U2720Q", Serial: "A1", X: 0},
		{Name: "HDMI-A-1", Make: "LG", Model: "27GP850", Serial: "B2", X: 1000, MirrorOf: "DP-1"},
	}
	prof := New("mirror-desk", []OutputConfig{
		{Key: monitors[0].HardwareKey(), Name: monitors[0].Name, Enabled: true, Scale: 1, Mode: "2560x1440@144.00Hz"},
		{Key: monitors[1].HardwareKey(), Name: monitors[1].Name, Enabled: true, Scale: 1, Mode: "1920x1080@60.00Hz", MirrorOf: monitors[0].HardwareKey()},
	})
	prof.Workspaces = WorkspaceSettings{
		Enabled:       true,
		Strategy:      WorkspaceStrategySequential,
		MaxWorkspaces: 6,
		GroupSize:     3,
		MonitorOrder:  hypr.MonitorOrder(monitors),
	}

	rules := ResolveWorkspaceRules(prof, monitors)
	for _, rule := range rules {
		if rule.OutputKey == monitors[1].HardwareKey() {
			t.Fatalf("mirrored monitor should not receive workspace rules, got rule for workspace %s", rule.Workspace)
		}
	}
	if len(rules) != 6 {
		t.Fatalf("expected 6 rules, got %d", len(rules))
	}
	if rules[0].OutputName != "DP-1" || rules[5].OutputName != "DP-1" {
		t.Fatalf("all rules should be assigned to the non-mirrored monitor: %+v", rules)
	}
}

func TestMonitorOrderExcludesMirrored(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "DP-1", Make: "Dell", Model: "U2720Q", Serial: "A1", X: 0},
		{Name: "HDMI-A-1", Make: "LG", Model: "27GP850", Serial: "B2", X: 1000, MirrorOf: "DP-1"},
		{Name: "DP-2", Make: "Samsung", Model: "Odyssey", Serial: "C3", X: 2000},
	}
	order := hypr.MonitorOrder(monitors)
	if len(order) != 2 {
		t.Fatalf("expected 2 monitors in order (mirrored excluded), got %d: %v", len(order), order)
	}
	for _, key := range order {
		if key == monitors[1].HardwareKey() {
			t.Fatalf("mirrored monitor should not appear in MonitorOrder")
		}
	}
}

func TestWorkspaceSettingsFromHyprInfersSequentialStrategy(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "DP-1", Make: "Dell", Model: "U2720Q", Serial: "A1", X: 0},
		{Name: "HDMI-A-1", Make: "LG", Model: "27GP850", Serial: "B2", X: 1000},
	}
	prof := New("desk", []OutputConfig{
		{Key: monitors[0].HardwareKey(), Name: monitors[0].Name, Enabled: true, Scale: 1, Mode: "2560x1440@144.00Hz"},
		{Key: monitors[1].HardwareKey(), Name: monitors[1].Name, Enabled: true, Scale: 1, Mode: "2560x1440@144.00Hz"},
	})
	prof.Workspaces = WorkspaceSettings{
		Enabled:       true,
		Strategy:      WorkspaceStrategySequential,
		MaxWorkspaces: 6,
		GroupSize:     3,
		MonitorOrder:  []string{monitors[0].HardwareKey(), monitors[1].HardwareKey()},
	}

	resolved := ResolveWorkspaceRules(prof, monitors)
	hyprRules := make([]hypr.WorkspaceRule, 0, len(resolved))
	for _, rule := range resolved {
		hyprRules = append(hyprRules, hypr.WorkspaceRule{
			WorkspaceString: rule.Workspace,
			Monitor:         rule.OutputName,
			Default:         rule.Default,
			Persistent:      rule.Persistent,
		})
	}

	settings := WorkspaceSettingsFromHypr(monitors, hyprRules)
	if settings.Strategy != WorkspaceStrategySequential {
		t.Fatalf("expected sequential strategy, got %q", settings.Strategy)
	}
	if settings.GroupSize != 3 {
		t.Fatalf("expected group size 3, got %d", settings.GroupSize)
	}
	if settings.MaxWorkspaces != 6 {
		t.Fatalf("expected max workspaces 6, got %d", settings.MaxWorkspaces)
	}
}

func TestWorkspaceSettingsFromHyprInfersInterleaveStrategy(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "DP-1", Make: "Dell", Model: "U2720Q", Serial: "A1", X: 0},
		{Name: "HDMI-A-1", Make: "LG", Model: "27GP850", Serial: "B2", X: 1000},
	}
	prof := New("desk", []OutputConfig{
		{Key: monitors[0].HardwareKey(), Name: monitors[0].Name, Enabled: true, Scale: 1, Mode: "2560x1440@144.00Hz"},
		{Key: monitors[1].HardwareKey(), Name: monitors[1].Name, Enabled: true, Scale: 1, Mode: "2560x1440@144.00Hz"},
	})
	prof.Workspaces = WorkspaceSettings{
		Enabled:       true,
		Strategy:      WorkspaceStrategyInterleave,
		MaxWorkspaces: 6,
		GroupSize:     3,
		MonitorOrder:  []string{monitors[0].HardwareKey(), monitors[1].HardwareKey()},
	}

	resolved := ResolveWorkspaceRules(prof, monitors)
	hyprRules := make([]hypr.WorkspaceRule, 0, len(resolved))
	for _, rule := range resolved {
		hyprRules = append(hyprRules, hypr.WorkspaceRule{
			WorkspaceString: rule.Workspace,
			Monitor:         rule.OutputName,
			Default:         rule.Default,
			Persistent:      rule.Persistent,
		})
	}

	settings := WorkspaceSettingsFromHypr(monitors, hyprRules)
	if settings.Strategy != WorkspaceStrategyInterleave {
		t.Fatalf("expected interleave strategy, got %q", settings.Strategy)
	}
	if settings.MaxWorkspaces != 6 {
		t.Fatalf("expected max workspaces 6, got %d", settings.MaxWorkspaces)
	}
}

func TestWorkspaceSettingsFromHyprPreservesCanonicalMonitorOrder(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "DP-1", Make: "Dell", Model: "U2720Q", Serial: "A1", X: 0},
		{Name: "HDMI-A-1", Make: "LG", Model: "27GP850", Serial: "B2", X: 1000},
	}
	rules := []hypr.WorkspaceRule{
		{WorkspaceString: "1", Monitor: "DP-1", Default: true, Persistent: true},
		{WorkspaceString: "2", Monitor: "DP-1"},
		{WorkspaceString: "3", Monitor: "HDMI-A-1", Default: true, Persistent: true},
		{WorkspaceString: "4", Monitor: "HDMI-A-1"},
	}

	settings := WorkspaceSettingsFromHypr(monitors, rules)
	want := hypr.MonitorOrder(monitors)
	if len(settings.MonitorOrder) != len(want) {
		t.Fatalf("expected monitor order %v, got %v", want, settings.MonitorOrder)
	}
	for i := range want {
		if settings.MonitorOrder[i] != want[i] {
			t.Fatalf("expected monitor order %v, got %v", want, settings.MonitorOrder)
		}
	}
}

func TestResolveWorkspaceRulesFallsBackToManualRuleOrder(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "DP-1", Make: "Dell", Model: "U2720Q", Serial: "A1", X: 1000},
		{Name: "eDP-1", Make: "BOE", Model: "Panel", Serial: "B2", X: 0},
	}
	prof := New("desk", []OutputConfig{
		{Key: monitors[0].HardwareKey(), Name: monitors[0].Name, Enabled: true, Scale: 1, Mode: "2560x1440@144.00Hz"},
		{Key: monitors[1].HardwareKey(), Name: monitors[1].Name, Enabled: true, Scale: 1, Mode: "1920x1200@60.00Hz"},
	})
	prof.Workspaces = WorkspaceSettings{
		Enabled:       true,
		Strategy:      WorkspaceStrategySequential,
		MaxWorkspaces: 6,
		GroupSize:     3,
		Rules: []WorkspaceRule{
			{Workspace: "1", OutputName: "DP-1"},
			{Workspace: "2", OutputName: "DP-1"},
			{Workspace: "3", OutputName: "DP-1"},
			{Workspace: "4", OutputName: "eDP-1"},
			{Workspace: "5", OutputName: "eDP-1"},
			{Workspace: "6", OutputName: "eDP-1"},
		},
	}

	rules := ResolveWorkspaceRules(prof, nil)
	if len(rules) != 6 {
		t.Fatalf("expected 6 generated rules, got %d", len(rules))
	}
	if rules[0].OutputName != "DP-1" || rules[3].OutputName != "eDP-1" {
		t.Fatalf("expected manual-rule order fallback, got %+v", rules)
	}
}

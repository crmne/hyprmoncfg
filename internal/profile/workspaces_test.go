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

func TestGeneratedWorkspaceRulesHaveNoLegacyUICeiling(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "DP-1", Make: "Dell", Model: "U2720Q", Serial: "A1", X: 0},
		{Name: "HDMI-A-1", Make: "LG", Model: "27GP850", Serial: "B2", X: 1000},
	}
	prof := New("many-workspaces", []OutputConfig{
		{Key: monitors[0].HardwareKey(), Name: monitors[0].Name, Enabled: true, Scale: 1, Mode: "2560x1440@144.00Hz"},
		{Key: monitors[1].HardwareKey(), Name: monitors[1].Name, Enabled: true, Scale: 1, Mode: "2560x1440@144.00Hz"},
	})
	prof.Workspaces = WorkspaceSettings{
		Enabled:       true,
		Strategy:      WorkspaceStrategySequential,
		MaxWorkspaces: 64,
		GroupSize:     40,
		MonitorOrder:  []string{monitors[0].HardwareKey(), monitors[1].HardwareKey()},
	}

	rules := ResolveWorkspaceRules(prof, monitors)
	if len(rules) != 64 {
		t.Fatalf("expected all 64 workspace rules, got %d", len(rules))
	}
	if rules[39].OutputName != "DP-1" || rules[40].OutputName != "HDMI-A-1" {
		t.Fatalf("expected group size 40 to be preserved, boundary rules: %+v %+v", rules[39], rules[40])
	}
}

func TestNormalizeAssignsUniqueKeysToDuplicateOutputs(t *testing.T) {
	legacyKey := "vie|c24pulse|0x01010101"
	prof := Profile{
		Name: "desk",
		Outputs: []OutputConfig{
			{Key: legacyKey, Name: "DP-5", Make: "VIE", Model: "C24PULSE", Serial: "0x01010101", Enabled: true, Scale: 1},
			{Key: legacyKey, Name: "DP-6", Make: "VIE", Model: "C24PULSE", Serial: "0x01010101", Enabled: true, Scale: 1},
		},
		Workspaces: WorkspaceSettings{
			Enabled:      true,
			Strategy:     WorkspaceStrategyManual,
			MonitorOrder: []string{legacyKey, legacyKey},
			Rules: []WorkspaceRule{
				{Workspace: "1", OutputKey: legacyKey, OutputName: "DP-5"},
				{Workspace: "2", OutputKey: legacyKey, OutputName: "DP-6"},
			},
		},
	}

	prof.Normalize()

	if prof.Outputs[0].Key == prof.Outputs[1].Key {
		t.Fatalf("expected duplicate outputs to receive distinct keys, got %+v", prof.Outputs)
	}
	if prof.Outputs[0].MatchKey != legacyKey || prof.Outputs[1].MatchKey != legacyKey {
		t.Fatalf("expected duplicate outputs to preserve shared match key, got %+v", prof.Outputs)
	}
	if prof.Workspaces.MonitorOrder[0] == prof.Workspaces.MonitorOrder[1] {
		t.Fatalf("expected monitor order to be rewritten to distinct keys, got %v", prof.Workspaces.MonitorOrder)
	}
	if prof.Workspaces.Rules[0].OutputKey == prof.Workspaces.Rules[1].OutputKey {
		t.Fatalf("expected workspace rules to target distinct outputs, got %+v", prof.Workspaces.Rules)
	}
}

func TestNormalizePreservesStableDuplicateOutputKeys(t *testing.T) {
	prof := Profile{
		Name: "desk",
		Outputs: []OutputConfig{
			{Key: "sceptre tech inc|sceptre z27@mst-3", MatchKey: "sceptre tech inc|sceptre z27", Name: "DP-8", Make: "Sceptre Tech Inc", Model: "Sceptre Z27", Enabled: true, Scale: 1},
			{Key: "sceptre tech inc|sceptre z27@mst-2", MatchKey: "sceptre tech inc|sceptre z27", Name: "DP-5", Make: "Sceptre Tech Inc", Model: "Sceptre Z27", Enabled: true, Scale: 1},
		},
		Workspaces: WorkspaceSettings{
			Enabled:      true,
			Strategy:     WorkspaceStrategySequential,
			MonitorOrder: []string{"sceptre tech inc|sceptre z27@mst-3", "sceptre tech inc|sceptre z27@mst-2"},
		},
	}

	prof.Normalize()

	keys := prof.Keys()
	if keys[0] != "sceptre tech inc|sceptre z27@mst-2" || keys[1] != "sceptre tech inc|sceptre z27@mst-3" {
		t.Fatalf("expected stable keys to survive normalization, got %v", keys)
	}
	if prof.Workspaces.MonitorOrder[0] != "sceptre tech inc|sceptre z27@mst-3" ||
		prof.Workspaces.MonitorOrder[1] != "sceptre tech inc|sceptre z27@mst-2" {
		t.Fatalf("expected stable monitor order to survive normalization, got %v", prof.Workspaces.MonitorOrder)
	}
}

func TestMonitorResolverMatchesStableKeyAfterConnectorRename(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "DP-10", Make: "Sceptre Tech Inc", Model: "Sceptre Z27", ConnectorPath: "mst:532-3"},
		{Name: "DP-6", Make: "Sceptre Tech Inc", Model: "Sceptre Z27", ConnectorPath: "mst:519-2"},
	}
	resolver := NewMonitorResolver(monitors)

	monitor, ok := resolver.ResolveOutput(OutputConfig{
		Key:      "sceptre tech inc|sceptre z27@mst-2",
		MatchKey: "sceptre tech inc|sceptre z27",
		Name:     "DP-5",
	})
	if !ok {
		t.Fatal("expected stable key to resolve after connector rename")
	}
	if monitor.Name != "DP-6" {
		t.Fatalf("expected stable key to resolve to DP-6, got %q", monitor.Name)
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

func TestWorkspaceMonitorOrderSurvivesLiveReadback(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "DP-2", Make: "Dell", Model: "P2725DE", Serial: "desk", X: -1185},
		{Name: "eDP-1", Make: "LG", Model: "Panel", X: 1438},
	}
	for _, strategy := range []WorkspaceStrategy{WorkspaceStrategySequential, WorkspaceStrategyInterleave} {
		t.Run(string(strategy), func(t *testing.T) {
			original := FromState("duals", monitors, nil)
			original.Workspaces = WorkspaceSettings{
				Enabled: true, Strategy: strategy, MaxWorkspaces: 10, GroupSize: 3,
				MonitorOrder: []string{monitors[1].HardwareKey(), monitors[0].HardwareKey()},
			}
			want := ResolveWorkspaceRules(original, monitors)
			liveRules := make([]hypr.WorkspaceRule, 0, len(want))
			for _, rule := range want {
				liveRules = append(liveRules, hypr.WorkspaceRule{
					WorkspaceString: rule.Workspace, Monitor: rule.OutputName,
					Default: rule.Default, Persistent: rule.Persistent,
				})
			}
			// Applying an unsaved order leaves no exact saved profile to restore.
			draft, _, _ := EditorProfileFromState(nil, monitors, liveRules)
			if draft.Workspaces.MonitorOrder[0] != monitors[1].HardwareKey() {
				t.Fatalf("readback replaced selected order: %v", draft.Workspaces.MonitorOrder)
			}
			got := ResolveWorkspaceRules(draft, monitors)
			if len(got) != len(want) {
				t.Fatalf("got %d rules, want %d", len(got), len(want))
			}
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("workspace %s changed after readback: got %+v, want %+v", want[i].Workspace, got[i], want[i])
				}
			}
		})
	}
}

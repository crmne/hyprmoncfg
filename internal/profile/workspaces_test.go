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

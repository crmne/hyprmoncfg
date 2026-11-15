package profile

import (
	"testing"

	"github.com/crmne/hyprmoncfg/internal/hypr"
)

// staleConnectorProfile is a manual plan saved while the ultrawide was on
// DP-2. The displays have since swapped connectors, so every rule still says
// DP-2 although its output key names the display that is now on DP-1.
func staleConnectorProfile() (Profile, hypr.Monitor, hypr.Monitor) {
	ultrawide := hypr.Monitor{Name: "DP-1", Make: "LG Electronics", Model: "LG ULTRAWIDE", Serial: "605PMXY023762"}
	side := hypr.Monitor{Name: "DP-2", Make: "Acer Technologies", Model: "VG240Y P", Serial: "0x94319E1D"}
	prof := New("2 Monitor Setup", []OutputConfig{
		{Key: ultrawide.HardwareKey(), Name: ultrawide.Name, Make: ultrawide.Make, Model: ultrawide.Model, Serial: ultrawide.Serial, Enabled: true, Scale: 1, Mode: "3440x1440@160.00Hz"},
		{Key: side.HardwareKey(), Name: side.Name, Make: side.Make, Model: side.Model, Serial: side.Serial, Enabled: true, Scale: 1, Mode: "1920x1080@144.00Hz"},
	})
	prof.Workspaces = WorkspaceSettings{
		Enabled:  true,
		Strategy: WorkspaceStrategyManual,
		Rules: []WorkspaceRule{
			{Workspace: "1", OutputKey: ultrawide.HardwareKey(), OutputName: "DP-2", Default: true},
			{Workspace: "2", OutputKey: ultrawide.HardwareKey(), OutputName: "DP-2"},
			{Workspace: "8", OutputKey: side.HardwareKey(), OutputName: "DP-2", Default: true},
		},
	}
	return prof, ultrawide, side
}

func TestManualRulesFollowTheirOutputKeyNotAStaleConnector(t *testing.T) {
	prof, ultrawide, side := staleConnectorProfile()

	for _, rule := range ResolveWorkspaceRules(prof, nil) {
		want := ultrawide.Name
		if rule.OutputKey == side.HardwareKey() {
			want = side.Name
		}
		if rule.OutputName != want {
			t.Fatalf("workspace %s: OutputName = %q, want %q (its output key's connector)", rule.Workspace, rule.OutputName, want)
		}
	}
}

func TestNormalizeRefreshesAStaleRuleConnector(t *testing.T) {
	prof, ultrawide, _ := staleConnectorProfile()
	prof.Normalize()
	if got := prof.Workspaces.Rules[0].OutputName; got != ultrawide.Name {
		t.Fatalf("saved rule keeps stale connector %q, want %q", got, ultrawide.Name)
	}
}

func TestARuleWithAnUnknownKeyKeepsItsConnector(t *testing.T) {
	prof, _, _ := staleConnectorProfile()
	prof.Workspaces.Rules = []WorkspaceRule{{Workspace: "9", OutputKey: "gone|display|1", OutputName: "HDMI-A-1"}}
	rules := ResolveWorkspaceRules(prof, nil)
	if len(rules) != 1 || rules[0].OutputName != "HDMI-A-1" {
		t.Fatalf("a rule for a display the profile lacks keeps its connector as the fallback: %+v", rules)
	}
}

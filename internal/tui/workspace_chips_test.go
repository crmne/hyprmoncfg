package tui

import (
	"reflect"
	"testing"

	"github.com/crmne/hyprmoncfg/internal/profile"
)

// A manual plan saved before two displays swapped connectors still records
// the old connector on every rule. The cards must follow each rule's output
// key, as applying the profile does, or every chip lands on one card.
func TestWorkspaceChipsFollowTheOutputKeyAfterAConnectorSwap(t *testing.T) {
	p := profile.New("2 Monitor Setup", []profile.OutputConfig{
		{Key: "lg|ultrawide|1", Name: "DP-1", Enabled: true, Width: 3440, Height: 1440, Refresh: 160, Scale: 1},
		{Key: "acer|vg240y|2", Name: "DP-2", Enabled: true, Width: 1920, Height: 1080, Refresh: 144, Scale: 1},
	})
	p.Workspaces = profile.WorkspaceSettings{
		Enabled:  true,
		Strategy: profile.WorkspaceStrategyManual,
		Rules: []profile.WorkspaceRule{
			{Workspace: "1", OutputKey: "lg|ultrawide|1", OutputName: "DP-2"},
			{Workspace: "2", OutputKey: "lg|ultrawide|1", OutputName: "DP-2"},
			{Workspace: "8", OutputKey: "acer|vg240y|2", OutputName: "DP-2"},
		},
	}

	plan := workspacePlanByConnector(profile.ResolveWorkspaceRules(p, nil))
	if want := []string{"1", "2"}; !reflect.DeepEqual(plan["DP-1"], want) {
		t.Fatalf("DP-1 chips = %v, want %v", plan["DP-1"], want)
	}
	if want := []string{"8"}; !reflect.DeepEqual(plan["DP-2"], want) {
		t.Fatalf("DP-2 chips = %v, want %v", plan["DP-2"], want)
	}
}

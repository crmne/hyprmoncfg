package appstatus

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

func TestLiveWorkspacePlanUsesActualRulesAndStableIdentity(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "DP-2", Description: "Desk A", Make: "Example", Model: "Desk", Serial: "A", Width: 1920, Height: 1080, Scale: 1},
		{Name: "DP-1", Description: "Desk B", Make: "Example", Model: "Desk", Serial: "B", Width: 1920, Height: 1080, Scale: 1, X: 1920},
	}
	rules := []hypr.WorkspaceRule{
		{WorkspaceString: "4", Monitor: "desc:Desk A"},
		{WorkspaceString: "2", Monitor: "DP-1"},
		{WorkspaceString: "6", Monitor: "DP-1"},
		{WorkspaceString: "special:scratchpad"},
		{WorkspaceString: "9", Monitor: "disconnected"},
	}
	doc := Build("test", true, nil, monitors, rules, profile.MatchOptions{})
	want := []WorkspacePlan{
		{OutputKey: monitors[0].HardwareKey(), OutputName: "DP-2", Workspaces: []string{"4"}},
		{OutputKey: monitors[1].HardwareKey(), OutputName: "DP-1", Workspaces: []string{"2", "6"}},
	}
	if !reflect.DeepEqual(doc.WorkspacePlan, want) {
		t.Fatalf("live plan = %#v, want %#v", doc.WorkspacePlan, want)
	}
	// A recommended saved plan must not replace the live assignments.
	saved := profile.FromState("Old plan", monitors, []hypr.WorkspaceRule{{WorkspaceString: "1", Monitor: "DP-2"}})
	withSaved := Build("test", true, []profile.Profile{saved}, monitors, rules, profile.MatchOptions{})
	if !reflect.DeepEqual(withSaved.WorkspacePlan, want) {
		t.Fatal("saved profile changed the reported live plan")
	}
}

func TestLiveWorkspacePlanOmitsNonIndependentOutputs(t *testing.T) {
	base := hypr.Monitor{Name: "DP-1", Width: 1920, Height: 1080}
	for _, kind := range []string{"disabled", "mirror", "modeless", "fallback"} {
		m := base
		switch kind {
		case "disabled":
			m.Disabled = true
		case "mirror":
			m.MirrorOf = "DP-2"
		case "modeless":
			m.Width = 0
		case "fallback":
			m.Name = "FALLBACK"
		}
		if got := LiveWorkspacePlan([]hypr.Monitor{m}, []hypr.WorkspaceRule{{WorkspaceString: "1", Monitor: m.Name}}); len(got) != 0 {
			t.Fatalf("%s acquired an independent workspace group: %+v", kind, got)
		}
	}
	encoded, err := json.Marshal(Build("test", true, nil, nil, nil, profile.MatchOptions{}))
	if err != nil || !strings.Contains(string(encoded), `"workspace_plan":[]`) {
		t.Fatalf("empty live plan must be distinguishable from older daemons: %s, %v", encoded, err)
	}
}

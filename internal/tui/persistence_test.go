package tui

import (
	"github.com/crmne/hyprmoncfg/internal/profile"
	"testing"
)

func TestWorkspacePersistenceEditor(t *testing.T) {
	m := Model{workspaceEdit: workspaceEditor{Enabled: true, Strategy: profile.WorkspaceStrategySequential, MaxWorkspaces: 4, GroupSize: 2, SelectedField: 3}}
	if m.workspaceFieldValue(3) != "First per display" {
		t.Fatal("legacy default changed")
	}
	m.adjustWorkspaceField(1)
	if m.workspaceFieldValue(3) != "All assigned" {
		t.Fatal("persistence action failed")
	}
	w := workspaceEditorFromSettings(m.workspaceEdit.settings(), nil)
	if !w.PersistAll {
		t.Fatal("editor roundtrip lost policy")
	}
	m.workspaceEdit.Strategy = profile.WorkspaceStrategyManual
	m.adjustWorkspaceField(1)
	if !m.workspaceEdit.PersistAll || m.workspaceFieldValue(3) != "Custom (per rule)" {
		t.Fatal("manual policy changed")
	}
	rules := normalizeManualWorkspaceDefaults([]profile.WorkspaceRule{{Workspace: "1", OutputKey: "a", Persistent: true}, {Workspace: "2", OutputKey: "a", Persistent: true}})
	if !rules[1].Persistent {
		t.Fatal("manual conversion lost persistence")
	}
}

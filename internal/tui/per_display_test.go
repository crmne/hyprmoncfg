package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

func TestPerDisplayFixesTheCountAndKeepsTheOrder(t *testing.T) {
	m := Model{workspaceEdit: workspaceEditor{
		Enabled:       true,
		Strategy:      profile.WorkspaceStrategyPerDisplay,
		MaxWorkspaces: 6,
		MonitorOrder:  []string{"mon-a", "mon-b"},
	}}
	if got := m.workspaceFieldValue(1); got != "10 per display" {
		t.Fatalf("Max workspaces = %q", got)
	}
	m.workspaceEdit.SelectedField = 1
	m.adjustWorkspaceField(1)
	updated, _ := m.updateWorkspaceKeys(tea.KeyMsg{Type: tea.KeyEnter})
	if updated.(Model).mode == modeNumericInput || m.workspaceEdit.MaxWorkspaces != 6 {
		t.Fatal("per-display workspace count must not be editable")
	}
	if got := m.workspaceFieldValue(3); got != "First per display" {
		t.Fatalf("Persistence = %q", got)
	}
	m.workspaceEdit.SelectedOrder = 0
	m.moveWorkspaceOrder(1)
	if m.workspaceEdit.MonitorOrder[0] != "mon-b" {
		t.Fatal("hyprmoncfg's own per-display blocks follow the monitor order")
	}
}

// With Omarchy running per-display workspaces, it picks blocks and which
// workspaces stay open, so those rows are inert.
func TestPerDisplayRowsDeferToOmarchy(t *testing.T) {
	profile.SetPerDisplayDelegate("omarchy", func([]profile.OutputConfig) map[string]int { return nil })
	t.Cleanup(func() { profile.SetPerDisplayDelegate("", nil) })

	m := Model{workspaceEdit: workspaceEditor{
		Enabled:      true,
		Strategy:     profile.WorkspaceStrategyPerDisplay,
		MonitorOrder: []string{"mon-a", "mon-b"},
	}}
	if got := m.workspaceFieldValue(3); got != "Set by Omarchy" {
		t.Fatalf("Persistence = %q", got)
	}
	m.workspaceEdit.SelectedField = 3
	m.adjustWorkspaceField(1)
	if m.workspaceEdit.PersistAll {
		t.Fatal("persistence changed under Omarchy")
	}
	m.workspaceEdit.SelectedOrder = 0
	m.moveWorkspaceOrder(1)
	if m.workspaceEdit.MonitorOrder[0] != "mon-a" {
		t.Fatal("monitor order changed under Omarchy")
	}
}

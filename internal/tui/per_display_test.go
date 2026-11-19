package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

func TestPerDisplayCountIsAdjustableFromOneToTenAndKeepsTheOrder(t *testing.T) {
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
	if got := m.workspaceFieldValue(1); got != "10 per display" {
		t.Fatalf("stepped past ten: %q", got)
	}
	m.adjustWorkspaceField(-1)
	if got := m.workspaceEdit.settings(); got.SlotsPerDisplay != 9 || got.MaxWorkspaces != 6 {
		t.Fatalf("settings = %+v, want 9 per display and max workspaces untouched", got)
	}
	for range 20 {
		m.adjustWorkspaceField(-1)
	}
	if got := m.workspaceFieldValue(1); got != "1 per display" {
		t.Fatalf("stepped below one: %q", got)
	}

	updated, _ := m.updateWorkspaceKeys(tea.KeyMsg{Type: tea.KeyEnter})
	typed := updated.(Model)
	if typed.mode != modeNumericInput {
		t.Fatal("Enter types the count")
	}
	typed.input.Input.SetValue("11")
	if typed.commitWorkspaceNumericInput(); typed.input.Input.Err == nil || typed.workspaceEdit.SlotsPerDisplay != 1 {
		t.Fatal("eleven per display accepted")
	}
	typed.input.Input.Err = nil
	typed.input.Input.SetValue("5")
	typed.commitWorkspaceNumericInput()
	if typed.workspaceEdit.SlotsPerDisplay != 5 {
		t.Fatalf("typed count = %d, want 5", typed.workspaceEdit.SlotsPerDisplay)
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

func TestPerDisplayPlanShowsSlotsNotBlockNumbers(t *testing.T) {
	settings := profile.WorkspaceSettings{Enabled: true, Strategy: profile.WorkspaceStrategyPerDisplay}
	rules := []profile.WorkspaceRule{{Workspace: "1"}, {Workspace: "10"}, {Workspace: "11"}, {Workspace: "20"}, {Workspace: "name:web"}}
	got := slotLabeledRules(settings, rules)
	for i, want := range []string{"1", "10", "1", "10", "name:web"} {
		if got[i].Workspace != want {
			t.Fatalf("label %d = %q, want %q", i, got[i].Workspace, want)
		}
	}
	if rules[2].Workspace != "11" {
		t.Fatal("labelling changed the plan itself")
	}
	settings.Strategy = profile.WorkspaceStrategySequential
	if got := slotLabeledRules(settings, rules); got[2].Workspace != "11" {
		t.Fatal("only per-display plans are relabelled")
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

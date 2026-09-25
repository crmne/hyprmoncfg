package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

// Off is a Strategy choice that clears Enabled and keeps the saved plan, so a
// user whose workspaces are managed elsewhere (#70) turns rules off in one place.
func TestWorkspaceStrategyOffKeepsThePlan(t *testing.T) {
	m := Model{
		editOutputs: []editableOutput{
			{Key: "mon-a", Name: "DP-1", Enabled: true, Scale: 1},
			{Key: "mon-b", Name: "HDMI-A-1", Enabled: true, Scale: 1},
		},
		workspaceEdit: workspaceEditorFromSettings(profile.WorkspaceSettings{
			Enabled:       true,
			Strategy:      profile.WorkspaceStrategyManual,
			MaxWorkspaces: 2,
			Rules: []profile.WorkspaceRule{
				{Workspace: "1", OutputKey: "mon-a"},
				{Workspace: "2", OutputKey: "mon-b"},
			},
		}, nil),
	}
	if got := m.workspaceFieldValue(0); got != "Manual" {
		t.Fatalf("Strategy = %q, want Manual", got)
	}

	m.adjustWorkspaceField(-1)
	if m.workspaceFieldValue(0) != "Off" || m.workspaceEdit.Enabled {
		t.Fatalf("moving left from Manual should select Off, got %q enabled=%v", m.workspaceFieldValue(0), m.workspaceEdit.Enabled)
	}
	settings := m.workspaceEdit.settings()
	if settings.Enabled || settings.Strategy != profile.WorkspaceStrategyManual || settings.MaxWorkspaces != 2 || len(settings.Rules) != 2 {
		t.Fatalf("Off must keep the saved plan and only clear Enabled, got %+v", settings)
	}

	m.adjustWorkspaceField(1)
	if !m.workspaceEdit.Enabled || m.workspaceEdit.Strategy != profile.WorkspaceStrategyManual || len(m.workspaceEdit.Rules) != 2 {
		t.Fatalf("leaving Off should turn Manual back on with its rules, got %+v", m.workspaceEdit)
	}
}

func TestWorkspaceStrategyCycleOrder(t *testing.T) {
	m := Model{workspaceEdit: workspaceEditor{Enabled: true, Strategy: profile.WorkspaceStrategyInterleave, ManualRulesInitialized: true}}
	var seen []string
	for range workspaceStrategyChoices {
		m.adjustWorkspaceField(1)
		seen = append(seen, m.workspaceFieldValue(0))
	}
	if got := strings.Join(seen, ","); got != "Off,Manual,Sequential,Interleaved" {
		t.Fatalf("strategy cycle = %s", got)
	}
}

func TestSavedDisabledPlannerLoadsAsOff(t *testing.T) {
	w := workspaceEditorFromSettings(profile.WorkspaceSettings{
		Enabled:       false,
		Strategy:      profile.WorkspaceStrategyInterleave,
		MaxWorkspaces: 6,
		MonitorOrder:  []string{"mon-a"},
	}, nil)
	m := Model{workspaceEdit: w}
	if got := m.workspaceFieldValue(0); got != "Off" {
		t.Fatalf("enabled:false should load as Off, got %q", got)
	}
	if settings := m.workspaceEdit.settings(); settings.Strategy != profile.WorkspaceStrategyInterleave || settings.Enabled {
		t.Fatalf("loading must not migrate the stored strategy, got %+v", settings)
	}
}

func TestWorkspaceRowsAreInertWhileOff(t *testing.T) {
	m := Model{workspaceEdit: workspaceEditor{
		Strategy:      profile.WorkspaceStrategySequential,
		MaxWorkspaces: 6,
		GroupSize:     3,
		MonitorOrder:  []string{"mon-a", "mon-b"},
	}}
	for field := 1; field < len(workspaceFields); field++ {
		if got := m.workspaceFieldValue(field); got != "—" {
			t.Fatalf("%s shows %q while Off", workspaceFields[field], got)
		}
		m.workspaceEdit.SelectedField = field
		m.adjustWorkspaceField(1)
		updated, _ := m.updateWorkspaceKeys(tea.KeyMsg{Type: tea.KeyEnter})
		if updated.(Model).mode == modeNumericInput {
			t.Fatalf("Enter on %s opened input while Off", workspaceFields[field])
		}
	}
	if m.workspaceEdit.MaxWorkspaces != 6 || m.workspaceEdit.GroupSize != 3 || m.workspaceEdit.PersistAll {
		t.Fatalf("Off rows changed the saved plan: %+v", m.workspaceEdit)
	}
	if m.workspaceItemCount() != len(workspaceFields) {
		t.Fatalf("Off should hide the monitor order list, got %d items", m.workspaceItemCount())
	}
	if footer := (Model{tab: tabWorkspaces, workspaceEdit: m.workspaceEdit}).footerHelpText(); strings.Contains(footer, "type count") {
		t.Fatalf("Off footer advertises count entry: %s", footer)
	}
}

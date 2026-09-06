package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestColorPickerLabelsKeepHyprlandValues(t *testing.T) {
	m := Model{styles: newStyles(), width: 120, editOutputs: []editableOutput{{Name: "DP-1", Enabled: true, CM: "wide"}}}
	m.openFieldPicker(layoutFields[4], 4, []string{"srgb", "wide", "hdr", "hdredid"})
	selected := m.picker.List.SelectedItem().(fieldPickerItem)
	if selected.Value() != "wide" || selected.Title() != "BT.2020 (SDR)" {
		t.Fatalf("selected item = %#v", selected)
	}
	m.picker.List.Select(2)
	m.commitModePicker()
	if got := m.editOutputs[0].CM; got != "hdr" {
		t.Fatalf("saved color preset = %q, want Hyprland's hdr", got)
	}
}

func TestColorInspectorLabelsFitTerminal(t *testing.T) {
	m := Model{styles: newStyles(), inspectorTab: inspectorTabColor, editOutputs: []editableOutput{{Name: "DP-1", Enabled: true, CM: "hdr"}}}
	for _, width := range []int{32, 48, 80} {
		view := m.renderInspectorPane(width, 50, false)
		for _, line := range strings.Split(view, "\n") {
			if got := lipgloss.Width(line); got > width {
				t.Errorf("inspector width %d: line width %d: %q", width, got, line)
			}
		}
		if width == 80 && !strings.Contains(view, "SDR luminance scale") {
			t.Errorf("wide inspector omitted full color label:\n%s", view)
		}
	}
}

func TestColorDefaultsDisplayAndAdjustNeutralMultipliers(t *testing.T) {
	m := Model{styles: newStyles(), editOutputs: []editableOutput{{Name: "DP-1", Enabled: true}}}
	for _, field := range []int{10, 11} {
		if got := m.layoutFieldValue(m.editOutputs[0], field); got != "1.00" {
			t.Errorf("default field %d = %q, want 1.00", field, got)
		}
		m.inspectorField = field
		m.adjustInspectorField(1)
		if got := m.layoutFieldValue(m.editOutputs[0], field); got != "1.05" {
			t.Errorf("adjusted field %d = %q, want 1.05", field, got)
		}
	}
}

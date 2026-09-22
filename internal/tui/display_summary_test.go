package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

func TestDisplaySummaryMatchesPanelVocabulary(t *testing.T) {
	o := editableOutput{Name: "DP-2", Make: "Microstep", Model: "Panel", Width: 3840, Height: 2160,
		Refresh: 143.99, Scale: 1.333333, X: -2880, Y: 20, PhysicalWidth: 708, PhysicalHeight: 399}
	if got := o.modelSizeLabel(); got != "Microstep Panel 32\"" {
		t.Fatal(got)
	}
	if got := displayModeLabel(o.DisplayMode()); got != "3840x2160@144Hz" {
		t.Fatal(got)
	}
	if got := o.placementLabel(); got != "Scale 1.33x  Position -2880,20" {
		t.Fatal(got)
	}
	if got := o.maximumResolutionLabel(); got != "Not reported" {
		t.Fatal("active mode is not hardware maximum", got)
	}
	o.HardwareModes = []string{"bad", "1920x1080@240Hz", "3840x2160@60Hz"}
	if got := o.maximumResolutionLabel(); got != "3840x2160" {
		t.Fatal(got)
	}
	if o.Refresh != 143.99 || o.Scale != 1.333333 {
		t.Fatal("display formatting mutated settings")
	}
	if got := displayModeLabel("preferred"); got != "preferred" {
		t.Fatal(got)
	}
}

func TestHardwareSummaryAndDetailsAreHardwareOnly(t *testing.T) {
	m := paneTestModel(t, tabLayout, []hypr.Monitor{paneTestDesk}, nil)
	summary := ansi.Strip(strings.Join(m.inspectorDetailLines(m.editOutputs[0]), "\n"))
	requireContains(t, summary, "Connector", "Model", "Max resolution", "Panel size", "Type", "Serial")
	if strings.Contains(summary, "[i ") {
		t.Fatal("shortcut hint leaked into action label")
	}
	for _, forbidden := range []string{"Layout px", "DPMS", "Workspace", "More details"} {
		if strings.Contains(summary, forbidden) {
			t.Fatal(summary)
		}
	}
}

func TestPostApplyCommandIsLastAndClickable(t *testing.T) {
	p := profile.FromState("Desk", []hypr.Monitor{paneTestDesk}, nil)
	m := paneTestModel(t, tabProfiles, []hypr.Monitor{paneTestDesk}, []profile.Profile{p})
	rows := m.profileDetailRows(p, m.profileMatchSummaries()[0], 70)
	if !strings.Contains(ansi.Strip(rows[len(rows)-2].value), "Post-apply command") {
		t.Fatal(rows)
	}
	x, y := findVisiblePosition(t, m.View(), "[Edit command]")
	if strings.Contains(ansi.Strip(m.View()), "(e)") {
		t.Fatal("shortcut hint leaked into action label")
	}
	next, _ := m.updateMouse(mousePressAt(x, y))
	if next.(Model).mode != modeProfileExecInput {
		t.Fatal("command click did not open")
	}
}

func TestHardwareSummaryFitsReviewTerminalSizes(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {113, 33}} {
		m := paneTestModel(t, tabLayout, []hypr.Monitor{paneTestDesk}, nil)
		m.width, m.height = size[0], size[1]
		m.editOutputs[0].Make = "Microstep"
		m.editOutputs[0].Model = "MPG321UR-QD"
		m.editOutputs[0].PhysicalWidth = 710
		m.editOutputs[0].PhysicalHeight = 400
		view := ansi.Strip(m.View())
		requireContains(t, view, "Max resolution", "Panel size", "Type", "Serial")
		if lines := strings.Count(view, "\n") + 1; lines > m.height {
			t.Fatalf("%dx%d view overflowed to %d lines", m.width, m.height, lines)
		}
	}
}

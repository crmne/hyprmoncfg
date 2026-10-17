package tui

import (
	"reflect"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/crmne/hyprmoncfg/internal/hypr"
)

var modeTestModes = []string{
	"3840x2160@60.00Hz", "2560x1440@59.95Hz", "3840x2160@144.00Hz", "1920x1080@60.00Hz",
	"2560x1440@144.00Hz", "3840x2160@120.00Hz", "3840x2160@144.00Hz", "1080x1920@60.00Hz",
}

func modeTestModel(t *testing.T, width, height int) Model {
	t.Helper()
	desk := paneTestMonitor("DP-1", "Example", "Studio", 3840, 2160, 0, 0)
	desk.RefreshRate = 144
	desk.AvailableModes = modeTestModes
	m := paneTestModel(t, tabLayout, []hypr.Monitor{desk}, nil)
	m.width, m.height = width, height
	m.layoutFocus = layoutFocusInspector
	m.inspectorField = 1
	return m
}

func TestResolutionsAreListedOnceBestFirst(t *testing.T) {
	got := resolutionChoices(append([]string{"preferred"}, modeTestModes...))
	want := []string{"3840x2160", "2560x1440", "1920x1080", "1080x1920", "preferred"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("resolutions %v, want %v", got, want)
	}
}

func TestRefreshRowListsEachRateOfTheResolutionSlowestFirst(t *testing.T) {
	m := modeTestModel(t, 158, 37)
	indexes, labels := m.editOutputs[0].refreshRow()
	if !reflect.DeepEqual(labels, []string{"60Hz", "120Hz", "144Hz"}) || len(indexes) != 3 {
		t.Fatalf("refresh row %v %v", indexes, labels)
	}
	if got := refreshLabels([]float64{59.94, 60, 59.95}); got[0] == got[2] {
		t.Fatalf("rates that round alike need more precision, got %v", got)
	}
}

func TestResolutionChangeKeepsTheRateOrTakesTheHighest(t *testing.T) {
	m := modeTestModel(t, 158, 37)
	output := &m.editOutputs[0]
	if output.Refresh != 144 {
		t.Fatalf("starts at %v", output.Refresh)
	}
	output.selectResolution("2560x1440")
	if got := output.DisplayMode(); got != "2560x1440@144.00Hz" {
		t.Fatalf("144Hz exists at 2560x1440, got %s", got)
	}
	output.selectResolution("1920x1080")
	if got := output.DisplayMode(); got != "1920x1080@60.00Hz" {
		t.Fatalf("only 60Hz at 1920x1080, got %s", got)
	}
	output.selectResolution("2560x1440")
	if got := output.DisplayMode(); got != "2560x1440@144.00Hz" {
		t.Fatalf("60Hz is not offered at 2560x1440 (59.95 is), so the highest wins, got %s", got)
	}
	// What is stored stays the advertised mode string, as before the split.
	if got := output.profileOutput().Mode; got != "2560x1440@144.00Hz" {
		t.Fatalf("stored mode %q", got)
	}
}

func TestResolutionRowWrapsAndPickerSelects(t *testing.T) {
	m := modeTestModel(t, 158, 37)
	view := ansi.Strip(m.View())
	requireContains(t, view, "Resolution", "3840x2160", "Refresh", " 60Hz ", " 120Hz ", " 144Hz ")

	next, _ := m.updateLayoutKeys(tea.KeyMsg{Type: tea.KeyLeft})
	m = mustModel(t, next)
	if got := resolutionKey(m.editOutputs[0].DisplayMode()); got != "1080x1920" {
		t.Fatalf("left from the best resolution wraps to the last, got %s", got)
	}

	m = modeTestModel(t, 158, 37)
	next, _ = m.updateLayoutKeys(tea.KeyMsg{Type: tea.KeyEnter})
	m = mustModel(t, next)
	if m.mode != modeModePicker || m.picker.FieldIndex >= 0 {
		t.Fatal("Enter on Resolution opens the resolution list")
	}
	if items := m.picker.List.Items(); len(items) != 4 {
		t.Fatalf("one item per resolution, got %d", len(items))
	}
	requireContains(t, ansi.Strip(m.View()), "Select Resolution")
	m.picker.List.Select(1)
	m.commitModePicker()
	if got := m.editOutputs[0].DisplayMode(); got != "2560x1440@144.00Hz" {
		t.Fatalf("picking 2560x1440 set %s", got)
	}
}

func TestRefreshRowClampsArrowsWrapsEnterAndTakesClicks(t *testing.T) {
	m := modeTestModel(t, 158, 37)
	m.inspectorField = fieldRefresh
	next, _ := m.updateLayoutKeys(tea.KeyMsg{Type: tea.KeyRight})
	m = mustModel(t, next)
	if m.editOutputs[0].Refresh != 144 {
		t.Fatal("right on the fastest rate stays")
	}
	next, _ = m.updateLayoutKeys(tea.KeyMsg{Type: tea.KeyEnter})
	m = mustModel(t, next)
	if m.editOutputs[0].Refresh != 60 {
		t.Fatalf("Enter on the last rate wraps to the first, got %v", m.editOutputs[0].Refresh)
	}
	next, _ = m.updateLayoutKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	m = mustModel(t, next)
	if m.editOutputs[0].Refresh != 60 {
		t.Fatal("h on the slowest rate stays")
	}
	if got := m.editOutputs[0].Width; got != 3840 {
		t.Fatalf("Refresh never changes the resolution, width %d", got)
	}

	inspector, _ := m.layoutInspectorRect()
	x, y := findVisiblePositionBelow(t, m.View(), " 120Hz ", inspector.y)
	next, _ = m.updateMouse(mousePressAt(x+1, y))
	if got := mustModel(t, next).editOutputs[0].Refresh; got != 120 {
		t.Fatalf("clicking 120Hz set %v", got)
	}
}

func TestModeControlsFitSmallTerminals(t *testing.T) {
	m := modeTestModel(t, 80, 24)
	view := m.View()
	requireContains(t, ansi.Strip(view), "Resolution", "Refresh", "144Hz")
	if h := lipgloss.Height(view); h > 24 {
		t.Fatalf("height %d", h)
	}
	if w := maxRenderedLineWidth(view); w > 80 {
		t.Fatalf("width %d", w)
	}
}

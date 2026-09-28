package tui

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/scaling"
)

func scalePillModel(t *testing.T, width, height int, scale float64) Model {
	t.Helper()
	m := paneTestModel(t, tabLayout, []hypr.Monitor{paneTestDesk}, nil)
	m.width, m.height = width, height
	m.editOutputs[0].Scale = scale
	m.layoutFocus = layoutFocusInspector
	m.inspectorField = 2
	return m
}

// The Scale row shows Omarchy's presets mapped onto the display, then More….
func TestScaleRowShowsThePresetPills(t *testing.T) {
	m := scalePillModel(t, 158, 37, 1.5)
	view := ansi.Strip(m.View())
	// Wide inspectors may wrap the row; the pills keep their order.
	lines, _ := m.renderScaleRow(m.editOutputs[0], 200, false, true)
	if got := ansi.Strip(lines[0]); got != " 1x   1.25x   1.5x   1.6x   2x   3x   More… " {
		t.Fatalf("preset row = %q", got)
	}
	requireContains(t, view, " 1x ", " 1.25x ", " 1.5x ", " 1.6x ", " 2x ", " 3x ", " More… ")
	if strings.Contains(view, "1.07x") {
		t.Fatal("the row shows presets, not the full sharp list")
	}
}

// A current scale outside the presets is its own selected pill, exactly as
// saved; an unsharp one is marked and never rewritten.
func TestCurrentScaleOutsideThePresetsIsItsOwnPill(t *testing.T) {
	m := scalePillModel(t, 158, 37, 1.33333)
	lines, _ := m.renderScaleRow(m.editOutputs[0], 200, false, true)
	requireContains(t, ansi.Strip(lines[0]), " 1.25x   1.33x   1.5x ")

	m = scalePillModel(t, 158, 37, 1.33)
	view := ansi.Strip(m.View())
	requireContains(t, view, " 1.33x ⚠ ", "⚠ fractional px")
	if m.editOutputs[0].Scale != 1.33 {
		t.Fatal("rendering rewrote the scale")
	}
}

// Arrows walk the full shared sharp list and stop at its ends, like
// Omarchy's pills; the row then shows the stepped value as its own pill.
func TestScaleArrowsWalkTheFullSharpList(t *testing.T) {
	m := scalePillModel(t, 158, 37, 1)
	walked := []float64{m.editOutputs[0].Scale}
	for i := 0; i < 30; i++ {
		next, _ := m.updateLayoutKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
		m = mustModel(t, next)
		if s := m.editOutputs[0].Scale; s != walked[len(walked)-1] {
			walked = append(walked, s)
		}
	}
	if want := scaling.SharpChoices(3840, 2160, 1); !reflect.DeepEqual(walked, want) {
		t.Fatalf("l walked %v, want the backend list %v", walked, want)
	}
	m.editOutputs[0].Scale = 1
	next, _ := m.updateLayoutKeys(tea.KeyMsg{Type: tea.KeyLeft})
	if got := mustModel(t, next).editOutputs[0].Scale; got != 1 {
		t.Fatalf("left of 1x should stay at 1 (no sub-1 scales), got %v", got)
	}
	m.editOutputs[0].Scale = 1.33
	next, _ = m.updateLayoutKeys(tea.KeyMsg{Type: tea.KeyRight})
	if got := mustModel(t, next).editOutputs[0].Scale; got != 1.33333 {
		t.Fatalf("right of an unsharp 1.33 is the next listed scale, got %v", got)
	}
}

// Enter and More… open the full sharp list; Custom… types any value.
func TestMoreOpensTheFullSharpList(t *testing.T) {
	m := scalePillModel(t, 158, 37, 1.5)
	next, _ := m.updateLayoutKeys(tea.KeyMsg{Type: tea.KeyEnter})
	m = mustModel(t, next)
	if m.mode != modeModePicker || m.picker.FieldIndex != 2 {
		t.Fatal("Enter on Scale should open the scale list")
	}
	items := m.picker.List.Items()
	if len(items) != len(scaling.Choices(3840, 2160, 1.5))+1 {
		t.Fatalf("list has %d items", len(items))
	}
	m.picker.List.Select(1) // 1.06667
	m.commitModePicker()
	if m.editOutputs[0].Scale != 1.06667 || m.mode != modeMain {
		t.Fatalf("picking from the list set %v", m.editOutputs[0].Scale)
	}

	m = scalePillModel(t, 158, 37, 1.5)
	inspector, _ := m.layoutInspectorRect()
	x, y := findVisiblePositionBelow(t, m.View(), "More…", inspector.y)
	next, _ = m.updateMouse(mousePressAt(x+1, y))
	if got := mustModel(t, next); got.mode != modeModePicker {
		t.Fatal("clicking More… should open the scale list")
	}
	x, y = findVisiblePositionBelow(t, m.View(), " 2x ", inspector.y)
	next, _ = m.updateMouse(mousePressAt(x+1, y))
	if got := mustModel(t, next).editOutputs[0].Scale; got != 2 {
		t.Fatalf("clicking 2x set %v", got)
	}
}

// Two scales that round alike get the precision that tells them apart.
func TestScalePillLabelsStayDistinct(t *testing.T) {
	labels := scaleChoiceLabels([]float64{1.33, 1.33333, 1.5})
	if labels[0] == labels[1] || labels[2] != "1.5x" {
		t.Fatalf("labels %v", labels)
	}
}

// At 80x24 the row stays one line and the view fits; a narrow row slides
// around the selection and marks what is hidden.
func TestScaleRowFitsSmallTerminals(t *testing.T) {
	m := scalePillModel(t, 80, 24, 2)
	view := m.View()
	requireContains(t, ansi.Strip(view), " 2x ", "More…")
	if h := lipgloss.Height(view); h > 24 {
		t.Fatalf("height %d", h)
	}
	if w := maxRenderedLineWidth(view); w > 80 {
		t.Fatalf("width %d", w)
	}
	lines, spans := m.renderScaleRow(m.editOutputs[0], 24, true, false)
	plain := ansi.Strip(lines[0])
	if len(lines) != 1 || !strings.Contains(plain, " 2x ") || !strings.Contains(plain, "‹") || lipgloss.Width(lines[0]) > 24 {
		t.Fatalf("narrow row = %q", plain)
	}
	for _, span := range spans {
		if span.end > 24 {
			t.Fatalf("span %+v past the row", span)
		}
	}
}

// Every choice row steps like the pills: arrows clamp, Enter wraps.
func TestChoiceRowsClampArrowsAndWrapEnter(t *testing.T) {
	m := scalePillModel(t, 158, 37, 1)
	m.inspectorField = 5
	m.editOutputs[0].VRR = 2
	next, _ := m.updateLayoutKeys(tea.KeyMsg{Type: tea.KeyRight})
	m = mustModel(t, next)
	if m.editOutputs[0].VRR != 2 {
		t.Fatal("right on the last option should stay")
	}
	next, _ = m.updateLayoutKeys(tea.KeyMsg{Type: tea.KeyEnter})
	m = mustModel(t, next)
	if m.editOutputs[0].VRR != 0 {
		t.Fatal("Enter on the last option should wrap to the first")
	}
	next, _ = m.updateLayoutKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	if mustModel(t, next).editOutputs[0].VRR != 0 {
		t.Fatal("h on the first option should stay")
	}
}

// Pill labels match the panel's, per the shared vectors' label rule.
func TestScalePillLabelsMatchThePanel(t *testing.T) {
	data, err := os.ReadFile("../scaling/testdata/panel-scale-vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Presets []struct {
			Width, Height int
			Pills         []float64
			Labels        []string
		}
	}
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatal(err)
	}
	for _, v := range file.Presets {
		if got := scaleChoiceLabels(v.Pills); !reflect.DeepEqual(got, v.Labels) {
			t.Errorf("%dx%d: TUI %v, panel %v", v.Width, v.Height, got, v.Labels)
		}
	}
}

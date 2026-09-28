package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/profile"
	"github.com/crmne/hyprmoncfg/internal/scaling"
)

// A card puts identity at the top with workspace chips beside the connector,
// and the mode and placement on its bottom rows.
func TestStageCardAnchorsIdentityTopAndPlacementBottom(t *testing.T) {
	m := paneTestModel(t, tabLayout, []hypr.Monitor{paneTestDesk}, nil)
	output := m.editOutputs[0]
	grid := m.newCanvasCells(60, 14)
	rect := canvasRect{x: 1, y: 1, w: 50, h: 12}
	colors := m.canvasCardStyle(output, false)
	paintCard(grid, rect, false, colors, func(maxLines, maxWidth int) []cardLine {
		return m.monitorCardLines(output, []string{"1", "2", "3"}, monitorCardLayout, maxLines, maxWidth, colors, "", m.styles.palette.warning)
	})
	lines := strings.Split(ansi.Strip(renderCanvasCells(grid)), "\n")
	top, bottom := lines[rect.y+1], lines[rect.y+rect.h-2]
	if !strings.Contains(top, "DP-1") || !strings.Contains(top, " 1   2   3 ") {
		t.Fatalf("name row should carry the connector and chips, got %q", top)
	}
	if strings.Index(top, "DP-1") > strings.Index(top, " 1 ") {
		t.Fatalf("chips belong right of the connector, got %q", top)
	}
	if !strings.Contains(lines[rect.y+2], "Microstep MPG321UR-QD") {
		t.Fatalf("model with whole-inch size belongs under the connector, got %q", lines[rect.y+2])
	}
	if !strings.Contains(bottom, "Scale 1x  Position 0,0") || !strings.Contains(lines[rect.y+rect.h-3], "3840x2160@60Hz") {
		t.Fatalf("mode and placement belong on the bottom rows, got %q / %q", lines[rect.y+rect.h-3], bottom)
	}
}

// Selection is visible without color: the selected card uses heavy lines.
func TestSelectedStageCardUsesHeavyBorder(t *testing.T) {
	m := paneTestModel(t, tabLayout, []hypr.Monitor{paneTestDesk, paneTestSide}, nil)
	m.selectedOutput = 1
	view := ansi.Strip(m.renderCanvas(100, 20))
	if strings.Count(view, "┏") != 1 || strings.Count(view, "╭") != 1 {
		t.Fatalf("expected exactly one heavy (selected) and one light card, got:\n%s", view)
	}
}

// Tiny cards keep the workspaces readable instead of dropping them.
func TestTinyStageCardsKeepWorkspaceIDs(t *testing.T) {
	m := paneTestModel(t, tabWorkspaces, []hypr.Monitor{paneTestDesk, paneTestSide}, nil)
	m.workspaceEdit.Enabled = true
	m.workspaceEdit.Strategy = profile.WorkspaceStrategySequential
	m.workspaceEdit.MaxWorkspaces, m.workspaceEdit.GroupSize = 6, 3
	m.width, m.height = 80, 24
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "1,2,3") && !strings.Contains(view, " 1   2   3 ") {
		t.Fatalf("workspace IDs missing from small stage:\n%s", view)
	}
}

func TestClosedSetsRenderAsChoiceRows(t *testing.T) {
	m := paneTestModel(t, tabLayout, []hypr.Monitor{paneTestDesk}, nil)
	m.width, m.height = 158, 37
	m.layoutFocus = layoutFocusInspector
	view := ansi.Strip(m.View())
	requireContains(t, view, " On   Off ", " Off   On   Fullscreen ", "Normal", "1x")

	m.inspectorTab = inspectorTabColor
	m.inspectorField = 3
	requireContains(t, ansi.Strip(m.View()), " 8-bit   10-bit ")
	m.width = 200
	requireContains(t, ansi.Strip(m.View()), " Default   Gamma 2.2   sRGB ")

	// A row too narrow for its options falls back to the selected value.
	if _, _, ok := m.renderChoiceRow(18, "auto", 20, false); ok {
		t.Fatal("three capability options cannot fit 20 columns")
	}
	row, spans, ok := m.renderChoiceRow(18, "auto", 40, false)
	if !ok || len(spans) != 3 || ansi.Strip(row) != " Force off   Auto-detect   Force on " {
		t.Fatalf("capability choice row = %q %v", ansi.Strip(row), spans)
	}
}

func TestChoiceRowsCycleByKeyAndSelectByPointer(t *testing.T) {
	m := paneTestModel(t, tabLayout, []hypr.Monitor{paneTestDesk}, nil)
	m.width, m.height = 158, 37
	m.layoutFocus = layoutFocusInspector
	m.inspectorField = 5

	next, _ := m.updateLayoutKeys(tea.KeyMsg{Type: tea.KeyEnter})
	m = mustModel(t, next)
	if m.editOutputs[0].VRR != 1 || m.mode != modeMain {
		t.Fatalf("Enter should cycle VRR in place, got VRR=%d mode=%v", m.editOutputs[0].VRR, m.mode)
	}

	inspector, _ := m.layoutInspectorRect()
	x, y := findVisiblePositionBelow(t, m.View(), "Fullscreen", inspector.y)
	next, _ = m.updateMouse(mousePressAt(x+1, y))
	m = mustModel(t, next)
	if m.editOutputs[0].VRR != 2 || !m.dirty {
		t.Fatalf("clicking Fullscreen should select it in the draft, got VRR=%d dirty=%v", m.editOutputs[0].VRR, m.dirty)
	}
}

func TestScaleArrowsStepBetweenSharpScales(t *testing.T) {
	m := paneTestModel(t, tabLayout, []hypr.Monitor{paneTestDesk}, nil)
	m.layoutFocus = layoutFocusInspector
	m.inspectorField = 2
	m.editOutputs[0].Scale = 1.5
	m.adjustInspectorField(1)
	up := m.editOutputs[0].Scale
	m.adjustInspectorField(-1)
	down := m.editOutputs[0].Scale
	if up <= 1.5 || !scaling.Sharp(3840, 2160, up) {
		t.Fatalf("right should reach the next sharp scale above 1.5, got %v", up)
	}
	if down != 1.5 {
		t.Fatalf("left should return to 1.5, got %v", down)
	}
}

func TestShiftArrowsMovePositionByOnePixel(t *testing.T) {
	m := paneTestModel(t, tabLayout, []hypr.Monitor{paneTestDesk}, nil)
	m.layoutFocus = layoutFocusInspector
	m.inspectorField = 7
	next, _ := m.updateLayoutKeys(tea.KeyMsg{Type: tea.KeyShiftRight})
	m = mustModel(t, next)
	m.inspectorField = 8
	next, _ = m.updateLayoutKeys(tea.KeyMsg{Type: tea.KeyShiftLeft})
	m = mustModel(t, next)
	if m.editOutputs[0].X != 1 || m.editOutputs[0].Y != -1 {
		t.Fatalf("expected 1px steps, got %d,%d", m.editOutputs[0].X, m.editOutputs[0].Y)
	}
}

// Exact position entry happens on the row itself, so the stage stays in view.
func TestPositionEntryEditsInPlace(t *testing.T) {
	m := paneTestModel(t, tabLayout, []hypr.Monitor{paneTestDesk, paneTestSide}, nil)
	m.width, m.height = 158, 37
	m.layoutFocus = layoutFocusInspector
	m.inspectorField = 7
	m.activateInspectorField()
	if m.mode != modeNumericInput || m.input == nil {
		t.Fatal("Enter on Position X should start exact entry")
	}
	view := ansi.Strip(m.View())
	requireContains(t, view, "Monitor Layout", "Hardware", "px")
	if strings.Contains(view, "Set Position X") {
		t.Fatal("position entry should not cover the stage with a dialog")
	}
	m.input.Input.SetValue("5000")
	next, _ := m.updateNumericInputKeys(tea.KeyMsg{Type: tea.KeyEnter})
	m = mustModel(t, next)
	if m.editOutputs[m.selectedOutput].X != 5000 || m.mode != modeMain {
		t.Fatalf("typed position not applied, got X=%d mode=%v", m.editOutputs[m.selectedOutput].X, m.mode)
	}
}

func confirmTestModel(t *testing.T, width, height int) Model {
	t.Helper()
	m := paneTestModel(t, tabLayout, []hypr.Monitor{paneTestDesk, paneTestSide}, nil)
	m.width, m.height = width, height
	start := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	m.now = func() time.Time { return start }
	m.pending = &pendingApply{profile: profile.New("Desk", m.currentProfileOutputs()), deadline: start.Add(15 * time.Second), total: 30 * time.Second}
	m.mode = modeConfirm
	return m
}

func TestKeepRevertShowsCountdownAndButtonsAtEverySize(t *testing.T) {
	for _, size := range [][2]int{{158, 37}, {80, 24}, {60, 16}, {60, 12}} {
		m := confirmTestModel(t, size[0], size[1])
		view := m.View()
		plain := ansi.Strip(view)
		requireContains(t, plain, "Keep this profile?", "Desk · the previous", "15s", confirmKeepLabel, confirmRevertLabel)
		if w := maxRenderedLineWidth(view); w > m.terminalWidth() {
			t.Fatalf("%v: width %d exceeds terminal", size, w)
		}
		if h := lipgloss.Height(view); h > m.terminalHeight() {
			t.Fatalf("%v: height %d exceeds terminal", size, h)
		}
	}

	m := confirmTestModel(t, 80, 24)
	meter := ansi.Strip(m.renderCountdownMeter(15*time.Second, 30*time.Second, 40))
	if filled, empty := strings.Count(meter, "━"), strings.Count(meter, "─"); filled != empty || filled == 0 {
		t.Fatalf("half the window left should fill half the meter, got %q", meter)
	}
}

func TestKeepRevertCountdownFollowsTheClock(t *testing.T) {
	m := confirmTestModel(t, 80, 24)
	later := m.pending.deadline.Add(-4 * time.Second)
	m.now = func() time.Time { return later }
	requireContains(t, ansi.Strip(m.View()), "returns in 4 seconds", " 4s")
	gone := m.pending.deadline.Add(time.Second)
	m.now = func() time.Time { return gone }
	if _, cmd := m.Update(tickMsg(gone)); cmd == nil {
		t.Fatal("an expired preview must start the revert")
	}
}

func TestKeepAndRevertButtonsWorkWithThePointer(t *testing.T) {
	m := confirmTestModel(t, 80, 24)
	x, y := findVisiblePosition(t, m.View(), confirmKeepLabel)
	next, _ := m.updateMouse(mousePressAt(x+1, y))
	if got := mustModel(t, next); got.mode != modeMain || got.pending != nil {
		t.Fatalf("Keep click should confirm, got mode=%v pending=%v", got.mode, got.pending)
	}

	m = confirmTestModel(t, 80, 24)
	x, y = findVisiblePosition(t, m.View(), confirmRevertLabel)
	if _, cmd := m.updateMouse(mousePressAt(x+1, y)); cmd == nil {
		t.Fatal("Revert click should start the revert")
	}
}

func TestDeleteDialogButtonsWorkWithThePointer(t *testing.T) {
	p := profile.FromState("Desk", []hypr.Monitor{paneTestDesk}, nil)
	m := paneTestModel(t, tabProfiles, []hypr.Monitor{paneTestDesk}, []profile.Profile{p})
	m.mode, m.deleteProfileName = modeDeleteConfirm, "Desk"
	x, y := findVisiblePosition(t, m.View(), deleteCancelLabel)
	next, cmd := m.updateMouse(mousePressAt(x+1, y))
	if got := mustModel(t, next); got.mode != modeMain || cmd != nil {
		t.Fatal("Cancel must close without deleting")
	}
	x, y = findVisiblePosition(t, m.View(), deleteConfirmLabel)
	if _, cmd := m.updateMouse(mousePressAt(x+1, y)); cmd == nil {
		t.Fatal("Delete profile click must start the deletion")
	}
}

// Previews put the stage first; the text below gets the rows it needs and
// the stage the rest.
func TestPreviewStageTakesTheRowsTheTextDoesNotNeed(t *testing.T) {
	m := paneTestModel(t, tabProfiles, []hypr.Monitor{paneTestDesk, paneTestSide}, nil)
	outputs := m.editOutputs
	frame := m.styles.staticPane.GetVerticalFrameSize()
	stage, text := m.stageAndTextHeights(outputs, 100, 40, 12)
	if text != 12+frame || stage != 40-text {
		t.Fatalf("stage=%d text=%d, want text %d", stage, text, 12+frame)
	}
	if _, text := m.stageAndTextHeights(outputs, 100, 40, 60); text != 20 {
		t.Fatalf("long text should stop at half the column, got %d", text)
	}
	if stage, _ := m.stageAndTextHeights(outputs, 100, 10, 12); stage != 0 {
		t.Fatal("a short column keeps only the text")
	}
}

// The save dialog's help wraps instead of being cut off, and still fits 80x24.
func TestSaveDialogHelpFitsSmallTerminals(t *testing.T) {
	desk := profile.FromState("Desk", []hypr.Monitor{paneTestDesk}, nil)
	for _, size := range [][2]int{{80, 24}, {158, 37}} {
		m := paneTestModel(t, tabLayout, []hypr.Monitor{paneTestDesk}, []profile.Profile{desk})
		m.width, m.height = size[0], size[1]
		next, _ := m.openSaveDialog()
		view := next.(*Model).View()
		requireContains(t, ansi.Strip(view), "[Save & Apply]", "Esc cancels.")
		if h := lipgloss.Height(view); h > m.height {
			t.Fatalf("%v: save dialog height %d exceeds terminal", size, h)
		}
		if strings.Contains(ansi.Strip(view), "▸") {
			t.Fatal("the highlighted profile needs no arrow")
		}
	}
}

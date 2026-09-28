package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

type dragRig struct {
	m      Model
	canvas hitRect
	inner  hitRect
}

func newDragRig(t *testing.T, monitors ...hypr.Monitor) dragRig {
	t.Helper()
	m := paneTestModel(t, tabLayout, monitors, nil)
	m.width, m.height = 158, 37
	canvas, _ := m.layoutCanvasRect()
	return dragRig{m: m, canvas: canvas, inner: canvas.inner(m.styles.inactivePane)}
}

func (r dragRig) layout(m Model) canvasGeometry {
	return m.canvasLayout(r.canvas.w-m.styles.inactivePane.GetHorizontalFrameSize(), m.canvasMouseHeight())
}

func (r dragRig) rectOf(m Model, index int) canvasRect {
	for _, rect := range r.layout(m).rects {
		if rect.index == index {
			return rect
		}
	}
	return canvasRect{}
}

func outputIndexNamed(t *testing.T, m Model, name string) int {
	t.Helper()
	for idx, output := range m.editOutputs {
		if output.Name == name {
			return idx
		}
	}
	t.Fatalf("no output %s", name)
	return -1
}

func mouseAt(x, y int, action tea.MouseAction) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Action: action, Button: tea.MouseButtonLeft}
}

// A long drag keeps the card under the pointer the whole way: the stage
// transform is frozen at the press, and the position comes from the grab
// origin rather than from summed motion deltas.
func TestLongDragKeepsTheCardUnderThePointer(t *testing.T) {
	r := newDragRig(t, paneTestDesk, paneTestSide)
	m := r.m
	side, desk := outputIndexNamed(t, m, "DP-2"), outputIndexNamed(t, m, "DP-1")
	_ = desk
	rect := r.rectOf(m, side)
	startX, startY := r.inner.x+rect.x+3, r.inner.y+rect.y+2
	grab := startX - (r.inner.x + rect.x)
	next, _ := m.updateMouse(mouseAt(startX, startY, tea.MouseActionPress))
	m = mustModel(t, next)
	orig := m.editOutputs[side]

	for step := 1; step <= 40; step++ {
		next, _ = m.updateMouse(mouseAt(startX+step, startY+step/8, tea.MouseActionMotion))
		m = mustModel(t, next)
		card := r.rectOf(m, side)
		if got := r.inner.x + card.x + grab; got != startX+step {
			t.Fatalf("step %d: card grab point at column %d, pointer at %d", step, got, startX+step)
		}
	}

	// Jitter back and forth must not accumulate rounding.
	for _, dx := range []int{-7, 13, -2, 9, -13} {
		next, _ = m.updateMouse(mouseAt(startX+40+dx, startY+5, tea.MouseActionMotion))
		m = mustModel(t, next)
	}
	next, _ = m.updateMouse(mouseAt(startX+40, startY+5, tea.MouseActionMotion))
	m = mustModel(t, next)
	d := m.drag
	wantX, wantY := d.dragTarget(startX+40, startY+5)
	if m.editOutputs[side].X != wantX || m.editOutputs[side].Y != wantY {
		t.Fatalf("after jitter at %d,%d, want %d,%d", m.editOutputs[side].X, m.editOutputs[side].Y, wantX, wantY)
	}
	if wantX-orig.X != int(float64(40)/(d.Geometry.scale*d.Geometry.cellW)+0.5) {
		t.Fatalf("40 cells should be %v logical px, moved %d", float64(40)/(d.Geometry.scale*d.Geometry.cellW), wantX-orig.X)
	}

	// Far from any edge, the drop lands exactly where the pointer is.
	next, _ = m.updateMouse(mouseAt(startX+40, startY+5, tea.MouseActionRelease))
	m = mustModel(t, next)
	if m.drag != nil || m.editOutputs[side].X != wantX || m.editOutputs[side].Y != wantY {
		t.Fatalf("dropped at %d,%d, want %d,%d", m.editOutputs[side].X, m.editOutputs[side].Y, wantX, wantY)
	}
	if m.layoutErr != nil {
		t.Fatalf("drop left an invalid layout: %v", m.layoutErr)
	}
}

// A burst of motion events costs constant work each: no layout, hit test, or
// validation until the drop.
func TestDragMotionDoesBoundedWork(t *testing.T) {
	r := newDragRig(t, paneTestDesk, paneTestSide)
	side := outputIndexNamed(t, r.m, "DP-2")
	rect := r.rectOf(r.m, side)
	startX, startY := r.inner.x+rect.x+3, r.inner.y+rect.y+2
	next, _ := r.m.updateMouse(mouseAt(startX, startY, tea.MouseActionPress))
	m := mustModel(t, next)
	i := 0
	allocs := testing.AllocsPerRun(1000, func() {
		i++
		m.dragMotion(startX-i%80, startY+i%9)
	})
	if allocs != 0 {
		t.Fatalf("a drag motion allocated %.1f times; it should only set the position", allocs)
	}
	if m.layoutErr != nil {
		t.Fatal("motion must not validate mid-drag")
	}
}

// A drop on top of another display resolves the way the panel's editor does:
// the TUI and the daemon's ApplyEditorEdit land on the same position.
func TestDropOnADisplayMatchesThePanelResolution(t *testing.T) {
	r := newDragRig(t, paneTestDesk, paneTestSide)
	m := r.m
	side, desk := outputIndexNamed(t, m, "DP-2"), outputIndexNamed(t, m, "DP-1")
	_ = desk
	rect := r.rectOf(m, side)
	deskRect := r.rectOf(m, desk)
	startX, startY := r.inner.x+rect.x+3, r.inner.y+rect.y+2
	endX, endY := r.inner.x+deskRect.x+deskRect.w/2, r.inner.y+deskRect.y+deskRect.h/2
	next, _ := m.updateMouse(mouseAt(startX, startY, tea.MouseActionPress))
	m = mustModel(t, next)
	d := *m.drag
	next, _ = m.updateMouse(mouseAt(endX, endY, tea.MouseActionMotion))
	m = mustModel(t, next)
	next, _ = m.updateMouse(mouseAt(endX, endY, tea.MouseActionRelease))
	m = mustModel(t, next)

	if err := profile.ValidateLayout(m.currentProfileOutputs()); err != nil {
		t.Fatalf("drop left displays overlapping: %v", err)
	}
	x, y := d.dragTarget(endX, endY)
	draft := profile.Profile{Outputs: append([]profile.OutputConfig(nil), r.m.currentProfileOutputs()...)}
	key := draft.Outputs[side].Key
	panel, err := profile.ApplyEditorEdit(draft, profile.EditorEdit{OutputKey: key, X: &x, Y: &y, SnapDistance: d.dropSnapDistance()})
	if err != nil {
		t.Fatal(err)
	}
	want, _ := panel.OutputByKey(key)
	if got := m.editOutputs[side]; got.X != want.X || got.Y != want.Y {
		t.Fatalf("TUI drop at %d,%d, panel path at %d,%d", got.X, got.Y, want.X, want.Y)
	}
}

// Releasing near a neighbour's edge snaps flush to it.
func TestDropNearAnEdgeSnapsFlush(t *testing.T) {
	r := newDragRig(t, paneTestDesk, paneTestSide)
	m := r.m
	side, desk := outputIndexNamed(t, m, "DP-2"), outputIndexNamed(t, m, "DP-1")
	_ = desk
	rect := r.rectOf(m, side)
	startX, startY := r.inner.x+rect.x+3, r.inner.y+rect.y+2
	next, _ := m.updateMouse(mouseAt(startX, startY, tea.MouseActionPress))
	m = mustModel(t, next)
	next, _ = m.updateMouse(mouseAt(startX+1, startY, tea.MouseActionMotion))
	m = mustModel(t, next)
	next, _ = m.updateMouse(mouseAt(startX+1, startY, tea.MouseActionRelease))
	m = mustModel(t, next)
	deskW, _ := m.editOutputs[desk].logicalSize()
	if m.editOutputs[side].X != deskW || m.editOutputs[side].Y != 0 {
		t.Fatalf("a one-cell nudge off the edge should snap back flush, got %d,%d", m.editOutputs[side].X, m.editOutputs[side].Y)
	}
}

// A click without movement selects the display and never moves or snaps it.
func TestClickWithoutDragDoesNotMove(t *testing.T) {
	r := newDragRig(t, paneTestDesk, paneTestSide)
	m := r.m
	side, desk := outputIndexNamed(t, m, "DP-2"), outputIndexNamed(t, m, "DP-1")
	_ = desk
	m.editOutputs[side].X += 7 // within snap distance of the edge
	rect := r.rectOf(m, side)
	x, y := r.inner.x+rect.x+3, r.inner.y+rect.y+2
	next, _ := m.updateMouse(mouseAt(x, y, tea.MouseActionPress))
	m = mustModel(t, next)
	next, _ = m.updateMouse(mouseAt(x, y, tea.MouseActionRelease))
	m = mustModel(t, next)
	if m.editOutputs[side].X != 3847 || m.dirty {
		t.Fatalf("a click moved the display to %d (dirty %v)", m.editOutputs[side].X, m.dirty)
	}
}

// Keyboard moves use the same rule as the panel's nudges: a move that would
// cover another display is refused with a reason, and nothing changes.
func TestKeyboardMovesNeverOverlap(t *testing.T) {
	m := paneTestModel(t, tabLayout, []hypr.Monitor{paneTestDesk, paneTestSide}, nil)
	side, desk := outputIndexNamed(t, m, "DP-2"), outputIndexNamed(t, m, "DP-1")
	_ = desk
	_, _ = side, desk
	m.selectedOutput = side
	next, _ := m.updateLayoutKeys(tea.KeyMsg{Type: tea.KeyLeft})
	m = mustModel(t, next)
	if m.editOutputs[side].X != 3840 || !m.statusErr || !strings.Contains(m.status, "DP-2 would overlap DP-1") {
		t.Fatalf("nudge into a neighbour should be refused, got X=%d status %q", m.editOutputs[side].X, m.status)
	}
	next, _ = m.updateLayoutKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'0'}})
	m = mustModel(t, next)
	if m.editOutputs[side].X != 3840 {
		t.Fatal("moving to 0,0 on top of another display should be refused")
	}
	next, _ = m.updateLayoutKeys(tea.KeyMsg{Type: tea.KeyRight})
	m = mustModel(t, next)
	if m.editOutputs[side].X != 3940 || m.statusErr {
		t.Fatalf("a free nudge should move, got X=%d", m.editOutputs[side].X)
	}
}

// Place beside (Alt+arrows) goes through the shared rules and matches them.
func TestPlaceBesideUsesTheSharedPlacement(t *testing.T) {
	m := paneTestModel(t, tabLayout, []hypr.Monitor{paneTestDesk, paneTestSide}, nil)
	side, desk := outputIndexNamed(t, m, "DP-2"), outputIndexNamed(t, m, "DP-1")
	_ = desk
	_, _ = side, desk
	m.selectedOutput = side
	x, y, _, ok := profile.BesidePosition(m.currentProfileOutputs(), side, profile.PlaceBelow)
	if !ok {
		t.Fatal("no anchor")
	}
	next, _ := m.updateLayoutKeys(tea.KeyMsg{Type: tea.KeyDown, Alt: true})
	m = mustModel(t, next)
	if m.editOutputs[side].X != x || m.editOutputs[side].Y != y {
		t.Fatalf("placed at %d,%d, shared rule says %d,%d", m.editOutputs[side].X, m.editOutputs[side].Y, x, y)
	}
}

// Inspector edits that would grow a display into a neighbour are refused;
// typed positions that would overlap keep the entry open with the reason.
func TestInspectorEditsNeverCreateOverlap(t *testing.T) {
	m := paneTestModel(t, tabLayout, []hypr.Monitor{paneTestDesk, paneTestSide}, nil)
	side, desk := outputIndexNamed(t, m, "DP-2"), outputIndexNamed(t, m, "DP-1")
	_ = desk
	_, _ = side, desk
	m.selectedOutput = desk
	m.layoutFocus = layoutFocusInspector
	m.inspectorField = 7
	m.activateInspectorField()
	m.input.Input.SetValue("100")
	next, _ := m.updateNumericInputKeys(tea.KeyMsg{Type: tea.KeyEnter})
	m = mustModel(t, next)
	if m.mode != modeNumericInput || m.input.Input.Err == nil || m.editOutputs[desk].X != 0 {
		t.Fatalf("typed overlap should keep the entry open with an error, got mode %v X=%d", m.mode, m.editOutputs[desk].X)
	}
	requireContains(t, ansi.Strip(m.View()), "would overlap")

	// Turning on a display whose saved position is covered is refused too.
	m.mode, m.input = modeMain, nil
	m.editOutputs[side].Enabled = false
	m.editOutputs[side].X = 100
	m.revalidate()
	m.selectedOutput = side
	m.inspectorField = 0
	m.setInspectorChoice(0, "on")
	if m.editOutputs[side].Enabled || !m.statusErr || !strings.Contains(m.status, "change not made") {
		t.Fatalf("enabling onto another display should be refused, enabled=%v status %q", m.editOutputs[side].Enabled, m.status)
	}
}

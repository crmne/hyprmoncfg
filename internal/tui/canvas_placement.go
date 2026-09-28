package tui

import (
	"math"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/crmne/hyprmoncfg/internal/profile"
)

// placeSelected moves the selected display through the shared placement
// rules (profile.MoveOutput), the same rules the panel reaches through the
// daemon's editor. A positive snap distance makes it a pointer drop. A move
// that would overlap another display is refused with a status message and
// the display keeps its position.
func (m *Model) placeSelected(x, y, snapDistance int) (profile.SnapAnalysis, bool) {
	if len(m.editOutputs) == 0 || m.selectedOutput < 0 || m.selectedOutput >= len(m.editOutputs) {
		return profile.SnapAnalysis{}, false
	}
	outputs := m.currentProfileOutputs()
	analysis, err := profile.MoveOutput(outputs, m.selectedOutput, x, y, snapDistance)
	if err != nil {
		m.setStatusErr(err.Error())
		return analysis, false
	}
	if m.statusErr {
		// A refusal from an earlier move no longer applies.
		m.status, m.statusErr = "", false
	}
	output := &m.editOutputs[m.selectedOutput]
	if output.X == outputs[m.selectedOutput].X && output.Y == outputs[m.selectedOutput].Y {
		return analysis, true
	}
	output.X, output.Y = outputs[m.selectedOutput].X, outputs[m.selectedOutput].Y
	m.layoutChanged()
	return analysis, true
}

// guardLayoutEdit applies an inspector edit and takes it back when it would
// make displays overlap (a larger mode or scale, a rotation, turning one on),
// as the panel's editor refuses such an edit. A layout that already overlapped
// stays editable so it can be fixed.
func (m *Model) guardLayoutEdit(edit func()) bool {
	before := append([]editableOutput(nil), m.editOutputs...)
	beforeOutputs := m.currentProfileOutputs()
	wasDirty, wasSaved := m.dirty, m.draftSaved
	edit()
	if err := profile.RejectNewOverlap(beforeOutputs, m.currentProfileOutputs()); err != nil {
		m.editOutputs = before
		m.dirty, m.draftSaved = wasDirty, wasSaved
		m.revalidate()
		m.setStatusErr(err.Error() + "; change not made")
		return false
	}
	return true
}

// canvasDragState is one pointer drag on the stage. The canvas transform is
// frozen at the press, so the display tracks the pointer even when the drag
// grows the arrangement; the position is computed from the grab origin, not
// accumulated from motion deltas, so rounding never drifts.
type canvasDragState struct {
	OutputIndex int
	// StartX, StartY are the pointer cell at the press.
	StartX, StartY int
	// OrigX, OrigY are the display's logical position at the press.
	OrigX, OrigY int
	// Geometry is the canvas transform at the press.
	Geometry canvasGeometry
	// Moved reports that the pointer left the press cell.
	Moved bool
}

// dragTarget is where the dragged display is under the pointer, in logical
// pixels, from the grab origin and the frozen transform.
func (d canvasDragState) dragTarget(x, y int) (int, int) {
	scale := d.Geometry.scale
	if scale <= 0 {
		scale = 1
	}
	cellW := d.Geometry.cellW
	if cellW <= 0 {
		cellW = 1
	}
	return d.OrigX + int(math.Round(float64(x-d.StartX)/(scale*cellW))),
		d.OrigY + int(math.Round(float64(y-d.StartY)/scale))
}

// dropSnapDistance converts the panel's release snap (a fixed on-screen
// distance) to the terminal: one canvas row, the coarsest pointer step, in
// logical pixels.
func (d canvasDragState) dropSnapDistance() int {
	if d.Geometry.scale <= 0 {
		return 1
	}
	return max(1, int(math.Round(1/d.Geometry.scale)))
}

// dragMotion follows the pointer. It only sets the draft position; overlap
// checks, snapping, and validation wait for the drop, as in the panel, so a
// burst of motion events costs a few arithmetic operations each.
func (m *Model) dragMotion(x, y int) {
	d := m.drag
	if d == nil || d.OutputIndex < 0 || d.OutputIndex >= len(m.editOutputs) {
		return
	}
	if x != d.StartX || y != d.StartY {
		d.Moved = true
	}
	if !d.Moved {
		return
	}
	targetX, targetY := d.dragTarget(x, y)
	output := &m.editOutputs[d.OutputIndex]
	if output.X == targetX && output.Y == targetY {
		return
	}
	output.X, output.Y = targetX, targetY
	m.markDirty()
}

// dragRelease drops the display: back at its grab position first, then
// through the shared drop rules. A drop that cannot be resolved returns the
// display to where it was picked up.
func (m *Model) dragRelease(x, y int) tea.Cmd {
	d := m.drag
	m.drag = nil
	if d == nil || d.OutputIndex < 0 || d.OutputIndex >= len(m.editOutputs) {
		return nil
	}
	m.selectedOutput = d.OutputIndex
	if !d.Moved && x == d.StartX && y == d.StartY {
		return nil
	}
	targetX, targetY := d.dragTarget(x, y)
	output := &m.editOutputs[d.OutputIndex]
	output.X, output.Y = d.OrigX, d.OrigY
	m.revalidate()
	analysis, ok := m.placeSelected(targetX, targetY, d.dropSnapDistance())
	if !ok {
		return nil
	}
	var marks []snapMark
	for _, axis := range []profile.SnapAxisCandidate{analysis.X, analysis.Y} {
		if axis.Dist <= d.dropSnapDistance() {
			for _, mark := range axis.Marks {
				marks = append(marks, snapMark{OutputIndex: mark.OutputIndex, Edge: snapEdge(mark.Edge)})
			}
		}
	}
	if len(marks) == 0 {
		return nil
	}
	return m.showSnapHint(&snapHintState{Marks: marks})
}

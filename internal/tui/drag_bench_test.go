package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/crmne/hyprmoncfg/internal/hypr"
)

func dragBenchModel(b *testing.B) Model {
	m := Model{styles: newStyles(), mode: modeMain, tab: tabLayout, width: 158, height: 37,
		monitors: []hypr.Monitor{paneTestDesk, paneTestSide}}
	m.loadLiveState()
	m.status = ""
	return m
}

// BenchmarkLayoutDrag measures what one pointer motion costs: Bubble Tea runs
// Update and then View for every event.
func BenchmarkLayoutDrag(b *testing.B) {
	base := dragBenchModel(b)
	canvas, _ := base.layoutCanvasRect()
	layout := base.canvasLayout(canvas.w-base.styles.inactivePane.GetHorizontalFrameSize(), base.canvasMouseHeight())
	rect := layout.rects[1]
	inner := canvas.inner(base.styles.inactivePane)
	startX, startY := inner.x+rect.x+2, inner.y+rect.y+1
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		m := base
		m.editOutputs = append([]editableOutput(nil), base.editOutputs...)
		var model tea.Model = m
		model, _ = model.Update(tea.MouseMsg{X: startX, Y: startY, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
		b.StartTimer()
		for step := 1; step <= 60; step++ {
			model, _ = model.Update(tea.MouseMsg{X: startX - step, Y: startY + step/6, Action: tea.MouseActionMotion, Button: tea.MouseButtonLeft})
			_ = model.View()
		}
		model, _ = model.Update(tea.MouseMsg{X: startX - 60, Y: startY + 10, Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft})
		_ = model.View()
	}
}

func BenchmarkLayoutView(b *testing.B) {
	m := dragBenchModel(b)
	for i := 0; i < b.N; i++ {
		_ = m.View()
	}
}

// BenchmarkDragMotionUpdate is Update alone for one pointer motion mid-drag.
func BenchmarkDragMotionUpdate(b *testing.B) {
	base := dragBenchModel(b)
	canvas, _ := base.layoutCanvasRect()
	layout := base.canvasLayout(canvas.w-base.styles.inactivePane.GetHorizontalFrameSize(), base.canvasMouseHeight())
	rect := layout.rects[1]
	inner := canvas.inner(base.styles.inactivePane)
	startX, startY := inner.x+rect.x+2, inner.y+rect.y+1
	var model tea.Model = base
	model, _ = model.Update(tea.MouseMsg{X: startX, Y: startY, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		model, _ = model.Update(tea.MouseMsg{X: startX - i%60, Y: startY + i%7, Action: tea.MouseActionMotion, Button: tea.MouseButtonLeft})
	}
}

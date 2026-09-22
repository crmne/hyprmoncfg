package tui

import (
	"github.com/charmbracelet/lipgloss"
)

// Hidden displays have their own selectable rows, outside spatial geometry.
// Rendering and pointer handling use these same rows and action boundaries.
type hiddenDisplayRow struct {
	index   int
	label   string
	action  string
	actionX int
}

func (m Model) hiddenDisplayRows(width, height int) []hiddenDisplayRow {
	var rows []hiddenDisplayRow
	for i, o := range m.editOutputs {
		if o.Enabled && o.MirrorOf == "" {
			continue
		}
		state, action := "Off", "[Enable]"
		if o.Enabled {
			state, action = "Mirrors "+outputNameForKeyIn(m.editOutputs, o.MirrorOf), ""
		}
		connected := false
		for _, live := range m.monitors {
			if live.Name == o.Name {
				connected = true
				break
			}
		}
		if !connected {
			state, action = "Not connected", ""
		}
		labelWidth := max(1, width-lipgloss.Width(action)-1)
		label := "[ " + fitString(o.Name+"  "+state+"  "+o.modelSizeLabel(), max(1, labelWidth-4)) + " ]"
		rows = append(rows, hiddenDisplayRow{i, label, action, lipgloss.Width(label) + 1})
	}
	limit := max(1, height/3)
	start := 0
	for i, row := range rows {
		if row.index == m.selectedOutput && i >= limit {
			start = i - limit + 1
		}
	}
	return rows[start:min(len(rows), start+limit)]
}

func (m Model) paintHiddenDisplays(grid [][]canvasCell) {
	if len(grid) == 0 {
		return
	}
	p := m.styles.palette
	for y, row := range m.hiddenDisplayRows(len(grid[0])-2, len(grid)) {
		for x := range grid[y] {
			grid[y][x] = canvasCell{ch: ' ', fg: p.cardMuted}
		}
		fg := p.cardDisabledFg
		if row.index == m.selectedOutput {
			fg = p.cardSelectedBorder
		}
		paintCanvasSegments(grid, y, 1, []canvasSegment{{text: row.label, fg: fg, bold: true}, {text: " " + row.action, fg: p.cardSelectedBorder, bold: true}})
	}
}

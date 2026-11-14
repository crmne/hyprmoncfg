package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// wallpaperListHeaderRows is the hint line and the blank line under it.
const wallpaperListHeaderRows = 2

// wallpaperPanes splits the body like the panel's Wallpapers page: the
// profile's stage on top, the synced display list below. Rendering and
// pointer hit-testing share it.
func (m Model) wallpaperPanes(body hitRect) (stage, list hitRect) {
	p, ok := m.wallpaperProfile()
	if !ok {
		return hitRect{}, body
	}
	listLines := len(m.wallpaperRows()) + wallpaperListHeaderRows + 2
	stageH, listH := m.stageAndTextHeights(m.profileEditableOutputs(p), body.w, body.h, listLines)
	return hitRect{x: body.x, y: body.y, w: body.w, h: stageH},
		hitRect{x: body.x, y: body.y + stageH, w: body.w, h: listH}
}

func (m Model) renderWallpapersView(height int) string {
	body := hitRect{w: m.footerContentWidth(), h: height}
	stage, list := m.wallpaperPanes(body)
	listPane := m.renderWallpaperListPane(list.w, list.h)
	if stage.h == 0 {
		return listPane
	}
	style := m.paneStyle(paneToneStatic)
	innerW := max(1, stage.w-style.GetHorizontalFrameSize())
	innerH := max(1, stage.h-style.GetVerticalFrameSize())
	p, _ := m.wallpaperProfile()
	canvas := fitBlock(m.renderProfileCanvas(p, innerW, innerH), innerW, innerH)
	return lipgloss.JoinVertical(lipgloss.Left, m.renderTitledPane(paneToneStatic, "Monitor Layout", canvas, stage.w), listPane)
}

func (m Model) renderWallpaperListPane(width, height int) string {
	style := m.paneStyle(paneToneFocused)
	innerW := max(1, width-style.GetHorizontalFrameSize())
	innerH := max(1, height-style.GetVerticalFrameSize())
	title := "Wallpapers"
	if _, ok := m.wallpaperProfile(); ok {
		title += " · " + m.wallpaperProfileName()
	}
	lines := m.wallpaperListLines(innerW)
	return m.renderTitledPane(paneToneFocused, title, fitBlock(strings.Join(lines, "\n"), innerW, innerH), width)
}

func (m Model) wallpaperListLines(width int) []string {
	if m.wallpaperErr != nil {
		return []string{m.styles.statusError.MaxWidth(width).Render(m.wallpaperErr.Error())}
	}
	if _, ok := m.wallpaperProfile(); !ok {
		return []string{m.styles.subtle.Width(width).Render("Wallpapers follow a saved profile. Use or save a profile for these displays first.")}
	}
	rows := m.wallpaperRows()
	if len(rows) == 0 {
		return []string{m.styles.subtle.Render("No displays are on in this profile.")}
	}
	lines := []string{m.styles.help.MaxWidth(width).Render(m.wallpaperHint()), ""}
	connectorW, labelW := 0, 0
	for _, row := range rows {
		connectorW = max(connectorW, lipgloss.Width(row.Connector))
		labelW = max(labelW, lipgloss.Width(row.Label))
	}
	selected := clampIndex(m.wallpaperSelected, len(rows))
	for idx, row := range rows {
		picture := "Omarchy background"
		if row.Path != "" {
			picture = filepath.Base(row.Path)
		}
		plain := fitString(fmt.Sprintf("%-*s  %-*s  %s", connectorW, row.Connector, labelW, row.Label, picture), width)
		switch {
		case idx == selected:
			// The highlight spans the row, so no arrow is needed.
			lines = append(lines, m.styles.fieldSelected.Padding(0).Bold(true).Render(fmt.Sprintf("%-*s", width, plain)))
		case row.Path == "":
			lines = append(lines, m.styles.subtle.Render(plain))
		default:
			lines = append(lines, m.styles.value.Render(plain))
		}
	}
	return append(lines, "", m.styles.value.Render(fitString(m.wallpaperActions(), width)))
}

// wallpaperHint says when the pictures show, in the panel's words. That the
// panel plugin draws them is in the keys help, where there is room.
func (m Model) wallpaperHint() string {
	if m.wallpaperProfileName() == strings.TrimSpace(m.activeProfileName) {
		return "Each display keeps its own picture while this profile is active."
	}
	return "Saved now; shown when this profile becomes active."
}

func (m Model) wallpaperActions() string {
	actions := []string{"[Path…]"}
	if m.omarchyAvailable() {
		actions = append(actions, "[Choose…]")
	}
	return strings.Join(append(actions, "[Clear]"), " ")
}

func (m Model) updateWallpapersMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return m, nil
	}
	for _, action := range []struct{ label, key string }{{"[Path…]", "enter"}, {"[Choose…]", "c"}, {"[Clear]", "d"}} {
		if m.visibleActionAt(msg.X, msg.Y, action.label) {
			if action.key == "enter" {
				return m.updateWallpaperKeys(tea.KeyMsg{Type: tea.KeyEnter})
			}
			return m.updateWallpaperKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(action.key)})
		}
	}
	_, list := m.wallpaperPanes(m.bodyRect())
	inner := list.inner(m.styles.activePane)
	row := msg.Y - inner.y - wallpaperListHeaderRows
	if inner.contains(msg.X, msg.Y) && row >= 0 && row < len(m.wallpaperRows()) {
		m.wallpaperSelected = row
	}
	return m, nil
}

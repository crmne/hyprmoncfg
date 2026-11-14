package tui

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/crmne/hyprmoncfg/internal/wallpaper"
)

type wallpaperPathState struct {
	Row   wallpaperRow
	Input textinput.Model
	Err   error
}

func (m *Model) openWallpaperPath(row wallpaperRow, value string) tea.Cmd {
	input := textinput.New()
	input.Prompt = ""
	input.CharLimit = 4096
	input.Width = clampInt(m.modalMaxWidth()-16, 24, 72)
	input.TextStyle = m.styles.value
	input.Placeholder = "~/Pictures/wallpaper.png"
	input.PlaceholderStyle = m.styles.subtle
	input.Cursor.Style = lipgloss.NewStyle()
	input.SetValue(value)
	cmd := input.Focus()
	m.wallpaperPath = &wallpaperPathState{Row: row, Input: input}
	m.mode = modeWallpaperPathInput
	return cmd
}

func (m Model) renderWallpaperPath() string {
	if m.wallpaperPath == nil {
		return ""
	}
	inputBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(m.styles.palette.paneActiveBorder)).
		Padding(0, 1).
		Render(m.wallpaperPath.Input.View())
	width := max(20, m.modalMaxWidth()-6)
	body := []string{m.styles.label.Render("Picture (PNG, JPEG or WebP)"), inputBox}
	if m.wallpaperPath.Err != nil {
		body = append(body, "", m.styles.statusError.MaxWidth(width).Render(m.wallpaperPath.Err.Error()))
	}
	body = append(body, "", m.styles.help.MaxWidth(width).Render("Enter saves the picture for this display. Esc cancels."))
	return m.renderModalFrame("Wallpaper for "+m.wallpaperPath.Row.Connector, body)
}

func (m Model) updateWallpaperPathKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.wallpaperPath == nil {
		m.mode = modeMain
		return m, nil
	}
	switch msg.String() {
	case "esc":
		m.wallpaperPath = nil
		m.mode = modeMain
		return m, nil
	case "enter":
		m.commitWallpaperPath()
		return m, nil
	}
	var cmd tea.Cmd
	m.wallpaperPath.Input, cmd = m.wallpaperPath.Input.Update(msg)
	m.wallpaperPath.Err = nil
	return m, cmd
}

// commitWallpaperPath keeps the dialog open on any error, so the path can be
// fixed without typing it again.
func (m *Model) commitWallpaperPath() {
	path, err := wallpaper.ResolveImage(m.wallpaperPath.Input.Value())
	if err == nil {
		err = m.setWallpaper(m.wallpaperPath.Row, path)
	}
	if err != nil {
		m.wallpaperPath.Err = err
		return
	}
	m.wallpaperPath = nil
	m.mode = modeMain
}

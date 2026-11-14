package tui

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/crmne/hyprmoncfg/internal/profile"
)

type profileRenameState struct {
	Name  string
	Input textinput.Model
	Err   error
}

// renameAvailable hides Rename on a daemon too old to do it. Without a daemon
// the TUI is the writer and renames directly.
func (m Model) renameAvailable() bool {
	return m.ipc == nil || m.daemonRenames
}

func (m *Model) openProfileRename() tea.Cmd {
	if m.selectedProfile < 0 || m.selectedProfile >= len(m.profiles) {
		return nil
	}
	name := m.profiles[m.selectedProfile].Name
	input := textinput.New()
	input.Prompt = ""
	input.CharLimit = 64
	input.Width = clampInt(m.modalMaxWidth()-16, 24, 72)
	input.TextStyle = m.styles.value
	input.PlaceholderStyle = m.styles.subtle
	input.Cursor.Style = lipgloss.NewStyle()
	input.SetValue(name)
	cmd := input.Focus()
	m.renameInput = &profileRenameState{Name: name, Input: input}
	m.mode = modeProfileRenameInput
	return cmd
}

func (m Model) renderProfileRename() string {
	if m.renameInput == nil {
		return ""
	}
	inputBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(m.styles.palette.paneActiveBorder)).
		Padding(0, 1).
		Render(m.renameInput.Input.View())
	width := max(20, m.modalMaxWidth()-6)
	body := []string{m.styles.label.Render("New name"), inputBox}
	if m.renameInput.Err != nil {
		body = append(body, "", m.styles.statusError.MaxWidth(width).Render(m.renameInput.Err.Error()))
	}
	body = append(body, "", m.styles.help.MaxWidth(width).Render("Enter renames the profile. Nothing is applied. Esc cancels."))
	return m.renderModalFrame(fmt.Sprintf("Rename %s", m.renameInput.Name), body)
}

func (m Model) updateProfileRenameKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.renameInput == nil {
		m.mode = modeMain
		return m, nil
	}
	switch msg.String() {
	case "esc":
		m.renameInput = nil
		m.mode = modeMain
		return m, nil
	case "enter":
		cmd := m.commitProfileRename()
		return m, cmd
	}
	var cmd tea.Cmd
	m.renameInput.Input, cmd = m.renameInput.Input.Update(msg)
	m.renameInput.Err = nil
	return m, cmd
}

// commitProfileRename catches what it can in the dialog. The store checks
// again, including for files the profile list does not show.
func (m *Model) commitProfileRename() tea.Cmd {
	name, newName := m.renameInput.Name, strings.TrimSpace(m.renameInput.Input.Value())
	if newName == "" {
		m.renameInput.Err = errors.New("Enter a name")
		return nil
	}
	if newName != name {
		for _, saved := range m.profiles {
			if saved.Name != name && m.store.SameFiles(saved.Name, newName) {
				m.renameInput.Err = fmt.Errorf("%q already exists", saved.Name)
				return nil
			}
		}
	}
	m.renameInput = nil
	m.mode = modeMain
	if newName == name {
		return nil
	}
	return m.renameCmd(name, newName)
}

// followProfileRename keeps the selection and every record of the old name.
func (m *Model) followProfileRename(name, newName string) {
	for _, tracked := range []*string{&m.draftProfileName, &m.matchedProfileName, &m.activeProfileName, &m.profileOverride, &m.wallpaperOverride} {
		if strings.EqualFold(strings.TrimSpace(*tracked), strings.TrimSpace(name)) {
			*tracked = newName
		}
	}
	index := slices.IndexFunc(m.profiles, func(p profile.Profile) bool { return p.Name == name })
	if index < 0 {
		return
	}
	m.profiles[index].Name = newName
	slices.SortFunc(m.profiles, func(a, b profile.Profile) int { return strings.Compare(a.Name, b.Name) })
	m.selectedProfile = slices.IndexFunc(m.profiles, func(p profile.Profile) bool { return p.Name == newName })
}

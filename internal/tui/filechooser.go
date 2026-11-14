package tui

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// wallpaperChosenMsg carries what Omarchy's file chooser returned. An empty
// path with no error means the user closed the chooser without picking.
type wallpaperChosenMsg struct {
	path string
	err  error
}

func (m Model) omarchyAvailable() bool {
	if m.hasOmarchy == nil {
		return hasOmarchyCommand()
	}
	return m.hasOmarchy()
}

func (m Model) chooseWallpaperCmd() tea.Cmd {
	choose := m.chooseFile
	if choose == nil {
		choose = omarchyFileSelect
	}
	return func() tea.Msg {
		path, err := choose()
		return wallpaperChosenMsg{path: path, err: err}
	}
}

func hasOmarchyCommand() bool {
	_, err := exec.LookPath("omarchy")
	return err == nil
}

// omarchyFileSelect opens Omarchy's desktop file chooser, as the panel does.
// Exit 1 means nothing was picked; exit 2 means no chooser could run.
func omarchyFileSelect() (string, error) {
	out, err := exec.Command("omarchy", "file", "select",
		"--title", "Choose wallpaper", "--extensions", "png jpg jpeg webp").Output()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("the file chooser could not open: %w", err)
	}
	path, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	return path, nil
}

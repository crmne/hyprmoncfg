package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/crmne/hyprmoncfg/internal/ipc"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

func renameModel(t *testing.T, names ...string) Model {
	t.Helper()
	store := profile.NewStore(t.TempDir())
	m := Model{styles: newStyles(), tab: tabProfiles, store: store, width: 120, height: 40}
	for _, name := range names {
		p := testProfile(name, 1)
		if err := store.Save(p); err != nil {
			t.Fatal(err)
		}
		m.profiles = append(m.profiles, p)
	}
	return m
}

func press(t *testing.T, m Model, msg tea.KeyMsg) (Model, tea.Cmd) {
	t.Helper()
	updated, cmd := m.Update(msg)
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("update returned %T", updated)
	}
	return next, cmd
}

func retype(t *testing.T, m Model, name string) Model {
	t.Helper()
	m.renameInput.Input.SetValue(name)
	return m
}

func TestProfilesTabRenamesWithRAndKeepsTheSelection(t *testing.T) {
	m := renameModel(t, "Alpha", "Dock")
	m.selectedProfile = 1
	m.draftProfileName, m.activeProfileName = "Dock", "Dock"

	m, _ = press(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if m.mode != modeProfileRenameInput || m.renameInput.Input.Value() != "Dock" || m.resetRequested {
		t.Fatalf("r did not open rename: mode %v", m.mode)
	}
	m, cmd := press(t, retype(t, m, "  Aardvark "), tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != modeMain || cmd == nil {
		t.Fatalf("enter did not rename: mode %v", m.mode)
	}
	result := cmd()
	if msg, ok := result.(renameMsg); !ok || msg.err != nil || msg.newName != "Aardvark" {
		t.Fatalf("rename result %+v", result)
	}
	if _, err := m.store.Load("Aardvark"); err != nil {
		t.Fatal(err)
	}
	m.followProfileRename("Dock", "Aardvark")
	if got := m.profiles[m.selectedProfile].Name; got != "Aardvark" || m.selectedProfile != 0 {
		t.Fatalf("selection on %q at %d", got, m.selectedProfile)
	}
	if m.draftProfileName != "Aardvark" || m.activeProfileName != "Aardvark" {
		t.Fatalf("tracked names: %q %q", m.draftProfileName, m.activeProfileName)
	}
}

func TestRenameDialogValidatesBeforeTouchingAnything(t *testing.T) {
	m := renameModel(t, "Work Desk", "Dock")
	m.selectedProfile = 1
	m, _ = press(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})

	for _, name := range []string{"   ", "work-desk"} {
		next, cmd := press(t, retype(t, m, name), tea.KeyMsg{Type: tea.KeyEnter})
		if cmd != nil || next.mode != modeProfileRenameInput || next.renameInput.Err == nil {
			t.Fatalf("%q accepted", name)
		}
		m = next
	}
	requireContains(t, ansi.Strip(m.renderProfileRename()), "already exists")

	unchanged, cmd := press(t, retype(t, m, "Dock"), tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || unchanged.mode != modeMain {
		t.Fatal("the same name must just close the dialog")
	}
	m.renameInput.Err = nil
	cancelled, cmd := press(t, retype(t, m, "Other"), tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil || cancelled.mode != modeMain || cancelled.renameInput != nil {
		t.Fatal("esc must cancel")
	}
}

func TestOlderDaemonHidesRenameAndKeepsR(t *testing.T) {
	m := renameModel(t, "Dock")
	m.ipc = &ipc.Client{}
	m.draftProfileName = "Dock"
	if m.renameAvailable() {
		t.Fatal("rename offered on a daemon without the capability")
	}
	m, _ = press(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if m.mode == modeProfileRenameInput || !m.resetRequested {
		t.Fatalf("r should still reset: mode %v", m.mode)
	}
	if view := ansi.Strip(m.renderProfileListPane(make([]profileMatchSummary, len(m.profiles)), 60, 12)); strings.Contains(view, "[Rename]") {
		t.Fatalf("hidden action shown:\n%s", view)
	}
	m.daemonRenames = true
	if view := ansi.Strip(m.renderProfileListPane(make([]profileMatchSummary, len(m.profiles)), 60, 12)); !strings.Contains(view, "[Rename]") {
		t.Fatalf("action missing:\n%s", view)
	}
}

func TestRenameButtonOpensTheDialog(t *testing.T) {
	m := renameModel(t, "Dock")
	m.width, m.height = 100, 30
	x, y := findVisiblePosition(t, m.View(), "[Rename]")
	updated, _ := m.updateProfilesMouse(tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if updated.(Model).mode != modeProfileRenameInput {
		t.Fatal("button did not open rename")
	}
}

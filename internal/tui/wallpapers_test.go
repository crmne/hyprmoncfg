package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/crmne/hyprmoncfg/internal/profile"
	"github.com/crmne/hyprmoncfg/internal/wallpaper"
)

func runes(key string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
}

// wallpaperModel has two saved profiles. Desk drives a Dell and keeps an LG
// off; Home drives one display.
func wallpaperModel(t *testing.T) Model {
	t.Helper()
	m := renameModel(t, "Home")
	desk := profile.New("Desk", []profile.OutputConfig{
		{Key: "DEL|U2720Q|A", Name: "DP-1", Make: "Dell", Model: "U2720Q", Enabled: true, Width: 3840, Height: 2160, Refresh: 60, Scale: 1.5},
		{Key: "LG|27GL|B", Name: "HDMI-A-1", Make: "LG", Model: "27GL", Enabled: false, Width: 1920, Height: 1080, Refresh: 60, Scale: 1},
	})
	if err := m.store.Save(desk); err != nil {
		t.Fatal(err)
	}
	// Profiles reach the TUI from the store, with normalized output keys.
	saved, err := m.store.Load("Desk")
	if err != nil {
		t.Fatal(err)
	}
	m.profiles = append([]profile.Profile{saved}, m.profiles...)
	m.activeProfileName = "Desk"
	m.hasOmarchy = func() bool { return true }
	return m
}

// deskKey is the stored key of Desk's enabled display.
func deskKey(m Model) string {
	return m.profiles[0].Outputs[0].Key
}

func wallpaperStoreOf(m Model) *wallpaper.Store {
	return wallpaper.NewStore(m.store.BaseDir())
}

func writePicture(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestKeyFourOpensWallpapersTabForTheActiveProfile(t *testing.T) {
	m := wallpaperModel(t)
	m.tab = tabLayout
	m, _ = press(t, m, runes("4"))
	if m.tab != tabWallpapers || m.wallpaperProfileName() != "Desk" {
		t.Fatalf("tab=%v profile=%q", m.tab, m.wallpaperProfileName())
	}
	requireContains(t, ansi.Strip(m.View()), "4 Wallpapers", "Wallpapers · Desk")
}

func TestTabBarClickOpensWallpapers(t *testing.T) {
	m := wallpaperModel(t)
	x, y := findVisiblePosition(t, m.View(), "Wallpapers")
	updated, _ := press2(t, m, mousePressAt(x, y))
	if updated.tab != tabWallpapers {
		t.Fatal("clicking the tab did not open Wallpapers")
	}
}

func TestWallpapersTabListsEnabledOutputsOfTheProfile(t *testing.T) {
	m := wallpaperModel(t)
	if err := wallpaperStoreOf(m).Save(wallpaper.NewDocument().Set("Desk", deskKey(m), "/pics/mountain.png")); err != nil {
		t.Fatal(err)
	}
	m, _ = press(t, m, runes("4"))
	view := ansi.Strip(m.View())
	requireContains(t, view, "DP-1", "Dell U2720Q", "mountain.png")
	if strings.Contains(view, "HDMI-A-1  LG") {
		t.Fatalf("a display kept off must not get a wallpaper row:\n%s", view)
	}

	m, _ = press(t, m, runes("4"))
	m.wallpaperOverride = "Home"
	requireContains(t, ansi.Strip(m.View()), "Omarchy background")
}

func TestWallpapersTabExplainsAMissingProfile(t *testing.T) {
	m := wallpaperModel(t)
	m.activeProfileName = ""
	m, _ = press(t, m, runes("4"))
	requireContains(t, ansi.Strip(m.View()), "Wallpapers follow a saved profile")
}

func TestProfilesWOpensWallpapersForTheSelectedProfile(t *testing.T) {
	m := wallpaperModel(t)
	m.selectedProfile = 1 // Home
	m, _ = press(t, m, runes("w"))
	if m.tab != tabWallpapers || m.wallpaperProfileName() != "Home" {
		t.Fatalf("tab=%v profile=%q", m.tab, m.wallpaperProfileName())
	}
	m, _ = press(t, m, runes("1"))
	if m.wallpaperOverride != "" {
		t.Fatal("leaving the tab must drop the profile override, as the panel does")
	}
}

func TestEnterOpensPathDialogAndSavesAValidPath(t *testing.T) {
	m := wallpaperModel(t)
	picture := writePicture(t, "lake.JPG")
	m, _ = press(t, m, runes("4"))
	m, _ = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != modeWallpaperPathInput {
		t.Fatalf("mode = %v", m.mode)
	}
	requireContains(t, ansi.Strip(m.View()), "Wallpaper for DP-1")
	m.wallpaperPath.Input.SetValue(picture)
	m, _ = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != modeMain {
		t.Fatalf("dialog stayed open: %v", m.wallpaperPath)
	}
	doc, err := wallpaperStoreOf(m).Load()
	if err != nil || doc.For("Desk", deskKey(m)) != picture {
		t.Fatalf("saved %#v, %v", doc.Profiles, err)
	}
	requireContains(t, ansi.Strip(m.View()), "lake.JPG")
}

func TestPathDialogRejectsAnUnsupportedFile(t *testing.T) {
	m := wallpaperModel(t)
	clip := writePicture(t, "clip.mp4")
	m, _ = press(t, m, runes("4"))
	m, _ = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m.wallpaperPath.Input.SetValue(clip)
	m, _ = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != modeWallpaperPathInput {
		t.Fatal("an unsupported file must keep the dialog open")
	}
	requireContains(t, ansi.Strip(m.View()), "PNG, JPEG or WebP")
	if _, err := os.Stat(wallpaperStoreOf(m).Path()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("nothing may be written: %v", err)
	}
	m, _ = press(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != modeMain || m.wallpaperPath != nil {
		t.Fatal("Esc must close the dialog")
	}
}

func TestDClearsTheSelectedWallpaper(t *testing.T) {
	m := wallpaperModel(t)
	if err := wallpaperStoreOf(m).Save(wallpaper.NewDocument().
		Set("Desk", deskKey(m), "/pics/a.png").
		Set("Home", "Home-0", "/pics/h.png")); err != nil {
		t.Fatal(err)
	}
	m, _ = press(t, m, runes("4"))
	m, _ = press(t, m, runes("d"))
	doc, _ := wallpaperStoreOf(m).Load()
	if doc.Has("Desk") || doc.For("Home", "Home-0") != "/pics/h.png" {
		t.Fatalf("after clear: %#v", doc.Profiles)
	}
	requireContains(t, ansi.Strip(m.View()), "Omarchy background")
}

func TestChooseFillsTheDialogFromOmarchyFileSelect(t *testing.T) {
	m := wallpaperModel(t)
	picture := writePicture(t, "dunes.webp")
	m.chooseFile = func() (string, error) { return picture, nil }
	m, _ = press(t, m, runes("4"))
	m, cmd := press(t, m, runes("c"))
	if cmd == nil {
		t.Fatal("c must start the chooser")
	}
	m, _ = press2(t, m, cmd())
	if m.mode != modeWallpaperPathInput || m.wallpaperPath.Input.Value() != picture {
		t.Fatalf("chooser result must fill the dialog: mode=%v", m.mode)
	}
}

func TestChooserCancelChangesNothing(t *testing.T) {
	m := wallpaperModel(t)
	m.chooseFile = func() (string, error) { return "", nil }
	m, _ = press(t, m, runes("4"))
	_, cmd := press(t, m, runes("c"))
	m, _ = press2(t, m, cmd())
	if m.mode != modeMain || m.statusErr {
		t.Fatalf("a cancelled chooser is a decision, not an error: mode=%v status=%q", m.mode, m.status)
	}
}

func TestChooseIsHiddenWithoutOmarchy(t *testing.T) {
	m := wallpaperModel(t)
	m.hasOmarchy = func() bool { return false }
	m, _ = press(t, m, runes("4"))
	if _, cmd := press(t, m, runes("c")); cmd != nil {
		t.Fatal("c must do nothing without omarchy")
	}
	view := ansi.Strip(m.View())
	if strings.Contains(view, "[Choose…]") || strings.Contains(m.footerHelpText(), "`c`") {
		t.Fatalf("Choose must be hidden without omarchy:\n%s", view)
	}
}

func TestWallpaperButtonsWorkWithThePointer(t *testing.T) {
	m := wallpaperModel(t)
	m.chooseFile = func() (string, error) { return "", nil }
	m, _ = press(t, m, runes("4"))
	x, y := findVisiblePosition(t, m.View(), "[Path…]")
	updated, _ := press2(t, m, mousePressAt(x, y))
	if updated.mode != modeWallpaperPathInput {
		t.Fatal("[Path…] did not open the dialog")
	}
	x, y = findVisiblePosition(t, m.View(), "[Choose…]")
	if _, cmd := press2(t, m, mousePressAt(x, y)); cmd == nil {
		t.Fatal("[Choose…] did not start the chooser")
	}
}

func TestWallpapersFooterAndHelpListTheKeys(t *testing.T) {
	m := wallpaperModel(t)
	m, _ = press(t, m, runes("4"))
	requireContains(t, m.footerHelpText(), "`Enter` path", "`c` choose", "`d` clear")
	m, _ = press(t, m, runes("?"))
	requireContains(t, ansi.Strip(m.View()), "Selected display", "1 2 3 4", "Shown by the Omarchy panel plugin")
}

func TestUnreadableWallpapersFileShowsAnErrorAndBlocksEdits(t *testing.T) {
	m := wallpaperModel(t)
	const newer = `{"version":2,"profiles":{}}`
	if err := os.WriteFile(wallpaperStoreOf(m).Path(), []byte(newer), 0o644); err != nil {
		t.Fatal(err)
	}
	m, _ = press(t, m, runes("4"))
	requireContains(t, ansi.Strip(m.View()), "not readable")
	m, _ = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode == modeWallpaperPathInput {
		t.Fatal("edits must be blocked while the file cannot be read")
	}
	if got, _ := os.ReadFile(wallpaperStoreOf(m).Path()); string(got) != newer {
		t.Fatalf("file changed: %s", got)
	}
}

func press2(t *testing.T, m Model, msg tea.Msg) (Model, tea.Cmd) {
	t.Helper()
	updated, cmd := m.Update(msg)
	switch next := updated.(type) {
	case Model:
		return next, cmd
	case *Model:
		return *next, cmd
	}
	t.Fatalf("update returned %T", updated)
	return m, nil
}

func TestNarrowTabRowKeepsWallpapersAndTheRestartHintApart(t *testing.T) {
	m := wallpaperModel(t)
	m.width, m.height, m.daemonOK, m.daemonVersion = 80, 24, true, "1.13.0"
	tabs := ansi.Strip(m.renderTabs())
	requireContains(t, tabs, "4 Wallpapers", "restart: R")
	x, y := findVisiblePosition(t, m.View(), "Wallpapers")
	if m.restartHintAt(x+len("Wallpapers")-1, y) {
		t.Fatal("the restart hint must not cover the Wallpapers tab")
	}
	updated, _ := press2(t, m, mousePressAt(x, y))
	if updated.tab != tabWallpapers {
		t.Fatal("the tab must stay clickable at 80 columns")
	}
}

func TestNarrowFooterKeepsTheWallpaperKeys(t *testing.T) {
	m := wallpaperModel(t)
	m.width, m.height = 60, 24
	m, _ = press(t, m, runes("4"))
	footer := ansi.Strip(m.footerLayout().text)
	requireContains(t, footer, "Enter path", "c choose", "d clear")
}

func TestProfilesWallpapersButtonOpensTheTab(t *testing.T) {
	for _, width := range []int{80, 120} {
		m := wallpaperModel(t)
		m.width, m.height = width, 30
		m.tab, m.selectedProfile = tabProfiles, 1
		x, y := findVisiblePosition(t, m.View(), "[Wallpapers…]")
		updated, _ := press2(t, m, mousePressAt(x, y))
		if updated.tab != tabWallpapers || updated.wallpaperProfileName() != "Home" {
			t.Fatalf("width %d: tab=%v profile=%q", width, updated.tab, updated.wallpaperProfileName())
		}
	}
}

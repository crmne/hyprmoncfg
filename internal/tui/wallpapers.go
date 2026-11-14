package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/crmne/hyprmoncfg/internal/profile"
	"github.com/crmne/hyprmoncfg/internal/wallpaper"
)

// The Wallpapers tab edits the Omarchy panel plugin's per-display pictures
// for one saved profile: the active one, or the one picked on Profiles with w.
// The plugin draws them; this tab only reads and writes its file.

type wallpaperRow struct {
	Key       string
	Connector string
	Label     string
	Path      string
}

func (m Model) wallpaperStore() *wallpaper.Store {
	return wallpaper.NewStore(m.store.BaseDir())
}

func (m Model) wallpaperProfileName() string {
	if m.wallpaperOverride != "" {
		return m.wallpaperOverride
	}
	return strings.TrimSpace(m.activeProfileName)
}

func (m Model) wallpaperProfile() (profile.Profile, bool) {
	name := m.wallpaperProfileName()
	for _, p := range m.profiles {
		if name != "" && p.Name == name {
			return p, true
		}
	}
	return profile.Profile{}, false
}

// wallpaperRows lists the displays the profile turns on, in profile order. A
// display kept off shows nothing, so it gets no picture.
func (m Model) wallpaperRows() []wallpaperRow {
	p, ok := m.wallpaperProfile()
	if !ok {
		return nil
	}
	rows := make([]wallpaperRow, 0, len(p.Outputs))
	for _, output := range p.Outputs {
		if !output.Enabled {
			continue
		}
		rows = append(rows, wallpaperRow{
			Key:       output.Key,
			Connector: output.Name,
			Label:     strings.TrimSpace(output.Make + " " + output.Model),
			Path:      m.wallpaperDoc.For(p.Name, output.Key),
		})
	}
	return rows
}

func (m Model) selectedWallpaperRow() (wallpaperRow, bool) {
	rows := m.wallpaperRows()
	if len(rows) == 0 {
		return wallpaperRow{}, false
	}
	return rows[clampIndex(m.wallpaperSelected, len(rows))], true
}

// switchTab changes tabs. Leaving Wallpapers drops the profile picked on
// Profiles, as the panel does, so 4 shows the active profile again.
func (m *Model) switchTab(tab mainTab) {
	if tab != tabWallpapers {
		m.wallpaperOverride = ""
	} else if m.tab != tabWallpapers {
		m.wallpaperSelected = 0
		m.reloadWallpapers()
	}
	m.tab = tab
}

func (m *Model) openWallpapersFor(name string) {
	m.switchTab(tabWallpapers)
	m.wallpaperOverride = name
	m.wallpaperSelected = 0
}

func (m *Model) reloadWallpapers() {
	doc, err := m.wallpaperStore().Load()
	if err != nil {
		doc = wallpaper.NewDocument()
	}
	m.wallpaperDoc, m.wallpaperErr = doc, err
}

// wallpaperEditable reports why edits are blocked, if they are. A file this
// version cannot read belongs to someone else's format and is left alone.
func (m *Model) wallpaperEditable() (wallpaperRow, bool) {
	if m.wallpaperErr != nil {
		m.setStatusErr(m.wallpaperErr.Error())
		return wallpaperRow{}, false
	}
	return m.selectedWallpaperRow()
}

func (m *Model) setWallpaper(row wallpaperRow, path string) error {
	name := m.wallpaperProfileName()
	err := m.wallpaperStore().Update(func(doc wallpaper.Document) wallpaper.Document {
		return doc.Set(name, row.Key, path)
	})
	m.reloadWallpapers()
	if err == nil {
		m.setStatusOK("Wallpaper set for " + row.Connector)
	}
	return err
}

func (m *Model) clearWallpaper(row wallpaperRow) {
	name := m.wallpaperProfileName()
	err := m.wallpaperStore().Update(func(doc wallpaper.Document) wallpaper.Document {
		return doc.Clear(name, row.Key)
	})
	m.reloadWallpapers()
	if err != nil {
		m.setStatusErr(err.Error())
		return
	}
	m.setStatusOK(row.Connector + " uses the Omarchy background")
}

func (m Model) updateWallpaperKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		m.wallpaperSelected = clampIndex(m.wallpaperSelected-1, len(m.wallpaperRows()))
	case "down", "j":
		m.wallpaperSelected = clampIndex(m.wallpaperSelected+1, len(m.wallpaperRows()))
	case "enter":
		if row, ok := m.wallpaperEditable(); ok {
			return m, m.openWallpaperPath(row, row.Path)
		}
	case "c":
		if !m.omarchyAvailable() {
			return m, nil
		}
		if _, ok := m.wallpaperEditable(); ok {
			return m, m.chooseWallpaperCmd()
		}
	case "d":
		if row, ok := m.wallpaperEditable(); ok && row.Path != "" {
			m.clearWallpaper(row)
		}
	}
	return m, nil
}

// handleWallpaperChosen fills the path dialog with the chooser's pick, so the
// same validation and save path runs for typed and chosen files.
func (m Model) handleWallpaperChosen(msg wallpaperChosenMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.setStatusErr(msg.err.Error())
		return m, nil
	}
	row, ok := m.selectedWallpaperRow()
	if msg.path == "" || !ok || m.mode != modeMain || m.tab != tabWallpapers {
		return m, nil
	}
	return m, m.openWallpaperPath(row, msg.path)
}

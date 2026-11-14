// Package wallpaper reads and edits the per-profile, per-display wallpapers
// that the Omarchy panel plugin draws. The file belongs to the plugin, which
// watches it; this package mirrors the plugin's WallpaperModel.js so the TUI
// and profile renames edit it without losing anything the plugin wrote.
package wallpaper

// Version is the only file format this package reads or writes.
const Version = 1

// Document maps a profile name to its output keys, and each key to the
// picture's path. Output keys are profile.OutputConfig.Key values, so moving a
// cable keeps each picture on its physical display.
type Document struct {
	Version  int                          `json:"version"`
	Profiles map[string]map[string]string `json:"profiles"`
}

func NewDocument() Document {
	return Document{Version: Version, Profiles: map[string]map[string]string{}}
}

// For returns the picture for one display of a profile, or "" when it keeps
// Omarchy's background.
func (d Document) For(profile, key string) string {
	return d.Profiles[profile][key]
}

// Has reports whether a profile has any wallpaper.
func (d Document) Has(profile string) bool {
	return len(d.Profiles[profile]) > 0
}

func (d Document) Set(profile, key, path string) Document {
	if profile == "" || key == "" || path == "" {
		return d
	}
	next := d.clone()
	entries := next.Profiles[profile]
	if entries == nil {
		entries = map[string]string{}
		next.Profiles[profile] = entries
	}
	entries[key] = path
	return next
}

func (d Document) Clear(profile, key string) Document {
	if d.Profiles[profile][key] == "" {
		return d
	}
	next := d.clone()
	delete(next.Profiles[profile], key)
	if len(next.Profiles[profile]) == 0 {
		delete(next.Profiles, profile)
	}
	return next
}

// Rename moves a profile's wallpapers to its new name, replacing whatever a
// profile deleted elsewhere left under that name, as the plugin does. The
// profile store has already refused to rename onto a live profile.
func (d Document) Rename(from, to string) Document {
	if to == "" || from == to || !d.Has(from) {
		return d
	}
	next := d.clone()
	next.Profiles[to] = next.Profiles[from]
	delete(next.Profiles, from)
	return next
}

func (d Document) Drop(profile string) Document {
	if !d.Has(profile) {
		return d
	}
	next := d.clone()
	delete(next.Profiles, profile)
	return next
}

func (d Document) clone() Document {
	next := NewDocument()
	for profile, entries := range d.Profiles {
		copied := make(map[string]string, len(entries))
		for key, path := range entries {
			copied[key] = path
		}
		next.Profiles[profile] = copied
	}
	return next
}

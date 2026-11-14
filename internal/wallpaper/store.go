package wallpaper

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"github.com/crmne/hyprmoncfg/internal/config"
)

// FileName is the plugin-owned file inside the hyprmoncfg config directory.
const FileName = "wallpapers.json"

// ErrUnreadable means the file exists but is not a format this package
// understands. Writing would destroy what the plugin (or a newer one) stored,
// so callers must leave it alone.
var ErrUnreadable = errors.New("wallpapers file is not readable")

type Store struct {
	path string
}

// NewStore keeps wallpapers in baseDir, the directory config.BaseDir returns.
func NewStore(baseDir string) *Store {
	return &Store{path: filepath.Join(baseDir, FileName)}
}

func (s *Store) Path() string {
	return s.path
}

// Load returns the stored wallpapers. A missing file is an empty document.
func (s *Store) Load() (Document, error) {
	buf, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return NewDocument(), nil
	}
	if err != nil {
		return Document{}, err
	}
	var doc Document
	if err := json.Unmarshal(buf, &doc); err != nil {
		return Document{}, fmt.Errorf("%w: %s: %v", ErrUnreadable, s.path, err)
	}
	if doc.Version != Version {
		return Document{}, fmt.Errorf("%w: %s: version %d", ErrUnreadable, s.path, doc.Version)
	}
	return normalize(doc), nil
}

// Save writes doc atomically, formatted as the plugin writes it.
func (s *Store) Save(doc Document) error {
	doc = normalize(doc)
	buf, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return config.WriteFileAtomic(s.path, append(buf, '\n'), 0o644)
}

// Update reads the current file, applies change and writes the result. It
// writes nothing when the change leaves the document as it was.
func (s *Store) Update(change func(Document) Document) error {
	doc, err := s.Load()
	if err != nil {
		return err
	}
	next := normalize(change(doc))
	if reflect.DeepEqual(next, doc) {
		return nil
	}
	return s.Save(next)
}

// normalize drops empty paths and empty profiles, as WallpaperModel.js does.
func normalize(doc Document) Document {
	next := NewDocument()
	for profile, entries := range doc.Profiles {
		for key, path := range entries {
			next = next.Set(profile, key, path)
		}
	}
	return next
}

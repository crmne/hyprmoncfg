package profile

import (
	"errors"
	"path/filepath"

	"github.com/crmne/hyprmoncfg/internal/wallpaper"
)

// updateWallpapers is (*wallpaper.Store).Update, replaceable so tests can fail
// the wallpaper step of a rename.
var updateWallpapers = (*wallpaper.Store).Update

// Wallpapers are the Omarchy panel plugin's, kept per profile name beside the
// profiles directory. A rename or delete carries them along so every path
// (daemon, CLI, TUI) keeps them consistent with the profile files.
func (s *Store) wallpapers() *wallpaper.Store {
	return wallpaper.NewStore(s.BaseDir())
}

// BaseDir is the hyprmoncfg config directory that holds profiles/.
func (s *Store) BaseDir() string {
	return filepath.Dir(s.dir)
}

// changeWallpapers applies change to the wallpapers file. A file this version
// cannot read is not ours to rewrite, so it is left alone and reported as not
// changed rather than failing the profile operation.
func (s *Store) changeWallpapers(change func(wallpaper.Document) wallpaper.Document) (bool, error) {
	err := updateWallpapers(s.wallpapers(), change)
	if errors.Is(err, wallpaper.ErrUnreadable) {
		return false, nil
	}
	return err == nil, err
}

func (s *Store) renameWallpapers(from, to string) (bool, error) {
	return s.changeWallpapers(func(doc wallpaper.Document) wallpaper.Document { return doc.Rename(from, to) })
}

func (s *Store) dropWallpapers(name string) error {
	_, err := s.changeWallpapers(func(doc wallpaper.Document) wallpaper.Document { return doc.Drop(name) })
	return err
}

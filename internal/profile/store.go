package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/crmne/hyprmoncfg/internal/config"
)

// ErrProfileExists is returned when a rename would take over another
// profile's files.
var ErrProfileExists = errors.New("profile already exists")

// moveFile is os.Rename, replaceable so tests can fail one step of a rename.
var moveFile = os.Rename

type Store struct {
	dir string
}

type FilePaths struct {
	JSON string
	Conf string
	Lua  string
}

func NewStore(baseDir string) *Store {
	return &Store{dir: filepath.Join(baseDir, "profiles")}
}

func (s *Store) Ensure() error {
	return os.MkdirAll(s.dir, 0o755)
}

func (s *Store) List() ([]Profile, error) {
	if err := s.Ensure(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	profiles := make([]Profile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(s.dir, entry.Name())
		p, err := s.loadFromPath(path)
		if err != nil {
			return nil, fmt.Errorf("invalid profile file %s: %w", entry.Name(), err)
		}
		profiles = append(profiles, p)
	}
	sort.Slice(profiles, func(i, j int) bool {
		return profiles[i].Name < profiles[j].Name
	})
	return profiles, nil
}

func (s *Store) Load(name string) (Profile, error) {
	if err := s.Ensure(); err != nil {
		return Profile{}, err
	}
	path := s.PathsForName(name).JSON
	return s.loadFromPath(path)
}

func (s *Store) Save(p Profile) error {
	if err := s.Ensure(); err != nil {
		return err
	}
	p.Normalize()
	if err := p.Validate(); err != nil {
		return err
	}

	path := s.PathsForName(p.Name).JSON
	if existing, err := s.loadFromPath(path); err == nil {
		p.CreatedAt = existing.CreatedAt
	}
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now().UTC()
	}
	p.UpdatedAt = time.Now().UTC()
	p.Normalize()

	buf, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	buf = append(buf, '\n')

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	return nil
}

func (s *Store) Delete(name string) error {
	paths := s.PathsForName(name)
	for _, path := range []string{paths.JSON, paths.Conf, paths.Lua} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// Rename gives a saved profile a new name and returns it. The new JSON appears
// before the old one goes, so a crash leaves a duplicate rather than a lost
// profile, and an error restores what was moved. A name that maps to the same
// files, such as a change of case, is one atomic write. Symlinks stay links, so
// a dotfile manager keeps owning its file under its original name. Rename
// neither applies the profile nor runs its post-apply command.
func (s *Store) Rename(oldName, newName string) (Profile, error) {
	newName = strings.TrimSpace(newName)
	if strings.TrimSpace(oldName) == "" || newName == "" {
		return Profile{}, errors.New("profile name is required")
	}
	p, err := s.Load(oldName)
	if errors.Is(err, os.ErrNotExist) {
		return Profile{}, fmt.Errorf("profile %q not found", oldName)
	}
	if err != nil {
		return Profile{}, err
	}
	if p.Name == newName {
		return p, nil
	}
	from, to := s.PathsForName(oldName), s.PathsForName(newName)
	if from != to {
		for _, path := range []string{to.JSON, to.Conf, to.Lua} {
			if _, err := os.Lstat(path); err == nil {
				return Profile{}, fmt.Errorf("%w: %s", ErrProfileExists, filepath.Base(path))
			} else if !errors.Is(err, os.ErrNotExist) {
				return Profile{}, err
			}
		}
	}

	p.Name = newName
	p.UpdatedAt = time.Now().UTC()
	buf, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return Profile{}, err
	}
	buf = append(buf, '\n')
	if from == to {
		return p, config.WriteFileAtomic(from.JSON, buf, 0o644)
	}
	return p, renameProfileFiles(from, to, buf)
}

func renameProfileFiles(from, to FilePaths, content []byte) (err error) {
	original, err := os.ReadFile(from.JSON)
	if err != nil {
		return err
	}
	target, err := os.Readlink(from.JSON)
	linked := err == nil

	var moved [][2]string
	defer func() {
		if err == nil {
			return
		}
		for _, move := range moved {
			err = errors.Join(err, moveFile(move[1], move[0]))
		}
		if removeErr := os.Remove(to.JSON); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			err = errors.Join(err, removeErr)
		}
		if linked {
			err = errors.Join(err, config.WriteFileAtomic(from.JSON, original, 0o644))
		}
	}()

	if linked {
		if err := os.Symlink(target, to.JSON); err != nil {
			return err
		}
	}
	if err := config.WriteFileAtomic(to.JSON, content, 0o644); err != nil {
		return err
	}
	for _, move := range [][2]string{{from.Conf, to.Conf}, {from.Lua, to.Lua}} {
		if err := moveFile(move[0], move[1]); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return err
		}
		moved = append(moved, move)
	}
	return os.Remove(from.JSON)
}

// SameFiles reports whether two profile names share one set of files.
func (s *Store) SameFiles(a, b string) bool {
	return s.PathsForName(a) == s.PathsForName(b)
}

func (s *Store) PathsForName(name string) FilePaths {
	slug := slugify(name)
	if slug == "" {
		slug = "profile"
	}
	base := filepath.Join(s.dir, slug)
	return FilePaths{
		JSON: base + ".json",
		Conf: base + ".conf",
		Lua:  base + ".lua",
	}
}

func (s *Store) loadFromPath(path string) (Profile, error) {
	buf, err := os.ReadFile(path)
	if err != nil {
		return Profile{}, err
	}
	var p Profile
	if err := json.Unmarshal(buf, &p); err != nil {
		return Profile{}, err
	}
	for i := range p.Outputs {
		if p.Outputs[i].MirrorOf == "none" {
			p.Outputs[i].MirrorOf = ""
		}
	}
	p.Normalize()
	if err := p.Validate(); err != nil {
		return Profile{}, err
	}
	return p, nil
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	prevDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			prevDash = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	return out
}

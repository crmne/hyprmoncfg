package profile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Store struct {
	dir string
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
	path := s.pathForName(name)
	return s.loadFromPath(path)
}

func (s *Store) Save(p Profile) error {
	if err := s.Ensure(); err != nil {
		return err
	}
	if err := p.Validate(); err != nil {
		return err
	}

	path := s.pathForName(p.Name)
	if existing, err := s.loadFromPath(path); err == nil {
		p.CreatedAt = existing.CreatedAt
	}
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now().UTC()
	}
	p.UpdatedAt = time.Now().UTC()
	p.SortOutputs()

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
	path := s.pathForName(name)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *Store) pathForName(name string) string {
	slug := slugify(name)
	if slug == "" {
		slug = "profile"
	}
	return filepath.Join(s.dir, slug+".json")
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

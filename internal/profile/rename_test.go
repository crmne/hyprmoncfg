package profile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func renameFixture(t *testing.T, name string) (*Store, Profile) {
	t.Helper()
	store := NewStore(t.TempDir())
	p := New(name, []OutputConfig{{Key: "dell|a", Name: "DP-1", Enabled: true, Width: 1920, Height: 1080, Refresh: 60, Scale: 1}})
	p.Exec = "/bin/true"
	p.CreatedAt = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := store.Save(p); err != nil {
		t.Fatal(err)
	}
	paths := store.PathsForName(name)
	for path, content := range map[string]string{paths.Conf: "conf", paths.Lua: "lua"} {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	saved, err := store.Load(name)
	if err != nil {
		t.Fatal(err)
	}
	return store, saved
}

func requireMissing(t *testing.T, paths ...string) {
	t.Helper()
	for _, path := range paths {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s still there: %v", path, err)
		}
	}
}

func requireContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("%s = %q, want %q", path, got, want)
	}
}

func TestRenameMovesTheProfileAndItsSidecars(t *testing.T) {
	store, saved := renameFixture(t, "Desk Dock")
	renamed, err := store.Rename("Desk Dock", "  Studio  ")
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Name != "Studio" {
		t.Fatalf("renamed to %q", renamed.Name)
	}
	loaded, err := store.Load("Studio")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Name != "Studio" || loaded.Exec != saved.Exec || len(loaded.Outputs) != 1 || !loaded.CreatedAt.Equal(saved.CreatedAt) {
		t.Fatalf("renamed profile changed: %+v", loaded)
	}
	old, paths := store.PathsForName("Desk Dock"), store.PathsForName("Studio")
	requireMissing(t, old.JSON, old.Conf, old.Lua)
	requireContent(t, paths.Conf, "conf")
	requireContent(t, paths.Lua, "lua")
	profiles, err := store.List()
	if err != nil || len(profiles) != 1 {
		t.Fatalf("listed %d profiles: %v", len(profiles), err)
	}
}

func TestRenameWorksWithoutSidecars(t *testing.T) {
	store, _ := renameFixture(t, "Desk")
	paths := store.PathsForName("Desk")
	if err := os.Remove(paths.Conf); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(paths.Lua); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Rename("Desk", "Studio"); err != nil {
		t.Fatal(err)
	}
}

func TestRenameChangingOnlyCaseKeepsTheFiles(t *testing.T) {
	store, _ := renameFixture(t, "laptop")
	if _, err := store.Rename("laptop", "Laptop"); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load("Laptop")
	if err != nil || loaded.Name != "Laptop" {
		t.Fatalf("loaded %q: %v", loaded.Name, err)
	}
	requireContent(t, store.PathsForName("Laptop").Conf, "conf")
}

func TestRenameRefusesWhatItWouldOverwrite(t *testing.T) {
	store, saved := renameFixture(t, "Desk")
	other := saved
	other.Name, other.Exec = "Work Desk", ""
	if err := store.Save(other); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Work Desk", "work-desk", "WORK  desk"} {
		if _, err := store.Rename("Desk", name); !errors.Is(err, ErrProfileExists) {
			t.Fatalf("rename to %q: %v", name, err)
		}
	}
	// A sidecar with no profile is still somebody's file.
	orphan := store.PathsForName("Studio").Lua
	if err := os.WriteFile(orphan, []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Rename("Desk", "Studio"); !errors.Is(err, ErrProfileExists) {
		t.Fatalf("rename over orphan: %v", err)
	}
	requireContent(t, orphan, "mine")
	if loaded, err := store.Load("Desk"); err != nil || loaded.Name != "Desk" {
		t.Fatalf("source changed: %+v %v", loaded, err)
	}
}

func TestRenameRejectsMissingAndEmptyNames(t *testing.T) {
	store, _ := renameFixture(t, "Desk")
	if _, err := store.Rename("Absent", "Studio"); err == nil {
		t.Fatal("renamed a missing profile")
	}
	if _, err := store.Rename("Desk", "   "); err == nil {
		t.Fatal("renamed to nothing")
	}
}

func TestRenameFailureRestoresEverything(t *testing.T) {
	store, _ := renameFixture(t, "Desk")
	moves := 0
	moveFile = func(from, to string) error {
		moves++
		if moves == 2 {
			return errors.New("disk trouble")
		}
		return os.Rename(from, to)
	}
	t.Cleanup(func() { moveFile = os.Rename })

	if _, err := store.Rename("Desk", "Studio"); err == nil {
		t.Fatal("expected failure")
	}
	paths, target := store.PathsForName("Desk"), store.PathsForName("Studio")
	requireMissing(t, target.JSON, target.Conf, target.Lua)
	requireContent(t, paths.Conf, "conf")
	requireContent(t, paths.Lua, "lua")
	if loaded, err := store.Load("Desk"); err != nil || loaded.Name != "Desk" {
		t.Fatalf("source changed: %+v %v", loaded, err)
	}
}

func TestRenameKeepsDotfileSymlinks(t *testing.T) {
	store, _ := renameFixture(t, "Desk")
	paths := store.PathsForName("Desk")
	dotfiles := t.TempDir()
	link := func(path string) string {
		target := filepath.Join(dotfiles, filepath.Base(path))
		if err := os.Rename(path, target); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, path); err != nil {
			t.Fatal(err)
		}
		return target
	}
	jsonTarget, confTarget := link(paths.JSON), link(paths.Conf)

	if _, err := store.Rename("Desk", "Studio"); err != nil {
		t.Fatal(err)
	}
	renamed := store.PathsForName("Studio")
	for path, target := range map[string]string{renamed.JSON: jsonTarget, renamed.Conf: confTarget} {
		if got, err := os.Readlink(path); err != nil || got != target {
			t.Fatalf("%s links to %q: %v", path, got, err)
		}
	}
	requireMissing(t, paths.JSON, paths.Conf)
	if loaded, err := store.Load("Studio"); err != nil || loaded.Name != "Studio" {
		t.Fatalf("loaded through link: %+v %v", loaded, err)
	}
	if info, err := os.Lstat(jsonTarget); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("dotfile target: %v %v", info, err)
	}
}

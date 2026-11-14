package profile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/crmne/hyprmoncfg/internal/wallpaper"
)

// wallpaperBase is the config directory that holds the store's profiles/.
func wallpaperBase(store *Store) string {
	return filepath.Dir(store.dir)
}

func seedWallpapers(t *testing.T, store *Store, doc wallpaper.Document) *wallpaper.Store {
	t.Helper()
	walls := wallpaper.NewStore(wallpaperBase(store))
	if err := walls.Save(doc); err != nil {
		t.Fatal(err)
	}
	return walls
}

func loadWallpapers(t *testing.T, walls *wallpaper.Store) wallpaper.Document {
	t.Helper()
	doc, err := walls.Load()
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestRenameMovesWallpapers(t *testing.T) {
	store, _ := renameFixture(t, "Desk Dock")
	walls := seedWallpapers(t, store, wallpaper.NewDocument().
		Set("Desk Dock", "dell|a", "/w/a.png").
		Set("Home", "dell|a", "/w/h.png"))

	if _, err := store.Rename("desk dock", "Studio"); err != nil {
		t.Fatal(err)
	}
	doc := loadWallpapers(t, walls)
	if doc.For("Studio", "dell|a") != "/w/a.png" || doc.Has("Desk Dock") {
		t.Fatalf("wallpapers did not follow the rename: %#v", doc.Profiles)
	}
	if doc.For("Home", "dell|a") != "/w/h.png" {
		t.Fatalf("other profiles must be untouched: %#v", doc.Profiles)
	}
}

func TestRenameCaseOnlyMovesWallpapers(t *testing.T) {
	store, _ := renameFixture(t, "desk")
	walls := seedWallpapers(t, store, wallpaper.NewDocument().Set("desk", "dell|a", "/w/a.png"))

	if _, err := store.Rename("desk", "Desk"); err != nil {
		t.Fatal(err)
	}
	doc := loadWallpapers(t, walls)
	if doc.For("Desk", "dell|a") != "/w/a.png" || doc.Has("desk") {
		t.Fatalf("case-only rename must move wallpapers: %#v", doc.Profiles)
	}
}

func TestRenameWithoutWallpapersLeavesNoFile(t *testing.T) {
	store, _ := renameFixture(t, "Desk")
	if _, err := store.Rename("Desk", "Studio"); err != nil {
		t.Fatal(err)
	}
	requireMissing(t, filepath.Join(wallpaperBase(store), wallpaper.FileName))
}

func TestRenameRollsBackWhenWallpaperWriteFails(t *testing.T) {
	store, _ := renameFixture(t, "Desk")
	walls := seedWallpapers(t, store, wallpaper.NewDocument().Set("Desk", "dell|a", "/w/a.png"))
	updateWallpapers = func(*wallpaper.Store, func(wallpaper.Document) wallpaper.Document) error {
		return errors.New("disk trouble")
	}
	t.Cleanup(func() { updateWallpapers = (*wallpaper.Store).Update })

	if _, err := store.Rename("Desk", "Studio"); err == nil {
		t.Fatal("expected failure")
	}
	paths, target := store.PathsForName("Desk"), store.PathsForName("Studio")
	requireMissing(t, target.JSON, target.Conf, target.Lua)
	requireContent(t, paths.Conf, "conf")
	if loaded, err := store.Load("Desk"); err != nil || loaded.Name != "Desk" {
		t.Fatalf("profile changed: %+v %v", loaded, err)
	}
	if doc := loadWallpapers(t, walls); doc.For("Desk", "dell|a") != "/w/a.png" {
		t.Fatalf("wallpapers changed: %#v", doc.Profiles)
	}
}

func TestRenameFailureRestoresWallpapers(t *testing.T) {
	store, _ := renameFixture(t, "Desk")
	walls := seedWallpapers(t, store, wallpaper.NewDocument().Set("Desk", "dell|a", "/w/a.png"))
	moveFile = func(string, string) error { return errors.New("disk trouble") }
	t.Cleanup(func() { moveFile = os.Rename })

	if _, err := store.Rename("Desk", "Studio"); err == nil {
		t.Fatal("expected failure")
	}
	doc := loadWallpapers(t, walls)
	if doc.For("Desk", "dell|a") != "/w/a.png" || doc.Has("Studio") {
		t.Fatalf("wallpapers must be restored when the rename fails: %#v", doc.Profiles)
	}
}

func TestRenameWithUnreadableWallpapersStillRenamesAndLeavesTheFile(t *testing.T) {
	store, _ := renameFixture(t, "Desk")
	path := filepath.Join(wallpaperBase(store), wallpaper.FileName)
	const newer = `{"version":2,"profiles":{"Desk":{"dell|a":"/w/a.png"}}}`
	if err := os.WriteFile(path, []byte(newer), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Rename("Desk", "Studio"); err != nil {
		t.Fatalf("an unreadable wallpapers file must not block a rename: %v", err)
	}
	requireContent(t, path, newer)
}

func TestDeleteDropsWallpapers(t *testing.T) {
	store, _ := renameFixture(t, "Desk")
	walls := seedWallpapers(t, store, wallpaper.NewDocument().
		Set("Desk", "dell|a", "/w/a.png").
		Set("Home", "dell|a", "/w/h.png"))

	if err := store.Delete("desk"); err != nil {
		t.Fatal(err)
	}
	doc := loadWallpapers(t, walls)
	if doc.Has("Desk") || doc.For("Home", "dell|a") != "/w/h.png" {
		t.Fatalf("delete must drop only that profile's wallpapers: %#v", doc.Profiles)
	}
}

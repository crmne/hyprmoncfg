package wallpaper

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// pluginFile is what the panel's WallpaperStore.qml writes.
const pluginFile = `{
  "version": 1,
  "profiles": {
    "Desk": {
      "DEL|U2720Q|ABC": "/home/me/Pictures/a.png"
    }
  }
}
`

func writeFile(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, FileName)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestStoreRoundTripsThePluginFormat(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, pluginFile)
	store := NewStore(dir)

	doc, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if doc.For("Desk", "DEL|U2720Q|ABC") != "/home/me/Pictures/a.png" {
		t.Fatalf("Load = %#v", doc)
	}
	if err := store.Save(doc); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != pluginFile {
		t.Fatalf("Save wrote\n%s\nwant\n%s", got, pluginFile)
	}
}

func TestStoreDropsEmptyEntriesLikeThePlugin(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, `{"version":1,"profiles":{"Desk":{"k":""},"Home":{"k":"/h.png"}}}`)
	doc, err := NewStore(dir).Load()
	if err != nil {
		t.Fatal(err)
	}
	if doc.Has("Desk") || doc.For("Home", "k") != "/h.png" {
		t.Fatalf("Load = %#v", doc.Profiles)
	}
}

func TestStoreRefusesToOverwriteUnknownOrCorruptFiles(t *testing.T) {
	for name, content := range map[string]string{
		"newer version": `{"version":2,"profiles":{}}`,
		"invalid json":  `{"version":1,`,
		"wrong shape":   `{"version":1,"profiles":{"Desk":{"k":3}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := writeFile(t, dir, content)
			store := NewStore(dir)
			if _, err := store.Load(); !errors.Is(err, ErrUnreadable) {
				t.Fatalf("Load error = %v, want ErrUnreadable", err)
			}
			if err := store.Update(func(d Document) Document { return d.Set("Desk", "k", "/a.png") }); !errors.Is(err, ErrUnreadable) {
				t.Fatalf("Update error = %v, want ErrUnreadable", err)
			}
			if got, _ := os.ReadFile(path); string(got) != content {
				t.Fatalf("file changed to %s", got)
			}
		})
	}
}

func TestStoreMissingFileIsEmpty(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	doc, err := store.Load()
	if err != nil || len(doc.Profiles) != 0 || doc.Version != Version {
		t.Fatalf("Load = %#v, %v", doc, err)
	}
	if err := store.Update(func(d Document) Document { return d.Set("Desk", "k", "/a.png") }); err != nil {
		t.Fatal(err)
	}
	doc, _ = store.Load()
	if doc.For("Desk", "k") != "/a.png" {
		t.Fatalf("Update did not persist: %#v", doc)
	}
}

func TestStoreUpdateThatChangesNothingDoesNotCreateTheFile(t *testing.T) {
	dir := t.TempDir()
	if err := NewStore(dir).Update(func(d Document) Document { return d.Rename("A", "B") }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, FileName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a no-op update must not write: %v", err)
	}
}

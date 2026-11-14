package wallpaper

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestExpandAndValidateImagePath(t *testing.T) {
	home := t.TempDir()
	for _, name := range []string{"a.PNG", "b.jpeg", "c.webp", "clip.mp4"} {
		if err := os.WriteFile(filepath.Join(home, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(home, "dir.png"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := resolveImage("  ~/a.PNG ", home)
	if err != nil || got != filepath.Join(home, "a.PNG") {
		t.Fatalf("resolveImage(~/a.PNG) = %q, %v", got, err)
	}
	if got, err := resolveImage(filepath.Join(home, "b.jpeg"), home); err != nil || got != filepath.Join(home, "b.jpeg") {
		t.Fatalf("absolute path = %q, %v", got, err)
	}
	if _, err := resolveImage("~/c.webp", home); err != nil {
		t.Fatalf("webp must be accepted: %v", err)
	}

	for input, want := range map[string]error{
		"":               ErrNoPath,
		"~/clip.mp4":     ErrUnsupportedImage,
		"~/missing.png":  os.ErrNotExist,
		"~/dir.png":      ErrNotAFile,
		"relative/a.png": ErrNotAbsolute,
	} {
		if _, err := resolveImage(input, home); !errors.Is(err, want) {
			t.Errorf("resolveImage(%q) error = %v, want %v", input, err, want)
		}
	}
}

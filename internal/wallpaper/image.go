package wallpaper

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var (
	ErrNoPath           = errors.New("enter a path to a picture")
	ErrNotAbsolute      = errors.New("use a full path, or one starting with ~/")
	ErrNotAFile         = errors.New("not a file")
	ErrUnsupportedImage = errors.New("use a PNG, JPEG or WebP picture")
)

// ResolveImage turns what the user typed into the absolute path the plugin
// can draw: ~ expanded, an existing regular file, and a still image the
// plugin's layer supports (WallpaperModel.isSupportedImage).
func ResolveImage(input string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return resolveImage(input, home)
}

func resolveImage(input, home string) (string, error) {
	path := strings.TrimSpace(input)
	if path == "" {
		return "", ErrNoPath
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		path = filepath.Join(home, strings.TrimPrefix(path, "~"))
	}
	if !filepath.IsAbs(path) {
		return "", ErrNotAbsolute
	}
	path = filepath.Clean(path)
	if !IsSupportedImage(path) {
		return "", ErrUnsupportedImage
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s: %w", path, ErrNotAFile)
	}
	return path, nil
}

// IsSupportedImage matches the plugin's extension check.
func IsSupportedImage(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".webp":
		return true
	}
	return false
}

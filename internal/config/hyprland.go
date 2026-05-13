package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type FileSnapshot struct {
	Path    string
	Exists  bool
	Content []byte
}

type HyprConfigFormat int

const (
	HyprConfigLegacy HyprConfigFormat = iota
	HyprConfigLua
)

type ResolvedHyprConfig struct {
	Format       HyprConfigFormat
	RootPath     string
	MonitorsPath string
}

func HyprlandDir() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "hypr"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if home == "" {
		return "", errors.New("unable to resolve home directory")
	}
	return filepath.Join(home, ".config", "hypr"), nil
}

func HyprlandMonitorsConfPath() (string, error) {
	dir, err := HyprlandDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "monitors.conf"), nil
}

func HyprlandMonitorsLuaPath() (string, error) {
	dir, err := HyprlandDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "monitors.lua"), nil
}

func HyprlandMainConfigPath() (string, error) {
	dir, err := HyprlandDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "hyprland.conf"), nil
}

func HyprlandLuaConfigPath() (string, error) {
	dir, err := HyprlandDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "hyprland.lua"), nil
}

func ResolveMonitorsConfPath(explicit string) (string, error) {
	if strings.TrimSpace(explicit) == "" {
		return HyprlandMonitorsConfPath()
	}
	return resolvePath(explicit, "")
}

func ResolveHyprlandConfigPath(explicit string) (string, error) {
	if strings.TrimSpace(explicit) == "" {
		return HyprlandMainConfigPath()
	}
	return resolvePath(explicit, "")
}

func ResolveHyprlandConfig(version string, explicitMonitorsPath string, explicitRootPath string) (ResolvedHyprConfig, error) {
	format, rootPath, err := resolveHyprConfigRoot(version, explicitRootPath)
	if err != nil {
		return ResolvedHyprConfig{}, err
	}

	monitorsPath, err := resolveHyprConfigMonitors(format, explicitMonitorsPath)
	if err != nil {
		return ResolvedHyprConfig{}, err
	}

	return ResolvedHyprConfig{
		Format:       format,
		RootPath:     rootPath,
		MonitorsPath: monitorsPath,
	}, nil
}

func resolveHyprConfigRoot(version string, explicit string) (HyprConfigFormat, string, error) {
	if strings.TrimSpace(explicit) != "" {
		rootPath, err := resolvePath(explicit, "")
		if err != nil {
			return HyprConfigLegacy, "", err
		}
		switch strings.ToLower(filepath.Ext(rootPath)) {
		case ".lua":
			return HyprConfigLua, rootPath, nil
		case ".conf":
			return HyprConfigLegacy, rootPath, nil
		default:
			return HyprConfigLegacy, "", fmt.Errorf("cannot infer Hyprland config format from %s", rootPath)
		}
	}

	if versionAtLeast(version, 0, 55, 0) {
		luaPath, err := HyprlandLuaConfigPath()
		if err != nil {
			return HyprConfigLegacy, "", err
		}
		if _, err := os.Stat(luaPath); err == nil {
			return HyprConfigLua, luaPath, nil
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return HyprConfigLegacy, "", err
		}
	}

	rootPath, err := HyprlandMainConfigPath()
	return HyprConfigLegacy, rootPath, err
}

func resolveHyprConfigMonitors(format HyprConfigFormat, explicit string) (string, error) {
	if strings.TrimSpace(explicit) != "" {
		return resolvePath(explicit, "")
	}
	if format == HyprConfigLua {
		return HyprlandMonitorsLuaPath()
	}
	return HyprlandMonitorsConfPath()
}

func VerifyIncludeChain(format HyprConfigFormat, rootConfigPath string, targetPath string) error {
	if format == HyprConfigLua {
		return VerifyLuaIncludeChain(rootConfigPath, targetPath)
	}
	return VerifySourceChain(rootConfigPath, targetPath)
}

func resolvePath(value string, baseDir string) (string, error) {
	value = strings.TrimSpace(strings.Trim(value, `"'`))
	if value == "" {
		return "", errors.New("path is empty")
	}

	expanded, err := expandHome(value)
	if err != nil {
		return "", err
	}
	expanded = os.ExpandEnv(expanded)
	if !filepath.IsAbs(expanded) {
		if baseDir != "" {
			expanded = filepath.Join(baseDir, expanded)
		} else {
			expanded, err = filepath.Abs(expanded)
			if err != nil {
				return "", err
			}
		}
	}
	return filepath.Clean(expanded), nil
}

func expandHome(value string) (string, error) {
	if value == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return home, nil
	}
	if !strings.HasPrefix(value, "~/") {
		return value, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, value[2:]), nil
}

func hasGlob(value string) bool {
	return strings.ContainsAny(value, "*?[")
}

func versionAtLeast(value string, major int, minor int, patch int) bool {
	parts := strings.Split(strings.TrimPrefix(strings.TrimSpace(value), "v"), ".")
	got := []int{0, 0, 0}
	for i := 0; i < len(parts) && i < len(got); i++ {
		part := parts[i]
		for j, r := range part {
			if r < '0' || r > '9' {
				part = part[:j]
				break
			}
		}
		if part == "" {
			continue
		}
		parsed, err := strconv.Atoi(part)
		if err != nil {
			continue
		}
		got[i] = parsed
	}
	want := []int{major, minor, patch}
	for i := range want {
		if got[i] > want[i] {
			return true
		}
		if got[i] < want[i] {
			return false
		}
	}
	return true
}

func SnapshotFile(path string) (FileSnapshot, error) {
	content, err := os.ReadFile(path)
	if err == nil {
		return FileSnapshot{
			Path:    path,
			Exists:  true,
			Content: content,
		}, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return FileSnapshot{Path: path}, nil
	}
	return FileSnapshot{}, err
}

func (s FileSnapshot) Restore() error {
	if s.Path == "" {
		return nil
	}
	if s.Exists {
		return WriteFileAtomic(s.Path, s.Content, 0o644)
	}
	if err := os.Remove(s.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func WriteFileAtomic(path string, content []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), ".hyprmoncfg-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

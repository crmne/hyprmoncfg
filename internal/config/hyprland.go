package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type FileSnapshot struct {
	Path    string
	Exists  bool
	Content []byte
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

func HyprlandMainConfigPath() (string, error) {
	dir, err := HyprlandDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "hyprland.conf"), nil
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

func VerifySourceChain(rootConfigPath string, targetPath string) error {
	rootConfigPath, err := resolvePath(rootConfigPath, "")
	if err != nil {
		return err
	}
	targetPath, err = resolvePath(targetPath, "")
	if err != nil {
		return err
	}

	ok, err := isPathSourced(rootConfigPath, targetPath, map[string]bool{})
	if err != nil {
		return err
	}
	if ok {
		return nil
	}

	return fmt.Errorf("%s is not sourced by %s; add `source = %s` to your Hyprland config or pass a different --monitors-conf target", targetPath, rootConfigPath, targetPath)
}

func isPathSourced(rootConfigPath string, targetPath string, visited map[string]bool) (bool, error) {
	rootConfigPath = filepath.Clean(rootConfigPath)
	targetPath = filepath.Clean(targetPath)
	if rootConfigPath == targetPath {
		return true, nil
	}
	if visited[rootConfigPath] {
		return false, nil
	}
	visited[rootConfigPath] = true

	content, err := os.ReadFile(rootConfigPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, fmt.Errorf("Hyprland config %s does not exist", rootConfigPath)
		}
		return false, err
	}

	for _, sourceValue := range parseSourceLines(string(content)) {
		sourcePaths, err := expandSourceValue(sourceValue, filepath.Dir(rootConfigPath))
		if err != nil {
			return false, err
		}
		for _, sourcePath := range sourcePaths {
			if sourcePath == targetPath {
				return true, nil
			}

			info, err := os.Stat(sourcePath)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					continue
				}
				return false, err
			}
			if info.IsDir() {
				continue
			}
			ok, err := isPathSourced(sourcePath, targetPath, visited)
			if err != nil {
				return false, err
			}
			if ok {
				return true, nil
			}
		}
	}
	return false, nil
}

func parseSourceLines(content string) []string {
	scanner := bufio.NewScanner(strings.NewReader(content))
	out := make([]string, 0)
	for scanner.Scan() {
		line := stripComments(scanner.Text())
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		if strings.TrimSpace(parts[0]) != "source" {
			continue
		}
		value := strings.TrimSpace(parts[1])
		value = strings.Trim(value, `"'`)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func stripComments(line string) string {
	if idx := strings.Index(line, "#"); idx >= 0 {
		return line[:idx]
	}
	return line
}

func expandSourceValue(value string, baseDir string) ([]string, error) {
	resolved, err := resolvePath(value, baseDir)
	if err != nil {
		return nil, err
	}
	if hasGlob(resolved) {
		matches, err := filepath.Glob(resolved)
		if err != nil {
			return nil, err
		}
		if len(matches) == 0 {
			return []string{resolved}, nil
		}
		return matches, nil
	}
	return []string{resolved}, nil
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

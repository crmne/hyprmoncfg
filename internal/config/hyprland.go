package config

import (
	"bufio"
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

func VerifyLuaIncludeChain(rootConfigPath string, targetPath string) error {
	rootConfigPath, err := resolvePath(rootConfigPath, "")
	if err != nil {
		return err
	}
	targetPath, err = resolvePath(targetPath, "")
	if err != nil {
		return err
	}

	ok, err := isLuaPathIncluded(rootConfigPath, targetPath, map[string]bool{})
	if err != nil {
		return err
	}
	if ok {
		return nil
	}

	return fmt.Errorf("%s is not included by %s; add `dofile(%q)` to your Hyprland Lua config or pass a different --monitors-conf target", targetPath, rootConfigPath, targetPath)
}

func isLuaPathIncluded(rootConfigPath string, targetPath string, visited map[string]bool) (bool, error) {
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

	for _, includeValue := range parseLuaIncludeCalls(string(content)) {
		includePath, err := resolvePath(includeValue, filepath.Dir(rootConfigPath))
		if err != nil {
			return false, err
		}
		if includePath == targetPath {
			return true, nil
		}
		info, err := os.Stat(includePath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return false, err
		}
		if info.IsDir() {
			continue
		}
		ok, err := isLuaPathIncluded(includePath, targetPath, visited)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

func parseLuaIncludeCalls(content string) []string {
	scanner := bufio.NewScanner(strings.NewReader(content))
	out := make([]string, 0)
	for scanner.Scan() {
		line := stripLuaComments(scanner.Text())
		out = append(out, parseLuaLiteralCall(line, "dofile")...)
		out = append(out, parseLuaLiteralCall(line, "source")...)
	}
	return out
}

func stripLuaComments(line string) string {
	if idx := strings.Index(line, "--"); idx >= 0 {
		return line[:idx]
	}
	return line
}

func parseLuaLiteralCall(line string, name string) []string {
	line = strings.TrimSpace(line)
	out := make([]string, 0, 1)
	for {
		idx := strings.Index(line, name)
		if idx < 0 {
			return out
		}
		beforeOK := idx == 0 || !isLuaIdentifierByte(line[idx-1])
		line = line[idx+len(name):]
		if !beforeOK {
			continue
		}
		line = strings.TrimLeft(line, " \t")
		if !strings.HasPrefix(line, "(") {
			continue
		}
		line = strings.TrimLeft(line[1:], " \t")
		if line == "" || (line[0] != '\'' && line[0] != '"') {
			continue
		}
		quote := line[0]
		line = line[1:]
		end := strings.IndexByte(line, quote)
		if end < 0 {
			continue
		}
		value := line[:end]
		line = line[end+1:]
		if strings.TrimSpace(value) != "" {
			out = append(out, value)
		}
	}
}

func isLuaIdentifierByte(b byte) bool {
	return b == '_' || b >= '0' && b <= '9' || b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z'
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

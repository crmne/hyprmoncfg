package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

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

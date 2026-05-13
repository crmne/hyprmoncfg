package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

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

	return fmt.Errorf("%s is not included by %s; add `%s` to your Hyprland Lua config or pass a different --monitors-conf target", targetPath, rootConfigPath, luaIncludeSuggestion(rootConfigPath, targetPath))
}

func luaIncludeSuggestion(rootConfigPath string, targetPath string) string {
	rootDir := filepath.Dir(rootConfigPath)
	targetDir := filepath.Dir(targetPath)
	moduleName := strings.TrimSuffix(filepath.Base(targetPath), filepath.Ext(targetPath))
	if targetDir == rootDir {
		return "require(" + quoteLuaArg(moduleName) + ")"
	}
	return "dofile(" + quoteLuaArg(targetPath) + ")"
}

func quoteLuaArg(value string) string {
	return fmt.Sprintf("%q", value)
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
		includePaths, err := resolveLuaIncludePaths(includeValue, filepath.Dir(rootConfigPath))
		if err != nil {
			return false, err
		}
		for _, includePath := range includePaths {
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
	}
	return false, nil
}

func resolveLuaIncludePaths(value string, baseDir string) ([]string, error) {
	if strings.HasSuffix(value, ".lua") || strings.Contains(value, "/") {
		resolved, err := resolvePath(value, baseDir)
		if err != nil {
			return nil, err
		}
		return []string{resolved}, nil
	}

	modulePath := strings.ReplaceAll(value, ".", string(os.PathSeparator)) + ".lua"
	candidates := []string{filepath.Join(baseDir, modulePath)}
	parent := filepath.Dir(baseDir)
	if parent != baseDir {
		candidates = append(candidates, filepath.Join(parent, modulePath))
	}
	for i := range candidates {
		candidates[i] = filepath.Clean(candidates[i])
	}
	return candidates, nil
}

func parseLuaIncludeCalls(content string) []string {
	scanner := bufio.NewScanner(strings.NewReader(content))
	out := make([]string, 0)
	for scanner.Scan() {
		line := stripLuaComments(scanner.Text())
		out = append(out, parseLuaLiteralCall(line, "require")...)
		out = append(out, parseLuaLiteralCall(line, "dofile")...)
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
		if strings.HasPrefix(line, "(") {
			line = strings.TrimLeft(line[1:], " \t")
		}
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

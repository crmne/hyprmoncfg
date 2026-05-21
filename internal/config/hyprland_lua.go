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
	if targetDir == rootDir {
		moduleName := strings.TrimSuffix(filepath.Base(targetPath), filepath.Ext(targetPath))
		return "require(" + quoteLuaArg(moduleName) + ")"
	}
	if rel, err := filepath.Rel(rootDir, targetPath); err == nil && !isParentRelativePath(rel) {
		moduleName := strings.TrimSuffix(rel, filepath.Ext(rel))
		return "require(" + quoteLuaArg(filepath.ToSlash(moduleName)) + ")"
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

	contentString := string(content)
	for _, include := range parseLuaIncludeCalls(contentString) {
		includePaths, err := resolveLuaIncludePaths(include, filepath.Dir(rootConfigPath), contentString)
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

type luaIncludeKind int

const (
	luaIncludeRequire luaIncludeKind = iota
	luaIncludeDofile
)

type luaIncludeCall struct {
	Kind  luaIncludeKind
	Value string
}

func resolveLuaIncludePaths(include luaIncludeCall, baseDir string, content string) ([]string, error) {
	if include.Kind == luaIncludeDofile {
		resolved, err := resolvePath(include.Value, baseDir)
		if err != nil {
			return nil, err
		}
		return []string{resolved}, nil
	}

	modulePath := luaModulePath(include.Value)
	candidates := []string{filepath.Join(baseDir, modulePath)}
	moduleName := strings.TrimSuffix(modulePath, ".lua")
	for _, pattern := range parseLuaPackagePathPatterns(content) {
		if strings.Contains(pattern, "?") {
			candidates = append(candidates, strings.ReplaceAll(pattern, "?", moduleName))
		}
	}

	seen := make(map[string]bool, len(candidates))
	out := make([]string, 0, len(candidates))
	for i := range candidates {
		candidate := filepath.Clean(candidates[i])
		if !seen[candidate] {
			seen[candidate] = true
			out = append(out, candidate)
		}
	}
	return out, nil
}

func luaModulePath(value string) string {
	value = strings.TrimSuffix(value, ".lua")
	separators := func(r rune) bool { return r == '.' }
	if strings.Contains(value, "/") || strings.Contains(value, "\\") {
		separators = func(r rune) bool { return r == '/' || r == '\\' }
	}
	parts := strings.FieldsFunc(value, separators)
	return filepath.Join(append(parts[:0:0], parts...)...) + ".lua"
}

func parseLuaPackagePathPatterns(content string) []string {
	if !strings.Contains(content, "package.path") {
		return nil
	}
	patterns := make([]string, 0)
	for _, literal := range parseLuaStringLiterals(content) {
		if !strings.Contains(literal, "?") || !strings.Contains(literal, ".lua") {
			continue
		}
		patterns = append(patterns, expandLuaPackagePathLiteral(literal)...)
	}
	patterns = append(patterns, parseLuaPackagePathEnvPatterns(content)...)
	return patterns
}

func parseLuaPackagePathEnvPatterns(content string) []string {
	out := make([]string, 0)
	activeEnv := ""
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := stripLuaComments(scanner.Text())
		if strings.TrimSpace(line) == "" {
			continue
		}
		if envName, ok := parseLuaGetenvName(line); ok {
			activeEnv = envName
		}
		if activeEnv == "" {
			continue
		}
		base := strings.TrimSpace(os.Getenv(activeEnv))
		if base == "" {
			continue
		}
		for _, literal := range parseLuaStringLiterals(line) {
			for _, part := range strings.Split(literal, ";") {
				part = strings.TrimSpace(part)
				if !strings.Contains(part, "?") || !strings.Contains(part, ".lua") || !strings.HasPrefix(part, "/") {
					continue
				}
				out = append(out, filepath.Join(base, strings.TrimPrefix(part, "/")))
			}
		}
	}
	return out
}

func parseLuaGetenvName(line string) (string, bool) {
	idx := strings.Index(line, "os.getenv")
	if idx < 0 {
		return "", false
	}
	line = strings.TrimLeft(line[idx+len("os.getenv"):], " \t")
	if !strings.HasPrefix(line, "(") {
		return "", false
	}
	line = strings.TrimLeft(line[1:], " \t")
	if line == "" || (line[0] != '\'' && line[0] != '"') {
		return "", false
	}
	quote := line[0]
	line = line[1:]
	end := strings.IndexByte(line, quote)
	if end < 0 {
		return "", false
	}
	return line[:end], true
}

func expandLuaPackagePathLiteral(value string) []string {
	parts := strings.Split(value, ";")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		switch {
		case strings.HasPrefix(part, "/.config/"):
			if home := strings.TrimSpace(os.Getenv("HOME")); home != "" {
				out = append(out, filepath.Join(home, strings.TrimPrefix(part, "/")))
			}
		case strings.HasPrefix(part, "/") || strings.HasPrefix(part, "~"):
			resolved, err := resolvePath(part, "")
			if err == nil {
				out = append(out, resolved)
			}
		case strings.HasPrefix(part, ".config/"):
			if home := strings.TrimSpace(os.Getenv("HOME")); home != "" {
				out = append(out, filepath.Join(home, part))
			}
		default:
			if strings.HasPrefix(part, ".local/share/omarchy/") {
				if omarchyPath := strings.TrimSpace(os.Getenv("OMARCHY_PATH")); omarchyPath != "" {
					out = append(out, filepath.Join(omarchyPath, strings.TrimPrefix(part, ".local/share/omarchy/")))
				} else if home := strings.TrimSpace(os.Getenv("HOME")); home != "" {
					out = append(out, filepath.Join(home, part))
				}
			}
		}
	}
	return out
}

func parseLuaStringLiterals(content string) []string {
	out := make([]string, 0)
	for i := 0; i < len(content); i++ {
		if content[i] != '\'' && content[i] != '"' {
			continue
		}
		quote := content[i]
		start := i + 1
		for i = start; i < len(content); i++ {
			if content[i] == '\\' {
				i++
				continue
			}
			if content[i] == quote {
				out = append(out, content[start:i])
				break
			}
		}
	}
	return out
}

func parseLuaIncludeCalls(content string) []luaIncludeCall {
	scanner := bufio.NewScanner(strings.NewReader(content))
	out := make([]luaIncludeCall, 0)
	for scanner.Scan() {
		line := stripLuaComments(scanner.Text())
		for _, value := range parseLuaLiteralCall(line, "require") {
			out = append(out, luaIncludeCall{Kind: luaIncludeRequire, Value: value})
		}
		for _, value := range parseLuaLiteralCall(line, "dofile") {
			out = append(out, luaIncludeCall{Kind: luaIncludeDofile, Value: value})
		}
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

func isParentRelativePath(value string) bool {
	return value == ".." || strings.HasPrefix(value, ".."+string(os.PathSeparator))
}

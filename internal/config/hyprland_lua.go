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
	scanner := bufio.NewScanner(strings.NewReader(content))
	inPackagePath := false
	pendingPackagePathContinuation := false
	activePackagePathEnv := ""
	longCommentClose := ""
	for scanner.Scan() {
		line := strings.TrimSpace(stripLuaCommentsState(scanner.Text(), &longCommentClose))
		if line == "" {
			inPackagePath = false
			pendingPackagePathContinuation = false
			activePackagePathEnv = ""
			continue
		}

		assignmentLine := false
		leadingConcatLine := strings.HasPrefix(line, "..")
		if rhs, ok := parseLuaPackagePathAssignmentRHS(line); ok {
			assignmentLine = true
			inPackagePath = true
			pendingPackagePathContinuation = true
			activePackagePathEnv = ""
			line = rhs
		} else if pendingPackagePathContinuation && leadingConcatLine {
			inPackagePath = true
		} else if !inPackagePath {
			continue
		} else if !leadingConcatLine && pendingPackagePathContinuation {
			inPackagePath = false
			pendingPackagePathContinuation = false
			activePackagePathEnv = ""
			continue
		}

		if envName, ok := parseLuaGetenvName(line); ok {
			activePackagePathEnv = envName
		}
		for _, literal := range parseLuaStringLiterals(line) {
			if !strings.Contains(literal, "?") || !strings.Contains(literal, ".lua") {
				continue
			}
			patterns = append(patterns, expandLuaPackagePathLiteral(literal)...)
			patterns = append(patterns, expandLuaPackagePathEnvLiteral(literal, activePackagePathEnv)...)
		}

		pendingPackagePathContinuation = assignmentLine || leadingConcatLine || isLuaPackagePathContinuation(line)
		if !pendingPackagePathContinuation {
			inPackagePath = false
			activePackagePathEnv = ""
		}
	}
	return patterns
}

func parseLuaPackagePathAssignmentRHS(line string) (string, bool) {
	line = strings.TrimSpace(line)
	for strings.HasPrefix(line, ";") {
		line = strings.TrimSpace(line[1:])
	}
	const lhs = "package.path"
	if !strings.HasPrefix(line, lhs) {
		return "", false
	}
	if len(line) > len(lhs) && isLuaIdentifierByte(line[len(lhs)]) {
		return "", false
	}
	line = strings.TrimLeft(line[len(lhs):], " \f\n\r\t\v")
	if line == "" || line[0] != '=' {
		return "", false
	}
	if len(line) > 1 && (line[1] == '=' || line[1] == '~' || line[1] == '<' || line[1] == '>') {
		return "", false
	}
	return strings.TrimSpace(line[1:]), true
}

func isLuaPackagePathContinuation(line string) bool {
	if idx := strings.Index(line, "="); idx >= 0 {
		rhs := strings.TrimSpace(line[idx+1:])
		if rhs == "package.path" {
			return false
		}
	}
	return strings.HasSuffix(strings.TrimSpace(line), "..")
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
		}
	}
	return out
}

func expandLuaPackagePathEnvLiteral(value string, envName string) []string {
	base := strings.TrimSpace(os.Getenv(envName))
	if base == "" {
		return nil
	}
	parts := strings.Split(value, ";")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if !strings.Contains(part, "?") || !strings.Contains(part, ".lua") || !strings.HasPrefix(part, "/") {
			continue
		}
		out = append(out, filepath.Join(base, strings.TrimPrefix(part, "/")))
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

func parseLuaStringLiterals(content string) []string {
	out := make([]string, 0)
	for i := 0; i < len(content); i++ {
		if value, next, ok := parseLuaLongBracket(content, i); ok {
			out = append(out, value)
			i = next - 1
			continue
		}
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
	longCommentClose := ""
	for scanner.Scan() {
		line := stripLuaCommentsState(scanner.Text(), &longCommentClose)
		for _, value := range parseLuaLiteralCall(line, "require") {
			out = append(out, luaIncludeCall{Kind: luaIncludeRequire, Value: value})
		}
		for _, value := range parseLuaLiteralCall(line, "dofile") {
			out = append(out, luaIncludeCall{Kind: luaIncludeDofile, Value: value})
		}
	}
	return out
}

func stripLuaCommentsState(line string, longCommentClose *string) string {
	if *longCommentClose != "" {
		if idx := strings.Index(line, *longCommentClose); idx >= 0 {
			line = line[idx+len(*longCommentClose):]
			*longCommentClose = ""
		} else {
			return ""
		}
	}
	for i := 0; i < len(line); i++ {
		if line[i] == '\'' || line[i] == '"' {
			if next, ok := skipLuaShortString(line, i); ok {
				i = next - 1
				continue
			}
		}
		if _, next, ok := parseLuaLongBracket(line, i); ok {
			i = next - 1
			continue
		}
		if i+1 < len(line) && line[i] == '-' && line[i+1] == '-' {
			if close, contentStart, ok := parseLuaLongBracketDelimiter(line, i+2); ok {
				if end := strings.Index(line[contentStart:], close); end >= 0 {
					rest := stripLuaCommentsState(line[contentStart+end+len(close):], longCommentClose)
					return line[:i] + rest
				}
				*longCommentClose = close
			}
			return line[:i]
		}
	}
	return line
}

func parseLuaLongBracketDelimiter(line string, idx int) (string, int, bool) {
	if idx >= len(line) || line[idx] != '[' {
		return "", idx, false
	}
	endOpen := idx + 1
	for endOpen < len(line) && line[endOpen] == '=' {
		endOpen++
	}
	if endOpen >= len(line) || line[endOpen] != '[' {
		return "", idx, false
	}
	return "]" + strings.Repeat("=", endOpen-idx-1) + "]", endOpen + 1, true
}

func parseLuaLiteralCall(line string, name string) []string {
	out := make([]string, 0, 1)
	for i := 0; i < len(line); i++ {
		if isLuaSpace(line[i]) || line[i] == ';' {
			continue
		}
		if line[i] == '\'' || line[i] == '"' {
			if next, ok := skipLuaShortString(line, i); ok {
				i = next - 1
				continue
			}
		}
		if _, next, ok := parseLuaLongBracket(line, i); ok {
			i = next - 1
			continue
		}
		if i+1 < len(line) && line[i] == '-' && line[i+1] == '-' {
			return out
		}
		if !isLuaNameAt(line, i, name) {
			continue
		}
		value, next, ok := parseLuaLiteralCallArg(line, i+len(name))
		if ok && strings.TrimSpace(value) != "" {
			out = append(out, value)
			i = next - 1
		}
	}
	return out
}

func parseLuaLiteralCallArg(line string, idx int) (string, int, bool) {
	idx = trimLuaSpaceLeft(line, idx)
	needsCloseParen := false
	if idx < len(line) && line[idx] == '(' {
		needsCloseParen = true
		idx = trimLuaSpaceLeft(line, idx+1)
	}
	if idx >= len(line) {
		return "", idx, false
	}
	if value, next, ok := parseLuaLongBracket(line, idx); ok {
		if needsCloseParen {
			closeIdx := trimLuaSpaceLeft(line, next)
			if closeIdx >= len(line) || line[closeIdx] != ')' {
				return "", idx, false
			}
			return value, closeIdx + 1, true
		}
		return value, next, true
	}
	if line[idx] != '\'' && line[idx] != '"' {
		return "", idx, false
	}
	quote := line[idx]
	start := idx + 1
	for idx = start; idx < len(line); idx++ {
		if line[idx] == '\\' {
			idx++
			continue
		}
		if line[idx] == quote {
			if needsCloseParen {
				closeIdx := trimLuaSpaceLeft(line, idx+1)
				if closeIdx >= len(line) || line[closeIdx] != ')' {
					return "", idx, false
				}
				return line[start:idx], closeIdx + 1, true
			}
			return line[start:idx], idx + 1, true
		}
	}
	return "", idx, false
}

func parseLuaLongBracket(line string, idx int) (string, int, bool) {
	close, start, ok := parseLuaLongBracketDelimiter(line, idx)
	if !ok {
		return "", idx, false
	}
	end := strings.Index(line[start:], close)
	if end < 0 {
		return "", idx, false
	}
	return line[start : start+end], start + end + len(close), true
}

func skipLuaShortString(line string, idx int) (int, bool) {
	quote := line[idx]
	for idx++; idx < len(line); idx++ {
		if line[idx] == '\\' {
			idx++
			continue
		}
		if line[idx] == quote {
			return idx + 1, true
		}
	}
	return idx, false
}

func isLuaNameAt(line string, idx int, name string) bool {
	if idx > 0 && (isLuaIdentifierByte(line[idx-1]) || line[idx-1] == '.' || line[idx-1] == ':') {
		return false
	}
	if !strings.HasPrefix(line[idx:], name) {
		return false
	}
	next := idx + len(name)
	return next == len(line) || !isLuaIdentifierByte(line[next])
}

func trimLuaSpaceLeft(line string, idx int) int {
	for idx < len(line) && isLuaSpace(line[idx]) {
		idx++
	}
	return idx
}

func isLuaSpace(b byte) bool {
	return b == ' ' || b == '\f' || b == '\n' || b == '\r' || b == '\t' || b == '\v'
}

func isLuaIdentifierByte(b byte) bool {
	return b == '_' || b >= '0' && b <= '9' || b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z'
}

func isParentRelativePath(value string) bool {
	return value == ".." || strings.HasPrefix(value, ".."+string(os.PathSeparator))
}

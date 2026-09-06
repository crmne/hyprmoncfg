package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// GeneratedMonitorsBasename is the file hyprmoncfg creates and owns outright.
// Writing here instead of the conventional monitors file means hyprmoncfg never
// has to destroy a config someone else wrote, and lets its rules be loaded last
// so nothing that came before can override the applied layout.
const GeneratedMonitorsBasename = "hyprmoncfg-monitors"

const includeMarker = "hyprmoncfg"

// IncludeResult reports what EnsureIncluded did to a root config.
type IncludeResult struct {
	RootPath string
	// Previous and Written describe the exact edit for preview rollback.
	// They are populated only when this call changed the root config.
	Previous FileSnapshot
	Written  []byte
	// Line is the include as it now appears, so callers can tell a user whose
	// dotfiles are managed elsewhere which line to keep.
	Line      string
	Added     bool
	MovedLast bool
	// ReadOnly reports a root config hyprmoncfg cannot edit, such as a symlink
	// into the Nix store. Nothing was written and Line is what to add by hand.
	ReadOnly bool
}

func (r IncludeResult) Changed() bool {
	return r.Added || r.MovedLast
}

// EnsureIncluded makes the generated monitor config the last thing the root
// Hyprland config loads. It adds the include when it is missing and moves it
// back to the end when something was appended after it, so hyprmoncfg keeps the
// last word without reordering anyone else's lines.
//
// It rebuilds the same text every time, so running it again on a config it
// already settled writes nothing.
func EnsureIncluded(rootPath string, format HyprConfigFormat, targetPath string) (IncludeResult, error) {
	result := IncludeResult{RootPath: rootPath, Line: IncludeLine(format, targetPath)}
	if strings.TrimSpace(rootPath) == "" || strings.TrimSpace(targetPath) == "" {
		return result, nil
	}
	if !Writable(rootPath) {
		// A read-only config belongs to whoever generates it. Say what it needs
		// rather than failing every apply over a file that will never budge.
		result.ReadOnly = true
		return result, nil
	}

	content, err := os.ReadFile(rootPath)
	if err != nil {
		return result, fmt.Errorf("read %s: %w", rootPath, err)
	}

	lines := strings.Split(string(content), "\n")
	kept := make([]string, 0, len(lines))
	hadInclude := false
	for _, line := range lines {
		switch {
		case isHyprmoncfgInclude(line, format):
			hadInclude = true
		case isHyprmoncfgComment(line):
		default:
			kept = append(kept, line)
		}
	}

	body := strings.TrimRight(strings.Join(kept, "\n"), "\n")
	desired := body + "\n\n" + includeComment(format) + "\n" + result.Line + "\n"
	if desired == string(content) {
		return result, nil
	}

	if err := WriteFileAtomic(rootPath, []byte(desired), 0o644); err != nil {
		return result, fmt.Errorf("rewrite %s: %w", rootPath, err)
	}
	result.Added = !hadInclude
	result.MovedLast = hadInclude
	result.Previous = FileSnapshot{Path: rootPath, Exists: true, Content: content}
	result.Written = []byte(desired)
	return result, nil
}

// RemoveResult reports what RemoveInclude took out of a root config.
type RemoveResult struct {
	RootPath string
	Removed  bool
	// ReadOnly reports a root config hyprmoncfg cannot edit. Nothing was
	// written, and the include is the user's to take out.
	ReadOnly bool
}

// RemoveInclude takes hyprmoncfg's include back out of the root Hyprland
// config, handing the last word back to whatever the user or their distro
// configured. Stopping the daemon on its own does not do that: the generated
// rules keep loading last on every reload, so anything else writing monitor
// config still loses to a hyprmoncfg that is no longer running.
//
// The generated monitor file stays on disk. Once nothing loads it, it changes
// nothing, and keeping it makes turning management back on instant.
//
// This is the inverse of EnsureIncluded and matches lines the same way, so a
// config it already cleaned is left untouched.
func RemoveInclude(rootPath string, format HyprConfigFormat) (RemoveResult, error) {
	result := RemoveResult{RootPath: rootPath}
	if strings.TrimSpace(rootPath) == "" {
		return result, nil
	}
	if !Writable(rootPath) {
		result.ReadOnly = true
		return result, nil
	}

	content, err := os.ReadFile(rootPath)
	if err != nil {
		if os.IsNotExist(err) {
			return result, nil
		}
		return result, fmt.Errorf("read %s: %w", rootPath, err)
	}

	lines := strings.Split(string(content), "\n")
	kept := make([]string, 0, len(lines))
	found := false
	for _, line := range lines {
		if isHyprmoncfgInclude(line, format) || isHyprmoncfgComment(line) {
			found = true
			continue
		}
		kept = append(kept, line)
	}
	if !found {
		return result, nil
	}

	desired := strings.TrimRight(strings.Join(kept, "\n"), "\n") + "\n"
	if desired == string(content) {
		return result, nil
	}
	if err := WriteFileAtomic(rootPath, []byte(desired), 0o644); err != nil {
		return result, fmt.Errorf("rewrite %s: %w", rootPath, err)
	}
	result.Removed = true
	return result, nil
}

func isHyprmoncfgInclude(line string, format HyprConfigFormat) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || !strings.Contains(trimmed, includeMarker) {
		return false
	}
	if format == HyprConfigLua {
		return strings.HasPrefix(trimmed, "dofile(") ||
			strings.HasPrefix(trimmed, "require(") ||
			(strings.HasPrefix(trimmed, "do local ") && strings.Contains(trimmed, "dofile("))
	}
	return strings.HasPrefix(trimmed, "source")
}

func isHyprmoncfgComment(line string) bool {
	return strings.Contains(line, includeCommentText)
}

const includeCommentText = "Added by hyprmoncfg: its generated monitor rules load last"

func includeComment(format HyprConfigFormat) string {
	comment := "-- "
	if format == HyprConfigLegacy {
		comment = "# "
	}
	return comment + includeCommentText + ", so nothing before this can override the applied layout."
}

// IncludeLine renders the line that loads the generated monitor config.
//
// For a target in the usual place it resolves the path at load time from the
// environment rather than baking one in, so the same config works under a
// different user, a different home, or a dotfile repo shared across machines.
// Lua uses dofile rather than require: require depends on package.path, which
// Hyprland does not set up, and caches its result, which would keep a reload
// from re-applying the layout.
func IncludeLine(format HyprConfigFormat, targetPath string) string {
	if format == HyprConfigLegacy {
		if relative, ok := configHomeRelative(targetPath); ok {
			return "source = ~/.config/" + relative
		}
		return "source = " + targetPath
	}

	relative, ok := configHomeRelative(targetPath)
	if !ok {
		return guardedLuaInclude(luaQuote(targetPath))
	}
	// Spelling out XDG_CONFIG_HOME only earns its noise when it is actually
	// set. Everywhere else the config home is ~/.config, and the short form
	// says the same thing.
	if os.Getenv("XDG_CONFIG_HOME") == "" {
		return guardedLuaInclude(fmt.Sprintf(`os.getenv("HOME") .. %s`, luaQuote("/.config/"+relative)))
	}
	return guardedLuaInclude(fmt.Sprintf(
		`(os.getenv("XDG_CONFIG_HOME") or os.getenv("HOME") .. "/.config") .. %s`,
		luaQuote("/"+relative)))
}

func guardedLuaInclude(pathExpression string) string {
	return fmt.Sprintf(
		`do local path = %s; local file = io.open(path, "r"); if file then file:close(); dofile(path) end end`,
		pathExpression,
	)
}

// configHomeRelative reports a target's path below the XDG config home, so the
// include can name it without a machine-specific prefix.
func configHomeRelative(targetPath string) (string, bool) {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return "", false
		}
		configHome = filepath.Join(home, ".config")
	}

	relative, err := filepath.Rel(configHome, targetPath)
	if err != nil || strings.HasPrefix(relative, "..") || filepath.IsAbs(relative) {
		return "", false
	}
	return filepath.ToSlash(relative), true
}

func luaQuote(value string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value) + `"`
}

// retiredNotice replaces the body of a monitors file an older hyprmoncfg wrote,
// so a stale copy of the layout cannot keep being applied from the old place.
func retiredNotice(format HyprConfigFormat, currentPath string) string {
	comment := "--"
	if format == HyprConfigLegacy {
		comment = "#"
	}
	return fmt.Sprintf(
		"%s hyprmoncfg no longer writes this file.\n%s Its generated monitor rules now live in %s.\n%s Anything you add here is yours to keep, but hyprmoncfg loads its own file after it.\n",
		comment, comment, currentPath, comment,
	)
}

// RetireLegacyMonitorsFile empties out the monitors file hyprmoncfg used to
// own, once it writes its own file instead. It returns the path it retired, or
// an empty string when there was nothing of ours to retire: a file hyprmoncfg
// did not generate belongs to whoever wrote it and is left untouched.
func RetireLegacyMonitorsFile(format HyprConfigFormat, currentPath string) (string, error) {
	legacyPath, err := LegacyMonitorsPath(format)
	if err != nil {
		return "", err
	}
	if legacyPath == currentPath {
		return "", nil
	}

	content, err := os.ReadFile(legacyPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read %s: %w", legacyPath, err)
	}
	if !IsGeneratedMonitorsConfig(content) {
		return "", nil
	}

	notice := retiredNotice(format, currentPath)
	if string(content) == notice {
		return "", nil
	}
	if err := WriteFileAtomic(legacyPath, []byte(notice), 0o644); err != nil {
		return "", fmt.Errorf("retire %s: %w", legacyPath, err)
	}
	return legacyPath, nil
}

// VerifyLoadedLast reports whether the root config loads the generated monitor
// config, and does so after everything else. Anything loaded later could
// override the applied layout.
func VerifyLoadedLast(rootPath string, format HyprConfigFormat, targetPath string) error {
	if err := VerifyGeneratedMonitors(targetPath); err != nil {
		return err
	}
	content, err := os.ReadFile(rootPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", rootPath, err)
	}

	lines := strings.Split(string(content), "\n")
	for idx := len(lines) - 1; idx >= 0; idx-- {
		line := strings.TrimSpace(lines[idx])
		if line == "" || isHyprmoncfgComment(lines[idx]) {
			continue
		}
		if isHyprmoncfgInclude(lines[idx], format) {
			return nil
		}
		return fmt.Errorf("%s does not load %s last; add `%s` at its end", rootPath, targetPath, IncludeLine(format, targetPath))
	}
	return fmt.Errorf("%s does not load %s; add `%s` at its end", rootPath, targetPath, IncludeLine(format, targetPath))
}

// VerifyGeneratedMonitors rejects an include whose target is missing or cannot
// be read. The Lua include is defensive, but a missing generated file still
// means Hyprland has no hyprmoncfg monitor rules to load.
func VerifyGeneratedMonitors(targetPath string) error {
	file, err := os.Open(targetPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("generated monitor config %s does not exist: %w", targetPath, err)
		}
		return fmt.Errorf("generated monitor config %s is not readable: %w", targetPath, err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("inspect generated monitor config %s: %w", targetPath, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("generated monitor config %s is not a regular file", targetPath)
	}
	return nil
}

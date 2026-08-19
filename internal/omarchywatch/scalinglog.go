package omarchywatch

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	// Omarchy's Display panel and Super+/ scaler append one line here for
	// every successful set. Clamshell and the monitor watcher do not.
	scaleLogRelPath = "omarchy/monitor-scaling.log"

	// Long enough to cover hyprmoncfgd's 5s poll plus debounce, short enough
	// that an old click cannot rewrite a profile after a later clamshell reset.
	ScaleLogMaxAge = 15 * time.Second

	scaleLogReadTail = 32 * 1024
)

// RecentRequestedScales returns the newest Omarchy Display-panel scale
// request per connector that is no older than ScaleLogMaxAge.
func RecentRequestedScales(now time.Time) map[string]float64 {
	return ParseRequestedScales(readScaleLogTail(), now, ScaleLogMaxAge)
}

func readScaleLogTail() []byte {
	path := scaleLogPath()
	if path == "" {
		return nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil
	}
	start := info.Size() - scaleLogReadTail
	if start < 0 {
		start = 0
	}
	if start > 0 {
		if _, err := file.Seek(start, io.SeekStart); err != nil {
			return nil
		}
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return nil
	}
	return data
}

func scaleLogPath() string {
	state := os.Getenv("XDG_STATE_HOME")
	if state == "" {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return ""
		}
		state = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(state, scaleLogRelPath)
}

// ParseRequestedScales is the testable core of RecentRequestedScales.
func ParseRequestedScales(data []byte, now time.Time, maxAge time.Duration) map[string]float64 {
	if len(data) == 0 || maxAge <= 0 {
		return nil
	}

	requested := make(map[string]float64)
	seenAt := make(map[string]time.Time)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		monitor, scale, at, ok := parseScaleLogLine(scanner.Text())
		if !ok {
			continue
		}
		if now.Sub(at) > maxAge || at.After(now.Add(time.Second)) {
			continue
		}
		if previous, exists := seenAt[monitor]; exists && !at.After(previous) {
			continue
		}
		requested[monitor] = scale
		seenAt[monitor] = at
	}
	if len(requested) == 0 {
		return nil
	}
	return requested
}

func parseScaleLogLine(line string) (string, float64, time.Time, bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", 0, time.Time{}, false
	}

	fields := make(map[string]string)
	for _, field := range strings.Split(line, "\t") {
		key, value, ok := strings.Cut(field, "=")
		if !ok {
			continue
		}
		fields[key] = value
	}

	at, err := time.Parse(time.RFC3339, fields["at"])
	if err != nil {
		return "", 0, time.Time{}, false
	}
	scale, err := strconv.ParseFloat(fields["new"], 64)
	if err != nil || scale <= 0 {
		return "", 0, time.Time{}, false
	}
	monitor := strings.TrimSpace(fields["monitor"])
	if monitor == "" {
		return "", 0, time.Time{}, false
	}
	return monitor, scale, at, true
}

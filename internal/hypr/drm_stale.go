package hypr

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// StaleDRMConnectors names the connectors the kernel still drives although
// they are disconnected: sysfs reports them disconnected but enabled, so a
// CRTC stays routed to a display that is gone. aquamarine 0.15.0 leaves them
// behind when a display disconnects (hyprwm/aquamarine#386), and a display
// later handed that CRTC fails every modeset.
func StaleDRMConnectors() []string {
	return staleDRMConnectors(defaultDRMSysRoot)
}

func staleDRMConnectors(sysRoot string) []string {
	matches, err := filepath.Glob(filepath.Join(sysRoot, "card*-*"))
	if err != nil {
		return nil
	}
	var stale []string
	for _, path := range matches {
		card, name, ok := strings.Cut(filepath.Base(path), "-")
		if !ok || card == "" || name == "" {
			continue
		}
		status, err := os.ReadFile(filepath.Join(path, "status"))
		if err != nil || strings.TrimSpace(string(status)) != "disconnected" {
			continue
		}
		enabled, err := os.ReadFile(filepath.Join(path, "enabled"))
		if err != nil || strings.TrimSpace(string(enabled)) != "enabled" {
			continue
		}
		stale = append(stale, name)
	}
	sort.Strings(stale)
	return stale
}

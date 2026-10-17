package hypr

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DRMConnectorState is what sysfs says about a connector: whether a display
// is connected, and whether the kernel still routes a CRTC to it.
type DRMConnectorState struct {
	Name      string
	Connected bool
	Enabled   bool
}

// DRMConnectorStates reads every connector's state from sysfs. aquamarine
// 0.15.0 leaves CRTCs routed in the kernel that it no longer accounts for
// (hyprwm/aquamarine#386, #428): a disconnected connector that stays
// enabled, or two connectors handed each other's CRTC after reconnecting.
// A display then fails every modeset while the kernel still drives its pipe.
func DRMConnectorStates() []DRMConnectorState {
	return drmConnectorStates(defaultDRMSysRoot)
}

func drmConnectorStates(sysRoot string) []DRMConnectorState {
	matches, err := filepath.Glob(filepath.Join(sysRoot, "card*-*"))
	if err != nil {
		return nil
	}
	var states []DRMConnectorState
	for _, path := range matches {
		card, name, ok := strings.Cut(filepath.Base(path), "-")
		if !ok || card == "" || name == "" {
			continue
		}
		status, err := os.ReadFile(filepath.Join(path, "status"))
		if err != nil {
			continue
		}
		enabled, err := os.ReadFile(filepath.Join(path, "enabled"))
		if err != nil {
			continue
		}
		states = append(states, DRMConnectorState{
			Name:      name,
			Connected: strings.TrimSpace(string(status)) == "connected",
			Enabled:   strings.TrimSpace(string(enabled)) == "enabled",
		})
	}
	sort.Slice(states, func(i, j int) bool { return states[i].Name < states[j].Name })
	return states
}

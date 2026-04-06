package hypr

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
)

// VRRMode handles hyprctl reporting VRR as either a bool or an int.
// bool: false=0, true=1. int: 0=off, 1=on, 2=fullscreen-only.
type VRRMode int

func (v *VRRMode) UnmarshalJSON(data []byte) error {
	var i int
	if err := json.Unmarshal(data, &i); err == nil {
		*v = VRRMode(i)
		return nil
	}
	var b bool
	if err := json.Unmarshal(data, &b); err == nil {
		if b {
			*v = 1
		} else {
			*v = 0
		}
		return nil
	}
	return fmt.Errorf("VRR: expected bool or int, got %s", data)
}

type Workspace struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type WorkspaceState struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Monitor      string `json:"monitor"`
	MonitorID    int    `json:"monitorID"`
	Windows      int    `json:"windows"`
	IsPersistent bool   `json:"ispersistent"`
}

type WorkspaceRule struct {
	WorkspaceString string `json:"workspaceString"`
	Monitor         string `json:"monitor"`
	Default         bool   `json:"default"`
	Persistent      bool   `json:"persistent"`
}

type Monitor struct {
	ID                    int       `json:"id"`
	Name                  string    `json:"name"`
	Description           string    `json:"description"`
	Make                  string    `json:"make"`
	Model                 string    `json:"model"`
	Serial                string    `json:"serial"`
	Width                 int       `json:"width"`
	Height                int       `json:"height"`
	PhysicalWidth         int       `json:"physicalWidth"`
	PhysicalHeight        int       `json:"physicalHeight"`
	RefreshRate           float64   `json:"refreshRate"`
	X                     int       `json:"x"`
	Y                     int       `json:"y"`
	Scale                 float64   `json:"scale"`
	Transform             int       `json:"transform"`
	Focused               bool      `json:"focused"`
	DPMSStatus            bool      `json:"dpmsStatus"`
	VRR                   VRRMode   `json:"vrr"`
	Disabled              bool      `json:"disabled"`
	MirrorOf              string    `json:"mirrorOf"`
	CurrentFormat         string    `json:"currentFormat"`
	ColorManagementPreset string    `json:"colorManagementPreset"`
	SDRBrightness         float64   `json:"sdrBrightness"`
	SDRSaturation         float64   `json:"sdrSaturation"`
	SDRMinLuminance       float64   `json:"sdrMinLuminance"`
	SDRMaxLuminance       int       `json:"sdrMaxLuminance"`
	AvailableModes        []string  `json:"availableModes"`
	ActiveWorkspace       Workspace `json:"activeWorkspace"`
}

func (m Monitor) Bitdepth() int {
	switch m.CurrentFormat {
	case "XBGR2101010", "XRGB2101010", "ABGR2101010", "ARGB2101010":
		return 10
	case "XBGR16161616F", "XRGB16161616F", "ABGR16161616F", "ARGB16161616F":
		return 16
	default:
		return 8
	}
}

func (m Monitor) IsInternal() bool {
	n := strings.ToLower(m.Name)
	return strings.HasPrefix(n, "edp") || strings.HasPrefix(n, "lvds") || strings.HasPrefix(n, "dsi")
}

func (m Monitor) HardwareKey() string {
	parts := []string{
		cleanIDPart(m.Make),
		cleanIDPart(m.Model),
		cleanIDPart(m.Serial),
	}
	nonEmpty := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			nonEmpty = append(nonEmpty, p)
		}
	}
	if len(nonEmpty) > 0 {
		return strings.Join(nonEmpty, "|")
	}
	if d := cleanIDPart(m.Description); d != "" {
		return d
	}
	return cleanIDPart(m.Name)
}

func MonitorMatchCounts(monitors []Monitor) map[string]int {
	counts := make(map[string]int, len(monitors))
	for _, monitor := range monitors {
		counts[monitor.HardwareKey()]++
	}
	return counts
}

func UniqueOutputKey(matchKey string, connector string, duplicates int) string {
	matchKey = cleanIDPart(matchKey)
	connector = cleanIDPart(connector)

	switch {
	case duplicates <= 1 && matchKey != "":
		return matchKey
	case matchKey == "" && connector != "":
		return connector
	case connector == "":
		return matchKey
	default:
		return matchKey + "@" + connector
	}
}

func MonitorOutputKey(m Monitor, matchCounts map[string]int) string {
	matchKey := m.HardwareKey()
	return UniqueOutputKey(matchKey, m.Name, matchCounts[matchKey])
}

func (m Monitor) ModeString() string {
	return FormatMode(m.Width, m.Height, m.RefreshRate)
}

func (m Monitor) MonitorSelector() string {
	if desc := strings.TrimSpace(m.Description); desc != "" {
		return "desc:" + desc
	}
	return m.Name
}

func (m Monitor) LogicalSize() (int, int) {
	scale := m.Scale
	if scale <= 0 {
		scale = 1
	}
	width := int(math.Round(float64(m.Width) / scale))
	height := int(math.Round(float64(m.Height) / scale))

	if m.Transform%2 == 1 {
		width, height = height, width
	}
	return width, height
}

func cleanIDPart(v string) string {
	v = strings.TrimSpace(strings.ToLower(v))
	v = strings.Join(strings.Fields(v), " ")
	return v
}

func FormatMode(width, height int, refresh float64) string {
	if width == 0 || height == 0 {
		return "preferred"
	}
	if refresh <= 0 {
		return fmt.Sprintf("%dx%d", width, height)
	}
	return fmt.Sprintf("%dx%d@%.2fHz", width, height, refresh)
}

func ParseMode(mode string) (int, int, float64, bool) {
	mode = strings.TrimSpace(strings.TrimSuffix(mode, "Hz"))
	if mode == "" || mode == "preferred" {
		return 0, 0, 0, false
	}

	var (
		width   int
		height  int
		refresh float64
	)

	if _, err := fmt.Sscanf(mode, "%dx%d@%f", &width, &height, &refresh); err == nil {
		return width, height, refresh, true
	}
	if _, err := fmt.Sscanf(mode, "%dx%d", &width, &height); err == nil {
		return width, height, 0, true
	}
	return 0, 0, 0, false
}

func MonitorOrder(monitors []Monitor) []string {
	sorted := append([]Monitor(nil), monitors...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].X != sorted[j].X {
			return sorted[i].X < sorted[j].X
		}
		if sorted[i].Y != sorted[j].Y {
			return sorted[i].Y < sorted[j].Y
		}
		return sorted[i].Name < sorted[j].Name
	})

	matchCounts := MonitorMatchCounts(sorted)
	keys := make([]string, 0, len(sorted))
	for _, monitor := range sorted {
		if monitor.MirrorOf != "" {
			continue
		}
		keys = append(keys, MonitorOutputKey(monitor, matchCounts))
	}
	return keys
}

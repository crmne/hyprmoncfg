package profile

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/crmne/hyprmoncfg/internal/hypr"
)

type OutputConfig struct {
	Key               string  `json:"key"`
	MatchKey          string  `json:"match_key,omitempty"`
	Name              string  `json:"name"`
	Make              string  `json:"make,omitempty"`
	Model             string  `json:"model,omitempty"`
	Serial            string  `json:"serial,omitempty"`
	Enabled           bool    `json:"enabled"`
	Mode              string  `json:"mode,omitempty"`
	Width             int     `json:"width"`
	Height            int     `json:"height"`
	Refresh           float64 `json:"refresh"`
	X                 int     `json:"x"`
	Y                 int     `json:"y"`
	Scale             float64 `json:"scale"`
	VRR               int     `json:"vrr,omitempty"`
	Transform         int     `json:"transform"`
	MirrorOf          string  `json:"mirror_of,omitempty"`
	Bitdepth          int     `json:"bitdepth,omitempty"`
	CM                string  `json:"cm,omitempty"`
	SDRBrightness     float64 `json:"sdr_brightness,omitempty"`
	SDRSaturation     float64 `json:"sdr_saturation,omitempty"`
	SDRMinLuminance   float64 `json:"sdr_min_luminance"`
	SDRMaxLuminance   int     `json:"sdr_max_luminance,omitempty"`
	MinLuminance      float64 `json:"min_luminance"`
	MaxLuminance      int     `json:"max_luminance,omitempty"`
	SupportsWideColor int     `json:"supports_wide_color,omitempty"`
	SupportsHDR       int     `json:"supports_hdr,omitempty"`
	MaxAvgLuminance   int     `json:"max_avg_luminance,omitempty"`
	SDREOTF           string  `json:"sdr_eotf,omitempty"`
	ICC               string  `json:"icc,omitempty"`
}

type WorkspaceStrategy string

const (
	WorkspaceStrategyManual     WorkspaceStrategy = "manual"
	WorkspaceStrategySequential WorkspaceStrategy = "sequential"
	WorkspaceStrategyInterleave WorkspaceStrategy = "interleave"
)

type WorkspaceRule struct {
	Workspace  string `json:"workspace"`
	OutputKey  string `json:"output_key"`
	OutputName string `json:"output_name,omitempty"`
	Default    bool   `json:"default,omitempty"`
	Persistent bool   `json:"persistent,omitempty"`
}

type WorkspaceSettings struct {
	Enabled       bool              `json:"enabled"`
	Strategy      WorkspaceStrategy `json:"strategy,omitempty"`
	MaxWorkspaces int               `json:"max_workspaces,omitempty"`
	GroupSize     int               `json:"group_size,omitempty"`
	MonitorOrder  []string          `json:"monitor_order,omitempty"`
	Rules         []WorkspaceRule   `json:"rules,omitempty"`
}

type Profile struct {
	Name       string            `json:"name"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
	Outputs    []OutputConfig    `json:"outputs"`
	Workspaces WorkspaceSettings `json:"workspaces,omitempty"`
	Exec       string            `json:"exec"`
}

func New(name string, outputs []OutputConfig) Profile {
	now := time.Now().UTC()
	p := Profile{
		Name:      strings.TrimSpace(name),
		CreatedAt: now,
		UpdatedAt: now,
		Outputs:   append([]OutputConfig(nil), outputs...),
	}
	p.normalizeIdentityRefs()
	p.SortOutputs()
	return p
}

func FromMonitors(name string, monitors []hypr.Monitor) Profile {
	return FromState(name, monitors, nil)
}

func FromState(name string, monitors []hypr.Monitor, rules []hypr.WorkspaceRule) Profile {
	matchCounts := hypr.MonitorMatchCounts(monitors)
	nameToKey := make(map[string]string, len(monitors))
	for _, m := range monitors {
		nameToKey[m.Name] = hypr.MonitorOutputKey(m, matchCounts)
	}

	outputs := make([]OutputConfig, 0, len(monitors))
	for _, m := range monitors {
		mirrorOf := ""
		if m.MirrorOf != "" {
			mirrorOf = nameToKey[m.MirrorOf]
		}
		outputs = append(outputs, OutputConfig{
			Key:             hypr.MonitorOutputKey(m, matchCounts),
			MatchKey:        m.HardwareKey(),
			Name:            m.Name,
			Make:            m.Make,
			Model:           m.Model,
			Serial:          m.Serial,
			Enabled:         !m.Disabled,
			Mode:            m.ModeString(),
			Width:           m.Width,
			Height:          m.Height,
			Refresh:         m.RefreshRate,
			X:               m.X,
			Y:               m.Y,
			Scale:           m.Scale,
			VRR:             int(m.VRR),
			Transform:       m.Transform,
			MirrorOf:        mirrorOf,
			Bitdepth:        m.Bitdepth(),
			CM:              m.ColorManagementPreset,
			SDRBrightness:   m.SDRBrightness,
			SDRSaturation:   m.SDRSaturation,
			SDRMinLuminance: m.SDRMinLuminance,
			SDRMaxLuminance: m.SDRMaxLuminance,
		})
	}
	p := New(name, outputs)
	p.Workspaces = WorkspaceSettingsFromHypr(monitors, rules)
	p.normalizeIdentityRefs()
	return p
}

func (p *Profile) SortOutputs() {
	sort.SliceStable(p.Outputs, func(i, j int) bool {
		if p.Outputs[i].Enabled != p.Outputs[j].Enabled {
			return p.Outputs[i].Enabled
		}
		return p.Outputs[i].Key < p.Outputs[j].Key
	})
}

func (p Profile) Validate() error {
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("profile name cannot be empty")
	}
	if len(p.Outputs) == 0 {
		return fmt.Errorf("profile has no outputs")
	}
	for i, out := range p.Outputs {
		if out.Key == "" {
			return fmt.Errorf("output %d has empty key", i)
		}
		if out.Enabled {
			if out.Scale <= 0 {
				return fmt.Errorf("output %d has invalid scale", i)
			}
			if out.Width < 0 || out.Height < 0 {
				return fmt.Errorf("output %d has invalid resolution", i)
			}
		}
		if out.VRR < 0 || out.VRR > 2 {
			return fmt.Errorf("output %d has invalid VRR mode", i)
		}
		if out.Bitdepth != 0 && out.Bitdepth != 8 && out.Bitdepth != 10 && out.Bitdepth != 16 {
			return fmt.Errorf("output %d has invalid bitdepth %d", i, out.Bitdepth)
		}
	}
	if err := p.Workspaces.Validate(); err != nil {
		return err
	}
	return nil
}

func (p Profile) OutputByKey(key string) (OutputConfig, bool) {
	for _, out := range p.Outputs {
		if out.Key == key {
			return out, true
		}
	}
	return OutputConfig{}, false
}

func (p Profile) Keys() []string {
	keys := make([]string, 0, len(p.Outputs))
	for _, out := range p.Outputs {
		keys = append(keys, out.Key)
	}
	sort.Strings(keys)
	return keys
}

func (o OutputConfig) NormalizedMode() string {
	if strings.TrimSpace(o.Mode) != "" {
		return strings.TrimSpace(o.Mode)
	}
	return hypr.FormatMode(o.Width, o.Height, o.Refresh)
}

func (o OutputConfig) MatchIdentity() string {
	matchKey := strings.TrimSpace(o.MatchKey)
	if matchKey != "" {
		return matchKey
	}
	if matchKey = outputMatchKeyFromFields(o); matchKey != "" {
		return matchKey
	}
	return strings.TrimSpace(o.Key)
}

func (p *Profile) normalizeIdentityRefs() {
	normalizeIdentityRefs(p.Outputs, &p.Workspaces)
}

func (p *Profile) Normalize() {
	p.normalizeIdentityRefs()
	p.SortOutputs()
}

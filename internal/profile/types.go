package profile

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/crmne/hyprmoncfg/internal/hypr"
)

type OutputConfig struct {
	Key       string  `json:"key"`
	Name      string  `json:"name"`
	Make      string  `json:"make,omitempty"`
	Model     string  `json:"model,omitempty"`
	Serial    string  `json:"serial,omitempty"`
	Enabled   bool    `json:"enabled"`
	Mode      string  `json:"mode,omitempty"`
	Width     int     `json:"width"`
	Height    int     `json:"height"`
	Refresh   float64 `json:"refresh"`
	X         int     `json:"x"`
	Y         int     `json:"y"`
	Scale     float64 `json:"scale"`
	VRR       int     `json:"vrr,omitempty"`
	Transform int     `json:"transform"`
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
}

func New(name string, outputs []OutputConfig) Profile {
	now := time.Now().UTC()
	p := Profile{
		Name:      strings.TrimSpace(name),
		CreatedAt: now,
		UpdatedAt: now,
		Outputs:   append([]OutputConfig(nil), outputs...),
	}
	p.SortOutputs()
	return p
}

func FromMonitors(name string, monitors []hypr.Monitor) Profile {
	return FromState(name, monitors, nil)
}

func FromState(name string, monitors []hypr.Monitor, rules []hypr.WorkspaceRule) Profile {
	outputs := make([]OutputConfig, 0, len(monitors))
	for _, m := range monitors {
		outputs = append(outputs, OutputConfig{
			Key:       m.HardwareKey(),
			Name:      m.Name,
			Make:      m.Make,
			Model:     m.Model,
			Serial:    m.Serial,
			Enabled:   !m.Disabled,
			Mode:      m.ModeString(),
			Width:     m.Width,
			Height:    m.Height,
			Refresh:   m.RefreshRate,
			X:         m.X,
			Y:         m.Y,
			Scale:     m.Scale,
			VRR:       boolToVRR(m.VRR),
			Transform: m.Transform,
		})
	}
	p := New(name, outputs)
	p.Workspaces = WorkspaceSettingsFromHypr(monitors, rules)
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

func boolToVRR(v bool) int {
	if v {
		return 1
	}
	return 0
}

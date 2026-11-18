package profile

import (
	"sort"
	"strings"

	"github.com/crmne/hyprmoncfg/internal/hypr"
)

// ScoreModelMatch rewards a saved display that stands in for a connected one
// of the same make and model but a different unit (hot-desking: another copy
// of the office monitor). It is strictly below an exact match, so the unit a
// profile was saved with always counts for more, and above the penalties for
// a missing and an unknown display it replaces, so the profile that describes
// the desk beats one that would only extend the new unit.
const ScoreModelMatch = 60

// ModelMatch pairs a saved display with the connected unit of the same make
// and model that takes its place in the layout.
type ModelMatch struct {
	// OutputKey is the saved output's key in the profile.
	OutputKey string
	// Monitor is the connected unit standing in for it.
	Monitor hypr.Monitor
}

// WithProfiles records which displays the saved profiles name exactly. A
// connected display some saved profile knows by its make, model and serial is
// never matched to another profile by model: that profile is the one saved
// for it, and an exact match always wins.
func (o MatchOptions) WithProfiles(profiles []Profile) MatchOptions {
	o.knownHardware = knownHardware(profiles)
	return o
}

func knownHardware(profiles []Profile) map[string]struct{} {
	known := make(map[string]struct{})
	for _, p := range profiles {
		for _, output := range p.Outputs {
			key := strings.TrimSpace(outputMatchKeyFromFields(output))
			if key == "" {
				key = strings.TrimSpace(output.MatchIdentity())
			}
			if key != "" {
				known[key] = struct{}{}
			}
		}
	}
	return known
}

func modelIdentity(manufacturer, model string) string {
	manufacturer, model = cleanProfileIDPart(manufacturer), cleanProfileIDPart(model)
	if manufacturer == "" || model == "" {
		return ""
	}
	return manufacturer + "|" + model
}

// modelMatches finds the saved displays a profile can place on a connected
// unit of the same make and model when nothing matches that unit exactly.
// It only ever pairs one leftover saved display with one leftover connected
// unit of a model: two identical saved displays, or two identical new units,
// give no way to tell which goes where, so they stay unmatched and the new
// units are extended as before. Built-in panels and disabled saved displays
// are never stand-ins, and a strict profile keeps every unnamed display off.
// p must be normalized.
func modelMatches(p Profile, monitors []hypr.Monitor, known map[string]struct{}) []ModelMatch {
	if p.DisableUnknownOutputs || len(monitors) == 0 || len(p.Outputs) == 0 {
		return nil
	}

	outputsByKey := make(map[string][]int, len(p.Outputs))
	for i, output := range p.Outputs {
		key := output.MatchIdentity()
		outputsByKey[key] = append(outputsByKey[key], i)
	}
	monitorsByKey := make(map[string][]hypr.Monitor, len(monitors))
	for _, monitor := range monitors {
		key := monitor.HardwareKey()
		monitorsByKey[key] = append(monitorsByKey[key], monitor)
	}

	type group struct {
		outputs   []int
		monitors  []hypr.Monitor
		ambiguous bool
	}
	groups := make(map[string]*group)
	groupFor := func(model string) *group {
		g := groups[model]
		if g == nil {
			g = &group{}
			groups[model] = g
		}
		return g
	}

	for key, indices := range outputsByKey {
		first := p.Outputs[indices[0]]
		model := modelIdentity(first.Make, first.Model)
		if model == "" {
			continue
		}
		if connected := len(monitorsByKey[key]); connected > 0 {
			// Identical saved displays partly matched exactly: which one
			// is left over is a guess, so the model has no stand-in.
			if len(indices) > connected {
				groupFor(model).ambiguous = true
			}
			continue
		}
		g := groupFor(model)
		g.outputs = append(g.outputs, indices...)
	}
	for key, units := range monitorsByKey {
		model := modelIdentity(units[0].Make, units[0].Model)
		if model == "" {
			continue
		}
		if saved := len(outputsByKey[key]); saved > 0 {
			if len(units) > saved {
				groupFor(model).ambiguous = true
			}
			continue
		}
		g := groupFor(model)
		g.monitors = append(g.monitors, units...)
	}

	models := make([]string, 0, len(groups))
	for model := range groups {
		models = append(models, model)
	}
	sort.Strings(models)

	var matches []ModelMatch
	for _, model := range models {
		g := groups[model]
		if g.ambiguous || len(g.outputs) != 1 || len(g.monitors) != 1 {
			continue
		}
		output := p.Outputs[g.outputs[0]]
		monitor := g.monitors[0]
		if !output.Enabled || hypr.IsInternalConnector(output.Name) || monitor.IsInternal() || monitor.Name == "FALLBACK" {
			continue
		}
		if _, ok := known[monitor.HardwareKey()]; ok {
			continue
		}
		matches = append(matches, ModelMatch{OutputKey: output.Key, Monitor: monitor})
	}
	return matches
}

// BindModelMatches returns the profile with each saved display that matches
// only by model moved onto the connected unit standing in for it, so the
// layout renders, applies and verifies against that unit (its connector and
// its own description in `desc:` selectors) like any exact match. The result
// is ephemeral: the saved profile is never modified.
//
// A stand-in keeps the saved layout and signal settings (mode, position,
// scale, rotation, VRR, bit depth, color preset, SDR mapping), which belong
// to the model and the desk. It drops what was measured on the saved unit:
// its ICC profile and the HDR luminance overrides, which would mis-calibrate
// a different panel. Hyprland then uses the new unit's own EDID values.
//
// profiles are all saved profiles; see MatchOptions.WithProfiles.
func BindModelMatches(p Profile, monitors []hypr.Monitor, profiles []Profile) (Profile, []ModelMatch) {
	return bindModelMatches(p, monitors, knownHardware(profiles))
}

func bindModelMatches(p Profile, monitors []hypr.Monitor, known map[string]struct{}) (Profile, []ModelMatch) {
	p.Outputs = append([]OutputConfig(nil), p.Outputs...)
	p.Workspaces.MonitorOrder = append([]string(nil), p.Workspaces.MonitorOrder...)
	p.Workspaces.Rules = append([]WorkspaceRule(nil), p.Workspaces.Rules...)
	p.Normalize()
	matches := modelMatches(p, monitors, known)
	if len(matches) == 0 {
		return p, nil
	}

	counts := hypr.MonitorMatchCounts(monitors)
	for _, match := range matches {
		index := -1
		for i := range p.Outputs {
			if p.Outputs[i].Key == match.OutputKey {
				index = i
				break
			}
		}
		if index < 0 {
			continue
		}
		monitor := match.Monitor
		oldKey := match.OutputKey
		newKey := hypr.MonitorOutputKey(monitor, counts)

		output := &p.Outputs[index]
		output.Key = newKey
		output.MatchKey = monitor.HardwareKey()
		output.Name = monitor.Name
		output.Description = monitor.Description
		output.Make = monitor.Make
		output.Model = monitor.Model
		output.Serial = monitor.Serial
		output.ICC = ""
		output.MinLuminance = 0
		output.MaxLuminance = 0
		output.MaxAvgLuminance = 0

		for i := range p.Outputs {
			if p.Outputs[i].MirrorOf == oldKey {
				p.Outputs[i].MirrorOf = newKey
			}
		}
		for i := range p.Workspaces.Rules {
			if p.Workspaces.Rules[i].OutputKey == oldKey {
				p.Workspaces.Rules[i].OutputKey = newKey
				p.Workspaces.Rules[i].OutputName = monitor.Name
			}
		}
		for i := range p.Workspaces.MonitorOrder {
			if p.Workspaces.MonitorOrder[i] == oldKey {
				p.Workspaces.MonitorOrder[i] = newKey
			}
		}
	}
	p.Normalize()
	return p, matches
}

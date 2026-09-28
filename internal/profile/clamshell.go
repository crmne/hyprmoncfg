package profile

import (
	"sort"
	"strings"

	"github.com/crmne/hyprmoncfg/internal/hypr"
)

type ClosedLidAdjustment struct {
	Applied              bool
	DisabledOutputNames  []string
	WorkspaceTargetName  string
	RetargetedWorkspaces int
}

type workspaceTarget struct {
	key  string
	name string
}

func ApplyClosedLidPolicy(p Profile, monitors []hypr.Monitor) (Profile, ClosedLidAdjustment) {
	adjusted := cloneProfile(p)
	adjusted.Normalize()

	if !hasUsableExternalOutput(adjusted, monitors) {
		return adjusted, ClosedLidAdjustment{}
	}

	resolver := NewMonitorResolver(monitors)
	matchCounts := hypr.MonitorMatchCounts(monitors)
	internalKeys := make(map[string]bool)
	internalNames := make(map[string]bool)
	adjustment := ClosedLidAdjustment{}

	for idx := range adjusted.Outputs {
		monitor, ok := resolver.ResolveOutput(adjusted.Outputs[idx])
		if !ok || !monitor.IsInternal() {
			continue
		}

		internalKeys[adjusted.Outputs[idx].Key] = true
		internalNames[strings.TrimSpace(monitor.Name)] = true
		if adjusted.Outputs[idx].Enabled {
			adjustment.DisabledOutputNames = append(adjustment.DisabledOutputNames, monitor.Name)
		}
		adjusted.Outputs[idx].Enabled = false
		adjusted.Outputs[idx].MirrorOf = ""
	}

	for _, monitor := range monitors {
		if !monitor.IsInternal() || internalNames[strings.TrimSpace(monitor.Name)] {
			continue
		}
		output := OutputConfig{
			Key:       hypr.MonitorOutputKey(monitor, matchCounts),
			MatchKey:  monitor.HardwareKey(),
			Name:      monitor.Name,
			Make:      monitor.Make,
			Model:     monitor.Model,
			Serial:    monitor.Serial,
			Enabled:   false,
			Mode:      monitor.ModeString(),
			Width:     monitor.Width,
			Height:    monitor.Height,
			Refresh:   monitor.RefreshRate,
			X:         monitor.X,
			Y:         monitor.Y,
			Scale:     monitor.Scale,
			VRR:       int(monitor.VRR),
			Transform: monitor.Transform,
		}
		adjusted.Outputs = append(adjusted.Outputs, output)
		internalKeys[output.Key] = true
		internalNames[strings.TrimSpace(monitor.Name)] = true
		if !monitor.Disabled {
			adjustment.DisabledOutputNames = append(adjustment.DisabledOutputNames, monitor.Name)
		}
	}

	if len(internalKeys) == 0 {
		return adjusted, adjustment
	}

	for idx := range adjusted.Outputs {
		if internalKeys[adjusted.Outputs[idx].MirrorOf] {
			adjusted.Outputs[idx].MirrorOf = ""
		}
	}

	target := closedLidWorkspaceTarget(adjusted, monitors)
	if target.key != "" {
		adjustment.WorkspaceTargetName = target.name
		adjustment.RetargetedWorkspaces = retargetInternalWorkspaceRules(&adjusted.Workspaces, internalKeys, internalNames, target)
	}
	adjusted.Workspaces.MonitorOrder = removeInternalMonitorOrderRefs(adjusted.Workspaces.MonitorOrder, internalKeys, internalNames)
	sort.Strings(adjustment.DisabledOutputNames)
	adjustment.Applied = len(adjustment.DisabledOutputNames) > 0 || adjustment.RetargetedWorkspaces > 0
	return adjusted, adjustment
}

func cloneProfile(p Profile) Profile {
	p.Outputs = append([]OutputConfig(nil), p.Outputs...)
	p.Workspaces.MonitorOrder = append([]string(nil), p.Workspaces.MonitorOrder...)
	p.Workspaces.Rules = append([]WorkspaceRule(nil), p.Workspaces.Rules...)
	return p
}

// Presence alone is insufficient: docks can enumerate an output before it has
// a usable signal. Never force the laptop off on the strength of that alone.
func hasUsableExternalOutput(p Profile, monitors []hypr.Monitor) bool {
	resolver := NewMonitorResolver(monitors)
	for _, output := range p.Outputs {
		monitor, ok := resolver.ResolveOutput(output)
		if ok && output.Enabled && !monitor.IsInternal() && monitor.Name != "FALLBACK" &&
			!monitor.Disabled && monitor.DPMSStatus && monitor.Width > 0 && monitor.Height > 0 {
			return true
		}
	}
	return false
}

func closedLidWorkspaceTarget(p Profile, monitors []hypr.Monitor) workspaceTarget {
	resolver := NewMonitorResolver(monitors)
	for _, key := range orderedOutputKeys(p, monitors) {
		output, ok := p.OutputByKey(key)
		if !ok || !output.Enabled || output.MirrorOf != "" {
			continue
		}
		monitor, ok := resolver.ResolveOutput(output)
		if ok && !monitor.IsInternal() {
			return workspaceTarget{key: output.Key, name: firstNonEmpty(output.Name, monitor.Name)}
		}
	}

	for _, output := range p.Outputs {
		if !output.Enabled || output.MirrorOf != "" {
			continue
		}
		monitor, ok := resolver.ResolveOutput(output)
		if ok && !monitor.IsInternal() {
			return workspaceTarget{key: output.Key, name: firstNonEmpty(output.Name, monitor.Name)}
		}
	}

	matchCounts := hypr.MonitorMatchCounts(monitors)
	for _, monitor := range monitors {
		if monitor.IsInternal() {
			continue
		}
		return workspaceTarget{
			key:  hypr.MonitorOutputKey(monitor, matchCounts),
			name: monitor.Name,
		}
	}
	return workspaceTarget{}
}

func retargetInternalWorkspaceRules(settings *WorkspaceSettings, internalKeys map[string]bool, internalNames map[string]bool, target workspaceTarget) int {
	if settings == nil || !settings.Enabled || len(settings.Rules) == 0 || target.key == "" {
		return 0
	}

	retargeted := 0
	for idx := range settings.Rules {
		rule := &settings.Rules[idx]
		if !workspaceRuleTargetsInternal(*rule, internalKeys, internalNames) {
			continue
		}
		rule.OutputKey = target.key
		rule.OutputName = target.name
		retargeted++
	}
	return retargeted
}

func workspaceRuleTargetsInternal(rule WorkspaceRule, internalKeys map[string]bool, internalNames map[string]bool) bool {
	if internalKeys[strings.TrimSpace(rule.OutputKey)] {
		return true
	}
	return internalNames[strings.TrimSpace(rule.OutputName)]
}

func removeInternalMonitorOrderRefs(order []string, internalKeys map[string]bool, internalNames map[string]bool) []string {
	if len(order) == 0 {
		return nil
	}

	out := make([]string, 0, len(order))
	for _, ref := range order {
		ref = strings.TrimSpace(ref)
		if ref == "" || internalKeys[ref] || internalNames[ref] {
			continue
		}
		out = append(out, ref)
	}
	return out
}

// KeepBuiltInPanelOn is the other half of the closed-lid rule: the built-in
// panel may stay off only while an external display actually shows a picture.
// When every external the profile keeps on is modeless, asleep or missing, and
// a connected built-in panel would be off, it returns p with that panel on, to
// the right of the other enabled displays so nothing overlaps. Some drivers
// only bring a modeless external back once another output is lit; without
// this, a clamshell or external-only profile can leave the machine dark until
// the lid is opened. The returned names are the panels it turned on.
func KeepBuiltInPanelOn(p Profile, monitors []hypr.Monitor) (Profile, []string) {
	if hasUsableExternalOutput(p, monitors) {
		return p, nil
	}
	adjusted := cloneProfile(p)
	adjusted.Normalize()
	resolver := NewMonitorResolver(monitors)
	matchCounts := hypr.MonitorMatchCounts(monitors)

	right, top := 0, 0
	for _, output := range adjusted.Outputs {
		if !output.Enabled || output.MirrorOf != "" {
			continue
		}
		m := hypr.Monitor{Width: output.Width, Height: output.Height, Scale: output.Scale, Transform: output.Transform}
		w, _ := m.LogicalSize()
		if output.X+w > right {
			right, top = output.X+w, output.Y
		}
	}

	var turnedOn []string
	for _, monitor := range monitors {
		if !monitor.IsInternal() {
			continue
		}
		idx := -1
		for i := range adjusted.Outputs {
			if found, ok := resolver.ResolveOutput(adjusted.Outputs[i]); ok && found.Name == monitor.Name {
				idx = i
				break
			}
		}
		if idx >= 0 && adjusted.Outputs[idx].Enabled {
			return p, nil
		}
		output := OutputConfig{
			Key:      hypr.MonitorOutputKey(monitor, matchCounts),
			MatchKey: monitor.HardwareKey(),
			Name:     monitor.Name,
			Make:     monitor.Make,
			Model:    monitor.Model,
			Serial:   monitor.Serial,
		}
		if idx >= 0 {
			output = adjusted.Outputs[idx]
		}
		if output.Width <= 0 || output.Height <= 0 {
			for _, mode := range monitor.AvailableModes {
				if w, h, hz, ok := hypr.ParseMode(mode); ok && w*h > output.Width*output.Height {
					output.Mode, output.Width, output.Height, output.Refresh = mode, w, h, hz
				}
			}
		}
		if output.Width <= 0 || output.Height <= 0 {
			continue
		}
		if output.Scale <= 0 {
			output.Scale = 1
		}
		output.Enabled, output.MirrorOf = true, ""
		output.X, output.Y = right, top
		m := hypr.Monitor{Width: output.Width, Height: output.Height, Scale: output.Scale, Transform: output.Transform}
		w, _ := m.LogicalSize()
		right += w
		if idx >= 0 {
			adjusted.Outputs[idx] = output
		} else {
			adjusted.Outputs = append(adjusted.Outputs, output)
		}
		turnedOn = append(turnedOn, monitor.Name)
	}
	if len(turnedOn) == 0 {
		return p, nil
	}
	adjusted.Normalize()
	return adjusted, turnedOn
}

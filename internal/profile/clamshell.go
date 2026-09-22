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

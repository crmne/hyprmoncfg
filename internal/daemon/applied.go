package daemon

import (
	"reflect"
	"slices"

	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

// Keep the request and its resolved result separate. Preferred modes and
// compositor defaults need not read back verbatim; VRR activity and buffer
// formats can change without any configuration changing at all.
type appliedState struct {
	requested profile.Profile
	live      profile.Profile
}

func rememberApplied(requested profile.Profile, monitors []hypr.Monitor) *appliedState {
	requested.Outputs = slices.Clone(requested.Outputs)
	requested.Workspaces.MonitorOrder = slices.Clone(requested.Workspaces.MonitorOrder)
	requested.Workspaces.Rules = slices.Clone(requested.Workspaces.Rules)
	live := profile.FromMonitors(requested.Name, monitors)
	live.Workspaces = requested.Workspaces
	// Retain precise requested scales when hyprctl rounded their readback.
	resolver := profile.NewMonitorResolver(monitors)
	for _, output := range requested.Outputs {
		monitor, ok := resolver.ResolveOutput(output)
		if !ok || !profile.ScaleMatchesRoundedReadback(monitor.Width, monitor.Height, output.Scale, monitor.Scale) {
			continue
		}
		for i := range live.Outputs {
			if live.Outputs[i].Name == monitor.Name {
				live.Outputs[i].Scale = output.Scale
			}
		}
	}
	return &appliedState{requested: requested, live: live}
}

func (a *appliedState) matches(requested profile.Profile, monitors []hypr.Monitor, rules []hypr.WorkspaceRule) bool {
	if a == nil || a.requested.Name != requested.Name || a.requested.Exec != requested.Exec ||
		!reflect.DeepEqual(a.requested.Outputs, requested.Outputs) ||
		!reflect.DeepEqual(a.requested.Workspaces, requested.Workspaces) {
		return false
	}
	_, matches := profile.ExactStateMatch([]profile.Profile{a.live}, monitors, rules)
	return matches
}

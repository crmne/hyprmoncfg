package daemon

import (
	"errors"
	"reflect"
	"slices"

	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

var errAutomaticApplyRejected = errors.New("automatic layout was rejected; repeated applies are paused")

type rejectedAutomaticApply struct {
	requested profile.Profile
	liveHash  string
	rules     []hypr.WorkspaceRule
}

func rememberRejected(p profile.Profile, monitors []hypr.Monitor, rules []hypr.WorkspaceRule) *rejectedAutomaticApply {
	return &rejectedAutomaticApply{
		requested: rememberApplied(p, nil).requested,
		liveHash:  rejectedStateHash(monitors),
		rules:     slices.Clone(rules),
	}
}

func (r *rejectedAutomaticApply) matches(p profile.Profile, monitors []hypr.Monitor, rules []hypr.WorkspaceRule) bool {
	return r != nil && r.liveHash == rejectedStateHash(monitors) &&
		r.requested.Name == p.Name && r.requested.Exec == p.Exec &&
		r.requested.DisableUnknownOutputs == p.DisableUnknownOutputs &&
		reflect.DeepEqual(r.requested.Outputs, p.Outputs) &&
		reflect.DeepEqual(r.requested.Workspaces, p.Workspaces) &&
		reflect.DeepEqual(r.rules, rules) &&
		intendedOutputsUsable(p, monitors)
}

// A rejected mode is not a failed wake. Keep trying when an intended display
// is missing or modeless, but do not keep modesetting a usable restored setup.
func intendedOutputsUsable(p profile.Profile, monitors []hypr.Monitor) bool {
	resolver := profile.NewMonitorResolver(monitors)
	count := 0
	for _, out := range p.Outputs {
		if !out.Enabled {
			continue
		}
		m, ok := resolver.ResolveOutput(out)
		if !ok || m.Disabled || !m.DPMSStatus || m.Width <= 0 || m.Height <= 0 || m.Name == "FALLBACK" {
			return false
		}
		count++
	}
	return count > 0
}

// Existing usable displays are the base for an unsaved setup. Only displays
// without a usable live mode need automatic extension and mode selection.
func liveDraft(monitors []hypr.Monitor, rules []hypr.WorkspaceRule) profile.Profile {
	p := profile.FromState("draft", monitors, rules)
	resolver := profile.NewMonitorResolver(monitors)
	kept := p.Outputs[:0]
	for _, out := range p.Outputs {
		m, ok := resolver.ResolveOutput(out)
		if ok && !m.Disabled && m.Width > 0 && m.Height > 0 && m.Name != "FALLBACK" {
			kept = append(kept, out)
		}
	}
	p.Outputs = kept
	if profile.ValidateLayout(p.Outputs) != nil {
		// An overlapping discovery layout is not a usable arrangement to retain.
		p.Outputs = nil
	}
	return profile.ExtendConnected(p, monitors)
}

// VRR activity and the current scanout buffer format are live telemetry, not
// a person's request to retry an already rejected mode.
func rejectedStateHash(monitors []hypr.Monitor) string {
	stable := slices.Clone(monitors)
	for i := range stable {
		stable[i].VRR = 0
		stable[i].CurrentFormat = ""
	}
	return profile.MonitorStateHash(stable)
}

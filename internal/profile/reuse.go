package profile

import (
	"fmt"
	"math"
	"strings"

	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/scaling"
)

// ReuseLayout copies layout roles onto explicitly selected current outputs. It
// returns an unnamed draft only: the caller must preview and save it separately.
// Hardware identities and unassigned outputs always come from the live state.
func ReuseLayout(saved Profile, profiles []Profile, monitors []hypr.Monitor, rules []hypr.WorkspaceRule, mapping map[string]string) (Profile, []string, error) {
	saved = cloneForReuse(saved)
	saved.Normalize()
	draft := FromState("", monitors, rules)
	liveWorkspaceRules := append([]WorkspaceRule(nil), draft.Workspaces.Rules...)
	if len(draft.Outputs) == 0 {
		return Profile{}, nil, fmt.Errorf("no connected displays to reuse a layout on")
	}
	warnings := []string{}
	byTarget := make(map[string]int, len(draft.Outputs))
	byMonitor := make(map[string]hypr.Monitor, len(monitors))
	counts := hypr.MonitorMatchCounts(monitors)
	for _, monitor := range monitors {
		byMonitor[hypr.MonitorOutputKey(monitor, counts)] = monitor
	}
	for i, output := range draft.Outputs {
		byTarget[output.Key] = i
	}
	bySource := make(map[string]OutputConfig, len(saved.Outputs))
	savedIdentityCounts := make(map[string]int, len(saved.Outputs))
	for _, output := range saved.Outputs {
		bySource[output.Key] = output
		savedIdentityCounts[output.MatchIdentity()]++
	}
	// The template alone is not evidence that an uncertain current output is
	// the old physical display. Use only its unambiguous serial matches as a
	// fallback, including skipped roles that remain live. A separate current
	// exact/best profile then supplies settings by current output key, following
	// the ordinary editor's recovery policy without using the role mapping.
	trusted := Profile{}
	for _, output := range draft.Outputs {
		if source, ok := bySource[output.Key]; ok && reuseSameDisplay(source, output, savedIdentityCounts, counts) {
			trusted.Outputs = append(trusted.Outputs, source)
		}
	}
	PreserveUnreportedSettings(&draft, trusted)
	donors := make([]Profile, 0, len(profiles))
	for _, candidate := range profiles {
		if candidate.Name != saved.Name {
			donors = append(donors, candidate)
		}
	}
	if active, ok := ExactStateMatch(donors, monitors, rules); ok {
		PreserveUnreportedSettings(&draft, active)
	} else if best, _, ok := BestMatch(donors, monitors); ok {
		PreserveUnreportedSettings(&draft, best)
	}
	// Calibration belongs to the current hardware; unknown-display policy
	// belongs to the selected template, even when its donor has another policy.
	draft.DisableUnknownOutputs = saved.DisableUnknownOutputs
	for key := range mapping {
		if _, ok := bySource[key]; !ok {
			return Profile{}, nil, fmt.Errorf("saved display mapping is stale; reload the selected layout")
		}
	}
	used := make(map[string]bool, len(mapping))
	needsPlacement := make(map[string]bool)
	for _, source := range saved.Outputs {
		targetKey, specified := mapping[source.Key]
		if !specified && source.Enabled {
			return Profile{}, nil, fmt.Errorf("choose a current display or Skip for saved display %s", source.Name)
		}
		if targetKey == "" {
			warnings = append(warnings, fmt.Sprintf("Skipped saved display %s and its workspace assignments.", source.Name))
			continue
		}
		idx, ok := byTarget[targetKey]
		if !ok {
			return Profile{}, nil, fmt.Errorf("a selected display is no longer connected; refresh the hardware mapping")
		}
		if used[targetKey] {
			return Profile{}, nil, fmt.Errorf("each current display can fill only one saved layout role")
		}
		used[targetKey] = true
		current := draft.Outputs[idx]
		// Copy the layout, never the old machine/monitor identity.
		output := source
		output.Key, output.MatchKey = current.Key, current.MatchKey
		output.Name, output.Description = current.Name, current.Description
		output.Make, output.Model, output.Serial = current.Make, current.Model, current.Serial
		output.MirrorOf = "" // resolved only after all assignments are validated
		if !reuseModeSupported(source, byMonitor[targetKey]) {
			output.Mode, output.Width, output.Height, output.Refresh = current.Mode, current.Width, current.Height, current.Refresh
			if output.Width <= 0 || output.Height <= 0 {
				for _, mode := range byMonitor[targetKey].AvailableModes {
					if w, h, hz, ok := hypr.ParseMode(mode); ok && w > 0 && h > 0 {
						output.Mode, output.Width, output.Height, output.Refresh = mode, w, h, hz
						break
					}
				}
				if output.Enabled && (output.Width <= 0 || output.Height <= 0) {
					return Profile{}, nil, fmt.Errorf("no usable mode is available for %s yet; wait for the display to finish connecting", current.Name)
				}
			}
			warnings = append(warnings, fmt.Sprintf("%s: saved mode is unavailable; kept current mode %s.", current.Name, output.NormalizedMode()))
		}
		if adjusted, ok := scaling.ClosestSharp(output.Width, output.Height, output.Scale); ok && math.Abs(adjusted-output.Scale) > 0.00001 {
			output.Scale = adjusted
			warnings = append(warnings, fmt.Sprintf("%s: adjusted scale to %.5g for its display mode.", current.Name, adjusted))
		}
		oldWidth, oldHeight := source.LogicalSize()
		newWidth, newHeight := output.LogicalSize()
		if oldWidth != newWidth || oldHeight != newHeight {
			needsPlacement[targetKey] = true
		}
		// Color/ICC and hardware overrides are display-specific. Keep live
		// settings when identity changes, even for another unit of one model.
		if !reuseSameDisplay(source, current, savedIdentityCounts, counts) {
			output.VRR, output.Bitdepth, output.CM = current.VRR, current.Bitdepth, current.CM
			output.SDRBrightness, output.SDRSaturation = current.SDRBrightness, current.SDRSaturation
			output.SDRMinLuminance, output.SDRMaxLuminance = current.SDRMinLuminance, current.SDRMaxLuminance
			output.MinLuminance, output.MaxLuminance, output.MaxAvgLuminance = current.MinLuminance, current.MaxLuminance, current.MaxAvgLuminance
			output.SupportsWideColor, output.SupportsHDR = current.SupportsWideColor, current.SupportsHDR
			output.SDREOTF, output.ICC = current.SDREOTF, current.ICC
			warnings = append(warnings, fmt.Sprintf("%s: used current color and VRR settings, with HDR/ICC calibration recovered where known for this display.", current.Name))
		}
		draft.Outputs[idx] = output
	}
	for _, source := range saved.Outputs {
		targetKey := mapping[source.Key]
		if targetKey == "" || source.MirrorOf == "" {
			continue
		}
		idx := byTarget[targetKey]
		mirrorKey := mapping[source.MirrorOf]
		if mirrorKey == "" {
			needsPlacement[targetKey] = true
			warnings = append(warnings, fmt.Sprintf("%s: skipped mirror source; retained it as an independent display.", draft.Outputs[idx].Name))
			continue
		}
		mirror := draft.Outputs[byTarget[mirrorKey]]
		if !mirror.Enabled {
			needsPlacement[targetKey] = true
			warnings = append(warnings, fmt.Sprintf("%s: mirror source is disabled; made this display independent.", draft.Outputs[idx].Name))
			continue
		}
		draft.Outputs[idx].MirrorOf = mirrorKey
		draft.Outputs[idx].X, draft.Outputs[idx].Y = mirror.X, mirror.Y
	}
	// Copy workspace intent while rewriting every identity reference. Skipped
	// roles are dropped rather than handed to a same-named connector by fallback.
	settings := saved.Workspaces
	settings.MonitorOrder = nil
	settings.Rules = nil
	order := saved.Workspaces.MonitorOrder
	if settings.Strategy == WorkspaceStrategySequential || settings.Strategy == WorkspaceStrategyInterleave {
		// Resolve implicit/rule-derived order on the saved identities before
		// replacing their keys or appending unassigned current displays.
		order = orderedOutputKeys(saved, nil)
	}
	for _, key := range order {
		if target := mapping[key]; target != "" {
			settings.MonitorOrder = append(settings.MonitorOrder, target)
		}
	}
	for _, rule := range saved.Workspaces.Rules {
		if target := mapping[rule.OutputKey]; target != "" {
			rule.OutputKey = target
			rule.OutputName = draft.Outputs[byTarget[target]].Name
			settings.Rules = append(settings.Rules, rule)
		}
	}
	if settings.Strategy == "" || settings.Strategy == WorkspaceStrategyManual {
		claimedWorkspaces := make(map[string]bool)
		for _, rule := range settings.Rules {
			claimedWorkspaces[rule.Workspace] = true
		}
		for _, rule := range liveWorkspaceRules {
			if !used[rule.OutputKey] && !claimedWorkspaces[rule.Workspace] {
				settings.Rules = append(settings.Rules, rule)
				claimedWorkspaces[rule.Workspace] = true
			}
		}
	}
	draft.Workspaces = settings
	// Reusing geometry must not silently reuse an arbitrary execution hook.
	if strings.TrimSpace(saved.Exec) != "" {
		warnings = append(warnings, "The saved layout's Exec command was not copied.")
	}
	draft.Exec = ""
	// A live, unassigned mirror may have lost its source because that role
	// was disabled or became a mirror. Do not leave it dependent on an invalid
	// final source or a chain/cycle; make it independently usable instead.
	for i := range draft.Outputs {
		output := &draft.Outputs[i]
		if output.MirrorOf == "" {
			continue
		}
		targetIndex, exists := byTarget[output.MirrorOf]
		if !exists || targetIndex == i || !draft.Outputs[targetIndex].Enabled || draft.Outputs[targetIndex].MirrorOf != "" {
			output.MirrorOf = ""
			needsPlacement[output.Key] = true
			warnings = append(warnings, fmt.Sprintf("%s: mirror source is no longer independent and enabled; made this display independent.", output.Name))
		}
	}
	// Unassigned outputs stay live. Mirror repairs and mode/scale adjustments
	// may require more room for mapped outputs. Preserve all other mapped
	// positions and move only overlapping extras or adapted rectangles.
	for idx := range draft.Outputs {
		output := &draft.Outputs[idx]
		if used[output.Key] && !needsPlacement[output.Key] {
			continue
		}
		if !used[output.Key] {
			warnings = append(warnings, fmt.Sprintf("%s was not assigned a saved role; kept its current settings.", output.Name))
		}
		if output.Enabled && output.MirrorOf == "" && reuseOverlaps(*output, draft.Outputs) {
			right := output.X
			for _, other := range draft.Outputs {
				if other.Key != output.Key && other.Enabled && other.MirrorOf == "" {
					width, _ := other.LogicalSize()
					right = max(right, other.X+width)
				}
			}
			output.X = right
			warnings = append(warnings, fmt.Sprintf("%s was placed to the right to avoid overlapping the reused layout.", output.Name))
		}
		if !used[output.Key] && output.Enabled && output.MirrorOf == "" {
			draft.Workspaces.MonitorOrder = append(draft.Workspaces.MonitorOrder, output.Key)
		}
	}
	// A valid mirror follows its source if collision repair moved that source.
	for i := range draft.Outputs {
		output := &draft.Outputs[i]
		if output.MirrorOf != "" {
			source := draft.Outputs[byTarget[output.MirrorOf]]
			output.X, output.Y = source.X, source.Y
		}
	}
	// Validate the configured draft mode, not live DPMS/disabled state: reuse
	// can enable an inactive display using one of its advertised modes.
	usable := 0
	for _, output := range draft.Outputs {
		if output.Enabled && output.MirrorOf == "" && output.Width > 0 && output.Height > 0 &&
			!strings.EqualFold(strings.TrimSpace(output.Name), "FALLBACK") {
			usable++
		}
	}
	if usable == 0 {
		return Profile{}, nil, fmt.Errorf("the reused layout must keep at least one real independent display enabled with a usable mode")
	}
	draft.Normalize()
	if err := ValidateLayout(draft.Outputs); err != nil {
		return Profile{}, nil, fmt.Errorf("cannot reuse saved layout: %w", err)
	}
	return draft, warnings, nil
}

func reuseSameDisplay(source, current OutputConfig, sourceCounts, currentCounts map[string]int) bool {
	return source.MatchIdentity() == current.MatchIdentity() && strings.TrimSpace(source.Serial) != "" &&
		sourceCounts[source.MatchIdentity()] == 1 && currentCounts[current.MatchIdentity()] == 1
}

func cloneForReuse(p Profile) Profile {
	p.Outputs = append([]OutputConfig(nil), p.Outputs...)
	p.Workspaces.Rules = append([]WorkspaceRule(nil), p.Workspaces.Rules...)
	p.Workspaces.MonitorOrder = append([]string(nil), p.Workspaces.MonitorOrder...)
	return p
}

func reuseModeSupported(output OutputConfig, monitor hypr.Monitor) bool {
	modes := append([]string{monitor.ModeString()}, monitor.AvailableModes...)
	for _, mode := range modes {
		w, h, hz, ok := hypr.ParseMode(mode)
		if ok && w == output.Width && h == output.Height && math.Abs(hz-output.Refresh) <= 0.1 {
			return true
		}
	}
	return false
}

func reuseOverlaps(output OutputConfig, all []OutputConfig) bool {
	w, h := output.LogicalSize()
	for _, other := range all {
		if output.Key == other.Key || !other.Enabled || other.MirrorOf != "" {
			continue
		}
		ow, oh := other.LogicalSize()
		if output.X < other.X+ow && output.X+w > other.X && output.Y < other.Y+oh && output.Y+h > other.Y {
			return true
		}
	}
	return false
}

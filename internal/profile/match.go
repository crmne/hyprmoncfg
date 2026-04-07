package profile

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/crmne/hyprmoncfg/internal/hypr"
)

func MatchScore(p Profile, monitors []hypr.Monitor) int {
	p.normalizeIdentityRefs()
	if len(monitors) == 0 || len(p.Outputs) == 0 {
		return 0
	}

	connected := make(map[string]int, len(monitors))
	for _, m := range monitors {
		connected[m.HardwareKey()]++
	}

	profileEnabled := make(map[string]int, len(p.Outputs))
	profileKnown := make(map[string]int, len(p.Outputs))
	for _, o := range p.Outputs {
		matchKey := o.MatchIdentity()
		profileKnown[matchKey]++
		if o.Enabled {
			profileEnabled[matchKey]++
		}
	}
	if len(profileEnabled) == 0 {
		return 0
	}

	enabledMatch := 0
	disabledMatch := 0
	for key, connectedCount := range connected {
		enabledForKey := min(connectedCount, profileEnabled[key])
		enabledMatch += enabledForKey

		disabledKnown := profileKnown[key] - profileEnabled[key]
		if disabledKnown > 0 {
			disabledMatch += min(connectedCount-enabledForKey, disabledKnown)
		}
	}
	if enabledMatch == 0 {
		return 0
	}

	missingFromCurrent := 0
	for key, wanted := range profileEnabled {
		missingFromCurrent += max(0, wanted-connected[key])
	}
	unknownCurrent := 0
	for key, connectedCount := range connected {
		unknownCurrent += max(0, connectedCount-profileKnown[key])
	}

	// High reward for enabled match, moderate reward for disabled match,
	// moderate penalty for mismatch.
	return enabledMatch*100 + disabledMatch*50 - missingFromCurrent*30 - unknownCurrent*20
}

func BestMatch(profiles []Profile, monitors []hypr.Monitor) (Profile, int, bool) {
	type candidate struct {
		profile Profile
		score   int
	}
	candidates := make([]candidate, 0, len(profiles))
	for _, p := range profiles {
		score := MatchScore(p, monitors)
		if score <= 0 {
			continue
		}
		candidates = append(candidates, candidate{profile: p, score: score})
	}
	if len(candidates) == 0 {
		return Profile{}, 0, false
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return strings.ToLower(candidates[i].profile.Name) < strings.ToLower(candidates[j].profile.Name)
	})
	return candidates[0].profile, candidates[0].score, true
}

func ExactStateMatch(profiles []Profile, monitors []hypr.Monitor, rules []hypr.WorkspaceRule) (Profile, bool) {
	if len(profiles) == 0 || len(monitors) == 0 {
		return Profile{}, false
	}

	current := FromState("", monitors, rules)
	var match Profile
	matches := 0
	for _, candidate := range profiles {
		if !profilesShareEffectiveState(candidate, current, monitors) {
			continue
		}
		match = candidate
		matches++
		if matches > 1 {
			return Profile{}, false
		}
	}

	if matches == 1 {
		return match, true
	}
	return Profile{}, false
}

func profilesShareEffectiveState(a, b Profile, monitors []hypr.Monitor) bool {
	a.normalizeIdentityRefs()
	b.normalizeIdentityRefs()
	if !outputsShareEffectiveState(a.Outputs, b.Outputs) {
		return false
	}

	aRules := ResolveWorkspaceRules(a, monitors)
	bRules := ResolveWorkspaceRules(b, monitors)
	if len(aRules) != len(bRules) {
		return false
	}
	for idx := range aRules {
		if aRules[idx].Workspace != bRules[idx].Workspace {
			return false
		}
		if !workspaceRuleTargetsEqual(aRules[idx], bRules[idx]) {
			return false
		}
		if aRules[idx].Default != bRules[idx].Default || aRules[idx].Persistent != bRules[idx].Persistent {
			return false
		}
	}
	return true
}

func outputsShareEffectiveState(a, b []OutputConfig) bool {
	if len(a) != len(b) {
		return false
	}

	byKey := make(map[string]OutputConfig, len(a))
	for _, output := range a {
		byKey[output.Key] = output
	}

	for _, output := range b {
		left, ok := byKey[output.Key]
		if !ok {
			return false
		}
		if !outputConfigsShareEffectiveState(left, output) {
			return false
		}
	}
	return true
}

func outputConfigsShareEffectiveState(a, b OutputConfig) bool {
	if a.Key != b.Key || a.Enabled != b.Enabled {
		return false
	}
	if !a.Enabled {
		return true
	}

	// Only compare fields that hyprctl accurately reports. Config-only
	// fields (VRR mode, EDID luminance/overrides, SDR EOTF, ICC) are
	// excluded because FromState cannot populate them from live state.
	return a.NormalizedMode() == b.NormalizedMode() &&
		a.X == b.X &&
		a.Y == b.Y &&
		clampStateScale(a.Scale) == clampStateScale(b.Scale) &&
		a.Transform == b.Transform &&
		a.Bitdepth == b.Bitdepth &&
		a.CM == b.CM &&
		a.SDRBrightness == b.SDRBrightness &&
		a.SDRSaturation == b.SDRSaturation &&
		a.SDRMinLuminance == b.SDRMinLuminance &&
		a.SDRMaxLuminance == b.SDRMaxLuminance &&
		firstNonEmpty(a.MirrorOf, "") == firstNonEmpty(b.MirrorOf, "")
}

func MonitorSetHash(monitors []hypr.Monitor) string {
	if len(monitors) == 0 {
		return "none"
	}
	keys := make([]string, 0, len(monitors))
	for _, m := range monitors {
		keys = append(keys, m.HardwareKey())
	}
	sort.Strings(keys)
	return strings.Join(keys, ",")
}

func MonitorStateHash(monitors []hypr.Monitor) string {
	if len(monitors) == 0 {
		return "none"
	}

	states := make([]string, 0, len(monitors))
	for _, m := range monitors {
		states = append(states, monitorStateSignature(m))
	}
	sort.Strings(states)
	return strings.Join(states, ",")
}

func monitorStateSignature(m hypr.Monitor) string {
	return fmt.Sprintf(
		"%s|%s|disabled=%t|%dx%d@%.2f|%dx%d|scale=%s|transform=%d|vrr=%d|fmt=%s|cm=%s|sdrbr=%.2f|sdrsat=%.2f|sdrmin=%.3f|sdrmax=%d",
		m.HardwareKey(),
		strings.ToLower(strings.TrimSpace(m.Name)),
		m.Disabled,
		m.Width,
		m.Height,
		m.RefreshRate,
		m.X,
		m.Y,
		strconv.FormatFloat(clampStateScale(m.Scale), 'f', 3, 64),
		m.Transform,
		m.VRR,
		m.CurrentFormat,
		m.ColorManagementPreset,
		m.SDRBrightness,
		m.SDRSaturation,
		m.SDRMinLuminance,
		m.SDRMaxLuminance,
	)
}

func clampStateScale(scale float64) float64 {
	if scale <= 0 {
		return 1
	}
	return scale
}

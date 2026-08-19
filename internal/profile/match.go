package profile

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/scaling"
)

type MatchResult struct {
	Score                   int
	ConnectedEnabledOutputs int

	// Score breakdown, so callers can explain the number they show.
	// Every field counts outputs, not points.
	EnabledMatched    int // profile outputs enabled here and currently connected
	DisabledMatched   int // profile outputs kept off here and currently connected
	MissingOutputs    int // profile outputs enabled here but not connected
	MissingOffOutputs int // profile outputs kept off here and not connected either
	UnknownOutputs    int // connected outputs this profile says nothing about
}

// Score weights, exported so the breakdown can be rendered as arithmetic.
// Every output a profile names but cannot find costs something, so the profile
// that describes exactly the connected displays wins over one that also
// carries rules for displays that are not here.
const (
	ScoreEnabledMatch     = 100
	ScoreDisabledMatch    = 50
	ScoreMissingOutput    = -30
	ScoreMissingOffOutput = -10
	ScoreUnknownOutput    = -20
)

func EvaluateMatch(p Profile, monitors []hypr.Monitor) MatchResult {
	p.Normalize()
	if len(monitors) == 0 || len(p.Outputs) == 0 {
		return MatchResult{}
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
		return MatchResult{}
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

	missingFromCurrent := 0
	for key, wanted := range profileEnabled {
		missingFromCurrent += max(0, wanted-connected[key])
	}
	missingOffFromCurrent := 0
	for key, known := range profileKnown {
		wantedOff := known - profileEnabled[key]
		if wantedOff <= 0 {
			continue
		}
		// Connected outputs are claimed by the profile's enabled entries
		// first, so only what is left over can cover the disabled ones.
		spare := max(0, connected[key]-min(connected[key], profileEnabled[key]))
		missingOffFromCurrent += max(0, wantedOff-spare)
	}
	unknownCurrent := 0
	for key, connectedCount := range connected {
		unknownCurrent += max(0, connectedCount-profileKnown[key])
	}

	result := MatchResult{
		EnabledMatched:    enabledMatch,
		DisabledMatched:   disabledMatch,
		MissingOutputs:    missingFromCurrent,
		MissingOffOutputs: missingOffFromCurrent,
		UnknownOutputs:    unknownCurrent,
	}
	// A profile that would leave every connected output off is not a
	// candidate at all, so it keeps a zero score and no partial credit.
	if enabledMatch == 0 {
		return result
	}

	// High reward for enabled match, moderate reward for disabled match,
	// moderate penalty for mismatch.
	result.Score = enabledMatch*ScoreEnabledMatch + disabledMatch*ScoreDisabledMatch +
		missingFromCurrent*ScoreMissingOutput + missingOffFromCurrent*ScoreMissingOffOutput +
		unknownCurrent*ScoreUnknownOutput
	result.ConnectedEnabledOutputs = enabledMatch
	return result
}

func MatchScore(p Profile, monitors []hypr.Monitor) int {
	return EvaluateMatch(p, monitors).Score
}

func BestMatch(profiles []Profile, monitors []hypr.Monitor) (Profile, int, bool) {
	type candidate struct {
		profile Profile
		score   int
	}
	candidates := make([]candidate, 0, len(profiles))
	for _, p := range profiles {
		score := EvaluateMatch(p, monitors).Score
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
	a.Normalize()
	b.Normalize()
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
	// A mirroring output shows its source's image, and Hyprland places and
	// drives it as it sees fit: the position it reports is not the one we
	// asked for. Only the mirror target is ours to compare, which is how
	// apply verification already treats it.
	if a.MirrorOf != "" || b.MirrorOf != "" {
		return a.MirrorOf == b.MirrorOf
	}

	// Only compare fields that hyprctl accurately reports. Config-only
	// fields (VRR mode, EDID luminance/overrides, SDR EOTF, ICC) are
	// excluded because FromState cannot populate them from live state.
	return a.NormalizedMode() == b.NormalizedMode() &&
		a.X == b.X &&
		a.Y == b.Y &&
		stateScalesEqual(a.Width, a.Height, a.Scale, b.Scale) &&
		a.Transform == b.Transform &&
		effectiveBitdepth(a.Bitdepth) == effectiveBitdepth(b.Bitdepth) &&
		effectiveCM(a.CM) == effectiveCM(b.CM) &&
		effectiveSDRMultiplier(a.SDRBrightness) == effectiveSDRMultiplier(b.SDRBrightness) &&
		effectiveSDRMultiplier(a.SDRSaturation) == effectiveSDRMultiplier(b.SDRSaturation) &&
		a.SDRMinLuminance == b.SDRMinLuminance &&
		a.SDRMaxLuminance == b.SDRMaxLuminance
}

func effectiveBitdepth(value int) int {
	if value == 0 {
		return 8
	}
	return value
}

func effectiveCM(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "srgb"
	}
	return value
}

func effectiveSDRMultiplier(value float64) float64 {
	if value == 0 {
		return 1
	}
	return value
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
		strconv.FormatFloat(normalizeStateScale(m.Width, m.Height, m.Scale), 'f', 5, 64),
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

func normalizeStateScale(width, height int, scale float64) float64 {
	return scaling.Round(scaling.Default(scale))
}

func stateScalesEqual(width, height int, a, b float64) bool {
	a = normalizeStateScale(width, height, a)
	b = normalizeStateScale(width, height, b)
	return a == b || ScaleMatchesRoundedReadback(width, height, a, b)
}

func ScaleMatchesRoundedReadback(width, height int, savedScale, reportedScale float64) bool {
	savedScale = normalizeStateScale(width, height, savedScale)
	reportedScale = normalizeStateScale(width, height, reportedScale)
	if savedScale == reportedScale {
		return true
	}
	if !scaling.Sharp(width, height, savedScale) || scaling.Sharp(width, height, reportedScale) {
		return false
	}
	return roundScaleForHyprlandReadback(savedScale) == roundScaleForHyprlandReadback(reportedScale)
}

func roundScaleForHyprlandReadback(scale float64) float64 {
	return math.Round(scale*100) / 100
}

// AbsorbRequestedScales copies connector scales from requested into the
// matching enabled outputs when Hyprland has already landed on that value.
// Position, mode, and enablement stay as the profile saved them.
func AbsorbRequestedScales(p Profile, monitors []hypr.Monitor, requested map[string]float64) (Profile, []string) {
	p.Normalize()
	if len(requested) == 0 || len(p.Outputs) == 0 || len(monitors) == 0 {
		return p, nil
	}

	resolver := NewMonitorResolver(monitors)
	updated := p
	updated.Outputs = append([]OutputConfig(nil), p.Outputs...)
	var changed []string

	for i, output := range updated.Outputs {
		if !output.Enabled {
			continue
		}
		monitor, ok := resolver.ResolveOutput(output)
		if !ok || monitor.Disabled {
			continue
		}
		want, ok := requested[monitor.Name]
		if !ok || want <= 0 {
			continue
		}
		if !stateScalesEqual(monitor.Width, monitor.Height, monitor.Scale, want) {
			continue
		}
		if stateScalesEqual(monitor.Width, monitor.Height, output.Scale, want) {
			continue
		}
		updated.Outputs[i].Scale = want
		changed = append(changed, monitor.Name)
	}
	if len(changed) == 0 {
		return p, nil
	}
	return updated, changed
}

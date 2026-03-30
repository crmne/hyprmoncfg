package profile

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/crmne/hyprmoncfg/internal/hypr"
)

func MatchScore(p Profile, monitors []hypr.Monitor) int {
	if len(monitors) == 0 || len(p.Outputs) == 0 {
		return 0
	}

	connected := make(map[string]struct{}, len(monitors))
	for _, m := range monitors {
		connected[m.HardwareKey()] = struct{}{}
	}

	profileEnabled := make(map[string]struct{})
	profileKnown := make(map[string]struct{})
	for _, o := range p.Outputs {
		profileKnown[o.Key] = struct{}{}
		if o.Enabled {
			profileEnabled[o.Key] = struct{}{}
		}
	}
	if len(profileEnabled) == 0 {
		return 0
	}

	enabledMatch := 0
	for key := range connected {
		if _, ok := profileEnabled[key]; ok {
			enabledMatch++
		}
	}
	if enabledMatch == 0 {
		return 0
	}

	disabledMatch := 0
	for key := range connected {
		if _, inKnown := profileKnown[key]; inKnown {
			if _, inEnabled := profileEnabled[key]; !inEnabled {
				disabledMatch++
			}
		}
	}

	missingFromCurrent := len(profileEnabled) - enabledMatch
	unknownCurrent := 0
	for key := range connected {
		if _, ok := profileKnown[key]; !ok {
			unknownCurrent++
		}
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
		"%s|%s|disabled=%t|%dx%d@%.2f|%dx%d|scale=%s|transform=%d|vrr=%t",
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
	)
}

func clampStateScale(scale float64) float64 {
	if scale <= 0 {
		return 1
	}
	return scale
}

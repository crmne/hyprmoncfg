package profile

import (
	"fmt"
	"math"

	"github.com/crmne/hyprmoncfg/internal/hypr"
)

// FallbackStep is how far a display that will not stay on in its saved
// settings has been stepped down. Steps are cumulative and only ever change
// VRR and refresh rate: resolution, scale and position stay, so no other
// display or window moves.
type FallbackStep int

const (
	FallbackNone FallbackStep = iota
	FallbackVRROff
	FallbackLowerRefresh
	FallbackSafeRefresh
)

// safeRefresh is the rate every display supports at its native resolution.
const safeRefresh = 60.0

// OutputWithFallback returns out as it should be applied at step, choosing
// refresh rates from the live monitor's advertised modes.
func OutputWithFallback(out OutputConfig, monitor hypr.Monitor, step FallbackStep) OutputConfig {
	if step <= FallbackNone || !out.Enabled || out.MirrorOf != "" {
		return out
	}
	out.VRR = 0
	if step < FallbackLowerRefresh {
		return out
	}

	width, height := out.Width, out.Height
	saved := out.Refresh
	if w, h, hz, ok := hypr.ParseMode(out.Mode); ok {
		width, height, saved = w, h, hz
	}
	chosenMode, chosen := "", 0.0
	for _, mode := range monitor.AvailableModes {
		w, h, hz, ok := hypr.ParseMode(mode)
		if !ok || w != width || h != height || hz <= 0 {
			continue
		}
		if step == FallbackLowerRefresh {
			// The fastest rate clearly below the saved one.
			if hz < saved-0.5 && hz > chosen {
				chosenMode, chosen = mode, hz
			}
			continue
		}
		// The rate closest to 60 Hz, preferring one not above it.
		if chosenMode == "" || safeRefreshBetter(hz, chosen) {
			chosenMode, chosen = mode, hz
		}
	}
	if chosenMode == "" || (step == FallbackSafeRefresh && chosen >= saved-0.5) {
		return out
	}
	out.Mode, out.Refresh = chosenMode, chosen
	out.Width, out.Height = width, height
	return out
}

func safeRefreshBetter(candidate, current float64) bool {
	candidateOver, currentOver := candidate > safeRefresh+0.1, current > safeRefresh+0.1
	if candidateOver != currentOver {
		return !candidateOver
	}
	return math.Abs(candidate-safeRefresh) < math.Abs(current-safeRefresh)
}

// NextFallback returns the first step after current that actually changes
// what is applied, and false when no gentler setting is left to try.
func NextFallback(out OutputConfig, monitor hypr.Monitor, current FallbackStep) (FallbackStep, bool) {
	applied := OutputWithFallback(out, monitor, current)
	for step := current + 1; step <= FallbackSafeRefresh; step++ {
		next := OutputWithFallback(out, monitor, step)
		if next.VRR != applied.VRR || next.NormalizedMode() != applied.NormalizedMode() {
			return step, true
		}
	}
	return current, false
}

// DescribeFallback says in plain words how a stepped-down output runs.
func DescribeFallback(out OutputConfig, monitor hypr.Monitor, step FallbackStep) string {
	applied := OutputWithFallback(out, monitor, step)
	refresh := ""
	if applied.NormalizedMode() != out.NormalizedMode() {
		refresh = fmt.Sprintf("at %.0f Hz", applied.Refresh)
	}
	switch {
	case refresh == "":
		return "without VRR"
	case out.VRR != 0:
		return refresh + " without VRR"
	default:
		return refresh
	}
}

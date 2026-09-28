package scaling

import "sort"

// SharpChoices is the scale list an editor offers for a mode: every sharp
// scale from 1 to MaxScale, in order, plus the sharp scale closest to current
// when that falls outside the list (a sub-1 scale, for example). It is the
// single source for the daemon's editor scale_options, which the Omarchy
// panel uses, and for the TUI's Scale arrows and full scale list.
func SharpChoices(width, height int, current float64) []float64 {
	options := GridScales(width, height, 1, MaxScale)
	candidate := Round(current)
	if !Sharp(width, height, candidate) {
		var ok bool
		candidate, ok = ClosestSharp(width, height, candidate)
		if !ok {
			return options
		}
	}
	for _, option := range options {
		if option == candidate {
			return options
		}
	}
	options = append(options, candidate)
	sort.Float64s(options)
	return options
}

// Choices is SharpChoices with the exact current scale added when it is not
// already one of them, so a saved non-sharp scale stays selectable as itself
// and is never rewritten. The panel builds the same list from scale_options
// plus its current value.
func Choices(width, height int, current float64) []float64 {
	options := SharpChoices(width, height, current)
	if current <= 0 {
		return options
	}
	exact := Round(current)
	for _, option := range options {
		if option == exact {
			return options
		}
	}
	options = append(options, exact)
	sort.Float64s(options)
	return options
}

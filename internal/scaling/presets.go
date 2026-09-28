package scaling

import "sort"

// Presets is the short scale row both hyprmoncfg editors show, after the
// scale pills of Omarchy's own Display panel, each preset mapped onto the
// selected display by CleanScale. Very wide modes also get 4x (see
// presetsFor).
var Presets = []float64{1, 1.25, 1.5, 1.6, 2, 3}

// presetsFor adds 4x for modes at least 5120 pixels wide whose sharp list
// reaches 4, where a 4x desktop is still a usable size.
func presetsFor(width int, sharp []float64) []float64 {
	presets := append([]float64(nil), Presets...)
	if width >= 5120 && len(sharp) > 0 && sharp[len(sharp)-1] == 4 {
		presets = append(presets, 4)
	}
	return presets
}

// CleanScale is Omarchy's cleanScale: the smallest sharp scale at or above
// the requested one on Hyprland's 1/120 grid, capped at the largest scale
// that divides the mode. It rounds up, never down, so a preset never makes
// the desktop larger than asked.
func CleanScale(width, height int, requested float64) (float64, bool) {
	if width <= 0 || height <= 0 || requested <= 0 {
		return 0, false
	}
	divisor := gcd(width*hyprlandScaleSteps, height*hyprlandScaleSteps)
	units := int(roundHalfUp(requested * hyprlandScaleSteps))
	if units > divisor {
		units = divisor
	}
	if units < 1 {
		units = 1
	}
	for divisor%units != 0 {
		units++
	}
	return Round(float64(units) / hyprlandScaleSteps), true
}

// PresetChoices maps the presets onto a mode as Omarchy's availableScales
// does: presets that land on the same scale (compared at two decimals) keep
// only the one requested closest to it, in preset order. A preset that lands
// above the mode's largest listed sharp scale is dropped.
func PresetChoices(width, height int) []float64 {
	sharp := GridScales(width, height, 1, MaxScale)
	largest := 0.0
	if len(sharp) > 0 {
		largest = sharp[len(sharp)-1]
	}
	type candidate struct {
		value    float64
		index    int
		distance float64
	}
	byKey := map[float64]candidate{}
	for index, requested := range presetsFor(width, sharp) {
		effective, ok := CleanScale(width, height, requested)
		if !ok || effective > largest {
			continue
		}
		key := roundHalfUp(effective*100) / 100
		distance := requested - effective
		if distance < 0 {
			distance = -distance
		}
		if existing, seen := byKey[key]; !seen || distance < existing.distance {
			byKey[key] = candidate{value: effective, index: index, distance: distance}
		}
	}
	kept := make([]candidate, 0, len(byKey))
	for _, c := range byKey {
		kept = append(kept, c)
	}
	sort.Slice(kept, func(i, j int) bool { return kept[i].index < kept[j].index })
	out := make([]float64, len(kept))
	for i, c := range kept {
		out[i] = c.value
	}
	return out
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

// roundHalfUp matches JavaScript's Math.round for the positive values here.
func roundHalfUp(value float64) float64 {
	return float64(int64(value + 0.5))
}

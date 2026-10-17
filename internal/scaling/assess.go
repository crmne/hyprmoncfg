package scaling

import (
	"fmt"
	"math"
)

// Assessment is everything an editor says about one scale on one mode: the
// scale as stored, whether it is sharp, the logical size it gives, and the
// closest sharp scale with its size when it is not. Both editors build their
// wording from it, so neither recomputes sharpness on its own.
type Assessment struct {
	Scale          float64     `json:"scale"`
	Sharp          bool        `json:"sharp"`
	Logical        [2]float64  `json:"logical"`
	Closest        float64     `json:"closest,omitempty"`
	ClosestLogical *[2]float64 `json:"closest_logical,omitempty"`
	// Apply is what typed entry applies: the scale when it is sharp,
	// otherwise the closest sharp scale when there is one.
	Apply float64 `json:"apply"`
}

// ErrScaleRange is returned for a scale outside what Hyprland accepts.
var ErrScaleRange = fmt.Errorf("scale must be between %s and %s", Format(MinScale), Format(MaxScale))

// Assess checks a typed or stored scale against a mode. Scales outside
// MinScale to MaxScale are refused; the rest are rounded as profiles store
// them.
func Assess(width, height int, value float64) (Assessment, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < MinScale || value > MaxScale {
		return Assessment{}, ErrScaleRange
	}
	scale := Round(value)
	w, h := LogicalSize(width, height, scale)
	result := Assessment{Scale: scale, Sharp: Sharp(width, height, scale), Logical: [2]float64{w, h}, Apply: scale}
	if result.Sharp {
		// Hyprland snaps a sharp scale to whole logical pixels.
		result.Logical = [2]float64{math.Round(w), math.Round(h)}
		return result, nil
	}
	if closest, ok := ClosestSharp(width, height, scale); ok {
		cw, ch := LogicalSize(width, height, closest)
		result.Closest, result.ClosestLogical, result.Apply = closest, &[2]float64{math.Round(cw), math.Round(ch)}, closest
	}
	return result, nil
}

// Info is the Scale row for one mode and scale: the preset pills, the full
// list More shows, and the assessment of the scale itself.
type Info struct {
	Assessment
	Presets []float64 `json:"presets"`
	Options []float64 `json:"options"`
}

// Describe builds Info for a mode and its current scale.
func Describe(width, height int, value float64) (Info, error) {
	assessment, err := Assess(width, height, value)
	if err != nil {
		return Info{}, err
	}
	presets := PresetChoices(width, height)
	if presets == nil {
		presets = []float64{}
	}
	options := SharpChoices(width, height, value)
	if options == nil {
		options = []float64{}
	}
	return Info{Assessment: assessment, Presets: presets, Options: options}, nil
}

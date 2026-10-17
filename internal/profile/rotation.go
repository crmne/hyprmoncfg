package profile

import (
	"errors"

	"github.com/crmne/hyprmoncfg/internal/hypr"
)

// A built-in panel set to rotate automatically follows the device's
// orientation sensor. Its saved transform is how the panel is turned when the
// device is upright; the sensor's quarter turns add to it. The saved profile
// always describes the upright layout. The orientation is live state that the
// daemon applies on top, as it does with display fallbacks.

// RotateTransform turns a Hyprland transform by quarter turns
// counterclockwise, keeping its flip.
func RotateTransform(transform, quarters int) int {
	if transform < 0 || transform > 7 {
		transform = 0
	}
	turns := ((transform%4+quarters)%4 + 4) % 4
	return transform&4 | turns
}

// CanRotateAutomatically reports whether an output can follow the orientation
// sensor: only a built-in panel turns with the device.
func CanRotateAutomatically(output OutputConfig) bool {
	return (hypr.Monitor{Name: output.Name}).IsInternal()
}

var errRotationAutoExternal = errors.New("only a built-in panel can rotate with the device")

// Oriented returns the layout an upright profile has when the device is
// turned by quarters: each enabled output that rotates automatically gets
// the extra quarter turns, and attached displays move with its new size the
// way a resize edit moves them. Other outputs keep their settings.
func Oriented(p Profile, quarters int) Profile {
	quarters = ((quarters % 4) + 4) % 4
	if quarters == 0 {
		return p
	}
	rotated := false
	for _, output := range p.Outputs {
		if output.RotationAuto && output.Enabled && output.MirrorOf == "" {
			rotated = true
			break
		}
	}
	if !rotated {
		return p
	}
	p.Outputs = append([]OutputConfig(nil), p.Outputs...)
	for idx := range p.Outputs {
		output := &p.Outputs[idx]
		if !output.RotationAuto || !output.Enabled || output.MirrorOf != "" {
			continue
		}
		oldWidth, oldHeight := output.LogicalSize()
		output.Transform = RotateTransform(output.Transform, quarters)
		ReflowAfterResize(p.Outputs, idx, oldWidth, oldHeight)
	}
	return p
}

// OrientedProfiles orients every profile, for matching live state taken
// while the device is turned.
func OrientedProfiles(profiles []Profile, quarters int) []Profile {
	if ((quarters%4)+4)%4 == 0 {
		return profiles
	}
	oriented := make([]Profile, len(profiles))
	for idx, p := range profiles {
		oriented[idx] = Oriented(p, quarters)
	}
	return oriented
}

// uprightDraft turns a draft read from a turned device back upright. With an
// exact saved match the saved geometry is the upright one; otherwise each
// automatically rotating output loses the sensor's quarter turns and the
// rest stays where it is.
func uprightDraft(draft *Profile, quarters int, exact *Profile) {
	if ((quarters%4)+4)%4 == 0 {
		return
	}
	for idx := range draft.Outputs {
		output := &draft.Outputs[idx]
		if exact != nil {
			if saved, ok := exact.OutputByKey(output.Key); ok {
				output.X, output.Y, output.Transform = saved.X, saved.Y, saved.Transform
				output.RotationAuto = saved.RotationAuto
			}
			continue
		}
		if output.RotationAuto {
			output.Transform = RotateTransform(output.Transform, -quarters)
		}
	}
}

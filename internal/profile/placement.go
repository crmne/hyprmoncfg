package profile

import "fmt"

// Canvas placement shared by every layout editor. The Omarchy panel reaches
// these rules through ApplyEditorEdit (a drag release sends x, y and a snap
// distance); the TUI calls them directly. Keeping one implementation is what
// makes a drop, a nudge, or Place beside land in the same spot in both.
//
// The rules:
//   - A drop (snap distance > 0) snaps each axis to the nearest neighbour edge
//     or alignment within the distance (ApplySnap), then, if the display still
//     covers another one, moves it to the nearest clear edge
//     (PlaceOutsideOverlaps). If no clear edge exists, the drop is refused and
//     the display returns to where it was.
//   - An exact move (nudge, typed coordinate, Place beside, move to 0,0) is
//     taken as given, or refused if it would overlap another display.
//   - A display that is off can be moved freely; a mirror cannot be moved.
//   - Place beside puts the display flush against the nearest enabled,
//     independent display on the requested side, centred on the other axis.

// PlaceDirection names the side for Place beside.
type PlaceDirection int

const (
	PlaceLeft PlaceDirection = iota
	PlaceRight
	PlaceAbove
	PlaceBelow
)

// NearestAnchor is the enabled, independent display whose centre is closest
// to the selected display's centre, or -1 when there is none. Ties keep the
// earlier display, as the panel's snapAnchorName does.
func NearestAnchor(outputs []OutputConfig, index int) int {
	if index < 0 || index >= len(outputs) || !placeable(outputs[index]) {
		return -1
	}
	selected := outputs[index]
	width, height := selected.LogicalSize()
	centerX := int64(selected.X)*2 + int64(width)
	centerY := int64(selected.Y)*2 + int64(height)

	nearest := -1
	var nearestDistance int64
	for idx, other := range outputs {
		if idx == index || !placeable(other) {
			continue
		}
		otherWidth, otherHeight := other.LogicalSize()
		dx := centerX - (int64(other.X)*2 + int64(otherWidth))
		dy := centerY - (int64(other.Y)*2 + int64(otherHeight))
		distance := dx*dx + dy*dy
		if nearest < 0 || distance < nearestDistance {
			nearest, nearestDistance = idx, distance
		}
	}
	return nearest
}

// BesidePosition is where Place beside puts the selected display, and the
// anchor it placed it against. It does not move anything.
func BesidePosition(outputs []OutputConfig, index int, direction PlaceDirection) (x, y, anchor int, ok bool) {
	anchor = NearestAnchor(outputs, index)
	if anchor < 0 {
		return 0, 0, -1, false
	}
	width, height := outputs[index].LogicalSize()
	other := outputs[anchor]
	otherWidth, otherHeight := other.LogicalSize()
	switch direction {
	case PlaceLeft:
		return other.X - width, other.Y + (otherHeight-height)/2, anchor, true
	case PlaceRight:
		return other.X + otherWidth, other.Y + (otherHeight-height)/2, anchor, true
	case PlaceAbove:
		return other.X + (otherWidth-width)/2, other.Y - height, anchor, true
	case PlaceBelow:
		return other.X + (otherWidth-width)/2, other.Y + otherHeight, anchor, true
	default:
		return 0, 0, -1, false
	}
}

// ResolveDrop applies the drop rules to a display already moved to the
// release position: snap within snapDistance, then leave any overlap by the
// nearest clear edge. It reports the snap analysis for edge highlighting.
func ResolveDrop(outputs []OutputConfig, index, snapDistance int) SnapAnalysis {
	analysis := ApplySnap(outputs, index, snapDistance)
	PlaceOutsideOverlaps(outputs, index)
	return analysis
}

// MoveOutput moves one display to x, y. With a positive snap distance the move
// is a pointer drop and ResolveDrop may adjust it; otherwise it is exact. A
// move that would leave the display covering another is refused: the display
// keeps its previous position and the overlap is returned as the error.
func MoveOutput(outputs []OutputConfig, index, x, y, snapDistance int) (SnapAnalysis, error) {
	if index < 0 || index >= len(outputs) {
		return SnapAnalysis{}, fmt.Errorf("no display at index %d", index)
	}
	if outputs[index].MirrorOf != "" {
		return SnapAnalysis{}, fmt.Errorf("%s mirrors another display and follows it", displayName(outputs[index]))
	}
	oldX, oldY := outputs[index].X, outputs[index].Y
	outputs[index].X, outputs[index].Y = x, y
	if !outputs[index].Enabled {
		// An off display occupies nothing; its position is kept for when it
		// is turned on again, exactly as typed.
		return SnapAnalysis{}, nil
	}
	var analysis SnapAnalysis
	if snapDistance > 0 {
		analysis = ResolveDrop(outputs, index, snapDistance)
	}
	if other := overlappedBy(outputs, index); other >= 0 {
		outputs[index].X, outputs[index].Y = oldX, oldY
		return analysis, fmt.Errorf("%s would overlap %s", displayName(outputs[index]), displayName(outputs[other]))
	}
	return analysis, nil
}

// RejectNewOverlap reports an overlap that an edit introduced. A layout that
// already overlapped before the edit is not blocked, so the edit that fixes it
// can go through.
func RejectNewOverlap(before, after []OutputConfig) error {
	if ValidateLayout(before) != nil {
		return nil
	}
	return ValidateLayout(after)
}

func placeable(output OutputConfig) bool {
	return output.Enabled && output.MirrorOf == ""
}

// overlappedBy returns the first display the selected one covers, or -1.
// Displays without a mode have no area, as in ValidateLayout.
func overlappedBy(outputs []OutputConfig, index int) int {
	selected := outputs[index]
	if selected.Width <= 0 || selected.Height <= 0 {
		return -1
	}
	width, height := selected.LogicalSize()
	for idx, other := range outputs {
		if idx == index || !placeable(other) || other.Width <= 0 || other.Height <= 0 {
			continue
		}
		otherWidth, otherHeight := other.LogicalSize()
		if selected.X < other.X+otherWidth && selected.X+width > other.X &&
			selected.Y < other.Y+otherHeight && selected.Y+height > other.Y {
			return idx
		}
	}
	return -1
}

func displayName(output OutputConfig) string {
	if output.Name != "" {
		return output.Name
	}
	if output.Key != "" {
		return output.Key
	}
	return "display"
}

package profile

import "fmt"

// Canvas placement shared by every layout editor. The Omarchy panel reaches
// these rules through ApplyEditorEdit (a drag release sends x, y and a snap
// distance); the TUI calls them directly. Keeping one implementation is what
// makes a drop, a nudge, or Place beside land in the same spot in both.
//
// Moves and edits retain their requested position when clear. Collisions move
// the edited display to the nearest clear edge, keeping other displays in place.
// Disabled, mirrored, and modeless outputs occupy no independent layout area.

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
// is a pointer drop and ResolveDrop may snap it. Every move resolves collisions
// to a clear edge, including typed coordinates and keyboard moves.
func MoveOutput(outputs []OutputConfig, index, x, y, snapDistance int) (SnapAnalysis, error) {
	if index < 0 || index >= len(outputs) {
		return SnapAnalysis{}, fmt.Errorf("no display at index %d", index)
	}
	if outputs[index].MirrorOf != "" {
		return SnapAnalysis{}, fmt.Errorf("%s mirrors another display and follows it", displayName(outputs[index]))
	}
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
	ResolveLayoutOverlaps(outputs, index)
	return analysis, nil
}

// ResolveLayoutOverlaps clears collisions after an editor operation. Resolve the
// edited display first so enabling it never displaces an otherwise valid layout.
// Then repair any pre-existing collisions. Each placement is clear of every
// other independent display, so later placements cannot introduce new overlaps.
func ResolveLayoutOverlaps(outputs []OutputConfig, edited int) {
	PlaceOutsideOverlaps(outputs, edited)
	for i := range outputs {
		if i != edited {
			PlaceOutsideOverlaps(outputs, i)
		}
	}
}

func placeable(output OutputConfig) bool {
	return output.Enabled && output.MirrorOf == ""
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

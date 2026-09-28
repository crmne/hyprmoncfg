package profile

import (
	"strings"
	"testing"
)

// Vectors ported from omarchy-hyprmoncfg tests/model.test.js ("Alt-arrow
// snapping matches the TUI's nearest-monitor placement"): identical inputs and
// expected outputs.
func TestBesidePositionMatchesPanelAltArrowVectors(t *testing.T) {
	outputs := []OutputConfig{
		{Key: "selected", Enabled: true, Width: 2560, Height: 1440, Scale: 2, Transform: 0, X: 2000, Y: 300},
		{Key: "near", Enabled: true, Width: 1920, Height: 1080, Scale: 1, Transform: 0, X: 0, Y: 0},
		{Key: "far", Enabled: true, Width: 3840, Height: 2160, Scale: 1, Transform: 0, X: 7000, Y: 0},
		{Key: "mirror", Enabled: true, MirrorOf: "near", Width: 1920, Height: 1080, Scale: 1, X: 1900, Y: 0},
	}
	for _, tc := range []struct {
		direction PlaceDirection
		x, y      int
	}{
		{PlaceRight, 1920, 180},
		{PlaceAbove, 320, -720},
	} {
		x, y, anchor, ok := BesidePosition(outputs, 0, tc.direction)
		if !ok || x != tc.x || y != tc.y || outputs[anchor].Key != "near" {
			t.Errorf("direction %d = %d,%d anchor %d ok %v, want %d,%d beside near", tc.direction, x, y, anchor, ok, tc.x, tc.y)
		}
	}
	if _, _, _, ok := BesidePosition(outputs[:1], 0, PlaceLeft); ok {
		t.Error("a lone display has nothing to be placed beside")
	}
}

// Vectors ported from omarchy-hyprmoncfg tests/inspector.test.js ("placement
// uses the same anchor and positions as Alt+arrow snapping").
func TestBesidePositionMatchesPanelPlacementVectors(t *testing.T) {
	outputs := []OutputConfig{
		{Key: "a", Name: "DP-1", Enabled: true, X: 0, Y: 0, Width: 2560, Height: 1440, Scale: 1},
		{Key: "b", Name: "DP-2", Enabled: true, X: 3000, Y: 900, Width: 1920, Height: 1080, Scale: 1},
		{Key: "c", Name: "HDMI-A-1", Enabled: false, X: 0, Y: 0, Width: 1920, Height: 1080, Scale: 1},
	}
	if anchor := NearestAnchor(outputs, 1); anchor < 0 || outputs[anchor].Name != "DP-1" {
		t.Fatalf("anchor of b = %d, want DP-1", anchor)
	}
	if anchor := NearestAnchor(outputs, 2); anchor != -1 {
		t.Fatal("an off display has no placement")
	}
	for _, tc := range []struct {
		direction PlaceDirection
		x, y      int
	}{
		{PlaceRight, 2560, 180},
		{PlaceBelow, 320, 1440},
	} {
		if x, y, _, ok := BesidePosition(outputs, 1, tc.direction); !ok || x != tc.x || y != tc.y {
			t.Errorf("direction %d = %d,%d, want %d,%d", tc.direction, x, y, tc.x, tc.y)
		}
	}
}

// The panel drops through ApplyEditorEdit with a snap distance; the TUI drops
// through MoveOutput. Every case must land in the same place, or be refused
// by both.
func TestDropAndMoveMatchThePanelEditPath(t *testing.T) {
	pair := func() []OutputConfig {
		return []OutputConfig{
			{Key: "left", Name: "DP-1", Enabled: true, Width: 1920, Height: 1080, Scale: 1, X: 0, Y: 0},
			{Key: "right", Name: "DP-2", Enabled: true, Width: 1920, Height: 1080, Scale: 1, X: 1920, Y: 0},
		}
	}
	trio := func() []OutputConfig {
		return []OutputConfig{
			{Key: "anchor", Name: "DP-1", Enabled: true, Width: 1920, Height: 1080, Scale: 1, X: 0, Y: 0},
			{Key: "moving", Name: "DP-2", Enabled: true, Width: 1280, Height: 720, Scale: 1, X: 1920, Y: 0},
			{Key: "below", Name: "HDMI-A-1", Enabled: true, Width: 1920, Height: 1080, Scale: 1, X: 0, Y: 1080},
		}
	}
	for _, tc := range []struct {
		name       string
		outputs    func() []OutputConfig
		index      int
		x, y, snap int
		wantX      int
		wantY      int
		refused    bool
	}{
		// Existing editor vectors: near an edge snaps to it; a drop on top of
		// a neighbour leaves by the nearest clear edge.
		{"snap to neighbour edge", pair, 1, 1912, 9, 24, 1920, 0, false},
		{"overlapping drop leaves by nearest edge", pair, 1, 1810, 40, 24, 1920, 40, false},
		{"far drop outside snap distance stays put", pair, 1, 2600, 500, 24, 2600, 500, false},
		{"snap to origin alignment", pair, 1, 1930, -20, 24, 1920, 0, false},
		{"drop across two displays leaves by the nearest clear edge", trio, 1, 500, 800, 24, 500, 2160, false},
		{"drop bottom alignment with the lower display", trio, 1, 1935, 1450, 24, 1920, 1440, false},
		// Exact moves are taken as given or refused.
		{"exact move into a neighbour is refused", pair, 1, 1000, 0, 0, 1920, 0, true},
		{"exact move to a free spot", pair, 1, 1920, 1080, 0, 1920, 1080, false},
		{"exact move to origin on top of a display is refused", trio, 1, 0, 0, 0, 1920, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			outputs := tc.outputs()
			_, err := MoveOutput(outputs, tc.index, tc.x, tc.y, tc.snap)
			if (err != nil) != tc.refused {
				t.Fatalf("MoveOutput error = %v, refused want %v", err, tc.refused)
			}
			if outputs[tc.index].X != tc.wantX || outputs[tc.index].Y != tc.wantY {
				t.Fatalf("MoveOutput landed at %d,%d, want %d,%d", outputs[tc.index].X, outputs[tc.index].Y, tc.wantX, tc.wantY)
			}
			if ValidateLayout(outputs) != nil {
				t.Fatal("a move left displays overlapping")
			}

			// Same input through the panel's edit path.
			draft := Profile{Outputs: tc.outputs()}
			key := draft.Outputs[tc.index].Key
			x, y := tc.x, tc.y
			got, panelErr := ApplyEditorEdit(draft, EditorEdit{OutputKey: key, X: &x, Y: &y, SnapDistance: tc.snap})
			if (panelErr != nil) != tc.refused {
				t.Fatalf("panel path error = %v, refused want %v", panelErr, tc.refused)
			}
			if panelErr == nil {
				output, _ := got.OutputByKey(key)
				if output.X != outputs[tc.index].X || output.Y != outputs[tc.index].Y {
					t.Fatalf("panel path landed at %d,%d, TUI path at %d,%d", output.X, output.Y, outputs[tc.index].X, outputs[tc.index].Y)
				}
			}
		})
	}
}

func TestMoveOutputNamesTheOverlapAndKeepsThePosition(t *testing.T) {
	outputs := []OutputConfig{
		{Key: "a", Name: "DP-1", Enabled: true, Width: 1920, Height: 1080, Scale: 1},
		{Key: "b", Name: "DP-2", Enabled: true, Width: 1920, Height: 1080, Scale: 1, X: 1920},
	}
	_, err := MoveOutput(outputs, 1, 100, 100, 0)
	if err == nil || !strings.Contains(err.Error(), "DP-2 would overlap DP-1") {
		t.Fatalf("error = %v", err)
	}
	if outputs[1].X != 1920 || outputs[1].Y != 0 {
		t.Fatal("a refused move must not change the position")
	}
	outputs[1].Enabled = false
	if _, err := MoveOutput(outputs, 1, 100, 100, 0); err != nil || outputs[1].X != 100 {
		t.Fatalf("an off display occupies nothing and moves freely, got %v at %d", err, outputs[1].X)
	}
	outputs[1].Enabled, outputs[1].MirrorOf = true, "a"
	if _, err := MoveOutput(outputs, 1, 5000, 0, 0); err == nil {
		t.Fatal("a mirroring display follows its source and cannot be moved")
	}
}

// A display without a mode has no area: it never blocks a move, matching
// ValidateLayout.
func TestMoveOutputIgnoresDisplaysWithoutAMode(t *testing.T) {
	outputs := []OutputConfig{
		{Key: "a", Name: "DP-1", Enabled: true, Width: 1920, Height: 1080, Scale: 1},
		{Key: "b", Name: "HDMI-A-1", Enabled: true, Scale: 1, X: 0, Y: 0},
	}
	if _, err := MoveOutput(outputs, 0, 10, 10, 0); err != nil {
		t.Fatalf("modeless display blocked a move: %v", err)
	}
}

func TestRejectNewOverlapOnlyBlocksNewOverlaps(t *testing.T) {
	clear := []OutputConfig{
		{Key: "a", Name: "DP-1", Enabled: true, Width: 1920, Height: 1080, Scale: 1},
		{Key: "b", Name: "DP-2", Enabled: true, Width: 1920, Height: 1080, Scale: 1, X: 1920},
	}
	overlapping := append([]OutputConfig(nil), clear...)
	overlapping[1].X = 1000
	if RejectNewOverlap(clear, overlapping) == nil {
		t.Fatal("an edit that creates an overlap must be refused")
	}
	if RejectNewOverlap(overlapping, overlapping) != nil {
		t.Fatal("a layout that already overlapped must stay editable so it can be fixed")
	}
}

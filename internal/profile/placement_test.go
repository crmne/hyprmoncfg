package profile

import (
	"fmt"
	"math/rand/v2"
	"reflect"
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
		// Exact moves retain clear coordinates and resolve occupied coordinates.
		{"exact move into a neighbour reflows", pair, 1, 1000, 0, 0, 1920, 0, false},
		{"exact move to a free spot", pair, 1, 1920, 1080, 0, 1920, 1080, false},
		{"exact move to occupied origin reflows", trio, 1, 0, 0, 0, 0, -720, false},
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

func TestMoveOutputResolvesOverlapAndPreservesOtherDisplays(t *testing.T) {
	outputs := []OutputConfig{
		{Key: "a", Name: "DP-1", Enabled: true, Width: 1920, Height: 1080, Scale: 1},
		{Key: "b", Name: "DP-2", Enabled: true, Width: 1920, Height: 1080, Scale: 1, X: 1920},
	}
	_, err := MoveOutput(outputs, 1, 100, 100, 0)
	if err != nil || ValidateLayout(outputs) != nil {
		t.Fatalf("move should resolve the overlap: %v", err)
	}
	if outputs[0].X != 0 || outputs[0].Y != 0 {
		t.Fatal("the other display moved")
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

func TestResolveLayoutOverlapsRepairsExistingCollisions(t *testing.T) {
	outputs := []OutputConfig{
		{Key: "a", Enabled: true, Width: 1920, Height: 1080, Scale: 1},
		{Key: "b", Enabled: true, Width: 1920, Height: 1080, Scale: 1},
		{Key: "c", Enabled: true, Width: 1920, Height: 1080, Scale: 1},
	}
	ResolveLayoutOverlaps(outputs, 1)
	if err := ValidateLayout(outputs); err != nil {
		t.Fatal(err)
	}
	before := append([]OutputConfig(nil), outputs...)
	ResolveLayoutOverlaps(outputs, 1)
	for i := range outputs {
		if outputs[i].X != before[i].X || outputs[i].Y != before[i].Y {
			t.Fatal("a clear layout moved")
		}
	}
}

func TestEnableReflowsOccupiedPosition(t *testing.T) {
	for _, transform := range []int{0, 1, 3} {
		t.Run(fmt.Sprint(transform), func(t *testing.T) {
			draft := Profile{Outputs: []OutputConfig{
				{Key: "desk", Enabled: true, Width: 1920, Height: 1080, Scale: 1},
				{Key: "returning", Enabled: false, Width: 2560, Height: 1440, Scale: 2, Transform: transform, X: 100, Y: 100},
				{Key: "off", Enabled: false, Width: 1920, Height: 1080, Scale: 1},
				{Key: "mirror", Enabled: true, MirrorOf: "desk", Width: 1920, Height: 1080, Scale: 1},
				{Key: "modeless", Enabled: true, X: 100, Y: 100},
			}}
			before := append([]OutputConfig(nil), draft.Outputs...)
			enabled := true
			got, err := ApplyEditorEdit(draft, EditorEdit{OutputKey: "returning", Enabled: &enabled})
			if err != nil {
				t.Fatal(err)
			}
			if err := ValidateLayout(got.Outputs); err != nil {
				t.Fatal(err)
			}
			returning, _ := got.OutputByKey("returning")
			if !returning.Enabled || (returning.X == 100 && returning.Y == 100) {
				t.Fatal("returning display was not enabled and placed")
			}
			for _, key := range []string{"desk", "off", "mirror", "modeless"} {
				old, _ := (Profile{Outputs: before}).OutputByKey(key)
				now, _ := got.OutputByKey(key)
				if old.X != now.X || old.Y != now.Y || old.Enabled != now.Enabled {
					t.Fatalf("%s changed", key)
				}
			}
		})
	}
}

func TestOverlapResolutionEscapesSurroundedDisplay(t *testing.T) {
	outputs := []OutputConfig{
		{Key: "center", Enabled: true, Width: 100, Height: 100, Scale: 1},
		{Key: "left", Enabled: true, Width: 100, Height: 100, Scale: 1, X: -100},
		{Key: "right", Enabled: true, Width: 100, Height: 100, Scale: 1, X: 100},
		{Key: "top", Enabled: true, Width: 100, Height: 100, Scale: 1, Y: -100},
		{Key: "bottom", Enabled: true, Width: 100, Height: 100, Scale: 1, Y: 100},
		{Key: "returning", Enabled: true, Width: 100, Height: 100, Scale: 1},
	}
	before := append([]OutputConfig(nil), outputs...)
	ResolveLayoutOverlaps(outputs, 5)
	if err := ValidateLayout(outputs); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if outputs[i] != before[i] {
			t.Fatalf("unaffected display %s moved", outputs[i].Key)
		}
	}
}

func TestOverlapResolutionIgnoresModelessAndMirroredAreas(t *testing.T) {
	outputs := []OutputConfig{
		{Key: "selected", Enabled: true, Width: 100, Height: 100, Scale: 1},
		{Key: "mirror", Enabled: true, MirrorOf: "selected", Width: 100, Height: 100, Scale: 1},
		{Key: "modeless", Enabled: true, Scale: 1},
		{Key: "off", Width: 100, Height: 100, Scale: 1},
	}
	before := append([]OutputConfig(nil), outputs...)
	ResolveLayoutOverlaps(outputs, 0)
	if !reflect.DeepEqual(outputs, before) {
		t.Fatal("outputs without independent area affected placement")
	}
}

func TestUnmirroringFindsIndependentSpace(t *testing.T) {
	draft := Profile{Outputs: []OutputConfig{
		{Key: "desk", Enabled: true, Width: 1920, Height: 1080, Scale: 1},
		{Key: "mirror", Enabled: true, MirrorOf: "desk", Width: 1920, Height: 1080, Scale: 1},
	}}
	target := ""
	got, err := ApplyEditorEdit(draft, EditorEdit{OutputKey: "mirror", MirrorOf: &target})
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateLayout(got.Outputs); err != nil {
		t.Fatal(err)
	}
	independent, _ := got.OutputByKey("mirror")
	if independent.MirrorOf != "" || (independent.X == 0 && independent.Y == 0) {
		t.Fatal("unmirrored display has no independent space")
	}
}

func TestResolveLayoutOverlapsMixedGeometry(t *testing.T) {
	rng := rand.New(rand.NewPCG(23, 59))
	for iteration := 0; iteration < 500; iteration++ {
		outputs := make([]OutputConfig, 10)
		for i := range outputs {
			outputs[i] = OutputConfig{Key: fmt.Sprint(i), Enabled: true,
				Width: 100 + rng.IntN(4000), Height: 100 + rng.IntN(2000),
				Scale: float64(1+rng.IntN(8)) / 2, Transform: rng.IntN(8),
				X: rng.IntN(4000) - 2000, Y: rng.IntN(4000) - 2000}
		}
		ResolveLayoutOverlaps(outputs, rng.IntN(len(outputs)))
		if err := ValidateLayout(outputs); err != nil {
			t.Fatalf("iteration %d: %v", iteration, err)
		}
	}
}

package profile

import (
	"fmt"
	"math/rand/v2"
	"reflect"
	"slices"
	"testing"

	"github.com/crmne/hyprmoncfg/internal/hypr"
)

func TestApplyEditorEditReflowsNeighborsAfterScaleChange(t *testing.T) {
	draft := Profile{Outputs: []OutputConfig{
		{Key: "left", Name: "DP-1", Enabled: true, Width: 3840, Height: 2160, Scale: 2, X: 0, Y: 0},
		{Key: "right", Name: "DP-2", Enabled: true, Width: 2560, Height: 1440, Scale: 1, X: 1920, Y: 0},
	}}
	scale := 1.5

	got, err := ApplyEditorEdit(draft, EditorEdit{OutputKey: "left", Scale: &scale})
	if err != nil {
		t.Fatal(err)
	}
	if got.Outputs[1].X != 2560 {
		t.Fatalf("right output X = %d, want 2560", got.Outputs[1].X)
	}
}

func TestApplyEditorEditSnapsReleasedPositionToNeighbor(t *testing.T) {
	draft := Profile{Outputs: []OutputConfig{
		{Key: "left", Name: "DP-1", Enabled: true, Width: 1920, Height: 1080, Scale: 1, X: 0, Y: 0},
		{Key: "right", Name: "DP-2", Enabled: true, Width: 1920, Height: 1080, Scale: 1, X: 1920, Y: 0},
	}}
	x, y := 1912, 9

	got, err := ApplyEditorEdit(draft, EditorEdit{OutputKey: "right", X: &x, Y: &y, SnapDistance: 24})
	if err != nil {
		t.Fatal(err)
	}
	if got.Outputs[1].X != 1920 || got.Outputs[1].Y != 0 {
		t.Fatalf("snapped position = %d,%d, want 1920,0", got.Outputs[1].X, got.Outputs[1].Y)
	}
}

func TestApplyEditorEditPlacesAnOverlappingDropOutsideItsNeighbor(t *testing.T) {
	draft := Profile{Outputs: []OutputConfig{
		{Key: "left", Name: "DP-1", Enabled: true, Width: 1920, Height: 1080, Scale: 1, X: 0, Y: 0},
		{Key: "right", Name: "DP-2", Enabled: true, Width: 1920, Height: 1080, Scale: 1, X: 1920, Y: 0},
	}}
	x, y := 1810, 40

	got, err := ApplyEditorEdit(draft, EditorEdit{OutputKey: "right", X: &x, Y: &y, SnapDistance: 24})
	if err != nil {
		t.Fatal(err)
	}
	if got.Outputs[1].X != 1920 || got.Outputs[1].Y != 40 {
		t.Fatalf("resolved position = %d,%d, want 1920,40", got.Outputs[1].X, got.Outputs[1].Y)
	}
}

func TestPlaceOutsideOverlapsChoosesTheNearestClearEdge(t *testing.T) {
	outputs := []OutputConfig{
		{Key: "anchor", Name: "DP-1", Enabled: true, Width: 1920, Height: 1080, Scale: 1, X: 0, Y: 0},
		{Key: "moving", Name: "DP-2", Enabled: true, Width: 1280, Height: 720, Scale: 1, X: 900, Y: 900},
	}

	if !PlaceOutsideOverlaps(outputs, 1) {
		t.Fatal("expected the overlapping output to be repositioned")
	}
	if outputs[1].X != 900 || outputs[1].Y != 1080 {
		t.Fatalf("resolved position = %d,%d, want 900,1080", outputs[1].X, outputs[1].Y)
	}
	if err := ValidateLayout(outputs); err != nil {
		t.Fatalf("resolved layout still overlaps: %v", err)
	}
}

func TestApplyEditorEditRejectsAnAllDisabledDraft(t *testing.T) {
	draft := Profile{Outputs: []OutputConfig{
		{Key: "only", Name: "eDP-1", Enabled: true, Width: 1920, Height: 1080, Scale: 1},
	}}
	enabled := false

	if _, err := ApplyEditorEdit(draft, EditorEdit{OutputKey: "only", Enabled: &enabled}); err == nil {
		t.Fatal("expected the last enabled display to be protected")
	}
}

func TestApplyEditorEditOwnsAdvancedDisplayValidation(t *testing.T) {
	draft := Profile{Outputs: []OutputConfig{{
		Key: "desk", Name: "DP-1", Enabled: true, Width: 3840, Height: 2160, Scale: 1,
	}}}
	vrr, bitdepth := 2, 10
	cm := "hdr"
	sdrBrightness, sdrSaturation := 1.25, 1.1
	sdrMin, sdrMax := 0.02, 300
	sdrEOTF := "gamma22"
	minLum, maxLum, maxAvg := 0.005, 1000, 600
	forceWide, forceHDR := 1, 1
	icc := "/profiles/desk.icc"

	got, err := ApplyEditorEdit(draft, EditorEdit{
		OutputKey: "desk", VRR: &vrr, Bitdepth: &bitdepth, CM: &cm,
		SDRBrightness: &sdrBrightness, SDRSaturation: &sdrSaturation,
		SDRMinLum: &sdrMin, SDRMaxLum: &sdrMax, SDREOTF: &sdrEOTF,
		MinLuminance: &minLum, MaxLuminance: &maxLum, MaxAvgLum: &maxAvg,
		ForceWide: &forceWide, ForceHDR: &forceHDR, ICC: &icc,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := OutputConfig{
		Key: "desk", MatchKey: "desk", Name: "DP-1", Enabled: true, Width: 3840, Height: 2160, Scale: 1,
		VRR: 2, Bitdepth: 10, CM: "hdr", SDRBrightness: 1.25, SDRSaturation: 1.1,
		SDRMinLuminance: 0.02, SDRMaxLuminance: 300, SDREOTF: "gamma22",
		MinLuminance: 0.005, MaxLuminance: 1000, MaxAvgLuminance: 600,
		SupportsWideColor: 1, SupportsHDR: 1, ICC: "/profiles/desk.icc",
	}
	if !reflect.DeepEqual(got.Outputs[0], want) {
		t.Fatalf("advanced output = %+v, want %+v", got.Outputs[0], want)
	}

	relative := "desk.icc"
	if _, err := ApplyEditorEdit(draft, EditorEdit{OutputKey: "desk", ICC: &relative}); err == nil {
		t.Fatal("expected a relative ICC profile path to be rejected")
	}
}

func TestEditorProfileFromStatePreservesProfileOnlySettings(t *testing.T) {
	monitor := hypr.Monitor{
		Name: "eDP-1", Make: "Framework", Model: "Panel", Serial: "A1",
		Width: 3840, Height: 2160, Scale: 1.33,
	}
	saved := FromState("Laptop", []hypr.Monitor{monitor}, nil)
	saved.Outputs[0].Scale = 1.33333
	saved.Outputs[0].ICC = "/profiles/laptop.icc"
	saved.Outputs[0].SupportsHDR = 1

	draft, source, suggested := EditorProfileFromState([]Profile{saved}, []hypr.Monitor{monitor}, nil)
	if source != "Laptop" || suggested != "Laptop" {
		t.Fatalf("source=%q suggested=%q", source, suggested)
	}
	if draft.Outputs[0].Scale != 1.33333 || draft.Outputs[0].ICC != "/profiles/laptop.icc" || draft.Outputs[0].SupportsHDR != 1 {
		t.Fatalf("draft did not preserve saved-only settings: %+v", draft.Outputs[0])
	}
}

func TestValidateLayoutIgnoresAnOutputWithoutAMode(t *testing.T) {
	outputs := []OutputConfig{
		{Name: "DP-1", Enabled: true, Width: 3840, Height: 2160, Scale: 1},
		// Hyprland reports a modeless output at 0,0, on top of DP-1.
		{Name: "HDMI-A-1", Enabled: true, Mode: "preferred", Scale: 1},
	}
	if err := ValidateLayout(outputs); err != nil {
		t.Fatalf("a display without a mode has no area to overlap: %v", err)
	}
	outputs[1].Width, outputs[1].Height = 1920, 1080
	if err := ValidateLayout(outputs); err == nil {
		t.Fatal("once it has a size at 0,0, it does overlap DP-1")
	}
}

func TestReflowAfterResizeLeavesDisplaysInOtherRowsAlone(t *testing.T) {
	// A wide monitor with a laptop to its right and a small display below.
	// Shrinking the small display must not slide the laptop into the wide
	// monitor just because the laptop lies past the small display's right edge.
	outputs := []OutputConfig{
		{Key: "wide", Name: "DP-6", Enabled: true, Width: 3840, Height: 1600, Scale: 1, X: 3009, Y: 1728},
		{Key: "laptop", Name: "eDP-1", Enabled: true, Width: 1920, Height: 1200, Scale: 1, X: 6849, Y: 2128},
		{Key: "small", Name: "DP-4", Enabled: true, Width: 1920, Height: 1080, Scale: 1, X: 4002, Y: 3328},
	}
	oldWidth, oldHeight := outputs[2].LogicalSize()
	before := append([]OutputConfig(nil), outputs...)
	outputs[2].Scale = 1.25

	ReflowAfterResize(outputs, 2, oldWidth, oldHeight)

	for idx := range 2 {
		if outputs[idx].X != before[idx].X || outputs[idx].Y != before[idx].Y {
			t.Fatalf("%s moved to %d,%d", outputs[idx].Name, outputs[idx].X, outputs[idx].Y)
		}
	}
	if err := RejectNewOverlap(before, outputs); err != nil {
		t.Fatalf("scale change introduced an overlap: %v", err)
	}
}

func TestReflowAfterResizeShiftsAChainedRow(t *testing.T) {
	// The third display only touches the second one's span, so it moves
	// because the second one moves with the resized display.
	outputs := []OutputConfig{
		{Key: "a", Name: "DP-1", Enabled: true, Width: 3840, Height: 2160, Scale: 2, X: 0, Y: 0},
		{Key: "b", Name: "DP-2", Enabled: true, Width: 1920, Height: 2160, Scale: 1, X: 1920, Y: 0},
		{Key: "c", Name: "DP-3", Enabled: true, Width: 1920, Height: 1080, Scale: 1, X: 3840, Y: 1500},
		{Key: "elsewhere", Name: "DP-4", Enabled: true, Width: 1920, Height: 1080, Scale: 1, X: 3840, Y: -3000},
	}
	oldWidth, oldHeight := outputs[0].LogicalSize()
	outputs[0].Scale = 1

	ReflowAfterResize(outputs, 0, oldWidth, oldHeight)

	got := []int{outputs[1].X, outputs[2].X, outputs[3].X}
	if want := []int{3840, 5760, 3840}; !reflect.DeepEqual(got, want) {
		t.Fatalf("X positions = %v, want %v", got, want)
	}
}

func TestReflowAfterResizeKeepsADiagonalNeighborClearOfAGrowingOutput(t *testing.T) {
	outputs := []OutputConfig{
		{Key: "grow", Name: "DP-1", Enabled: true, Width: 1920, Height: 1080, Scale: 2, X: 0, Y: 0},
		{Key: "diagonal", Name: "DP-2", Enabled: true, Width: 1920, Height: 1080, Scale: 1, X: 960, Y: 540},
	}
	oldWidth, oldHeight := outputs[0].LogicalSize()
	before := append([]OutputConfig(nil), outputs...)
	outputs[0].Scale = 1

	ReflowAfterResize(outputs, 0, oldWidth, oldHeight)

	if outputs[1].X != 1920 || outputs[1].Y != 1080 {
		t.Fatalf("diagonal output at %d,%d, want 1920,1080", outputs[1].X, outputs[1].Y)
	}
	if err := RejectNewOverlap(before, outputs); err != nil {
		t.Fatalf("growing introduced an overlap: %v", err)
	}
}

func TestApplyEditorEditScaleMovesACornerNeighborWithTheDiagonal(t *testing.T) {
	// B shifts right and down; C only touches B's corner, so it must move
	// on both axes too or B lands on it.
	draft := Profile{Outputs: []OutputConfig{
		{Key: "a", Name: "DP-1", Enabled: true, Width: 1920, Height: 1080, Scale: 2, X: 0, Y: 0},
		{Key: "b", Name: "DP-2", Enabled: true, Width: 1280, Height: 720, Scale: 2, X: 1100, Y: 650},
		{Key: "c", Name: "DP-3", Enabled: true, Width: 1920, Height: 1080, Scale: 1, X: 1740, Y: 1010},
	}}
	scale := 1.33333

	got, err := ApplyEditorEdit(draft, EditorEdit{OutputKey: "a", Scale: &scale})
	if err != nil {
		t.Fatal(err)
	}
	if got.Outputs[1].X != 1580 || got.Outputs[1].Y != 920 {
		t.Fatalf("b at %d,%d, want 1580,920", got.Outputs[1].X, got.Outputs[1].Y)
	}
	if got.Outputs[2].X != 2220 || got.Outputs[2].Y != 1280 {
		t.Fatalf("c at %d,%d, want 2220,1280", got.Outputs[2].X, got.Outputs[2].Y)
	}
}

// blanketReflow is the previous reflow: every enabled output past the old
// right or bottom edge moved with it.
func blanketReflow(outputs []OutputConfig, index, oldWidth, oldHeight int) {
	newWidth, newHeight := outputs[index].LogicalSize()
	oldRight, oldBottom := outputs[index].X+oldWidth, outputs[index].Y+oldHeight
	for idx := range outputs {
		if idx == index || !outputs[idx].Enabled {
			continue
		}
		if outputs[idx].X >= oldRight {
			outputs[idx].X += newWidth - oldWidth
		}
		if outputs[idx].Y >= oldBottom {
			outputs[idx].Y += newHeight - oldHeight
		}
	}
}

func positionsByName(outputs []OutputConfig) map[string][2]int {
	positions := make(map[string][2]int, len(outputs))
	for _, output := range outputs {
		positions[output.Name] = [2]int{output.X, output.Y}
	}
	return positions
}

func TestReflowAfterResizeKeepsClearWhatTheBlanketShiftKeptClear(t *testing.T) {
	modes := [][2]int{{1920, 1080}, {2560, 1440}, {1280, 720}, {3840, 1600}, {1920, 1200}}
	scales := []float64{1, 1.25, 1.5, 2}
	random := rand.New(rand.NewPCG(1, 2))
	checked := 0
	for range 20000 {
		outputs := make([]OutputConfig, 2+random.IntN(4))
		for idx := range outputs {
			mode := modes[random.IntN(len(modes))]
			outputs[idx] = OutputConfig{
				Name: fmt.Sprintf("DP-%d", idx), Enabled: true, Width: mode[0], Height: mode[1],
				Scale: scales[random.IntN(len(scales))], Transform: random.IntN(2),
				X: random.IntN(40) * 64, Y: random.IntN(40) * 36,
			}
		}
		if ValidateLayout(outputs) != nil {
			continue
		}
		index := random.IntN(len(outputs))
		oldWidth, oldHeight := outputs[index].LogicalSize()
		outputs[index].Scale = scales[random.IntN(len(scales))]
		outputs[index].Transform = random.IntN(2)
		if width, height := outputs[index].LogicalSize(); width == oldWidth && height == oldHeight {
			continue
		}

		current := append([]OutputConfig(nil), outputs...)
		ReflowAfterResize(current, index, oldWidth, oldHeight)

		// Reversing the order must not change where anything lands.
		reversed := append([]OutputConfig(nil), outputs...)
		slices.Reverse(reversed)
		ReflowAfterResize(reversed, len(outputs)-1-index, oldWidth, oldHeight)
		if !reflect.DeepEqual(positionsByName(current), positionsByName(reversed)) {
			t.Fatalf("order changed the result for %+v: %v vs %v", outputs, positionsByName(current), positionsByName(reversed))
		}

		previous := append([]OutputConfig(nil), outputs...)
		blanketReflow(previous, index, oldWidth, oldHeight)
		if ValidateLayout(previous) != nil {
			continue
		}
		if err := ValidateLayout(current); err != nil {
			t.Fatalf("resizing %s in %+v: %v", outputs[index].Name, outputs, err)
		}
		checked++
	}
	if checked < 1000 {
		t.Fatalf("only %d layouts exercised", checked)
	}
}

func TestReflowAfterResizeDoesNotDependOnOutputOrder(t *testing.T) {
	base := []OutputConfig{
		{Name: "B", Enabled: true, Width: 400, Height: 100, Scale: 1, X: 600, Y: 400},
		{Name: "C", Enabled: true, Width: 200, Height: 400, Scale: 1, X: 300, Y: 200},
		{Name: "D", Enabled: true, Width: 500, Height: 200, Scale: 1, X: 400, Y: 600},
		{Name: "E", Enabled: true, Width: 500, Height: 400, Scale: 1, X: 1000, Y: 300},
	}
	var want map[string][2]int
	var permute func(prefix, rest []OutputConfig)
	permute = func(prefix, rest []OutputConfig) {
		if len(rest) == 0 {
			outputs := append([]OutputConfig{{Name: "A", Enabled: true, Width: 200, Height: 200, Scale: 1}}, prefix...)
			ReflowAfterResize(outputs, 0, 300, 300)
			got := positionsByName(outputs)
			if want == nil {
				want = got
			} else if !reflect.DeepEqual(got, want) {
				t.Fatalf("order %v moved displays to %v, want %v", positionsByName(prefix), got, want)
			}
			return
		}
		for idx := range rest {
			next := append(append([]OutputConfig(nil), rest[:idx]...), rest[idx+1:]...)
			permute(append(append([]OutputConfig(nil), prefix...), rest[idx]), next)
		}
	}
	permute(nil, base)
}

func TestReflowAfterResizeKeepsACornerNeighborOnTheCorner(t *testing.T) {
	outputs := []OutputConfig{
		{Name: "A", Enabled: true, Width: 1920, Height: 1080, Scale: 1, X: 0, Y: 0},
		{Name: "B", Enabled: true, Width: 1920, Height: 1080, Scale: 1, X: 1920, Y: 1080},
	}
	oldWidth, oldHeight := outputs[0].LogicalSize()
	outputs[0].Scale = 2

	ReflowAfterResize(outputs, 0, oldWidth, oldHeight)

	if outputs[1].X != 960 || outputs[1].Y != 540 {
		t.Fatalf("corner neighbor at %d,%d, want 960,540", outputs[1].X, outputs[1].Y)
	}
}

func TestReflowAfterResizeLeavesMirrorsAndModelessOutputsAlone(t *testing.T) {
	outputs := []OutputConfig{
		{Name: "source", Enabled: true, Width: 1920, Height: 1080, Scale: 1, X: 0, Y: 0},
		{Name: "mirror", Enabled: true, MirrorOf: "source", Width: 1920, Height: 1080, Scale: 1, X: 1920, Y: 0},
		{Name: "modeless", Enabled: true, X: 1920, Y: 0},
		{Name: "right", Enabled: true, Width: 1920, Height: 1080, Scale: 1, X: 1920, Y: 0},
	}
	oldWidth, oldHeight := outputs[0].LogicalSize()
	outputs[0].Scale = 2

	ReflowAfterResize(outputs, 0, oldWidth, oldHeight)

	got := positionsByName(outputs)
	want := map[string][2]int{"source": {0, 0}, "mirror": {1920, 0}, "modeless": {1920, 0}, "right": {960, 0}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("positions = %v, want %v", got, want)
	}
}

func TestReflowAfterResizeIgnoresMirrors(t *testing.T) {
	outputs := []OutputConfig{
		{Key: "source", Name: "DP-1", Enabled: true, Width: 1920, Height: 1080, Scale: 1, X: 0, Y: 0},
		{Key: "mirror", Name: "DP-2", Enabled: true, MirrorOf: "DP-1", Width: 1920, Height: 1080, Scale: 1, X: 0, Y: 0},
		{Key: "right", Name: "DP-3", Enabled: true, Width: 1920, Height: 1080, Scale: 1, X: 1920, Y: 0},
	}
	oldWidth, oldHeight := outputs[1].LogicalSize()
	outputs[1].Width = 2560

	ReflowAfterResize(outputs, 1, oldWidth, oldHeight)

	if outputs[2].X != 1920 {
		t.Fatalf("resizing a mirror moved DP-3 to X = %d", outputs[2].X)
	}
}

func TestReflowAfterResizeKeepsAGapInsideTheMovingGroup(t *testing.T) {
	// B sits on A's corner and C keeps a deliberate 100px gap to B's right.
	outputs := []OutputConfig{
		{Name: "A", Enabled: true, Width: 100, Height: 100, Scale: 1, X: 0, Y: 0},
		{Name: "B", Enabled: true, Width: 50, Height: 50, Scale: 1, X: 200, Y: 200},
		{Name: "C", Enabled: true, Width: 50, Height: 50, Scale: 1, X: 350, Y: 200},
	}

	ReflowAfterResize(outputs, 0, 200, 200)

	got := positionsByName(outputs)
	want := map[string][2]int{"A": {0, 0}, "B": {100, 100}, "C": {250, 100}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("positions = %v, want %v", got, want)
	}
}

func TestReflowAfterResizeLeavesADisplayOnTheTopEdgeAlone(t *testing.T) {
	// B rests on A's top edge, clear of its corner and of the area A grows
	// into, so it belongs to another row.
	outputs := []OutputConfig{
		{Name: "A", Enabled: true, Width: 200, Height: 100, Scale: 1, X: 0, Y: 100},
		{Name: "B", Enabled: true, Width: 25, Height: 50, Scale: 1, X: 150, Y: 50},
	}

	ReflowAfterResize(outputs, 0, 100, 100)

	if outputs[1].X != 150 || outputs[1].Y != 50 {
		t.Fatalf("B moved to %d,%d", outputs[1].X, outputs[1].Y)
	}
}

func TestReflowAfterResizeLeavesADisplayStackedOnTheRowAlone(t *testing.T) {
	// C sits on top of B, which is in A's row; D is beside C. Only B moves.
	outputs := []OutputConfig{
		{Name: "A", Enabled: true, Width: 100, Height: 100, Scale: 1, X: 0, Y: 0},
		{Name: "B", Enabled: true, Width: 100, Height: 100, Scale: 1, X: 200, Y: 0},
		{Name: "C", Enabled: true, Width: 100, Height: 100, Scale: 1, X: 250, Y: -100},
		{Name: "D", Enabled: true, Width: 100, Height: 100, Scale: 1, X: 150, Y: -100},
	}

	ReflowAfterResize(outputs, 0, 200, 100)

	got := positionsByName(outputs)
	want := map[string][2]int{"A": {0, 0}, "B": {100, 0}, "C": {250, -100}, "D": {150, -100}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("positions = %v, want %v", got, want)
	}
}

func TestApplyEditorEditScaleLeavesTheLaptopBesideTheWideMonitor(t *testing.T) {
	// The reported layout: scaling the small display below the wide monitor
	// used to slide the laptop into the wide monitor and refuse the edit.
	draft := Profile{Outputs: []OutputConfig{
		{Key: "wide", Name: "DP-6", Enabled: true, Width: 3840, Height: 1600, Scale: 1, X: 3009, Y: 1728},
		{Key: "laptop", Name: "eDP-1", Enabled: true, Width: 1920, Height: 1200, Scale: 1, X: 6849, Y: 2128},
		{Key: "small", Name: "DP-4", Enabled: true, Width: 1920, Height: 1080, Scale: 1, X: 4002, Y: 3328},
	}}
	scale := 1.25

	got, err := ApplyEditorEdit(draft, EditorEdit{OutputKey: "small", Scale: &scale})
	if err != nil {
		t.Fatal(err)
	}
	if position := positionsByName(got.Outputs)["eDP-1"]; position != [2]int{6849, 2128} {
		t.Fatalf("laptop moved to %v", position)
	}
}

func TestReflowAfterResizeLeavesADisplayStackedUnderTheRowAlone(t *testing.T) {
	outputs := []OutputConfig{
		{Name: "A", Enabled: true, Width: 100, Height: 100, Scale: 1, X: 0, Y: 0},
		{Name: "B", Enabled: true, Width: 100, Height: 100, Scale: 1, X: 300, Y: 0},
		{Name: "C", Enabled: true, Width: 100, Height: 100, Scale: 1, X: 300, Y: 100},
	}

	ReflowAfterResize(outputs, 0, 200, 100)

	got := positionsByName(outputs)
	want := map[string][2]int{"A": {0, 0}, "B": {200, 0}, "C": {300, 100}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("positions = %v, want %v", got, want)
	}
}

func TestReflowAfterResizeMovesADisplayAMoverWouldLandOn(t *testing.T) {
	// B sits on A's corner; C is clear of A but where B lands once A grows.
	outputs := []OutputConfig{
		{Name: "A", Enabled: true, Width: 200, Height: 200, Scale: 1, X: 0, Y: 0},
		{Name: "B", Enabled: true, Width: 50, Height: 50, Scale: 1, X: 100, Y: 100},
		{Name: "C", Enabled: true, Width: 50, Height: 50, Scale: 1, X: 200, Y: 200},
	}

	ReflowAfterResize(outputs, 0, 100, 100)

	got := positionsByName(outputs)
	want := map[string][2]int{"A": {0, 0}, "B": {200, 200}, "C": {300, 300}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("positions = %v, want %v", got, want)
	}
}

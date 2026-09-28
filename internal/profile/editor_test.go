package profile

import (
	"reflect"
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

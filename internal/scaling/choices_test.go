package scaling

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

// Vectors written by the Omarchy panel's designer from the list the panel
// shows as scale pills. SharpChoices is the one source both editors use, so it
// must reproduce every list exactly, in order.
func TestSharpChoicesMatchThePanelScalePills(t *testing.T) {
	data, err := os.ReadFile("testdata/panel-scale-vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Vectors []struct {
			Width, Height int
			Scales        []float64
		}
	}
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatal(err)
	}
	if len(file.Vectors) == 0 {
		t.Fatal("no vectors")
	}
	for _, v := range file.Vectors {
		// Scale 1 is sharp for every mode, so nothing is appended.
		if got := SharpChoices(v.Width, v.Height, 1); !reflect.DeepEqual(got, v.Scales) {
			t.Errorf("%dx%d: Go %v, panel %v", v.Width, v.Height, got, v.Scales)
		}
		if got := Choices(v.Width, v.Height, 1); !reflect.DeepEqual(got, v.Scales) {
			t.Errorf("%dx%d: Choices %v, panel %v", v.Width, v.Height, got, v.Scales)
		}
	}
}

func TestChoicesKeepAnUnsharpCurrentScaleAsItself(t *testing.T) {
	got := Choices(3840, 2160, 1.33)
	found133, found4over3 := false, false
	for _, value := range got {
		found133 = found133 || value == 1.33
		found4over3 = found4over3 || value == 1.33333
	}
	if !found133 || !found4over3 {
		t.Fatalf("an unsharp 1.33 must stay as itself beside the sharp 1.33333, got %v", got)
	}
	if sharp := SharpChoices(3840, 2160, 1.33); len(sharp) != len(got)-1 {
		t.Fatalf("only the exact current value is added, got %v vs %v", sharp, got)
	}
	// A sub-1 current value brings its closest sharp scale, as the editor did.
	if got := SharpChoices(3840, 2160, 0.8); got[0] != 0.8 {
		t.Fatalf("sub-1 current scale missing, got %v", got)
	}
}

// The preset pills and their labels for each mode, from the same panel
// vectors. Labels are the TUI's; they follow the vectors' label rule.
func TestPresetChoicesMatchThePanelPills(t *testing.T) {
	data, err := os.ReadFile("testdata/panel-scale-vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Presets []struct {
			Width, Height int
			Pills         []float64
			Labels        []string
		}
	}
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatal(err)
	}
	if len(file.Presets) == 0 {
		t.Fatal("no preset vectors")
	}
	for _, v := range file.Presets {
		if got := PresetChoices(v.Width, v.Height); !reflect.DeepEqual(got, v.Pills) {
			t.Errorf("%dx%d: Go %v, panel %v", v.Width, v.Height, got, v.Pills)
		}
	}
}

// CleanScale is Omarchy's cleanScale: round up to the next sharp scale.
func TestCleanScaleRoundsUpToTheNextSharpScale(t *testing.T) {
	for _, tc := range []struct {
		w, h      int
		requested float64
		want      float64
	}{
		{2560, 1440, 1.5, 1.6},
		{2560, 1440, 3, 3.2},
		{1366, 768, 1.25, 2},
		{1366, 768, 3, 2},
		{3024, 1964, 3, 4},
		{3840, 2160, 1.25, 1.25},
	} {
		if got, _ := CleanScale(tc.w, tc.h, tc.requested); got != tc.want {
			t.Errorf("%dx%d %v: got %v want %v", tc.w, tc.h, tc.requested, got, tc.want)
		}
	}
}

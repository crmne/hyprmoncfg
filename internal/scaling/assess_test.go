package scaling

import (
	"errors"
	"reflect"
	"testing"
)

func TestAssessSharpBlurryAndRange(t *testing.T) {
	sharp, err := Assess(3840, 2160, 1.5)
	if err != nil || !sharp.Sharp || sharp.Logical != [2]float64{2560, 1440} || sharp.Apply != 1.5 || sharp.ClosestLogical != nil {
		t.Fatalf("1.5 on 4K = %+v, %v", sharp, err)
	}

	blurry, err := Assess(3840, 2160, 1.33)
	if err != nil || blurry.Sharp || blurry.Closest != 1.33333 || blurry.Apply != 1.33333 {
		t.Fatalf("1.33 on 4K = %+v, %v", blurry, err)
	}
	if *blurry.ClosestLogical != [2]float64{2880, 1620} {
		t.Fatalf("closest logical %v", *blurry.ClosestLogical)
	}

	if got, _ := Assess(3840, 2160, 1.3333333); got.Scale != 1.33333 {
		t.Fatalf("scales are rounded as stored, got %v", got.Scale)
	}
	for _, value := range []float64{0.2, 4.01} {
		if _, err := Assess(3840, 2160, value); !errors.Is(err, ErrScaleRange) {
			t.Fatalf("%v: %v", value, err)
		}
	}
}

func TestDescribeIsTheScaleRow(t *testing.T) {
	info, err := Describe(3840, 2160, 1.33)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(info.Presets, PresetChoices(3840, 2160)) {
		t.Fatalf("presets %v", info.Presets)
	}
	if !reflect.DeepEqual(info.Options, SharpChoices(3840, 2160, 1.33)) {
		t.Fatalf("options %v", info.Options)
	}
	if info.Sharp {
		t.Fatal("1.33 is blurry on 4K")
	}
}

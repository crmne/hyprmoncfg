package tui

import (
	"testing"

	"github.com/crmne/hyprmoncfg/internal/appstatus"
)

func TestRotationLabelShowsAutoTheUprightTurnAndTheLock(t *testing.T) {
	for _, tc := range []struct {
		output   editableOutput
		rotation *appstatus.Rotation
		want     string
	}{
		{editableOutput{Transform: 1}, nil, "90"},
		{editableOutput{RotationAuto: true}, &appstatus.Rotation{Available: true}, "Auto"},
		{editableOutput{RotationAuto: true, Transform: 3}, &appstatus.Rotation{Available: true}, "Auto, upright 270°"},
		{editableOutput{RotationAuto: true}, &appstatus.Rotation{Available: true, Locked: true}, "Auto, locked"},
	} {
		if got := rotationLabel(tc.output, tc.rotation); got != tc.want {
			t.Errorf("rotationLabel(%+v) = %q, want %q", tc.output, got, tc.want)
		}
	}
}

func TestOnlyABuiltInPanelWithASensorOffersAutoRotation(t *testing.T) {
	m := Model{rotation: &appstatus.Rotation{Available: true}}
	if !m.rotationAutoSupported(editableOutput{IsInternal: true}) {
		t.Fatal("a built-in panel with a sensor should offer Auto")
	}
	if m.rotationAutoSupported(editableOutput{}) {
		t.Fatal("an external display offered Auto")
	}
	m.rotation.Available = false
	if m.rotationAutoSupported(editableOutput{IsInternal: true}) {
		t.Fatal("Auto offered without a sensor")
	}
}

func TestChoosingARotationTurnsAutoOff(t *testing.T) {
	m := Model{}
	output := editableOutput{IsInternal: true, Transform: 3}
	m.applyFieldPickerValue(&output, 6, rotationAutoValue)
	if !output.RotationAuto || output.Transform != 3 {
		t.Fatalf("Auto = %+v, want the rotation kept as upright", output)
	}
	m.applyFieldPickerValue(&output, 6, "90")
	if output.RotationAuto || output.Transform != 1 {
		t.Fatalf("90 = %+v, want a fixed rotation", output)
	}
}

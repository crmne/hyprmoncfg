package profile

import (
	"errors"
	"testing"

	"github.com/crmne/hyprmoncfg/internal/hypr"
)

func TestRotateTransformTurnsAndKeepsTheFlip(t *testing.T) {
	for _, tc := range []struct{ transform, quarters, want int }{
		{0, 1, 1}, {1, 1, 2}, {3, 1, 0}, {0, -1, 3}, {2, 2, 0},
		{4, 1, 5}, {7, 1, 4}, {5, -1, 4}, {0, 5, 1}, {9, 1, 1},
	} {
		if got := RotateTransform(tc.transform, tc.quarters); got != tc.want {
			t.Errorf("RotateTransform(%d, %d) = %d, want %d", tc.transform, tc.quarters, got, tc.want)
		}
	}
}

// A 2-in-1 with a 1920x1200 panel and a monitor to its right. Turning the
// device sideways makes the panel 1200 wide, and the monitor stays touching.
func rotatingDesk() (Profile, []hypr.Monitor) {
	panel := hypr.Monitor{Name: "eDP-1", Description: "BOE 0x0BCA", Make: "BOE", Model: "0x0BCA",
		Width: 1920, Height: 1200, RefreshRate: 60, Scale: 1, DPMSStatus: true, AvailableModes: []string{"1920x1200@60.00Hz"}}
	external := hypr.Monitor{Name: "DP-1", Description: "Dell U2720Q ABC", Make: "Dell", Model: "U2720Q", Serial: "ABC",
		Width: 2560, Height: 1440, RefreshRate: 60, X: 1920, Scale: 1, DPMSStatus: true, AvailableModes: []string{"2560x1440@60.00Hz"}}
	p := FromMonitors("Desk", []hypr.Monitor{panel, external})
	p.Outputs[0].RotationAuto = true
	return p, []hypr.Monitor{panel, external}
}

func TestOrientedTurnsTheAutoPanelAndMovesItsNeighbour(t *testing.T) {
	p, _ := rotatingDesk()
	turned := Oriented(p, 1)
	panel, _ := turned.OutputByKey(p.Outputs[0].Key)
	external, _ := turned.OutputByKey(p.Outputs[1].Key)
	if panel.Transform != 1 || external.Transform != 0 {
		t.Fatalf("transforms = %d, %d; want 1, 0", panel.Transform, external.Transform)
	}
	if external.X != 1200 || external.Y != 0 {
		t.Fatalf("external at %d,%d; want 1200,0 touching the turned panel", external.X, external.Y)
	}
	if saved, _ := p.OutputByKey(p.Outputs[0].Key); saved.Transform != 0 {
		t.Fatal("Oriented changed the saved profile")
	}
	if back := Oriented(p, 4); back.Outputs[1].X != 1920 {
		t.Fatalf("a full turn moved the external to %d", back.Outputs[1].X)
	}
}

func TestOrientedAddsToAPortraitNativePanel(t *testing.T) {
	p, _ := rotatingDesk()
	p.Outputs[0].Transform = 3
	if got := Oriented(p, 1).Outputs[0].Transform; got != 0 {
		t.Fatalf("an upright offset of 3 turned once = %d, want 0", got)
	}
}

func TestOrientedLeavesProfilesWithoutAutoRotationAlone(t *testing.T) {
	p, _ := rotatingDesk()
	p.Outputs[0].RotationAuto = false
	if got := Oriented(p, 1); got.Outputs[0].Transform != 0 || got.Outputs[1].X != 1920 {
		t.Fatalf("rotated a profile without auto rotation: %+v", got.Outputs)
	}
}

func TestOnlyBuiltInPanelsRotateAutomatically(t *testing.T) {
	p, _ := rotatingDesk()
	yes := true
	if _, err := ApplyEditorEdit(p, EditorEdit{OutputKey: p.Outputs[1].Key, RotationAuto: &yes}); !errors.Is(err, errRotationAutoExternal) {
		t.Fatalf("edit on an external = %v, want a refusal", err)
	}
	edited, err := ApplyEditorEdit(p, EditorEdit{OutputKey: p.Outputs[0].Key, RotationAuto: &yes})
	if err != nil || !edited.Outputs[0].RotationAuto {
		t.Fatalf("edit on the panel = %v, %+v", err, edited.Outputs[0])
	}
	p.Outputs[1].RotationAuto = true
	if err := p.Validate(); !errors.Is(err, errRotationAutoExternal) {
		t.Fatalf("Validate = %v, want a refusal for the external", err)
	}
}

// Live state read while the device is turned still drafts the upright
// saved layout, and names its profile.
func TestEditorDraftOfATurnedDeviceIsUpright(t *testing.T) {
	p, monitors := rotatingDesk()
	turned := []hypr.Monitor{monitors[0], monitors[1]}
	turned[0].Transform = 1
	turned[1].X = 1200

	draft, source, _ := EditorProfileFromOrientedState([]Profile{p}, turned, nil, 1)
	if source != "Desk" {
		t.Fatalf("source = %q, want Desk", source)
	}
	panel, _ := draft.OutputByKey(p.Outputs[0].Key)
	external, _ := draft.OutputByKey(p.Outputs[1].Key)
	if panel.Transform != 0 || !panel.RotationAuto || external.X != 1920 {
		t.Fatalf("draft = %+v / %+v, want the upright saved layout", panel, external)
	}
	if active, ok := OrientedExactStateMatch([]Profile{p}, turned, nil, 1); !ok || active.Outputs[0].Transform != 0 {
		t.Fatalf("OrientedExactStateMatch = %+v, %v", active, ok)
	}
	if _, ok := ExactStateMatch([]Profile{p}, turned, nil); ok {
		t.Fatal("the turned state matched without the orientation")
	}

	// A custom layout keeps its positions, and the panel loses only the
	// sensor's turn.
	custom := []hypr.Monitor{turned[0], turned[1]}
	custom[1].Y = 300
	draft, source, _ = EditorProfileFromOrientedState([]Profile{p}, custom, nil, 1)
	panel, _ = draft.OutputByKey(p.Outputs[0].Key)
	external, _ = draft.OutputByKey(p.Outputs[1].Key)
	if source != "" || panel.Transform != 0 || !panel.RotationAuto || external.X != 1200 || external.Y != 300 {
		t.Fatalf("custom draft = %q %+v / %+v", source, panel, external)
	}
}

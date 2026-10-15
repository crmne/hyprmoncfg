package profile

import (
	"testing"

	"github.com/crmne/hyprmoncfg/internal/hypr"
)

func TestRotationsAgreeWithResolvedState(t *testing.T) {
	for _, tc := range []struct {
		name            string
		auto            bool
		requested, live int
		want            bool
	}{
		{"auto accepts normal", true, 0, 0, true},
		{"auto accepts a rotated live value", true, 0, 3, true},
		{"auto ignores a stale captured value", true, 1, 2, true},
		{"auto accepts a flipped live value", true, 2, 5, true},
		{"explicit matches itself", false, 3, 3, true},
		{"explicit rejects a different live rotation", false, 3, 0, false},
		{"explicit rejects a flipped variant", false, 1, 5, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requested := OutputConfig{Transform: tc.requested, RotationAuto: tc.auto}
			live := OutputConfig{Transform: tc.live}
			if got := rotationsAgree(requested, live); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestExactStateMatchRecognizesAutoRotation(t *testing.T) {
	monitors := []hypr.Monitor{{Name: "DSI-1", Make: "NYM", Model: "Panel", Width: 1200, Height: 1920,
		RefreshRate: 50, Scale: 1, Transform: 3}}
	saved := FromMonitors("Tablet", monitors)
	saved.Outputs[0].RotationAuto = true

	if _, ok := ExactStateMatch([]Profile{saved}, monitors, nil); !ok {
		t.Fatal("auto rotation did not recognize its resolved state")
	}

	// An orientation sensor rotating the panel must not stop the profile from
	// matching: that is the whole point of auto.
	rotated := []hypr.Monitor{{Name: "DSI-1", Make: "NYM", Model: "Panel", Width: 1200, Height: 1920,
		RefreshRate: 50, Scale: 1, Transform: 1}}
	if _, ok := ExactStateMatch([]Profile{saved}, rotated, nil); !ok {
		t.Fatal("auto rotation did not match a rotated live state")
	}
}

func TestExplicitRotationStillRejectsADifferentLiveRotation(t *testing.T) {
	monitors := []hypr.Monitor{{Name: "DSI-1", Make: "NYM", Model: "Panel", Width: 1200, Height: 1920,
		RefreshRate: 50, Scale: 1, Transform: 3}}
	saved := FromMonitors("Tablet", monitors)

	rotated := []hypr.Monitor{{Name: "DSI-1", Make: "NYM", Model: "Panel", Width: 1200, Height: 1920,
		RefreshRate: 50, Scale: 1, Transform: 1}}
	if _, ok := ExactStateMatch([]Profile{saved}, rotated, nil); ok {
		t.Fatal("a saved explicit rotation must not match a live rotation it did not ask for")
	}
}

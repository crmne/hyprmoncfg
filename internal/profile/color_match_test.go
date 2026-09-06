package profile

import (
	"testing"

	"github.com/crmne/hyprmoncfg/internal/hypr"
)

func TestColorPresetsAgreeWithResolvedState(t *testing.T) {
	for _, tc := range []struct {
		requested, live string
		want            bool
	}{
		{"auto", "srgb", true}, {" AUTO ", "wide", true}, {"auto", "auto", true},
		{"auto", "hdr", false}, {"auto", "hdredid", false}, {"auto", "unknown", false},
		{"hdr", "auto", false}, {"srgb", "auto", false}, {"hdr", "srgb", false},
		{"", "srgb", true}, {"srgb", "", true}, {"wide", "wide", true},
		{"edid", "edid", true}, {"hdredid", "hdr", false},
	} {
		t.Run(tc.requested+"/"+tc.live, func(t *testing.T) {
			if got := colorPresetsAgree(tc.requested, tc.live); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestExactStateMatchRecognizesAutoColor(t *testing.T) {
	monitors := []hypr.Monitor{{Name: "DP-1", Make: "Dell", Model: "Desk", Width: 1920, Height: 1080,
		RefreshRate: 60, Scale: 1, ColorManagementPreset: "srgb"}}
	saved := FromMonitors("Desk", monitors)
	saved.Outputs[0].CM = "auto"
	if _, ok := ExactStateMatch([]Profile{saved}, monitors, nil); !ok {
		t.Fatal("auto profile did not recognize its resolved state")
	}
	monitors[0].ColorManagementPreset = "hdr"
	if _, ok := ExactStateMatch([]Profile{saved}, monitors, nil); ok {
		t.Fatal("auto profile incorrectly claimed an HDR state")
	}
}

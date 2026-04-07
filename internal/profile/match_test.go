package profile

import (
	"testing"

	"github.com/crmne/hyprmoncfg/internal/hypr"
)

func TestBestMatchPrefersExactSet(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "DP-1", Make: "Dell", Model: "U2720Q", Serial: "A1"},
		{Name: "HDMI-A-1", Make: "LG", Model: "27GP850", Serial: "B2"},
	}

	exact := New("desk", []OutputConfig{
		{Key: monitors[0].HardwareKey(), Enabled: true, Scale: 1, Width: 2560, Height: 1440},
		{Key: monitors[1].HardwareKey(), Enabled: true, Scale: 1, Width: 2560, Height: 1440},
	})
	partial := New("single", []OutputConfig{
		{Key: monitors[0].HardwareKey(), Enabled: true, Scale: 1, Width: 2560, Height: 1440},
	})

	picked, _, ok := BestMatch([]Profile{partial, exact}, monitors)
	if !ok {
		t.Fatalf("expected match")
	}
	if picked.Name != "desk" {
		t.Fatalf("expected desk, got %s", picked.Name)
	}
}

func TestBestMatchPrefersProfileWithDisabledOutput(t *testing.T) {
	laptop := hypr.Monitor{Name: "eDP-1", Make: "BOE", Model: "Panel", Serial: "C3"}
	external := hypr.Monitor{Name: "DP-6", Make: "Dell", Model: "P3421W", Serial: "DW1"}
	monitors := []hypr.Monitor{laptop, external}

	// profile-internal-only: only knows about the laptop
	internalOnly := New("profile-internal-only", []OutputConfig{
		{Key: laptop.HardwareKey(), Enabled: true, Scale: 1, Width: 2880, Height: 1920},
	})
	// profile-work-wide: knows about both, disables the laptop
	workWide := New("profile-work-wide", []OutputConfig{
		{Key: external.HardwareKey(), Enabled: true, Scale: 1, Width: 3440, Height: 1440},
		{Key: laptop.HardwareKey(), Enabled: false},
	})

	picked, _, ok := BestMatch([]Profile{internalOnly, workWide}, monitors)
	if !ok {
		t.Fatalf("expected match")
	}
	if picked.Name != "profile-work-wide" {
		t.Fatalf("expected profile-work-wide, got %s (work-wide knows about both monitors including disabled laptop)", picked.Name)
	}
}

func TestBestMatchCountsDuplicateMonitorsSeparately(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "DP-5", Make: "VIE", Model: "C24PULSE", Serial: "0x01010101"},
		{Name: "DP-6", Make: "VIE", Model: "C24PULSE", Serial: "0x01010101"},
	}
	legacyKey := monitors[0].HardwareKey()

	single := New("single", []OutputConfig{
		{Key: legacyKey, Name: "DP-5", Enabled: true, Width: 1920, Height: 1080, Scale: 1},
	})
	dual := New("dual", []OutputConfig{
		{Key: legacyKey, Name: "DP-5", Enabled: true, Width: 1920, Height: 1080, Scale: 1},
		{Key: legacyKey, Name: "DP-6", Enabled: true, Width: 1920, Height: 1080, Scale: 1},
	})

	picked, _, ok := BestMatch([]Profile{single, dual}, monitors)
	if !ok {
		t.Fatal("expected match")
	}
	if picked.Name != "dual" {
		t.Fatalf("expected duplicate-aware match to prefer dual, got %q", picked.Name)
	}
}

func TestMonitorSetHashIsStable(t *testing.T) {
	m1 := hypr.Monitor{Name: "DP-1", Make: "Dell", Model: "U2720Q", Serial: "A1"}
	m2 := hypr.Monitor{Name: "HDMI-A-1", Make: "LG", Model: "27GP850", Serial: "B2"}

	h1 := MonitorSetHash([]hypr.Monitor{m1, m2})
	h2 := MonitorSetHash([]hypr.Monitor{m2, m1})

	if h1 != h2 {
		t.Fatalf("expected stable hash, got %q vs %q", h1, h2)
	}
}

func TestMonitorStateHashIsStableAndTracksState(t *testing.T) {
	m1 := hypr.Monitor{
		Name:        "DP-1",
		Make:        "Dell",
		Model:       "U2720Q",
		Serial:      "A1",
		Width:       2560,
		Height:      1440,
		RefreshRate: 144,
		X:           0,
		Y:           0,
		Scale:       1,
	}
	m2 := hypr.Monitor{
		Name:        "eDP-1",
		Make:        "BOE",
		Model:       "Panel",
		Serial:      "C3",
		Width:       1920,
		Height:      1200,
		RefreshRate: 60,
		X:           2560,
		Y:           0,
		Scale:       1.25,
	}

	h1 := MonitorStateHash([]hypr.Monitor{m1, m2})
	h2 := MonitorStateHash([]hypr.Monitor{m2, m1})

	if h1 != h2 {
		t.Fatalf("expected stable hash, got %q vs %q", h1, h2)
	}

	changed := m1
	changed.Disabled = true

	if MonitorStateHash([]hypr.Monitor{changed, m2}) == h1 {
		t.Fatalf("expected disabled state change to affect monitor state hash")
	}
}

func TestExactStateMatchFindsUniqueExactProfile(t *testing.T) {
	monitors := []hypr.Monitor{
		{
			Name:        "DP-1",
			Make:        "Dell",
			Model:       "U2720Q",
			Serial:      "A1",
			Width:       2560,
			Height:      1440,
			RefreshRate: 144,
			X:           0,
			Y:           0,
			Scale:       1,
		},
		{
			Name:        "eDP-1",
			Make:        "BOE",
			Model:       "Panel",
			Serial:      "C3",
			Width:       1920,
			Height:      1200,
			RefreshRate: 60,
			X:           2560,
			Y:           0,
			Scale:       1.25,
		},
	}
	rules := []hypr.WorkspaceRule{
		{WorkspaceString: "1", Monitor: "DP-1", Default: true, Persistent: true},
		{WorkspaceString: "2", Monitor: "eDP-1", Default: true, Persistent: true},
	}

	exact := FromState("desk", monitors, rules)
	changed := exact
	changed.Name = "desk-shifted"
	changed.Outputs = append([]OutputConfig(nil), exact.Outputs...)
	changed.Outputs[0].X = 50

	got, ok := ExactStateMatch([]Profile{changed, exact}, monitors, rules)
	if !ok {
		t.Fatal("expected exact state match")
	}
	if got.Name != "desk" {
		t.Fatalf("expected desk exact state match, got %q", got.Name)
	}
}

func TestExactStateMatchRejectsAmbiguousDuplicateProfiles(t *testing.T) {
	monitors := []hypr.Monitor{
		{
			Name:        "DP-1",
			Make:        "Dell",
			Model:       "U2720Q",
			Serial:      "A1",
			Width:       2560,
			Height:      1440,
			RefreshRate: 144,
			X:           0,
			Y:           0,
			Scale:       1,
		},
	}

	left := FromState("desk-a", monitors, nil)
	right := FromState("desk-b", monitors, nil)

	if _, ok := ExactStateMatch([]Profile{left, right}, monitors, nil); ok {
		t.Fatal("expected ambiguous exact profile matches to be rejected")
	}
}

func TestExactStateMatchIgnoresConfigOnlyFields(t *testing.T) {
	monitors := []hypr.Monitor{{
		Name: "DP-1", Make: "Dell", Model: "U2720Q", Serial: "A1",
		Width: 2560, Height: 1440, RefreshRate: 144,
		Scale: 1,
	}}

	saved := FromState("desk", monitors, nil)
	saved.Outputs[0].VRR = 2
	saved.Outputs[0].MinLuminance = 0.005
	saved.Outputs[0].MaxLuminance = 800
	saved.Outputs[0].SupportsWideColor = 1
	saved.Outputs[0].SupportsHDR = 1
	saved.Outputs[0].MaxAvgLuminance = 500
	saved.Outputs[0].SDREOTF = "gamma22"
	saved.Outputs[0].ICC = "/path/to/icc"

	got, ok := ExactStateMatch([]Profile{saved}, monitors, nil)
	if !ok {
		t.Fatal("expected ExactStateMatch to succeed despite config-only field differences")
	}
	if got.Name != "desk" {
		t.Fatalf("expected desk, got %q", got.Name)
	}
}

func TestExactStateMatchDetectsBitdepthAndCMDifference(t *testing.T) {
	monitors := []hypr.Monitor{{
		Name: "DP-1", Make: "Dell", Model: "U2720Q", Serial: "A1",
		Width: 2560, Height: 1440, RefreshRate: 144,
		Scale: 1, CurrentFormat: "XRGB8888", ColorManagementPreset: "srgb",
	}}

	saved := FromState("desk", monitors, nil)
	saved.Outputs[0].Bitdepth = 10
	saved.Outputs[0].CM = "wide"

	if _, ok := ExactStateMatch([]Profile{saved}, monitors, nil); ok {
		t.Fatal("expected ExactStateMatch to fail when Bitdepth and CM differ from live state")
	}
}

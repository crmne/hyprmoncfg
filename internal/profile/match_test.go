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

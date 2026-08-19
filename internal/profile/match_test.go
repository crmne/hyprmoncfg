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

func TestEvaluateMatchReportsConnectedEnabledOutputs(t *testing.T) {
	laptop := hypr.Monitor{Name: "eDP-1", Make: "BOE", Model: "Panel", Serial: "C3"}
	external := hypr.Monitor{Name: "DP-6", Make: "Dell", Model: "P3421W", Serial: "DW1"}
	projector := hypr.Monitor{Name: "HDMI-A-1", Make: "Epson", Model: "Projector", Serial: "P1"}

	desk := New("desk", []OutputConfig{
		{Key: external.HardwareKey(), Enabled: true, Scale: 1},
		{Key: laptop.HardwareKey(), Enabled: false},
	})
	result := EvaluateMatch(desk, []hypr.Monitor{laptop, external})
	if result.ConnectedEnabledOutputs != 1 || result.Score != 150 {
		t.Fatalf("desk match = %+v, want one connected enabled output and score 150", result)
	}

	unavailable := New("projector", []OutputConfig{{Key: projector.HardwareKey(), Enabled: true, Scale: 1}})
	result = EvaluateMatch(unavailable, []hypr.Monitor{laptop, external})
	if result.ConnectedEnabledOutputs != 0 || result.Score != 0 {
		t.Fatalf("unavailable match = %+v, want zero value", result)
	}

	unsafe := New("external-only", []OutputConfig{
		{Key: projector.HardwareKey(), Enabled: true, Scale: 1},
		{Key: laptop.HardwareKey(), Enabled: false},
	})
	result = EvaluateMatch(unsafe, []hypr.Monitor{laptop})
	if result.ConnectedEnabledOutputs != 0 || result.Score != 0 {
		t.Fatalf("disabled-only hardware match = %+v, want profile to be unavailable", result)
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

func TestExactStateMatchAcceptsRoundedHyprlandScaleReadback(t *testing.T) {
	monitors := []hypr.Monitor{{
		Name: "DP-1", Make: "Microstep", Model: "MPG321UR-QD", Serial: "A1",
		Width: 3840, Height: 2160, RefreshRate: 144,
		Scale: 1.33,
	}}

	saved := FromState("desk", []hypr.Monitor{{
		Name: "DP-1", Make: "Microstep", Model: "MPG321UR-QD", Serial: "A1",
		Width: 3840, Height: 2160, RefreshRate: 144,
		Scale: 1.33333,
	}}, nil)

	if _, ok := ExactStateMatch([]Profile{saved}, monitors, nil); !ok {
		t.Fatal("expected ExactStateMatch to accept Hyprland's rounded scale readback")
	}
}

func TestRoundedScaleReadbackRequiresSharpSavedScale(t *testing.T) {
	if ScaleMatchesRoundedReadback(3840, 2160, 1.37, 1.368) {
		t.Fatal("expected rounded readback matching to require a sharp saved scale")
	}
}

func TestAbsorbRequestedScalesKeepsLayoutAndUpdatesMatchingOutput(t *testing.T) {
	laptop := hypr.Monitor{
		Name: "eDP-1", Make: "BOE", Model: "Panel", Serial: "C3",
		Width: 1920, Height: 1080, RefreshRate: 144, X: 0, Y: 0, Scale: 1.25,
	}
	external := hypr.Monitor{
		Name: "HDMI-A-1", Make: "LG", Model: "FHD", Serial: "D4",
		Width: 1920, Height: 1080, RefreshRate: 60, X: 1920, Y: 76, Scale: 1,
	}
	saved := New("desk", []OutputConfig{
		{Key: laptop.HardwareKey(), Name: laptop.Name, Enabled: true, Width: 1920, Height: 1080, Refresh: 144, X: 1920, Y: 76, Scale: 2},
		{Key: external.HardwareKey(), Name: external.Name, Enabled: true, Width: 1920, Height: 1080, Refresh: 60, X: 0, Y: -700, Scale: 1},
	})

	updated, changed := AbsorbRequestedScales(saved, []hypr.Monitor{laptop, external}, map[string]float64{"eDP-1": 1.25})
	if len(changed) != 1 || changed[0] != "eDP-1" {
		t.Fatalf("changed = %v, want [eDP-1]", changed)
	}
	laptopOut, ok := updated.OutputByKey(laptop.HardwareKey())
	if !ok || laptopOut.Scale != 1.25 || laptopOut.X != 1920 || laptopOut.Y != 76 {
		t.Fatalf("laptop output = %+v", laptopOut)
	}
	externalOut, ok := updated.OutputByKey(external.HardwareKey())
	if !ok || externalOut.Scale != 1 || externalOut.X != 0 {
		t.Fatalf("external output = %+v", externalOut)
	}
}

func TestAbsorbRequestedScalesIgnoresUnlandedAndDisabledOutputs(t *testing.T) {
	laptop := hypr.Monitor{
		Name: "eDP-1", Make: "BOE", Model: "Panel", Serial: "C3",
		Width: 1920, Height: 1080, Scale: 2,
	}
	saved := New("desk", []OutputConfig{
		{Key: laptop.HardwareKey(), Name: laptop.Name, Enabled: true, Width: 1920, Height: 1080, Scale: 2},
	})

	if _, changed := AbsorbRequestedScales(saved, []hypr.Monitor{laptop}, map[string]float64{"eDP-1": 1.25}); changed != nil {
		t.Fatalf("live scale did not land, changed = %v", changed)
	}

	disabled := laptop
	disabled.Disabled = true
	disabled.Scale = 1.25
	if _, changed := AbsorbRequestedScales(saved, []hypr.Monitor{disabled}, map[string]float64{"eDP-1": 1.25}); changed != nil {
		t.Fatalf("disabled output, changed = %v", changed)
	}
}

func TestExactStateMatchTreatsOmittedDefaultsAsLiveDefaults(t *testing.T) {
	monitors := []hypr.Monitor{{
		Name: "DP-1", Make: "Microstep", Model: "MPG321UR-QD", Serial: "A1",
		Width: 3840, Height: 2160, RefreshRate: 144,
		Scale: 1.33, CurrentFormat: "XRGB8888", ColorManagementPreset: "srgb",
		SDRBrightness: 1, SDRSaturation: 1,
	}}

	saved := FromState("desk", []hypr.Monitor{{
		Name: "DP-1", Make: "Microstep", Model: "MPG321UR-QD", Serial: "A1",
		Width: 3840, Height: 2160, RefreshRate: 144,
		Scale: 1.33333,
	}}, nil)
	saved.Outputs[0].Bitdepth = 0
	saved.Outputs[0].CM = ""
	saved.Outputs[0].SDRBrightness = 0
	saved.Outputs[0].SDRSaturation = 0

	if _, ok := ExactStateMatch([]Profile{saved}, monitors, nil); !ok {
		t.Fatal("expected ExactStateMatch to treat omitted defaults as live defaults")
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

func TestEvaluateMatchPenalizesDisplaysThatAreNotConnected(t *testing.T) {
	desk := hypr.Monitor{Name: "DP-1", Make: "Microstep", Model: "MPG321UR-QD", Serial: "M1"}
	tv := hypr.Monitor{Name: "HDMI-A-1", Make: "LG", Model: "TV", Serial: "T1"}
	connected := []hypr.Monitor{desk}

	solo := New("solo", []OutputConfig{
		{Key: desk.HardwareKey(), Enabled: true, Scale: 1},
	})
	withTVOff := New("tv-off", []OutputConfig{
		{Key: desk.HardwareKey(), Enabled: true, Scale: 1},
		{Key: tv.HardwareKey(), Enabled: false},
	})
	withTVOn := New("tv-on", []OutputConfig{
		{Key: desk.HardwareKey(), Enabled: true, Scale: 1},
		{Key: tv.HardwareKey(), Enabled: true, Scale: 1},
	})

	soloResult := EvaluateMatch(solo, connected)
	offResult := EvaluateMatch(withTVOff, connected)
	onResult := EvaluateMatch(withTVOn, connected)

	if soloResult.Score <= offResult.Score {
		t.Fatalf("profile describing only the connected display should win: solo=%d tv-off=%d",
			soloResult.Score, offResult.Score)
	}
	if offResult.Score <= onResult.Score {
		t.Fatalf("an absent display kept off should cost less than one left on: tv-off=%d tv-on=%d",
			offResult.Score, onResult.Score)
	}
	if offResult.MissingOffOutputs != 1 || offResult.MissingOutputs != 0 {
		t.Fatalf("tv-off breakdown = %+v, want one display missing while kept off", offResult)
	}
	if onResult.MissingOutputs != 1 || onResult.MissingOffOutputs != 0 {
		t.Fatalf("tv-on breakdown = %+v, want one display missing while enabled", onResult)
	}

	picked, _, ok := BestMatch([]Profile{withTVOff, withTVOn, solo}, connected)
	if !ok || picked.Name != "solo" {
		t.Fatalf("BestMatch = %q (ok=%v), want solo", picked.Name, ok)
	}
}

func TestEvaluateMatchStillRewardsDisplaysItKeepsOff(t *testing.T) {
	laptop := hypr.Monitor{Name: "eDP-1", Make: "BOE", Model: "Panel", Serial: "C3"}
	desk := hypr.Monitor{Name: "DP-1", Make: "Dell", Model: "P3421W", Serial: "DW1"}
	connected := []hypr.Monitor{laptop, desk}

	clamshell := New("clamshell", []OutputConfig{
		{Key: desk.HardwareKey(), Enabled: true, Scale: 1},
		{Key: laptop.HardwareKey(), Enabled: false},
	})
	deskOnly := New("desk-only", []OutputConfig{
		{Key: desk.HardwareKey(), Enabled: true, Scale: 1},
	})

	if got, want := EvaluateMatch(clamshell, connected).Score, 150; got != want {
		t.Fatalf("clamshell score = %d, want %d", got, want)
	}
	picked, _, ok := BestMatch([]Profile{deskOnly, clamshell}, connected)
	if !ok || picked.Name != "clamshell" {
		t.Fatalf("BestMatch = %q (ok=%v), want clamshell", picked.Name, ok)
	}
}

func TestExactStateMatchIgnoresWhereHyprlandPutsAMirror(t *testing.T) {
	desk := hypr.Monitor{
		Name: "DP-1", Description: "Microstep MPG321UR-QD", Make: "Microstep", Model: "MPG321UR-QD",
		Width: 3840, Height: 2160, RefreshRate: 143.99, X: 0, Y: 0, Scale: 1.33333,
	}
	// Hyprland reports the mirror wherever it decided to put it, not where the
	// profile asked for it.
	tv := hypr.Monitor{
		Name: "HDMI-A-1", Description: "LG TV", Make: "LG", Model: "TV",
		Width: 3840, Height: 2160, RefreshRate: 60, X: 3820, Y: 927, Scale: 1.5,
		MirrorOf: "DP-1",
	}
	monitors := []hypr.Monitor{desk, tv}

	saved := FromState("desk", monitors, nil)
	for idx := range saved.Outputs {
		if saved.Outputs[idx].Name == "HDMI-A-1" {
			saved.Outputs[idx].X, saved.Outputs[idx].Y = 0, 0
		}
	}

	matched, ok := ExactStateMatch([]Profile{saved}, monitors, nil)
	if !ok || matched.Name != "desk" {
		t.Fatalf("expected the profile to still be recognized as active, got %q (ok=%v)", matched.Name, ok)
	}

	// The mirror target itself still has to agree.
	elsewhere := FromState("elsewhere", monitors, nil)
	for idx := range elsewhere.Outputs {
		if elsewhere.Outputs[idx].Name == "HDMI-A-1" {
			elsewhere.Outputs[idx].MirrorOf = ""
		}
	}
	if _, ok := ExactStateMatch([]Profile{elsewhere}, monitors, nil); ok {
		t.Fatal("expected a profile that does not mirror to be a different state")
	}
}

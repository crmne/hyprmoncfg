package profile

import (
	"testing"

	"github.com/crmne/hyprmoncfg/internal/hypr"
)

// Issue #126: hot-desking onto another unit of the saved monitor model.
var (
	deskLaptop = hypr.Monitor{Name: "eDP-1", Description: "BOE NE135", Make: "BOE", Model: "NE135", Width: 2880, Height: 1920, RefreshRate: 120, Scale: 2}
	deskHome   = hypr.Monitor{Name: "DP-1", Description: "Dell Inc. DELL U2723QE HOME", Make: "Dell Inc.", Model: "DELL U2723QE", Serial: "HOME", Width: 3840, Height: 2160, RefreshRate: 60, Scale: 1.5}
	deskOffice = hypr.Monitor{Name: "DP-3", Description: "Dell Inc. DELL U2723QE OFFICE", Make: "Dell Inc.", Model: "DELL U2723QE", Serial: "OFFICE", Width: 3840, Height: 2160, RefreshRate: 60, Scale: 1.5}
)

// deskProfile is the laptop with the external above it, which Hyprland would
// never pick on its own, and calibration measured on the saved unit.
func deskProfile(t *testing.T, external hypr.Monitor) Profile {
	t.Helper()
	laptop := deskLaptop
	laptop.X, laptop.Y = 640, 1440
	external.X, external.Y = 0, 0
	p := FromState("Desk", []hypr.Monitor{laptop, external}, nil)
	for i := range p.Outputs {
		if p.Outputs[i].Serial == external.Serial {
			p.Outputs[i].ICC = "/home/me/.local/share/icc/u2723qe-" + external.Serial + ".icc"
			p.Outputs[i].CM = "hdr"
			p.Outputs[i].MinLuminance = 0.05
			p.Outputs[i].MaxLuminance = 600
			p.Outputs[i].MaxAvgLuminance = 400
			p.Outputs[i].SDRMaxLuminance = 250
		}
	}
	p.Workspaces = WorkspaceSettings{
		Enabled:  true,
		Strategy: WorkspaceStrategyManual,
		Rules: []WorkspaceRule{
			{Workspace: "1", OutputKey: external.HardwareKey(), Default: true},
			{Workspace: "2", OutputKey: deskLaptop.HardwareKey()},
		},
	}
	p.Normalize()
	return p
}

func laptopOnlyProfile() Profile {
	return FromState("Laptop", []hypr.Monitor{deskLaptop}, nil)
}

func TestAnotherUnitOfTheSavedModelSelectsTheSavedLayout(t *testing.T) {
	desk := deskProfile(t, deskHome)
	laptop := laptopOnlyProfile()
	monitors := []hypr.Monitor{deskLaptop, deskOffice}
	opts := MatchOptions{}.WithProfiles([]Profile{laptop, desk})

	picked, score, ok := BestMatch([]Profile{laptop, desk}, monitors)
	if !ok || picked.Name != "Desk" {
		t.Fatalf("picked %q (ok=%t), want Desk on another unit of its monitor model", picked.Name, ok)
	}
	if want := ScoreEnabledMatch + ScoreModelMatch; score != want {
		t.Fatalf("score = %d, want %d", score, want)
	}
	if laptopScore := EvaluateMatchWith(laptop, monitors, opts).Score; laptopScore >= score {
		t.Fatalf("laptop-only score %d should lose to %d", laptopScore, score)
	}

	result := EvaluateMatchWith(desk, monitors, opts)
	if result.ModelMatched != 1 || result.MissingOutputs != 0 || result.UnknownOutputs != 0 || result.ConnectedEnabledOutputs != 2 {
		t.Fatalf("desk match = %+v, want one display matched by model and nothing missing or unknown", result)
	}
	if result.ExactDisplayMatch() {
		t.Fatalf("a different unit is not an exact display match: %+v", result)
	}
	reasons := ExplainMatch(result)
	want := []MatchReason{
		{Kind: MatchReasonConnected, Count: 1, Points: ScoreEnabledMatch},
		{Kind: MatchReasonConnectedSameModel, Count: 1, Points: ScoreModelMatch},
	}
	if len(reasons) != len(want) || reasons[0] != want[0] || reasons[1] != want[1] {
		t.Fatalf("reasons = %+v, want %+v", reasons, want)
	}
}

func TestTheSavedUnitAlwaysBeatsAnotherUnitOfItsModel(t *testing.T) {
	home := deskProfile(t, deskHome)
	home.Name = "Home"
	office := deskProfile(t, deskOffice)
	office.Name = "Office"
	monitors := []hypr.Monitor{deskLaptop, deskOffice}

	picked, score, ok := BestMatch([]Profile{home, office}, monitors)
	if !ok || picked.Name != "Office" || score != 2*ScoreEnabledMatch {
		t.Fatalf("picked %q score %d (ok=%t), want the profile saved with this unit", picked.Name, score, ok)
	}
	// Office names this unit, so Home does not claim it by model.
	result := EvaluateMatchWith(home, monitors, MatchOptions{}.WithProfiles([]Profile{home, office}))
	if result.ModelMatched != 0 || result.UnknownOutputs != 1 || result.MissingOutputs != 1 {
		t.Fatalf("home match = %+v, want the office unit left unknown", result)
	}
}

// A profile that names the unit exactly wins even when it scores below what a
// model match on its own would: it keeps the laptop off, worth less than on.
func TestAProfileNamingTheUnitWinsOverAModelMatchElsewhere(t *testing.T) {
	desk := deskProfile(t, deskHome)
	officeOnly := New("Office monitor", []OutputConfig{
		{Key: deskOffice.HardwareKey(), Name: deskOffice.Name, Make: deskOffice.Make, Model: deskOffice.Model, Serial: deskOffice.Serial, Enabled: true, Scale: 1.5, Width: 3840, Height: 2160},
		{Key: deskLaptop.HardwareKey(), Name: deskLaptop.Name, Make: deskLaptop.Make, Model: deskLaptop.Model, Enabled: false},
	})
	monitors := []hypr.Monitor{deskLaptop, deskOffice}

	if alone := EvaluateMatch(desk, monitors).Score; alone <= EvaluateMatch(officeOnly, monitors).Score {
		t.Fatalf("fixture: desk alone scores %d, want it above the office profile to prove the rule", alone)
	}
	picked, _, ok := BestMatch([]Profile{desk, officeOnly}, monitors)
	if !ok || picked.Name != "Office monitor" {
		t.Fatalf("picked %q (ok=%t), want the profile that names the connected unit", picked.Name, ok)
	}
}

func TestModelMatchingNeverGuessesBetweenIdenticalUnits(t *testing.T) {
	other := deskOffice
	other.Name, other.Serial, other.Description = "DP-4", "SPARE", "Dell Inc. DELL U2723QE SPARE"

	t.Run("one saved unit, two new ones", func(t *testing.T) {
		desk := deskProfile(t, deskHome)
		result := EvaluateMatch(desk, []hypr.Monitor{deskLaptop, deskOffice, other})
		if result.ModelMatched != 0 {
			t.Fatalf("match = %+v, want no model match with two candidates", result)
		}
	})

	t.Run("two saved units, two new ones", func(t *testing.T) {
		left, right := deskHome, deskHome
		right.Name, right.Serial, right.Description, right.X = "DP-2", "HOME2", "Dell Inc. DELL U2723QE HOME2", 2560
		twin := FromState("Twin", []hypr.Monitor{left, right}, nil)
		result := EvaluateMatch(twin, []hypr.Monitor{deskOffice, other})
		if result.ModelMatched != 0 || result.Score != 0 {
			t.Fatalf("match = %+v, want no guess about which unit goes where", result)
		}
		bound, matches := BindModelMatches(twin, []hypr.Monitor{deskOffice, other}, []Profile{twin})
		if len(matches) != 0 || bound.Outputs[0].Serial == deskOffice.Serial || bound.Outputs[1].Serial == deskOffice.Serial {
			t.Fatalf("bound %+v, want the twin layout left alone", bound.Outputs)
		}
	})

	t.Run("two saved units, one still here", func(t *testing.T) {
		left, right := deskHome, deskHome
		right.Name, right.Serial, right.Description, right.X = "DP-2", "HOME2", "Dell Inc. DELL U2723QE HOME2", 2560
		twin := FromState("Twin", []hypr.Monitor{left, right}, nil)
		result := EvaluateMatch(twin, []hypr.Monitor{deskHome, deskOffice})
		if result.EnabledMatched != 1 || result.ModelMatched != 1 || result.MissingOutputs != 0 {
			t.Fatalf("match = %+v, want the saved unit exact and the other placed by model", result)
		}
	})
}

func TestModelMatchingLeavesStrictProfilesAlone(t *testing.T) {
	desk := deskProfile(t, deskHome)
	desk.DisableUnknownOutputs = true
	monitors := []hypr.Monitor{deskLaptop, deskOffice}

	result := EvaluateMatch(desk, monitors)
	if result.ModelMatched != 0 || result.UnknownOutputs != 1 {
		t.Fatalf("match = %+v, want Keep other displays off to keep the new unit off", result)
	}
	if _, matches := BindModelMatches(desk, monitors, []Profile{desk}); len(matches) != 0 {
		t.Fatalf("strict profile bound %+v", matches)
	}
}

func TestModelMatchingSkipsBuiltInAndKeptOffDisplays(t *testing.T) {
	otherPanel := deskLaptop
	otherPanel.Serial = "PANEL2"
	if result := EvaluateMatch(laptopOnlyProfile(), []hypr.Monitor{otherPanel}); result.ModelMatched != 0 {
		t.Fatalf("built-in panel matched by model: %+v", result)
	}

	desk := deskProfile(t, deskHome)
	for i := range desk.Outputs {
		if desk.Outputs[i].Serial == deskHome.Serial {
			desk.Outputs[i].Enabled = false
		}
	}
	if result := EvaluateMatch(desk, []hypr.Monitor{deskLaptop, deskOffice}); result.ModelMatched != 0 || result.UnknownOutputs != 1 {
		t.Fatalf("kept-off display stood in for a new unit: %+v", result)
	}
}

func TestBindModelMatchesMovesTheLayoutOntoTheConnectedUnit(t *testing.T) {
	desk := deskProfile(t, deskHome)
	monitors := []hypr.Monitor{deskLaptop, deskOffice}

	bound, matches := BindModelMatches(desk, monitors, []Profile{desk})
	if len(matches) != 1 || matches[0].Monitor.Name != deskOffice.Name || matches[0].OutputKey != deskHome.HardwareKey() {
		t.Fatalf("matches = %+v", matches)
	}
	output, ok := bound.OutputByKey(deskOffice.HardwareKey())
	if !ok {
		t.Fatalf("bound outputs %+v, want one keyed to the office unit", bound.Outputs)
	}
	if output.Serial != "OFFICE" || output.Name != "DP-3" || output.Description != deskOffice.Description || output.MatchIdentity() != deskOffice.HardwareKey() {
		t.Fatalf("bound identity = %+v, want the connected unit's", output)
	}
	if output.X != 0 || output.Y != 0 || output.Scale != 1.5 || output.CM != "hdr" || output.SDRMaxLuminance != 250 {
		t.Fatalf("bound layout = %+v, want the saved layout and signal settings", output)
	}
	if output.ICC != "" || output.MinLuminance != 0 || output.MaxLuminance != 0 || output.MaxAvgLuminance != 0 {
		t.Fatalf("bound calibration = %+v, want the saved unit's ICC and luminance overrides dropped", output)
	}
	if _, stale := bound.OutputByKey(deskHome.HardwareKey()); stale {
		t.Fatalf("bound profile still names the saved unit: %+v", bound.Outputs)
	}
	if rule := bound.Workspaces.Rules[0]; rule.Workspace != "1" || rule.OutputKey != deskOffice.HardwareKey() || rule.OutputName != "DP-3" {
		t.Fatalf("workspace rule = %+v, want it to follow the layout onto the unit", rule)
	}
	if omitted := OmittedMonitors(bound, monitors); len(omitted) != 0 {
		t.Fatalf("omitted %+v, want every connected display placed", omitted)
	}

	// The saved profile is untouched.
	saved, _ := desk.OutputByKey(deskHome.HardwareKey())
	if saved.Serial != "HOME" || saved.ICC == "" || saved.MaxLuminance != 600 {
		t.Fatalf("saved output changed: %+v", saved)
	}
}

func TestExactStateMatchRecognizesTheLayoutOnAnotherUnit(t *testing.T) {
	saved := []hypr.Monitor{deskLaptop, deskHome}
	saved[0].X, saved[0].Y = 640, 1440
	desk := FromState("Desk", saved, nil)
	laptop := laptopOnlyProfile()
	live := []hypr.Monitor{saved[0], deskOffice}

	active, ok := ExactStateMatch([]Profile{laptop, desk}, live, nil)
	if !ok || active.Name != "Desk" {
		t.Fatalf("active %q (ok=%t), want Desk shown as active on the office unit", active.Name, ok)
	}
	if output, _ := active.OutputByKey(deskHome.HardwareKey()); output.Serial != "HOME" {
		t.Fatalf("ExactStateMatch returned %+v, want the saved profile", active.Outputs)
	}
}

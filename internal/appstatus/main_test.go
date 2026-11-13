package appstatus

import (
	"testing"

	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

func twoDisplays() []hypr.Monitor {
	return []hypr.Monitor{
		{Name: "DP-1", Make: "Example", Model: "Left", Serial: "A", Width: 1920, Height: 1080, RefreshRate: 60, Scale: 1},
		{Name: "DP-2", Make: "Example", Model: "Right", Serial: "B", Width: 1920, Height: 1080, RefreshRate: 60, Scale: 1, X: 1920},
	}
}

func withMain(p profile.Profile, index int) profile.Profile {
	p.Outputs = append([]profile.OutputConfig(nil), p.Outputs...)
	p.Outputs[index].Main = true
	return p
}

func TestActiveProfileMainUsesSelectionForLayoutTwins(t *testing.T) {
	monitors := twoDisplays()
	left := withMain(profile.FromState("Left main", monitors, nil), 0)
	right := withMain(profile.FromState("Right main", monitors, nil), 1)
	profiles := []profile.Profile{left, right}

	if document := Build("test", true, profiles, monitors, nil, profile.MatchOptions{}); document.ActiveProfile != nil {
		t.Fatalf("without a selection twins must stay ambiguous, got %+v", document.ActiveProfile)
	}
	for _, want := range []profile.Profile{left, right} {
		document := BuildWithSelection("test", true, profiles, monitors, nil, profile.MatchOptions{}, want.Name)
		mainOutput, _ := want.MainOutput()
		if document.ActiveProfile == nil || document.ActiveProfile.Name != want.Name || document.ActiveProfile.Main != mainOutput.Key {
			t.Fatalf("selected %q: active = %+v", want.Name, document.ActiveProfile)
		}
	}
	if document := BuildWithSelection("test", true, profiles, monitors, nil, profile.MatchOptions{}, "Elsewhere"); document.ActiveProfile != nil {
		t.Fatalf("a selection outside the matches must not pick one, got %+v", document.ActiveProfile)
	}
}

func TestActiveProfileMainUsesSuffixedLiveKeyForDuplicates(t *testing.T) {
	monitors := twoDisplays()
	for i := range monitors {
		monitors[i].Model, monitors[i].Serial = "Twin", ""
	}
	saved := withMain(profile.FromState("Twins", monitors, nil), 1)
	document := Build("test", true, []profile.Profile{saved}, monitors, nil, profile.MatchOptions{})
	if document.ActiveProfile == nil || document.ActiveProfile.Main == "" {
		t.Fatalf("active = %+v", document.ActiveProfile)
	}
	var want string
	for _, monitor := range document.Monitors {
		if monitor.Name == "DP-2" {
			want = monitor.Key
		}
	}
	if document.ActiveProfile.Main != want {
		t.Fatalf("main = %q, want monitors[].key %q", document.ActiveProfile.Main, want)
	}
}

func TestActiveProfileMainResolvesUnsuffixedSavedKeyToLiveKey(t *testing.T) {
	monitors := twoDisplays()
	saved := withMain(profile.FromState("Desk", monitors, nil), 1)
	// Strict profile omitting a disabled display: live has a duplicate of
	// the main display's identity, so its live key gains a suffix while the
	// saved key does not.
	saved.DisableUnknownOutputs = true
	live := append(twoDisplays(), hypr.Monitor{Name: "DP-3", Make: "Example", Model: "Right", Serial: "B", Disabled: true})
	document := Build("test", true, []profile.Profile{saved}, live, nil, profile.MatchOptions{})
	if document.ActiveProfile == nil {
		t.Fatal("strict profile omitting a disabled duplicate should stay active")
	}
	want := "example|right|b@dp-2"
	if mainOutput, _ := saved.MainOutput(); mainOutput.Key == want {
		t.Fatalf("fixture should save an unsuffixed key, got %q", mainOutput.Key)
	}
	for _, monitor := range document.Monitors {
		if monitor.Name == "DP-2" && monitor.Key != want {
			t.Fatalf("monitors[].key = %q, want %q", monitor.Key, want)
		}
	}
	if document.ActiveProfile.Main != want {
		t.Fatalf("main = %q, want %q", document.ActiveProfile.Main, want)
	}
}

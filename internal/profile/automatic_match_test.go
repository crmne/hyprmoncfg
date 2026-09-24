package profile

import (
	"reflect"
	"testing"

	"github.com/crmne/hyprmoncfg/internal/hypr"
)

func TestPartialMatchExtendsUnfamiliarDisplaysWithoutChangingSavedLayout(t *testing.T) {
	laptop := hypr.Monitor{Name: "eDP-1", Make: "Example", Model: "Laptop", Width: 1920, Height: 1080, RefreshRate: 60, Scale: 1}
	projector := hypr.Monitor{Name: "DP-1", Make: "Example", Model: "Projector", Width: 1920, Height: 1080, RefreshRate: 60, Scale: 1, X: 1920}
	home := []hypr.Monitor{laptop,
		{Name: "DP-2", Make: "Example", Model: "Wide Panel", Width: 1920, Height: 1080, RefreshRate: 60, Scale: 1},
		{Name: "DP-3", Make: "Example", Model: "Panel", Serial: "Left", Width: 1920, Height: 1080, RefreshRate: 60, Scale: 1},
		{Name: "DP-4", Make: "Example", Model: "Panel", Serial: "Right", Width: 1920, Height: 1080, RefreshRate: 60, Scale: 1},
	}
	meeting := FromMonitors("Meeting", []hypr.Monitor{laptop, projector})
	original := cloneProfile(meeting)
	best, score, ok := BestMatch([]Profile{meeting}, home)
	if !ok || score != 10 {
		t.Fatalf("fixture must reproduce positive partial match: score=%d ok=%v", score, ok)
	}
	extended := ExtendConnected(best, home)
	for _, monitor := range home {
		output, exists := extended.OutputByKey(monitor.HardwareKey())
		if !exists || !output.Enabled {
			t.Fatalf("partial base omitted a connected display: %+v", monitor)
		}
	}
	if err := ValidateLayout(extended.Outputs); err != nil {
		t.Fatalf("extended layout overlaps: %v", err)
	}
	if !reflect.DeepEqual(meeting, original) {
		t.Fatal("extending the partial match changed the saved layout")
	}
	profiles := []Profile{meeting, FromMonitors("Home", home)}
	if p, _, ok := BestMatch(profiles, home); !ok || p.Name != "Home" {
		t.Fatalf("expected exact Home selection, got %q ok=%v", p.Name, ok)
	}
	if p, _, ok := BestMatch([]Profile{meeting}, []hypr.Monitor{laptop}); !ok || p.Name != "Meeting" {
		t.Fatalf("undocking must retain laptop fallback, got %q ok=%v", p.Name, ok)
	}
}

func TestPartialMatchHonorsExplicitStrictPolicy(t *testing.T) {
	monitors := []hypr.Monitor{{Name: "eDP-1", Make: "Example", Model: "Laptop"}, {Name: "DP-1", Make: "Example", Model: "Desk"}}
	saved := FromMonitors("Strict laptop", monitors[:1])
	saved.DisableUnknownOutputs = true
	best, _, ok := BestMatch([]Profile{saved}, monitors)
	if !ok || best.Name != saved.Name {
		t.Fatalf("explicit strict profile must remain selectable: %q ok=%v", best.Name, ok)
	}
	if extended := ExtendConnected(best, monitors); !reflect.DeepEqual(extended, saved) {
		t.Fatal("automatic extension ignored the explicit strict policy")
	}
}

func TestMatchHonorsExplicitlyDisabledKnownDisplays(t *testing.T) {
	monitors := []hypr.Monitor{{Name: "eDP-1", Make: "Example", Model: "Laptop"}, {Name: "DP-1", Make: "Example", Model: "Desk"}}
	p := FromMonitors("Laptop only at desk", monitors)
	p.Outputs[1].Enabled = false
	if got, _, ok := BestMatch([]Profile{p}, monitors); !ok || got.Name != p.Name {
		t.Fatalf("known disabled display must remain selectable: %q ok=%v", got.Name, ok)
	}
}

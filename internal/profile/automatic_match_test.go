package profile

import (
	"testing"

	"github.com/crmne/hyprmoncfg/internal/hypr"
)

func TestAutomaticMatchRejectsOtherLocationSharingLaptop(t *testing.T) {
	laptop := hypr.Monitor{Name: "eDP-1", Make: "Example", Model: "Laptop"}
	projector := hypr.Monitor{Name: "DP-1", Make: "Example", Model: "Projector"}
	home := []hypr.Monitor{laptop,
		{Name: "DP-2", Make: "Example", Model: "Wide Panel"},
		{Name: "DP-3", Make: "Example", Model: "Panel", Serial: "Left"},
		{Name: "DP-4", Make: "Example", Model: "Panel", Serial: "Right"},
	}
	meeting := FromMonitors("Meeting", []hypr.Monitor{laptop, projector})
	if _, score, ok := BestMatch([]Profile{meeting}, home); !ok || score != 10 {
		t.Fatalf("fixture must reproduce positive partial match: score=%d ok=%v", score, ok)
	}
	if p, _, ok := BestAutomaticMatch([]Profile{meeting}, home); ok {
		t.Fatalf("unexpected automatic selection of %q on unfamiliar home displays", p.Name)
	}
	profiles := []Profile{meeting, FromMonitors("Home", home)}
	if p, _, ok := BestAutomaticMatch(profiles, home); !ok || p.Name != "Home" {
		t.Fatalf("expected exact Home selection, got %q ok=%v", p.Name, ok)
	}
	if p, _, ok := BestAutomaticMatch([]Profile{meeting}, []hypr.Monitor{laptop}); !ok || p.Name != "Meeting" {
		t.Fatalf("undocking must retain laptop fallback, got %q ok=%v", p.Name, ok)
	}
}

func TestAutomaticMatchHonorsExplicitlyDisabledKnownDisplays(t *testing.T) {
	monitors := []hypr.Monitor{{Name: "eDP-1", Make: "Example", Model: "Laptop"}, {Name: "DP-1", Make: "Example", Model: "Desk"}}
	p := FromMonitors("Laptop only at desk", monitors)
	p.Outputs[1].Enabled = false
	if got, _, ok := BestAutomaticMatch([]Profile{p}, monitors); !ok || got.Name != p.Name {
		t.Fatalf("known disabled display must remain selectable: %q ok=%v", got.Name, ok)
	}
}

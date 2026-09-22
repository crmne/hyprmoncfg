package apply

import (
	"strings"
	"testing"

	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

func TestValidationReportsEveryFailedOutput(t *testing.T) {
	before := []hypr.Monitor{{Name: "DP-1", Make: "A", Model: "One", Width: 1920, Height: 1080, Scale: 1}, {Name: "DP-2", Make: "B", Model: "Two", Width: 1920, Height: 1080, Scale: 1}}
	after := append([]hypr.Monitor(nil), before...)
	for i := range after {
		after[i].Width, after[i].Height = 0, 0
	}
	err := ValidateAppliedProfile(profile.FromMonitors("Desk", before), before, after)
	if err == nil || !strings.Contains(err.Error(), "DP-1 mode mismatch") || !strings.Contains(err.Error(), "DP-2 mode mismatch") {
		t.Fatal(err)
	}
}

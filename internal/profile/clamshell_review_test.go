package profile

import (
	"reflect"
	"testing"

	"github.com/crmne/hyprmoncfg/internal/hypr"
)

// Issue #62: a saved enabled laptop must remain enabled even when its current
// live state is already disabled after a previous closed-lid apply.
func TestIssue62SavedLaptopSurvivesModelessExternalAndRecoversPolicy(t *testing.T) {
	internal := hypr.Monitor{Name: "eDP-1", Make: "Laptop", Model: "Panel", Serial: "internal", Width: 1920, Height: 1200, Scale: 1, DPMSStatus: true}
	external := hypr.Monitor{Name: "DP-1", Make: "Dell", Model: "Panel", Serial: "external", Width: 2560, Height: 1440, Scale: 1, DPMSStatus: true}
	saved := FromMonitors("Dock", []hypr.Monitor{internal, external})
	before := cloneProfile(saved)
	internal.Disabled = true
	external.Width, external.Height = 0, 0
	for _, dpms := range []bool{false, true} {
		external.DPMSStatus = dpms
		got, adjustment := ApplyClosedLidPolicy(saved, []hypr.Monitor{internal, external})
		out, ok := got.OutputByKey(internal.HardwareKey())
		if !ok || !out.Enabled || adjustment.Applied {
			t.Fatal("modeless external suppressed saved laptop", got)
		}
		if !reflect.DeepEqual(saved, before) {
			t.Fatal("saved profile changed")
		}
	}
	external.Width, external.Height, external.DPMSStatus = 2560, 1440, true
	got, adjustment := ApplyClosedLidPolicy(saved, []hypr.Monitor{internal, external})
	out, _ := got.OutputByKey(internal.HardwareKey())
	if out.Enabled || !adjustment.Applied {
		t.Fatal("usable external did not restore clamshell policy")
	}
}

package profile

import (
	"github.com/crmne/hyprmoncfg/internal/hypr"
	"reflect"
	"testing"
)

func TestPowerRefreshOnlyChangesSupportedInternalMode(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "eDP-1", Make: "Laptop", Model: "Panel", Width: 1920, Height: 1080, RefreshRate: 120, Scale: 1, AvailableModes: []string{"1920x1080@144Hz", "1920x1080@59.94Hz", "1280x720@240Hz"}},
		{Name: "DP-1", Make: "External", Model: "Panel", Width: 1920, Height: 1080, RefreshRate: 120, Scale: 1, AvailableModes: []string{"1920x1080@60Hz", "1920x1080@144Hz"}},
	}
	p := FromMonitors("Laptop", monitors)
	before := append([]OutputConfig(nil), p.Outputs...)
	for _, battery := range []bool{true, false} {
		got := WithPowerRefresh(p, monitors, battery)
		want := 144.0
		if battery {
			want = 59.94
		}
		for i, out := range got.Outputs {
			if out.Name == "eDP-1" {
				if out.Refresh != want || out.Width != 1920 || out.Height != 1080 {
					t.Fatalf("battery=%v: %+v", battery, out)
				}
			} else if !reflect.DeepEqual(out, before[i]) {
				t.Fatal("changed external output")
			}
		}
		if !reflect.DeepEqual(p.Outputs, before) {
			t.Fatal("mutated saved profile")
		}
	}
	monitors[0].AvailableModes = []string{"1920x1080@144Hz", "1920x1080@90Hz"}
	got := WithPowerRefresh(p, monitors, true)
	for _, out := range got.Outputs {
		if out.Name == "eDP-1" && out.Refresh != 90 {
			t.Fatal("missing 60Hz must use lowest supported mode")
		}
	}
	for i := range p.Outputs {
		p.Outputs[i].Enabled = false
	}
	if !reflect.DeepEqual(WithPowerRefresh(p, monitors, true), p) {
		t.Fatal("modified intentionally disabled output")
	}
}

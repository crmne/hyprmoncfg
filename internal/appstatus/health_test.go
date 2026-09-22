package appstatus

import (
	"github.com/crmne/hyprmoncfg/internal/hypr"
	"testing"
)

func TestMonitorHealthSeparatesPresenceFromUsability(t *testing.T) {
	for _, tc := range []struct {
		m    hypr.Monitor
		want string
	}{
		{hypr.Monitor{Name: "DP-1", DPMSStatus: true}, "no_signal"},
		{hypr.Monitor{Name: "DP-1", Width: 1920, Height: 1080}, "sleeping"},
		{hypr.Monitor{Name: "DP-1", Disabled: true}, "off"},
		{hypr.Monitor{Name: "FALLBACK", DPMSStatus: true, Width: 1920, Height: 1080}, "synthetic"},
		{hypr.Monitor{Name: "DP-1", DPMSStatus: true, Width: 1920, Height: 1080}, "usable"},
	} {
		if got := monitorHealth(tc.m); got != tc.want {
			t.Fatalf("got %s want %s", got, tc.want)
		}
	}
}

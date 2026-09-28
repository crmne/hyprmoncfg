package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/crmne/hyprmoncfg/internal/appstatus"
)

func TestDisplayStatusDoesNotCountModelessDisplaysAsWorking(t *testing.T) {
	var out bytes.Buffer
	writeDisplayStatus(&out, []appstatus.MonitorSummary{
		{Name: "DP-1", Enabled: true, Width: 2560, Height: 1440},
		{Name: "DP-2", Enabled: true, Width: 2560, Height: 1440},
		{Name: "HDMI-A-1", Enabled: true},
		{Name: "DP-3", Enabled: true},
	})
	for _, want := range []string{
		"Displays: 4 enabled (2 without a usable mode), 4 connected\n",
		"Display HDMI-A-1: no usable mode (0x0)\n",
		"Display DP-3: no usable mode (0x0)\n",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q in:\n%s", want, out.String())
		}
	}
}

func TestDisplayStatusExplainsASteppedDownDisplay(t *testing.T) {
	var out bytes.Buffer
	writeDisplayStatus(&out, []appstatus.MonitorSummary{
		{Name: "DP-1", Enabled: true, Width: 2560, Height: 1600},
		{Name: "DP-2", Enabled: true, Width: 3840, Height: 2160, Fallback: &appstatus.MonitorFallback{Reason: "dropping", Running: "at 120 Hz without VRR"}},
	})
	if !strings.HasPrefix(out.String(), "Displays: 2 enabled, 2 connected\n") {
		t.Fatalf("healthy displays should keep the plain summary:\n%s", out.String())
	}
	want := "Display DP-2: kept disconnecting right after connecting at its saved settings, so it runs at 120 Hz without VRR. The saved profile is unchanged; applying a profile tries its saved settings again.\n"
	if !strings.Contains(out.String(), want) {
		t.Fatalf("missing %q in:\n%s", want, out.String())
	}
}

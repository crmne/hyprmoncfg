package omarchywatch

import (
	"testing"
	"time"
)

func TestParseRequestedScalesKeepsNewestInWindow(t *testing.T) {
	now := time.Date(2026, 8, 19, 17, 3, 0, 0, time.FixedZone("CEST", 2*3600))
	log := "" +
		"at=2026-08-19T17:02:41+02:00\trequested=1.25\tcurrent=2\tnew=1.25\tmonitor=eDP-1\n" +
		"at=2026-08-19T17:02:55+02:00\trequested=1.6\tcurrent=1.25\tnew=1.6\tmonitor=eDP-1\n" +
		"at=2026-08-19T17:02:50+02:00\trequested=1.25\tcurrent=1\tnew=1.25\tmonitor=HDMI-A-1\n"

	got := ParseRequestedScales([]byte(log), now, 15*time.Second)
	if got["eDP-1"] != 1.6 {
		t.Fatalf("eDP-1 scale = %v, want 1.6", got["eDP-1"])
	}
	if got["HDMI-A-1"] != 1.25 {
		t.Fatalf("HDMI-A-1 scale = %v, want 1.25", got["HDMI-A-1"])
	}
}

func TestParseRequestedScalesDropsStaleAndFutureLines(t *testing.T) {
	now := time.Date(2026, 8, 19, 17, 3, 0, 0, time.UTC)
	log := "" +
		"at=2026-08-19T17:02:00Z\tnew=1.25\tmonitor=eDP-1\n" +
		"at=2026-08-19T17:04:00Z\tnew=1.6\tmonitor=eDP-1\n"

	if got := ParseRequestedScales([]byte(log), now, 15*time.Second); got != nil {
		t.Fatalf("expected no recent requests, got %v", got)
	}
}

func TestParseRequestedScalesIgnoresJunk(t *testing.T) {
	now := time.Date(2026, 8, 19, 17, 3, 0, 0, time.UTC)
	log := "not a log line\nat=nope\tnew=1\tmonitor=eDP-1\n\n"

	if got := ParseRequestedScales([]byte(log), now, time.Minute); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

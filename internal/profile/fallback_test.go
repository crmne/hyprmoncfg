package profile

import (
	"testing"

	"github.com/crmne/hyprmoncfg/internal/hypr"
)

func fallbackMonitor() (OutputConfig, hypr.Monitor) {
	monitor := hypr.Monitor{
		Name: "DP-2", Description: "Microstep MPG321UR-QD",
		AvailableModes: []string{
			"3840x2160@60.00Hz", "3840x2160@143.99Hz", "3840x2160@119.88Hz",
			"3840x2160@59.94Hz", "2560x1440@143.97Hz", "1920x1080@60.00Hz",
		},
	}
	out := OutputConfig{
		Name: "DP-2", Description: monitor.Description, Enabled: true,
		Mode: "3840x2160@143.99Hz", Width: 3840, Height: 2160, Refresh: 143.99,
		X: 3820, Y: 927, Scale: 1.33333, VRR: 1,
	}
	return out, monitor
}

func TestFallbackStepsDownOneSettingAtATimeWithoutMovingTheDisplay(t *testing.T) {
	out, monitor := fallbackMonitor()

	want := []struct {
		step FallbackStep
		mode string
		text string
	}{
		{FallbackVRROff, "3840x2160@143.99Hz", "without VRR"},
		{FallbackLowerRefresh, "3840x2160@119.88Hz", "at 120 Hz without VRR"},
		{FallbackSafeRefresh, "3840x2160@60.00Hz", "at 60 Hz without VRR"},
	}
	step := FallbackNone
	for _, expected := range want {
		next, ok := NextFallback(out, monitor, step)
		if !ok || next != expected.step {
			t.Fatalf("after step %d got next %d ok=%v, want %d", step, next, ok, expected.step)
		}
		step = next
		applied := OutputWithFallback(out, monitor, step)
		if applied.VRR != 0 || applied.NormalizedMode() != expected.mode {
			t.Fatalf("step %d applied vrr=%d mode=%s, want vrr=0 mode=%s", step, applied.VRR, applied.NormalizedMode(), expected.mode)
		}
		if applied.X != out.X || applied.Y != out.Y || applied.Scale != out.Scale || applied.Width != out.Width || applied.Height != out.Height {
			t.Fatalf("step %d moved or resized the display: %+v", step, applied)
		}
		if text := DescribeFallback(out, monitor, step); text != expected.text {
			t.Fatalf("step %d described as %q, want %q", step, text, expected.text)
		}
	}
	if _, ok := NextFallback(out, monitor, step); ok {
		t.Fatal("expected no step after the safe refresh rate")
	}
}

func TestFallbackSkipsStepsThatChangeNothing(t *testing.T) {
	out, monitor := fallbackMonitor()
	out.VRR = 0
	if next, ok := NextFallback(out, monitor, FallbackNone); !ok || next != FallbackLowerRefresh {
		t.Fatalf("VRR already off should go straight to a lower refresh, got %d ok=%v", next, ok)
	}
	if text := DescribeFallback(out, monitor, FallbackLowerRefresh); text != "at 120 Hz" {
		t.Fatalf("got %q", text)
	}

	// A 60 Hz-only display has nothing gentler to offer once VRR is off.
	out, monitor = fallbackMonitor()
	out.Mode, out.Refresh = "3840x2160@60.00Hz", 60
	if next, ok := NextFallback(out, monitor, FallbackNone); !ok || next != FallbackVRROff {
		t.Fatalf("got %d ok=%v", next, ok)
	}
	if next, ok := NextFallback(out, monitor, FallbackVRROff); ok {
		t.Fatalf("expected no gentler step for a 60 Hz display, got %d", next)
	}
}

func TestFallbackLeavesDisabledAndMirroredOutputsAlone(t *testing.T) {
	out, monitor := fallbackMonitor()
	out.Enabled = false
	if got := OutputWithFallback(out, monitor, FallbackSafeRefresh); got.VRR != 1 || got.NormalizedMode() != out.NormalizedMode() {
		t.Fatalf("disabled output changed: %+v", got)
	}
	out.Enabled, out.MirrorOf = true, "eDP-1"
	if _, ok := NextFallback(out, monitor, FallbackNone); ok {
		t.Fatal("a mirror follows its source and has no settings of its own to step down")
	}
}

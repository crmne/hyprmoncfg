package tui

import (
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/crmne/hyprmoncfg/internal/appstatus"
	"github.com/crmne/hyprmoncfg/internal/hypr"
)

func dumpView(t *testing.T, name, view string) {
	t.Helper()
	if dir := os.Getenv("HYPRMONCFG_TUI_DUMP"); dir != "" {
		if err := os.WriteFile(dir+"/"+name+".txt", []byte(view), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// An enabled display without a mode shows nothing. It gets its own row naming
// that state, not an invisible rectangle that stretches the canvas.
func TestLayoutNamesAnEnabledDisplayWithoutAMode(t *testing.T) {
	modeless := paneTestSide
	modeless.Width, modeless.Height = 0, 0
	m := paneTestModel(t, tabLayout, []hypr.Monitor{paneTestDesk, modeless}, nil)
	m.selectedOutput = 0
	view := m.View()
	dumpView(t, "modeless", view)
	plain := ansi.Strip(view)
	requireContains(t, plain, "[ "+modeless.Name+"  No usable signal")
	for _, rect := range canvasLayoutFor(m.editOutputs, 80, 24).rects {
		if m.editOutputs[rect.index].Name == modeless.Name {
			t.Fatal("a display without a mode must not be drawn on the canvas")
		}
	}
}

// A display the daemon runs below its saved settings says so on its card.
func TestLayoutCardExplainsASteppedDownDisplay(t *testing.T) {
	m := paneTestModel(t, tabLayout, []hypr.Monitor{paneTestDesk, paneTestSide}, nil)
	m.width, m.height = 136, 38
	m.fallbacks = map[string]appstatus.MonitorFallback{
		paneTestDesk.Name: {Reason: "dropping", Running: "at 120 Hz without VRR"},
	}
	m.selectedOutput = 1
	view := m.View()
	dumpView(t, "fallback", view)
	requireContains(t, ansi.Strip(view), "running at 120 Hz without VRR")
	for _, output := range m.editOutputs {
		if output.Name == paneTestSide.Name {
			if issue, ok := m.canvasOutputIssue(output); ok {
				t.Fatalf("a healthy display must not get a note, got %q", issue)
			}
		}
	}
}

// The daemon's fallback reaches the model through the status probe.
func TestRefreshKeepsDaemonFallbacksUntilTheDaemonAnswersAgain(t *testing.T) {
	m := paneTestModel(t, tabLayout, []hypr.Monitor{paneTestDesk}, nil)
	fallbacks := map[string]appstatus.MonitorFallback{paneTestDesk.Name: {Reason: "dropping", Running: "without VRR"}}
	updated, _ := m.Update(refreshMsg{monitors: m.monitors, daemonOK: true, fallbacks: fallbacks, background: true})
	m = updated.(Model)
	if m.fallbacks[paneTestDesk.Name].Running != "without VRR" {
		t.Fatal("fallback from the daemon was dropped")
	}
	// A busy daemon keeps the last answer; a stopped one clears it.
	updated, _ = m.Update(refreshMsg{monitors: m.monitors, daemonUnknown: true, background: true})
	m = updated.(Model)
	if len(m.fallbacks) != 1 {
		t.Fatal("a busy daemon must not clear the last known fallback")
	}
	updated, _ = m.Update(refreshMsg{monitors: m.monitors, background: true})
	m = updated.(Model)
	if len(m.fallbacks) != 0 {
		t.Fatal("a stopped daemon runs nothing stepped down")
	}
	if !strings.Contains(ansi.Strip(m.View()), paneTestDesk.Name) {
		t.Fatal("view lost the display")
	}
}

// A display without a mode, reported at 0,0, sits in its own row; it must not
// also be reported as overlapping the display that really is at 0,0.
func TestLayoutDoesNotReportADisplayWithoutAModeAsOverlapping(t *testing.T) {
	hdmi := hypr.Monitor{Name: "HDMI-A-1", Description: "Example TV", Make: "Example", Model: "TV", Serial: "T1",
		DPMSStatus: true, AvailableModes: []string{"1920x1080@60.00Hz"}}
	m := paneTestModel(t, tabLayout, []hypr.Monitor{paneTestDesk, hdmi}, nil)
	if m.layoutErr != nil {
		t.Fatalf("unexpected layout error: %v", m.layoutErr)
	}
}

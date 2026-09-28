package daemon

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

func fallbackDesk() (profile.Profile, hypr.Monitor, hypr.Monitor) {
	lg := hypr.Monitor{Name: "DP-1", Description: "LG Electronics 16MR70 311NZSJ039494", Make: "LG Electronics", Model: "16MR70", Serial: "311NZSJ039494",
		Width: 2560, Height: 1600, RefreshRate: 59.97, DPMSStatus: true, AvailableModes: []string{"2560x1600@59.97Hz"}}
	msi := hypr.Monitor{Name: "DP-2", Description: "Microstep MPG321UR-QD", Make: "Microstep", Model: "MPG321UR-QD",
		Width: 3840, Height: 2160, RefreshRate: 143.99, DPMSStatus: true,
		AvailableModes: []string{"3840x2160@60.00Hz", "3840x2160@143.99Hz", "3840x2160@119.88Hz"}}
	p := profile.New("Desktop Work", []profile.OutputConfig{
		{Key: lg.HardwareKey(), Name: lg.Name, Description: lg.Description, Enabled: true, Width: 2560, Height: 1600, Refresh: 59.97, X: 6700, Y: 1458, Scale: 1.6},
		{Key: msi.HardwareKey(), Name: msi.Name, Description: msi.Description, Enabled: true, Width: 3840, Height: 2160, Refresh: 143.99, X: 3820, Y: 927, Scale: 1.33333, VRR: 1},
	})
	return p, lg, msi
}

func msiEvent(kind hypr.EventType) hypr.Event {
	return hypr.Event{Type: kind, Value: "1,DP-2,Microstep MPG321UR-QD"}
}

func msiOutput(t *testing.T, p profile.Profile) profile.OutputConfig {
	t.Helper()
	for _, output := range p.Outputs {
		if output.Description == "Microstep MPG321UR-QD" {
			return output
		}
	}
	t.Fatal("MSI output missing")
	return profile.OutputConfig{}
}

// connectFor simulates the display connecting and dropping after lasted.
func connectFor(f *displayFallbacks, at time.Time, lasted time.Duration) bool {
	f.observeEvent(msiEvent(hypr.EventMonitorAdded), at)
	return f.observeEvent(msiEvent(hypr.EventMonitorRemoved), at.Add(lasted))
}

func TestFallbackIgnoresOrdinaryPowerCyclesAndOneBounce(t *testing.T) {
	f := newDisplayFallbacks("", t.Logf)
	start := time.Now()
	// The MSI bounces once each power-on, and people turn displays off after
	// using them for a while. Neither is a display failing to stay on.
	if connectFor(f, start, 2*time.Second) {
		t.Fatal("one bounce must not step the display down")
	}
	for i := 1; i <= 5; i++ {
		if connectFor(f, start.Add(time.Duration(i)*30*time.Second), 20*time.Second) {
			t.Fatal("connections that lasted must not count as drops")
		}
	}
	// Hyprland's v1 events carry no description and must not double count.
	f.observeEvent(hypr.Event{Type: hypr.EventMonitorAdded, Value: "DP-2"}, start)
	if f.observeEvent(hypr.Event{Type: hypr.EventMonitorRemoved, Value: "DP-2"}, start.Add(time.Second)) {
		t.Fatal("v1 events must be ignored")
	}
	// Short connections spread beyond the window do not add up either.
	f = newDisplayFallbacks("", t.Logf)
	for i := 0; i < 3; i++ {
		if connectFor(f, start.Add(time.Duration(i)*2*fallbackDropWindow), time.Second) {
			t.Fatal("drops minutes apart must not add up")
		}
	}
}

func TestFallbackStepsDownADisplayThatKeepsDroppingAndRemembersIt(t *testing.T) {
	dir := t.TempDir()
	f := newDisplayFallbacks(dir, t.Logf)
	p, lg, msi := fallbackDesk()

	// Seen while connected, so its modes are known once it drops.
	if got := msiOutput(t, f.apply(p, []hypr.Monitor{lg, msi})); got.VRR != 1 {
		t.Fatalf("a healthy display must keep its saved settings, got vrr=%d", got.VRR)
	}
	start := time.Now()
	due := false
	for i := 0; i < fallbackDropThreshold; i++ {
		due = connectFor(f, start.Add(time.Duration(i)*5*time.Second), 1500*time.Millisecond)
	}
	if !due {
		t.Fatal("repeated drops right after connecting should make the display due")
	}

	// The step is taken while the display is gone, so the generated config
	// already holds the gentler rule when it reconnects.
	stepped := msiOutput(t, f.apply(p, []hypr.Monitor{lg}))
	if stepped.VRR != 0 || stepped.NormalizedMode() != msiOutput(t, p).NormalizedMode() {
		t.Fatalf("first step should only turn VRR off, got vrr=%d mode=%s", stepped.VRR, stepped.NormalizedMode())
	}
	if _, err := os.Stat(filepath.Join(dir, fallbackStateFile)); err != nil {
		t.Fatalf("step should be remembered across restarts: %v", err)
	}
	if steps := f.describe([]hypr.Monitor{lg, msi}); steps["DP-2"].Running != "without VRR" || steps["DP-2"].Reason != fallbackReasonDropping {
		t.Fatalf("status should explain the step, got %+v", steps)
	}

	// A restarted daemon keeps the step once it has seen the display again.
	restarted := newDisplayFallbacks(dir, t.Logf)
	if got := msiOutput(t, restarted.apply(p, []hypr.Monitor{lg, msi})); got.VRR != 0 {
		t.Fatalf("restart forgot the step: vrr=%d", got.VRR)
	}

	// Still dropping: the next step lowers the refresh rate at the same size.
	for i := 0; i < fallbackDropThreshold; i++ {
		connectFor(restarted, start.Add(time.Minute+time.Duration(i)*5*time.Second), time.Second)
	}
	if got := msiOutput(t, restarted.apply(p, []hypr.Monitor{lg})); got.NormalizedMode() != "3840x2160@119.88Hz" || got.X != 3820 || got.Scale != 1.33333 {
		t.Fatalf("second step should drop to 120 Hz in place, got %s at %d scale %v", got.NormalizedMode(), got.X, got.Scale)
	}

	// Changing the saved settings means the step no longer applies.
	edited := p
	edited.Outputs = append([]profile.OutputConfig(nil), p.Outputs...)
	for i := range edited.Outputs {
		if edited.Outputs[i].Description == msi.Description {
			edited.Outputs[i].Refresh = 119.88
		}
	}
	if got := msiOutput(t, restarted.apply(edited, []hypr.Monitor{lg, msi})); got.VRR != 1 || got.Refresh != 119.88 {
		t.Fatalf("an edited profile should be applied as saved, got vrr=%d refresh=%v", got.VRR, got.Refresh)
	}
	if _, err := os.Stat(filepath.Join(dir, fallbackStateFile)); !os.IsNotExist(err) {
		t.Fatalf("no step left, so the state file should be gone: %v", err)
	}
}

func TestFallbackResetRestoresSavedSettings(t *testing.T) {
	dir := t.TempDir()
	f := newDisplayFallbacks(dir, t.Logf)
	p, lg, msi := fallbackDesk()
	f.apply(p, []hypr.Monitor{lg, msi})
	for i := 0; i < fallbackDropThreshold; i++ {
		connectFor(f, time.Now().Add(time.Duration(i)*5*time.Second), time.Second)
	}
	f.apply(p, []hypr.Monitor{lg})

	f.reset()
	if got := msiOutput(t, f.apply(p, []hypr.Monitor{lg, msi})); got.VRR != 1 {
		t.Fatalf("applying by hand should try the saved settings again, got vrr=%d", got.VRR)
	}
	if _, err := os.Stat(filepath.Join(dir, fallbackStateFile)); !os.IsNotExist(err) {
		t.Fatalf("reset should remove the remembered steps: %v", err)
	}
}

func TestFallbackStepsDownADisplayThatWakesWithoutAMode(t *testing.T) {
	f := newDisplayFallbacks("", t.Logf)
	p, lg, msi := fallbackDesk()
	f.apply(p, []hypr.Monitor{lg, msi})

	modeless := msi
	modeless.Width, modeless.Height = 0, 0
	f.observeFailedApply(p, []hypr.Monitor{lg, modeless})
	if got := msiOutput(t, f.apply(p, []hypr.Monitor{lg, modeless})); got.VRR != 1 {
		t.Fatal("one modeless apply must not step the display down")
	}
	f.observeFailedApply(p, []hypr.Monitor{lg, modeless})
	if got := msiOutput(t, f.apply(p, []hypr.Monitor{lg, modeless})); got.VRR != 0 {
		t.Fatalf("a display that keeps coming back without a mode should step down, got vrr=%d", got.VRR)
	}

	// A verified apply in between starts the count over.
	f = newDisplayFallbacks("", t.Logf)
	f.apply(p, []hypr.Monitor{lg, msi})
	f.observeFailedApply(p, []hypr.Monitor{lg, modeless})
	f.observeApplied()
	f.observeFailedApply(p, []hypr.Monitor{lg, modeless})
	if got := msiOutput(t, f.apply(p, []hypr.Monitor{lg, modeless})); got.VRR != 1 {
		t.Fatal("strikes separated by a successful apply must not add up")
	}

	// A display asleep by choice is not failing.
	asleep := modeless
	asleep.DPMSStatus = false
	f = newDisplayFallbacks("", t.Logf)
	f.apply(p, []hypr.Monitor{lg, msi})
	f.observeFailedApply(p, []hypr.Monitor{lg, asleep})
	f.observeFailedApply(p, []hypr.Monitor{lg, asleep})
	if got := msiOutput(t, f.apply(p, []hypr.Monitor{lg, asleep})); got.VRR != 1 {
		t.Fatal("a sleeping display must not be stepped down")
	}
}

func TestFallbackLeavesIdenticalDisplaysAlone(t *testing.T) {
	f := newDisplayFallbacks("", t.Logf)
	p, _, msi := fallbackDesk()
	twin := msi
	twin.Name = "DP-3"
	f.apply(p, []hypr.Monitor{msi, twin})
	for i := 0; i < fallbackDropThreshold; i++ {
		connectFor(f, time.Now().Add(time.Duration(i)*5*time.Second), time.Second)
	}
	if got := msiOutput(t, f.apply(p, []hypr.Monitor{msi, twin})); got.VRR != 1 {
		t.Fatal("two displays with one description cannot be told apart and must not be stepped")
	}
}

func TestApplyBestWritesTheSteppedRuleWhileTheDisplayIsDisconnected(t *testing.T) {
	// Only the LG is connected: the MSI just dropped off again.
	lgOnly := `[{"id":1,"name":"DP-1","description":"LG Electronics 16MR70 311NZSJ039494","make":"LG Electronics","model":"16MR70","serial":"311NZSJ039494","width":2560,"height":1600,"refreshRate":59.97,"x":6700,"y":1458,"scale":1.6,"transform":0,"dpmsStatus":true,"disabled":false,"mirrorOf":"","availableModes":["2560x1600@59.97Hz"]}]`
	env := newApplyBestTestEnvWithMonitors(t, lgOnly, lgOnly)
	p, lg, msi := fallbackDesk()
	if err := env.store.Save(p); err != nil {
		t.Fatal(err)
	}
	svc := New(env.client, env.store, Config{MonitorsConf: env.monitorsConfPath, HyprConfig: env.hyprlandConfigPath})
	svc.fallbacks.apply(p, []hypr.Monitor{lg, msi})
	for i := 0; i < fallbackDropThreshold; i++ {
		connectFor(svc.fallbacks, time.Now().Add(time.Duration(i)*5*time.Second), time.Second)
	}

	if err := svc.applyBest(context.Background()); err != nil {
		t.Fatalf("applyBest: %v", err)
	}
	rendered := readMonitorsConf(t, env)
	if !strings.Contains(rendered, "output = desc:Microstep MPG321UR-QD") {
		t.Fatalf("the disconnected MSI has no rule:\n%s", rendered)
	}
	block := rendered[strings.Index(rendered, "output = desc:Microstep MPG321UR-QD"):]
	block = block[:strings.Index(block, "}")]
	for _, want := range []string{"mode = 3840x2160@143.99", "vrr = 0"} {
		if !strings.Contains(block, want) {
			t.Fatalf("the disconnected MSI should reconnect with VRR off; missing %q in:\n%s", want, rendered)
		}
	}
}

func TestStatusReportsASteppedDownDisplay(t *testing.T) {
	both := `[{"id":1,"name":"DP-1","description":"LG Electronics 16MR70 311NZSJ039494","make":"LG Electronics","model":"16MR70","serial":"311NZSJ039494","width":2560,"height":1600,"refreshRate":59.97,"x":6700,"y":1458,"scale":1.6,"transform":0,"dpmsStatus":true,"disabled":false,"mirrorOf":""},` +
		`{"id":2,"name":"DP-2","description":"Microstep MPG321UR-QD","make":"Microstep","model":"MPG321UR-QD","width":3840,"height":2160,"refreshRate":143.99,"x":3820,"y":927,"scale":1.33333,"transform":0,"dpmsStatus":true,"disabled":false,"mirrorOf":""}]`
	env := newApplyBestTestEnvWithMonitors(t, both, both)
	p, lg, msi := fallbackDesk()
	if err := env.store.Save(p); err != nil {
		t.Fatal(err)
	}
	svc := New(env.client, env.store, Config{MonitorsConf: env.monitorsConfPath, HyprConfig: env.hyprlandConfigPath})
	svc.fallbacks.apply(p, []hypr.Monitor{lg, msi})
	for i := 0; i < fallbackDropThreshold; i++ {
		connectFor(svc.fallbacks, time.Now().Add(time.Duration(i)*5*time.Second), time.Second)
	}
	svc.fallbacks.apply(p, []hypr.Monitor{lg})

	document, err := svc.Status()
	if err != nil {
		t.Fatal(err)
	}
	for _, monitor := range document.Monitors {
		switch monitor.Name {
		case "DP-2":
			if monitor.Fallback == nil || monitor.Fallback.Reason != "dropping" || monitor.Fallback.Running != "without VRR" {
				t.Fatalf("status should explain the stepped-down MSI, got %+v", monitor.Fallback)
			}
		case "DP-1":
			if monitor.Fallback != nil {
				t.Fatalf("a healthy display must not report a fallback, got %+v", monitor.Fallback)
			}
		}
	}
}

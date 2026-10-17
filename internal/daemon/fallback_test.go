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

// clock is a settable time for the tracker.
type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }
func trackerAt(dir string, t *testing.T) (*displayFallbacks, *clock) {
	f := newDisplayFallbacks(dir, t.Logf)
	c := &clock{t: time.Date(2026, 9, 28, 4, 0, 0, 0, time.UTC)}
	f.now = c.now
	return f, c
}

func connect(f *displayFallbacks, c *clock) bool {
	return f.observeEvent(msiEvent(hypr.EventMonitorAdded), c.now())
}

func drop(f *displayFallbacks, c *clock) bool {
	return f.observeEvent(msiEvent(hypr.EventMonitorRemoved), c.now())
}

// bounce is what the MSI does at 144 Hz: connect, then drop a second later.
func bounce(f *displayFallbacks, c *clock) bool {
	connect(f, c)
	c.advance(time.Second)
	return drop(f, c)
}

func TestFallbackLeavesDisplaysThatStayOnAlone(t *testing.T) {
	f, c := trackerAt("", t)
	p, lg, msi := fallbackDesk()
	f.apply(p, []hypr.Monitor{lg, msi})
	// Turning a display off after using it, or unplugging and replugging it
	// by hand, is not a display failing to come up.
	for _, lasted := range []time.Duration{20 * time.Second, 5 * time.Second} {
		connect(f, c)
		c.advance(lasted)
		if drop(f, c) {
			t.Fatalf("a %s connection must not mark the display", lasted)
		}
	}
	// Hyprland's v1 events carry no description and are ignored.
	f.observeEvent(hypr.Event{Type: hypr.EventMonitorAdded, Value: "DP-2"}, c.now())
	if f.observeEvent(hypr.Event{Type: hypr.EventMonitorRemoved, Value: "DP-2"}, c.now().Add(time.Second)) {
		t.Fatal("v1 events must be ignored")
	}
	connect(f, c)
	if got := msiOutput(t, f.apply(p, []hypr.Monitor{lg, msi})); got.VRR != 1 || got.Refresh != 143.99 {
		t.Fatalf("a display that stays on keeps its saved settings in one step, got %s vrr=%d", got.NormalizedMode(), got.VRR)
	}
	if _, waiting := f.settleDelay(); waiting {
		t.Fatal("nothing should wait to settle")
	}
}

func TestModelessRecoveryKeepsRetryingSavedSettingsWithGrowingWaits(t *testing.T) {
	dir := t.TempDir()
	f, c := trackerAt(dir, t)
	p, lg, msi := fallbackDesk()
	key := fallbackKey(msi.Description)
	f.active[key] = displayFallback{Step: profile.FallbackSafeRefresh, Reason: fallbackReasonNoMode,
		Saved: "3840x2160@143.99Hz vrr 1", GentleWake: true}
	modeless := msi
	modeless.Width, modeless.Height = 0, 0
	f.apply(p, []hypr.Monitor{lg, modeless})
	c.advance(2 * fallbackSettle)
	if got := msiOutput(t, f.apply(p, []hypr.Monitor{lg, modeless})); got.Refresh != 60 {
		t.Fatal("time spent modeless must not restore the saved mode")
	}
	msi.RefreshRate = 60
	f.apply(p, []hypr.Monitor{lg, msi})
	if wait, ok := f.settleDelay(); !ok || wait != fallbackSettle {
		t.Fatalf("usable recovery must arm a settle timer: %v %v", wait, ok)
	}
	c.advance(fallbackSettle)
	f.settleFired()
	if _, pending := f.settleDelay(); pending {
		t.Fatal("a delivered restoration timer must not re-arm while management is deferred")
	}
	got := msiOutput(t, f.apply(p, []hypr.Monitor{lg, msi}))
	if got.Refresh != 143.99 || got.VRR != 1 {
		t.Fatalf("stable recovery should retry the saved settings: %+v", got)
	}
	// Each retry blanks the display, so a display that cannot hold its
	// saved settings is retried less and less often, but never abandoned.
	for _, delay := range []time.Duration{20 * time.Second, 40 * time.Second, 80 * time.Second,
		160 * time.Second, fallbackRestoreMaxWait, fallbackRestoreMaxWait} {
		if !f.restoreFailed() {
			t.Fatal("rejected saved settings must restore the previous fallback")
		}
		if got := msiOutput(t, f.apply(p, []hypr.Monitor{lg, msi})); got.Refresh != 60 {
			t.Fatal("failed restoration must return to the working fallback")
		}
		if wait, pending := f.settleDelay(); !pending || wait != delay {
			t.Fatalf("next restoration should wait %s, got %s pending=%v", delay, wait, pending)
		}
		c.advance(delay - time.Millisecond)
		if got := msiOutput(t, f.apply(p, []hypr.Monitor{lg, msi})); got.Refresh != 60 {
			t.Fatalf("retried before %s elapsed", delay)
		}
		c.advance(time.Millisecond)
		f.settleFired()
		if got := msiOutput(t, f.apply(p, []hypr.Monitor{lg, msi})); got.Refresh != 143.99 || got.VRR != 1 {
			t.Fatalf("recovery abandoned the saved settings: %+v", got)
		}
	}
	f.observeApplied()
	if f.restoreFailed() {
		t.Fatal("a successful restoration must finish its transaction")
	}
	if _, pending := f.settleDelay(); pending {
		t.Fatal("successful restoration must stop retrying")
	}
	// Once the saved settings have held, the next fallback starts over.
	c.advance(fallbackShortConnection + time.Second)
	f.apply(p, []hypr.Monitor{lg, msi})
	if n := f.restoreFailures[key]; n != 0 {
		t.Fatalf("held saved settings must clear failed retries, got %d", n)
	}
}

func TestRestorationThatDropsTheDisplayCountsAsAFailedRetry(t *testing.T) {
	f, c := trackerAt("", t)
	p, lg, msi := fallbackDesk()
	key := fallbackKey(msi.Description)
	f.active[key] = displayFallback{Step: profile.FallbackSafeRefresh, Reason: fallbackReasonDropping,
		Saved: "3840x2160@143.99Hz vrr 1", GentleWake: true}
	msi.RefreshRate = 60
	f.apply(p, []hypr.Monitor{lg, msi})
	c.advance(fallbackSettle)
	if got := msiOutput(t, f.apply(p, []hypr.Monitor{lg, msi})); got.Refresh != 143.99 {
		t.Fatalf("expected a retry of the saved settings: %+v", got)
	}
	f.observeApplied()
	c.advance(2 * time.Second)
	drop(f, c)
	if f.restoreFailures[key] != 1 {
		t.Fatal("a drop right after a retry must count against the saved settings")
	}
	if wait := f.restoreWait(key); wait != 2*fallbackSettle {
		t.Fatalf("the next retry must wait longer, got %s", wait)
	}
}

func TestModelessRecoverySettleRestartsAfterSleep(t *testing.T) {
	f, c := trackerAt("", t)
	p, lg, msi := fallbackDesk()
	f.active[fallbackKey(msi.Description)] = displayFallback{Step: profile.FallbackSafeRefresh,
		Reason: fallbackReasonNoMode, Saved: "3840x2160@143.99Hz vrr 1"}
	f.apply(p, []hypr.Monitor{lg, msi})
	c.advance(fallbackSettle / 2)
	msi.DPMSStatus = false
	f.apply(p, []hypr.Monitor{lg, msi})
	c.advance(fallbackSettle)
	msi.DPMSStatus = true
	if got := msiOutput(t, f.apply(p, []hypr.Monitor{lg, msi})); got.Refresh != 60 {
		t.Fatal("sleep must interrupt the stable recovery interval")
	}
	if wait, ok := f.settleDelay(); !ok || wait != fallbackSettle {
		t.Fatalf("wake must start a fresh interval: %v %v", wait, ok)
	}
}

func TestRestorationPauseRestartsTheUsableInterval(t *testing.T) {
	f, c := trackerAt("", t)
	p, lg, msi := fallbackDesk()
	f.active[fallbackKey(msi.Description)] = displayFallback{Step: profile.FallbackSafeRefresh,
		Reason: fallbackReasonDropping, Saved: "3840x2160@143.99Hz vrr 1"}
	f.apply(p, []hypr.Monitor{lg, msi})
	f.pauseRestoration()
	c.advance(time.Hour)
	if _, pending := f.settleDelay(); pending {
		t.Fatal("suspended restoration left a timer armed")
	}
	if got := msiOutput(t, f.apply(p, []hypr.Monitor{lg, msi})); got.Refresh != 60 {
		t.Fatal("suspended time counted as stable display time")
	}
	if wait, pending := f.settleDelay(); !pending || wait != fallbackSettle {
		t.Fatalf("resume must start a fresh stable interval, got %s pending=%v", wait, pending)
	}
}

func TestGentleWakeDoesNotPromoteAnAwakeButModelessDisplay(t *testing.T) {
	f, c := trackerAt("", t)
	p, lg, msi := fallbackDesk()
	f.active[fallbackKey(msi.Description)] = displayFallback{GentleWake: true}
	connect(f, c)
	msi.Width, msi.Height = 0, 0
	c.advance(10 * time.Second)
	f.settleFired()
	if got := msiOutput(t, f.apply(p, []hypr.Monitor{lg, msi})); got.Refresh != 60 || got.VRR != 0 {
		t.Fatal("connected duration cannot promote a modeless display")
	}
	if _, pending := f.settleDelay(); pending {
		t.Fatal("a modeless display must use the recovery timer, not spin the settle timer")
	}
}

func TestFallbackWakesABouncingDisplayAt60HzThenSwitchesToItsSavedSettings(t *testing.T) {
	dir := t.TempDir()
	f, c := trackerAt(dir, t)
	p, lg, msi := fallbackDesk()
	f.apply(p, []hypr.Monitor{lg, msi})

	// One bounce at the saved settings is enough.
	if !bounce(f, c) {
		t.Fatal("a drop right after connecting should call for a gentle-wake rule now")
	}
	if _, err := os.Stat(filepath.Join(dir, fallbackStateFile)); err != nil {
		t.Fatalf("the gentle wake should be remembered across restarts: %v", err)
	}
	// Written while it is gone, so it reconnects at 60 Hz.
	gone := msiOutput(t, f.apply(p, []hypr.Monitor{lg}))
	if gone.NormalizedMode() != "3840x2160@60.00Hz" || gone.VRR != 0 || gone.X != 3820 || gone.Scale != 1.33333 {
		t.Fatalf("disconnected rule should wake at 60 Hz in place, got %s vrr=%d at %d scale %v", gone.NormalizedMode(), gone.VRR, gone.X, gone.Scale)
	}

	c.advance(time.Second)
	connect(f, c)
	if got := msiOutput(t, f.apply(p, []hypr.Monitor{lg, msi})); got.NormalizedMode() != "3840x2160@60.00Hz" {
		t.Fatalf("just reconnected, it should stay at 60 Hz, got %s", got.NormalizedMode())
	}
	if wait, ok := f.settleDelay(); !ok || wait != fallbackSettle {
		t.Fatalf("should wait %s to settle, got %s ok=%v", fallbackSettle, wait, ok)
	}
	// A drop while waking gently changes nothing.
	c.advance(time.Second)
	if drop(f, c) {
		t.Fatal("a drop during the gentle wake must not step anything down")
	}
	connect(f, c)

	c.advance(fallbackSettle)
	if wait, ok := f.settleDelay(); !ok || wait != 0 {
		t.Fatalf("should be due now, got %s ok=%v", wait, ok)
	}
	if got := msiOutput(t, f.apply(p, []hypr.Monitor{lg, msi})); got.NormalizedMode() != "3840x2160@143.99Hz" || got.VRR != 1 {
		t.Fatalf("settled, it should get its saved settings, got %s vrr=%d", got.NormalizedMode(), got.VRR)
	}
	if _, waiting := f.settleDelay(); waiting {
		t.Fatal("nothing should wait once switched")
	}
	if steps := f.describe([]hypr.Monitor{lg, msi}); steps != nil && steps["DP-2"].Step != profile.FallbackNone {
		t.Fatalf("a gentle wake that ends at the saved settings is not a fallback, got %+v", steps)
	}

	// Remembered: after a restart the next power-on wakes gently straight away.
	restarted, rc := trackerAt(dir, t)
	restarted.apply(p, []hypr.Monitor{lg, msi})
	if got := msiOutput(t, restarted.apply(p, []hypr.Monitor{lg})); got.NormalizedMode() != "3840x2160@60.00Hz" {
		t.Fatalf("restart forgot the gentle wake, got %s", got.NormalizedMode())
	}
	connect(restarted, rc)
	if got := msiOutput(t, restarted.apply(p, []hypr.Monitor{lg, msi})); got.NormalizedMode() != "3840x2160@60.00Hz" {
		t.Fatalf("after restart it should wake at 60 Hz, got %s", got.NormalizedMode())
	}
}

func TestFallbackStepsDownWhenTheSwitchFromAGentleWakeDrops(t *testing.T) {
	f, c := trackerAt(t.TempDir(), t)
	p, lg, msi := fallbackDesk()
	f.apply(p, []hypr.Monitor{lg, msi})
	bounce(f, c)
	f.apply(p, []hypr.Monitor{lg})

	settleThenSwitch := func() profile.OutputConfig {
		c.advance(time.Second)
		connect(f, c)
		f.apply(p, []hypr.Monitor{lg, msi})
		c.advance(fallbackSettle)
		return msiOutput(t, f.apply(p, []hypr.Monitor{lg, msi}))
	}
	if got := settleThenSwitch(); got.VRR != 1 {
		t.Fatalf("first switch should try the saved settings, got vrr=%d", got.VRR)
	}
	// It drops right after the switch: step down, and still wake gently.
	c.advance(2 * time.Second)
	if !drop(f, c) {
		t.Fatal("a drop right after the switch should call for a gentler setting")
	}
	if got := msiOutput(t, f.apply(p, []hypr.Monitor{lg})); got.NormalizedMode() != "3840x2160@60.00Hz" {
		t.Fatalf("still wakes at 60 Hz, got %s", got.NormalizedMode())
	}
	if got := settleThenSwitch(); got.NormalizedMode() != "3840x2160@143.99Hz" || got.VRR != 0 {
		t.Fatalf("second switch should drop VRR only, got %s vrr=%d", got.NormalizedMode(), got.VRR)
	}
	if steps := f.describe([]hypr.Monitor{lg, msi}); steps["DP-2"].Running != "without VRR" {
		t.Fatalf("status should explain the step, got %+v", steps)
	}
	c.advance(time.Second)
	drop(f, c)
	f.apply(p, []hypr.Monitor{lg})
	if got := settleThenSwitch(); got.NormalizedMode() != "3840x2160@119.88Hz" {
		t.Fatalf("third switch should go to 120 Hz, got %s", got.NormalizedMode())
	}

	// Applying a layout by hand clears the steps but keeps the gentle wake.
	f.reset()
	if got := msiOutput(t, f.apply(p, []hypr.Monitor{lg})); got.NormalizedMode() != "3840x2160@60.00Hz" {
		t.Fatalf("reset must keep the gentle wake, got %s", got.NormalizedMode())
	}
	if got := settleThenSwitch(); got.NormalizedMode() != "3840x2160@143.99Hz" || got.VRR != 1 {
		t.Fatalf("after reset the switch should try the saved settings, got %s vrr=%d", got.NormalizedMode(), got.VRR)
	}
}

func TestFallbackKeepsADisplayThatWasOnAtStartupAtItsSavedSettings(t *testing.T) {
	dir := t.TempDir()
	f, c := trackerAt(dir, t)
	p, lg, msi := fallbackDesk()
	f.apply(p, []hypr.Monitor{lg, msi})
	bounce(f, c)

	// A restart with the display already on must not drop it to 60 Hz.
	restarted, _ := trackerAt(dir, t)
	if got := msiOutput(t, restarted.apply(p, []hypr.Monitor{lg, msi})); got.NormalizedMode() != "3840x2160@143.99Hz" || got.VRR != 1 {
		t.Fatalf("a display already on should keep its saved settings, got %s vrr=%d", got.NormalizedMode(), got.VRR)
	}
}

func TestFallbackStepsDownADisplayThatWakesWithoutAMode(t *testing.T) {
	f, _ := trackerAt("", t)
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
	f, _ = trackerAt("", t)
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
	f, _ = trackerAt("", t)
	f.apply(p, []hypr.Monitor{lg, msi})
	f.observeFailedApply(p, []hypr.Monitor{lg, asleep})
	f.observeFailedApply(p, []hypr.Monitor{lg, asleep})
	if got := msiOutput(t, f.apply(p, []hypr.Monitor{lg, asleep})); got.VRR != 1 {
		t.Fatal("a sleeping display must not be stepped down")
	}
}

func TestFallbackLeavesIdenticalDisplaysAlone(t *testing.T) {
	f, c := trackerAt("", t)
	p, _, msi := fallbackDesk()
	twin := msi
	twin.Name = "DP-3"
	f.apply(p, []hypr.Monitor{msi, twin})
	bounce(f, c)
	if got := msiOutput(t, f.apply(p, []hypr.Monitor{twin})); got.NormalizedMode() != "3840x2160@143.99Hz" || got.VRR != 1 {
		t.Fatal("two displays with one description cannot be told apart and must not be changed")
	}
}

func TestApplyBestWritesTheGentleWakeRuleWhileTheDisplayIsDisconnected(t *testing.T) {
	// Only the LG is connected: the MSI just dropped off again.
	lgOnly := `[{"id":1,"name":"DP-1","description":"LG Electronics 16MR70 311NZSJ039494","make":"LG Electronics","model":"16MR70","serial":"311NZSJ039494","width":2560,"height":1600,"refreshRate":59.97,"x":6700,"y":1458,"scale":1.6,"transform":0,"dpmsStatus":true,"disabled":false,"mirrorOf":"","availableModes":["2560x1600@59.97Hz"]}]`
	env := newApplyBestTestEnvWithMonitors(t, lgOnly, lgOnly)
	p, lg, msi := fallbackDesk()
	if err := env.store.Save(p); err != nil {
		t.Fatal(err)
	}
	svc := New(env.client, env.store, Config{MonitorsConf: env.monitorsConfPath, HyprConfig: env.hyprlandConfigPath})
	svc.fallbacks.apply(p, []hypr.Monitor{lg, msi})
	bounce(svc.fallbacks, &clock{t: time.Now()})

	if err := svc.applyBest(context.Background()); err != nil {
		t.Fatalf("applyBest: %v", err)
	}
	rendered := readMonitorsConf(t, env)
	if !strings.Contains(rendered, "output = desc:Microstep MPG321UR-QD") {
		t.Fatalf("the disconnected MSI has no rule:\n%s", rendered)
	}
	block := rendered[strings.Index(rendered, "output = desc:Microstep MPG321UR-QD"):]
	block = block[:strings.Index(block, "}")]
	for _, want := range []string{"mode = 3840x2160@60.00", "vrr = 0"} {
		if !strings.Contains(block, want) {
			t.Fatalf("the disconnected MSI should reconnect at 60 Hz; missing %q in:\n%s", want, rendered)
		}
	}
}

func TestApplyBestPreparesGentleWakeWithNoDisplaysConnected(t *testing.T) {
	env := newApplyBestTestEnvWithMonitors(t, "[]", "[]")
	p, lg, msi := fallbackDesk()
	svc := New(env.client, env.store, Config{MonitorsConf: env.monitorsConfPath, HyprConfig: env.hyprlandConfigPath})
	svc.lastProfile = p
	svc.fallbacks.apply(p, []hypr.Monitor{lg, msi})
	bounce(svc.fallbacks, &clock{t: time.Now()})
	for range 2 {
		if err := svc.applyBest(context.Background()); err != nil {
			t.Fatal(err)
		}
		rendered := readMonitorsConf(t, env)
		start := strings.Index(rendered, "output = desc:Microstep MPG321UR-QD")
		if start < 0 {
			t.Fatalf("missing MSI wake rule:\n%s", rendered)
		}
		block := rendered[start:]
		block = block[:strings.Index(block, "}")]
		for _, want := range []string{"mode = 3840x2160@60.00", "vrr = 0"} {
			if !strings.Contains(block, want) {
				t.Fatalf("missing %q in prepared wake rule:\n%s", want, block)
			}
		}
		if svc.applied != nil {
			t.Fatal("preparing disconnected rules must not claim successful recovery")
		}
		if svc.lastProfile.Outputs[1].NormalizedMode() != p.Outputs[1].NormalizedMode() {
			t.Fatal("preparing wake rules changed the saved target")
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
	c := &clock{t: time.Now()}
	svc.fallbacks.now = c.now
	svc.fallbacks.apply(p, []hypr.Monitor{lg, msi})
	// A gentle wake, a switch to the saved settings, and a drop right after.
	bounce(svc.fallbacks, c)
	connect(svc.fallbacks, c)
	c.advance(fallbackSettle)
	svc.fallbacks.apply(p, []hypr.Monitor{lg, msi})
	c.advance(time.Second)
	drop(svc.fallbacks, c)
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

// One settle tick per connection: a reconciliation that does not switch the
// display, such as one with monitor management off, must not re-arm it.
func TestFallbackSettleTickFiresOncePerConnection(t *testing.T) {
	f, c := trackerAt(t.TempDir(), t)
	p, lg, msi := fallbackDesk()
	f.apply(p, []hypr.Monitor{lg, msi})
	bounce(f, c)

	c.advance(time.Second)
	connect(f, c)
	c.advance(fallbackSettle)
	if wait, ok := f.settleDelay(); !ok || wait != 0 {
		t.Fatalf("should be due now, got %s ok=%v", wait, ok)
	}
	f.settleFired()
	if wait, ok := f.settleDelay(); ok {
		t.Fatalf("a delivered settle tick must not be reported again, got %s", wait)
	}

	// The next connection gets its own tick.
	c.advance(time.Second)
	drop(f, c)
	connect(f, c)
	if wait, ok := f.settleDelay(); !ok || wait != fallbackSettle {
		t.Fatalf("a new connection should wait %s to settle, got %s ok=%v", fallbackSettle, wait, ok)
	}
}

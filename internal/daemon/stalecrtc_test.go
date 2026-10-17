package daemon

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/crmne/hyprmoncfg/internal/hypr"
)

func TestStaleCRTCRecoverySwitchesOnTheFirstFailureThenBacksOff(t *testing.T) {
	now := time.Unix(1000, 0)
	r := &staleCRTCRecovery{now: func() time.Time { return now }}
	stuck, stale := []string{"DP-1"}, []string{"DP-2"}
	if matched, due := r.observe(stuck, stale); !matched || !due {
		t.Fatalf("first failure: matched=%v due=%v, want a console switch", matched, due)
	}
	for _, wait := range []time.Duration{30 * time.Second, time.Minute, 2 * time.Minute} {
		now = now.Add(wait - time.Second)
		if _, due := r.observe(stuck, stale); due {
			t.Fatalf("switched again before %s", wait)
		}
		now = now.Add(time.Second)
		if _, due := r.observe(stuck, stale); !due {
			t.Fatalf("no switch after %s", wait)
		}
	}
	if matched, _ := r.observe(stuck, nil); matched {
		t.Fatal("matched without a stale connector")
	}
	if _, due := r.observe(stuck, stale); !due {
		t.Fatal("a new episode should switch on its first failure")
	}
}

func TestStaleCRTCStuckNeedsAnAwakeEnabledOutputWithModes(t *testing.T) {
	modes := []string{"2560x1600@59.97Hz"}
	monitors := []hypr.Monitor{
		{Name: "DP-1", DPMSStatus: true, AvailableModes: modes},
		{Name: "DP-2", DPMSStatus: true, Width: 3840, Height: 2160, AvailableModes: modes},
		{Name: "DP-3", DPMSStatus: false, AvailableModes: modes},
		{Name: "DP-4", DPMSStatus: true, Disabled: true, AvailableModes: modes},
		{Name: "DP-5", DPMSStatus: true},
		{Name: "FALLBACK", DPMSStatus: true, AvailableModes: modes},
	}
	if got := staleCRTCStuck(monitors); len(got) != 1 || got[0] != "DP-1" {
		t.Fatalf("stuck = %v, want [DP-1]", got)
	}
}

type fakeConsole struct {
	mu       sync.Mutex
	switches int
	onSwitch func()
}

func (c *fakeConsole) switchAwayAndBack(context.Context, time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.switches++
	if c.onSwitch != nil {
		c.onSwitch()
	}
	return nil
}

func (c *fakeConsole) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.switches
}

func staleCRTCDesk() (working, stuck hypr.Monitor) {
	working = hypr.Monitor{Name: "DP-1", Description: "LG Electronics 16MR70 311NZSJ039494", Make: "LG Electronics", Model: "16MR70",
		Width: 2560, Height: 1600, RefreshRate: 59.972, Scale: 1.6, DPMSStatus: true,
		AvailableModes: []string{"2560x1600@59.97Hz", "1920x1200@59.88Hz"}}
	stuck = working
	stuck.Width, stuck.Height = 0, 0
	return working, stuck
}

// The LG comes back alone and is handed the CRTC the kernel still routes to
// the disconnected MSI: every modeset fails. The daemon switches consoles
// once, reloads, and the next retry applies the layout.
func TestRunSwitchesConsolesToClearAStaleCRTC(t *testing.T) {
	working, stuck := staleCRTCDesk()
	fake := &fakeConsole{}
	env := newRunTestEnvConfigured(t, []hypr.Monitor{working}, func(cfg *Config) {
		cfg.StaleConnectors = func() []string { return []string{"DP-2"} }
		cfg.SwitchConsole = fake.switchAwayAndBack
	}, func(s *Service) {
		s.cfg.RecoveryInterval = 20 * time.Millisecond
		s.cfg.RecoveryMaxInterval = 40 * time.Millisecond
		s.staleCRTC.returnWait = 0
		state := os.Getenv("HYPRCTL_MONITORS")
		if err := writeMonitorState(state, []hypr.Monitor{stuck}); err != nil {
			t.Fatal(err)
		}
		// The console switch resets the kernel state; the reload after it
		// gives the display its mode.
		fake.onSwitch = func() {
			if err := writeMonitorState(state+".next", []hypr.Monitor{working}); err != nil {
				t.Error(err)
			}
		}
	})
	defer env.stop()
	waitFor(t, 15*time.Second, func() bool { return env.logs.contains("applied profile: Home") }, "layout applied after the console switch")
	if got := fake.count(); got != 1 {
		t.Fatalf("switched consoles %d times, want 1:\n%s", got, env.logs.all())
	}
	if !env.logs.contains("aquamarine 0.15.0") {
		t.Fatalf("the log should name the cause:\n%s", env.logs.all())
	}
	if env.logs.contains("running it at") || env.logs.contains("without VRR") {
		t.Fatalf("a stale CRTC must not step the display's settings down:\n%s", env.logs.all())
	}
}

func TestRunLeavesTheConsoleAloneWithoutAStaleConnector(t *testing.T) {
	working, stuck := staleCRTCDesk()
	fake := &fakeConsole{}
	env := newRunTestEnvConfigured(t, []hypr.Monitor{working}, func(cfg *Config) {
		cfg.StaleConnectors = func() []string { return nil }
		cfg.SwitchConsole = fake.switchAwayAndBack
	}, func(s *Service) {
		s.cfg.RecoveryInterval = 20 * time.Millisecond
		s.cfg.RecoveryMaxInterval = 40 * time.Millisecond
		if err := writeMonitorState(os.Getenv("HYPRCTL_MONITORS"), []hypr.Monitor{stuck}); err != nil {
			t.Fatal(err)
		}
	})
	defer env.stop()
	defer func() {
		if t.Failed() {
			t.Logf("logs:\n%s", env.logs.all())
		}
	}()
	// Each failed apply waits out its 3 s verification.
	waitFor(t, 15*time.Second, func() bool { return strings.Count(env.logs.all(), "apply failed") >= 3 }, "several failed applies")
	if got := fake.count(); got != 0 {
		t.Fatalf("switched consoles %d times without a stale connector", got)
	}
}

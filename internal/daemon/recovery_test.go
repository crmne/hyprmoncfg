package daemon

import (
	"strings"
	"testing"
	"time"

	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

func TestRecoveryDefersForPreviewAndResumesWithoutAnEvent(t *testing.T) {
	monitors := []hypr.Monitor{{Name: "DP-1", Make: "Test", Model: "Panel", Width: 1920, Height: 1080, RefreshRate: 60, Scale: 1, DPMSStatus: true}}
	env := newRunTestEnv(t, monitors, func(s *Service) {
		s.cfg.RecoveryInterval = 10 * time.Millisecond
		s.cfg.RecoveryMaxInterval = 20 * time.Millisecond
		s.pending = &pendingTransaction{}
	})
	defer env.stop()
	waitFor(t, time.Second, func() bool { return env.logs.contains("paused during interactive preview") }, "preview deferral")
	time.Sleep(100 * time.Millisecond)
	if reloadCount(env.logPath) != 0 {
		t.Fatal("recovery wrote during preview")
	}
	env.svc.pendingMu.Lock()
	env.svc.pending = nil
	env.svc.pendingMu.Unlock()
	waitFor(t, time.Second, func() bool { return reloadCount(env.logPath) > 0 }, "recovery after preview")
}

func TestRecoveryRetriesWithoutNewMonitorEventsAndStopsOnSuccess(t *testing.T) {
	monitors := []hypr.Monitor{{Name: "DP-1", Make: "Test", Model: "Panel", Width: 1920, Height: 1080, RefreshRate: 60, Scale: 1, DPMSStatus: true}}
	env := newRunTestEnv(t, monitors, func(s *Service) {
		s.cfg.ForcedProfile = "Retry"
		s.cfg.RecoveryInterval = 10 * time.Millisecond
		s.cfg.RecoveryMaxInterval = 20 * time.Millisecond
	})
	defer env.stop()
	waitFor(t, time.Second, func() bool { return strings.Count(env.logs.all(), "apply failed:") >= 3 }, "independent recovery retries")
	if err := env.svc.store.Save(profile.FromMonitors("Retry", monitors)); err != nil {
		t.Fatal(err)
	}
	waitFor(t, time.Second, func() bool { return reloadCount(env.logPath) > 0 }, "retry after profile becomes available without monitor event")
	// Multiple retry intervals without any additional event must remain quiet.
	time.Sleep(150 * time.Millisecond)
	if got := reloadCount(env.logPath); got != 1 {
		t.Fatalf("successful recovery kept reapplying: %d\n%s", got, env.logs.all())
	}
}

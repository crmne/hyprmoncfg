package daemon

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/lid"
)

func dpmsWakeCount(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	return strings.Count(string(data), "dispatch dpms on")
}

// Right after resume, displays still reporting DPMS off are a wake that did
// not take. Repeat it a bounded number of times before calling it sleep.
func TestRunRepeatsAWakeThatDidNotTakeThenRespectsSleep(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "DP-1", Description: "Dell U2720Q", Make: "Dell", Model: "U2720Q", Serial: "B1", Width: 3840, Height: 2160, RefreshRate: 60, Scale: 2, DPMSStatus: true},
	}
	env := newRunTestEnv(t, monitors)
	defer env.stop()
	defer func() {
		if t.Failed() {
			t.Logf("daemon logs:\n%s", env.logs.all())
		}
	}()
	waitFor(t, time.Second, func() bool { return env.logs.contains("applied profile: Home") }, "startup apply")

	env.suspendEvents <- true
	asleep := append([]hypr.Monitor(nil), monitors...)
	asleep[0].DPMSStatus = false
	if err := writeMonitorState(env.monitorStatePath, asleep); err != nil {
		t.Fatal(err)
	}
	env.suspendEvents <- false

	// The fake compositor ignores every wake, as a stuck one would.
	waitFor(t, 2*time.Second, func() bool {
		return strings.Count(env.logs.all(), "displays still asleep after a wake request; waking them again") == wakeRetryLimit
	}, "bounded wake retries")
	waitFor(t, time.Second, func() bool { return env.logs.contains("display sleep detected; pausing automatic switching") }, "sleep respected after retries")
	if got := dpmsWakeCount(env.logPath); got != 1+wakeRetryLimit {
		t.Fatalf("wake dispatches = %d, want %d", got, 1+wakeRetryLimit)
	}
	time.Sleep(3 * env.svc.cfg.WakeSettle)
	if got := dpmsWakeCount(env.logPath); got != 1+wakeRetryLimit {
		t.Fatalf("kept waking displays after giving up: %d dispatches", got)
	}
}

// The clamshell case from #59: the panel was switched off with the lid shut,
// and after opening the lid the external display stays asleep. The saved
// profile is what brings the panel back, so the sleep guard must let it
// through instead of waiting for the external to wake.
func TestRunRestoresTheLaptopPanelWhenTheExternalStaysAsleepAfterLidOpen(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "eDP-1", Description: "Framework Panel", Make: "Framework", Model: "Panel", Serial: "A1", Width: 2880, Height: 1800, RefreshRate: 120, Scale: 1.5, DPMSStatus: true},
		{Name: "DP-1", Description: "Dell U2720Q", Make: "Dell", Model: "U2720Q", Serial: "B1", Width: 3840, Height: 2160, RefreshRate: 60, X: 2880, Scale: 2, DPMSStatus: true},
	}
	env := newRunTestEnv(t, monitors)
	defer env.stop()
	defer func() {
		if t.Failed() {
			t.Logf("daemon logs:\n%s", env.logs.all())
		}
	}()
	waitFor(t, time.Second, func() bool { return env.logs.contains("applied profile: Home") }, "startup apply")

	clamshell := append([]hypr.Monitor(nil), monitors...)
	clamshell[0].Disabled = true
	if err := writeMonitorState(env.monitorStatePath+".next", clamshell); err != nil {
		t.Fatal(err)
	}
	env.setLid(lid.Closed)
	env.lidStates <- lid.Closed
	waitFor(t, time.Second, func() bool { return env.logs.contains("lid closed: forced internal outputs off") }, "clamshell apply")
	waitFor(t, time.Second, func() bool { return reloadCount(env.logPath) == 2 }, "clamshell reload")
	// Let that apply finish verifying before the displays change under it.
	waitFor(t, 2*time.Second, func() bool {
		return strings.Count(env.logs.all(), "automatic reconciliation completed") >= 2
	}, "clamshell reconciliation")

	// The external falls asleep and stays asleep through the lid opening.
	stuck := append([]hypr.Monitor(nil), clamshell...)
	stuck[1].DPMSStatus = false
	if err := writeMonitorState(env.monitorStatePath, stuck); err != nil {
		t.Fatal(err)
	}
	restored := append([]hypr.Monitor(nil), monitors...)
	restored[1].DPMSStatus = false
	if err := writeMonitorState(env.monitorStatePath+".next", restored); err != nil {
		t.Fatal(err)
	}
	env.setLid(lid.Open)
	env.lidStates <- lid.Open

	waitFor(t, 2*time.Second, func() bool { return reloadCount(env.logPath) == 3 }, "panel restored despite the sleeping external")
	if env.logs.contains("automatic switching deferred while displays sleep: lid:open") {
		t.Fatal("the lid-open reconciliation was deferred as display sleep")
	}
}

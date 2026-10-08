package daemon

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/crmne/hyprmoncfg/internal/hypr"
)

func TestLinkFlapsTreatsARelitRemovedOutputAsAFlap(t *testing.T) {
	now := time.Now()
	flaps := newLinkFlaps()
	flaps.Removed("DP-1", now.Add(-2*time.Second))

	relit, ok := flaps.RelitDuringSleep([]hypr.Monitor{
		{Name: "DP-1", DPMSStatus: true},
		{Name: "HDMI-A-1", DPMSStatus: false},
		{Name: "DP-3", DPMSStatus: false},
	}, now)
	if !ok {
		t.Fatal("a sleeping output that dropped its link and came back lit was not a flap")
	}
	if strings.Join(relit, ",") != "DP-1" {
		t.Fatalf("flapped connectors = %v, want [DP-1]", relit)
	}
}

func TestLinkFlapsTreatsAnOutputThatNeverDroppedAsAWake(t *testing.T) {
	now := time.Now()
	flaps := newLinkFlaps()
	flaps.Removed("DP-1", now.Add(-2*time.Second))

	if _, ok := flaps.RelitDuringSleep([]hypr.Monitor{
		{Name: "DP-1", DPMSStatus: true},
		{Name: "HDMI-A-1", DPMSStatus: true},
	}, now); ok {
		t.Fatal("an output lit without dropping its link is a real wake, not a flap")
	}
}

func TestLinkFlapsIgnoresRemovalsOutsideTheFlapWindow(t *testing.T) {
	now := time.Now()
	flaps := newLinkFlaps()
	flaps.Removed("DP-1", now.Add(-displayFlapWindow-time.Second))

	if _, ok := flaps.RelitDuringSleep([]hypr.Monitor{
		{Name: "DP-1", DPMSStatus: true},
	}, now); ok {
		t.Fatal("a relight long after the link drop is a real wake, not a flap")
	}
}

func TestLinkFlapsExplainsOneRelightPerRemoval(t *testing.T) {
	now := time.Now()
	flaps := newLinkFlaps()
	flaps.Removed("DP-1", now.Add(-2*time.Second))

	relit, ok := flaps.RelitDuringSleep([]hypr.Monitor{{Name: "DP-1", DPMSStatus: true}}, now)
	if !ok {
		t.Fatal("first relight should be a flap")
	}
	flaps.Consume(relit)

	if _, ok := flaps.RelitDuringSleep([]hypr.Monitor{{Name: "DP-1", DPMSStatus: true}}, now); ok {
		t.Fatal("consumed flap records must not explain a second relight")
	}
}

func TestLinkFlapsResetClearsRecords(t *testing.T) {
	now := time.Now()
	flaps := newLinkFlaps()
	flaps.Removed("DP-1", now)

	flaps.Reset()

	if _, ok := flaps.RelitDuringSleep([]hypr.Monitor{{Name: "DP-1", DPMSStatus: true}}, now); ok {
		t.Fatal("reset did not clear the flap records")
	}
}

// flapRunMonitors is the desktop that reported the bug: three externals, the
// LG on DP-1 flapping because its input auto-switch scans while the source
// stops sending signal.
func flapRunMonitors() []hypr.Monitor {
	return []hypr.Monitor{
		{Name: "DP-1", Description: "LG Electronics LG ULTRAGEAR 305MANJCD775", Make: "LG Electronics", Model: "LG ULTRAGEAR", Serial: "305MANJCD775", Width: 2560, Height: 1440, RefreshRate: 179.96, X: 2560, Scale: 1, DPMSStatus: true},
		{Name: "HDMI-A-1", Description: "Microstep MSI MP273QP PB6H014400336", Make: "Microstep", Model: "MSI MP273QP", Serial: "PB6H014400336", Width: 2560, Height: 1440, RefreshRate: 75, Scale: 1, DPMSStatus: true},
		{Name: "DP-3", Description: "Microstep MSI MP273QP PB6H014400711", Make: "Microstep", Model: "MSI MP273QP", Serial: "PB6H014400711", Width: 2560, Height: 1440, RefreshRate: 75, X: 5120, Scale: 1, DPMSStatus: true},
	}
}

func flapMonitorsAsleep(monitors []hypr.Monitor) []hypr.Monitor {
	asleep := append([]hypr.Monitor(nil), monitors...)
	for i := range asleep {
		asleep[i].DPMSStatus = false
	}
	return asleep
}

// flapDropDP1 is the monitor set with the LG's link dropped by its input scan.
func flapDropDP1(asleep []hypr.Monitor) []hypr.Monitor {
	return []hypr.Monitor{asleep[1], asleep[2]}
}

// flapRelitDP1 is the monitor set with the LG back and lit while the other
// outputs never left DPMS off.
func flapRelitDP1(asleep []hypr.Monitor) []hypr.Monitor {
	relit := append([]hypr.Monitor(nil), asleep...)
	relit[0].DPMSStatus = true
	return relit
}

// flapSocket2 is a fake Hyprland socket2 for the daemon to subscribe to, in
// its own short-lived runtime dir so the socket path stays within the unix
// socket limit no matter how long the test name is.
type flapSocket2 struct {
	accepted chan net.Conn
}

func startFlapSocket2(t *testing.T) *flapSocket2 {
	t.Helper()
	runtimeDir, err := os.MkdirTemp("", "hmc-flap-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(runtimeDir) })
	socketDir := filepath.Join(runtimeDir, "hypr", "sig-test")
	if err := os.MkdirAll(socketDir, 0o700); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", filepath.Join(socketDir, ".socket2.sock"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	socket := &flapSocket2{accepted: make(chan net.Conn, 1)}
	go func() {
		if conn, err := listener.Accept(); err == nil {
			socket.accepted <- conn
		}
	}()
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	return socket
}

// connect waits for the daemon to subscribe and returns how to inject events.
func (s *flapSocket2) connect(t *testing.T) func(string) {
	t.Helper()
	var conn net.Conn
	select {
	case conn = <-s.accepted:
	case <-time.After(time.Second):
		t.Fatal("daemon did not connect to fake socket2")
	}
	t.Cleanup(func() { _ = conn.Close() })
	return func(line string) {
		t.Helper()
		if _, err := fmt.Fprintln(conn, line); err != nil {
			t.Fatalf("send socket2 event %q: %v", line, err)
		}
	}
}

func TestRunKeepsDisplaysAsleepWhenASleepingOutputFlapsBackOn(t *testing.T) {
	monitors := flapRunMonitors()
	socket := startFlapSocket2(t)
	env := newRunTestEnv(t, monitors)
	defer env.stop()
	sendEvent := socket.connect(t)
	defer func() {
		if t.Failed() {
			t.Logf("daemon logs:\n%s", env.logs.all())
		}
	}()
	waitFor(t, time.Second, func() bool { return env.logs.contains("applied profile: Home") }, "startup apply")

	asleep := flapMonitorsAsleep(monitors)
	if err := writeMonitorState(env.monitorStatePath, asleep); err != nil {
		t.Fatal(err)
	}
	sendEvent("monitoradded>>HDMI-A-1")
	waitFor(t, time.Second, func() bool { return env.logs.contains("display sleep detected; pausing automatic switching") }, "display sleep")

	// The input scan drops the LG's link; while the displays sleep that is
	// noise, not a disconnect to reconcile.
	if err := writeMonitorState(env.monitorStatePath, flapDropDP1(asleep)); err != nil {
		t.Fatal(err)
	}
	sendEvent("monitorremoved>>DP-1")
	waitFor(t, time.Second, func() bool {
		return env.logs.contains("automatic switching deferred while displays sleep: monitorremoved:DP-1")
	}, "removal deferred during sleep")

	// The scan finds nothing and returns to the same input, lighting the
	// output again while the other displays stay asleep.
	if err := writeMonitorState(env.monitorStatePath, flapRelitDP1(asleep)); err != nil {
		t.Fatal(err)
	}
	sendEvent("monitoradded>>DP-1")
	waitFor(t, time.Second, func() bool {
		return env.logs.contains("display link flap during sleep; keeping displays asleep: DP-1")
	}, "flap kept asleep")
	waitFor(t, time.Second, func() bool { return hyprctlLogContains(env.logPath, "dispatch dpms off") }, "sleep re-asserted")

	// The re-assert lands and the episode stays quiet.
	if err := writeMonitorState(env.monitorStatePath, asleep); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * env.svc.cfg.Debounce)
	if got := reloadCount(env.logPath); got != 1 {
		t.Fatalf("flap reconciled a profile: reload count = %d, want 1", got)
	}
	if env.logs.contains("triggered: display-wake") {
		t.Fatalf("flap was reported as a display wake:\n%s", env.logs.all())
	}
}

func TestRunKeepsSuppressingARepeatedLinkFlap(t *testing.T) {
	monitors := flapRunMonitors()
	socket := startFlapSocket2(t)
	env := newRunTestEnv(t, monitors)
	defer env.stop()
	sendEvent := socket.connect(t)
	defer func() {
		if t.Failed() {
			t.Logf("daemon logs:\n%s", env.logs.all())
		}
	}()
	waitFor(t, time.Second, func() bool { return env.logs.contains("applied profile: Home") }, "startup apply")

	asleep := flapMonitorsAsleep(monitors)
	if err := writeMonitorState(env.monitorStatePath, asleep); err != nil {
		t.Fatal(err)
	}
	sendEvent("monitoradded>>HDMI-A-1")
	waitFor(t, time.Second, func() bool { return env.logs.contains("display sleep detected; pausing automatic switching") }, "display sleep")

	// A monitor that scans again every time it loses signal must not win by
	// repetition: every relight is put back to sleep.
	for i := 0; i < 4; i++ {
		if err := writeMonitorState(env.monitorStatePath, flapDropDP1(asleep)); err != nil {
			t.Fatal(err)
		}
		sendEvent("monitorremoved>>DP-1")
		if err := writeMonitorState(env.monitorStatePath, flapRelitDP1(asleep)); err != nil {
			t.Fatal(err)
		}
		sendEvent("monitoradded>>DP-1")
		waitFor(t, time.Second, func() bool {
			return strings.Count(env.logs.all(), "display link flap during sleep; keeping displays asleep: DP-1") == i+1
		}, "flap suppressed")
		if err := writeMonitorState(env.monitorStatePath, asleep); err != nil {
			t.Fatal(err)
		}
	}

	time.Sleep(5 * env.svc.cfg.Debounce)
	if got := reloadCount(env.logPath); got != 1 {
		t.Fatalf("repeated flaps reconciled a profile: reload count = %d, want 1", got)
	}
	if env.logs.contains("triggered: display-wake") {
		t.Fatalf("repeated flaps were reported as a display wake:\n%s", env.logs.all())
	}
}

func TestRunWakesForANewOutputDuringDisplaySleep(t *testing.T) {
	monitors := flapRunMonitors()
	socket := startFlapSocket2(t)
	env := newRunTestEnv(t, monitors)
	defer env.stop()
	sendEvent := socket.connect(t)
	defer func() {
		if t.Failed() {
			t.Logf("daemon logs:\n%s", env.logs.all())
		}
	}()
	waitFor(t, time.Second, func() bool { return env.logs.contains("applied profile: Home") }, "startup apply")

	asleep := flapMonitorsAsleep(monitors)
	if err := writeMonitorState(env.monitorStatePath, asleep); err != nil {
		t.Fatal(err)
	}
	sendEvent("monitoradded>>HDMI-A-1")
	waitFor(t, time.Second, func() bool { return env.logs.contains("display sleep detected; pausing automatic switching") }, "display sleep")

	// A display that appears without having dropped its link is a real
	// hotplug, not a flap: it lights the screens and gets a layout.
	extended := append(append([]hypr.Monitor(nil), asleep...), hypr.Monitor{
		Name: "HDMI-A-2", Description: "Some Projector PJ1000 X9", Make: "Some", Model: "PJ1000", Serial: "X9",
		Width: 1920, Height: 1080, RefreshRate: 60, X: 7680, Scale: 1, DPMSStatus: true,
	})
	if err := writeMonitorState(env.monitorStatePath, extended); err != nil {
		t.Fatal(err)
	}
	sendEvent("monitoradded>>0,HDMI-A-2,Some Projector PJ1000 X9")
	waitFor(t, time.Second, func() bool { return env.logs.contains("display wake detected; waiting") }, "new output wakes displays")
	waitFor(t, time.Second, func() bool { return reloadCount(env.logPath) >= 2 }, "new output reconciled")
	if env.logs.contains("display link flap during sleep") {
		t.Fatalf("a real hotplug was treated as a link flap:\n%s", env.logs.all())
	}
}

func TestRunAcceptsAGenuineWakeAfterSuppressingAFlap(t *testing.T) {
	monitors := flapRunMonitors()
	socket := startFlapSocket2(t)
	env := newRunTestEnv(t, monitors)
	defer env.stop()
	sendEvent := socket.connect(t)
	defer func() {
		if t.Failed() {
			t.Logf("daemon logs:\n%s", env.logs.all())
		}
	}()
	waitFor(t, time.Second, func() bool { return env.logs.contains("applied profile: Home") }, "startup apply")

	asleep := flapMonitorsAsleep(monitors)
	if err := writeMonitorState(env.monitorStatePath, asleep); err != nil {
		t.Fatal(err)
	}
	sendEvent("monitoradded>>HDMI-A-1")
	waitFor(t, time.Second, func() bool { return env.logs.contains("display sleep detected; pausing automatic switching") }, "display sleep")

	if err := writeMonitorState(env.monitorStatePath, flapDropDP1(asleep)); err != nil {
		t.Fatal(err)
	}
	sendEvent("monitorremoved>>DP-1")
	if err := writeMonitorState(env.monitorStatePath, flapRelitDP1(asleep)); err != nil {
		t.Fatal(err)
	}
	sendEvent("monitoradded>>DP-1")
	waitFor(t, time.Second, func() bool {
		return env.logs.contains("display link flap during sleep; keeping displays asleep: DP-1")
	}, "flap kept asleep")
	if err := writeMonitorState(env.monitorStatePath, asleep); err != nil {
		t.Fatal(err)
	}

	// The person comes back: every output lights at once, and that wake wins
	// over anything the flap bookkeeping remembers.
	if err := writeMonitorState(env.monitorStatePath, monitors); err != nil {
		t.Fatal(err)
	}
	sendEvent("monitoradded>>HDMI-A-1")
	waitFor(t, time.Second, func() bool { return env.logs.contains("display wake detected; waiting") }, "genuine wake accepted")
	waitFor(t, time.Second, func() bool {
		return strings.Count(env.logs.all(), "automatic reconciliation completed") >= 2
	}, "genuine wake reconciled")
}

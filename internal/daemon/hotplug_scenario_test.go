package daemon

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/crmne/hyprmoncfg/internal/hypr"
)

// listenMonitorEvents serves a fake Hyprland event socket and returns a
// function that sends one event line to the daemon once it connects.
func listenMonitorEvents(t *testing.T) func() func(string) {
	t.Helper()
	runtimeDir, err := os.MkdirTemp("", "hmc-hotplug-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(runtimeDir) })
	socketDir := filepath.Join(runtimeDir, "hypr", "sig-test")
	if err := os.MkdirAll(socketDir, 0o700); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", filepath.Join(socketDir, ".socket2.sock"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	accepted := make(chan net.Conn, 1)
	go func() {
		if conn, err := listener.Accept(); err == nil {
			accepted <- conn
		}
	}()
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	return func() func(string) {
		var conn net.Conn
		select {
		case conn = <-accepted:
		case <-time.After(time.Second):
			t.Fatal("event listener not connected")
		}
		t.Cleanup(func() { conn.Close() })
		return func(event string) {
			t.Helper()
			if _, err := conn.Write([]byte(event + "\n")); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// Both displays are turned off, then only the MSI is turned back on. It must
// reconnect straight into its 60 Hz wake rule, through the bounce that
// follows a second later, and the saved 144 Hz comes only once it has
// settled.
func TestRunLightsTheMSIAloneAfterEveryDisplayWasTurnedOff(t *testing.T) {
	connected := listenMonitorEvents(t)
	_, lg, msi := fallbackDesk()
	lg.X, lg.Y, lg.Scale = 6700, 1458, 1.6
	msi.X, msi.Y, msi.Scale = 3820, 927, 1.33333
	both := []hypr.Monitor{lg, msi}
	env := newRunTestEnv(t, both, func(svc *Service) {
		svc.fallbacks.settle = 3 * time.Second
		svc.fallbacks.active[fallbackKey(msi.Description)] = displayFallback{GentleWake: true}
	})
	defer env.stop()
	defer func() {
		if t.Failed() {
			t.Logf("daemon logs:\n%s", env.logs.all())
		}
	}()
	send := connected()
	waitFor(t, 2*time.Second, func() bool { return env.logs.contains("applied profile: Home") }, "startup apply")
	if !strings.Contains(msiBlock(t, env.monitorsConfPath), "mode = 3840x2160@143.99") {
		t.Fatalf("an awake MSI should run its saved mode:\n%s", msiBlock(t, env.monitorsConfPath))
	}

	// Turn off the LG, then the MSI: the last display is gone.
	if err := writeMonitorState(env.monitorStatePath, []hypr.Monitor{msi}); err != nil {
		t.Fatal(err)
	}
	send("monitorremovedv2>>1,DP-1,LG Electronics 16MR70 311NZSJ039494")
	time.Sleep(200 * time.Millisecond)
	if err := writeMonitorState(env.monitorStatePath, nil); err != nil {
		t.Fatal(err)
	}
	send("monitorremovedv2>>2,DP-2,Microstep MPG321UR-QD")
	waitFor(t, 3*time.Second, func() bool {
		block := msiBlock(t, env.monitorsConfPath)
		return strings.Contains(block, "mode = 3840x2160@60.00") && strings.Contains(block, "vrr = 0")
	}, "60 Hz wake rule prepared with no display connected")

	// The MSI powers on alone, into the prepared rule, and bounces once.
	woken := msi
	woken.RefreshRate = 60
	reloads := reloadCount(env.logPath)
	if err := writeMonitorState(env.monitorStatePath, []hypr.Monitor{woken}); err != nil {
		t.Fatal(err)
	}
	send("monitoraddedv2>>2,DP-2,Microstep MPG321UR-QD")
	time.Sleep(1500 * time.Millisecond)
	if err := writeMonitorState(env.monitorStatePath, nil); err != nil {
		t.Fatal(err)
	}
	send("monitorremovedv2>>2,DP-2,Microstep MPG321UR-QD")
	time.Sleep(300 * time.Millisecond)
	if err := writeMonitorState(env.monitorStatePath, []hypr.Monitor{woken}); err != nil {
		t.Fatal(err)
	}
	send("monitoraddedv2>>2,DP-2,Microstep MPG321UR-QD")
	block := msiBlock(t, env.monitorsConfPath)
	if !strings.Contains(block, "mode = 3840x2160@60.00") {
		t.Fatalf("the MSI must reconnect on its wake rule:\n%s", block)
	}

	waitFor(t, 3*time.Second, func() bool { return reloadCount(env.logPath) > reloads }, "reconcile after the reconnect")
	// Once settled it gets its saved settings, without another event.
	if err := writeMonitorState(env.monitorStatePath+".next", []hypr.Monitor{msi}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 8*time.Second, func() bool {
		return strings.Contains(msiBlock(t, env.monitorsConfPath), "mode = 3840x2160@143.99")
	}, "saved mode after the MSI settled")
	if env.logs.contains("apply failed") {
		t.Fatalf("the reconnect should not need recovery:\n%s", env.logs.all())
	}
}

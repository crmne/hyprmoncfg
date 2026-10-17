package daemon

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/crmne/hyprmoncfg/internal/config"
	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

func TestRunRetriesRejectedRestorationAfterUsableRollback(t *testing.T) {
	monitor := hypr.Monitor{Name: "DP-2", Description: "Test Display", Make: "Test", Model: "Display",
		Width: 3840, Height: 2160, RefreshRate: 60, Scale: 1, DPMSStatus: true,
		AvailableModes: []string{"3840x2160@60Hz", "3840x2160@144Hz"}}
	env := newRunTestEnv(t, []hypr.Monitor{monitor}, func(s *Service) {
		s.fallbacks.settle = 50 * time.Millisecond
		s.cfg.RecoveryInterval = 20 * time.Millisecond
		s.cfg.RecoveryMaxInterval = 40 * time.Millisecond
		p := profile.FromMonitors("Home", []hypr.Monitor{monitor})
		p.Outputs[0].Mode, p.Outputs[0].Refresh = "3840x2160@144Hz", 144
		if err := s.store.Save(p); err != nil {
			t.Fatal(err)
		}
		s.fallbacks.active[fallbackKey(monitor.Description)] = displayFallback{
			Step: profile.FallbackSafeRefresh, Reason: fallbackReasonDropping, Saved: "3840x2160@144Hz vrr 0"}
	})
	defer env.stop()
	waitFor(t, 10*time.Second, func() bool {
		return strings.Count(env.logs.all(), "retrying its saved settings") >= 2
	}, "second saved-settings attempt without another monitor event")
	if env.logs.contains("automatic retry paused") {
		t.Fatalf("restoration entered the permanent rejection pause:\n%s", env.logs.all())
	}
	// Let a later attempt succeed. Once restored, the retry timer stops.
	monitor.RefreshRate = 144
	if err := writeMonitorState(env.monitorStatePath+".next", []hypr.Monitor{monitor}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 10*time.Second, func() bool {
		data, err := os.ReadFile(env.monitorStatePath)
		return err == nil && strings.Contains(string(data), `"refreshRate":144`) && env.logs.contains("applied profile: Home")
	}, "saved mode eventually applies")
	time.Sleep(300 * time.Millisecond)
	count := reloadCount(env.logPath)
	time.Sleep(300 * time.Millisecond)
	if reloadCount(env.logPath) != count {
		t.Fatalf("restored displays kept reloading:\n%s", env.logs.all())
	}
}

func msiBlock(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	rendered := string(content)
	start := strings.Index(rendered, "output = desc:Microstep MPG321UR-QD")
	if start < 0 {
		return ""
	}
	block := rendered[start:]
	if end := strings.Index(block, "}"); end >= 0 {
		block = block[:end]
	}
	return block
}

// The whole path through the event loop: the MSI bounces once at 144 Hz, the
// daemon writes a 60 Hz wake rule while it is gone, and once it has stayed
// connected the daemon switches it to its saved 144 Hz without another event.
func TestRunWakesABouncingDisplayGentlyThenRestoresItsSavedMode(t *testing.T) {
	runtimeDir, err := os.MkdirTemp("", "hmc-gentle-wake-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(runtimeDir)
	socketDir := filepath.Join(runtimeDir, "hypr", "sig-test")
	if err := os.MkdirAll(socketDir, 0o700); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", filepath.Join(socketDir, ".socket2.sock"))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		if conn, err := listener.Accept(); err == nil {
			accepted <- conn
		}
	}()
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)

	lg := hypr.Monitor{Name: "DP-1", Description: "LG Electronics 16MR70 311NZSJ039494", Make: "LG Electronics", Model: "16MR70", Serial: "311NZSJ039494",
		Width: 2560, Height: 1600, RefreshRate: 59.97, X: 6700, Y: 1458, Scale: 1.6, DPMSStatus: true, AvailableModes: []string{"2560x1600@59.97Hz"}}
	msi := hypr.Monitor{Name: "DP-2", Description: "Microstep MPG321UR-QD", Make: "Microstep", Model: "MPG321UR-QD",
		Width: 3840, Height: 2160, RefreshRate: 143.99, X: 3820, Y: 927, Scale: 1.33333, DPMSStatus: true,
		AvailableModes: []string{"3840x2160@60.00Hz", "3840x2160@143.99Hz", "3840x2160@119.88Hz"}}
	both := []hypr.Monitor{lg, msi}
	env := newRunTestEnv(t, both, func(svc *Service) { svc.fallbacks.settle = 3 * time.Second })
	defer env.stop()
	defer func() {
		if t.Failed() {
			t.Logf("daemon logs:\n%s", env.logs.all())
		}
	}()
	var conn net.Conn
	select {
	case conn = <-accepted:
	case <-time.After(time.Second):
		t.Fatal("event listener not connected")
	}
	defer conn.Close()
	waitFor(t, time.Second, func() bool { return env.logs.contains("applied profile: Home") }, "startup apply")
	send := func(event string) {
		if _, err := conn.Write([]byte(event + "\n")); err != nil {
			t.Fatal(err)
		}
	}

	// Power on at 144 Hz, then the bounce a moment later.
	send("monitoraddedv2>>2,DP-2,Microstep MPG321UR-QD")
	time.Sleep(100 * time.Millisecond)
	if err := writeMonitorState(env.monitorStatePath, []hypr.Monitor{lg}); err != nil {
		t.Fatal(err)
	}
	send("monitorremovedv2>>2,DP-2,Microstep MPG321UR-QD")
	waitFor(t, 2*time.Second, func() bool { return strings.Contains(msiBlock(t, env.monitorsConfPath), "mode = 3840x2160@60.00") }, "60 Hz wake rule written while disconnected")

	// It comes back at 60 Hz, as that rule says; the switch will bring 144 Hz.
	woken := msi
	woken.RefreshRate = 60
	if err := writeMonitorState(env.monitorStatePath, []hypr.Monitor{lg, woken}); err != nil {
		t.Fatal(err)
	}
	// Let the disconnected-rule apply finish logging before counting.
	waitFor(t, time.Second, func() bool {
		return strings.HasSuffix(strings.TrimSpace(env.logs.all()), "error=<nil>")
	}, "disconnected-rule apply finished")
	settled := strings.Count(env.logs.all(), "automatic reconciliation completed")
	send("monitoraddedv2>>2,DP-2,Microstep MPG321UR-QD")
	// Only the switch after settling reloads it into 144 Hz.
	waitFor(t, time.Second, func() bool {
		return strings.Count(env.logs.all(), "automatic reconciliation completed") > settled
	}, "reconnect reconciliation at 60 Hz")
	if env.logs.contains("triggered: display-settled") {
		t.Fatal("switched before the display settled")
	}
	if err := writeMonitorState(env.monitorStatePath+".next", both); err != nil {
		t.Fatal(err)
	}

	waitFor(t, 8*time.Second, func() bool { return strings.Contains(msiBlock(t, env.monitorsConfPath), "mode = 3840x2160@143.99") }, "saved mode restored after settling")
	if !env.logs.contains("triggered: display-settled") {
		t.Fatal("the switch should come from the settle timer, not another event")
	}
}

// With monitor management off, reconciling never switches a gently woken
// display, so the settle timer must not re-arm after every reconciliation
// (crmne/omarchy-hyprmoncfg#31: thousands of display-settled triggers a minute).
func TestRunSettlesAGentleWakeOnceWhenUnmanaged(t *testing.T) {
	runtimeDir, err := os.MkdirTemp("", "hmc-settle-once-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(runtimeDir)
	socketDir := filepath.Join(runtimeDir, "hypr", "sig-test")
	if err := os.MkdirAll(socketDir, 0o700); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", filepath.Join(socketDir, ".socket2.sock"))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		if conn, err := listener.Accept(); err == nil {
			accepted <- conn
		}
	}()
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)

	msi := hypr.Monitor{Name: "DP-2", Description: "Microstep MPG321UR-QD", Make: "Microstep", Model: "MPG321UR-QD",
		Width: 3840, Height: 2160, RefreshRate: 60, Scale: 1.33333, DPMSStatus: true,
		AvailableModes: []string{"3840x2160@60.00Hz", "3840x2160@143.99Hz"}}
	configDir := t.TempDir()
	if err := config.SetManaged(configDir, false); err != nil {
		t.Fatal(err)
	}
	env := newRunTestEnvConfigured(t, []hypr.Monitor{msi},
		func(cfg *Config) { cfg.ConfigDir = configDir },
		func(svc *Service) {
			svc.fallbacks.settle = 100 * time.Millisecond
			svc.fallbacks.active[fallbackKey(msi.Description)] = displayFallback{GentleWake: true}
		})
	defer env.stop()
	defer func() {
		if t.Failed() {
			t.Logf("daemon logs:\n%s", env.logs.all())
		}
	}()
	var conn net.Conn
	select {
	case conn = <-accepted:
	case <-time.After(time.Second):
		t.Fatal("event listener not connected")
	}
	defer conn.Close()

	if _, err := conn.Write([]byte("monitoraddedv2>>2,DP-2,Microstep MPG321UR-QD\n")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 2*time.Second, func() bool { return env.logs.contains("triggered: display-settled") }, "settle tick")
	time.Sleep(500 * time.Millisecond)
	if n := strings.Count(env.logs.all(), "triggered: display-settled"); n != 1 {
		t.Fatalf("settle tick should fire once per connection, fired %d times", n)
	}
}

package daemon

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/lid"
)

func TestRunWaitsForNewestProbeBeforeDebouncedApply(t *testing.T) {
	for _, tc := range []struct {
		name       string
		lidTrigger bool
		timeout    bool
	}{
		{name: "old-debounce"},
		{name: "current-generation-lid-trigger", lidTrigger: true},
		{name: "timeout-then-retry", lidTrigger: true, timeout: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runtimeDir, err := os.MkdirTemp("", "hmc-generation-")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(runtimeDir)
			socketDir := filepath.Join(runtimeDir, "hypr", "sig-test")
			if err := os.MkdirAll(socketDir, 0700); err != nil {
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
			monitors := []hypr.Monitor{{Name: "eDP-1", Make: "Example", Model: "Laptop", Serial: "one", Width: 1920, Height: 1080, RefreshRate: 60, Scale: 1, DPMSStatus: true}}
			env := newRunTestEnvConfigured(t, monitors, func(cfg *Config) {
				cfg.Debounce = 100 * time.Millisecond
				if tc.timeout {
					cfg.QueryTimeout = 300 * time.Millisecond
				}
			})
			defer env.stop()
			waitFor(t, 2*time.Second, func() bool { return env.logs.contains("applied profile: Home") }, "startup apply")
			var conn net.Conn
			select {
			case conn = <-accepted:
			case <-time.After(time.Second):
				t.Fatal("event listener not connected")
			}
			defer conn.Close()
			if _, err := conn.Write([]byte("monitoradded>>DP-1\n")); err != nil {
				t.Fatal(err)
			}
			waitFor(t, time.Second, func() bool { return env.logs.contains("triggered: monitoradded:DP-1") }, "old debounce armed")
			baseline := strings.Count(env.logs.all(), "automatic reconciliation completed")
			helper := filepath.Join(filepath.Dir(env.logPath), "hyprctl")
			source, err := os.ReadFile(helper)
			if err != nil {
				t.Fatal(err)
			}
			// Block only the probe's read, leaving any erroneous concurrent
			// reconciliation free to complete and expose the stale-timer bug.
			replacement := strings.Replace(string(source), `  cat "$HYPRCTL_MONITORS"`, `  if mkdir "$HYPRCTL_LOG.probe-blocked" 2>/dev/null; then
    printf 'probe-blocked\n' >> "$HYPRCTL_LOG"
    sleep 0.45
  fi
  cat "$HYPRCTL_MONITORS"`, 1)
			if err := os.WriteFile(helper+".next", []byte(replacement), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(helper+".next", helper); err != nil {
				t.Fatal(err)
			}
			if _, err := conn.Write([]byte("monitoradded>>DP-2\n")); err != nil {
				t.Fatal(err)
			}
			waitFor(t, time.Second, func() bool { return hyprctlLogContains(env.logPath, "probe-blocked") }, "latest probe blocked")
			if tc.lidTrigger {
				env.setLid(lid.Closed)
				env.lidStates <- lid.Closed
				waitFor(t, time.Second, func() bool { return env.logs.contains("deferred trigger while monitor probe pending: lid:closed") }, "current-generation lid trigger")
			}
			time.Sleep(200 * time.Millisecond)
			if got := strings.Count(env.logs.all(), "automatic reconciliation completed"); got != baseline {
				t.Fatalf("reconciled before the latest topology probe completed: before=%d after=%d\n%s", baseline, got, env.logs.all())
			}
			waitFor(t, 2*time.Second, func() bool {
				return strings.Count(env.logs.all(), "automatic reconciliation completed") == baseline+1
			}, "fresh debounce after latest probe succeeds")
			if tc.timeout && !env.logs.contains("monitor probe failed:") {
				t.Fatal("fixture did not exercise the timed-out latest probe")
			}
		})
	}
}

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

func TestRunLidWakeDiscardsPreWakeProbe(t *testing.T) {
	for _, hotplug := range []bool{false, true} {
		name := "poll"
		if hotplug {
			name = "pending-hotplug"
		}
		t.Run(name, func(t *testing.T) {
			runtimeDir, err := os.MkdirTemp("", "hmc-wake-probe-")
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
				if !hotplug {
					cfg.PollInterval = time.Second
				}
			})
			defer env.stop()
			var conn net.Conn
			select {
			case conn = <-accepted:
			case <-time.After(time.Second):
				t.Fatal("event listener not connected")
			}
			defer conn.Close()
			waitFor(t, time.Second, func() bool { return env.logs.contains("applied profile: Home") }, "startup apply")
			env.setLid(lid.Closed)
			env.lidStates <- lid.Closed
			waitFor(t, time.Second, func() bool { return strings.Count(env.logs.all(), "automatic reconciliation completed") >= 2 }, "lid close reconciliation")
			helper := filepath.Join(filepath.Dir(env.logPath), "hyprctl")
			source, err := os.ReadFile(helper)
			if err != nil {
				t.Fatal(err)
			}
			// Hold an already-captured DPMS-off snapshot until the explicit
			// lid-open wake has run. Only the first probe is delayed.
			replacement := strings.Replace(string(source), `  cat "$HYPRCTL_MONITORS"`, `  if mkdir "$HYPRCTL_LOG.old-probe" 2>/dev/null; then
    snapshot=$(cat "$HYPRCTL_MONITORS")
    printf 'old-probe-captured\n' >> "$HYPRCTL_LOG"
    while [[ ! -f "$HYPRCTL_LOG.release-probe" ]]; do sleep 0.01; done
    printf '%s' "$snapshot"
  else
    cat "$HYPRCTL_MONITORS"
  fi`, 1)
			if err := os.WriteFile(helper+".next", []byte(replacement), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(helper+".next", helper); err != nil {
				t.Fatal(err)
			}
			monitors[0].DPMSStatus = false
			if err := writeMonitorState(env.monitorStatePath, monitors); err != nil {
				t.Fatal(err)
			}
			if hotplug {
				if _, err := conn.Write([]byte("monitoradded>>DP-1\n")); err != nil {
					t.Fatal(err)
				}
			}
			waitFor(t, 2*time.Second, func() bool { return hyprctlLogContains(env.logPath, "old-probe-captured") }, "pre-wake snapshot captured")
			baseline := strings.Count(env.logs.all(), "automatic reconciliation completed")
			monitors[0].DPMSStatus = true
			if err := writeMonitorState(env.monitorStatePath, monitors); err != nil {
				t.Fatal(err)
			}
			env.setLid(lid.Open)
			env.lidStates <- lid.Open
			waitFor(t, time.Second, func() bool { return hyprctlLogContains(env.logPath, "dispatch dpms on") }, "explicit lid wake")
			if err := os.WriteFile(env.logPath+".release-probe", nil, 0600); err != nil {
				t.Fatal(err)
			}
			// Neither path may need the next periodic poll to recover. A
			// pending hotplug must get a replacement probe in the new generation.
			waitFor(t, 600*time.Millisecond, func() bool { return strings.Count(env.logs.all(), "automatic reconciliation completed") == baseline+1 }, "post-wake reconciliation")
			if env.logs.contains("display sleep detected") {
				t.Fatalf("pre-wake snapshot re-entered display sleep:\n%s", env.logs.all())
			}
		})
	}
}

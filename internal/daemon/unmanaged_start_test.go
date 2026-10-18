package daemon

import (
	"os"
	"testing"
	"time"

	"github.com/crmne/hyprmoncfg/internal/config"
	"github.com/crmne/hyprmoncfg/internal/hypr"
)

func TestRunUnmanagedDoesNotRestoreGeneratedInclude(t *testing.T) {
	const root = "# User-owned monitor configuration\nmonitor = ,preferred,auto,1\n"
	monitors := []hypr.Monitor{{Name: "eDP-1", Width: 1920, Height: 1080, RefreshRate: 60, Scale: 1, DPMSStatus: true}}
	configDir := t.TempDir()
	if err := config.SetManaged(configDir, false); err != nil {
		t.Fatal(err)
	}
	env := newRunTestEnvConfigured(t, monitors, func(cfg *Config) {
		cfg.ConfigDir = configDir
		if err := os.WriteFile(cfg.HyprConfig, []byte(root), 0600); err != nil {
			t.Fatal(err)
		}
	})
	defer env.stop()
	waitFor(t, time.Second, func() bool { return env.logs.contains("automatic reconciliation completed") }, "unmanaged startup")
	got, err := os.ReadFile(env.svc.cfg.HyprConfig)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != root {
		t.Fatalf("unmanaged startup restored generated include:\n%s", got)
	}
	if got := reloadCount(env.logPath); got != 0 {
		t.Fatalf("unmanaged startup reloaded compositor %d times", got)
	}
}

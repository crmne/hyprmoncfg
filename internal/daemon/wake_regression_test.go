package daemon

import (
	"os"
	"testing"
	"time"

	"github.com/crmne/hyprmoncfg/internal/hypr"
)

func TestFailedWakeReconcilesOutputs(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "eDP-1", Width: 2880, Height: 1800, RefreshRate: 120, Scale: 1.5, DPMSStatus: true},
		{Name: "HDMI-A-1", Width: 2560, Height: 1440, RefreshRate: 60, X: 1920, Scale: 1, DPMSStatus: true},
	}
	env := newRunTestEnv(t, monitors)
	defer env.stop()
	waitFor(t, time.Second, func() bool { return env.logs.contains("applied profile") }, "startup")
	broken := append([]hypr.Monitor(nil), monitors...)
	broken[0].Disabled = true
	broken[1].DPMSStatus = false
	if err := writeMonitorState(env.monitorStatePath, broken); err != nil {
		t.Fatal(err)
	}
	if err := writeMonitorState(env.monitorStatePath+".next", monitors); err != nil {
		t.Fatal(err)
	}
	env.suspendEvents <- false
	// The fixture moves .next into the live state only when configuration is applied.
	waitFor(t, 2*time.Second, func() bool { _, err := os.Stat(env.monitorStatePath + ".next"); return os.IsNotExist(err) }, "resume to re-enable the panel while HDMI reports asleep")
}

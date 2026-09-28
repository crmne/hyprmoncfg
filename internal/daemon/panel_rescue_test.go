package daemon

import (
	"context"
	"strings"
	"testing"

	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

// Issue #67: an external-only profile, and the HDMI display came back from a
// power cycle without a mode. Stepping its refresh down never helped; turning
// the built-in panel on did. The daemon now does that itself, and the apply
// that lights the panel succeeds while the external is still coming back.
func TestApplyBestTurnsTheBuiltInPanelOnWhenNoExternalShowsAPicture(t *testing.T) {
	const panelOff = `{"id":1,"name":"eDP-1","description":"BOE Panel","make":"BOE","model":"Panel","serial":"I1","width":0,"height":0,"refreshRate":0,"x":0,"y":0,"scale":1,"transform":0,"dpmsStatus":true,"disabled":true,"mirrorOf":"","availableModes":["1920x1080@60.00Hz"]}`
	const panelOn = `{"id":1,"name":"eDP-1","description":"BOE Panel","make":"BOE","model":"Panel","serial":"I1","width":1920,"height":1080,"refreshRate":60,"x":2560,"y":0,"scale":1,"transform":0,"dpmsStatus":true,"disabled":false,"mirrorOf":"","availableModes":["1920x1080@60.00Hz"]}`
	const hdmiModeless = `{"id":2,"name":"HDMI-A-1","description":"HKC 27E1QA","make":"HKC","model":"27E1QA","serial":"E1","width":0,"height":0,"refreshRate":0,"x":0,"y":0,"scale":1,"transform":0,"dpmsStatus":true,"disabled":false,"mirrorOf":"","availableModes":["2560x1440@144.00Hz","2560x1440@60.00Hz"]}`
	env := newApplyBestTestEnvWithMonitors(t, "["+panelOff+","+hdmiModeless+"]", "["+panelOn+","+hdmiModeless+"]")
	panel := hypr.Monitor{Name: "eDP-1", Description: "BOE Panel", Make: "BOE", Model: "Panel", Serial: "I1"}
	hdmi := hypr.Monitor{Name: "HDMI-A-1", Description: "HKC 27E1QA", Make: "HKC", Model: "27E1QA", Serial: "E1"}
	if err := env.store.Save(profile.New("casa", []profile.OutputConfig{
		{Key: panel.HardwareKey(), Name: panel.Name, Description: panel.Description, Enabled: false, Width: 1920, Height: 1080, Refresh: 60, Scale: 1},
		{Key: hdmi.HardwareKey(), Name: hdmi.Name, Description: hdmi.Description, Enabled: true, Width: 2560, Height: 1440, Refresh: 144, Scale: 1},
	})); err != nil {
		t.Fatal(err)
	}
	var logs []string
	svc := New(env.client, env.store, Config{MonitorsConf: env.monitorsConfPath, HyprConfig: env.hyprlandConfigPath,
		Logf: func(format string, args ...any) { logs = append(logs, format) }})

	if err := svc.applyBest(context.Background()); err != nil {
		t.Fatalf("the apply that lights the panel should succeed: %v", err)
	}
	rendered := readMonitorsConf(t, env)
	block := rendered[strings.Index(rendered, "output = desc:BOE Panel"):]
	block = block[:strings.Index(block, "}")]
	if strings.Contains(block, "disabled") || !strings.Contains(block, "position = 2560x0") {
		t.Fatalf("the built-in panel should be on, right of the external:\n%s", rendered)
	}
	if !strings.Contains(strings.Join(logs, "\n"), "no external display shows a picture") {
		t.Fatalf("the daemon should say why the panel came on: %v", logs)
	}
	if svc.fallbacks.due["hkc 27e1qa"] != "" || len(svc.fallbacks.noMode) != 0 {
		t.Fatal("with nothing lit, a missing mode must not step the external down")
	}
}

func TestFallbackCountsNoModeOnlyWhenSomethingElseIsLit(t *testing.T) {
	f, _ := trackerAt("", t)
	p, lg, msi := fallbackDesk()
	f.apply(p, []hypr.Monitor{lg, msi})
	modeless := msi
	modeless.Width, modeless.Height = 0, 0
	dark := lg
	dark.Disabled = true
	for i := 0; i < 3; i++ {
		f.observeFailedApply(p, []hypr.Monitor{dark, modeless})
	}
	if got := msiOutput(t, f.apply(p, []hypr.Monitor{dark, modeless})); got.VRR != 1 {
		t.Fatal("with no display lit, a missing mode is not evidence against the saved settings")
	}
}

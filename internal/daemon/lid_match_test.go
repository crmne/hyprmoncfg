package daemon

import (
	"context"
	"strings"
	"testing"

	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/lid"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

// Issue #76: docked with the lid closed, the daemon applied a three-screen
// profile and forced its panel off instead of the clamshell profile saved for
// exactly this setup. Status recommended the same wrong profile.
func TestClosedLidAppliesAndRecommendsTheClamshellProfile(t *testing.T) {
	const state = `[` +
		`{"id":0,"name":"eDP-1","description":"Sharp LQ134","make":"Sharp","model":"LQ134","serial":"","width":2880,"height":1800,"refreshRate":120,"x":0,"y":0,"scale":2,"transform":0,"dpmsStatus":true,"disabled":true,"mirrorOf":""},` +
		`{"id":1,"name":"DP-1","description":"Dell U2723QE A","make":"Dell","model":"U2723QE","serial":"A","width":3840,"height":2160,"refreshRate":60,"x":0,"y":0,"scale":1.5,"transform":0,"dpmsStatus":true,"disabled":false,"mirrorOf":""},` +
		`{"id":2,"name":"DP-2","description":"Dell U2723QE B","make":"Dell","model":"U2723QE","serial":"B","width":3840,"height":2160,"refreshRate":60,"x":2560,"y":0,"scale":1.5,"transform":0,"dpmsStatus":true,"disabled":false,"mirrorOf":""}]`
	// Booted docked with the lid closed: the panel is still on where Hyprland
	// put it, so no saved profile is on screen yet and scoring decides.
	booted := strings.Replace(strings.Replace(state, `"disabled":true`, `"disabled":false`, 1), `"x":0,"y":0,"scale":2`, `"x":0,"y":2160,"scale":2`, 1)
	env := newApplyBestTestEnvWithMonitors(t, booted, state)
	edp := hypr.Monitor{Name: "eDP-1", Description: "Sharp LQ134", Make: "Sharp", Model: "LQ134", Width: 2880, Height: 1800, RefreshRate: 120, Scale: 2, DPMSStatus: true}
	dp1 := hypr.Monitor{Name: "DP-1", Description: "Dell U2723QE A", Make: "Dell", Model: "U2723QE", Serial: "A", Width: 3840, Height: 2160, RefreshRate: 60, Scale: 1.5, DPMSStatus: true}
	dp2 := hypr.Monitor{Name: "DP-2", Description: "Dell U2723QE B", Make: "Dell", Model: "U2723QE", Serial: "B", Width: 3840, Height: 2160, RefreshRate: 60, X: 2560, Scale: 1.5, DPMSStatus: true}
	clamshell := []hypr.Monitor{edp, dp1, dp2}
	clamshell[0].Disabled = true
	three := []hypr.Monitor{edp, dp1, dp2}
	three[0].X = 5120
	for _, p := range []profile.Profile{
		profile.FromState("2 External", clamshell, nil),
		profile.FromState("3 Monitor", three, nil),
	} {
		if err := env.store.Save(p); err != nil {
			t.Fatal(err)
		}
	}
	svc := New(env.client, env.store, Config{MonitorsConf: env.monitorsConfPath, HyprConfig: env.hyprlandConfigPath})
	svc.setLidState(lid.Closed)

	document, err := svc.Status()
	if err != nil {
		t.Fatal(err)
	}
	if document.RecommendedProfile == nil || document.RecommendedProfile.Name != "2 External" {
		t.Fatalf("status should recommend the clamshell profile with the lid closed, got %+v", document.RecommendedProfile)
	}

	var chosen string
	svc.cfg.Logf = func(format string, args ...any) {
		if strings.HasPrefix(format, "best profile %q") && len(args) > 0 {
			chosen, _ = args[0].(string)
		}
	}
	if err := svc.applyBest(context.Background()); err != nil {
		t.Fatalf("applyBest: %v", err)
	}
	if chosen != "2 External" {
		t.Fatalf("the daemon applied %q with the lid closed, want the clamshell profile", chosen)
	}
}

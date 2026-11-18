package daemon

import (
	"context"
	"strings"
	"testing"

	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

// Issue #126: at a hot desk, another unit of the saved monitor model gets the
// saved layout, on that unit, without the saved unit's calibration, and the
// saved profile stays as it was.
func TestAnotherUnitOfTheSavedModelGetsTheSavedLayout(t *testing.T) {
	const panel = `{"id":0,"name":"eDP-1","description":"BOE NE135","make":"BOE","model":"NE135","serial":"","width":2880,"height":1920,"refreshRate":120,"x":%s,"y":%s,"scale":2,"transform":0,"dpmsStatus":true,"disabled":false,"mirrorOf":""}`
	const office = `{"id":1,"name":"DP-3","description":"Dell Inc. DELL U2723QE OFFICE","make":"Dell Inc.","model":"DELL U2723QE","serial":"OFFICE","width":3840,"height":2160,"refreshRate":60,"x":%s,"y":0,"scale":1.5,"transform":0,"dpmsStatus":true,"disabled":false,"mirrorOf":"","availableModes":["3840x2160@60.00Hz"]}`
	fill := func(format string, values ...string) string {
		for _, value := range values {
			format = strings.Replace(format, "%s", value, 1)
		}
		return format
	}
	// Hyprland extends the new unit to the right; the saved desk has the
	// external above the laptop.
	before := "[" + fill(panel, "0", "0") + "," + fill(office, "1440") + "]"
	after := "[" + fill(panel, "640", "1440") + "," + fill(office, "0") + "]"
	env := newApplyBestTestEnvWithMonitors(t, before, after)

	laptop := hypr.Monitor{Name: "eDP-1", Description: "BOE NE135", Make: "BOE", Model: "NE135", Width: 2880, Height: 1920, RefreshRate: 120, Scale: 2, X: 640, Y: 1440}
	home := hypr.Monitor{Name: "DP-1", Description: "Dell Inc. DELL U2723QE HOME", Make: "Dell Inc.", Model: "DELL U2723QE", Serial: "HOME", Width: 3840, Height: 2160, RefreshRate: 60, Scale: 1.5}
	desk := profile.FromState("Desk", []hypr.Monitor{laptop, home}, nil)
	for i := range desk.Outputs {
		if desk.Outputs[i].Serial == "HOME" {
			desk.Outputs[i].ICC = "/home/me/u2723qe-home.icc"
		}
	}
	laptopOnly := laptop
	laptopOnly.X, laptopOnly.Y = 0, 0
	for _, p := range []profile.Profile{desk, profile.FromState("Laptop", []hypr.Monitor{laptopOnly}, nil)} {
		if err := env.store.Save(p); err != nil {
			t.Fatal(err)
		}
	}

	svc := New(env.client, env.store, Config{MonitorsConf: env.monitorsConfPath, HyprConfig: env.hyprlandConfigPath})
	document, err := svc.Status()
	if err != nil {
		t.Fatal(err)
	}
	if document.RecommendedProfile == nil || document.RecommendedProfile.Name != "Desk" {
		t.Fatalf("recommended %+v, want Desk", document.RecommendedProfile)
	}
	for _, summary := range document.Profiles {
		if summary.Name != "Desk" {
			continue
		}
		sameModel := false
		for _, reason := range summary.MatchReasons {
			sameModel = sameModel || reason.Kind == profile.MatchReasonConnectedSameModel && reason.Count == 1
		}
		if !sameModel || summary.ExactDisplayMatch || summary.ConnectedOutputs != 2 {
			t.Fatalf("Desk summary = %+v, want it explained as a same-model match", summary)
		}
	}

	var logs []string
	svc.cfg.Logf = func(format string, args ...any) {
		logs = append(logs, format)
	}
	if err := svc.applyBest(context.Background()); err != nil {
		t.Fatalf("applyBest: %v", err)
	}
	rendered := readMonitorsConf(t, env)
	for _, want := range []string{"output = desc:Dell Inc. DELL U2723QE OFFICE", "position = 0x0", "position = 640x1440"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered config missing %q:\n%s", want, rendered)
		}
	}
	for _, unwanted := range []string{"HOME", "icc"} {
		if strings.Contains(rendered, unwanted) {
			t.Fatalf("rendered config mentions %q:\n%s", unwanted, rendered)
		}
	}
	if !strings.Contains(strings.Join(logs, "\n"), "same model, different unit") {
		t.Fatalf("daemon did not log the model match: %q", logs)
	}

	saved, err := env.store.Load("Desk")
	if err != nil {
		t.Fatal(err)
	}
	if output, ok := saved.OutputByKey(home.HardwareKey()); !ok || output.ICC == "" || output.Serial != "HOME" {
		t.Fatalf("saved Desk changed: %+v", saved.Outputs)
	}

	document, err = svc.Status()
	if err != nil {
		t.Fatal(err)
	}
	if document.ActiveProfile == nil || document.ActiveProfile.Name != "Desk" {
		t.Fatalf("active %+v, want Desk on the office unit", document.ActiveProfile)
	}
}

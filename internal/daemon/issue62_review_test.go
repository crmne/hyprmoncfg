package daemon

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/crmne/hyprmoncfg/internal/lid"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

func TestIssue62ApplyRestoresSavedLaptopWhenExternalHasNoMode(t *testing.T) {
	monitors := applyBestDualMonitors()
	for i := range monitors {
		monitors[i].DPMSStatus = true
	}
	healthy, err := json.Marshal(monitors)
	if err != nil {
		t.Fatal(err)
	}
	saved := profile.FromMonitors("Dock", monitors)
	for i := range monitors {
		if monitors[i].IsInternal() {
			monitors[i].Disabled = true
		} else {
			monitors[i].Width, monitors[i].Height = 0, 0
		}
	}
	broken, err := json.Marshal(monitors)
	if err != nil {
		t.Fatal(err)
	}
	env := newApplyBestTestEnvWithMonitors(t, string(broken), string(healthy))
	if err := env.store.Save(saved); err != nil {
		t.Fatal(err)
	}
	svc := New(env.client, env.store, Config{MonitorsConf: env.monitorsConfPath, HyprConfig: env.hyprlandConfigPath})
	svc.lidSupported = true
	svc.readLid = func(context.Context) (lid.State, error) { return lid.Closed, nil }
	if err := svc.applyBest(context.Background()); err != nil {
		t.Fatal(err)
	}
	if rendered := readMonitorsConf(t, env); strings.Contains(rendered, "disable") {
		t.Fatal("apply disabled an output despite modeless external", rendered)
	}
	persisted, err := env.store.Load("Dock")
	if err != nil {
		t.Fatal(err)
	}
	for _, out := range persisted.Outputs {
		if !out.Enabled {
			t.Fatal("saved profile was changed")
		}
	}
}

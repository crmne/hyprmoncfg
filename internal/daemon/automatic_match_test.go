package daemon

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

func TestApplyBestPreservesExplicitStrictPolicy(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "eDP-1", Make: "Example", Model: "Laptop", Width: 1920, Height: 1080, RefreshRate: 60, Scale: 1, DPMSStatus: true},
		{Name: "DP-1", Make: "Example", Model: "New display", Width: 1920, Height: 1080, RefreshRate: 60, Scale: 1, X: 1920, DPMSStatus: true},
	}
	saved := profile.FromMonitors("Strict laptop", monitors[:1])
	saved.DisableUnknownOutputs = true
	before, _ := json.Marshal(monitors)
	monitors[1].Disabled = true
	after, _ := json.Marshal(monitors)
	env := newApplyBestTestEnvWithMonitors(t, string(before), string(after))
	if err := env.store.Save(saved); err != nil {
		t.Fatal(err)
	}
	svc := New(env.client, env.store, Config{MonitorsConf: env.monitorsConfPath, HyprConfig: env.hyprlandConfigPath})
	if err := svc.applyBest(context.Background()); err != nil {
		t.Fatal(err)
	}
	if rendered := readMonitorsConf(t, env); !strings.Contains(rendered, "disable") {
		t.Fatalf("explicit strict profile was extended instead: %s", rendered)
	}
	stored, err := env.store.Load(saved.Name)
	if err != nil || !stored.DisableUnknownOutputs || len(stored.Outputs) != 1 {
		t.Fatalf("strict saved profile was changed: %+v (%v)", stored, err)
	}
}

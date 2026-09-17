package daemon

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

func TestApplyBestDoesNotDisableUnfamiliarConnectedDisplays(t *testing.T) {
	env := newApplyBestTestEnvWithMonitors(t, applyBestDualBeforeJSON, applyBestDualBeforeJSON)
	laptop := applyBestDualMonitors()[0]
	otherSite := hypr.Monitor{Name: "DP-9", Make: "Other", Model: "Projector", Width: 1920, Height: 1080, Scale: 1}
	if err := env.store.Save(profile.FromMonitors("Other location", []hypr.Monitor{laptop, otherSite})); err != nil {
		t.Fatal(err)
	}
	before := readMonitorsConf(t, env)
	svc := New(env.client, env.store, Config{MonitorsConf: env.monitorsConfPath, HyprConfig: env.hyprlandConfigPath})
	if err := svc.applyBest(context.Background()); err != nil {
		t.Fatal(err)
	}
	if after := readMonitorsConf(t, env); after != before {
		t.Fatalf("unfamiliar displays must leave generated config unchanged:\n%s", after)
	}
	log, err := os.ReadFile(env.logPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(log), "reload") {
		t.Fatalf("unfamiliar displays must not trigger a reload:\n%s", log)
	}
}

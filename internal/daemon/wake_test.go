package daemon

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crmne/hyprmoncfg/internal/ipc"
	"github.com/crmne/hyprmoncfg/internal/omarchywatch"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

func wakeTestService(t *testing.T) (*Service, profile.Profile, string, string) {
	t.Helper()
	state := `[{"name":"eDP-1","make":"BOE","model":"Panel","width":1920,"height":1080,"refreshRate":60,"x":4096,"y":576,"scale":1.33333,"dpmsStatus":true}]`
	env := newApplyBestTestEnvWithMonitors(t, state, state)
	t.Setenv("HOME", t.TempDir())
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "omarchy-hyprland-monitor-clamshell"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	path := filepath.Join(os.Getenv("HOME"), ".config/hypr/monitors.lua")
	before := "local omarchy_monitor_scale = 1.6\nhl.monitor({ output = \"\", scale = omarchy_monitor_scale })\n"
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(before), 0o644); err != nil {
		t.Fatal(err)
	}
	monitors, err := env.client.Monitors(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	p := profile.FromMonitors("Laptop", monitors)
	if err := env.store.Save(p); err != nil {
		t.Fatal(err)
	}
	svc := New(env.client, env.store, Config{MonitorsConf: env.monitorsConfPath, HyprConfig: env.hyprlandConfigPath,
		ConfigDir: t.TempDir(), WakeConfig: omarchywatch.NewWakeConfig()})
	t.Cleanup(func() { _ = svc.Shutdown() })
	return svc, p, path, before
}

func TestAppliedProfileSharesWakeSettingsUntilUnmanaged(t *testing.T) {
	svc, _, path, before := wakeTestService(t)
	if err := svc.applyBest(context.Background()); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(content), `position = "4096x576", scale = 1.33333`) {
		t.Fatalf("apply did not share wake geometry: %s (%v)", content, err)
	}
	svc.cfg.ReleaseWatcher = func(context.Context) error {
		content, err := os.ReadFile(path)
		if err != nil || string(content) != before {
			t.Error("resumed Omarchy before removing our wake settings")
		}
		return nil
	}
	if err := svc.Unmanage(); err != nil {
		t.Fatal(err)
	}
}

func TestPreviewSafetyRollbackRestoresOmarchyWakeSettings(t *testing.T) {
	for _, action := range []string{"timeout", "shutdown", "unmanage", "commit"} {
		t.Run(action, func(t *testing.T) {
			svc, p, path, before := wakeTestService(t)
			transaction, err := svc.Preview("panel", ipc.PreviewParams{Profile: &p, TimeoutSeconds: 10})
			if err != nil {
				t.Fatal(err)
			}
			preview, err := os.ReadFile(path)
			if err != nil || string(preview) == before {
				t.Fatal("preview did not update wake settings")
			}
			switch action {
			case "timeout":
				svc.expirePreview(transaction.ID)
			case "shutdown":
				err = svc.Shutdown()
			case "unmanage":
				err = svc.Unmanage()
			case "commit":
				err = svc.Confirm("panel", ipc.TransactionParams{TransactionID: transaction.ID})
			}
			if err != nil {
				t.Fatal(err)
			}
			want := before
			if action == "commit" {
				want = string(preview)
			}
			if content, err := os.ReadFile(path); err != nil || string(content) != want {
				t.Fatalf("%s left wrong wake settings: %s (%v)", action, content, err)
			}
		})
	}
}

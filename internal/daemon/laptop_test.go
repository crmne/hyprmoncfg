package daemon

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/omarchywatch"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

func TestNativeLaptopToggleEditsSavedGeometry(t *testing.T) {
	for _, disable := range []bool{false, true} {
		t.Run(map[bool]string{true: "disable", false: "enable"}[disable], func(t *testing.T) {
			monitors := []hypr.Monitor{
				{Name: "eDP-1", Make: "Laptop", Model: "Panel", Width: 1920, Height: 1080, RefreshRate: 60, Scale: 1, X: 1920, DPMSStatus: true, Disabled: !disable},
				{Name: "DP-1", Make: "Dell", Model: "External", Width: 1920, Height: 1080, RefreshRate: 60, Scale: 1, DPMSStatus: true},
			}
			saved := profile.FromMonitors("Desk", monitors)
			// A disabled monitor can report no placement or geometry at all.
			before := append([]hypr.Monitor(nil), monitors...)
			if !disable {
				before[0].X, before[0].Width, before[0].Height = 0, 0, 0
			}
			after := append([]hypr.Monitor(nil), monitors...)
			after[0].Disabled = disable
			beforeJSON, _ := json.Marshal(before)
			afterJSON, _ := json.Marshal(after)
			env := newApplyBestTestEnvWithMonitors(t, string(beforeJSON), string(afterJSON))
			if err := env.store.Save(saved); err != nil {
				t.Fatal(err)
			}
			t.Setenv("HOME", t.TempDir())
			bin := t.TempDir()
			if err := os.WriteFile(filepath.Join(bin, "omarchy-hyprland-monitor-clamshell"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			toggle := omarchywatch.NewLaptopToggle()
			if toggle == nil {
				t.Fatal("fixture has no native toggle")
			}
			if _, err := toggle.Sync(saved, monitors); err != nil {
				t.Fatal(err)
			}
			svc := New(env.client, env.store, Config{MonitorsConf: env.monitorsConfPath, HyprConfig: env.hyprlandConfigPath, LaptopToggle: toggle})
			svc.lastProfile, svc.lastMonitorSet = saved, profile.MonitorSetHash(monitors)
			flag := filepath.Join(os.Getenv("HOME"), ".local/state/omarchy/toggles/hypr/internal-monitor-disable.lua")
			if disable {
				if err := os.MkdirAll(filepath.Dir(flag), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(flag, []byte(`hl.monitor({ output = "eDP-1", disabled = true })`), 0o644); err != nil {
					t.Fatal(err)
				}
			} else if err := os.Remove(flag); err != nil {
				t.Fatal(err)
			}
			if err := svc.applyBest(context.Background()); err != nil {
				t.Fatal(err)
			}
			got, err := env.store.Load(saved.Name)
			if err != nil {
				t.Fatal(err)
			}
			for _, output := range got.Outputs {
				if output.Name == "eDP-1" && (output.Enabled == disable || output.X != 1920 || output.Width != 1920 || output.Height != 1080 || output.Scale != 1) {
					t.Fatalf("native toggle lost the saved layout: %+v", output)
				}
			}
			if _, manual := svc.manualOverride(profile.MonitorSetHash(monitors)); manual {
				t.Fatal("native toggle changed automatic selection to manual")
			}
			if _, changed, err := toggle.Changed(); err != nil || changed {
				t.Fatal("native toggle keeps triggering itself")
			}
			if disable {
				// Omarchy's wake recovery may clear the flag while a dock sleeps.
				// Restore the preference without saving that transient recovery.
				if err := os.Remove(flag); err != nil {
					t.Fatal(err)
				}
				toggle.Reset()
				if err := svc.applyBest(context.Background()); err != nil {
					t.Fatal(err)
				}
				if _, err := os.Stat(flag); err != nil {
					t.Fatal("resume lost the external-only preference")
				}
				resumed, err := env.store.Load(saved.Name)
				if err != nil {
					t.Fatal(err)
				}
				for _, output := range resumed.Outputs {
					if output.Name == "eDP-1" && output.Enabled {
						t.Fatal("resume rewrote the user's saved profile")
					}
				}
			}
		})
	}
}

func TestLaptopToggleAddsAnOmittedPanelWithoutMovingTheDesk(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "eDP-1", Make: "Laptop", Model: "Panel", Disabled: true, AvailableModes: []string{"1920x1080@60.00Hz"}},
		{Name: "DP-1", Make: "Dell", Model: "Desk", Width: 3840, Height: 2160, RefreshRate: 60, Scale: 2, X: 100, DPMSStatus: true},
	}
	saved := profile.FromMonitors("Desk", monitors[1:])
	got, err := setLaptopDisplay(saved, monitors, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Outputs) != 1 || len(got.Outputs) != 2 {
		t.Fatal("toggle did not add exactly the missing panel to a copy")
	}
	for _, output := range got.Outputs {
		if output.Name == "eDP-1" {
			if !output.Enabled || output.X != 2020 || output.Width != 1920 || output.Scale != 1 {
				t.Fatalf("unexpected laptop placement: %+v", output)
			}
		} else if output != saved.Outputs[0] {
			t.Fatalf("external display was changed: %+v", output)
		}
	}
	if err := profile.ValidateLayout(got.Outputs); err != nil {
		t.Fatal(err)
	}
}

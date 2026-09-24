package profile

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/crmne/hyprmoncfg/internal/hypr"
)

func TestReuseKeepsTemplateUnknownDisplayPolicy(t *testing.T) {
	for _, strict := range []bool{false, true} {
		for _, donorKind := range []string{"none", "exact", "best"} {
			t.Run(fmt.Sprintf("strict=%t/%s", strict, donorKind), func(t *testing.T) {
				saved, monitors, mapping := reuseFixture()
				saved.DisableUnknownOutputs = strict
				before := cloneForReuse(saved)
				var donors []Profile
				if donorKind != "none" {
					donor := FromMonitors("Current", monitors)
					donor.DisableUnknownOutputs = !strict
					for i := range donor.Outputs {
						donor.Outputs[i].ICC = "/current/calibration.icc"
					}
					if donorKind == "best" {
						for i := range donor.Outputs {
							donor.Outputs[i].Y += 100
						}
					}
					donors = []Profile{donor}
					if _, exact := ExactStateMatch(donors, monitors, nil); exact != (donorKind == "exact") {
						t.Fatal("fixture did not select the intended donor recovery path")
					}
				}
				draft, _, err := ReuseLayout(saved, donors, monitors, nil, mapping)
				if err != nil {
					t.Fatal(err)
				}
				if draft.DisableUnknownOutputs != strict {
					t.Fatalf("template strict=%t became %t after %s calibration recovery", strict, draft.DisableUnknownOutputs, donorKind)
				}
				if donorKind != "none" {
					for _, output := range draft.Outputs {
						if output.ICC != "/current/calibration.icc" {
							t.Fatal("policy preservation lost target calibration")
						}
					}
				}
				if !reflect.DeepEqual(saved, before) {
					t.Fatal("template was mutated")
				}
			})
		}
	}
}

func TestReuseRepairsAdaptedMappedGeometryAndKeepsMirrorsAligned(t *testing.T) {
	for _, adaptation := range []string{"mode", "rotated-mode", "scale"} {
		t.Run(adaptation, func(t *testing.T) {
			old := []hypr.Monitor{
				{Name: "DP-1", Make: "Example", Model: "Resized", Serial: "one", Width: 1920, Height: 1080, RefreshRate: 60, Scale: 1},
				{Name: "DP-2", Make: "Example", Model: "Unchanged", Serial: "two", Width: 1920, Height: 1080, RefreshRate: 60, Scale: 1},
				{Name: "DP-3", Make: "Example", Model: "Mirror", Serial: "three", Width: 1920, Height: 1080, RefreshRate: 60, Scale: 1, MirrorOf: "DP-1"},
			}
			if adaptation == "scale" {
				old[0].Scale = 1.51
			} else if adaptation == "rotated-mode" {
				old[0].Transform = 1
			}
			width, _ := old[0].LogicalSize()
			old[1].X = width
			saved := FromMonitors("Template", old)
			if err := ValidateLayout(saved.Outputs); err != nil {
				t.Fatalf("fixture must start without overlap: %v", err)
			}
			current := append([]hypr.Monitor(nil), old...)
			if adaptation != "scale" {
				current[0].Width, current[0].Height = 3840, 2160
			}
			mapping := make(map[string]string)
			for _, monitor := range current {
				mapping[monitor.HardwareKey()] = monitor.HardwareKey()
			}
			draft, warnings, err := ReuseLayout(saved, nil, current, nil, mapping)
			if err != nil {
				t.Fatal(err)
			}
			if err := ValidateLayout(draft.Outputs); err != nil {
				t.Fatalf("adapted mapped geometry cannot be previewed: %v", err)
			}
			resized, _ := draft.OutputByKey(current[0].HardwareKey())
			unchanged, _ := draft.OutputByKey(current[1].HardwareKey())
			mirror, _ := draft.OutputByKey(current[2].HardwareKey())
			if unchanged.X != old[1].X || unchanged.Y != old[1].Y || resized.X == old[0].X {
				t.Fatalf("did not move only adapted geometry: resized=%+v unchanged=%+v", resized, unchanged)
			}
			if mirror.MirrorOf != resized.Key || mirror.X != resized.X || mirror.Y != resized.Y {
				t.Fatalf("mirror was not aligned to its relocated source: mirror=%+v source=%+v", mirror, resized)
			}
			if !strings.Contains(strings.Join(warnings, " "), "placed to the right") {
				t.Fatal("placement adjustment was not disclosed")
			}
		})
	}
}

func TestReuseRejectsUnchangedOverlappingMappedLayout(t *testing.T) {
	saved, monitors, mapping := reuseFixture()
	saved.Outputs[1].X = saved.Outputs[0].X
	if _, _, err := ReuseLayout(saved, nil, monitors, nil, mapping); err == nil || !strings.Contains(err.Error(), "layout overlaps") {
		t.Fatalf("returned an invalid unchanged layout as a usable draft: %v", err)
	}
}

func TestReuseRejectsSkippedRolesWithoutARealConfiguredDisplay(t *testing.T) {
	saved := FromMonitors("Template", []hypr.Monitor{{Name: "DP-8", Make: "Example", Model: "Template", Width: 1920, Height: 1080, RefreshRate: 60, Scale: 1}})
	for _, tc := range []struct {
		name          string
		connector     string
		width, height int
	}{
		{"modeless", "DP-1", 0, 0},
		{"zero-width", "DP-1", 0, 1080},
		{"zero-height", "DP-1", 1920, 0},
		// The client already filters synthetic heads. Keep the shared helper's
		// final invariant valid for callers supplying monitor snapshots directly.
		{"synthetic", " fallback ", 1920, 1080},
	} {
		t.Run(tc.name, func(t *testing.T) {
			monitors := []hypr.Monitor{{Name: tc.connector, Make: "Example", Model: "Current", Width: tc.width, Height: tc.height, RefreshRate: 60, Scale: 1, DPMSStatus: true}}
			mapping := map[string]string{saved.Outputs[0].Key: ""}
			if _, _, err := ReuseLayout(saved, nil, monitors, nil, mapping); err == nil || !strings.Contains(err.Error(), "usable mode") {
				t.Fatalf("reuse accepted only an unconfigured or synthetic output: %v", err)
			}
		})
	}
}

func TestReuseAcceptsConfiguredModesForInactiveTargets(t *testing.T) {
	saved := FromMonitors("Template", []hypr.Monitor{{Name: "DP-8", Make: "Example", Model: "Template", Width: 1920, Height: 1080, RefreshRate: 60, Scale: 1}})
	for _, tc := range []struct {
		name          string
		disabled      bool
		width, height int
		mode          string
		wantWidth     int
		wantHeight    int
	}{
		{"disabled-supported-mode", true, 0, 0, "1920x1080@60Hz", 1920, 1080},
		{"modeless-supported-mode", false, 0, 0, "1920x1080@60Hz", 1920, 1080},
		{"disabled-fallback-mode", true, 0, 0, "1280x720@60Hz", 1280, 720},
		{"sleeping-current-mode", false, 1920, 1080, "", 1920, 1080},
	} {
		t.Run(tc.name, func(t *testing.T) {
			monitor := hypr.Monitor{Name: "DP-1", Make: "Example", Model: "Current", Disabled: tc.disabled, Width: tc.width, Height: tc.height, RefreshRate: 60, Scale: 1, DPMSStatus: false}
			if tc.mode != "" {
				monitor.AvailableModes = []string{tc.mode}
			}
			draft, _, err := ReuseLayout(saved, nil, []hypr.Monitor{monitor}, nil, map[string]string{saved.Outputs[0].Key: monitor.HardwareKey()})
			if err != nil {
				t.Fatalf("reuse rejected a configurable display: %v", err)
			}
			output := draft.Outputs[0]
			if !output.Enabled || output.MirrorOf != "" || output.Width != tc.wantWidth || output.Height != tc.wantHeight {
				t.Fatalf("draft did not configure an independent display: %+v", output)
			}
		})
	}
}

func TestReusePreservesGeneratedWorkspaceOrderBeforeAddingLiveDisplays(t *testing.T) {
	for _, strategy := range []WorkspaceStrategy{WorkspaceStrategySequential, WorkspaceStrategyInterleave} {
		for _, order := range []string{"explicit", "rules", "implicit"} {
			for _, extra := range []bool{false, true} {
				for _, skip := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/%s/extra=%t/skip=%t", strategy, order, extra, skip), func(t *testing.T) {
						saved, monitors, mapping := reuseFixture()
						saved.Workspaces.Strategy = strategy
						saved.Workspaces.MaxWorkspaces, saved.Workspaces.GroupSize = 12, 2
						switch order {
						case "explicit":
							saved.Workspaces.MonitorOrder = []string{saved.Outputs[1].Key, saved.Outputs[0].Key}
						case "rules":
							saved.Workspaces.MonitorOrder = nil
							saved.Workspaces.Rules[0], saved.Workspaces.Rules[1] = saved.Workspaces.Rules[1], saved.Workspaces.Rules[0]
						case "implicit":
							saved.Workspaces.MonitorOrder = nil
							saved.Workspaces.Rules = nil
						}
						if !extra {
							monitors = monitors[:2]
						}
						if skip {
							mapping[saved.Outputs[0].Key] = ""
						}
						before := cloneForReuse(saved)
						var want []string
						seen := map[string]bool{}
						for _, rule := range ResolveWorkspaceRules(saved, nil) {
							if target := mapping[rule.OutputKey]; target != "" && !seen[target] {
								want = append(want, target)
								seen[target] = true
							}
						}
						draft, _, err := ReuseLayout(saved, nil, monitors, nil, mapping)
						if err != nil {
							t.Fatal(err)
						}
						var got []string
						seen = map[string]bool{}
						for _, rule := range ResolveWorkspaceRules(draft, nil) {
							if !seen[rule.OutputKey] {
								got = append(got, rule.OutputKey)
								seen[rule.OutputKey] = true
							}
						}
						if len(got) != len(monitors) || len(got) < len(want) || !reflect.DeepEqual(got[:len(want)], want) {
							t.Fatalf("mapped workspace order must precede unassigned displays: got %v, want prefix %v", got, want)
						}
						if !reflect.DeepEqual(saved, before) {
							t.Fatal("template was mutated")
						}
					})
				}
			}
		}
	}
}

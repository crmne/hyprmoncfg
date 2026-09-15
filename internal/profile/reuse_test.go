package profile

import (
	"reflect"
	"strings"
	"testing"

	"github.com/crmne/hyprmoncfg/internal/hypr"
)

func reuseFixture() (Profile, []hypr.Monitor, map[string]string) {
	old := []hypr.Monitor{
		{Name: "DP-8", Make: "Example", Model: "Panel", Serial: "old-left", Width: 1920, Height: 1080, RefreshRate: 60, Scale: 1},
		{Name: "DP-9", Make: "Example", Model: "Panel", Serial: "old-right", Width: 1920, Height: 1080, RefreshRate: 60, Scale: 1, X: 1920},
	}
	current := []hypr.Monitor{
		{Name: "DP-1", Make: "Example", Model: "Panel", Serial: "new-a", Width: 1920, Height: 1080, RefreshRate: 60, Scale: 1},
		{Name: "DP-2", Make: "Example", Model: "Panel", Serial: "new-b", Width: 1920, Height: 1080, RefreshRate: 60, Scale: 1, X: 1920},
		{Name: "eDP-1", Make: "Example", Model: "Laptop", Width: 1920, Height: 1080, RefreshRate: 60, Scale: 1},
	}
	p := FromMonitors("Office", old)
	p.Workspaces = WorkspaceSettings{Enabled: true, Strategy: WorkspaceStrategyManual,
		MonitorOrder: []string{p.Outputs[0].Key, p.Outputs[1].Key},
		Rules:        []WorkspaceRule{{Workspace: "1", OutputKey: p.Outputs[0].Key, OutputName: "DP-8"}, {Workspace: "2", OutputKey: p.Outputs[1].Key, OutputName: "DP-9"}}}
	p.Exec = "do-not-run-or-copy"
	mapping := map[string]string{p.Outputs[0].Key: current[1].HardwareKey(), p.Outputs[1].Key: current[0].HardwareKey()}
	return p, current, mapping
}

func TestReuseLayoutRebindsIdentityAndWorkspaceRolesWithoutMutatingSource(t *testing.T) {
	saved, monitors, mapping := reuseFixture()
	before := cloneForReuse(saved)
	draft, warnings, err := ReuseLayout(saved, monitors, nil, mapping)
	if err != nil {
		t.Fatal(err)
	}
	if draft.Name != "" || draft.Exec != "" || len(draft.Outputs) != 3 {
		t.Fatalf("invalid new draft: %+v", draft)
	}
	if !reflect.DeepEqual(saved, before) {
		t.Fatal("source profile mutated")
	}
	for _, source := range saved.Outputs {
		target, ok := draft.OutputByKey(mapping[source.Key])
		if !ok || target.Serial == source.Serial || target.X != source.X || target.Name == source.Name {
			t.Fatalf("role not rebound: %+v", target)
		}
	}
	if draft.Workspaces.Rules[0].OutputKey != mapping[saved.Outputs[0].Key] || draft.Workspaces.Rules[0].OutputName != "DP-2" {
		t.Fatalf("workspace reference stale: %+v", draft.Workspaces)
	}
	if len(warnings) == 0 {
		t.Fatal("missing hardware/exec warnings")
	}
	laptop, _ := draft.OutputByKey(monitors[2].HardwareKey())
	if !laptop.Enabled || laptop.X < 3840 {
		t.Fatalf("extra display not kept clear: %+v", laptop)
	}
}

func TestReuseLayoutMapsMirrorsAndDropsExplicitlySkippedRoles(t *testing.T) {
	saved, monitors, mapping := reuseFixture()
	saved.Outputs[1].MirrorOf = saved.Outputs[0].Key
	draft, _, err := ReuseLayout(saved, monitors, nil, mapping)
	if err != nil {
		t.Fatal(err)
	}
	mirror, _ := draft.OutputByKey(mapping[saved.Outputs[1].Key])
	if mirror.MirrorOf != mapping[saved.Outputs[0].Key] {
		t.Fatalf("stale mirror: %+v", mirror)
	}
	mapping[saved.Outputs[0].Key] = ""
	draft, warnings, err := ReuseLayout(saved, monitors, nil, mapping)
	if err != nil {
		t.Fatal(err)
	}
	mirror, _ = draft.OutputByKey(mapping[saved.Outputs[1].Key])
	if mirror.MirrorOf != "" {
		t.Fatal("skipped mirror target retained")
	}
	if len(draft.Workspaces.Rules) != 1 || draft.Workspaces.Rules[0].Workspace != "2" {
		t.Fatalf("skipped workspace retained: %+v", draft.Workspaces)
	}
	if !strings.Contains(strings.Join(warnings, " "), "Skipped saved display") {
		t.Fatal("skip not disclosed")
	}
}

func TestReuseLayoutRejectsIncompleteStaleAndDuplicateMappings(t *testing.T) {
	for _, kind := range []string{"missing", "duplicate", "stale-source", "stale-target", "all-disabled"} {
		t.Run(kind, func(t *testing.T) {
			saved, monitors, mapping := reuseFixture()
			switch kind {
			case "missing":
				delete(mapping, saved.Outputs[0].Key)
			case "duplicate":
				mapping[saved.Outputs[1].Key] = mapping[saved.Outputs[0].Key]
			case "stale-source":
				mapping["not-saved"] = ""
			case "stale-target":
				mapping[saved.Outputs[0].Key] = "not-connected"
			case "all-disabled":
				for i := range saved.Outputs {
					saved.Outputs[i].Enabled = false
				}
				monitors = monitors[:2]
			}
			if _, _, err := ReuseLayout(saved, monitors, nil, mapping); err == nil {
				t.Fatal("invalid mapping accepted")
			}
		})
	}
}

func TestReuseLayoutAdaptsUnavailableModeAndKeepsTargetColor(t *testing.T) {
	saved, monitors, mapping := reuseFixture()
	saved.Outputs[0].Width, saved.Outputs[0].Height, saved.Outputs[0].Mode = 3840, 2160, "3840x2160@60"
	saved.Outputs[0].ICC, saved.Outputs[0].CM, saved.Outputs[0].SupportsHDR = "/old/icc", "hdr", 1
	draft, warnings, err := ReuseLayout(saved, monitors, nil, mapping)
	if err != nil {
		t.Fatal(err)
	}
	target, _ := draft.OutputByKey(mapping[saved.Outputs[0].Key])
	if target.Width != 1920 || target.ICC != "" || target.SupportsHDR != 0 {
		t.Fatalf("unsupported mode/calibration copied: %+v", target)
	}
	if !strings.Contains(strings.Join(warnings, " "), "mode is unavailable") {
		t.Fatal("mode adjustment not disclosed")
	}
}

func TestReuseLayoutRemapsGeneratedWorkspaceOrder(t *testing.T) {
	saved, monitors, mapping := reuseFixture()
	saved.Workspaces.Strategy = WorkspaceStrategyInterleave
	saved.Workspaces.MaxWorkspaces, saved.Workspaces.GroupSize = 12, 4
	draft, _, err := ReuseLayout(saved, monitors, nil, mapping)
	if err != nil {
		t.Fatal(err)
	}
	if draft.Workspaces.MonitorOrder[0] != mapping[saved.Outputs[0].Key] || draft.Workspaces.MaxWorkspaces != 12 {
		t.Fatalf("lost workspace strategy/order: %+v", draft.Workspaces)
	}
	for _, rule := range ResolveWorkspaceRules(draft, nil) {
		if _, ok := draft.OutputByKey(rule.OutputKey); !ok {
			t.Fatalf("stale generated reference: %+v", rule)
		}
	}
}

func TestReuseLayoutDoesNotTransferCalibrationBetweenSeriallessTwins(t *testing.T) {
	old := []hypr.Monitor{
		{Name: "DP-1", Make: "Example", Model: "Twin", Width: 1920, Height: 1080, RefreshRate: 60, Scale: 1},
		{Name: "DP-2", Make: "Example", Model: "Twin", Width: 1920, Height: 1080, RefreshRate: 60, Scale: 1, X: 1920},
	}
	saved := FromMonitors("Twins", old)
	saved.Outputs[0].ICC = "/calibration/left.icc"
	mapping := map[string]string{saved.Outputs[0].Key: saved.Outputs[1].Key, saved.Outputs[1].Key: saved.Outputs[0].Key}
	draft, _, err := ReuseLayout(saved, old, nil, mapping)
	if err != nil {
		t.Fatal(err)
	}
	for _, output := range draft.Outputs {
		if output.ICC != "" {
			t.Fatal("transferred calibration using ambiguous hardware identity")
		}
	}
}

func TestReuseLayoutRepairsUnassignedMirrorWhoseSourceWasDisabled(t *testing.T) {
	saved, monitors, mapping := reuseFixture()
	monitors[2].MirrorOf = "DP-2"
	saved.Outputs[0].Enabled = false // this role maps to current DP-2
	draft, warnings, err := ReuseLayout(saved, monitors, nil, mapping)
	if err != nil {
		t.Fatal(err)
	}
	laptop, _ := draft.OutputByKey(monitors[2].HardwareKey())
	if !laptop.Enabled || laptop.MirrorOf != "" {
		t.Fatalf("extra monitor left dependent on disabled mirror source: %+v", laptop)
	}
	if !strings.Contains(strings.Join(warnings, " "), "made this display independent") {
		t.Fatal("mirror repair not disclosed")
	}
}

package profile

import (
	"reflect"
	"testing"

	"github.com/crmne/hyprmoncfg/internal/hypr"
)

func TestExtendConnectedPlacesDisplaysAndPlansWorkspaces(t *testing.T) {
	laptop := hypr.Monitor{Name: "eDP-1", Width: 2880, Height: 1800, Scale: 1.5, X: -1920, Y: 100}
	rotated := hypr.Monitor{Name: "DP-1", Width: 1920, Height: 1080, Scale: 1, Transform: 1, Y: 100}
	projector := hypr.Monitor{Name: "HDMI-A-1", Disabled: true, AvailableModes: []string{"1920x1080@60.00Hz"}}
	other := hypr.Monitor{Name: "HDMI-A-2", Width: 1280, Height: 720, Scale: 1}
	saved := FromMonitors("desk", []hypr.Monitor{laptop, rotated})
	saved.Workspaces = WorkspaceSettings{Enabled: true, Strategy: WorkspaceStrategySequential, GroupSize: 3, MaxWorkspaces: 12}
	original := FromMonitors("desk", []hypr.Monitor{laptop, rotated})
	monitors := []hypr.Monitor{other, projector, rotated, laptop}
	got := ExtendConnected(saved, monitors)
	first, _ := got.OutputByKey(projector.HardwareKey())
	second, _ := got.OutputByKey(other.HardwareKey())
	if !first.Enabled || first.X != 1080 || first.Y != 520 || first.Width != 1920 || first.Scale != 1 {
		t.Fatalf("unexpected projector: %+v", first)
	}
	if !second.Enabled || second.X != 3000 || second.Y != 700 {
		t.Fatalf("unexpected second display: %+v", second)
	}
	rules := ResolveWorkspaceRules(got, monitors)
	if len(rules) != 12 || rules[6].OutputKey != first.Key || rules[9].OutputKey != second.Key {
		t.Fatalf("unexpected workspace plan: %+v", rules)
	}
	if !reflect.DeepEqual(saved.Outputs, original.Outputs) || len(saved.Workspaces.MonitorOrder) != 0 {
		t.Fatal("extension mutated the saved profile")
	}
	if again := ExtendConnected(got, monitors); !reflect.DeepEqual(got, again) {
		t.Fatal("extension is not idempotent")
	}
}

func TestExtendConnectedDefaultsAndExplicitDisable(t *testing.T) {
	laptop := hypr.Monitor{Name: "eDP-1", Width: 1920, Height: 1080, Scale: 1}
	external := hypr.Monitor{Name: "HDMI-A-1", Width: 1920, Height: 1080, Scale: 1, Disabled: true}
	monitors := []hypr.Monitor{laptop, external}
	saved := FromMonitors("laptop", monitors[:1])
	got := ExtendConnected(saved, monitors)
	if !got.Workspaces.Enabled || got.Workspaces.Strategy != WorkspaceStrategySequential || got.Workspaces.GroupSize != 3 {
		t.Fatalf("unexpected defaults: %+v", got.Workspaces)
	}
	external.Disabled, external.X = false, 1920
	if _, ok := ExactStateMatch([]Profile{saved}, []hypr.Monitor{laptop, external}, nil); ok {
		t.Fatal("extended layout should be an unsaved draft")
	}
	saved.DisableUnknownOutputs = true
	if got := ExtendConnected(saved, monitors); !reflect.DeepEqual(got, saved) {
		t.Fatal("explicit disable policy must not extend the profile")
	}
	explicit := FromMonitors("off", monitors)
	if got := ExtendConnected(explicit, monitors); got.Outputs[0].Enabled != explicit.Outputs[0].Enabled || got.Outputs[1].Enabled != explicit.Outputs[1].Enabled {
		t.Fatal("explicitly disabled display was enabled")
	}
}

func TestExtendConnectedPreservesWorkspaceStrategy(t *testing.T) {
	monitors := []hypr.Monitor{{Name: "eDP-1", Width: 1920, Height: 1080, Scale: 1}, {Name: "HDMI-A-1", Width: 1920, Height: 1080, Scale: 1}}
	p := FromMonitors("laptop", monitors[:1])
	p.Workspaces = WorkspaceSettings{Enabled: true, Strategy: WorkspaceStrategyInterleave, MaxWorkspaces: 6}
	got := ExtendConnected(p, monitors)
	rules := ResolveWorkspaceRules(got, monitors)
	if len(rules) != 6 || rules[1].OutputName != "HDMI-A-1" || got.Workspaces.Strategy != WorkspaceStrategyInterleave {
		t.Fatalf("workspace preferences lost: %+v", got.Workspaces)
	}
}

func TestExtendConnectedSelectsAdvertisedPairAndConservativeSignal(t *testing.T) {
	laptop := hypr.Monitor{Name: "eDP-1", Width: 2880, Height: 1800, Scale: 1.5, X: -1920, Y: -100}
	external := hypr.Monitor{Name: "DP-1", Width: 1920, Height: 1080, Scale: 1, VRR: 1,
		ColorManagementPreset: "hdr", AvailableModes: []string{
			"1920x1080@240Hz", "invalid", "3840x2160@60Hz", "3840x2160@120Hz", "3840x2160@30Hz",
		}}
	saved := FromMonitors("laptop", []hypr.Monitor{laptop})
	got := ExtendConnected(saved, []hypr.Monitor{laptop, external})
	out, _ := got.OutputByKey(external.HardwareKey())
	if out.Width != 3840 || out.Height != 2160 || out.Refresh != 120 || out.X != 0 || out.Y != -580 {
		t.Fatalf("unexpected mode or logical centering: %+v", out)
	}
	if out.VRR != 0 || out.Bitdepth != 8 || out.CM != "srgb" || out.MirrorOf != "" || out.Transform != 0 {
		t.Fatalf("unsafe new-output defaults: %+v", out)
	}
	surviving, _ := got.OutputByKey(laptop.HardwareKey())
	if !reflect.DeepEqual(saved.Outputs[0], surviving) {
		t.Fatal("changed the surviving display")
	}
}

func TestExtendConnectedWithoutSurvivingDisplayStartsAtOrigin(t *testing.T) {
	p := New("draft", nil)
	got := ExtendConnected(p, []hypr.Monitor{{Name: "DP-1", Disabled: true, AvailableModes: []string{"1920x1080@60Hz"}}})
	if len(got.Outputs) != 1 || got.Outputs[0].X != 0 || got.Outputs[0].Y != 0 {
		t.Fatalf("unexpected initial layout: %+v", got.Outputs)
	}
}

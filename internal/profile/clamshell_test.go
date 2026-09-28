package profile

import (
	"testing"

	"github.com/crmne/hyprmoncfg/internal/hypr"
)

func TestApplyClosedLidPolicyDisablesInternalOutput(t *testing.T) {
	internal := hypr.Monitor{Name: "eDP-1", Make: "Samsung", Model: "Panel", Serial: "I1"}
	external := hypr.Monitor{Name: "DP-1", Make: "Dell", Model: "U2720Q", Serial: "E1", Width: 3840, Height: 2160, DPMSStatus: true}
	monitors := []hypr.Monitor{internal, external}
	p := New("desk", []OutputConfig{
		{Key: internal.HardwareKey(), Name: internal.Name, Enabled: true, Scale: 1, Width: 2880, Height: 1800},
		{Key: external.HardwareKey(), Name: external.Name, Enabled: true, Scale: 1, Width: 3840, Height: 2160},
	})

	adjusted, state := ApplyClosedLidPolicy(p, monitors)

	if !state.Applied {
		t.Fatal("expected closed-lid policy to apply")
	}
	if len(state.DisabledOutputNames) != 1 || state.DisabledOutputNames[0] != "eDP-1" {
		t.Fatalf("expected eDP-1 to be reported disabled, got %#v", state.DisabledOutputNames)
	}
	internalOut, ok := adjusted.OutputByKey(internal.HardwareKey())
	if !ok {
		t.Fatal("expected adjusted profile to keep internal output")
	}
	if internalOut.Enabled {
		t.Fatal("expected internal output to be disabled")
	}

	originalOut, ok := p.OutputByKey(internal.HardwareKey())
	if !ok || !originalOut.Enabled {
		t.Fatal("expected original profile to remain unchanged")
	}
}

func TestApplyClosedLidPolicyRetargetsManualWorkspaceRules(t *testing.T) {
	internal := hypr.Monitor{Name: "eDP-1", Make: "Samsung", Model: "Panel", Serial: "I1"}
	external := hypr.Monitor{Name: "DP-1", Make: "Dell", Model: "U2720Q", Serial: "E1", Width: 3840, Height: 2160, DPMSStatus: true}
	monitors := []hypr.Monitor{internal, external}
	p := New("desk", []OutputConfig{
		{Key: internal.HardwareKey(), Name: internal.Name, Enabled: true, Scale: 1},
		{Key: external.HardwareKey(), Name: external.Name, Enabled: true, Scale: 1},
	})
	p.Workspaces = WorkspaceSettings{
		Enabled:  true,
		Strategy: WorkspaceStrategyManual,
		Rules: []WorkspaceRule{
			{Workspace: "1", OutputKey: external.HardwareKey(), OutputName: external.Name, Default: true, Persistent: true},
			{Workspace: "2", OutputKey: internal.HardwareKey(), OutputName: internal.Name, Default: true, Persistent: true},
		},
	}

	adjusted, state := ApplyClosedLidPolicy(p, monitors)

	if state.RetargetedWorkspaces != 1 {
		t.Fatalf("expected one workspace retarget, got %d", state.RetargetedWorkspaces)
	}
	rules := ResolveWorkspaceRules(adjusted, monitors)
	if len(rules) != 2 {
		t.Fatalf("expected two workspace rules, got %d", len(rules))
	}
	for _, rule := range rules {
		if rule.OutputKey != external.HardwareKey() {
			t.Fatalf("expected workspace %s to target external output, got %#v", rule.Workspace, rule)
		}
	}
}

func TestApplyClosedLidPolicyAddsMissingInternalOutputDisable(t *testing.T) {
	internal := hypr.Monitor{Name: "eDP-1", Make: "Samsung", Model: "Panel", Serial: "I1"}
	external := hypr.Monitor{Name: "DP-1", Make: "Dell", Model: "U2720Q", Serial: "E1", Width: 3840, Height: 2160, DPMSStatus: true}
	monitors := []hypr.Monitor{internal, external}
	p := New("external-only", []OutputConfig{
		{Key: external.HardwareKey(), Name: external.Name, Enabled: true, Scale: 1},
	})

	adjusted, state := ApplyClosedLidPolicy(p, monitors)

	if !state.Applied {
		t.Fatal("expected closed-lid policy to apply")
	}
	output, ok := adjusted.OutputByKey(internal.HardwareKey())
	if !ok {
		t.Fatal("expected missing internal output to be added")
	}
	if output.Enabled {
		t.Fatal("expected added internal output to be disabled")
	}
}

func TestApplyClosedLidPolicyGeneratedRulesUseExternalOutputsOnly(t *testing.T) {
	internal := hypr.Monitor{Name: "eDP-1", Make: "Samsung", Model: "Panel", Serial: "I1"}
	external := hypr.Monitor{Name: "DP-1", Make: "Dell", Model: "U2720Q", Serial: "E1", Width: 3840, Height: 2160, DPMSStatus: true}
	monitors := []hypr.Monitor{internal, external}
	p := New("desk", []OutputConfig{
		{Key: internal.HardwareKey(), Name: internal.Name, Enabled: true, Scale: 1},
		{Key: external.HardwareKey(), Name: external.Name, Enabled: true, Scale: 1},
	})
	p.Workspaces = WorkspaceSettings{
		Enabled:       true,
		Strategy:      WorkspaceStrategySequential,
		MaxWorkspaces: 4,
		GroupSize:     2,
		MonitorOrder:  []string{internal.HardwareKey(), external.HardwareKey()},
	}

	adjusted, _ := ApplyClosedLidPolicy(p, monitors)
	rules := ResolveWorkspaceRules(adjusted, monitors)

	if len(rules) != 4 {
		t.Fatalf("expected four workspace rules, got %d", len(rules))
	}
	for _, rule := range rules {
		if rule.OutputKey != external.HardwareKey() {
			t.Fatalf("expected generated workspace %s to target external output, got %#v", rule.Workspace, rule)
		}
	}
}

func TestApplyClosedLidPolicyKeepsInternalOutputWhenNoExternalMonitorExists(t *testing.T) {
	internal := hypr.Monitor{Name: "eDP-1", Make: "Samsung", Model: "Panel", Serial: "I1"}
	p := New("mobile", []OutputConfig{
		{Key: internal.HardwareKey(), Name: internal.Name, Enabled: true, Scale: 1},
	})

	adjusted, state := ApplyClosedLidPolicy(p, []hypr.Monitor{internal})

	if state.Applied {
		t.Fatal("expected closed-lid policy to be skipped without an external monitor")
	}
	output, ok := adjusted.OutputByKey(internal.HardwareKey())
	if !ok || !output.Enabled {
		t.Fatal("expected internal output to remain enabled")
	}
}

func TestClosedLidDoesNotTrustUnusableOrTargetDisabledExternal(t *testing.T) {
	internal := hypr.Monitor{Name: "eDP-1", Width: 1920, Height: 1080, Scale: 1, DPMSStatus: true}
	for _, tc := range []struct {
		name      string
		external  hypr.Monitor
		targetOff bool
	}{
		{"modeless", hypr.Monitor{Name: "DP-1", DPMSStatus: true}, false},
		{"sleeping", hypr.Monitor{Name: "DP-1", Width: 1920, Height: 1080}, false},
		{"disabled", hypr.Monitor{Name: "DP-1", Width: 1920, Height: 1080, DPMSStatus: true, Disabled: true}, false},
		{"synthetic", hypr.Monitor{Name: "FALLBACK", Width: 1920, Height: 1080, DPMSStatus: true}, false},
		{"target off", hypr.Monitor{Name: "DP-1", Width: 1920, Height: 1080, DPMSStatus: true}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			monitors := []hypr.Monitor{internal, tc.external}
			p := FromMonitors("desk", monitors)
			for i := range p.Outputs {
				if p.Outputs[i].Key == tc.external.HardwareKey() {
					p.Outputs[i].Enabled = !tc.targetOff
				}
			}
			got, adjustment := ApplyClosedLidPolicy(p, monitors)
			out, _ := got.OutputByKey(internal.HardwareKey())
			if !out.Enabled || adjustment.Applied {
				t.Fatalf("disabled the working internal screen: %+v", got)
			}
		})
	}
}

// Issue #67: with the built-in panel off, this laptop's HDMI came back from a
// power cycle without a mode and stayed dark until the panel was switched on.
func issue67Setup(externalWidth int) (Profile, []hypr.Monitor) {
	internal := hypr.Monitor{Name: "eDP-1", Make: "BOE", Model: "Panel", Serial: "I1", Disabled: true,
		AvailableModes: []string{"1920x1080@60.00Hz", "1920x1080@144.00Hz"}}
	external := hypr.Monitor{Name: "HDMI-A-1", Make: "HKC", Model: "27E1QA", Serial: "E1",
		Width: externalWidth, Height: externalWidth * 9 / 16, DPMSStatus: true}
	p := New("casa", []OutputConfig{
		{Key: internal.HardwareKey(), Name: internal.Name, Enabled: false, Scale: 1.25, Width: 1920, Height: 1080, Refresh: 144},
		{Key: external.HardwareKey(), Name: external.Name, Enabled: true, Scale: 1, Width: 2560, Height: 1440, Refresh: 144},
	})
	return p, []hypr.Monitor{internal, external}
}

func TestKeepBuiltInPanelOnWhenNoExternalShowsAPicture(t *testing.T) {
	p, monitors := issue67Setup(0)
	adjusted, turnedOn := KeepBuiltInPanelOn(p, monitors)
	if len(turnedOn) != 1 || turnedOn[0] != "eDP-1" {
		t.Fatalf("expected eDP-1 to be turned on, got %v", turnedOn)
	}
	panel, _ := adjusted.OutputByKey(monitors[0].HardwareKey())
	external, _ := adjusted.OutputByKey(monitors[1].HardwareKey())
	if !panel.Enabled || panel.Width != 1920 || panel.Scale != 1.25 || panel.X != 2560 {
		t.Fatalf("panel should come on with its saved mode, right of the external: %+v", panel)
	}
	if !external.Enabled || external.Width != 2560 {
		t.Fatalf("the external must stay as saved: %+v", external)
	}
	if original, _ := p.OutputByKey(monitors[0].HardwareKey()); original.Enabled {
		t.Fatal("the saved profile must not change")
	}

	// Hyprland's synthetic FALLBACK head is not a picture either.
	withFallback := append(monitors, hypr.Monitor{Name: "FALLBACK", Width: 1920, Height: 1080, DPMSStatus: true})
	if _, turnedOn := KeepBuiltInPanelOn(p, withFallback); len(turnedOn) != 1 {
		t.Fatal("FALLBACK must not count as a working display")
	}
}

func TestKeepBuiltInPanelOnLeavesAWorkingSetupAlone(t *testing.T) {
	p, monitors := issue67Setup(2560)
	if adjusted, turnedOn := KeepBuiltInPanelOn(p, monitors); turnedOn != nil || adjusted.Outputs[0].Enabled != p.Outputs[0].Enabled {
		t.Fatalf("a usable external keeps the panel off, got %v", turnedOn)
	}
	// A panel the profile already keeps on needs nothing.
	p, monitors = issue67Setup(0)
	for i := range p.Outputs {
		p.Outputs[i].Enabled = true
	}
	if _, turnedOn := KeepBuiltInPanelOn(p, monitors); turnedOn != nil {
		t.Fatalf("panel already on, got %v", turnedOn)
	}
	// A desktop has no built-in panel to turn on.
	p, monitors = issue67Setup(0)
	if _, turnedOn := KeepBuiltInPanelOn(p, monitors[1:]); turnedOn != nil {
		t.Fatalf("no built-in panel, got %v", turnedOn)
	}
}

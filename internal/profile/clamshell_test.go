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

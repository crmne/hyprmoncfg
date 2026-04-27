package daemon

import (
	"testing"

	"github.com/crmne/hyprmoncfg/internal/hypr"
)

func TestInternalOnlyFallbackProfileEnablesInternalWhenAllOutputsDisabled(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "eDP-1", Make: "Framework", Model: "Panel", Serial: "A1", Width: 2880, Height: 1800, RefreshRate: 120, X: 3840, Scale: 1.5, Disabled: true},
	}

	got, ok := internalOnlyFallbackProfile(monitors)
	if !ok {
		t.Fatal("expected fallback profile")
	}
	if len(got.Outputs) != 1 {
		t.Fatalf("expected 1 output, got %d", len(got.Outputs))
	}

	output := got.Outputs[0]
	if !output.Enabled {
		t.Fatal("expected internal output to be enabled")
	}
	if output.Mode != "2880x1800@120.00Hz" || output.Width != 2880 || output.Height != 1800 || output.Refresh != 120 {
		t.Fatalf("unexpected fallback mode: %+v", output)
	}
	if output.X != 0 || output.Y != 0 || output.Scale != 1.5 || output.MirrorOf != "" {
		t.Fatalf("unexpected fallback placement: %+v", output)
	}
	if got.Workspaces.Enabled {
		t.Fatalf("expected fallback workspace settings to be disabled: %+v", got.Workspaces)
	}
}

func TestInternalOnlyFallbackProfileLeavesExternalOutputsDisabled(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "DP-1", Make: "Dell", Model: "U2720Q", Serial: "B1", Disabled: true},
		{Name: "eDP-1", Make: "Framework", Model: "Panel", Serial: "A1", Disabled: true},
	}

	got, ok := internalOnlyFallbackProfile(monitors)
	if !ok {
		t.Fatal("expected fallback profile")
	}

	for _, output := range got.Outputs {
		if output.Name == "DP-1" && output.Enabled {
			t.Fatalf("expected external output to stay disabled: %+v", output)
		}
		if output.Name == "eDP-1" && !output.Enabled {
			t.Fatalf("expected internal output to be enabled: %+v", output)
		}
	}
}

func TestInternalOnlyFallbackProfileDoesNotOverrideEnabledOutput(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "DP-1", Make: "Dell", Model: "U2720Q", Serial: "B1"},
		{Name: "eDP-1", Make: "Framework", Model: "Panel", Serial: "A1", Disabled: true},
	}

	if _, ok := internalOnlyFallbackProfile(monitors); ok {
		t.Fatal("did not expect fallback while an output is enabled")
	}
}

func TestInternalOnlyFallbackProfileRequiresInternalOutput(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "DP-1", Make: "Dell", Model: "U2720Q", Serial: "B1", Disabled: true},
	}

	if _, ok := internalOnlyFallbackProfile(monitors); ok {
		t.Fatal("did not expect fallback without an internal output")
	}
}

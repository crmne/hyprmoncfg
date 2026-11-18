package apply

import (
	"strings"
	"testing"

	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

// Omarchy reconciles per-display workspaces itself; moving them as well
// would fight it.
func TestPerDisplayWorkspacesAreNotMovedWhenOmarchyRunsThem(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "eDP-1", Description: "Laptop panel", Width: 1920, Height: 1200, RefreshRate: 60, Scale: 1},
		{Name: "DP-1", Description: "Desk display", X: 1920, Width: 2560, Height: 1440, RefreshRate: 60, Scale: 1},
	}
	p := profile.FromMonitors("Desk", monitors)
	p.Workspaces = profile.WorkspaceSettings{Enabled: true, Strategy: profile.WorkspaceStrategyPerDisplay}

	own := WorkspaceCommandsForProfile(p, monitors)
	if !strings.Contains(strings.Join(own, "\n"), "moveworkspacetomonitor 11 ") {
		t.Fatalf("hyprmoncfg's own per-display workspaces must be placed: %v", own)
	}

	profile.SetPerDisplayDelegate("omarchy", func(outputs []profile.OutputConfig) map[string]int {
		return map[string]int{outputs[0].Key: 0, outputs[1].Key: 1}
	})
	t.Cleanup(func() { profile.SetPerDisplayDelegate("", nil) })
	if got := WorkspaceCommandsForProfile(p, monitors); len(got) != 0 {
		t.Fatalf("moved Omarchy's workspaces: %v", got)
	}
}

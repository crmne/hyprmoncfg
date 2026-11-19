package profile

import (
	"strconv"
	"testing"

	"github.com/crmne/hyprmoncfg/internal/hypr"
)

func perDisplayProfile() Profile {
	return Profile{
		Name: "Desk",
		Outputs: []OutputConfig{
			{Key: "laptop", Name: "eDP-1", Description: "Laptop panel", Enabled: true, Scale: 1},
			{Key: "desk", Name: "DP-1", Description: "Desk display", Enabled: true, Scale: 1},
			{Key: "copy", Name: "HDMI-A-1", Description: "Mirror", Enabled: true, Scale: 1, MirrorOf: "laptop"},
		},
		Workspaces: WorkspaceSettings{Enabled: true, Strategy: WorkspaceStrategyPerDisplay, MonitorOrder: []string{"desk", "laptop"}},
	}
}

func TestPerDisplayGivesEachDisplayTenWorkspacesInPlannerOrder(t *testing.T) {
	rules := ResolveWorkspaceRules(perDisplayProfile(), nil)
	if len(rules) != 20 {
		t.Fatalf("got %d rules, want 20 (a mirror has no workspaces): %+v", len(rules), rules)
	}
	for i, rule := range rules {
		wantKey := "desk"
		if i >= 10 {
			wantKey = "laptop"
		}
		if rule.Workspace != strconv.Itoa(i+1) || rule.OutputKey != wantKey {
			t.Fatalf("rule %d = %+v, want workspace %d on %s", i, rule, i+1, wantKey)
		}
		first := i%10 == 0
		if rule.Default != first || rule.Persistent != first {
			t.Fatalf("rule %d default=%v persistent=%v, want only each display's first", i, rule.Default, rule.Persistent)
		}
	}
}

func TestPerDisplayPersistAllKeepsEveryWorkspace(t *testing.T) {
	p := perDisplayProfile()
	p.Workspaces.PersistAll = true
	for _, rule := range ResolveWorkspaceRules(p, nil) {
		if !rule.Persistent {
			t.Fatalf("%+v not persistent", rule)
		}
	}
}

func TestPerDisplayDelegateNumbersTheBlocks(t *testing.T) {
	var seen []string
	SetPerDisplayDelegate("omarchy", func(outputs []OutputConfig) map[string]int {
		for _, output := range outputs {
			seen = append(seen, output.Key)
		}
		return map[string]int{"laptop": 0, "desk": 2}
	})
	t.Cleanup(func() { SetPerDisplayDelegate("", nil) })

	p := perDisplayProfile()
	if !DelegatesWorkspaces(p) {
		t.Fatal("per-display with a delegate must be delegated")
	}
	rules := ResolveWorkspaceRules(p, nil)
	if len(seen) != 2 || seen[0] != "desk" || seen[1] != "laptop" {
		t.Fatalf("delegate saw %v, want enabled unmirrored outputs in planner order", seen)
	}
	if rules[0].Workspace != "1" || rules[0].OutputKey != "laptop" || rules[10].Workspace != "21" || rules[10].OutputKey != "desk" {
		t.Fatalf("delegate blocks not used: %+v", rules)
	}

	own := ResolveOwnWorkspaceRules(p, nil)
	if own[0].OutputKey != "desk" || own[10].Workspace != "11" {
		t.Fatalf("own rules followed the delegate: %+v", own)
	}

	p.Workspaces.Strategy = WorkspaceStrategySequential
	if DelegatesWorkspaces(p) {
		t.Fatal("only per-display is delegated")
	}
	p.Workspaces.Strategy = WorkspaceStrategyPerDisplay
	p.Workspaces.Enabled = false
	if DelegatesWorkspaces(p) {
		t.Fatal("Off is not delegated through per-display")
	}
}

func TestPerDisplayValidates(t *testing.T) {
	if err := perDisplayProfile().Workspaces.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestPerDisplayWorkspaceReadsTheLoadedRules(t *testing.T) {
	rules := []hypr.WorkspaceRule{
		{WorkspaceString: "1", Monitor: "desc:Laptop panel", Default: true},
		{WorkspaceString: "2", Monitor: "desc:Laptop panel"},
		{WorkspaceString: "11", Monitor: "DP-1", Default: true},
		{WorkspaceString: "5", Monitor: "HDMI-A-1", Default: true},
	}
	for _, tc := range []struct {
		monitor hypr.Monitor
		slot    int
		want    string
	}{
		{hypr.Monitor{Name: "eDP-1", Description: "Laptop panel"}, 3, "3"},
		{hypr.Monitor{Name: "DP-1", Description: "Desk display"}, 3, "13"},
		{hypr.Monitor{Name: "DP-1", Description: "Desk display"}, 10, "20"},
		// A default that does not start a block is not a per-display block.
		{hypr.Monitor{Name: "HDMI-A-1"}, 2, "2"},
		{hypr.Monitor{Name: "DP-9"}, 7, "7"},
	} {
		if got := PerDisplayWorkspace(tc.slot, tc.monitor, rules); got != tc.want {
			t.Fatalf("%s slot %d = %s, want %s", tc.monitor.Name, tc.slot, got, tc.want)
		}
	}
}

// Omarchy runs a delegated per-display plan without workspace rules, so the
// live state, which then has none, must still read back as that profile.
func TestDelegatedPerDisplayProfileMatchesLiveStateWithoutRules(t *testing.T) {
	SetPerDisplayDelegate("omarchy", func(outputs []OutputConfig) map[string]int {
		blocks := map[string]int{}
		for i, output := range outputs {
			blocks[output.Key] = i
		}
		return blocks
	})
	t.Cleanup(func() { SetPerDisplayDelegate("", nil) })

	monitors := []hypr.Monitor{
		{ID: 0, Name: "eDP-1", Description: "Laptop panel", Make: "Acme", Model: "Panel", Width: 1920, Height: 1200, RefreshRate: 60, Scale: 1},
		{ID: 1, Name: "DP-1", Description: "Desk display", Make: "Acme", Model: "Desk", Serial: "1", X: 1920, Width: 2560, Height: 1440, RefreshRate: 60, Scale: 1},
	}
	saved := FromState("Desk", monitors, nil)
	saved.Workspaces = WorkspaceSettings{Enabled: true, Strategy: WorkspaceStrategyPerDisplay}

	draft, source, _ := EditorProfileFromOrientedState([]Profile{saved}, monitors, nil, 0)
	if source != "Desk" || draft.Workspaces.Strategy != WorkspaceStrategyPerDisplay {
		t.Fatalf("live state read back as %q with strategy %q", source, draft.Workspaces.Strategy)
	}
}

func TestPerDisplayCountKeepsBlocksOfTen(t *testing.T) {
	p := perDisplayProfile()
	p.Workspaces.SlotsPerDisplay = 3
	rules := ResolveWorkspaceRules(p, nil)
	var got []string
	for _, rule := range rules {
		got = append(got, rule.OutputKey+":"+rule.Workspace)
	}
	want := []string{"desk:1", "desk:2", "desk:3", "laptop:11", "laptop:12", "laptop:13"}
	if len(got) != len(want) {
		t.Fatalf("rules = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("rules = %v, want %v", got, want)
		}
	}
}

func TestPerDisplayCountMustBeOneToTen(t *testing.T) {
	settings := WorkspaceSettings{Enabled: true, Strategy: WorkspaceStrategyPerDisplay}
	for _, count := range []int{-1, 11} {
		settings.SlotsPerDisplay = count
		if settings.Validate() == nil {
			t.Fatalf("%d per display accepted", count)
		}
	}
	for count, want := range map[int]int{0: 10, 1: 1, 10: 10} {
		settings.SlotsPerDisplay = count
		if err := settings.Validate(); err != nil {
			t.Fatalf("%d per display: %v", count, err)
		}
		if got := settings.PerDisplaySlots(); got != want {
			t.Fatalf("PerDisplaySlots() with %d = %d, want %d", count, got, want)
		}
	}
}

// Where Omarchy already gives each display its own workspaces, new and
// extended profiles start there instead of turning it off for a global plan.
func TestDefaultStrategyFollowsTheSystem(t *testing.T) {
	monitors := []hypr.Monitor{
		{Name: "eDP-1", Description: "Laptop panel", Width: 2880, Height: 1800, RefreshRate: 120, Scale: 2},
		{Name: "DP-1", Description: "Desk display", X: 1440, Width: 2560, Height: 1440, RefreshRate: 60, Scale: 1},
	}
	if got := DefaultWorkspaceStrategy(); got != WorkspaceStrategySequential {
		t.Fatalf("default without Omarchy = %q", got)
	}
	if got := WorkspaceSettingsFromHypr(monitors, nil); got.Enabled || got.Strategy != WorkspaceStrategySequential {
		t.Fatalf("live state without rules or Omarchy = %+v, want Off with Sequential ready", got)
	}
	laptop := FromMonitors("Laptop", monitors[:1])
	if got := ExtendConnected(laptop, monitors).Workspaces; !got.Enabled || got.Strategy != WorkspaceStrategySequential {
		t.Fatalf("extended without Omarchy = %+v", got)
	}

	SetPerDisplayDelegate("omarchy", func([]OutputConfig) map[string]int { return nil })
	t.Cleanup(func() { SetPerDisplayDelegate("", nil) })
	if got := DefaultWorkspaceStrategy(); got != WorkspaceStrategyPerDisplay {
		t.Fatalf("default with Omarchy = %q", got)
	}
	if got := WorkspaceSettingsFromHypr(monitors, nil); !got.Enabled || got.Strategy != WorkspaceStrategyPerDisplay {
		t.Fatalf("live state under Omarchy = %+v, want Per display", got)
	}
	laptop = FromMonitors("Laptop", monitors[:1])
	laptop.Workspaces = WorkspaceSettings{}
	if got := ExtendConnected(laptop, monitors).Workspaces; !got.Enabled || got.Strategy != WorkspaceStrategyPerDisplay {
		t.Fatalf("extended under Omarchy = %+v, want Per display", got)
	}
}

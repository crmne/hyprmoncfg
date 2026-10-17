package daemon

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

func onWorkspace(m hypr.Monitor, ws string) hypr.Monitor {
	m.ActiveWorkspace = hypr.Workspace{Name: ws}
	return m
}

func plannedDesk() (profile.Profile, hypr.Monitor, hypr.Monitor) {
	p, lg, msi := fallbackDesk()
	p.Workspaces = profile.WorkspaceSettings{Enabled: true, Strategy: profile.WorkspaceStrategySequential,
		GroupSize: 3, MaxWorkspaces: 6, MonitorOrder: []string{msi.HardwareKey(), lg.HardwareKey()}}
	return p, lg, msi
}

// The MSI shows workspace 2, goes off and comes back on Hyprland's default,
// workspace 1. It gets workspace 2 back, once.
func TestWorkspaceMemoryGivesAReturningDisplayItsLastWorkspace(t *testing.T) {
	p, lg, msi := plannedDesk()
	m := newWorkspaceMemory()
	m.observe([]hypr.Monitor{onWorkspace(lg, "4"), onWorkspace(msi, "2")})
	m.observe([]hypr.Monitor{onWorkspace(lg, "2")})
	m.observe([]hypr.Monitor{onWorkspace(lg, "4"), onWorkspace(msi, "1")})
	m.observe([]hypr.Monitor{onWorkspace(lg, "4"), onWorkspace(msi, "1")})
	got := m.due(p, []hypr.Monitor{onWorkspace(lg, "4"), onWorkspace(msi, "1")})
	if len(got) != 1 || got[0].monitor.Name != "DP-2" || got[0].workspace != "2" {
		t.Fatalf("restores = %+v, want workspace 2 on DP-2", got)
	}
	if again := m.due(p, []hypr.Monitor{onWorkspace(lg, "4"), onWorkspace(msi, "1")}); len(again) != 0 {
		t.Fatalf("restored twice: %+v", again)
	}
	// From now on the MSI's own switches are remembered again.
	m.observe([]hypr.Monitor{onWorkspace(lg, "4"), onWorkspace(msi, "3")})
	if m.last[msi.HardwareKey()] != "3" {
		t.Fatalf("remembered %q, want 3", m.last[msi.HardwareKey()])
	}
}

func TestWorkspaceMemoryWaitsUntilTheReturningDisplayWorks(t *testing.T) {
	p, lg, msi := plannedDesk()
	m := newWorkspaceMemory()
	m.observe([]hypr.Monitor{onWorkspace(lg, "4"), onWorkspace(msi, "2")})
	m.observe([]hypr.Monitor{onWorkspace(lg, "4")})
	stuck := onWorkspace(msi, "1")
	stuck.Width, stuck.Height = 0, 0
	m.observe([]hypr.Monitor{onWorkspace(lg, "4"), stuck})
	if got := m.due(p, []hypr.Monitor{onWorkspace(lg, "4"), stuck}); len(got) != 0 {
		t.Fatalf("restored onto a display without a mode: %+v", got)
	}
	if got := m.due(p, []hypr.Monitor{onWorkspace(lg, "4"), onWorkspace(msi, "1")}); len(got) != 1 || got[0].workspace != "2" {
		t.Fatalf("restores after recovery = %+v, want workspace 2", got)
	}
}

func TestWorkspaceMemoryLeavesWorkspacesOtherDisplaysOwnOrShow(t *testing.T) {
	p, lg, msi := plannedDesk()
	if assigned := assignedWorkspaceMonitors(p, []hypr.Monitor{lg, msi}); assigned["2"] != "DP-2" || assigned["5"] != "DP-1" {
		t.Fatalf("assigned = %v", assigned)
	}
	for name, tc := range map[string]struct {
		last string
		lgOn string
	}{
		"assigned to the LG": {"5", "4"},
		"shown on the LG":    {"7", "7"},
	} {
		t.Run(name, func(t *testing.T) {
			m := newWorkspaceMemory()
			m.observe([]hypr.Monitor{onWorkspace(lg, "4"), onWorkspace(msi, tc.last)})
			m.observe([]hypr.Monitor{onWorkspace(lg, tc.lgOn)})
			back := []hypr.Monitor{onWorkspace(lg, tc.lgOn), onWorkspace(msi, "1")}
			m.observe(back)
			if got := m.due(p, back); len(got) != 0 {
				t.Fatalf("restores = %+v, want none", got)
			}
		})
	}
}

func TestWorkspaceMemoryIgnoresIdenticalDisplays(t *testing.T) {
	p, lg, _ := plannedDesk()
	twin := lg
	twin.Name = "DP-3"
	m := newWorkspaceMemory()
	m.observe([]hypr.Monitor{onWorkspace(lg, "4"), onWorkspace(twin, "5")})
	m.observe([]hypr.Monitor{onWorkspace(lg, "4")})
	back := []hypr.Monitor{onWorkspace(lg, "4"), onWorkspace(twin, "1")}
	m.observe(back)
	if got := m.due(p, back); len(got) != 0 {
		t.Fatalf("restores = %+v, want none for displays that can't be told apart", got)
	}
}

func TestWorkspaceRestoreCommandsPutFocusAndCursorBack(t *testing.T) {
	_, _, msi := plannedDesk()
	restores := []workspaceRestore{{monitor: msi, workspace: "2"}}
	workspaces := []hypr.WorkspaceState{{Name: "2", Monitor: "DP-1"}, {Name: "1", Monitor: "DP-2"}}
	cursor := &[2]int{7000, 1800}

	lua, _ := workspaceRestoreCommands(restores, workspaces, "DP-1", cursor, true)
	if want := []string{
		`dispatch hl.dsp.workspace.move({ workspace = "2", monitor = "DP-2" })`,
		`dispatch hl.dsp.focus({ monitor = "DP-2" })`,
		`dispatch hl.dsp.focus({ workspace = "2" })`,
		`dispatch hl.dsp.focus({ monitor = "DP-1" })`,
		`dispatch hl.dsp.cursor.move({ x = 7000, y = 1800 })`,
	}; !reflect.DeepEqual(lua, want) {
		t.Fatalf("lua commands:\n%s", strings.Join(lua, "\n"))
	}

	legacy, _ := workspaceRestoreCommands(restores, workspaces[1:], "DP-1", nil, false)
	if want := []string{"dispatch focusmonitor DP-2", "dispatch workspace 2", "dispatch focusmonitor DP-1"}; !reflect.DeepEqual(legacy, want) {
		t.Fatalf("legacy commands:\n%s", strings.Join(legacy, "\n"))
	}

	unsafe := []workspaceRestore{{monitor: msi, workspace: "name:a; dispatch exec x"}}
	if commands, done := workspaceRestoreCommands(unsafe, nil, "DP-1", nil, true); len(commands) != 0 || len(done) != 0 {
		t.Fatalf("an unsafe workspace name was dispatched: %v", commands)
	}
}

// End to end: the MSI was on workspace 2, turns off and on again, and the
// daemon shows workspace 2 on it after applying the layout.
func TestRunShowsAReturningDisplayItsLastWorkspace(t *testing.T) {
	connected := listenMonitorEvents(t)
	_, lg, msi := fallbackDesk()
	lg.Scale, msi.Scale, lg.X = 1, 1, 3840
	lg, msi = onWorkspace(lg, "4"), onWorkspace(msi, "2")
	env := newRunTestEnv(t, []hypr.Monitor{lg, msi}, func(s *Service) {
		p, err := s.store.Load("Home")
		if err != nil {
			t.Fatal(err)
		}
		p.Workspaces = profile.WorkspaceSettings{Enabled: true, Strategy: profile.WorkspaceStrategySequential,
			GroupSize: 3, MaxWorkspaces: 6, MonitorOrder: []string{msi.HardwareKey(), lg.HardwareKey()}}
		if err := s.store.Save(p); err != nil {
			t.Fatal(err)
		}
	})
	defer env.stop()
	defer func() {
		if t.Failed() {
			t.Logf("daemon logs:\n%s", env.logs.all())
		}
	}()
	send := connected()
	waitFor(t, 2*time.Second, func() bool { return env.logs.contains("applied profile: Home") }, "startup apply")

	if err := writeMonitorState(env.monitorStatePath, []hypr.Monitor{onWorkspace(lg, "2")}); err != nil {
		t.Fatal(err)
	}
	send("monitorremovedv2>>2,DP-2,Microstep MPG321UR-QD")
	waitFor(t, 3*time.Second, func() bool { return strings.Count(env.logs.all(), "applied profile: Home") >= 2 }, "apply without the MSI")

	focusedLG := onWorkspace(lg, "4")
	focusedLG.Focused = true
	if err := writeMonitorState(env.monitorStatePath, []hypr.Monitor{focusedLG, onWorkspace(msi, "1")}); err != nil {
		t.Fatal(err)
	}
	send("monitoraddedv2>>2,DP-2,Microstep MPG321UR-QD")
	waitFor(t, 3*time.Second, func() bool { return env.logs.contains("showing workspace 2 on DP-2 again") }, "workspace restored")
	if hyprctlLogContains(env.logPath, "moveworkspacetomonitor 2 DP-1") {
		t.Fatal("workspace 2 belongs to the MSI")
	}
	if !hyprctlLogContains(env.logPath, "dispatch workspace 2") || !hyprctlLogContains(env.logPath, "dispatch focusmonitor DP-1") {
		t.Fatal("restore commands missing from the hyprctl log")
	}
}

func TestRunLeavesWorkspacesAloneWithoutWorkspacePlanning(t *testing.T) {
	connected := listenMonitorEvents(t)
	_, lg, msi := fallbackDesk()
	lg.Scale, msi.Scale, lg.X = 1, 1, 3840
	lg, msi = onWorkspace(lg, "4"), onWorkspace(msi, "2")
	env := newRunTestEnv(t, []hypr.Monitor{lg, msi})
	defer env.stop()
	send := connected()
	waitFor(t, 2*time.Second, func() bool { return env.logs.contains("applied profile: Home") }, "startup apply")
	if err := writeMonitorState(env.monitorStatePath, []hypr.Monitor{onWorkspace(lg, "2")}); err != nil {
		t.Fatal(err)
	}
	send("monitorremovedv2>>2,DP-2,Microstep MPG321UR-QD")
	waitFor(t, 3*time.Second, func() bool { return strings.Count(env.logs.all(), "applied profile: Home") >= 2 }, "apply without the MSI")
	if err := writeMonitorState(env.monitorStatePath, []hypr.Monitor{onWorkspace(lg, "4"), onWorkspace(msi, "1")}); err != nil {
		t.Fatal(err)
	}
	send("monitoraddedv2>>2,DP-2,Microstep MPG321UR-QD")
	waitFor(t, 3*time.Second, func() bool { return strings.Count(env.logs.all(), "applied profile: Home") >= 3 }, "apply with the MSI back")
	if env.logs.contains("showing workspace") || hyprctlLogContains(env.logPath, "dispatch workspace") {
		t.Fatalf("touched workspaces without workspace planning:\n%s", env.logs.all())
	}
}

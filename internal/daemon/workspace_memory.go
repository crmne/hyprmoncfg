package daemon

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

// workspaceMemory remembers the workspace each display last showed, by
// hardware identity, so a display that comes back shows it again. Hyprland
// moves a disconnected display's workspaces elsewhere and gives it a default
// one when it returns.
type workspaceMemory struct {
	mu sync.Mutex
	// last is the workspace each display showed when last seen working.
	last map[string]string
	// connected holds the displays in the latest readback.
	connected map[string]bool
	// returning holds displays that connected since and have not had their
	// workspace back yet. Their readback is Hyprland's default, not a
	// choice, so it is not remembered.
	returning map[string]bool
}

func newWorkspaceMemory() *workspaceMemory {
	return &workspaceMemory{last: map[string]string{}, connected: map[string]bool{}, returning: map[string]bool{}}
}

// rememberableKeys maps each display that can be told apart to its key.
func rememberableKeys(monitors []hypr.Monitor) map[string]hypr.Monitor {
	counts := map[string]int{}
	for _, monitor := range monitors {
		if monitor.Name != "FALLBACK" {
			counts[monitor.HardwareKey()]++
		}
	}
	keys := map[string]hypr.Monitor{}
	for _, monitor := range monitors {
		if key := monitor.HardwareKey(); monitor.Name != "FALLBACK" && key != "" && counts[key] == 1 {
			keys[key] = monitor
		}
	}
	return keys
}

func showsWorkspace(monitor hypr.Monitor) bool {
	name := monitor.ActiveWorkspace.Name
	return !monitor.Disabled && monitor.Width > 0 && monitor.Height > 0 &&
		name != "" && !strings.HasPrefix(name, "special:")
}

// observe records a monitor readback.
func (m *workspaceMemory) observe(monitors []hypr.Monitor) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	keys := rememberableKeys(monitors)
	initial := len(m.connected) == 0 && len(m.last) == 0
	for key, monitor := range keys {
		if !m.connected[key] && !initial {
			m.returning[key] = true
		}
		if !m.returning[key] && showsWorkspace(monitor) {
			m.last[key] = monitor.ActiveWorkspace.Name
		}
	}
	for key := range m.returning {
		if _, ok := keys[key]; !ok {
			delete(m.returning, key)
		}
	}
	m.connected = map[string]bool{}
	for key := range keys {
		m.connected[key] = true
	}
}

type workspaceRestore struct {
	monitor   hypr.Monitor
	workspace string
}

// due lists the returning displays that now work and the workspace each
// showed before, and stops waiting for them. A workspace another display is
// showing, or that the profile assigns to another display, stays put.
func (m *workspaceMemory) due(p profile.Profile, monitors []hypr.Monitor) []workspaceRestore {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	keys := rememberableKeys(monitors)
	shown := map[string]string{}
	for _, monitor := range monitors {
		if showsWorkspace(monitor) {
			shown[monitor.ActiveWorkspace.Name] = monitor.Name
		}
	}
	assigned := assignedWorkspaceMonitors(p, monitors)
	var restores []workspaceRestore
	for key := range m.returning {
		monitor, ok := keys[key]
		if !ok || !showsWorkspace(monitor) {
			continue
		}
		delete(m.returning, key)
		want := m.last[key]
		if want == "" || want == monitor.ActiveWorkspace.Name {
			continue
		}
		if on, ok := shown[want]; ok && on != monitor.Name {
			continue
		}
		if on, ok := assigned[want]; ok && on != monitor.Name {
			continue
		}
		restores = append(restores, workspaceRestore{monitor: monitor, workspace: want})
	}
	return restores
}

// assignedWorkspaceMonitors maps each workspace the profile places to the
// connector of its display.
func assignedWorkspaceMonitors(p profile.Profile, monitors []hypr.Monitor) map[string]string {
	if p.Name == "" {
		return nil
	}
	p = profile.ExtendConnected(p, monitors)
	p.Normalize()
	resolver := profile.NewMonitorResolver(monitors)
	assigned := map[string]string{}
	for _, rule := range profile.ResolveWorkspaceRules(p, monitors) {
		output, ok := p.OutputByKey(rule.OutputKey)
		if !ok {
			output = profile.OutputConfig{Key: rule.OutputKey, Name: rule.OutputName}
		}
		monitor, ok := resolver.ResolveOutput(output)
		if !ok {
			monitor, ok = resolver.Resolve(output.MatchIdentity(), rule.OutputName)
		}
		if ok {
			assigned[rule.Workspace] = monitor.Name
		}
	}
	return assigned
}

// workspaceRestoreCommands shows each workspace on its display, then puts
// focus and the cursor back where they were: showing a workspace on a
// display focuses it. A restore whose names cannot be dispatched safely is
// skipped.
func workspaceRestoreCommands(restores []workspaceRestore, workspaces []hypr.WorkspaceState, focused string, cursor *[2]int, luaDispatch bool) ([]string, []workspaceRestore) {
	var commands []string
	var done []workspaceRestore
	for _, restore := range restores {
		target, ws := restore.monitor.Name, restore.workspace
		if !luaDispatch && strings.ContainsAny(ws, " \t") {
			continue
		}
		var next []string
		for _, existing := range workspaces {
			if existing.Name == ws && existing.Monitor != "" && existing.Monitor != target {
				next = append(next, workspaceDispatch(luaDispatch,
					fmt.Sprintf("hl.dsp.workspace.move({ workspace = %q, monitor = %q })", ws, target),
					"moveworkspacetomonitor "+ws+" "+target))
			}
		}
		next = append(next,
			workspaceDispatch(luaDispatch, fmt.Sprintf("hl.dsp.focus({ monitor = %q })", target), "focusmonitor "+target),
			workspaceDispatch(luaDispatch, fmt.Sprintf("hl.dsp.focus({ workspace = %q })", ws), "workspace "+ws))
		if !batchSafe(next) {
			continue
		}
		commands = append(commands, next...)
		done = append(done, restore)
	}
	if len(done) == 0 {
		return nil, nil
	}
	if focused != "" {
		commands = append(commands, workspaceDispatch(luaDispatch, fmt.Sprintf("hl.dsp.focus({ monitor = %q })", focused), "focusmonitor "+focused))
	}
	if cursor != nil {
		commands = append(commands, workspaceDispatch(luaDispatch,
			fmt.Sprintf("hl.dsp.cursor.move({ x = %d, y = %d })", cursor[0], cursor[1]),
			fmt.Sprintf("movecursor %d %d", cursor[0], cursor[1])))
	}
	return commands, done
}

func workspaceDispatch(luaDispatch bool, lua, legacy string) string {
	if luaDispatch {
		return "dispatch " + lua
	}
	return "dispatch " + legacy
}

// restoreWorkspaces gives each display that came back the workspace it
// showed before. It runs after a successful apply, which has already moved
// the profile's workspaces, and only for profiles that plan workspaces.
func (s *Service) restoreWorkspaces(ctx context.Context, p profile.Profile, monitors []hypr.Monitor) {
	restores := s.workspaces.due(p, monitors)
	// With workspace planning off, workspaces belong to the user or to a
	// plugin such as hyprland-virtual-desktops, which restores its own.
	if !p.Workspaces.Enabled || len(restores) == 0 {
		return
	}
	workspaces, err := s.client.Workspaces(ctx)
	if err != nil {
		s.cfg.Logf("restore workspaces: %v", err)
		return
	}
	focused := ""
	for _, monitor := range monitors {
		if monitor.Focused {
			focused = monitor.Name
		}
	}
	var cursor *[2]int
	if x, y, err := s.client.CursorPosition(ctx); err == nil {
		cursor = &[2]int{x, y}
	}
	commands, done := workspaceRestoreCommands(restores, workspaces, focused, cursor, s.luaDialect.Load())
	if len(done) == 0 {
		return
	}
	if err := s.client.Batch(ctx, commands); err != nil {
		s.cfg.Logf("restore workspaces: %v", err)
		return
	}
	for _, restore := range done {
		s.cfg.Logf("showing workspace %s on %s again", restore.workspace, restore.monitor.Name)
	}
}

func batchSafe(commands []string) bool {
	for _, command := range commands {
		if !hypr.BatchSafe(command) {
			return false
		}
	}
	return true
}

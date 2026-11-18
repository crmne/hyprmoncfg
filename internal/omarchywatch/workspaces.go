package omarchywatch

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/crmne/hyprmoncfg/internal/profile"
)

// Omarchy 4.5 and later give each monitor workspaces 1-10 of its own in
// default/hypr/monitor-workspaces.lua. It keeps each physical monitor's block
// in a state file, omarchy/monitor-workspaces under XDG_STATE_HOME in 4.5 and
// omarchy/monitors/workspaces after it, one line per monitor ever seen:
//
//	<block>\t<connector while plugged in>\t<identity>
//
// The identity is the monitor's Hyprland description (make, model and
// serial), with " #2" and so on for monitors that report the same one.
// hyprmoncfg only reads this file. Omarchy writes it.
const perMonitorWorkspacesModule = "default/hypr/monitor-workspaces.lua"

// PerMonitorWorkspaces is Omarchy's per-monitor workspace manager, found on
// this system.
type PerMonitorWorkspaces struct {
	// statePaths are where Omarchy versions keep the blocks, newest first.
	statePaths []string
}

// DetectPerMonitorWorkspaces finds Omarchy's per-monitor workspace module,
// or returns nil on systems without it, such as Omarchy before 4.5 or any
// other distribution.
func DetectPerMonitorWorkspaces() *PerMonitorWorkspaces {
	home, _ := os.UserHomeDir()
	return detectPerMonitorWorkspaces(home, os.Getenv)
}

func detectPerMonitorWorkspaces(home string, getenv func(string) string) *PerMonitorWorkspaces {
	envOr := func(name, fallback string) string {
		if value := getenv(name); value != "" {
			return value
		}
		return fallback
	}
	roots := []string{envOr("OMARCHY_PATH", "/usr/share/omarchy")}
	if home != "" {
		roots = append(roots, filepath.Join(home, ".local/share/omarchy"))
	}
	for _, root := range roots {
		if _, err := os.Stat(filepath.Join(root, perMonitorWorkspacesModule)); err == nil {
			state := envOr("XDG_STATE_HOME", filepath.Join(home, ".local/state"))
			return &PerMonitorWorkspaces{statePaths: []string{
				filepath.Join(state, "omarchy/monitors/workspaces"),
				filepath.Join(state, "omarchy/monitor-workspaces"),
			}}
		}
	}
	return nil
}

// Name is how the interfaces refer to the delegate.
func (w *PerMonitorWorkspaces) Name() string { return "omarchy" }

type savedBlock struct {
	block     int
	connector string
	identity  string
}

// read loads the newest state file there is. Omarchy's migration moves the
// 4.5 file to the newer place, so only one is current.
func (w *PerMonitorWorkspaces) read() []savedBlock {
	var file *os.File
	for _, path := range w.statePaths {
		if opened, err := os.Open(path); err == nil {
			file = opened
			break
		}
	}
	if file == nil {
		return nil
	}
	defer file.Close()

	var saved []savedBlock
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.SplitN(scanner.Text(), "\t", 3)
		if len(fields) != 3 || fields[2] == "" {
			continue
		}
		block, err := strconv.Atoi(fields[0])
		if err != nil || block < 0 {
			continue
		}
		saved = append(saved, savedBlock{block: block, connector: fields[1], identity: fields[2]})
	}
	return saved
}

var duplicateSuffix = regexp.MustCompile(` #\d+$`)

func internalConnector(name string) bool {
	return strings.HasPrefix(name, "eDP") || strings.HasPrefix(name, "LVDS") || strings.HasPrefix(name, "DSI")
}

// Blocks predicts the block Omarchy gives each display, the way its module
// assigns them: a monitor it has seen keeps its saved block, and a new one
// takes the lowest free block, the laptop panel first. It neither writes the
// state file nor needs the displays to be connected.
func (w *PerMonitorWorkspaces) Blocks(outputs []profile.OutputConfig) map[string]int {
	saved := w.read()
	blocks := make(map[string]int, len(saved))
	connectorOf := make(map[string]string, len(saved))
	for _, entry := range saved {
		blocks[entry.identity] = entry.block
		if entry.connector != "" {
			connectorOf[entry.connector] = entry.identity
		}
	}

	described := func(output profile.OutputConfig) string {
		if description := strings.TrimSpace(output.Description); description != "" {
			return description
		}
		return output.Name
	}

	// Identities, as Omarchy names them: the one recorded for the connector
	// when it still describes the same monitor, otherwise the description,
	// numbered for monitors that report the same one, in connector order.
	identities := make(map[string]string, len(outputs))
	taken := make(map[string]bool, len(outputs))
	for _, output := range outputs {
		identity, ok := connectorOf[output.Name]
		if ok && !taken[identity] && duplicateSuffix.ReplaceAllString(identity, "") == described(output) {
			identities[output.Key] = identity
			taken[identity] = true
		}
	}
	byName := append([]profile.OutputConfig(nil), outputs...)
	sort.SliceStable(byName, func(i, j int) bool { return byName[i].Name < byName[j].Name })
	for _, output := range byName {
		if _, ok := identities[output.Key]; ok {
			continue
		}
		identity := described(output)
		for count := 2; taken[identity]; count++ {
			identity = described(output) + " #" + strconv.Itoa(count)
		}
		identities[output.Key] = identity
		taken[identity] = true
	}

	// The laptop panel claims a block first; the rest follow in order.
	ordered := append([]profile.OutputConfig(nil), outputs...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return internalConnector(ordered[i].Name) && !internalConnector(ordered[j].Name)
	})
	used := make(map[int]bool, len(blocks))
	for _, block := range blocks {
		used[block] = true
	}
	result := make(map[string]int, len(outputs))
	for _, output := range ordered {
		identity := identities[output.Key]
		block, ok := blocks[identity]
		if !ok {
			for block = 0; used[block]; block++ {
			}
			used[block] = true
			blocks[identity] = block
		}
		result[output.Key] = block
	}
	return result
}

// UsePerMonitorWorkspaces hands the per-display workspace strategy to
// Omarchy when its module is installed, so hyprmoncfg and Omarchy do not both
// place workspaces. It reports whether Omarchy took it.
func UsePerMonitorWorkspaces() bool {
	workspaces := DetectPerMonitorWorkspaces()
	if workspaces == nil {
		profile.SetPerDisplayDelegate("", nil)
		return false
	}
	profile.SetPerDisplayDelegate(workspaces.Name(), workspaces.Blocks)
	return true
}

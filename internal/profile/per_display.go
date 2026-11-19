package profile

import (
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/crmne/hyprmoncfg/internal/hypr"
)

// DisplayBlocks assigns each display a block of PerDisplayBlockSize workspaces.
// It receives the enabled, unmirrored outputs in planner order and returns
// each output key's block, counting from zero: block 1 holds workspaces
// 11-20. An output it leaves out has no workspaces of its own.
type DisplayBlocks func(outputs []OutputConfig) map[string]int

var (
	delegateMu     sync.RWMutex
	delegateName   string
	delegateBlocks DisplayBlocks
)

// SetPerDisplayDelegate records that something other than hyprmoncfg runs
// per-display workspaces, as Omarchy 4.5 and later do, and how it numbers
// them. hyprmoncfg then writes no workspace rules or key bindings for the
// per-display strategy, and the plan it shows comes from blocks. An empty
// name clears the delegate.
func SetPerDisplayDelegate(name string, blocks DisplayBlocks) {
	delegateMu.Lock()
	defer delegateMu.Unlock()
	if name == "" {
		blocks = nil
	}
	delegateName, delegateBlocks = name, blocks
}

// PerDisplayDelegate names what runs per-display workspaces instead of
// hyprmoncfg, or returns "" when hyprmoncfg runs them itself.
func PerDisplayDelegate() string {
	delegateMu.RLock()
	defer delegateMu.RUnlock()
	return delegateName
}

func perDisplayDelegateBlocks() DisplayBlocks {
	delegateMu.RLock()
	defer delegateMu.RUnlock()
	return delegateBlocks
}

// DelegatesWorkspaces reports whether a profile's workspaces are left to the
// per-display delegate, so hyprmoncfg must not place them.
func DelegatesWorkspaces(p Profile) bool {
	return p.Workspaces.Enabled && p.Workspaces.Strategy == WorkspaceStrategyPerDisplay && PerDisplayDelegate() != ""
}

// ResolveOwnWorkspaceRules resolves a plan the way hyprmoncfg itself carries
// it out, ignoring a per-display delegate. The generated config uses it for
// the branch that runs when the delegate turns out to be missing.
func ResolveOwnWorkspaceRules(p Profile, monitors []hypr.Monitor) []WorkspaceRule {
	if p.Workspaces.Enabled && p.Workspaces.Strategy == WorkspaceStrategyPerDisplay {
		p.normalizeIdentityRefs()
		return perDisplayRules(p, OwnDisplayBlocks(p, monitors))
	}
	return ResolveWorkspaceRules(p, monitors)
}

// OwnDisplayBlocks numbers displays in the planner's monitor order: the
// first gets 1-10, the second 11-20.
func OwnDisplayBlocks(p Profile, monitors []hypr.Monitor) map[string]int {
	blocks := make(map[string]int)
	for index, key := range orderedOutputKeys(p, monitors) {
		blocks[key] = index
	}
	return blocks
}

func perDisplayWorkspaceRules(p Profile, monitors []hypr.Monitor) []WorkspaceRule {
	return perDisplayRules(p, PerDisplayBlocks(p, monitors))
}

// PerDisplayBlocks is each display's block under the per-display strategy,
// by output key: the delegate's, such as Omarchy's, when there is one, and
// otherwise the planner's order.
func PerDisplayBlocks(p Profile, monitors []hypr.Monitor) map[string]int {
	p.normalizeIdentityRefs()
	if source := perDisplayDelegateBlocks(); source != nil {
		order := orderedOutputKeys(p, monitors)
		outputs := make([]OutputConfig, 0, len(order))
		for _, key := range order {
			if output, ok := p.OutputByKey(key); ok {
				outputs = append(outputs, output)
			}
		}
		return source(outputs)
	}
	return OwnDisplayBlocks(p, monitors)
}

// perDisplayRules lays out each display's workspaces, the first slots of its
// block. The first is the display's default and persistent, so every display
// shows its own on start.
func perDisplayRules(p Profile, blocks map[string]int) []WorkspaceRule {
	type entry struct {
		key   string
		block int
	}
	entries := make([]entry, 0, len(blocks))
	for _, key := range orderedOutputKeys(p, nil) {
		if block, ok := blocks[key]; ok && block >= 0 {
			entries = append(entries, entry{key: key, block: block})
		}
	}
	// Lowest block first, so rules come out in workspace order.
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].block < entries[j].block })

	slots := p.Workspaces.PerDisplaySlots()
	rules := make([]WorkspaceRule, 0, len(entries)*slots)
	for _, item := range entries {
		output, _ := p.OutputByKey(item.key)
		for slot := 1; slot <= slots; slot++ {
			rules = append(rules, WorkspaceRule{
				Workspace:  strconv.Itoa(item.block*PerDisplayBlockSize + slot),
				OutputKey:  item.key,
				OutputName: output.Name,
				Default:    slot == 1,
				Persistent: slot == 1 || p.Workspaces.PersistAll,
			})
		}
	}
	return rules
}

// PerDisplayWorkspace is the workspace for a slot, 1-10, on a display, read
// from the workspace rules Hyprland has loaded: the display's block is the
// one whose first workspace the rules make its default. A display with no
// block of its own gets the plain slot number.
func PerDisplayWorkspace(slot int, monitor hypr.Monitor, rules []hypr.WorkspaceRule) string {
	selectors := map[string]bool{monitor.Name: true}
	if description := strings.TrimSpace(monitor.Description); description != "" {
		selectors["desc:"+description] = true
	}
	for _, rule := range rules {
		if !rule.Default || !selectors[strings.TrimSpace(rule.Monitor)] {
			continue
		}
		id, err := strconv.Atoi(strings.TrimSpace(rule.WorkspaceString))
		if err != nil || id < 1 || (id-1)%PerDisplayBlockSize != 0 {
			continue
		}
		return strconv.Itoa(id - 1 + slot)
	}
	return strconv.Itoa(slot)
}

// DefaultWorkspaceStrategy is the plan a profile starts with: Per display
// where the system already gives each display its own workspaces, as
// Omarchy 4.5 and later do, so hyprmoncfg never turns that off on its own,
// and Sequential everywhere else.
func DefaultWorkspaceStrategy() WorkspaceStrategy {
	if PerDisplayDelegate() != "" {
		return WorkspaceStrategyPerDisplay
	}
	return WorkspaceStrategySequential
}

// WorkspaceStrategies lists the strategies profiles can store, in the order
// editors offer them.
func WorkspaceStrategies() []WorkspaceStrategy {
	return []WorkspaceStrategy{
		WorkspaceStrategyManual,
		WorkspaceStrategySequential,
		WorkspaceStrategyInterleave,
		WorkspaceStrategyPerDisplay,
	}
}

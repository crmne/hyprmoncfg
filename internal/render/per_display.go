package render

import (
	_ "embed"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/crmne/hyprmoncfg/internal/profile"
)

//go:embed per_display.lua
var luaPerDisplay string

// perDisplayBlocks maps each display's monitor selector to its block of ten
// workspaces, collected from the resolved per-display rules.
type perDisplayBlocks map[string]int

func (b perDisplayBlocks) add(rule profile.WorkspaceRule, selector string) {
	if !rule.Default {
		return
	}
	id, err := strconv.Atoi(strings.TrimSpace(rule.Workspace))
	if err != nil || id < 1 {
		return
	}
	b[selector] = (id - 1) / profile.PerDisplayBlockSize
}

func (b perDisplayBlocks) lua() string {
	selectors := make([]string, 0, len(b))
	for selector := range b {
		selectors = append(selectors, selector)
	}
	sort.Slice(selectors, func(i, j int) bool {
		if b[selectors[i]] != b[selectors[j]] {
			return b[selectors[i]] < b[selectors[j]]
		}
		return selectors[i] < selectors[j]
	})
	if len(selectors) == 0 {
		return "{}"
	}
	entries := make([]string, 0, len(selectors))
	for _, selector := range selectors {
		entries = append(entries, fmt.Sprintf("[%s] = %d", luaQuote(selector), b[selector]))
	}
	return "{ " + strings.Join(entries, ", ") + " }"
}

// luaPerDisplaySection hands per-display workspaces to Omarchy when its
// module is loaded, and otherwise runs them here: each display's block of
// rules, and Super + number keys that go to the focused display's slot.
func luaPerDisplaySection(slots int, blocks perDisplayBlocks, workspaceLines []string) string {
	rules := "  -- No display is connected that the plan places."
	if len(workspaceLines) > 0 {
		indented := make([]string, len(workspaceLines))
		for i, line := range workspaceLines {
			indented[i] = "  " + line
		}
		rules = strings.Join(indented, "\n")
	}
	section := strings.TrimSpace(luaPerDisplay)
	section = strings.Replace(section, "10 -- hyprmoncfg:slots", strconv.Itoa(slots), 1)
	section = strings.Replace(section, "{} -- hyprmoncfg:blocks", blocks.lua(), 1)
	section = strings.Replace(section, "  -- hyprmoncfg:rules", rules, 1)
	return section
}

// hyprlangPerDisplayBindings points Super + number at the focused display's
// own workspaces. Hyprlang cannot work out the display itself, so the keys
// run hyprmoncfg, which reads the loaded workspace rules. Both the key symbol
// and the key code are unbound, so an existing binding of either does not
// also fire. Keys past the display's workspaces do nothing.
func hyprlangPerDisplayBindings(slots int) string {
	lines := []string{
		fmt.Sprintf("# hyprmoncfg gives each display workspaces 1-%d of its own. Super + number", slots),
		"# goes to that workspace on the focused display, Super + Shift + number",
		"# moves the focused window there, and Super + Shift + Alt + number moves",
		"# it without following.",
	}
	actions := []struct{ mods, action string }{
		{"SUPER", "focus"},
		{"SUPER SHIFT", "move"},
		{"SUPER SHIFT ALT", "move-silent"},
	}
	for slot := 1; slot <= profile.PerDisplayBlockSize; slot++ {
		key := strconv.Itoa(slot % 10)
		code := fmt.Sprintf("code:%d", slot+9)
		for _, item := range actions {
			lines = append(lines, "unbind = "+item.mods+", "+key, "unbind = "+item.mods+", "+code)
			if slot <= slots {
				lines = append(lines, fmt.Sprintf("bind = %s, %s, exec, hyprmoncfg workspace %s %d", item.mods, code, item.action, slot))
			}
		}
	}
	return strings.Join(lines, "\n")
}

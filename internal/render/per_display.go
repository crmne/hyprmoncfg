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
	b[selector] = (id - 1) / profile.PerDisplaySlots
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
func luaPerDisplaySection(blocks perDisplayBlocks, workspaceLines []string) string {
	rules := "  -- No display is connected that the plan places."
	if len(workspaceLines) > 0 {
		indented := make([]string, len(workspaceLines))
		for i, line := range workspaceLines {
			indented[i] = "  " + line
		}
		rules = strings.Join(indented, "\n")
	}
	section := strings.TrimSpace(luaPerDisplay)
	section = strings.Replace(section, "{} -- hyprmoncfg:blocks", blocks.lua(), 1)
	section = strings.Replace(section, "  -- hyprmoncfg:rules", rules, 1)
	return section
}

// hyprlangPerDisplayBindings points Super + number at the focused display's
// own workspaces. Hyprlang cannot work out the display itself, so the keys
// run hyprmoncfg, which reads the loaded workspace rules. Both the key symbol
// and the key code are unbound, so an existing binding of either does not
// also fire.
func hyprlangPerDisplayBindings() string {
	lines := []string{
		"# hyprmoncfg gives each display workspaces 1-10 of its own. Super + number",
		"# goes to that workspace on the focused display, and Super + Shift + number",
		"# moves the focused window there.",
	}
	for slot := 1; slot <= profile.PerDisplaySlots; slot++ {
		key := strconv.Itoa(slot % 10)
		code := fmt.Sprintf("code:%d", slot+9)
		lines = append(lines,
			"unbind = SUPER, "+key,
			"unbind = SUPER, "+code,
			fmt.Sprintf("bind = SUPER, %s, exec, hyprmoncfg workspace focus %d", code, slot),
			"unbind = SUPER SHIFT, "+key,
			"unbind = SUPER SHIFT, "+code,
			fmt.Sprintf("bind = SUPER SHIFT, %s, exec, hyprmoncfg workspace move %d", code, slot),
		)
	}
	return strings.Join(lines, "\n")
}

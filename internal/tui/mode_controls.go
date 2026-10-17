package tui

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/crmne/hyprmoncfg/internal/hypr"
)

// The Display tab edits a mode as two controls, like Omarchy's own panel:
// Resolution (field 1, a list of the display's resolutions) and Refresh
// (fieldRefresh, a choice row of the rates that resolution offers). Both
// edit the one stored mode string; only presentation is split.
const fieldRefresh = 21

// resolutionKey is the resolution part of a mode, "3840x2160". A mode that
// does not parse (such as "preferred") is its own resolution.
func resolutionKey(mode string) string {
	width, height, _, ok := hypr.ParseMode(mode)
	if !ok {
		return strings.TrimSpace(mode)
	}
	return fmt.Sprintf("%dx%d", width, height)
}

// resolutionChoices lists each resolution in modes once, best first: the
// most pixels, then the wider one. Modes that do not parse keep their order
// at the end.
func resolutionChoices(modes []string) []string {
	type resolution struct {
		key           string
		width, height int
		parsed        bool
		order         int
	}
	seen := map[string]bool{}
	var list []resolution
	for _, mode := range modes {
		key := resolutionKey(mode)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		width, height, _, ok := hypr.ParseMode(mode)
		list = append(list, resolution{key, width, height, ok, len(list)})
	}
	sort.SliceStable(list, func(i, j int) bool {
		a, b := list[i], list[j]
		if a.parsed != b.parsed {
			return a.parsed
		}
		if !a.parsed {
			return a.order < b.order
		}
		if a.width*a.height != b.width*b.height {
			return a.width*a.height > b.width*b.height
		}
		return a.width > b.width
	})
	keys := make([]string, len(list))
	for idx, item := range list {
		keys[idx] = item.key
	}
	return keys
}

// refreshChoices returns the indexes into modes of the rates the resolution
// offers, slowest first, each rate once (the first mode listing it wins, and
// the current mode is first in an editor's list).
func refreshChoices(modes []string, resolution string) []int {
	var indexes []int
	seen := map[string]bool{}
	for idx, mode := range modes {
		if resolutionKey(mode) != resolution {
			continue
		}
		_, _, refresh, _ := hypr.ParseMode(mode)
		rate := strconv.FormatFloat(refresh, 'f', 2, 64)
		if seen[rate] {
			continue
		}
		seen[rate] = true
		indexes = append(indexes, idx)
	}
	sort.SliceStable(indexes, func(i, j int) bool {
		return modeRefresh(modes[indexes[i]]) < modeRefresh(modes[indexes[j]])
	})
	return indexes
}

func modeRefresh(mode string) float64 {
	_, _, refresh, _ := hypr.ParseMode(mode)
	return refresh
}

// sameRefresh treats rates that print alike at two decimals as one rate.
func sameRefresh(a, b float64) bool {
	return math.Abs(a-b) < 0.005
}

// modeForResolution picks the mode a resolution change selects: the
// previous rate when the new resolution offers it, otherwise its highest.
func modeForResolution(modes []string, resolution string, previous float64) int {
	choices := refreshChoices(modes, resolution)
	if len(choices) == 0 {
		return -1
	}
	for _, idx := range choices {
		if sameRefresh(modeRefresh(modes[idx]), previous) {
			return idx
		}
	}
	return choices[len(choices)-1]
}

// refreshLabels prints rates as the cards do, "144Hz", with the
// precision that tells it apart from its neighbours in labels.
func refreshLabels(rates []float64) []string {
	labels := make([]string, len(rates))
	for precision := 1; precision <= 3; precision++ {
		seen := map[string]int{}
		for idx, rate := range rates {
			labels[idx] = refreshLabel(rate, precision)
			seen[labels[idx]]++
		}
		distinct := true
		for _, count := range seen {
			if count > 1 {
				distinct = false
			}
		}
		if distinct {
			break
		}
	}
	return labels
}

func refreshLabel(rate float64, precision int) string {
	if rate <= 0 {
		return "Auto"
	}
	return displayNumber(rate, precision) + "Hz"
}

// refreshRow is the Refresh row for the output's current
// resolution: mode indexes, and their labels.
func (o editableOutput) refreshRow() ([]int, []string) {
	indexes := refreshChoices(o.Modes, resolutionKey(o.DisplayMode()))
	rates := make([]float64, len(indexes))
	for idx, mode := range indexes {
		rates[idx] = modeRefresh(o.Modes[mode])
	}
	return indexes, refreshLabels(rates)
}

// selectMode makes modes[index] the output's mode.
func (o *editableOutput) selectMode(index int) {
	if index < 0 || index >= len(o.Modes) {
		return
	}
	o.ModeIndex = index
	if o.ModeUnsupported && index > 0 {
		o.ModeUnsupported = false
	}
	o.applyMode(o.Modes[index])
}

// selectResolution switches the output to a resolution, keeping its rate
// when the resolution offers it.
func (o *editableOutput) selectResolution(resolution string) bool {
	index := modeForResolution(o.Modes, resolution, o.Refresh)
	if index < 0 || index == o.ModeIndex {
		return false
	}
	o.selectMode(index)
	return true
}

// stepResolution moves to the neighbouring resolution in the list, wrapping
// like the list's own arrows.
func (o *editableOutput) stepResolution(delta int) {
	choices := resolutionChoices(o.Modes)
	if len(choices) == 0 {
		return
	}
	current := resolutionKey(o.DisplayMode())
	pos := 0
	for idx, key := range choices {
		if key == current {
			pos = idx
			break
		}
	}
	o.selectResolution(choices[wrapIndex(pos+delta, len(choices))])
}

// stepRefresh moves along the Refresh row; arrows stop at the ends and
// Enter (wrap) advances past the last rate to the first.
func (o *editableOutput) stepRefresh(delta int, wrap bool) {
	indexes, _ := o.refreshRow()
	if len(indexes) == 0 {
		return
	}
	pos := 0
	for idx, mode := range indexes {
		if mode == o.ModeIndex {
			pos = idx
			break
		}
	}
	next := clampInt(pos+delta, 0, len(indexes)-1)
	if wrap {
		next = wrapIndex(pos+delta, len(indexes))
	}
	if indexes[next] != o.ModeIndex {
		o.selectMode(indexes[next])
	}
}

// renderRefreshRow draws the rates of the current resolution as pills.
func (m Model) renderRefreshRow(output editableOutput, width int, focused, wrap bool) ([]string, []choiceSpan) {
	indexes, labels := output.refreshRow()
	if len(indexes) == 0 || width < 8 {
		return nil, nil
	}
	pills := make([]rowPill, len(indexes))
	selected := 0
	for idx, mode := range indexes {
		label := " " + labels[idx] + " "
		style := m.styles.subtle
		if mode == output.ModeIndex {
			selected = idx
			style = m.selectedPillStyle(focused, false)
		}
		pills[idx] = rowPill{label, style.Render(label), strconv.Itoa(mode)}
	}
	return m.layoutPillRow(pills, selected, width, wrap)
}

// openResolutionPicker lists the display's resolutions, best first.
func (m *Model) openResolutionPicker() {
	output := m.editOutputs[m.selectedOutput]
	choices := resolutionChoices(output.Modes)
	if len(choices) == 0 {
		return
	}
	current := resolutionKey(output.DisplayMode())
	selected := 0
	for idx, key := range choices {
		if key == current {
			selected = idx
		}
	}
	m.openLabeledPicker(fmt.Sprintf("Resolution for %s", output.Name), -1, choices, choices, selected)
}

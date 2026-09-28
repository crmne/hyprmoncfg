package tui

import (
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/crmne/hyprmoncfg/internal/scaling"
)

// Closed sets of two or three values read better as a choice row than as a
// picker: every option stays visible and the selection is one keypress or
// one click away. Larger sets (mode, rotation, color space, mirror) keep
// their pickers. The wire values are the ones layoutFieldValue reports.
func inspectorChoiceValues(field int) []string {
	switch field {
	case 0:
		return []string{"on", "off"}
	case 3:
		return []string{"8", "10"}
	case 5:
		return []string{"off", "on", "fullscreen"}
	case 14:
		return []string{"default", "gamma22", "srgb"}
	case 18, 19:
		return []string{"off", "auto", "on"}
	default:
		return nil
	}
}

// choiceSpan is one option's column range within its inspector line.
type choiceSpan struct {
	start, end int
	value      string
	// line is the span's line within its field, for rows that wrap.
	line int
}

// renderChoiceRow draws the options side by side and returns their column
// spans relative to the value column. It reports false when they do not fit,
// in which case the caller shows the selected value alone.
func (m Model) renderChoiceRow(field int, current string, width int, focused bool) (string, []choiceSpan, bool) {
	values := inspectorChoiceValues(field)
	if len(values) == 0 {
		return "", nil, false
	}
	total := 0
	for idx, value := range values {
		if idx > 0 {
			total++
		}
		total += lipgloss.Width(fieldOptionLabel(field, value)) + 2
	}
	if total > width {
		return "", nil, false
	}

	parts := make([]string, 0, len(values)*2)
	spans := make([]choiceSpan, 0, len(values))
	cursor := 0
	for idx, value := range values {
		if idx > 0 {
			parts = append(parts, " ")
			cursor++
		}
		label := " " + fieldOptionLabel(field, value) + " "
		style := m.styles.subtle
		if value == current {
			style = withBG(withFG(lipgloss.NewStyle().Bold(true), m.styles.palette.chipFg), m.styles.palette.chipBg)
			if focused {
				style = m.styles.focused.UnsetPadding()
			}
		}
		parts = append(parts, style.Render(label))
		spans = append(spans, choiceSpan{start: cursor, end: cursor + lipgloss.Width(label), value: value})
		cursor += lipgloss.Width(label)
	}
	return strings.Join(parts, ""), spans, true
}

// setInspectorChoice applies one option of a choice row to the selected
// display, with the same reflow as any other edit.
func (m *Model) setInspectorChoice(field int, value string) {
	if len(m.editOutputs) == 0 {
		return
	}
	output := &m.editOutputs[m.selectedOutput]
	if field == 2 {
		scale, err := strconv.ParseFloat(value, 64)
		if err != nil || scaling.Round(scale) == scaling.Round(output.Scale) {
			return
		}
		m.guardLayoutEdit(func() {
			output := &m.editOutputs[m.selectedOutput]
			oldWidth, oldHeight := output.logicalSize()
			output.Scale = scale
			m.reflowAfterResize(m.selectedOutput, oldWidth, oldHeight)
			m.layoutChanged()
		})
		return
	}
	if m.layoutFieldValue(*output, field) == value {
		return
	}
	m.guardLayoutEdit(func() {
		output := &m.editOutputs[m.selectedOutput]
		oldWidth, oldHeight := output.logicalSize()
		if field == 0 {
			output.Enabled = value == "on"
		} else {
			m.applyFieldPickerValue(output, field, value)
		}
		m.reflowAfterResize(m.selectedOutput, oldWidth, oldHeight)
		m.layoutChanged()
	})
}

// nextSharpScale steps to the neighbouring entry of the full scale list, the
// one More… opens and the daemon serves the panel (scaling.Choices), stopping
// at the ends like Omarchy's scale pills. Typed entry still accepts any scale.
func nextSharpScale(width, height int, current float64, delta int) float64 {
	options := scaling.Choices(width, height, current)
	if len(options) == 0 || delta == 0 {
		return scaling.Round(clampFloat(current+float64(delta)*0.05, scaling.MinScale, scaling.MaxScale))
	}
	pos := scaleChoiceIndex(options, current)
	return options[clampInt(pos+delta, 0, len(options)-1)]
}

func scaleChoiceIndex(options []float64, current float64) int {
	current = scaling.Round(current)
	best, bestDistance := 0, math.Inf(1)
	for idx, option := range options {
		if d := math.Abs(option - current); d < bestDistance {
			best, bestDistance = idx, d
		}
	}
	return best
}

// scaleChoiceLabels formats the Scale row like the cards (1.33x). Two scales
// that would read the same get the precision that tells them apart.
func scaleChoiceLabels(options []float64) []string {
	labels := make([]string, len(options))
	seen := make(map[string]int, len(options))
	for idx, option := range options {
		labels[idx] = displayNumber(option, 2) + "x"
		seen[labels[idx]]++
	}
	for idx, option := range options {
		if seen[labels[idx]] > 1 {
			labels[idx] = scaling.Format(option) + "x"
		}
	}
	return labels
}

const scaleMoreValue = "more"

// scaleRowChoices is what the Scale row shows: Omarchy's presets mapped onto
// this mode (scaling.PresetChoices) and, when the current scale is not one
// of them, the current scale itself, exactly as saved.
func scaleRowChoices(output editableOutput) []float64 {
	choices := scaling.PresetChoices(output.Width, output.Height)
	if output.Scale <= 0 {
		return choices
	}
	current := scaling.Round(output.Scale)
	for _, choice := range choices {
		if choice == current {
			return choices
		}
	}
	choices = append(choices, current)
	sort.Float64s(choices)
	return choices
}

// renderScaleRow draws Scale like Omarchy's scale pills: the preset row for
// this display, the current scale as its own pill when it is not a preset
// (marked ⚠ when it is not sharp), and More… for the full sharp list. Wide
// inspectors wrap the pills; compact ones keep one line and slide a window
// around the selected pill, with ‹ and › marking more.
func (m Model) renderScaleRow(output editableOutput, width int, focused, wrap bool) ([]string, []choiceSpan) {
	options := scaleRowChoices(output)
	if len(options) == 0 || width < 8 {
		return nil, nil
	}
	labels := scaleChoiceLabels(options)
	current := scaling.Round(output.Scale)
	selected := scaleChoiceIndex(options, output.Scale)
	sharp := scaling.Sharp(output.Width, output.Height, output.Scale)
	count := len(options) + 1 // the last pill is More…
	pill := func(idx int) (string, string, string) {
		if idx == len(options) {
			label := " More… "
			return label, m.styles.value.Render(label), scaleMoreValue
		}
		label := " " + labels[idx] + " "
		style := m.styles.subtle
		if options[idx] == current && !sharp {
			label = " " + labels[idx] + " ⚠ "
			style = m.styles.warning
		}
		if idx == selected {
			fg := m.styles.palette.chipFg
			if !sharp {
				fg = m.styles.palette.warning
			}
			style = withBG(withFG(lipgloss.NewStyle().Bold(true), fg), m.styles.palette.chipBg)
			if focused {
				style = m.styles.focused.UnsetPadding()
			}
		}
		return label, style.Render(label), strconv.FormatFloat(options[idx], 'f', -1, 64)
	}

	var lines []string
	var spans []choiceSpan
	if wrap {
		line, cursor := "", 0
		for idx := 0; idx < count; idx++ {
			plain, styled, value := pill(idx)
			w := lipgloss.Width(plain)
			if cursor > 0 && cursor+1+w > width {
				lines = append(lines, line)
				line, cursor = "", 0
			}
			if cursor > 0 {
				line += " "
				cursor++
			}
			spans = append(spans, choiceSpan{start: cursor, end: cursor + w, value: value, line: len(lines)})
			line += styled
			cursor += w
		}
		lines = append(lines, line)
		if !sharp {
			// Say why the saved scale is marked; it stays selectable as is.
			lines = append(lines, m.styles.warning.Render("⚠ fractional px"))
		}
		return lines, spans
	}

	// One line: grow a window around the selected pill until it is full.
	widthOf := func(idx int) int { plain, _, _ := pill(idx); return lipgloss.Width(plain) }
	first, last := selected, selected
	used := widthOf(selected)
	for {
		grew := false
		markers := 0
		if first > 0 {
			markers += 2
		}
		if last < count-1 {
			markers += 2
		}
		if last < count-1 && used+1+widthOf(last+1)+markers <= width {
			last++
			used += 1 + widthOf(last)
			grew = true
		}
		if first > 0 && used+1+widthOf(first-1)+markers <= width {
			first--
			used += 1 + widthOf(first)
			grew = true
		}
		if !grew {
			break
		}
	}
	line, cursor := "", 0
	if first > 0 {
		line, cursor = m.styles.subtle.Render("‹ "), 2
	}
	for idx := first; idx <= last; idx++ {
		if idx > first {
			line += " "
			cursor++
		}
		plain, styled, value := pill(idx)
		spans = append(spans, choiceSpan{start: cursor, end: cursor + lipgloss.Width(plain), value: value})
		line += styled
		cursor += lipgloss.Width(plain)
	}
	if last < count-1 {
		line += m.styles.subtle.Render(" ›")
	}
	return []string{line}, spans
}

// inlineEntryKind reports the exact-entry editors that edit in place, on the
// inspector row itself, so the stage stays visible while a position is typed.
// Scale keeps its dialog because it explains sharpness; the ICC path and the
// workspace counts keep theirs too.
func inlineEntryKind(kind numericInputKind) bool {
	switch kind {
	case numericInputPositionX, numericInputPositionY, numericInputFloat, numericInputInt:
		return true
	default:
		return false
	}
}

func (m Model) inlineEntryActive(field int) bool {
	return m.mode == modeNumericInput && m.input != nil && m.tab == tabLayout &&
		inlineEntryKind(m.input.Kind) && m.input.FieldIndex == field &&
		m.input.OutputIndex == m.selectedOutput
}

// renderInlineEntry draws the value being typed in place of the value, with
// its unit and any validation message beside it.
func (m Model) renderInlineEntry(width int) string {
	input := m.input.Input
	input.Width = clampInt(width-24, 6, 12)
	box := m.styles.focused.Render(input.View())
	note := m.styles.subtle.Render("px")
	if m.input.Kind == numericInputFloat || m.input.Kind == numericInputInt {
		note = ""
	}
	if input.Err != nil {
		// The row already names the display, so "DP-1 would overlap DP-2"
		// reads as "would overlap DP-2" beside it.
		text := input.Err.Error()
		if m.input.OutputIndex >= 0 && m.input.OutputIndex < len(m.editOutputs) {
			text = strings.TrimPrefix(text, m.editOutputs[m.input.OutputIndex].Name+" ")
		}
		note = m.styles.statusError.Render(fitString(text, max(1, width-lipgloss.Width(box)-1)))
	}
	if note == "" {
		return box
	}
	return box + " " + note
}

// inspectorTabLabels are the Display and Color tabs drawn in the controls
// pane's top border. Each label is a pill with one cell of padding.
var inspectorTabLabels = []string{"Display", "Color"}

func (m Model) renderInspectorTabs() (string, int) {
	parts := make([]string, 0, len(inspectorTabLabels))
	width := 0
	for idx, label := range inspectorTabLabels {
		style := m.styles.subtle
		if int(m.inspectorTab) == idx {
			style = withBG(withFG(lipgloss.NewStyle().Bold(true), m.styles.palette.tabActiveFg), m.styles.palette.tabPillBg)
		}
		parts = append(parts, style.Render(" "+label+" "))
		width += lipgloss.Width(label) + 2
	}
	return strings.Join(parts, ""), width
}

// nudgeInspectorPosition moves the selected display one logical pixel from
// the Position X or Position Y row.
func (m *Model) nudgeInspectorPosition(delta int) {
	if len(m.editOutputs) == 0 || !m.canMoveSelectedOutput() {
		return
	}
	output := m.editOutputs[m.selectedOutput]
	switch m.inspectorField {
	case 7:
		m.placeSelected(output.X+delta, output.Y, 0)
	case 8:
		m.placeSelected(output.X, output.Y+delta, 0)
	}
}

// cycleInspectorChoice moves a choice row to its next option, wrapping.
func (m *Model) cycleInspectorChoice(field, delta int) {
	values := inspectorChoiceValues(field)
	if len(values) == 0 || len(m.editOutputs) == 0 {
		return
	}
	current := m.layoutFieldValue(m.editOutputs[m.selectedOutput], field)
	pos := 0
	for idx, value := range values {
		if value == current {
			pos = idx
			break
		}
	}
	m.setInspectorChoice(field, values[wrapIndex(pos+delta, len(values))])
}

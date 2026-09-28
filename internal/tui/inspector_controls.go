package tui

import (
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

// nextSharpScale steps to the neighbouring scale that gives whole logical
// pixels for the current mode, the same set the panel's Scale list offers.
// Typed entry still accepts any scale.
func nextSharpScale(width, height int, current float64, delta int) float64 {
	options := scaling.GridScales(width, height, scaling.MinScale, scaling.MaxScale)
	if len(options) == 0 || delta == 0 {
		return scaling.Round(clampFloat(current+float64(delta)*0.05, scaling.MinScale, scaling.MaxScale))
	}
	current = scaling.Round(current)
	if delta > 0 {
		for _, option := range options {
			if option > current+1e-9 {
				return option
			}
		}
		return options[len(options)-1]
	}
	for idx := len(options) - 1; idx >= 0; idx-- {
		if options[idx] < current-1e-9 {
			return options[idx]
		}
	}
	return options[0]
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

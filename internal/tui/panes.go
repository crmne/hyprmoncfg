package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/crmne/hyprmoncfg/internal/profile"
)

// detailLabelWidth keeps every label/value pane in the app on the same column
// grid as the layout tab's Info pane.
const detailLabelWidth = 10

type detailRow struct {
	label string
	value string
}

// renderDetailRows renders "label   value" rows. A row with an empty label
// continues the row above it, indented under the value column.
func (m Model) renderDetailRows(rows []detailRow) []string {
	lines := make([]string, 0, len(rows))
	labelWidth := detailLabelWidth
	for _, row := range rows {
		labelWidth = max(labelWidth, lipgloss.Width(row.label))
	}
	for _, row := range rows {
		label := m.styles.label.Render(fmt.Sprintf("%-*s", labelWidth, row.label))
		lines = append(lines, label+" "+row.value)
	}
	return lines
}

// staticCardStyle paints a monitor that is simply there, on a canvas where
// nothing is selected or dragged.
func (m Model) staticCardStyle() canvasCardColors {
	p := m.styles.palette
	return canvasCardColors{
		bg:     p.cardBg,
		border: p.cardStaticBorder,
		fg:     p.cardFg,
		muted:  p.cardMuted,
	}
}

// canvasCard is one monitor rectangle on a read-only canvas.
type canvasCard struct {
	colors canvasCardColors
	body   func(maxLines, maxWidth int) []cardLine
}

type monitorCardEmphasis int

const (
	monitorCardLayout monitorCardEmphasis = iota
	monitorCardProfile
	monitorCardWorkspaces
)

// monitorCardLines is the shared monitor-card vocabulary for every tab. The
// emphasis changes only the visual weight of workspace assignments; geometry
// and identity remain available everywhere and collapse in the same order when
// a card is too small.
func (m Model) monitorCardLines(output editableOutput, workspaces []string, emphasis monitorCardEmphasis, maxLines, maxWidth int, colors canvasCardColors, issue, issueFG string) []cardLine {
	if len(workspaces) == 0 || maxLines < 2 {
		return output.cardLinesWithIssue(maxLines, colors.fg, colors.muted, issue, issueFG)
	}

	lines := output.cardLinesWithIssue(maxLines-1, colors.fg, colors.muted, issue, issueFG)
	for _, text := range fitWorkspaceLines(workspaces, maxWidth, 1) {
		lines = append(lines, cardLine{
			text: text,
			fg:   m.styles.palette.paneActiveBorder,
			bold: emphasis != monitorCardLayout,
		})
	}
	return lines
}

// renderStaticCanvas paints outputs on a canvas that cannot be edited. The
// profile and workspace previews use it so every canvas in the app shares the
// layout tab's geometry, grid, and card chrome.
func (m Model) renderStaticCanvas(outputs []editableOutput, width, height int, card func(editableOutput) canvasCard) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	if len(outputs) == 0 {
		return m.styles.subtle.Render("(no displays)")
	}

	layout := canvasLayoutFor(outputs, width, height)
	if !layout.ok {
		return m.styles.subtle.Render("(no enabled displays)")
	}

	grid := m.newCanvasCells(layout.width, layout.height)
	for _, rect := range layout.rects {
		content := card(outputs[rect.index])
		paintCard(grid, rect, false, content.colors, content.body)
	}
	paintCanvasSegments(grid, 0, 1, m.hiddenOutputSegments(outputs, -1, layout.width-2))
	return renderCanvasCells(grid)
}

// profileMatchSummary describes how a saved profile relates to the monitors
// that are connected right now.
type profileMatchSummary struct {
	result profile.MatchResult
	// active means the live state is already exactly this profile.
	active bool
	// recommended means this is the profile the daemon would apply.
	recommended bool
}

func (s profileMatchSummary) matches() bool {
	return s.result.Score > 0
}

func (m Model) profileMatchSummaries() []profileMatchSummary {
	summaries := make([]profileMatchSummary, len(m.profiles))

	activeName := ""
	if active, ok := profile.ExactStateMatch(m.profiles, m.monitors, m.workspaceRules); ok {
		activeName = active.Name
	}
	recommendedName := ""
	if best, _, ok := profile.BestMatch(m.profiles, m.monitors); ok {
		recommendedName = best.Name
	}

	for idx, saved := range m.profiles {
		summaries[idx] = profileMatchSummary{
			result:      profile.EvaluateMatch(saved, m.monitors),
			active:      saved.Name == activeName,
			recommended: saved.Name == recommendedName,
		}
	}
	return summaries
}

// profileEditableOutputs turns a saved profile into editor outputs, filling in
// whatever the connected hardware can tell us about them.
func (m Model) profileEditableOutputs(p profile.Profile) []editableOutput {
	outputs := make([]editableOutput, 0, len(p.Outputs))
	for _, saved := range p.Outputs {
		live, ok := m.findLiveMonitor(saved)
		outputs = append(outputs, editableOutputFromProfile(saved, live, ok))
	}
	return outputs
}

func (m Model) profileConnectedKeys(p profile.Profile) map[string]bool {
	connected := make(map[string]bool, len(p.Outputs))
	for _, saved := range p.Outputs {
		if _, ok := m.findLiveMonitor(saved); ok {
			connected[saved.Key] = true
		}
	}
	return connected
}

const (
	profileTagActive      = "active"
	profileTagRecommended = "best"
)

func (m Model) profileTag(summary profileMatchSummary) (string, lipgloss.Style) {
	switch {
	case summary.active:
		return profileTagActive, m.styles.statusOK
	case summary.recommended:
		return profileTagRecommended, m.styles.warning
	default:
		return "", m.styles.subtle
	}
}

func profileScoreLabel(summary profileMatchSummary) string {
	if !summary.matches() {
		return "–"
	}
	return fmt.Sprintf("%d", summary.result.Score)
}

// The Saved Profiles pane is a table: profile name, a badge for the active or
// recommended profile, and the match score against the connected displays.
const (
	profileListTagWidth        = 6
	profileListScoreWidth      = 5
	profileAutomaticPaneHeight = 3
	// Table header and spacer; actions remain pinned below the scrolling rows.
	profileListHeaderRows = 2
	profileListActionRows = 1
)

// profileListColumns is the shared column geometry of that table, so the
// header, the rows, and mouse hit-testing cannot drift apart.
type profileListColumns struct {
	name  int
	tag   int
	score int
}

// The name column takes whatever the badge and score columns do not need, so
// the match column stays flush with the right edge of the pane.
func (m Model) profileListColumns(width int) profileListColumns {
	cols := profileListColumns{tag: profileListTagWidth, score: profileListScoreWidth}
	available := width - (cols.tag + 1) - (cols.score + 1)
	if available < 12 {
		cols.tag = 0
		available = width - (cols.score + 1)
	}
	if available < 8 {
		cols.score = 0
		available = width
	}
	cols.name = max(1, available)
	return cols
}

const (
	profileListNameHeader  = "Profile"
	profileListScoreHeader = "Match"
)

func (m Model) profileListHeader(cols profileListColumns) string {
	row := fmt.Sprintf("%-*s", cols.name, fitString(profileListNameHeader, cols.name))
	if cols.tag > 0 {
		row += " " + fmt.Sprintf("%*s", cols.tag, "Status")
	}
	if cols.score > 0 {
		row += " " + fmt.Sprintf("%*s", cols.score, fitString(profileListScoreHeader, cols.score))
	}
	return m.styles.subtle.Render(row)
}

func (m Model) profileAutomaticRow(width int) string {
	label := "Enabled"
	state := "off"
	stateStyle := m.styles.warning
	if m.profileAutomatic() {
		state = "on"
		stateStyle = m.styles.statusOK
	}
	if m.profileModePending {
		state = "..."
		stateStyle = m.styles.subtle
	}
	stateWidth := lipgloss.Width(state)
	labelWidth := max(1, width-stateWidth-1)
	return m.styles.value.Render(fmt.Sprintf("%-*s", labelWidth, fitString(label, labelWidth))) + " " + stateStyle.Render(state)
}

// profileListRows renders one row per profile.
func (m Model) profileListRows(summaries []profileMatchSummary, cols profileListColumns) []string {
	if len(m.profiles) == 0 {
		return []string{m.styles.subtle.Render("(none)")}
	}

	rows := make([]string, 0, len(m.profiles))
	for idx, saved := range m.profiles {
		summary := summaries[idx]

		nameStyle := m.styles.value
		if !summary.matches() {
			nameStyle = m.styles.subtle
		}
		if idx == m.selectedProfile {
			nameStyle = m.styles.fieldSelected.Padding(0).Bold(true)
		}

		row := nameStyle.Render(fmt.Sprintf("%-*s", cols.name, fitString(saved.Name, cols.name)))
		if cols.tag > 0 {
			tag, style := m.profileTag(summary)
			row += " " + style.Render(fmt.Sprintf("%*s", cols.tag, tag))
		}
		if cols.score > 0 {
			scoreStyle := m.styles.subtle
			if summary.matches() {
				scoreStyle = m.styles.value
			}
			row += " " + scoreStyle.Render(fmt.Sprintf("%*s", cols.score, profileScoreLabel(summary)))
		}
		rows = append(rows, row)
	}
	return rows
}

// profileListScroll keeps the selected profile visible once the list is longer
// than the pane.
func (m Model) profileListScroll(innerHeight int) int {
	return inspectorScrollOffset(max(1, len(m.profiles)), m.selectedProfile, max(1, innerHeight-profileListHeaderRows-profileListActionRows))
}

// profileMatchVerdict is the headline answer to "does this profile fit the
// displays that are plugged in right now?".
func (m Model) profileMatchVerdict(summary profileMatchSummary) string {
	verdict, style := "No match", m.styles.subtle
	switch {
	case summary.active:
		verdict, style = "Active", m.styles.statusOK
	case summary.recommended:
		verdict, style = "Recommended", m.styles.warning
	case summary.matches():
		verdict, style = "Partial match", m.styles.value
	}

	line := style.Render(verdict)
	if summary.matches() {
		line += m.styles.subtle.Render(" · score ") + m.styles.value.Render(fmt.Sprintf("%d", summary.result.Score))
	}
	return line
}

// profileMatchReasons explains the score as the arithmetic that produced it,
// so a surprising number is never a mystery.
func (m Model) profileMatchReasons(summary profileMatchSummary) []string {
	result := summary.result
	reasons := profile.ExplainMatch(result)
	lines := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		points := ""
		if summary.matches() {
			points = fmt.Sprintf("%+d", reason.Points)
		}
		style := m.styles.subtle
		if reason.Points > 0 {
			style = m.styles.value
		}
		lines = append(lines, fmt.Sprintf("%s %s",
			m.styles.subtle.Render(fmt.Sprintf("%5s", points)),
			style.Render(fmt.Sprintf("%d %s %s", reason.Count, pluralDisplays(reason.Count), matchReasonLabel(reason.Kind))),
		))
	}
	if len(lines) == 0 {
		lines = append(lines, m.styles.subtle.Render("      no displays connected"))
	}
	return lines
}

func matchReasonLabel(kind profile.MatchReasonKind) string {
	switch kind {
	case profile.MatchReasonConnected:
		return "connected"
	case profile.MatchReasonConnectedKeptOff:
		return "connected, kept off"
	case profile.MatchReasonNotConnected:
		return "not connected"
	case profile.MatchReasonNotConnectedKeptOff:
		return "not connected, kept off"
	case profile.MatchReasonConnectedUnknown:
		return "connected, not in profile"
	default:
		return ""
	}
}

func pluralDisplays(count int) string {
	if count == 1 {
		return "display"
	}
	return "displays"
}

// profileDetailRows is the label/value block describing the selected profile.
func (m Model) profileDetailRows(p profile.Profile, summary profileMatchSummary, width int) []detailRow {
	valueWidth := max(4, width-detailLabelWidth-1)

	rows := []detailRow{
		{label: "Name", value: m.styles.value.Bold(true).Render(fitString(p.Name, valueWidth))},
		{label: "Updated", value: m.styles.value.Render(p.UpdatedAt.Local().Format("2006-01-02 15:04"))},
		{label: "Match", value: m.profileMatchVerdict(summary)},
	}
	for _, reason := range m.profileMatchReasons(summary) {
		rows = append(rows, detailRow{value: reason})
	}

	connected := m.profileConnectedKeys(p)
	rows = append(rows, detailRow{label: "Displays", value: m.styles.value.Render(
		fmt.Sprintf("%d saved · %d connected", len(p.Outputs), len(connected)))})

	// The canvas only draws displays this profile turns on and does not mirror,
	// so the rest are spelled out here instead.
	keptOff := make([]string, 0, len(p.Outputs))
	mirrors := make([]string, 0, len(p.Outputs))
	for _, output := range p.Outputs {
		switch {
		case !output.Enabled:
			keptOff = append(keptOff, outputDisplayLabel(output.Key, p.Outputs))
		case output.MirrorOf != "":
			mirrors = append(mirrors, fmt.Sprintf("%s → %s",
				outputDisplayLabel(output.Key, p.Outputs),
				outputDisplayLabel(output.MirrorOf, p.Outputs)))
		}
	}
	rows = append(rows, m.listRows("Kept off", keptOff, valueWidth)...)
	rows = append(rows, m.listRows("Mirrors", mirrors, valueWidth)...)

	planRows := m.workspacePlanRows(m.workspacePlan(p.Outputs, p.Workspaces), p.Outputs, valueWidth)
	for idx, row := range planRows {
		label := ""
		if idx == 0 {
			label = "Workspaces"
		}
		rows = append(rows, detailRow{label: label, value: row})
	}
	if len(planRows) == 0 {
		rows = append(rows, detailRow{label: "Workspaces", value: m.styles.subtle.Render("(not managed)")})
	}
	rows = append(rows, detailRow{value: m.styles.label.Render("Post-apply command")},
		detailRow{value: m.styles.value.Render(fitString("[Edit command] "+blankFallback(strings.TrimSpace(p.Exec), "Not set"), valueWidth))})

	return rows
}

// listRows labels the first entry of a list and indents the rest under it.
func (m Model) listRows(label string, values []string, width int) []detailRow {
	rows := make([]detailRow, 0, len(values))
	for idx, value := range values {
		rowLabel := ""
		if idx == 0 {
			rowLabel = label
		}
		rows = append(rows, detailRow{label: rowLabel, value: m.styles.value.Render(fitString(value, width))})
	}
	return rows
}

// workspacePlan resolves the workspace rules a set of outputs and settings
// would produce, which is what both the plan list and the canvas preview show.
func (m Model) workspacePlan(outputs []profile.OutputConfig, settings profile.WorkspaceSettings) []profile.WorkspaceRule {
	return profile.ResolveWorkspaceRules(profile.Profile{Outputs: outputs, Workspaces: settings}, m.monitors)
}

// workspacePlanRows lays the workspace plan out as an aligned two-column list
// so the monitor names and their workspaces read as a table.
func (m Model) workspacePlanRows(rules []profile.WorkspaceRule, outputs []profile.OutputConfig, width int) []string {
	pairs := workspacePlanPairs(rules, outputs)
	if len(pairs) == 0 {
		return nil
	}

	labelWidth := 0
	for _, pair := range pairs {
		labelWidth = max(labelWidth, lipgloss.Width(pair[0]))
	}
	labelWidth = min(labelWidth, max(8, width/2))

	rows := make([]string, 0, len(pairs))
	for _, pair := range pairs {
		label := fmt.Sprintf("%-*s", labelWidth, fitString(pair[0], labelWidth))
		rows = append(rows, m.styles.label.Render(label)+"  "+
			m.styles.value.Render(fitString(pair[1], max(1, width-labelWidth-2))))
	}
	return rows
}

// workspacePlanPairs pairs each monitor with the workspaces it owns, in the
// order the rules assign them, so the monitor holding workspace 1 comes first.
func workspacePlanPairs(rules []profile.WorkspaceRule, outputs []profile.OutputConfig) [][2]string {
	labels := make([]string, 0, len(outputs))
	values := make([]*strings.Builder, 0, len(outputs))
	position := make(map[string]int, len(outputs))

	for _, rule := range rules {
		key := rule.OutputKey
		if key == "" {
			key = rule.OutputName
		}
		idx, seen := position[key]
		if !seen {
			label := outputDisplayLabel(key, outputs)
			if label == key && rule.OutputName != "" {
				label = rule.OutputName
			}
			labels = append(labels, label)
			values = append(values, &strings.Builder{})
			idx = len(labels) - 1
			position[key] = idx
		}
		if values[idx].Len() > 0 {
			values[idx].WriteString(", ")
		}
		values[idx].WriteString(rule.Workspace)
	}

	pairs := make([][2]string, len(labels))
	for idx := range labels {
		pairs[idx] = [2]string{labels[idx], values[idx].String()}
	}
	return pairs
}

// workspacePlanByConnector groups the plan by the connector each rule targets,
// which is how the canvas finds the workspaces for a monitor card.
func workspacePlanByConnector(rules []profile.WorkspaceRule) map[string][]string {
	plan := make(map[string][]string, len(rules))
	for _, rule := range rules {
		key := rule.OutputName
		if key == "" {
			key = rule.OutputKey
		}
		plan[key] = append(plan[key], rule.Workspace)
	}
	return plan
}

// renderProfileCanvas draws the selected profile's monitor arrangement, marking
// the displays that are not plugged in right now.
func (m Model) renderProfileCanvas(p profile.Profile, width, height int) string {
	outputs := m.profileEditableOutputs(p)
	connected := m.profileConnectedKeys(p)
	workspaces := workspacePlanByConnector(profile.ResolveWorkspaceRules(p, nil))

	return m.renderStaticCanvas(outputs, width, height, func(output editableOutput) canvasCard {
		// Nothing here is being dragged, so a display the profile can drive gets
		// the plain "it is there" border; one the machine cannot find gets the
		// warning border and says why.
		colors := m.staticCardStyle()
		issue := ""
		if !connected[output.Key] {
			issue = "not connected"
			colors.border = m.styles.palette.warning
			colors.fg = m.styles.palette.cardMuted
		}
		return canvasCard{
			colors: colors,
			body: func(maxLines, maxWidth int) []cardLine {
				return m.monitorCardLines(output, workspaces[output.Name], monitorCardProfile,
					maxLines, maxWidth, colors, issue, m.styles.palette.warning)
			},
		}
	})
}

// renderWorkspaceCanvas draws the editor's monitors with the workspaces each
// one would own, so the plan can be read spatially.
func (m Model) renderWorkspaceCanvas(preview map[string][]string, width, height int) string {
	// The display affected by the selected list row takes the accent, whether
	// that row reorders a generated plan or assigns one manual workspace.
	selectedKey := ""
	if idx := m.workspaceEdit.SelectedField - len(workspaceFields); idx >= 0 {
		if m.workspaceEdit.Strategy == profile.WorkspaceStrategyManual && idx < len(m.workspaceEdit.Rules) {
			selectedKey = m.workspaceEdit.Rules[idx].OutputKey
		} else if idx < len(m.workspaceEdit.MonitorOrder) {
			selectedKey = m.workspaceEdit.MonitorOrder[idx]
		}
	}

	return m.renderStaticCanvas(m.editOutputs, width, height, func(output editableOutput) canvasCard {
		colors := m.staticCardStyle()
		if output.Key == selectedKey {
			colors = m.canvasCardStyle(output, true)
		}
		workspaces := preview[output.Name]
		if len(workspaces) == 0 {
			workspaces = preview[output.Key]
		}
		return canvasCard{
			colors: colors,
			body: func(maxLines, maxWidth int) []cardLine {
				return m.monitorCardLines(output, workspaces, monitorCardWorkspaces,
					maxLines, maxWidth, colors, "", m.styles.palette.warning)
			},
		}
	})
}

// fitWorkspaceLines packs workspace ids into at most maxLines rows, marking
// whatever did not fit with a +N overflow tag.
func fitWorkspaceLines(workspaces []string, width, maxLines int) []string {
	if len(workspaces) == 0 || width <= 2 || maxLines <= 0 {
		return nil
	}

	lines := make([]string, 0, maxLines)
	idx := 0
	for idx < len(workspaces) && len(lines) < maxLines {
		last := len(lines) == maxLines-1
		// Rows that continue on the next one keep a column for the comma that
		// says so, so a wrapped list never reads as two separate lists.
		lineWidth := width
		if !last {
			lineWidth = width - 1
		}
		line := ""
		for idx < len(workspaces) {
			candidate := workspaces[idx]
			if line != "" {
				candidate = line + ", " + workspaces[idx]
			}
			overflow := ""
			if last && idx < len(workspaces)-1 {
				overflow = fmt.Sprintf(" +%d", len(workspaces)-idx-1)
			}
			if lipgloss.Width(candidate+overflow) > lineWidth {
				break
			}
			line = candidate
			idx++
		}
		if line == "" {
			if len(lines) == 0 {
				lines = append(lines, fitString(fmt.Sprintf("+%d", len(workspaces)-idx), width))
			}
			break
		}
		if last && idx < len(workspaces) {
			line += fmt.Sprintf(" +%d", len(workspaces)-idx)
			idx = len(workspaces)
		}
		lines = append(lines, line)
	}
	for pos := 0; pos < len(lines)-1; pos++ {
		lines[pos] += ","
	}
	return lines
}

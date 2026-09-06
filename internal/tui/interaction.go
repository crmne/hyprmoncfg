package tui

import (
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/crmne/hyprmoncfg/internal/apply"
	"github.com/crmne/hyprmoncfg/internal/profile"
	"github.com/crmne/hyprmoncfg/internal/scaling"
)

type pickerItem string

func (i pickerItem) FilterValue() string { return string(i) }
func (i pickerItem) Title() string       { return string(i) }
func (i pickerItem) Description() string { return "" }
func (i pickerItem) Value() string       { return string(i) }

type fieldPickerItem struct {
	pickerItem
	label string
}

func (i fieldPickerItem) Title() string { return i.label }

type modePickerState struct {
	OutputIndex int
	FieldIndex  int // -1 for mode picker, >= 0 for field option picker
	List        list.Model
}

type numericInputKind int

const (
	numericInputScale numericInputKind = iota
	numericInputPositionX
	numericInputPositionY
	numericInputICC
	numericInputFloat
	numericInputInt
	numericInputWorkspaceCount
	numericInputWorkspaceGroupSize
)

type numericInputState struct {
	Kind        numericInputKind
	OutputIndex int
	FieldIndex  int
	Title       string
	Hint        string
	Input       textinput.Model
}

type profileExecInputState struct {
	ProfileIndex int
	Title        string
	Input        textinput.Model
	Err          error
}

type profileListItem struct {
	name    string
	updated time.Time
	outputs int
}

func (i profileListItem) FilterValue() string { return i.name }
func (i profileListItem) Title() string       { return i.name }
func (i profileListItem) Description() string {
	if i.updated.IsZero() {
		return fmt.Sprintf("%d outputs", i.outputs)
	}
	return fmt.Sprintf("updated %s  •  %d outputs", i.updated.Local().Format("2006-01-02 15:04"), i.outputs)
}

// arrowDelegate wraps list.DefaultDelegate and prepends a ▸ arrow on the selected item.
type arrowDelegate struct {
	list.DefaultDelegate
}

func (d arrowDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	var buf strings.Builder
	d.DefaultDelegate.Render(&buf, m, index, item)
	isSelected := index == m.Index()
	for i, line := range strings.Split(buf.String(), "\n") {
		if i > 0 {
			fmt.Fprint(w, "\n")
		}
		if i == 0 && isSelected {
			fmt.Fprintf(w, "▸ %s", line)
		} else {
			fmt.Fprintf(w, "  %s", line)
		}
	}
}

type saveDialogState struct {
	Input   textinput.Model
	List    list.Model
	All     []profileListItem
	Filter  string
	Action  saveAction
	Purpose saveDialogPurpose
}

type saveAction int

const (
	saveActionOnly saveAction = iota
	saveActionApply
	saveActionSaveQuit
	saveActionDiscardQuit
	saveActionCancel
)

type saveDialogPurpose int

const (
	saveDialogProfile saveDialogPurpose = iota
	saveDialogQuit
)

type canvasDragState struct {
	OutputIndex int
	LastX       int
	LastY       int
}

type canvasRect struct {
	index int
	x     int
	y     int
	w     int
	h     int
}

type canvasGeometry struct {
	ok      bool
	width   int
	height  int
	scale   float64
	cellW   float64
	offsetX int
	offsetY int
	rects   []canvasRect
}

type hitRect struct {
	x int
	y int
	w int
	h int
}

func (r hitRect) contains(x, y int) bool {
	return x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h
}

func (r hitRect) inner(style lipgloss.Style) hitRect {
	return hitRect{
		x: r.x + style.GetBorderLeftSize() + style.GetPaddingLeft(),
		y: r.y + style.GetBorderTopSize() + style.GetPaddingTop(),
		w: max(1, r.w-style.GetHorizontalFrameSize()),
		h: max(1, r.h-style.GetVerticalFrameSize()),
	}
}

func (m Model) renderModalFrame(title string, body []string) string {
	lines := []string{m.styles.modalTitle.Render(title)}
	if len(body) > 0 {
		lines = append(lines, "", strings.Join(body, "\n"))
	}
	return m.styles.modal.MaxWidth(m.modalMaxWidth()).Render(strings.Join(lines, "\n"))
}

func (m Model) renderModalScreen(overlay string) string {
	if strings.TrimSpace(overlay) == "" {
		return m.renderMain()
	}

	width := m.terminalWidth()
	height := m.terminalHeight()
	toast := m.renderToast()
	toastHeight := 0
	if toast != "" {
		toastHeight = lipgloss.Height(toast) + 1
	}

	tabs := m.renderTabs()
	bodyHeight := max(12, height-lipgloss.Height(tabs)-2-toastHeight)
	centered := lipgloss.Place(width-2, bodyHeight, lipgloss.Center, lipgloss.Center, overlay)
	body := m.styles.modalBackdrop.Width(width).Height(bodyHeight).Render(centered)
	if toast != "" {
		body = strings.Join([]string{
			body,
			lipgloss.PlaceHorizontal(max(1, width-2), lipgloss.Center, toast),
		}, "\n")
	}
	return strings.Join([]string{tabs, body}, "\n")
}

func (m Model) monitorStateBadge(output editableOutput) string {
	if !output.Enabled {
		return m.styles.badgeOff.Render("Disabled")
	}
	if output.Focused {
		return m.styles.badgeOn.Render("Focused")
	}
	return m.styles.badgeOn.Render("Enabled")
}

func (m Model) unsavedBadge() string {
	if m.dirty && !m.draftSaved {
		return m.styles.badgeAccent.Render("Unsaved Changes")
	}
	if m.dirty && m.draftSaved {
		return m.styles.badgeOn.Render("Saved Draft")
	}
	return m.styles.subtle.Render("Current setup")
}

func (m *Model) activateInspectorField() tea.Cmd {
	if len(m.editOutputs) == 0 {
		return nil
	}

	switch m.inspectorField {
	case 1:
		output := m.editOutputs[m.selectedOutput]
		if len(output.Modes) == 0 {
			return nil
		}
		items := make([]list.Item, 0, len(output.Modes))
		for _, mode := range output.Modes {
			items = append(items, pickerItem(mode))
		}
		inner := list.NewDefaultDelegate()
		inner.ShowDescription = false
		inner.SetHeight(1)
		inner.SetSpacing(0)
		inner.Styles.NormalTitle = m.styles.value
		inner.Styles.SelectedTitle = m.styles.focused.Copy().UnsetPadding()
		inner.Styles.DimmedTitle = m.styles.subtle
		inner.Styles.FilterMatch = m.styles.badgeAccent
		delegate := arrowDelegate{inner}
		picker := list.New(items, delegate, m.modePickerWidth()-2, m.modePickerHeight())
		picker.Title = fmt.Sprintf("Mode for %s", output.Name)
		picker.SetShowHelp(false)
		picker.SetShowPagination(false)
		picker.SetShowStatusBar(false)
		picker.SetFilteringEnabled(false)
		picker.DisableQuitKeybindings()
		picker.Styles.Title = m.styles.modalTitle
		picker.Styles.TitleBar = lipgloss.NewStyle().PaddingBottom(1)
		picker.Styles.PaginationStyle = m.styles.subtle
		picker.Styles.HelpStyle = m.styles.help
		picker.Styles.NoItems = m.styles.subtle
		picker.Select(clampIndex(output.ModeIndex, len(output.Modes)))
		m.picker = &modePickerState{
			OutputIndex: m.selectedOutput,
			FieldIndex:  -1,
			List:        picker,
		}
		m.mode = modeModePicker
		return nil
	case 2:
		output := m.editOutputs[m.selectedOutput]
		return m.openNumericInput(
			numericInputScale,
			m.selectedOutput,
			fmt.Sprintf("Set Scale for %s", output.Name),
			"Type a scale. Enter applies. Esc cancels.",
			scaling.Format(output.Scale),
		)
	case 7, 8:
		output := m.editOutputs[m.selectedOutput]
		kind := numericInputPositionX
		title := fmt.Sprintf("Set Position X for %s", output.Name)
		hint := "Type the exact X position in logical pixels. Enter applies. Esc cancels."
		value := strconv.Itoa(output.X)
		if m.inspectorField == 8 {
			kind = numericInputPositionY
			title = fmt.Sprintf("Set Position Y for %s", output.Name)
			hint = "Type the exact Y position in logical pixels. Enter applies. Esc cancels."
			value = strconv.Itoa(output.Y)
		}
		return m.openNumericInput(kind, m.selectedOutput, title, hint, value)
	case 3:
		m.openFieldPicker(layoutFields[3], m.inspectorField, []string{"8", "10"})
		return nil
	case 4:
		m.openFieldPicker(layoutFields[4], m.inspectorField, []string{"srgb", "auto", "wide", "hdr", "hdredid", "dcip3", "dp3", "adobe", "edid"})
		return nil
	case 5:
		m.openFieldPicker("VRR", m.inspectorField, []string{"off", "on", "fullscreen"})
		return nil
	case 6:
		m.openFieldPicker("Rotation", m.inspectorField, []string{"normal", "90", "180", "270", "flipped", "flipped+90", "flipped+180", "flipped+270"})
		return nil
	case 9:
		targets := []string{"None"}
		for i, other := range m.editOutputs {
			if i != m.selectedOutput {
				targets = append(targets, other.displayModelLabel())
			}
		}
		m.openFieldPicker("Mirror", m.inspectorField, targets)
		return nil
	case 10:
		output := m.editOutputs[m.selectedOutput]
		return m.openNumericInput(numericInputFloat, m.selectedOutput, layoutFields[10], "SDR-to-HDR luminance multiplier, 0–3 (0 uses 1). Enter applies. Esc cancels.", fmt.Sprintf("%.2f", sdrMultiplier(output.SDRBrightness)))
	case 11:
		output := m.editOutputs[m.selectedOutput]
		return m.openNumericInput(numericInputFloat, m.selectedOutput, layoutFields[11], "SDR-to-HDR saturation multiplier, 0–3 (0 uses 1). Enter applies. Esc cancels.", fmt.Sprintf("%.2f", sdrMultiplier(output.SDRSaturation)))
	case 12:
		output := m.editOutputs[m.selectedOutput]
		return m.openNumericInput(numericInputFloat, m.selectedOutput, layoutFields[12], "SDR-to-HDR black level, 0–1 cd/m². Enter applies. Esc cancels.", fmt.Sprintf("%.3f", output.SDRMinLuminance))
	case 13:
		output := m.editOutputs[m.selectedOutput]
		return m.openNumericInput(numericInputInt, m.selectedOutput, layoutFields[13], "SDR-to-HDR white level, 0–1000 cd/m². Enter applies. Esc cancels.", fmt.Sprintf("%d", output.SDRMaxLuminance))
	case 14:
		m.openFieldPicker(layoutFields[14], m.inspectorField, []string{"default", "gamma22", "srgb"})
		return nil
	case 15:
		output := m.editOutputs[m.selectedOutput]
		return m.openNumericInput(numericInputFloat, m.selectedOutput, layoutFields[15], "Display black-level metadata in cd/m². All-zero overrides use EDID.", fmt.Sprintf("%.3f", output.MinLuminance))
	case 16:
		output := m.editOutputs[m.selectedOutput]
		return m.openNumericInput(numericInputInt, m.selectedOutput, layoutFields[16], "Display peak-luminance metadata in cd/m². All-zero overrides use EDID.", fmt.Sprintf("%d", output.MaxLuminance))
	case 17:
		output := m.editOutputs[m.selectedOutput]
		return m.openNumericInput(numericInputInt, m.selectedOutput, layoutFields[17], "Maximum frame-average luminance metadata in cd/m². Zero uses EDID.", fmt.Sprintf("%d", output.MaxAvgLuminance))
	case 18:
		m.openFieldPicker(layoutFields[18], m.inspectorField, []string{"off", "auto", "on"})
		return nil
	case 19:
		m.openFieldPicker(layoutFields[19], m.inspectorField, []string{"off", "auto", "on"})
		return nil
	case 20:
		output := m.editOutputs[m.selectedOutput]
		return m.openNumericInput(
			numericInputICC,
			m.selectedOutput,
			fmt.Sprintf("%s for %s", layoutFields[20], output.Name),
			"Absolute path to an ICC device profile. Leave empty to clear. Enter applies. Esc cancels.",
			output.ICC,
		)
	default:
		m.adjustInspectorField(1)
		return nil
	}
}

func (m *Model) openFieldPicker(title string, fieldIndex int, options []string) {
	output := m.editOutputs[m.selectedOutput]
	currentValue := m.layoutFieldValue(output, fieldIndex)

	items := make([]list.Item, 0, len(options))
	selected := 0
	for i, opt := range options {
		items = append(items, fieldPickerItem{pickerItem(opt), fieldOptionLabel(fieldIndex, opt)})
		if opt == currentValue {
			selected = i
		}
	}
	inner := list.NewDefaultDelegate()
	inner.ShowDescription = false
	inner.SetHeight(1)
	inner.SetSpacing(0)
	inner.Styles.NormalTitle = m.styles.value
	inner.Styles.SelectedTitle = m.styles.focused.Copy().UnsetPadding()
	inner.Styles.DimmedTitle = m.styles.subtle
	inner.Styles.FilterMatch = m.styles.badgeAccent
	delegate := arrowDelegate{inner}
	height := clampInt(len(options)+2, 4, 12)
	picker := list.New(items, delegate, m.modePickerWidth()-2, height)
	picker.Title = title
	picker.SetShowHelp(false)
	picker.SetShowPagination(false)
	picker.SetShowStatusBar(false)
	picker.SetFilteringEnabled(false)
	picker.DisableQuitKeybindings()
	picker.Styles.Title = m.styles.modalTitle
	picker.Styles.TitleBar = lipgloss.NewStyle().PaddingBottom(1)
	picker.Styles.PaginationStyle = m.styles.subtle
	picker.Styles.HelpStyle = m.styles.help
	picker.Styles.NoItems = m.styles.subtle
	picker.Select(selected)
	m.picker = &modePickerState{
		OutputIndex: m.selectedOutput,
		FieldIndex:  fieldIndex,
		List:        picker,
	}
	m.mode = modeModePicker
}

func (m Model) numericInputWidthFor(kind numericInputKind) int {
	if kind == numericInputICC {
		return clampInt(m.modalMaxWidth()-16, 20, 60)
	}
	return clampInt(m.modalMaxWidth()-16, 8, 12)
}

func (m *Model) openNumericInput(kind numericInputKind, outputIndex int, title string, hint string, value string) tea.Cmd {
	input := textinput.New()
	input.Prompt = ""
	input.CharLimit = 12
	if kind == numericInputICC {
		input.CharLimit = 256
	} else if kind == numericInputWorkspaceCount || kind == numericInputWorkspaceGroupSize {
		// Match the native int used by the profile model, without imposing a
		// smaller UI policy limit.
		input.CharLimit = len(strconv.Itoa(int(^uint(0) >> 1)))
	}
	input.Width = m.numericInputWidthFor(kind)
	input.TextStyle = m.styles.value
	input.PlaceholderStyle = m.styles.subtle
	input.Cursor.Style = lipgloss.NewStyle()
	if kind == numericInputScale {
		input.Placeholder = "1.00"
	}
	input.SetValue(value)
	cmd := input.Focus()
	m.input = &numericInputState{
		Kind:        kind,
		OutputIndex: outputIndex,
		FieldIndex:  m.inspectorField,
		Title:       title,
		Hint:        hint,
		Input:       input,
	}
	m.mode = modeNumericInput
	return cmd
}

func (m Model) renderModePicker() string {
	if m.picker == nil || m.picker.OutputIndex < 0 || m.picker.OutputIndex >= len(m.editOutputs) {
		return ""
	}

	output := m.editOutputs[m.picker.OutputIndex]
	body := []string{
		m.styles.subtle.Render(fmt.Sprintf("Pick a display mode for %s.", output.Name)),
		"",
		m.picker.List.View(),
		"",
		m.styles.help.Render("Enter applies. Esc closes."),
	}
	return m.renderModalFrame("Select Mode", body)
}

func (m Model) renderNumericInput() string {
	if m.input == nil {
		return ""
	}

	inputBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(m.styles.palette.paneActiveBorder)).
		Padding(0, 1).
		Render(m.input.Input.View())
	body := []string{
		m.styles.subtle.Render(m.input.Hint),
		"",
		m.styles.label.Render("Value"),
		inputBox,
	}
	if m.input.Input.Err != nil && m.input.Kind != numericInputScale {
		body = append(body, "", m.styles.statusError.Render(m.input.Input.Err.Error()))
	}
	if m.input.Kind == numericInputScale {
		body = append(body, m.scaleInputFeedback()...)
	}
	return m.renderModalFrame(m.input.Title, body)
}

func (m Model) scaleInputFeedback() []string {
	if m.input == nil || m.input.OutputIndex < 0 || m.input.OutputIndex >= len(m.editOutputs) {
		return nil
	}

	output := m.editOutputs[m.input.OutputIndex]
	value, err := parseScaleInput(m.input.Input.Value())
	if err != nil {
		return []string{"", m.styles.statusError.Render(err.Error())}
	}

	logicalW, logicalH := scaling.LogicalSize(output.Width, output.Height, value)
	if scaling.Sharp(output.Width, output.Height, value) {
		return []string{
			"",
			m.styles.statusOK.Render(fmt.Sprintf(
				"Sharp: %d / %s = %d, %d / %s = %d logical px.",
				output.Width,
				scaling.Format(value),
				int(math.Round(logicalW)),
				output.Height,
				scaling.Format(value),
				int(math.Round(logicalH)),
			)),
		}
	}

	suggestion, ok := scaling.ClosestSharp(output.Width, output.Height, value)
	if !ok {
		return []string{
			"",
			m.styles.warning.Render(fmt.Sprintf(
				"⚠ Not sharp: final size has fractional px (%d / %s = %.2f, %d / %s = %.2f).",
				output.Width,
				scaling.Format(value),
				logicalW,
				output.Height,
				scaling.Format(value),
				logicalH,
			)),
		}
	}

	suggestedW, suggestedH := scaling.LogicalSize(output.Width, output.Height, suggestion)
	return []string{
		"",
		m.styles.warning.Render(fmt.Sprintf(
			"⚠ Not sharp: final size has fractional px (%d / %s = %.2f, %d / %s = %.2f).",
			output.Width,
			scaling.Format(value),
			logicalW,
			output.Height,
			scaling.Format(value),
			logicalH,
		)),
		m.styles.statusOK.Render(fmt.Sprintf(
			"Closest sharp: %s -> %d x %d logical px. Enter applies it.",
			scaling.Format(suggestion),
			int(math.Round(suggestedW)),
			int(math.Round(suggestedH)),
		)),
	}
}

func (m *Model) openProfileExecInput() tea.Cmd {
	if len(m.profiles) == 0 || m.selectedProfile < 0 || m.selectedProfile >= len(m.profiles) {
		return nil
	}

	selected := m.profiles[m.selectedProfile]
	input := textinput.New()
	input.Prompt = ""
	input.CharLimit = 512
	input.Width = clampInt(m.modalMaxWidth()-16, 24, 72)
	input.TextStyle = m.styles.value
	input.PlaceholderStyle = m.styles.subtle
	input.Cursor.Style = lipgloss.NewStyle()
	input.Placeholder = "/path/to/script.sh"
	input.SetValue(selected.Exec)
	cmd := input.Focus()

	m.execInput = &profileExecInputState{
		ProfileIndex: m.selectedProfile,
		Title:        fmt.Sprintf("Edit Exec for %s", selected.Name),
		Input:        input,
	}
	m.mode = modeProfileExecInput
	return cmd
}

func (m Model) renderProfileExecInput() string {
	if m.execInput == nil {
		return ""
	}

	inputBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(m.styles.palette.paneActiveBorder)).
		Padding(0, 1).
		Render(m.execInput.Input.View())
	body := []string{m.styles.label.Render("Exec"), inputBox}
	if m.execInput.Err != nil {
		body = append(body, "", m.styles.statusError.MaxWidth(max(20, m.modalMaxWidth()-6)).Render(m.execInput.Err.Error()))
	}
	body = append(body, "", m.styles.help.MaxWidth(max(20, m.modalMaxWidth()-6)).Render("Enter saves the profile when the command is executable. Leave empty to clear. Esc discards it."))
	return m.renderModalFrame(m.execInput.Title, body)
}

func (m *Model) openSaveDialog() (tea.Model, tea.Cmd) {
	return m.openSaveDialogFor(saveDialogProfile)
}

func (m *Model) openQuitSaveDialog() (tea.Model, tea.Cmd) {
	return m.openSaveDialogFor(saveDialogQuit)
}

func (m *Model) openSaveDialogFor(purpose saveDialogPurpose) (tea.Model, tea.Cmd) {
	input := textinput.New()
	input.Prompt = ""
	input.CharLimit = 64
	input.Width = m.saveDialogInputWidth()
	input.TextStyle = m.styles.value
	input.PlaceholderStyle = m.styles.subtle
	input.Cursor.Style = lipgloss.NewStyle()
	name := m.saveDialogSuggestedName()
	input.SetValue(name)
	cmd := input.Focus()

	items := make([]profileListItem, 0, len(m.profiles))
	for _, prof := range m.profiles {
		items = append(items, profileListItem{name: prof.Name, updated: prof.UpdatedAt, outputs: len(prof.Outputs)})
	}
	prioritizeProfileListItem(items, name)

	inner := list.NewDefaultDelegate()
	inner.Styles.NormalTitle = m.styles.value
	inner.Styles.NormalDesc = m.styles.subtle
	inner.Styles.SelectedTitle = m.styles.focused.Copy().UnsetPadding()
	inner.Styles.SelectedDesc = m.styles.selectedDesc
	inner.Styles.DimmedTitle = m.styles.subtle
	inner.Styles.DimmedDesc = m.styles.subtle
	inner.Styles.FilterMatch = m.styles.badgeAccent
	delegate := arrowDelegate{inner}

	listHeight := clampInt(defaultHeight(m.height)-18, 3, 10)
	profileList := list.New(nil, delegate, m.saveDialogListWidth()-2, listHeight)
	profileList.Title = "Existing Profiles"
	profileList.SetShowHelp(false)
	profileList.SetShowPagination(false)
	profileList.SetShowStatusBar(false)
	profileList.SetFilteringEnabled(false)
	profileList.DisableQuitKeybindings()
	profileList.Styles.Title = m.styles.modalTitle
	profileList.Styles.TitleBar = lipgloss.NewStyle().PaddingBottom(1)
	profileList.Styles.PaginationStyle = m.styles.subtle
	profileList.Styles.HelpStyle = m.styles.help
	profileList.Styles.NoItems = m.styles.subtle

	m.saveDialog = &saveDialogState{
		Input:   input,
		List:    profileList,
		All:     items,
		Filter:  "",
		Action:  defaultSaveAction(purpose),
		Purpose: purpose,
	}
	m.mode = modeSave
	m.saveOverwrite = ""
	m.rebuildSaveList(false)
	return m, cmd
}

func (m Model) saveDialogSuggestedName() string {
	if suggested := strings.TrimSpace(m.draftProfileName); suggested != "" {
		return suggested
	}
	if suggested := strings.TrimSpace(m.matchedProfileName); suggested != "" {
		return suggested
	}
	return defaultProfileName()
}

func prioritizeProfileListItem(items []profileListItem, name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	for idx, item := range items {
		if !strings.EqualFold(strings.TrimSpace(item.name), name) {
			continue
		}
		if idx == 0 {
			return
		}
		copy(items[1:idx+1], items[:idx])
		items[0] = item
		return
	}
}

func (m *Model) cycleSaveAction(delta int) {
	if m.saveDialog == nil {
		return
	}
	actions := saveActionsForPurpose(m.saveDialog.Purpose)
	current := 0
	for idx, action := range actions {
		if action == m.saveDialog.Action {
			current = idx
			break
		}
	}
	m.saveDialog.Action = actions[wrapIndex(current+delta, len(actions))]
}

func (m Model) selectedSaveAction() saveAction {
	if m.saveDialog == nil {
		return saveActionOnly
	}
	return m.saveDialog.Action
}

func (m Model) saveActionLabel(action saveAction) string {
	switch action {
	case saveActionApply:
		return "Save & Apply"
	case saveActionSaveQuit:
		return "Save, Apply & Quit"
	case saveActionDiscardQuit:
		return "Quit Without Saving"
	case saveActionCancel:
		return "Cancel"
	default:
		return "Save"
	}
}

func (m Model) renderSaveActionButtons() string {
	purpose := saveDialogProfile
	if m.saveDialog != nil {
		purpose = m.saveDialog.Purpose
	}
	actions := saveActionsForPurpose(purpose)
	parts := make([]string, 0, len(actions))
	for _, action := range actions {
		style := m.styles.field
		if action == m.selectedSaveAction() {
			style = m.styles.focused
		}
		parts = append(parts, style.Render(m.saveActionLabel(action)))
	}
	return lipgloss.JoinHorizontal(lipgloss.Left, parts...)
}

func defaultSaveAction(purpose saveDialogPurpose) saveAction {
	if purpose == saveDialogQuit {
		return saveActionSaveQuit
	}
	return saveActionApply
}

func saveActionsForPurpose(purpose saveDialogPurpose) []saveAction {
	if purpose == saveDialogQuit {
		return []saveAction{saveActionSaveQuit, saveActionDiscardQuit, saveActionCancel}
	}
	return []saveAction{saveActionOnly, saveActionApply, saveActionCancel}
}

func (m *Model) rebuildSaveList(resetSelection bool) {
	if m.saveDialog == nil {
		return
	}

	filter := strings.ToLower(strings.TrimSpace(m.saveDialog.Filter))
	current := ""
	if selected, ok := m.saveDialog.List.SelectedItem().(profileListItem); ok {
		current = selected.name
	}

	filtered := make([]list.Item, 0, len(m.saveDialog.All))
	for _, item := range m.saveDialog.All {
		if filter == "" || strings.Contains(strings.ToLower(item.name), filter) {
			filtered = append(filtered, item)
		}
	}
	m.saveDialog.List.SetItems(filtered)
	if len(filtered) == 0 {
		return
	}
	if resetSelection || current == "" {
		m.saveDialog.List.Select(0)
		return
	}
	for idx, item := range filtered {
		profileItem := item.(profileListItem)
		if profileItem.name == current {
			m.saveDialog.List.Select(idx)
			return
		}
	}
}

func (m *Model) syncSaveNameFromSelection() {
	if m.saveDialog == nil {
		return
	}
	if selected, ok := m.saveDialog.List.SelectedItem().(profileListItem); ok {
		m.saveDialog.Input.SetValue(selected.name)
		m.saveDialog.Input.CursorEnd()
	}
}

func (m Model) updateSaveKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.saveDialog == nil {
		m.mode = modeMain
		return m, nil
	}

	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		if m.saveDialog.Purpose == saveDialogQuit {
			m.setStatusOK("Quit cancelled")
		}
		m.mode = modeMain
		m.saveDialog = nil
		m.saveOverwrite = ""
		return m, nil
	case "tab", "right":
		m.cycleSaveAction(1)
		return m, nil
	case "shift+tab", "left":
		m.cycleSaveAction(-1)
		return m, nil
	case "enter":
		if m.selectedSaveAction() == saveActionDiscardQuit {
			return m, tea.Quit
		}
		if m.selectedSaveAction() == saveActionCancel {
			if m.saveDialog.Purpose == saveDialogQuit {
				m.setStatusOK("Quit cancelled")
			}
			m.mode = modeMain
			m.saveDialog = nil
			m.saveOverwrite = ""
			return m, nil
		}
		name := strings.TrimSpace(m.saveDialog.Input.Value())
		if name == "" {
			m.setStatusErr("Profile name cannot be empty")
			return m, nil
		}
		if m.profileExists(name) {
			m.saveOverwrite = name
			m.mode = modeSaveConfirm
			return m, nil
		}
		return m, m.saveCmd(m.currentProfile(name))
	case "up", "down", "pgup", "pgdown", "home", "end":
		var cmd tea.Cmd
		m.saveDialog.List, cmd = m.saveDialog.List.Update(msg)
		m.syncSaveNameFromSelection()
		return m, cmd
	default:
		var cmd tea.Cmd
		before := m.saveDialog.Input.Value()
		m.saveDialog.Input, cmd = m.saveDialog.Input.Update(msg)
		if m.saveDialog.Input.Value() != before {
			m.saveDialog.Filter = m.saveDialog.Input.Value()
			m.rebuildSaveList(true)
		}
		return m, cmd
	}
}

func (m Model) updateSaveConfirmKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "n":
		m.mode = modeSave
		return m, nil
	case "enter", "y":
		name := strings.TrimSpace(m.saveOverwrite)
		if name == "" {
			m.mode = modeSave
			return m, nil
		}
		return m, m.saveCmd(m.currentProfile(name))
	default:
		return m, nil
	}
}

func (m *Model) updateProfileExecInputKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.execInput == nil {
		m.mode = modeMain
		return m, nil
	}

	switch msg.String() {
	case "esc", "q":
		m.execInput = nil
		m.mode = modeMain
		return m, nil
	case "enter":
		return m, m.commitProfileExecInput()
	}

	var cmd tea.Cmd
	m.execInput.Input, cmd = m.execInput.Input.Update(msg)
	return m, cmd
}

func (m *Model) commitProfileExecInput() tea.Cmd {
	if m.execInput == nil {
		m.mode = modeMain
		return nil
	}
	if m.execInput.ProfileIndex < 0 || m.execInput.ProfileIndex >= len(m.profiles) {
		m.execInput = nil
		m.mode = modeMain
		return nil
	}

	execValue := strings.TrimSpace(m.execInput.Input.Value())
	if err := apply.ValidatePostApplyExec(execValue); err != nil {
		m.execInput.Err = err
		return nil
	}

	selected := &m.profiles[m.execInput.ProfileIndex]
	selected.Exec = execValue
	if strings.EqualFold(strings.TrimSpace(m.draftProfileName), strings.TrimSpace(selected.Name)) {
		m.draftExec = selected.Exec
	}
	if selected.Exec == "" {
		m.setStatusOK(fmt.Sprintf("Cleared exec for %q", selected.Name))
	} else {
		m.setStatusOK(fmt.Sprintf("Updated exec for %q", selected.Name))
	}
	m.execInput = nil
	m.mode = modeMain
	return m.saveProfileCmd(*selected)
}

func (m *Model) updateModePickerKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.picker == nil {
		m.mode = modeMain
		return m, nil
	}

	switch msg.String() {
	case "esc", "q":
		m.picker = nil
		m.mode = modeMain
		return m, nil
	case "enter":
		return m, m.commitModePicker()
	}

	var cmd tea.Cmd
	m.picker.List, cmd = m.picker.List.Update(msg)
	return m, cmd
}

func (m *Model) commitModePicker() tea.Cmd {
	if m.picker == nil {
		m.mode = modeMain
		return nil
	}
	if m.picker.OutputIndex < 0 || m.picker.OutputIndex >= len(m.editOutputs) {
		m.picker = nil
		m.mode = modeMain
		return nil
	}
	selected, ok := m.picker.List.SelectedItem().(interface{ Value() string })
	if !ok {
		m.picker = nil
		m.mode = modeMain
		return nil
	}

	output := &m.editOutputs[m.picker.OutputIndex]
	value := selected.Value()
	oldWidth, oldHeight := output.logicalSize()

	if m.picker.FieldIndex >= 0 {
		m.applyFieldPickerValue(output, m.picker.FieldIndex, value)
		m.reflowAfterResize(m.picker.OutputIndex, oldWidth, oldHeight)
		m.layoutChanged()
		m.setStatusOK(fmt.Sprintf("Set %s to %s for %s", layoutFields[m.picker.FieldIndex], fieldOptionLabel(m.picker.FieldIndex, value), output.Name))
		m.picker = nil
		m.mode = modeMain
		return nil
	}

	output.ModeIndex = indexOf(output.Modes, value)
	if output.ModeIndex < 0 {
		output.ModeIndex = 0
	}
	if output.ModeUnsupported && output.ModeIndex > 0 {
		output.ModeUnsupported = false
	}
	output.applyMode(output.Modes[output.ModeIndex])
	m.reflowAfterResize(m.picker.OutputIndex, oldWidth, oldHeight)
	m.layoutChanged()
	m.setStatusOK(fmt.Sprintf("Selected %s for %s", output.DisplayMode(), output.Name))
	m.picker = nil
	m.mode = modeMain
	return nil
}

func (m *Model) applyFieldPickerValue(output *editableOutput, field int, value string) {
	switch field {
	case 3:
		output.Bitdepth, _ = strconv.Atoi(value)
	case 4:
		output.CM = value
	case 5:
		switch value {
		case "on":
			output.VRR = 1
		case "fullscreen":
			output.VRR = 2
		default:
			output.VRR = 0
		}
	case 6:
		for i, label := range []string{"normal", "90", "180", "270", "flipped", "flipped+90", "flipped+180", "flipped+270"} {
			if label == value {
				output.Transform = i
				break
			}
		}
	case 9:
		if value == "None" {
			output.MirrorOf = ""
		} else {
			for _, other := range m.editOutputs {
				if other.displayModelLabel() == value {
					output.MirrorOf = other.Key
					break
				}
			}
		}
	case 14:
		output.SDREOTF = value
	case 18:
		switch value {
		case "off":
			output.SupportsWideColor = -1
		case "on":
			output.SupportsWideColor = 1
		default:
			output.SupportsWideColor = 0
		}
	case 19:
		switch value {
		case "off":
			output.SupportsHDR = -1
		case "on":
			output.SupportsHDR = 1
		default:
			output.SupportsHDR = 0
		}
	}
}

func (m *Model) applyNumericFieldValue(output *editableOutput, field int, value float64) {
	switch field {
	case 10:
		output.SDRBrightness = value
	case 11:
		output.SDRSaturation = value
	case 12:
		output.SDRMinLuminance = value
	case 13:
		output.SDRMaxLuminance = int(value)
	case 15:
		output.MinLuminance = value
	case 16:
		output.MaxLuminance = int(value)
	case 17:
		output.MaxAvgLuminance = int(value)
	}
}

func parseScaleInput(raw string) (float64, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, fmt.Errorf("scale is required")
	}
	if strings.HasSuffix(value, ".") {
		return 0, fmt.Errorf("scale must be a number")
	}

	scale, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("scale must be a number")
	}
	if scale < scaling.MinScale || scale > scaling.MaxScale {
		return 0, fmt.Errorf("scale must be between %s and %s", scaling.Format(scaling.MinScale), scaling.Format(scaling.MaxScale))
	}
	return scaling.Round(scale), nil
}

func (m *Model) updateNumericInputKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.input == nil {
		m.mode = modeMain
		return m, nil
	}

	switch msg.String() {
	case "esc", "q":
		m.input = nil
		m.mode = modeMain
		return m, nil
	case "enter":
		return m, m.commitNumericInput()
	}

	var cmd tea.Cmd
	before := m.input.Input.Value()
	m.input.Input, cmd = m.input.Input.Update(msg)
	if m.input.Input.Value() != before {
		m.input.Input.Err = nil
	}
	return m, cmd
}

func (m *Model) commitNumericInput() tea.Cmd {
	if m.input == nil {
		m.mode = modeMain
		return nil
	}
	if m.input.Kind == numericInputWorkspaceCount || m.input.Kind == numericInputWorkspaceGroupSize {
		return m.commitWorkspaceNumericInput()
	}
	if m.input.OutputIndex < 0 || m.input.OutputIndex >= len(m.editOutputs) {
		m.input = nil
		m.mode = modeMain
		return nil
	}

	output := &m.editOutputs[m.input.OutputIndex]
	oldWidth, oldHeight := output.logicalSize()
	var status string
	switch m.input.Kind {
	case numericInputScale:
		typedValue, err := parseScaleInput(m.input.Input.Value())
		if err != nil {
			m.input.Input.Err = err
			return nil
		}
		value := typedValue
		if !scaling.Sharp(output.Width, output.Height, typedValue) {
			if suggestion, ok := scaling.ClosestSharp(output.Width, output.Height, typedValue); ok {
				value = suggestion
			}
		}
		output.Scale = value
		status = fmt.Sprintf("Scale set to %s for %s", scaling.Format(value), output.Name)
		if value != typedValue {
			status += fmt.Sprintf(" (closest sharp scale for %s)", scaling.Format(typedValue))
		} else if !scaling.Sharp(output.Width, output.Height, value) {
			status += " (fractional logical size may look blurry)"
		}
	case numericInputPositionX:
		value, err := strconv.Atoi(strings.TrimSpace(m.input.Input.Value()))
		if err != nil {
			m.input.Input.Err = fmt.Errorf("position must be an integer")
			return nil
		}
		output.X = value
		status = fmt.Sprintf("Position X set to %d for %s", value, output.Name)
	case numericInputPositionY:
		value, err := strconv.Atoi(strings.TrimSpace(m.input.Input.Value()))
		if err != nil {
			m.input.Input.Err = fmt.Errorf("position must be an integer")
			return nil
		}
		output.Y = value
		status = fmt.Sprintf("Position Y set to %d for %s", value, output.Name)
	case numericInputICC:
		output.ICC = strings.TrimSpace(m.input.Input.Value())
		if output.ICC == "" {
			status = fmt.Sprintf("ICC profile cleared for %s", output.Name)
		} else {
			status = fmt.Sprintf("ICC profile set for %s", output.Name)
		}
	case numericInputFloat:
		value, err := strconv.ParseFloat(strings.TrimSpace(m.input.Input.Value()), 64)
		if err != nil {
			m.input.Input.Err = fmt.Errorf("must be a number")
			return nil
		}
		m.applyNumericFieldValue(output, m.input.FieldIndex, value)
		status = fmt.Sprintf("%s set for %s", m.input.Title, output.Name)
	case numericInputInt:
		value, err := strconv.Atoi(strings.TrimSpace(m.input.Input.Value()))
		if err != nil {
			m.input.Input.Err = fmt.Errorf("must be an integer")
			return nil
		}
		m.applyNumericFieldValue(output, m.input.FieldIndex, float64(value))
		status = fmt.Sprintf("%s set for %s", m.input.Title, output.Name)
	}
	m.reflowAfterResize(m.input.OutputIndex, oldWidth, oldHeight)
	m.layoutChanged()
	m.setStatusOK(status)
	m.input = nil
	m.mode = modeMain
	return nil
}

func (m *Model) commitWorkspaceNumericInput() tea.Cmd {
	value, err := strconv.Atoi(strings.TrimSpace(m.input.Input.Value()))
	if err != nil || value < 1 {
		m.input.Input.Err = fmt.Errorf("must be a positive integer")
		return nil
	}

	if m.input.Kind == numericInputWorkspaceCount {
		if m.workspaceEdit.Strategy == profile.WorkspaceStrategyManual {
			m.resizeManualWorkspaceRules(value)
		}
		m.workspaceEdit.MaxWorkspaces = value
		m.setStatusOK(fmt.Sprintf("Workspace count set to %d", value))
	} else {
		m.workspaceEdit.GroupSize = value
		m.workspaceEdit.LastSequentialGroupSize = value
		m.setStatusOK(fmt.Sprintf("Workspace group size set to %d", value))
	}
	m.markDirty()
	m.input = nil
	m.mode = modeMain
	return nil
}

func (m *Model) updateMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeSave:
		return m.updateSaveMouse(msg)
	case modeModePicker:
		return m.updateModePickerMouse(msg)
	case modeNumericInput, modeProfileExecInput, modeSaveConfirm, modeConfirm:
		return m, nil
	}

	if msg.Action == tea.MouseActionRelease {
		if m.drag != nil {
			m.selectedOutput = m.drag.OutputIndex
			cmd := m.showSnapHint(m.applySelectedSnap(36))
			m.layoutChanged()
			m.drag = nil
			return m, cmd
		}
		m.drag = nil
		return m, nil
	}

	if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft && msg.Y == m.appContentY() {
		plainTabs := ansi.Strip(m.renderTabs())
		localX := msg.X - m.appContentX()
		if start, ok := visibleTextColumn(plainTabs, "Daemon not running"); ok {
			if localX >= start && localX < start+lipgloss.Width("Daemon not running") {
				return m, m.openURLCmd("Daemon not running", daemonURL)
			}
		}
	}

	if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft && m.restartHintAt(msg.X, msg.Y) {
		return m, m.restartDaemonCmd()
	}

	if tab, ok := m.tabAt(msg.X, msg.Y); ok && msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
		m.tab = tab
		return m, nil
	}

	if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
		if link, ok := m.footerLinkAt(msg.X, msg.Y); ok {
			return m, m.openURLCmd(link.label, link.url)
		}
	}

	switch m.tab {
	case tabLayout:
		return m.updateLayoutMouse(msg)
	case tabProfiles:
		return m.updateProfilesMouse(msg)
	case tabWorkspaces:
		return m.updateWorkspaceMouse(msg)
	default:
		return m, nil
	}
}

func (m Model) updateSaveMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.saveDialog == nil {
		return m, nil
	}
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return m, nil
	}

	if index, ok := m.saveDialogItemIndexAt(msg.X, msg.Y); ok {
		m.saveDialog.List.Select(index)
		m.syncSaveNameFromSelection()
	}
	return m, nil
}

func visibleTextColumn(line, label string) (int, bool) {
	index := strings.Index(line, label)
	if index < 0 {
		return 0, false
	}
	return lipgloss.Width(line[:index]), true
}

func (m *Model) updateModePickerMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.picker == nil {
		return m, nil
	}

	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return m, nil
	}

	if index, ok := m.modePickerItemIndexAt(msg.X, msg.Y); ok {
		m.picker.List.Select(index)
		return m, m.commitModePicker()
	}

	return m, nil
}

func (m *Model) updateLayoutMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	canvasRect, _ := m.layoutCanvasRect()
	inspectorRect, compact := m.layoutInspectorRect()
	layout := m.canvasLayout(canvasRect.w-m.styles.inactivePane.GetHorizontalFrameSize(), m.canvasMouseHeight())

	if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft && m.paneTitleContains(msg.X, msg.Y, canvasRect, "Monitor Layout") {
		m.layoutFocus = layoutFocusCanvas
		return m, nil
	}

	if m.inCanvas(msg.X, msg.Y, canvasRect, layout) {
		m.layoutFocus = layoutFocusCanvas
		localX, localY := m.canvasLocalPoint(msg.X, msg.Y, canvasRect)
		if rect, ok := layout.rectAt(localX, localY); ok {
			m.selectedOutput = rect.index
			if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
				m.snap = nil
				m.drag = &canvasDragState{OutputIndex: rect.index, LastX: msg.X, LastY: msg.Y}
			}
		}
		if msg.Action == tea.MouseActionMotion && m.drag != nil && m.drag.OutputIndex >= 0 && m.drag.OutputIndex < len(m.editOutputs) {
			dxCells := msg.X - m.drag.LastX
			dyCells := msg.Y - m.drag.LastY
			if dxCells != 0 || dyCells != 0 {
				worldDX := cellsToWorldX(dxCells, layout.scale, layout.cellW)
				worldDY := cellsToWorldY(dyCells, layout.scale)
				m.selectedOutput = m.drag.OutputIndex
				m.moveSelectedOutput(worldDX, worldDY)
				m.drag.LastX = msg.X
				m.drag.LastY = msg.Y
			}
		}
		return m, nil
	}

	if inspectorRect.contains(msg.X, msg.Y) {
		wasFocused := m.layoutFocus == layoutFocusInspector && m.tab == tabLayout
		m.layoutFocus = layoutFocusInspector
		if tab, ok := m.inspectorTabAt(msg.X, msg.Y, inspectorRect); ok && msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			m.inspectorTab = tab
			m.normalizeInspectorField()
			return m, nil
		}
		if field, ok := m.inspectorFieldAt(msg.Y, inspectorRect, compact, wasFocused); ok && msg.Action == tea.MouseActionPress {
			m.inspectorField = field
			switch msg.Button {
			case tea.MouseButtonLeft:
				return m, m.activateInspectorField()
			case tea.MouseButtonWheelUp:
				m.adjustInspectorField(1)
			case tea.MouseButtonWheelDown:
				m.adjustInspectorField(-1)
			}
		}
	}

	return m, nil
}

func (m Model) inspectorTabAt(x, y int, inspectorRect hitRect) (inspectorTab, bool) {
	if y != inspectorRect.y {
		return inspectorTabDisplay, false
	}
	localX := x - inspectorRect.x
	cursor := 3 // rounded corner, border segment, and title padding
	labels := []string{"Display", "Color"}
	for idx, label := range labels {
		width := lipgloss.Width(label)
		if localX >= cursor && localX < cursor+width {
			return inspectorTab(idx), true
		}
		cursor += width + 3 // " - "
	}
	return inspectorTabDisplay, false
}

func (m Model) paneTitleContains(x, y int, pane hitRect, title string) bool {
	if y != pane.y {
		return false
	}
	start := pane.x + 3
	return x >= start && x < start+lipgloss.Width(title)
}

func (m Model) updateProfilesMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return m, nil
	}

	listRect := m.profilesListRect()
	if !listRect.contains(msg.X, msg.Y) {
		return m, nil
	}

	inner := listRect.inner(m.styles.activePane)
	if msg.Y == inner.y {
		return m.toggleProfileAutomatic()
	}
	row := msg.Y - inner.y - profileListHeaderRows + m.profileListScroll(inner.h)
	if row < 0 || row >= len(m.profiles) || msg.Y < inner.y+profileListHeaderRows {
		return m, nil
	}
	m.selectedProfile = row
	return m, nil
}

func (m Model) updateWorkspaceMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	settingsRect := m.workspaceSettingsRect()
	if !settingsRect.contains(msg.X, msg.Y) {
		return m, nil
	}

	inner := settingsRect.inner(m.styles.activePane)
	if msg.Action == tea.MouseActionPress {
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.moveWorkspaceSelection(-3, false)
			return m, nil
		case tea.MouseButtonWheelDown:
			m.moveWorkspaceSelection(3, false)
			return m, nil
		}
	}

	scrollOffset := m.workspaceSettingsScrollOffset(inner.h)
	fieldRow := msg.Y - inner.y + scrollOffset
	if fieldRow >= 0 && fieldRow < len(workspaceFields) && msg.Action == tea.MouseActionPress {
		m.workspaceEdit.SelectedField = fieldRow
		switch msg.Button {
		case tea.MouseButtonLeft:
			m.adjustWorkspaceField(1)
			m.markDirty()
		}
		return m, nil
	}

	// Strategy-specific items start after the fields, a blank, and their header.
	orderStart := inner.y + len(workspaceFields) + 2
	visualY := msg.Y + scrollOffset
	if msg.Action == tea.MouseActionPress && visualY >= orderStart {
		row := visualY - orderStart
		itemCount := len(m.workspaceEdit.MonitorOrder)
		if m.workspaceEdit.Strategy == profile.WorkspaceStrategyManual {
			itemCount = len(m.workspaceEdit.Rules)
		}
		if row >= 0 && row < itemCount {
			m.workspaceEdit.SelectedField = len(workspaceFields) + row
			m.workspaceEdit.SelectedOrder = row
		}
	}
	return m, nil
}

func (m Model) bodyOriginY() int {
	return m.bodyRect().y
}

func (m Model) appContentX() int {
	return m.styles.app.GetPaddingLeft()
}

func (m Model) appContentY() int {
	return m.styles.app.GetPaddingTop()
}

func (m Model) bodyRect() hitRect {
	tabs := m.renderTabs()
	footer := m.renderFooterBar()
	return hitRect{
		x: m.appContentX(),
		y: m.appContentY() + lipgloss.Height(tabs),
		w: m.footerContentWidth(),
		h: m.mainBodyHeight(tabs, "", footer),
	}
}

func (m Model) layoutCanvasRect() (hitRect, bool) {
	body := m.bodyRect()
	if m.useCompactLayout(body.h) {
		canvasHeight, _ := m.compactLayoutHeights(body.h)
		return hitRect{x: body.x, y: body.y, w: body.w, h: canvasHeight}, true
	}

	canvasWidth, _ := m.layoutPaneWidths()
	return hitRect{x: body.x, y: body.y, w: canvasWidth, h: body.h}, false
}

func (m Model) layoutInspectorRect() (hitRect, bool) {
	body := m.bodyRect()
	if m.useCompactLayout(body.h) {
		canvasHeight, inspectorHeight := m.compactLayoutHeights(body.h)
		preferencesHeight, infoHeight := m.inspectorPaneHeights(inspectorHeight)
		return hitRect{x: body.x, y: body.y + canvasHeight + infoHeight, w: body.w, h: preferencesHeight}, true
	}

	canvasWidth, inspectorWidth := m.layoutPaneWidths()
	preferencesHeight, infoHeight := m.inspectorPaneHeights(body.h)
	return hitRect{x: body.x + canvasWidth + paneGapWidth, y: body.y + infoHeight, w: inspectorWidth, h: preferencesHeight}, false
}

func (m Model) profilesListRect() hitRect {
	body := m.bodyRect()
	if m.terminalWidth() < 96 {
		listHeight := clampInt(len(m.profiles)+profileListHeaderRows+2, profileListHeaderRows+4, body.h/3)
		return hitRect{x: body.x, y: body.y, w: body.w, h: listHeight}
	}

	listWidth, _ := m.sidePaneWidths(35)
	return hitRect{x: body.x, y: body.y, w: listWidth, h: body.h}
}

func (m Model) workspaceSettingsRect() hitRect {
	body := m.bodyRect()
	if m.terminalWidth() < 96 {
		settingsHeight := clampInt(m.workspaceSettingsLineCount()+2, 6, (body.h*2)/3)
		return hitRect{x: body.x, y: body.y, w: body.w, h: settingsHeight}
	}

	leftWidth, _ := m.sidePaneWidths(35)
	return hitRect{x: body.x, y: body.y, w: leftWidth, h: body.h}
}

func (m Model) workspaceSettingsLineCount() int {
	count := len(workspaceFields) + 2
	itemCount := m.workspaceListItemCount()
	if itemCount == 0 {
		count++
	} else {
		count += itemCount
	}
	return count
}

func (m Model) modalOverlayRect(overlay string) hitRect {
	if overlay == "" {
		return hitRect{}
	}

	tabsHeight := lipgloss.Height(m.renderTabs())
	bodyHeight := max(12, m.terminalHeight()-tabsHeight-2)
	bodyWidth := m.terminalWidth() - m.styles.modalBackdrop.GetHorizontalFrameSize()

	return hitRect{
		x: m.styles.modalBackdrop.GetPaddingLeft() + max(0, (bodyWidth-lipgloss.Width(overlay))/2),
		y: tabsHeight + m.styles.modalBackdrop.GetPaddingTop() + max(0, (bodyHeight-lipgloss.Height(overlay))/2),
		w: lipgloss.Width(overlay),
		h: lipgloss.Height(overlay),
	}
}

func (m Model) modePickerListRect() hitRect {
	if m.picker == nil {
		return hitRect{}
	}

	overlay := m.renderModePicker()
	modalRect := m.modalOverlayRect(overlay)
	inner := modalRect.inner(m.styles.modal)
	listView := m.picker.List.View()
	return hitRect{
		x: inner.x,
		y: inner.y + 4,
		w: lipgloss.Width(listView),
		h: lipgloss.Height(listView),
	}
}

func (m Model) modePickerItemIndexAt(x, y int) (int, bool) {
	if m.picker == nil {
		return 0, false
	}

	items := m.picker.List.VisibleItems()
	start, end := m.picker.List.Paginator.GetSliceBounds(len(items))
	return visibleListItemIndexAt(m.View(), x, y, items, start, end)
}

func (m Model) saveDialogItemIndexAt(x, y int) (int, bool) {
	if m.saveDialog == nil {
		return 0, false
	}

	items := m.saveDialog.List.VisibleItems()
	start, end := m.saveDialog.List.Paginator.GetSliceBounds(len(items))
	return visibleListItemIndexAt(m.View(), x, y, items, start, end)
}

func visibleListItemIndexAt(view string, x, y int, items []list.Item, start, end int) (int, bool) {
	lines := strings.Split(ansi.Strip(view), "\n")
	if y < 0 || y >= len(lines) {
		return 0, false
	}
	line := lines[y]

	for index := start; index < end; index++ {
		for _, label := range visibleListItemLabels(items[index]) {
			col := strings.Index(line, label)
			if col < 0 {
				continue
			}
			labelStart := max(0, lipgloss.Width(line[:col])-2)
			labelEnd := lipgloss.Width(line[:col]) + lipgloss.Width(label)
			if x >= labelStart && x < labelEnd {
				return index, true
			}
		}
	}

	return 0, false
}

func visibleListItemLabels(item list.Item) []string {
	labels := []string{item.FilterValue()}

	if titled, ok := item.(interface{ Title() string }); ok {
		labels = append(labels, titled.Title())
	}
	if described, ok := item.(interface{ Description() string }); ok {
		if description := described.Description(); description != "" {
			labels = append(labels, description)
		}
	}

	return labels
}

func (m Model) canvasMouseHeight() int {
	panel := m.styles.inactivePane
	canvasRect, _ := m.layoutCanvasRect()
	innerHeight := max(1, canvasRect.h-panel.GetVerticalFrameSize())
	return innerHeight
}

// restartHintAt reports a click on the stale-daemon message, which sits at the
// right end of the tab row.
func (m Model) restartHintAt(x, y int) bool {
	if !m.daemonNeedsRestart() || y != m.appContentY() {
		return false
	}
	width := m.footerContentWidth()
	hint := lipgloss.Width(m.restartHint())
	localX := x - m.appContentX()
	return localX > width-hint-2 && localX <= width
}

func (m Model) tabAt(x, y int) (mainTab, bool) {
	tabY := m.appContentY()
	tabHeight := lipgloss.Height(m.renderTabs())
	if y < tabY || y >= tabY+tabHeight {
		return tabLayout, false
	}
	localX := x - m.appContentX()
	if localX < 0 {
		return tabLayout, false
	}

	labels := []string{"Layout", "Profiles", "Workspaces"}
	cursorX := 1
	for idx, label := range labels {
		width := lipgloss.Width(fmt.Sprintf(" %d %s ", idx+1, label))
		if localX >= cursorX && localX < cursorX+width {
			return mainTab(idx), true
		}
		cursorX += width + 1
	}
	return tabLayout, false
}

func (m Model) inCanvas(x, y int, canvasRect hitRect, layout canvasGeometry) bool {
	localX, localY := m.canvasLocalPoint(x, y, canvasRect)
	return localX >= 0 && localX < layout.width && localY >= 0 && localY < layout.height
}

func (m Model) canvasLocalPoint(x, y int, canvasRect hitRect) (int, int) {
	inner := canvasRect.inner(m.styles.inactivePane)
	canvasX := inner.x
	canvasY := inner.y
	return x - canvasX, y - canvasY
}

func (m Model) inspectorFieldAt(y int, inspectorRect hitRect, compact bool, wasFocused bool) (int, bool) {
	if len(m.editOutputs) == 0 {
		return 0, false
	}
	inner := inspectorRect.inner(m.styles.inactivePane)
	localY := y - inner.y
	if localY < 0 || localY >= inner.h {
		return 0, false
	}

	layout := m.buildInspectorLayout(m.editOutputs[m.selectedOutput], inner.w, compact)
	scrollOffset := 0
	if wasFocused {
		if row, ok := layout.fieldRows[m.inspectorField]; ok {
			scrollOffset = inspectorScrollOffset(len(layout.lines), row, inner.h)
		}
	}

	for idx := range layoutFields {
		row, ok := layout.fieldRows[idx]
		if !ok {
			continue
		}
		if row-scrollOffset == localY {
			return idx, true
		}
	}
	return 0, false
}

func (m Model) canvasLayout(width, height int) canvasGeometry {
	return canvasLayoutFor(m.editOutputs, width, height)
}

// canvasLayoutFor scales a set of outputs into a terminal-cell rectangle. The
// layout tab passes its editor outputs; the profile and workspace previews
// pass the outputs they want to show, so every canvas keeps the same geometry.
func canvasLayoutFor(outputs []editableOutput, width, height int) canvasGeometry {
	layout := canvasGeometry{
		width:  max(20, width-2),
		height: max(3, height),
		cellW:  2.2,
	}

	enabled := make([]editableOutput, 0, len(outputs))
	for _, output := range outputs {
		if output.Enabled && output.MirrorOf == "" {
			enabled = append(enabled, output)
		}
	}
	if len(enabled) == 0 {
		return layout
	}

	minX, minY := enabled[0].X, enabled[0].Y
	w0, h0 := enabled[0].logicalSize()
	maxX, maxY := enabled[0].X+w0, enabled[0].Y+h0
	for _, output := range enabled[1:] {
		w, h := output.logicalSize()
		minX = min(minX, output.X)
		minY = min(minY, output.Y)
		maxX = max(maxX, output.X+w)
		maxY = max(maxY, output.Y+h)
	}

	rangeW := max(1, maxX-minX)
	rangeH := max(1, maxY-minY)
	scaleX := float64(layout.width-4) / (float64(rangeW) * layout.cellW)
	scaleY := float64(layout.height-4) / float64(rangeH)
	layout.scale = math.Min(scaleX, scaleY)
	if layout.scale <= 0 {
		layout.scale = 1
	}
	contentW := int(math.Round(float64(rangeW) * layout.scale * layout.cellW))
	contentH := int(math.Round(float64(rangeH) * layout.scale))
	layout.offsetX = max(1, 1+(layout.width-2-contentW)/2)
	layout.offsetY = max(1, 1+(layout.height-2-contentH)/2)
	layout.ok = true

	for idx, output := range outputs {
		if !output.Enabled || output.MirrorOf != "" {
			continue
		}
		w, h := output.logicalSize()
		rx := layout.offsetX + int(math.Round(float64(output.X-minX)*layout.scale*layout.cellW))
		ry := layout.offsetY + int(math.Round(float64(output.Y-minY)*layout.scale))
		rw := max(8, int(math.Round(float64(w)*layout.scale*layout.cellW)))
		rh := max(3, int(math.Round(float64(h)*layout.scale)))

		if rx+rw >= layout.width {
			rw = max(4, layout.width-rx-1)
		}
		if ry+rh >= layout.height {
			rh = max(3, layout.height-ry-1)
		}

		layout.rects = append(layout.rects, canvasRect{index: idx, x: rx, y: ry, w: rw, h: rh})
	}
	return layout
}

func (g canvasGeometry) rectAt(x, y int) (canvasRect, bool) {
	for _, rect := range g.rects {
		if x >= rect.x && x < rect.x+rect.w && y >= rect.y && y < rect.y+rect.h {
			return rect, true
		}
	}
	return canvasRect{}, false
}

func cellsToWorldX(delta int, scale float64, cellW float64) int {
	if delta == 0 {
		return 0
	}
	if scale <= 0 {
		scale = 1
	}
	if cellW <= 0 {
		cellW = 1
	}
	value := int(math.Round(float64(delta) / (scale * cellW)))
	if value == 0 {
		if delta > 0 {
			return 1
		}
		return -1
	}
	return value
}

func cellsToWorldY(delta int, scale float64) int {
	if delta == 0 {
		return 0
	}
	if scale <= 0 {
		scale = 1
	}
	value := int(math.Round(float64(delta) / scale))
	if value == 0 {
		if delta > 0 {
			return 1
		}
		return -1
	}
	return value
}

func modalHeight(lines int) int {
	return lines + 4
}

func defaultHeight(height int) int {
	if height <= 0 {
		return 28
	}
	return height
}

func (m Model) modePickerHeight() int {
	return clampInt(defaultHeight(m.height)-14, 6, 10)
}

func (m Model) terminalWidth() int {
	if m.width <= 0 {
		return 100
	}
	return max(28, m.width)
}

func (m Model) terminalHeight() int {
	if m.height <= 0 {
		return 28
	}
	return max(12, m.height)
}

func (m Model) modalMaxWidth() int {
	return max(24, m.terminalWidth()-6)
}

func (m Model) modePickerWidth() int {
	return clampInt(m.modalMaxWidth()-6, 24, 44)
}

func (m Model) saveDialogInputWidth() int {
	return clampInt(m.modalMaxWidth()-18, 16, 28)
}

func (m Model) saveDialogListWidth() int {
	return clampInt(m.modalMaxWidth()-6, 24, 52)
}

func (m Model) layoutPaneWidths() (int, int) {
	return splitPaneWidths(m.terminalWidth(), 66, 18)
}

func (m Model) sidePaneWidths(leftPercent int) (int, int) {
	return splitPaneWidths(m.terminalWidth(), leftPercent, 16)
}

const paneGapWidth = 0

func splitPaneWidths(total int, leftPercent int, minPane int) (int, int) {
	available := max(2, total-paneGapWidth)
	left := (available * leftPercent) / 100
	right := available - left
	if available >= minPane*2 {
		if left < minPane {
			left = minPane
			right = available - left
		}
		if right < minPane {
			right = minPane
			left = available - right
		}
	}
	return max(1, left), max(1, right)
}

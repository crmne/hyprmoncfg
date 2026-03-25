package tui

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type pickerItem string

func (i pickerItem) FilterValue() string { return string(i) }
func (i pickerItem) Title() string       { return string(i) }
func (i pickerItem) Description() string { return "" }

type modePickerState struct {
	OutputIndex int
	List        list.Model
}

type numericInputKind int

const (
	numericInputScale numericInputKind = iota
	numericInputPositionX
	numericInputPositionY
)

type numericInputState struct {
	Kind        numericInputKind
	OutputIndex int
	Title       string
	Hint        string
	Input       textinput.Model
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

type saveDialogState struct {
	Input  textinput.Model
	List   list.Model
	All    []profileListItem
	Filter string
}

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

func (m Model) renderTitleBar() string {
	badges := []string{
		m.unsavedBadge(),
	}
	return lipgloss.JoinHorizontal(
		lipgloss.Left,
		m.styles.title.Render("hyprmoncfg"),
		" ",
		m.styles.subtitle.Render("Hyprland monitor layout and workspace planner"),
		"  ",
		lipgloss.JoinHorizontal(lipgloss.Left, badges...),
	)
}

func (m Model) renderModalFrame(title string, body []string) string {
	lines := []string{m.styles.modalTitle.Render(title)}
	if len(body) > 0 {
		lines = append(lines, "", strings.Join(body, "\n"))
	}
	return m.styles.modal.Render(strings.Join(lines, "\n"))
}

func (m Model) renderModalScreen(overlay string) string {
	if strings.TrimSpace(overlay) == "" {
		return m.renderMain()
	}

	width := m.width
	if width <= 0 {
		width = 100
	}
	height := m.height
	if height <= 0 {
		height = 28
	}

	title := m.renderTitleBar()
	tabs := m.renderTabs()
	bodyHeight := max(12, height-lipgloss.Height(title)-lipgloss.Height(tabs)-3)
	centered := lipgloss.Place(width-2, bodyHeight, lipgloss.Center, lipgloss.Center, overlay)
	body := m.styles.modalBackdrop.Width(width).Height(bodyHeight).Render(centered)
	return strings.Join([]string{title, tabs, "", body}, "\n")
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
	if m.dirty {
		return m.styles.badgeAccent.Render("Unsaved Changes")
	}
	return m.styles.badgeMuted.Render("Current setup")
}

func (m Model) activateInspectorField() (tea.Model, tea.Cmd) {
	if len(m.editOutputs) == 0 {
		return m, nil
	}

	switch m.inspectorField {
	case 1:
		output := m.editOutputs[m.selectedOutput]
		if len(output.Modes) == 0 {
			return m, nil
		}
		items := make([]list.Item, 0, len(output.Modes))
		for _, mode := range output.Modes {
			items = append(items, pickerItem(mode))
		}
		delegate := list.NewDefaultDelegate()
		delegate.SetHeight(1)
		delegate.SetSpacing(0)
		delegate.Styles.NormalTitle = m.styles.value
		delegate.Styles.SelectedTitle = m.styles.focused.Copy().UnsetPadding()
		delegate.Styles.DimmedTitle = m.styles.subtle
		delegate.Styles.FilterMatch = m.styles.badgeAccent
		picker := list.New(items, delegate, 44, m.modePickerHeight())
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
			List:        picker,
		}
		m.mode = modeModePicker
		return m, nil
	case 2:
		output := m.editOutputs[m.selectedOutput]
		input := textinput.New()
		input.Prompt = ""
		input.Placeholder = "1.00"
		input.CharLimit = 8
		input.Width = 12
		input.TextStyle = m.styles.value
		input.PlaceholderStyle = m.styles.subtle
		input.Cursor.Style = m.styles.focused
		input.SetValue(strconv.FormatFloat(output.Scale, 'f', 2, 64))
		cmd := input.Focus()
		m.input = &numericInputState{
			Kind:        numericInputScale,
			OutputIndex: m.selectedOutput,
			Title:       fmt.Sprintf("Set Scale for %s", output.Name),
			Hint:        "Type a scale like 1, 1.25, or 1.67. Enter applies. Esc cancels.",
			Input:       input,
		}
		m.mode = modeNumericInput
		return m, cmd
	case 5, 6:
		output := m.editOutputs[m.selectedOutput]
		input := textinput.New()
		input.Prompt = ""
		input.CharLimit = 8
		input.Width = 12
		input.TextStyle = m.styles.value
		input.PlaceholderStyle = m.styles.subtle
		input.Cursor.Style = m.styles.focused

		kind := numericInputPositionX
		title := fmt.Sprintf("Set Position X for %s", output.Name)
		hint := "Type the exact X position in logical pixels. Enter applies. Esc cancels."
		value := strconv.Itoa(output.X)
		if m.inspectorField == 6 {
			kind = numericInputPositionY
			title = fmt.Sprintf("Set Position Y for %s", output.Name)
			hint = "Type the exact Y position in logical pixels. Enter applies. Esc cancels."
			value = strconv.Itoa(output.Y)
		}

		input.SetValue(value)
		cmd := input.Focus()
		m.input = &numericInputState{
			Kind:        kind,
			OutputIndex: m.selectedOutput,
			Title:       title,
			Hint:        hint,
			Input:       input,
		}
		m.mode = modeNumericInput
		return m, cmd
	default:
		m.adjustInspectorField(1)
		m.dirty = true
		return m, nil
	}
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

	body := []string{
		m.styles.subtle.Render(m.input.Hint),
		"",
		fmt.Sprintf("%s %s", m.styles.label.Render("Value"), m.styles.focused.Render(m.input.Input.View())),
	}
	if m.input.Input.Err != nil {
		body = append(body, "", m.styles.statusError.Render(m.input.Input.Err.Error()))
	}
	return m.renderModalFrame(m.input.Title, body)
}

func (m *Model) openSaveDialog() (tea.Model, tea.Cmd) {
	input := textinput.New()
	input.Prompt = ""
	input.CharLimit = 64
	input.Width = 28
	input.TextStyle = m.styles.value
	input.PlaceholderStyle = m.styles.subtle
	input.Cursor.Style = m.styles.focused
	input.SetValue(defaultProfileName())
	cmd := input.Focus()

	items := make([]profileListItem, 0, len(m.profiles))
	for _, prof := range m.profiles {
		items = append(items, profileListItem{name: prof.Name, updated: prof.UpdatedAt, outputs: len(prof.Outputs)})
	}

	delegate := list.NewDefaultDelegate()
	delegate.Styles.NormalTitle = m.styles.value
	delegate.Styles.NormalDesc = m.styles.subtle
	delegate.Styles.SelectedTitle = m.styles.focused.Copy().UnsetPadding()
	delegate.Styles.SelectedDesc = m.styles.subtle.Copy().Foreground(lipgloss.Color("#DDEAFF"))
	delegate.Styles.DimmedTitle = m.styles.subtle
	delegate.Styles.DimmedDesc = m.styles.subtle
	delegate.Styles.FilterMatch = m.styles.badgeAccent

	listHeight := clampInt(defaultHeight(m.height)-18, 5, 10)
	profileList := list.New(nil, delegate, 52, listHeight)
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
		Input:  input,
		List:   profileList,
		All:    items,
		Filter: "",
	}
	m.mode = modeSave
	m.saveOverwrite = ""
	m.rebuildSaveList(false)
	return m, cmd
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
	if resetSelection {
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
		m.mode = modeMain
		m.saveDialog = nil
		m.saveOverwrite = ""
		return m, nil
	case "enter":
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
	case "up", "down", "j", "k", "pgup", "pgdown", "home", "end":
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

func (m Model) updateModePickerKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
		return m.commitModePicker()
	}

	var cmd tea.Cmd
	m.picker.List, cmd = m.picker.List.Update(msg)
	return m, cmd
}

func (m Model) commitModePicker() (tea.Model, tea.Cmd) {
	if m.picker == nil {
		m.mode = modeMain
		return m, nil
	}
	if m.picker.OutputIndex < 0 || m.picker.OutputIndex >= len(m.editOutputs) {
		m.picker = nil
		m.mode = modeMain
		return m, nil
	}
	selected, ok := m.picker.List.SelectedItem().(pickerItem)
	if !ok {
		m.picker = nil
		m.mode = modeMain
		return m, nil
	}

	output := &m.editOutputs[m.picker.OutputIndex]
	output.ModeIndex = indexOf(output.Modes, string(selected))
	if output.ModeIndex < 0 {
		output.ModeIndex = 0
	}
	output.applyMode(output.Modes[output.ModeIndex])
	m.dirty = true
	m.setStatusOK(fmt.Sprintf("Selected %s for %s", output.DisplayMode(), output.Name))
	m.picker = nil
	m.mode = modeMain
	return m, nil
}

func (m Model) updateNumericInputKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
		return m.commitNumericInput()
	}

	var cmd tea.Cmd
	m.input.Input, cmd = m.input.Input.Update(msg)
	return m, cmd
}

func (m Model) commitNumericInput() (tea.Model, tea.Cmd) {
	if m.input == nil {
		m.mode = modeMain
		return m, nil
	}
	if m.input.OutputIndex < 0 || m.input.OutputIndex >= len(m.editOutputs) {
		m.input = nil
		m.mode = modeMain
		return m, nil
	}

	switch m.input.Kind {
	case numericInputScale:
		value, err := strconv.ParseFloat(strings.TrimSpace(m.input.Input.Value()), 64)
		if err != nil {
			m.input.Input.Err = fmt.Errorf("scale must be a number")
			return m, nil
		}
		value = clampFloat(value, 0.25, 4.0)
		m.editOutputs[m.input.OutputIndex].Scale = value
		m.dirty = true
		m.setStatusOK(fmt.Sprintf("Scale set to %.2f for %s", value, m.editOutputs[m.input.OutputIndex].Name))
	case numericInputPositionX:
		value, err := strconv.Atoi(strings.TrimSpace(m.input.Input.Value()))
		if err != nil {
			m.input.Input.Err = fmt.Errorf("position must be an integer")
			return m, nil
		}
		m.editOutputs[m.input.OutputIndex].X = value
		m.dirty = true
		m.setStatusOK(fmt.Sprintf("Position X set to %d for %s", value, m.editOutputs[m.input.OutputIndex].Name))
	case numericInputPositionY:
		value, err := strconv.Atoi(strings.TrimSpace(m.input.Input.Value()))
		if err != nil {
			m.input.Input.Err = fmt.Errorf("position must be an integer")
			return m, nil
		}
		m.editOutputs[m.input.OutputIndex].Y = value
		m.dirty = true
		m.setStatusOK(fmt.Sprintf("Position Y set to %d for %s", value, m.editOutputs[m.input.OutputIndex].Name))
	}

	m.input = nil
	m.mode = modeMain
	return m, nil
}

func (m Model) updateMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeSave:
		return m.updateSaveMouse(msg)
	case modeModePicker:
		return m.updateModePickerMouse(msg)
	case modeNumericInput, modeSaveConfirm, modeConfirm:
		return m, nil
	}

	if msg.Action == tea.MouseActionRelease {
		if m.drag != nil {
			m.selectedOutput = m.drag.OutputIndex
			cmd := m.showSnapHint(m.applySelectedSnap(36))
			m.dirty = true
			m.drag = nil
			return m, cmd
		}
		m.drag = nil
		return m, nil
	}

	if tab, ok := m.tabAt(msg.X, msg.Y); ok && msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
		m.tab = tab
		return m, nil
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
	if msg.Action != tea.MouseActionPress && msg.Action != tea.MouseActionMotion {
		return m, nil
	}
	var cmd tea.Cmd
	m.saveDialog.List, cmd = m.saveDialog.List.Update(msg)
	if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
		m.syncSaveNameFromSelection()
	}
	return m, cmd
}

func (m Model) updateModePickerMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.picker == nil {
		return m, nil
	}

	var cmd tea.Cmd
	m.picker.List, cmd = m.picker.List.Update(msg)
	if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
		return m.commitModePicker()
	}
	return m, cmd
}

func (m Model) updateLayoutMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	bodyY := m.bodyOriginY()
	canvasWidth := max(48, (m.width*2)/3)
	inspectorWidth := max(34, m.width-canvasWidth-6)
	layout := m.canvasLayout(canvasWidth-4, clampInt(m.height-12, 12, 26))

	if m.inCanvas(msg.X, msg.Y, bodyY, canvasWidth, layout) {
		m.layoutFocus = layoutFocusCanvas
		localX, localY := m.canvasLocalPoint(msg.X, msg.Y, bodyY)
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
				m.dirty = true
			}
		}
		return m, nil
	}

	inspectorX := canvasWidth + 2
	if msg.X >= inspectorX && msg.X < inspectorX+inspectorWidth {
		m.layoutFocus = layoutFocusInspector
		if field, ok := m.inspectorFieldAt(msg.Y, bodyY); ok && msg.Action == tea.MouseActionPress {
			m.inspectorField = field
			switch msg.Button {
			case tea.MouseButtonLeft:
				if field == 0 || field == 1 || field == 2 {
					return m.activateInspectorField()
				}
			case tea.MouseButtonWheelUp:
				m.adjustInspectorField(1)
				m.dirty = true
			case tea.MouseButtonWheelDown:
				m.adjustInspectorField(-1)
				m.dirty = true
			}
		}
	}

	return m, nil
}

func (m Model) updateProfilesMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return m, nil
	}

	bodyY := m.bodyOriginY()
	row := msg.Y - (bodyY + 3)
	if row < 0 || row >= len(m.profiles) {
		return m, nil
	}
	m.selectedProfile = row
	return m, nil
}

func (m Model) updateWorkspaceMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	bodyY := m.bodyOriginY()
	leftWidth := max(40, m.width/3)

	if msg.X < 0 || msg.X >= leftWidth || msg.Y < bodyY {
		return m, nil
	}

	fieldRow := msg.Y - (bodyY + 3)
	if fieldRow >= 0 && fieldRow < len(workspaceFields) && msg.Action == tea.MouseActionPress {
		m.workspaceEdit.SelectedField = fieldRow
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.adjustWorkspaceField(1)
			m.dirty = true
		case tea.MouseButtonWheelDown:
			m.adjustWorkspaceField(-1)
			m.dirty = true
		}
		return m, nil
	}

	orderStart := bodyY + 3 + len(workspaceFields) + 2
	if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft && msg.Y >= orderStart {
		row := msg.Y - orderStart - 1
		if row >= 0 && row < len(m.workspaceEdit.MonitorOrder) {
			m.workspaceEdit.SelectedOrder = row
		}
	}
	return m, nil
}

func (m Model) bodyOriginY() int {
	return lipgloss.Height(m.renderTitleBar()) + lipgloss.Height(m.renderTabs()) + 1
}

func (m Model) tabAt(x, y int) (mainTab, bool) {
	titleH := lipgloss.Height(m.renderTitleBar())
	tabY := titleH
	tabHeight := lipgloss.Height(m.renderTabs())
	if y < tabY || y >= tabY+tabHeight {
		return tabLayout, false
	}

	labels := []string{"Layout", "Profiles", "Workspaces"}
	cursorX := 0
	for idx, label := range labels {
		style := m.styles.tabInactive
		if int(m.tab) == idx {
			style = m.styles.tabActive
		}
		width := lipgloss.Width(style.Render(fmt.Sprintf("%d %s", idx+1, label)))
		if x >= cursorX && x < cursorX+width {
			return mainTab(idx), true
		}
		cursorX += width
	}
	return tabLayout, false
}

func (m Model) inCanvas(x, y, bodyY, canvasWidth int, layout canvasGeometry) bool {
	localX, localY := m.canvasLocalPoint(x, y, bodyY)
	return localX >= 0 && localX < layout.width && localY >= 0 && localY < layout.height
}

func (m Model) canvasLocalPoint(x, y, bodyY int) (int, int) {
	canvasX := 2
	canvasY := bodyY + 3
	return x - canvasX, y - canvasY
}

func (m Model) inspectorFieldAt(y, bodyY int) (int, bool) {
	if len(m.editOutputs) == 0 {
		return 0, false
	}
	row := bodyY + 6
	if strings.TrimSpace(m.editOutputs[m.selectedOutput].Description) != "" {
		row++
	}
	for idx := range layoutFields {
		if y == row+idx {
			return idx, true
		}
	}
	return 0, false
}

func (m Model) canvasLayout(width, height int) canvasGeometry {
	layout := canvasGeometry{
		width:  clampInt(width-2, 30, 120),
		height: clampInt(height, 10, 30),
		cellW:  2.2,
	}

	enabled := make([]editableOutput, 0, len(m.editOutputs))
	for _, output := range m.editOutputs {
		if output.Enabled {
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

	for idx, output := range m.editOutputs {
		if !output.Enabled {
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

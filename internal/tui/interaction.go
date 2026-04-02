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

	"github.com/crmne/hyprmoncfg/internal/profile"
	"github.com/crmne/hyprmoncfg/internal/apply"
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
	Input  textinput.Model
	List   list.Model
	All    []profileListItem
	Filter string
	Action saveAction
}

type saveAction int

const (
	saveActionOnly saveAction = iota
	saveActionApply
	saveActionCancel
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

func (m Model) renderTitleBar() string {
	width := m.footerContentWidth()
	titleText := m.styles.title.Underline(true).Render("hyprmoncfg")
	title := osc8Link(homeURL, titleText)

	if width < 68 {
		return title
	}

	subtitleText := "Hyprland monitor layout and workspace planner"
	if width < 104 {
		subtitleText = "Hyprland monitor planner"
	}

	// Only show daemon status when it needs attention
	if !m.daemonOK {
		daemonLabel := m.daemonStatusLabel()
		daemonStyle := m.styles.statusError.Underline(true)
		daemon := osc8Link(daemonURL, daemonStyle.Render(daemonLabel))
		daemonWidth := lipgloss.Width(daemonLabel)

		subtitleBudget := max(12, width-lipgloss.Width(title)-daemonWidth-6)
		subtitle := m.styles.subtitle.Render(fitString(subtitleText, subtitleBudget))

		left := lipgloss.JoinHorizontal(lipgloss.Left, title, " ", subtitle)
		gap := max(1, width-lipgloss.Width(left)-daemonWidth)
		return left + strings.Repeat(" ", gap) + daemon
	}

	subtitleBudget := max(12, width-lipgloss.Width(title)-4)
	subtitle := m.styles.subtitle.Render(fitString(subtitleText, subtitleBudget))
	return lipgloss.JoinHorizontal(lipgloss.Left, title, " ", subtitle)
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

	title := m.renderTitleBar()
	tabs := m.renderTabs()
	bodyHeight := max(12, height-lipgloss.Height(title)-lipgloss.Height(tabs)-2)
	centered := lipgloss.Place(width-2, bodyHeight, lipgloss.Center, lipgloss.Center, overlay)
	body := m.styles.modalBackdrop.Width(width).Height(bodyHeight).Render(centered)
	return strings.Join([]string{title, tabs, body}, "\n")
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
		inner := list.NewDefaultDelegate()
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
		input.Width = clampInt(m.modalMaxWidth()-16, 8, 12)
		input.TextStyle = m.styles.value
		input.PlaceholderStyle = m.styles.subtle
		input.Cursor.Style = lipgloss.NewStyle()
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
		input.Width = clampInt(m.modalMaxWidth()-16, 8, 12)
		input.TextStyle = m.styles.value
		input.PlaceholderStyle = m.styles.subtle
		input.Cursor.Style = lipgloss.NewStyle()

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
		m.markDirty()
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
	if m.input.Input.Err != nil {
		body = append(body, "", m.styles.statusError.Render(m.input.Input.Err.Error()))
	}
	return m.renderModalFrame(m.input.Title, body)
}

func (m *Model) openSaveDialog() (tea.Model, tea.Cmd) {
	input := textinput.New()
	input.Prompt = ""
	input.CharLimit = 64
	input.Width = m.saveDialogInputWidth()
	input.TextStyle = m.styles.value
	input.PlaceholderStyle = m.styles.subtle
	input.Cursor.Style = lipgloss.NewStyle()
	name := defaultProfileName()
	if suggested := strings.TrimSpace(m.draftProfileName); suggested != "" {
		name = suggested
	}
	input.SetValue(name)
	cmd := input.Focus()

	items := make([]profileListItem, 0, len(m.profiles))
	for _, prof := range m.profiles {
		items = append(items, profileListItem{name: prof.Name, updated: prof.UpdatedAt, outputs: len(prof.Outputs)})
	}

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
		Input:  input,
		List:   profileList,
		All:    items,
		Filter: "",
		Action: saveActionApply,
	}
	m.mode = modeSave
	m.saveOverwrite = ""
	m.rebuildSaveList(false)
	return m, cmd
}

func (m *Model) cycleSaveAction(delta int) {
	if m.saveDialog == nil {
		return
	}
	actions := []saveAction{saveActionOnly, saveActionApply, saveActionCancel}
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
	case saveActionCancel:
		return "Cancel"
	default:
		return "Save"
	}
}

func (m Model) renderSaveActionButtons() string {
	actions := []saveAction{saveActionOnly, saveActionApply, saveActionCancel}
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
	case "tab":
		m.cycleSaveAction(1)
		return m, nil
	case "shift+tab":
		m.cycleSaveAction(-1)
		return m, nil
	case "enter":
		if m.selectedSaveAction() == saveActionCancel {
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

func (m *Model) commitModePicker() (tea.Model, tea.Cmd) {
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
	m.markDirty()
	m.setStatusOK(fmt.Sprintf("Selected %s for %s", output.DisplayMode(), output.Name))

	m.revalidate()

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
		m.markDirty()
		m.setStatusOK(fmt.Sprintf("Scale set to %.2f for %s", value, m.editOutputs[m.input.OutputIndex].Name))
	case numericInputPositionX:
		value, err := strconv.Atoi(strings.TrimSpace(m.input.Input.Value()))
		if err != nil {
			m.input.Input.Err = fmt.Errorf("position must be an integer")
			return m, nil
		}
		m.editOutputs[m.input.OutputIndex].X = value
		m.markDirty()
		m.setStatusOK(fmt.Sprintf("Position X set to %d for %s", value, m.editOutputs[m.input.OutputIndex].Name))
	case numericInputPositionY:
		value, err := strconv.Atoi(strings.TrimSpace(m.input.Input.Value()))
		if err != nil {
			m.input.Input.Err = fmt.Errorf("position must be an integer")
			return m, nil
		}
		m.editOutputs[m.input.OutputIndex].Y = value
		m.markDirty()
		m.setStatusOK(fmt.Sprintf("Position Y set to %d for %s", value, m.editOutputs[m.input.OutputIndex].Name))
	}

	m.revalidate()

	m.input = nil
	m.mode = modeMain
	return m, nil
}

func (m *Model) updateMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
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

			m.revalidate()

			m.markDirty()
			m.drag = nil
			return m, cmd
		}
		m.drag = nil
		return m, nil
	}

	if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft && msg.Y == m.appContentY() {
		titleWidth := lipgloss.Width(m.styles.title.Render("hyprmoncfg"))
		titleStart := m.appContentX()
		if msg.X >= titleStart && msg.X < titleStart+titleWidth {
			return m, m.openURLCmd("hyprmoncfg", homeURL)
		}
		// Daemon status link on the right side of title bar
		daemonWidth := lipgloss.Width(m.daemonStatusLabel())
		contentWidth := m.footerContentWidth()
		daemonStart := m.appContentX() + contentWidth - daemonWidth
		if msg.X >= daemonStart && msg.X < daemonStart+daemonWidth {
			return m, m.openURLCmd(m.daemonStatusLabel(), daemonURL)
		}
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

func (m Model) updateModePickerMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.picker == nil {
		return m, nil
	}

	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return m, nil
	}

	if index, ok := m.modePickerItemIndexAt(msg.X, msg.Y); ok {
		m.picker.List.Select(index)
		return m.commitModePicker()
	}

	return m, nil
}

func (m *Model) updateLayoutMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	canvasRect, _ := m.layoutCanvasRect()
	inspectorRect, compact := m.layoutInspectorRect()
	layout := m.canvasLayout(canvasRect.w-m.styles.inactivePane.GetHorizontalFrameSize(), m.canvasMouseHeight())

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

				m.revalidate()

				m.markDirty()
			}
		}
		return m, nil
	}

	if inspectorRect.contains(msg.X, msg.Y) {
		m.layoutFocus = layoutFocusInspector
		if field, ok := m.inspectorFieldAt(msg.Y, inspectorRect, compact); ok && msg.Action == tea.MouseActionPress {
			m.inspectorField = field
			switch msg.Button {
			case tea.MouseButtonLeft:
				return m.activateInspectorField()
			case tea.MouseButtonWheelUp:
				m.adjustInspectorField(1)
				m.markDirty()
			case tea.MouseButtonWheelDown:
				m.adjustInspectorField(-1)
				m.markDirty()
			}
		}
	}

	return m, nil
}

func (m Model) updateProfilesMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return m, nil
	}

	listRect := m.profilesListRect()
	if !listRect.contains(msg.X, msg.Y) {
		return m, nil
	}

	row := msg.Y - (listRect.inner(m.styles.activePane).y + 2)
	if row < 0 || row >= len(m.profiles) {
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
	fieldRow := msg.Y - (inner.y + 2)
	if fieldRow >= 0 && fieldRow < len(workspaceFields) && msg.Action == tea.MouseActionPress {
		m.workspaceEdit.SelectedField = fieldRow
		switch msg.Button {
		case tea.MouseButtonLeft:
			m.adjustWorkspaceField(1)
			m.markDirty()
		case tea.MouseButtonWheelUp:
			m.adjustWorkspaceField(1)
			m.markDirty()
		case tea.MouseButtonWheelDown:
			m.adjustWorkspaceField(-1)
			m.markDirty()
		}
		return m, nil
	}

	// "Monitor order" header + items start after fields + 2 lines (blank + header)
	orderStart := inner.y + len(workspaceFields) + 4
	if msg.Action == tea.MouseActionPress && msg.Y >= orderStart {
		row := msg.Y - orderStart
		if row >= 0 && row < len(m.workspaceEdit.MonitorOrder) {
			m.workspaceEdit.SelectedField = len(workspaceFields) + row
			m.workspaceEdit.SelectedOrder = row
			m.markDirty()
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
	title := m.renderTitleBar()
	tabs := m.renderTabs()
	footer := m.renderFooterBar()
	return hitRect{
		x: m.appContentX(),
		y: m.appContentY() + lipgloss.Height(title) + lipgloss.Height(tabs),
		w: m.footerContentWidth(),
		h: m.mainBodyHeight(title, tabs, "", footer),
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
		return hitRect{x: body.x, y: body.y + canvasHeight, w: body.w, h: inspectorHeight}, true
	}

	canvasWidth, inspectorWidth := m.layoutPaneWidths()
	return hitRect{x: body.x + canvasWidth + 2, y: body.y, w: inspectorWidth, h: body.h}, false
}

func (m Model) profilesListRect() hitRect {
	body := m.bodyRect()
	if m.terminalWidth() < 96 {
		listHeight := clampInt(len(m.profiles)+3, 4, body.h/3)
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
	count := 2 + len(workspaceFields) + 2
	if len(m.workspaceEdit.MonitorOrder) == 0 {
		count++
	} else {
		count += len(m.workspaceEdit.MonitorOrder)
	}
	if m.workspaceEdit.Strategy == profile.WorkspaceStrategyManual && len(m.workspaceEdit.Rules) > 0 {
		count += 3
	}
	return count
}

func (m Model) modalOverlayRect(overlay string) hitRect {
	if overlay == "" {
		return hitRect{}
	}

	titleHeight := lipgloss.Height(m.renderTitleBar())
	tabsHeight := lipgloss.Height(m.renderTabs())
	bodyHeight := max(12, m.terminalHeight()-titleHeight-tabsHeight-2)
	bodyWidth := m.terminalWidth() - m.styles.modalBackdrop.GetHorizontalFrameSize()

	return hitRect{
		x: m.styles.modalBackdrop.GetPaddingLeft() + max(0, (bodyWidth-lipgloss.Width(overlay))/2),
		y: titleHeight + tabsHeight + m.styles.modalBackdrop.GetPaddingTop() + max(0, (bodyHeight-lipgloss.Height(overlay))/2),
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
	innerWidth := max(1, canvasRect.w-panel.GetHorizontalFrameSize())

	nonCanvasLines := 1
	if innerHeight >= 8 && innerWidth >= 44 {
		nonCanvasLines += 2
	}
	if innerHeight >= 6 && innerWidth >= 34 {
		nonCanvasLines += 4
	}
	if innerHeight >= 10 {
		for _, output := range m.editOutputs {
			if !output.Enabled {
				nonCanvasLines++
				break
			}
		}
	}
	return max(1, innerHeight-nonCanvasLines)
}

func (m Model) tabAt(x, y int) (mainTab, bool) {
	titleH := lipgloss.Height(m.renderTitleBar())
	tabY := m.appContentY() + titleH
	tabHeight := lipgloss.Height(m.renderTabs())
	if y < tabY || y >= tabY+tabHeight {
		return tabLayout, false
	}
	localX := x - m.appContentX()
	if localX < 0 {
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
		if localX >= cursorX && localX < cursorX+width {
			return mainTab(idx), true
		}
		cursorX += width
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
	canvasY := inner.y + 1
	return x - canvasX, y - canvasY
}

func (m Model) inspectorFieldAt(y int, inspectorRect hitRect, compact bool) (int, bool) {
	if len(m.editOutputs) == 0 {
		return 0, false
	}
	inner := inspectorRect.inner(m.styles.inactivePane)
	row := inner.y + 4
	if !compact {
		output := m.editOutputs[m.selectedOutput]
		detailCount := len(m.inspectorDetailLines(output))
		// header + blank + info + details + blank + preferences
		row = inner.y + detailCount + 5
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
		width:  max(20, width-2),
		height: max(3, height),
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

func splitPaneWidths(total int, leftPercent int, minPane int) (int, int) {
	available := max(2, total-2)
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

func (m *Model) revalidate() {
	configs := make([]profile.OutputConfig, len(m.editOutputs))
	for i, out := range m.editOutputs {
		configs[i] = profile.OutputConfig{
			Name:      out.Name,
			Enabled:   out.Enabled,
			X:         out.X,
			Y:         out.Y,
			Width:     out.Width,
			Height:    out.Height,
			Scale:     out.Scale,
			Transform: out.Transform,
			MirrorOf:  out.MirrorOf,
		}
	}
	// Call the logic from apply.go
	m.layoutErr = apply.ValidateLayout(configs)
}

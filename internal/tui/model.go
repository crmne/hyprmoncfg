package tui

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/crmne/hyprmoncfg/internal/apply"
	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/lid"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

type uiMode int

const (
	modeMain uiMode = iota
	modeSave
	modeSaveConfirm
	modeConfirm
	modeModePicker
	modeNumericInput
	modeProfileExecInput
)

type mainTab int

const (
	tabLayout mainTab = iota
	tabProfiles
	tabWorkspaces
)

type layoutFocus int

const (
	layoutFocusCanvas layoutFocus = iota
	layoutFocusInspector
)

type refreshMsg struct {
	monitors       []hypr.Monitor
	profiles       []profile.Profile
	workspaceRules []hypr.WorkspaceRule
	workspaces     []hypr.WorkspaceState
	lidState       lid.State
	background     bool
	err            error
}

type saveMsg struct {
	name       string
	err        error
	profileTab bool
}

type deleteMsg struct {
	name string
	err  error
}

type applyMsg struct {
	profile  profile.Profile
	snapshot apply.RevertState
	err      error
}

type revertMsg struct {
	err    error
	reason string
}

type openURLMsg struct {
	label string
	url   string
	err   error
}

type clearToastMsg struct {
	token int
}

type clearSnapMsg struct {
	token int
}

type tickMsg time.Time

type pendingApply struct {
	profile  profile.Profile
	snapshot apply.RevertState
	deadline time.Time
}

type toastState struct {
	message string
	err     bool
	token   int
}

type editableOutput struct {
	Key               string
	MatchKey          string
	Name              string
	Description       string
	Make              string
	Model             string
	Serial            string
	PhysicalWidth     int
	PhysicalHeight    int
	Enabled           bool
	Modes             []string
	ModeIndex         int
	Width             int
	Height            int
	Refresh           float64
	X                 int
	Y                 int
	Scale             float64
	VRR               int
	Transform         int
	Focused           bool
	DPMSStatus        bool
	IsInternal        bool
	MirrorOf          string
	ActiveWorkspace   string
	Bitdepth          int
	CM                string
	SDRBrightness     float64
	SDRSaturation     float64
	SDRMinLuminance   float64
	SDRMaxLuminance   int
	MinLuminance      float64
	MaxLuminance      int
	SupportsWideColor int
	SupportsHDR       int
	MaxAvgLuminance   int
	SDREOTF           string
	ICC               string
}

type canvasCell struct {
	ch   rune
	fg   string
	bg   string
	bold bool
}

type canvasCardColors struct {
	bg     string
	border string
	fg     string
	muted  string
}

type snapEdge int

const (
	snapEdgeLeft snapEdge = iota
	snapEdgeRight
	snapEdgeTop
	snapEdgeBottom
)

type snapMark struct {
	OutputIndex int
	Edge        snapEdge
}

type snapHintState struct {
	Token int
	Marks []snapMark
}

type snapAxisCandidate struct {
	pos   int
	dist  int
	marks []snapMark
}

type snapAnalysis struct {
	x snapAxisCandidate
	y snapAxisCandidate
}

type workspaceEditor struct {
	Enabled                 bool
	Strategy                profile.WorkspaceStrategy
	MaxWorkspaces           int
	GroupSize               int
	LastSequentialGroupSize int
	MonitorOrder            []string
	Rules                   []profile.WorkspaceRule
	SelectedField           int
	SelectedOrder           int
}

type Model struct {
	client  *hypr.Client
	store   *profile.Store
	engine  apply.Engine
	openURL func(string) error

	styles styles

	mode        uiMode
	tab         mainTab
	layoutFocus layoutFocus

	monitors       []hypr.Monitor
	profiles       []profile.Profile
	workspaceRules []hypr.WorkspaceRule
	workspaces     []hypr.WorkspaceState
	lidState       lid.State

	editOutputs     []editableOutput
	workspaceEdit   workspaceEditor
	selectedOutput  int
	inspectorField  int
	selectedProfile int

	pending       *pendingApply
	saveDialog    *saveDialogState
	saveOverwrite string
	picker        *modePickerState
	input         *numericInputState
	execInput     *profileExecInputState
	drag          *canvasDragState
	toast         *toastState
	snap          *snapHintState
	snapSeq       int
	toastSeq      int

	resetRequested   bool
	status           string
	statusErr        bool
	dirty            bool
	draftSaved       bool
	draftProfileName string
	draftExec        string
	daemonOK         bool
	refreshInFlight  bool

	width  int
	height int

	layoutErr error
}

const defaultWorkspaceGroupSize = 3

func NewModel(client *hypr.Client, store *profile.Store, monitorsConfPath string, hyprlandConfigPath string) Model {
	return Model{
		client: client,
		store:  store,
		engine: apply.Engine{
			Client:             client,
			MonitorsConfPath:   monitorsConfPath,
			HyprlandConfigPath: hyprlandConfigPath,
			Logf: func(format string, args ...any) {
				fmt.Fprintf(os.Stderr, format, args...)
			},
		},
		openURL:     openExternalURL,
		styles:      newStyles(),
		mode:        modeMain,
		tab:         tabLayout,
		layoutFocus: layoutFocusCanvas,
		status:      "Loading Hyprland state...",
		workspaceEdit: workspaceEditor{
			Strategy:                profile.WorkspaceStrategySequential,
			MaxWorkspaces:           9,
			GroupSize:               defaultWorkspaceGroupSize,
			LastSequentialGroupSize: defaultWorkspaceGroupSize,
		},
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.refreshCmd(false), tickCmd())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.picker != nil {
			m.picker.List.SetSize(m.modePickerWidth(), m.modePickerHeight())
		}
		if m.saveDialog != nil {
			m.saveDialog.List.SetSize(m.saveDialogListWidth(), clampInt(defaultHeight(m.height)-18, 3, 10))
			m.saveDialog.Input.Width = m.saveDialogInputWidth()
		}
		if m.input != nil {
			m.input.Input.Width = m.numericInputWidthFor(m.input.Kind)
		}
		if m.execInput != nil {
			m.execInput.Input.Width = clampInt(m.modalMaxWidth()-16, 24, 72)
		}
		return m, nil

	case refreshMsg:
		m.refreshInFlight = false
		if msg.err != nil {
			m.setStatusErr(msg.err.Error())
			return m, nil
		}

		prevSig := m.liveConfigSignature()
		nextSig := liveConfigSignature(msg.monitors, msg.lidState)
		liveChanged := prevSig != nextSig
		wasDirty := m.dirty

		m.daemonOK = isDaemonRunning()
		m.monitors = msg.monitors
		m.profiles = msg.profiles
		m.workspaceRules = msg.workspaceRules
		m.workspaces = msg.workspaces
		m.lidState = msg.lidState

		reloadLive := len(m.editOutputs) == 0 || liveChanged || (!msg.background && !m.dirty)
		if reloadLive {
			m.loadLiveState()
			if liveChanged && wasDirty {
				m.markClean()
				m.setStatusOK("Monitor configuration changed. Reloaded live state.")
				m.syncSelections()
				return m, nil
			}
		}
		m.syncSelections()
		if !msg.background {
			m.status = ""
		}
		return m, nil

	case saveMsg:
		if msg.err != nil {
			m.setStatusErr(msg.err.Error())
			m.mode = modeMain
			return m, nil
		}
		if msg.profileTab {
			m.setStatusOK(fmt.Sprintf("Saved profile %q", msg.name))
			return m, m.refreshCmd(false)
		}
		action := saveActionOnly
		if m.saveDialog != nil {
			action = m.saveDialog.Action
		}
		m.saveDialog = nil
		m.saveOverwrite = ""
		m.draftProfileName = msg.name
		m.draftSaved = true
		m.mode = modeMain
		if action == saveActionCancel {
			m.setStatusOK("Save cancelled")
			return m, nil
		}
		if action == saveActionApply {
			return m, tea.Batch(
				m.refreshCmd(false),
				m.applyCmd(m.currentProfile(msg.name)),
			)
		}
		m.setStatusOK(fmt.Sprintf("Saved profile %q", msg.name))
		return m, m.refreshCmd(false)

	case clearSnapMsg:
		if m.snap != nil && msg.token == m.snap.Token {
			m.snap = nil
		}
		return m, nil

	case clearToastMsg:
		if m.toast != nil && msg.token == m.toast.token {
			m.toast = nil
		}
		return m, nil

	case deleteMsg:
		if msg.err != nil {
			m.setStatusErr(msg.err.Error())
			return m, nil
		}
		if strings.EqualFold(strings.TrimSpace(msg.name), strings.TrimSpace(m.draftProfileName)) {
			m.draftProfileName = ""
			m.draftExec = ""
		}
		m.setStatusOK(fmt.Sprintf("Deleted profile %q", msg.name))
		m.selectedProfile = clampIndex(m.selectedProfile, len(m.profiles))
		return m, m.refreshCmd(false)

	case applyMsg:
		if msg.err != nil {
			m.setStatusErr(msg.err.Error())
			m.mode = modeMain
			return m, nil
		}
		m.pending = &pendingApply{
			profile:  msg.profile,
			snapshot: msg.snapshot,
			deadline: time.Now().Add(10 * time.Second),
		}
		m.mode = modeConfirm
		m.statusErr = false
		m.status = fmt.Sprintf("%s applied. Changes are live until you confirm or revert.", targetLabel(msg.profile.Name))
		return m, tickCmd()

	case revertMsg:
		m.mode = modeMain
		m.pending = nil
		if msg.err != nil {
			m.setStatusErr(fmt.Sprintf("Revert failed: %v", msg.err))
			return m, nil
		}
		m.markClean()
		m.draftProfileName = ""
		m.draftExec = ""
		m.setStatusOK("Configuration reverted: " + msg.reason)
		return m, m.refreshCmd(false)

	case openURLMsg:
		if msg.err != nil {
			m.setStatusErr(fmt.Sprintf("Failed to open %s link: %v", msg.label, msg.err))
		}
		return m, nil

	case tickMsg:
		m.daemonOK = isDaemonRunning()
		if m.mode == modeConfirm && m.pending != nil {
			if time.Now().After(m.pending.deadline) {
				snapshot := m.pending.snapshot
				return m, m.revertCmd(snapshot, "timeout")
			}
		}
		cmds := []tea.Cmd{tickCmd()}
		if !m.refreshInFlight {
			m.refreshInFlight = true
			cmds = append(cmds, m.refreshCmd(true))
		}
		return m, tea.Batch(cmds...)

	case tea.KeyMsg:
		switch m.mode {
		case modeSave:
			return m.updateSaveKeys(msg)
		case modeSaveConfirm:
			return m.updateSaveConfirmKeys(msg)
		case modeConfirm:
			return m.updateConfirmKeys(msg)
		case modeModePicker:
			return m.updateModePickerKeys(msg)
		case modeNumericInput:
			return m.updateNumericInputKeys(msg)
		case modeProfileExecInput:
			return m.updateProfileExecInputKeys(msg)
		default:
			return m.updateMainKeys(msg)
		}

	case tea.MouseMsg:
		return m.updateMouse(msg)
	}

	// Forward unhandled messages (e.g. cursor blinks) to the active text input.
	switch m.mode {
	case modeSave:
		if m.saveDialog != nil {
			var cmd tea.Cmd
			m.saveDialog.Input, cmd = m.saveDialog.Input.Update(msg)
			return m, cmd
		}
	case modeNumericInput:
		if m.input != nil {
			var cmd tea.Cmd
			m.input.Input, cmd = m.input.Input.Update(msg)
			return m, cmd
		}
	case modeProfileExecInput:
		if m.execInput != nil {
			var cmd tea.Cmd
			m.execInput.Input, cmd = m.execInput.Input.Update(msg)
			return m, cmd
		}
	}

	return m, nil
}

func (m Model) updateMainKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "1":
		m.tab = tabLayout
		return m, nil
	case "2":
		m.tab = tabProfiles
		return m, nil
	case "3":
		m.tab = tabWorkspaces
		return m, nil
	case "r":
		m.resetRequested = true
		m.draftProfileName = ""
		m.draftExec = ""
		m.markClean()
		return m, m.refreshCmd(false)
	case "s":
		if m.tab == tabProfiles {
			if len(m.profiles) == 0 {
				m.setStatusErr("No profiles to save")
				return m, nil
			}
			return m, m.saveProfileCmd(m.profiles[m.selectedProfile])
		}
		return m.openSaveDialog()
	case "a":
		if m.tab == tabProfiles {
			if len(m.profiles) == 0 {
				m.setStatusErr("No profiles available")
				return m, nil
			}
			target := m.profiles[m.selectedProfile]
			return m, m.applyCmd(target)
		}
		return m, m.applyCmd(m.currentProfile("draft"))
	}

	switch m.tab {
	case tabLayout:
		return m.updateLayoutKeys(msg)
	case tabProfiles:
		return m.updateProfileKeys(msg)
	case tabWorkspaces:
		return m.updateWorkspaceKeys(msg)
	default:
		return m, nil
	}
}

func (m *Model) updateLayoutKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if len(m.editOutputs) == 0 {
		return m, nil
	}

	switch msg.String() {
	case "tab":
		if m.layoutFocus == layoutFocusCanvas {
			m.layoutFocus = layoutFocusInspector
		} else {
			m.layoutFocus = layoutFocusCanvas
		}
		return m, nil
	case "[", "shift+tab":
		m.selectedOutput = clampIndex(m.selectedOutput-1, len(m.editOutputs))
		return m, nil
	case "]":
		m.selectedOutput = clampIndex(m.selectedOutput+1, len(m.editOutputs))
		return m, nil
	}

	if m.layoutFocus == layoutFocusCanvas {
		if dx, dy, ok := layoutMoveDelta(msg.String()); ok {
			return m, m.nudgeSelectedOutput(dx, dy, 24)
		}
		switch msg.String() {
		case " ":
			m.toggleSelectedOutput()
			return m, nil
		case "enter":
			m.layoutFocus = layoutFocusInspector
			return m, nil
		default:
			return m, nil
		}
	}

	switch msg.String() {
	case "up", "k":
		m.inspectorField = clampIndex(m.inspectorField-1, len(layoutFields))
	case "down", "j":
		m.inspectorField = clampIndex(m.inspectorField+1, len(layoutFields))
	case "left", "h", "-", "_":
		m.adjustInspectorField(-1)
	case "right", "l", "+", "=":
		m.adjustInspectorField(1)
	case " ", "enter":
		return m, m.activateInspectorField()
	default:
		return m, nil
	}

	return m, nil
}

func (m Model) updateProfileKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		m.selectedProfile = clampIndex(m.selectedProfile-1, len(m.profiles))
	case "down", "j":
		m.selectedProfile = clampIndex(m.selectedProfile+1, len(m.profiles))
	case "e":
		if len(m.profiles) == 0 {
			m.setStatusErr("No profiles to edit")
			return m, nil
		}
		return m, m.openProfileExecInput()
	case "d":
		if len(m.profiles) == 0 {
			m.setStatusErr("No profiles to delete")
			return m, nil
		}
		return m, m.deleteCmd(m.profiles[m.selectedProfile].Name)
	case "enter", "l":
		if len(m.profiles) == 0 {
			m.setStatusErr("No profiles to load")
			return m, nil
		}
		m.loadProfile(m.profiles[m.selectedProfile])
		m.tab = tabLayout
	default:
		return m, nil
	}

	return m, nil
}

func (m Model) updateWorkspaceKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	totalItems := len(workspaceFields) + len(m.workspaceEdit.MonitorOrder)
	inOrder := m.workspaceEdit.SelectedField >= len(workspaceFields)

	switch msg.String() {
	case "up":
		m.workspaceEdit.SelectedField = clampIndex(m.workspaceEdit.SelectedField-1, totalItems)
	case "down":
		m.workspaceEdit.SelectedField = clampIndex(m.workspaceEdit.SelectedField+1, totalItems)
	case "left", "h", "-", "_":
		if inOrder {
			m.moveWorkspaceOrder(-1)
		} else {
			m.adjustWorkspaceField(-1)
		}
	case "right", "l", "+", "=":
		if inOrder {
			m.moveWorkspaceOrder(1)
		} else {
			m.adjustWorkspaceField(1)
		}
	case " ", "enter":
		if inOrder {
			m.moveWorkspaceOrder(1)
		} else {
			m.adjustWorkspaceField(1)
		}
	default:
		return m, nil
	}

	// Keep SelectedOrder in sync for monitor order operations
	if m.workspaceEdit.SelectedField >= len(workspaceFields) {
		m.workspaceEdit.SelectedOrder = m.workspaceEdit.SelectedField - len(workspaceFields)
	}

	m.markDirty()
	return m, nil
}

func (m Model) updateConfirmKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.pending == nil {
		m.mode = modeMain
		return m, nil
	}

	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "y", "enter":
		var toastCmd tea.Cmd

		p := m.pending.profile
		if target := strings.TrimSpace(p.Name); target != "" && target != "draft" {
			m.draftProfileName = target
		}

		if err := m.postApply(p); err != nil {
			toastCmd = m.notifyUser(fmt.Sprintf("Post-apply failed for %q: %v", p.Name, err), true)
		}

		m.mode = modeMain
		m.pending = nil
		m.markClean()
		m.setStatusOK("Configuration kept")
		return m, tea.Batch(m.refreshCmd(false), toastCmd)
	case "n", "esc":
		snapshot := m.pending.snapshot
		return m, m.revertCmd(snapshot, "user request")
	default:
		return m, nil
	}
}

func (m Model) View() string {
	switch m.mode {
	case modeSave:
		return m.renderModalScreen(m.renderSavePrompt())
	case modeSaveConfirm:
		return m.renderModalScreen(m.renderSaveConfirm())
	case modeConfirm:
		return m.renderModalScreen(m.renderConfirm())
	case modeModePicker:
		return m.renderModalScreen(m.renderModePicker())
	case modeNumericInput:
		return m.renderModalScreen(m.renderNumericInput())
	case modeProfileExecInput:
		return m.renderModalScreen(m.renderProfileExecInput())
	default:
		return m.renderMain()
	}
}

func (m Model) renderMain() string {
	title := m.renderTitleBar()
	tabs := m.renderTabs()
	toast := m.renderToast()
	toastHeight := 0
	if toast != "" {
		toastHeight = lipgloss.Height(toast) + 1
	}

	footerText := m.renderFooterBar()
	bodyHeight := max(3, m.mainBodyHeight(title, tabs, "", footerText)-toastHeight)

	var body string
	switch m.tab {
	case tabLayout:
		body = m.renderLayoutView(bodyHeight)
	case tabProfiles:
		body = m.renderProfilesView(bodyHeight)
	case tabWorkspaces:
		body = m.renderWorkspaceView(bodyHeight)
	}
	body = lipgloss.NewStyle().Height(bodyHeight).MaxHeight(bodyHeight).Render(body)

	styledFooter := m.decorateFooterBar(footerText)
	content := strings.Join([]string{
		title,
		tabs,
		body,
	}, "\n")
	if toast != "" {
		content = strings.Join([]string{
			content,
			lipgloss.PlaceHorizontal(m.footerContentWidth(), lipgloss.Center, toast),
		}, "\n")
	}
	content = strings.Join([]string{
		content,
		styledFooter,
	}, "\n")
	app := m.styles.app
	return app.Width(max(1, m.terminalWidth()-app.GetHorizontalFrameSize())).
		Height(max(1, m.terminalHeight()-app.GetVerticalFrameSize())).
		MaxHeight(max(1, m.terminalHeight()-app.GetVerticalFrameSize())).
		Render(content)
}

func (m Model) renderTabs() string {
	labels := []string{"Layout", "Profiles", "Workspaces"}
	parts := make([]string, 0, len(labels))
	for idx, label := range labels {
		style := m.styles.tabInactive
		if int(m.tab) == idx {
			style = m.styles.tabActive
		}
		numStyle := withFG(lipgloss.NewStyle().Bold(true), "2")
		tabText := fmt.Sprintf("%s %s", numStyle.Render(fmt.Sprintf("%d", idx+1)), label)
		parts = append(parts, style.Render(tabText))
	}
	return lipgloss.JoinHorizontal(lipgloss.Bottom, parts...)
}

func (m Model) renderLayoutView(height int) string {
	if m.useCompactLayout(height) {
		canvasHeight, inspectorHeight := m.compactLayoutHeights(height)
		width := m.terminalWidth() - m.styles.app.GetHorizontalFrameSize()
		canvas := m.renderCanvasPane(width, canvasHeight)
		inspector := m.renderInspectorPane(width, inspectorHeight, true)
		return lipgloss.JoinVertical(lipgloss.Left, canvas, inspector)
	}

	canvasWidth, inspectorWidth := m.layoutPaneWidths()
	canvas := m.renderCanvasPane(canvasWidth, height)
	inspector := m.renderInspectorPane(inspectorWidth, height, false)
	return lipgloss.JoinHorizontal(lipgloss.Top, canvas, "  ", inspector)
}

func (m Model) renderCanvasPane(width int, height int) string {
	panel := m.styles.inactivePane
	if m.layoutFocus == layoutFocusCanvas && m.tab == tabLayout {
		panel = m.styles.activePane
	}
	innerWidth := max(1, width-panel.GetHorizontalFrameSize())
	innerHeight := max(1, height-panel.GetVerticalFrameSize())

	showLegend := innerHeight >= 6 && innerWidth >= 34

	nonCanvasLines := 1
	if showLegend {
		nonCanvasLines += 4
	}
	showLidState := m.lidStatusLine() != "" && innerHeight >= 8
	if showLidState {
		nonCanvasLines++
	}

	disabled := make([]string, 0)
	for _, output := range m.editOutputs {
		if !output.Enabled {
			disabled = append(disabled, output.Name)
		}
	}
	mirrors := m.mirrorSummaryLabels()
	if len(disabled) > 0 && innerHeight >= 10 {
		nonCanvasLines++
	}
	if len(mirrors) > 0 && innerHeight >= 10 {
		nonCanvasLines++
	}

	canvasHeight := max(1, innerHeight-nonCanvasLines)
	lines := []string{m.styles.header.Render("Monitor Layout")}
	if showLidState {
		lines = append(lines, m.styles.subtle.Render(m.lidStatusLine()))
	}
	lines = append(lines, m.renderCanvas(max(1, innerWidth-2), canvasHeight))

	legend := lipgloss.JoinHorizontal(
		lipgloss.Center,
		m.styles.label.Render("Legend"),
		"  ",
		renderCanvasLegendItem("Selected", m.canvasCardStyle(editableOutput{Enabled: true}, true)),
		"  ",
		renderCanvasLegendItem("Enabled", m.canvasCardStyle(editableOutput{Enabled: true}, false)),
	)
	if showLegend {
		lines = append(lines, "", legend)
	}
	if len(disabled) > 0 && innerHeight >= 10 {
		lines = append(lines, m.styles.subtle.Render("Disabled: "+strings.Join(disabled, ", ")))
	}
	if len(mirrors) > 0 && innerHeight >= 10 {
		lines = append(lines, m.styles.subtle.Render("Mirrors: "+strings.Join(mirrors, ", ")))
	}
	return panel.Width(innerWidth).Render(fitBlock(strings.Join(lines, "\n"), innerWidth, innerHeight))
}

func (m Model) lidStatusLine() string {
	switch m.lidState {
	case lid.Closed:
		return "Lid: closed · internal displays are forced off when profiles apply"
	case lid.Open:
		return "Lid: open"
	default:
		return ""
	}
}

func (m Model) renderCanvas(width, height int) string {
	if len(m.editOutputs) == 0 {
		return "(no monitors)"
	}
	if height <= 2 {
		selected := m.editOutputs[m.selectedOutput]
		lines := []string{fitString(selected.Name, width)}
		if height == 2 {
			lines = append(lines, fitString(selected.DisplayMode(), width))
		}
		return strings.Join(lines, "\n")
	}

	layout := m.canvasLayout(width, height)
	if !layout.ok {
		if m.hasMirroredOutputs() {
			return "(mirrors shown below)"
		}
		return "(all monitors disabled)"
	}

	canvasW := layout.width
	canvasH := layout.height

	grid := m.newCanvasCells(canvasW, canvasH)

	rects := append([]canvasRect(nil), layout.rects...)
	sort.SliceStable(rects, func(i, j int) bool {
		if rects[i].index == m.selectedOutput {
			return false
		}
		if rects[j].index == m.selectedOutput {
			return true
		}
		return rects[i].index < rects[j].index
	})

	for _, rect := range rects {
		output := m.editOutputs[rect.index]
		selected := rect.index == m.selectedOutput
		paintMonitorCard(grid, rect, output, selected, m.canvasCardStyle(output, selected))
	}
	if m.snap != nil {
		for _, mark := range m.snap.Marks {
			for _, rect := range layout.rects {
				if rect.index == mark.OutputIndex {
					paintSnapMark(grid, rect, mark.Edge, m.styles.palette.snapHighlight)
				}
			}
		}
	}
	return renderCanvasCells(grid)
}

// inspectorLayout is the single source of truth shared by renderInspectorPane
// and inspectorFieldAt so their row math cannot drift.
type inspectorLayout struct {
	lines     []string
	fieldRows map[int]int // field index → index into lines
}

func (m Model) buildInspectorLayout(output editableOutput, innerWidth int, compact bool) inspectorLayout {
	lines := []string{m.styles.header.Render("Selected Monitor")}

	if compact {
		info := fmt.Sprintf("%s  %s  %s", output.Name, output.displayModelLabel(), output.DisplayMode())
		lines = append(lines, m.styles.subtle.Render(fitString(info, innerWidth)))
	} else {
		lines = append(lines, "")
		lines = append(lines, m.styles.header.Render("Info"))
		lines = append(lines, m.inspectorDetailLines(output)...)
	}

	lines = append(lines, "", m.styles.header.Render("Preferences"))

	labelWidth := 12
	shortLabels := compact || innerWidth < 34
	if shortLabels {
		labelWidth = 11
	}

	fieldRows := make(map[int]int, len(layoutFields))
	for idx := range layoutFields {
		if idx == advancedFieldStart {
			lines = append(lines, "")
		}
		labelText := layoutFields[idx]
		if shortLabels {
			labelText = layoutFieldShortLabel(idx)
		}
		value := m.styles.value.Render(m.layoutFieldValue(output, idx))
		if m.layoutFocus == layoutFocusInspector && idx == m.inspectorField && m.tab == tabLayout {
			value = m.styles.focused.Render(value)
		}
		label := m.styles.label.Render(fmt.Sprintf("%-*s", labelWidth, labelText))
		fieldRows[idx] = len(lines)
		lines = append(lines, fmt.Sprintf("%s %s", label, value))
	}

	return inspectorLayout{lines: lines, fieldRows: fieldRows}
}

func inspectorScrollOffset(totalLines, selectedLine, height int) int {
	if height <= 0 || selectedLine < height {
		return 0
	}
	offset := selectedLine - height + 1
	if offset < 0 {
		offset = 0
	}
	if offset >= totalLines {
		offset = totalLines - 1
	}
	return offset
}

func (m Model) renderInspectorPane(width int, height int, compact bool) string {
	panel := m.styles.inactivePane
	if m.layoutFocus == layoutFocusInspector && m.tab == tabLayout {
		panel = m.styles.activePane
	}
	innerWidth := max(1, width-panel.GetHorizontalFrameSize())
	innerHeight := max(1, height-panel.GetVerticalFrameSize())

	if len(m.editOutputs) == 0 {
		lines := []string{m.styles.header.Render("Selected Monitor"), "(none)"}
		return panel.Width(innerWidth).Render(fitBlock(strings.Join(lines, "\n"), innerWidth, innerHeight))
	}

	layout := m.buildInspectorLayout(m.editOutputs[m.selectedOutput], innerWidth, compact)
	lines := layout.lines

	if m.layoutFocus == layoutFocusInspector && m.tab == tabLayout {
		if row, ok := layout.fieldRows[m.inspectorField]; ok {
			offset := inspectorScrollOffset(len(lines), row, innerHeight)
			lines = lines[offset:]
		}
	}

	return panel.Width(innerWidth).Render(fitBlock(strings.Join(lines, "\n"), innerWidth, innerHeight))
}

func scrollLinesToFit(lines []string, selectedLine, height int) []string {
	offset := inspectorScrollOffset(len(lines), selectedLine, height)
	return lines[offset:]
}

func (m Model) renderProfilesView(height int) string {
	listWidth, detailWidth := m.sidePaneWidths(35)

	listLines := []string{m.styles.header.Render("Saved Profiles"), ""}
	if len(m.profiles) == 0 {
		listLines = append(listLines, "(none)")
	} else {
		for idx, prof := range m.profiles {
			prefix := "  "
			if idx == m.selectedProfile {
				prefix = "> "
			}
			listLines = append(listLines, fmt.Sprintf("%s%-20s outputs:%d", prefix, fitString(prof.Name, 20), len(prof.Outputs)))
		}
	}

	detailLines := []string{m.styles.header.Render("Profile Details"), ""}
	if len(m.profiles) == 0 {
		detailLines = append(detailLines, "(none)")
	} else {
		selected := m.profiles[m.selectedProfile]
		detailLines = append(detailLines, selected.Name)
		detailLines = append(detailLines, fmt.Sprintf("Updated: %s", selected.UpdatedAt.Local().Format("2006-01-02 15:04")))
		detailLines = append(detailLines, "")
		detailLines = append(detailLines, "Outputs:")
		for _, output := range selected.Outputs {
			state := "off"
			if output.Enabled {
				state = "on"
			}
			label := strings.TrimSpace(output.Make + " " + output.Model)
			if label == "" {
				label = output.Name
			}
			line := fmt.Sprintf("  %s %s %s pos:%dx%d scale:%.2f", label, state, output.NormalizedMode(), output.X, output.Y, output.Scale)
			if output.MirrorOf != "" {
				mirrorLabel := outputDisplayLabel(output.MirrorOf, selected.Outputs)
				line += fmt.Sprintf(" mirrors:%s", mirrorLabel)
			}
			detailLines = append(detailLines, line)
		}

		execDisplay := selected.Exec
		if execDisplay == "" {
			execDisplay = "<not set>"
		}

		detailLines = append(detailLines, "")
		detailLines = append(detailLines, fmt.Sprintf("Exec: %s", execDisplay))

		preview := profile.WorkspacePreview(selected.Workspaces, selected.Outputs, m.monitors)
		if len(preview) > 0 {
			detailLines = append(detailLines, "")
			detailLines = append(detailLines, "Workspace plan:")
			for _, line := range workspacePreviewLines(preview, selected.Workspaces.MonitorOrder, selected.Outputs) {
				detailLines = append(detailLines, "  "+line)
			}
		}
	}

	leftStyle := m.styles.activePane
	rightStyle := m.styles.inactivePane

	if m.terminalWidth() < 96 {
		// Compact: stack vertically, list gets enough for profiles + header, details gets the rest
		width := m.terminalWidth() - m.styles.app.GetHorizontalFrameSize()
		innerW := max(1, width-leftStyle.GetHorizontalFrameSize())
		listH := clampInt(len(m.profiles)+3, 4, height/3)
		detailH := max(3, height-listH)
		left := leftStyle.Width(innerW).Render(fitBlock(strings.Join(listLines, "\n"), innerW, max(1, listH-leftStyle.GetVerticalFrameSize())))
		right := rightStyle.Width(innerW).Render(fitBlock(strings.Join(detailLines, "\n"), innerW, max(1, detailH-rightStyle.GetVerticalFrameSize())))
		return lipgloss.JoinVertical(lipgloss.Left, left, right)
	}

	left := leftStyle.Width(max(1, listWidth-leftStyle.GetHorizontalFrameSize())).
		Render(fitBlock(strings.Join(listLines, "\n"), max(1, listWidth-leftStyle.GetHorizontalFrameSize()), max(1, height-leftStyle.GetVerticalFrameSize())))
	right := rightStyle.Width(max(1, detailWidth-rightStyle.GetHorizontalFrameSize())).
		Render(fitBlock(strings.Join(detailLines, "\n"), max(1, detailWidth-rightStyle.GetHorizontalFrameSize()), max(1, height-rightStyle.GetVerticalFrameSize())))
	return lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right)
}

func (m Model) renderWorkspaceView(height int) string {
	leftWidth, rightWidth := m.sidePaneWidths(35)

	settings := []string{
		m.styles.header.Render("Workspace Planner"),
		"",
	}

	for idx, field := range workspaceFields {
		value := m.workspaceFieldValue(idx)
		prefix := "  "
		if idx == m.workspaceEdit.SelectedField {
			prefix = "> "
			value = m.styles.focused.Render(value)
		}
		settings = append(settings, fmt.Sprintf("%s%-14s %s", prefix, field, value))
	}

	settings = append(settings, "")
	settings = append(settings, "Monitor order  ←/→ reorders")
	if len(m.workspaceEdit.MonitorOrder) == 0 {
		settings = append(settings, "  (none)")
	} else {
		for idx, key := range m.workspaceEdit.MonitorOrder {
			label := m.outputLabelForKey(key)
			prefix := "  "
			orderField := len(workspaceFields) + idx
			if orderField == m.workspaceEdit.SelectedField {
				prefix = "> "
				label = m.styles.focused.Render(label)
			}
			settings = append(settings, fmt.Sprintf("%s%d. %s", prefix, idx+1, label))
		}
	}

	if m.workspaceEdit.Strategy == profile.WorkspaceStrategyManual && len(m.workspaceEdit.Rules) > 0 {
		settings = append(settings, "")
		settings = append(settings, "Manual rules imported from current state or saved profile.")
		settings = append(settings, "Switch strategy to sequential or interleave to regenerate them.")
	}

	previewSettings := m.workspaceEdit.settings()
	previewDisabled := !previewSettings.Enabled
	if previewDisabled {
		previewSettings.Enabled = true
	}
	preview := profile.WorkspacePreview(previewSettings, m.currentProfileOutputs(), m.monitors)
	previewLines := []string{
		m.styles.header.Render("Workspace Preview"),
		"",
	}
	if previewDisabled {
		previewLines = append(previewLines, "(workspace rules disabled; preview only)")
		previewLines = append(previewLines, "")
	}
	if len(preview) == 0 {
		previewLines = append(previewLines, "(no workspace rules configured)")
	} else {
		for _, line := range workspacePreviewLines(preview, m.workspaceEdit.MonitorOrder, m.currentProfileOutputs()) {
			previewLines = append(previewLines, line)
		}
	}

	leftStyle := m.styles.activePane
	rightStyle := m.styles.inactivePane

	if m.terminalWidth() < 96 {
		// Compact: stack vertically, settings get enough room, preview gets the rest
		width := m.terminalWidth() - m.styles.app.GetHorizontalFrameSize()
		innerW := max(1, width-leftStyle.GetHorizontalFrameSize())
		settingsH := clampInt(len(settings)+2, 6, (height*2)/3)
		previewH := max(3, height-settingsH)
		left := leftStyle.Width(innerW).Render(fitBlock(strings.Join(settings, "\n"), innerW, max(1, settingsH-leftStyle.GetVerticalFrameSize())))
		right := rightStyle.Width(innerW).Render(fitBlock(strings.Join(previewLines, "\n"), innerW, max(1, previewH-rightStyle.GetVerticalFrameSize())))
		return lipgloss.JoinVertical(lipgloss.Left, left, right)
	}

	left := leftStyle.Width(max(1, leftWidth-leftStyle.GetHorizontalFrameSize())).
		Render(fitBlock(strings.Join(settings, "\n"), max(1, leftWidth-leftStyle.GetHorizontalFrameSize()), max(1, height-leftStyle.GetVerticalFrameSize())))
	right := rightStyle.Width(max(1, rightWidth-rightStyle.GetHorizontalFrameSize())).
		Render(fitBlock(strings.Join(previewLines, "\n"), max(1, rightWidth-rightStyle.GetHorizontalFrameSize()), max(1, height-rightStyle.GetVerticalFrameSize())))
	return lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right)
}

func (m Model) renderSavePrompt() string {
	if m.saveDialog == nil {
		return m.renderModalFrame("Save Profile", nil)
	}
	inputBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(m.styles.palette.paneActiveBorder)).
		Padding(0, 1).
		Render(m.saveDialog.Input.View())
	body := []string{
		m.styles.label.Render("Name"),
		inputBox,
		"",
		m.saveDialog.List.View(),
		"",
		m.styles.label.Render("Action"),
		m.renderSaveActionButtons(),
		"",
	}
	if status := m.renderErrorStatus(); status != "" {
		body = append(body, status, "")
	}
	body = append(body, m.styles.help.MaxWidth(max(20, m.modalMaxWidth()-6)).Render("Type to filter names. Up/Down selects an existing profile. Tab switches action. Enter confirms. Esc cancels."))
	return m.renderModalFrame("Save Profile", body)
}

func (m Model) renderSaveConfirm() string {
	consequence := "The existing profile will be replaced with the current draft."
	if m.saveDialog != nil && m.saveDialog.Action == saveActionApply {
		consequence = "The existing profile will be replaced and then applied to the live layout."
	}

	body := []string{
		m.styles.warning.Render(fmt.Sprintf("Overwrite profile %q?", m.saveOverwrite)),
		m.styles.subtle.Render(consequence),
		"",
		m.styles.help.Render("Enter or y overwrites. Esc or n cancels."),
	}
	return m.renderModalFrame("Confirm Overwrite", body)
}

func (m Model) renderConfirm() string {
	if m.pending == nil {
		return m.renderModalFrame("Confirm Apply", nil)
	}

	remaining := int(time.Until(m.pending.deadline).Seconds())
	if remaining < 0 {
		remaining = 0
	}

	body := []string{
		m.styles.warning.Render(fmt.Sprintf("%s is live now.", targetLabel(m.pending.profile.Name))),
		m.styles.subtle.Render(fmt.Sprintf("Keep it within %ds or the previous state will be restored.", remaining)),
		"",
		m.renderStatus(),
		m.styles.help.MaxWidth(max(20, m.modalMaxWidth()-6)).Render("Enter or y keeps the change. Esc or n reverts it."),
	}
	return m.renderModalFrame("Confirm Apply", body)
}

func (m Model) renderToast() string {
	if m.toast == nil || strings.TrimSpace(m.toast.message) == "" {
		return ""
	}
	style := m.styles.toast
	if m.toast.err {
		style = m.styles.toastError
	}
	return style.MaxWidth(max(24, m.terminalWidth()-8)).Render(m.toast.message)
}

func (m Model) renderStatus() string {
	if m.status == "" {
		return ""
	}
	if m.statusErr {
		return m.styles.statusError.MaxWidth(max(20, m.terminalWidth()-2)).Render(m.status)
	}
	return m.styles.statusOK.MaxWidth(max(20, m.terminalWidth()-2)).Render(m.status)
}

func (m Model) renderErrorStatus() string {
	if m.status == "" || !m.statusErr {
		return ""
	}
	return m.styles.statusError.MaxWidth(max(20, m.modalMaxWidth()-6)).Render(m.status)
}

func (m Model) mainBodyHeight(title string, tabs string, status string, help string) int {
	reserved := lipgloss.Height(title) + lipgloss.Height(tabs) + lipgloss.Height(help)
	return max(3, m.terminalHeight()-reserved)
}

func (m Model) useCompactLayout(bodyHeight int) bool {
	canvasWidth, inspectorWidth := m.layoutPaneWidths()
	return bodyHeight < 14 || canvasWidth < 96 || inspectorWidth < 50
}

func (m Model) compactLayoutHeights(total int) (int, int) {
	if total <= 6 {
		canvas := max(2, (total+1)/2)
		return canvas, max(1, total-canvas)
	}

	inspector := max(4, (total*7)/12)
	canvas := total - inspector
	if canvas < 4 {
		canvas = 4
		inspector = total - canvas
	}
	if inspector < 4 {
		inspector = 4
		canvas = total - inspector
	}
	if canvas < 3 {
		canvas = max(2, total/2)
		inspector = total - canvas
	}
	return max(2, canvas), max(1, inspector)
}

func (m Model) inspectorDetailLines(output editableOutput) []string {
	lines := []string{
		fmt.Sprintf("%s %s", m.styles.label.Render("Connector "), m.styles.value.Render(output.Name)),
		fmt.Sprintf("%s %s", m.styles.label.Render("Type      "), m.styles.value.Render(outputTypeLabel(output))),
		fmt.Sprintf("%s %s", m.styles.label.Render("Model     "), m.styles.value.Render(output.displayModelLabel())),
		fmt.Sprintf("%s %s", m.styles.label.Render("Serial    "), m.styles.value.Render(blankFallback(strings.TrimSpace(output.Serial), "(none)"))),
		fmt.Sprintf("%s %s", m.styles.label.Render("Layout px "), m.styles.value.Render(output.layoutSizeLabel())),
		fmt.Sprintf("%s %s", m.styles.label.Render("Workspace "), m.styles.value.Render(blankFallback(output.ActiveWorkspace, "(none)"))),
		fmt.Sprintf("%s %s", m.styles.label.Render("DPMS      "), m.styles.value.Render(boolText(output.DPMSStatus))),
	}
	if output.PhysicalWidth > 0 && output.PhysicalHeight > 0 {
		lines = append(lines, fmt.Sprintf("%s %s", m.styles.label.Render("Panel mm  "), m.styles.value.Render(fmt.Sprintf("%d x %d mm", output.PhysicalWidth, output.PhysicalHeight))))
	}
	return lines
}

func outputTypeLabel(output editableOutput) string {
	if output.IsInternal {
		return "Internal display"
	}
	return "External display"
}

func fitBlock(text string, width int, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	wrapper := lipgloss.NewStyle().Width(width).MaxWidth(width)
	raw := strings.Split(text, "\n")
	lines := make([]string, 0, height)
	for _, line := range raw {
		rendered := wrapper.Render(line)
		lines = append(lines, strings.Split(rendered, "\n")...)
		if len(lines) >= height {
			break
		}
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func (m *Model) loadLiveState() {
	prevOutputs := m.editOutputs
	selectedKey := ""
	if m.selectedOutput >= 0 && m.selectedOutput < len(prevOutputs) {
		selectedKey = prevOutputs[m.selectedOutput].Key
	}
	m.editOutputs = make([]editableOutput, 0, len(m.monitors))
	matchCounts := hypr.MonitorMatchCounts(m.monitors)
	nameToKey := make(map[string]string, len(m.monitors))
	for _, monitor := range m.monitors {
		nameToKey[monitor.Name] = hypr.MonitorOutputKey(monitor, matchCounts)
	}
	for _, monitor := range m.monitors {
		output := editableOutputFromMonitor(monitor, matchCounts)
		if output.MirrorOf != "" {
			if key, ok := nameToKey[output.MirrorOf]; ok {
				output.MirrorOf = key
			}
		}
		m.editOutputs = append(m.editOutputs, output)
	}
	m.recoverMirroredIdentity()

	settings := profile.WorkspaceSettingsFromHypr(m.monitors, m.workspaceRules)
	m.workspaceEdit = workspaceEditorFromSettings(settings, m.editOutputs)
	if matched, ok := profile.ExactStateMatch(m.profiles, m.monitors, m.workspaceRules); ok {
		m.draftProfileName = matched.Name
		m.draftExec = matched.Exec
	} else {
		m.draftProfileName = ""
		m.draftExec = ""
	}

	// Preserve fields that hyprctl cannot accurately report, unless the
	// user explicitly requested a reset.
	if !m.resetRequested {
		for i := range m.editOutputs {
			for _, prev := range prevOutputs {
				if prev.Key == m.editOutputs[i].Key {
					m.editOutputs[i].VRR = prev.VRR
					m.editOutputs[i].Bitdepth = prev.Bitdepth
					m.editOutputs[i].CM = prev.CM
					m.editOutputs[i].MinLuminance = prev.MinLuminance
					m.editOutputs[i].MaxLuminance = prev.MaxLuminance
					m.editOutputs[i].SupportsWideColor = prev.SupportsWideColor
					m.editOutputs[i].SupportsHDR = prev.SupportsHDR
					m.editOutputs[i].MaxAvgLuminance = prev.MaxAvgLuminance
					m.editOutputs[i].SDREOTF = prev.SDREOTF
					m.editOutputs[i].ICC = prev.ICC
					break
				}
			}
		}
	}
	if len(prevOutputs) == 0 || m.resetRequested {
		if best, _, ok := profile.BestMatch(m.profiles, m.monitors); ok {
			for i := range m.editOutputs {
				if saved, ok := best.OutputByKey(m.editOutputs[i].Key); ok {
					m.editOutputs[i].VRR = saved.VRR
					m.editOutputs[i].Bitdepth = saved.Bitdepth
					m.editOutputs[i].CM = saved.CM
					m.editOutputs[i].MinLuminance = saved.MinLuminance
					m.editOutputs[i].MaxLuminance = saved.MaxLuminance
					m.editOutputs[i].SupportsWideColor = saved.SupportsWideColor
					m.editOutputs[i].SupportsHDR = saved.SupportsHDR
					m.editOutputs[i].MaxAvgLuminance = saved.MaxAvgLuminance
					m.editOutputs[i].SDREOTF = saved.SDREOTF
					m.editOutputs[i].ICC = saved.ICC
				}
			}
		}
	}
	m.resetRequested = false
	if idx := focusedOutputIndex(m.editOutputs); idx >= 0 {
		m.selectedOutput = idx
	} else if selectedKey != "" {
		m.selectedOutput = outputIndexByKey(m.editOutputs, selectedKey)
	}
	m.selectedOutput = clampIndex(m.selectedOutput, len(m.editOutputs))
	m.inspectorField = clampIndex(m.inspectorField, len(layoutFields))
	m.picker = nil
	m.input = nil
	m.drag = nil
	m.markClean()

	m.revalidate()
}

func (m *Model) loadProfile(p profile.Profile) {
	outputs := make([]editableOutput, 0, len(p.Outputs))
	for _, saved := range p.Outputs {
		live, ok := m.findLiveMonitor(saved)
		outputs = append(outputs, editableOutputFromProfile(saved, live, ok))
	}
	m.editOutputs = outputs
	m.workspaceEdit = workspaceEditorFromSettings(p.Workspaces, m.editOutputs)
	m.selectedOutput = clampIndex(0, len(m.editOutputs))
	m.inspectorField = 0
	m.picker = nil
	m.input = nil
	m.drag = nil
	m.dirty = true
	m.draftSaved = true
	m.draftProfileName = p.Name
	m.draftExec = p.Exec
	m.setStatusOK(fmt.Sprintf("Loaded profile %q into editor", p.Name))

	m.revalidate()
}

// recoverMirroredIdentity restores Make/Model/Serial/Key for monitors whose
// identity was degraded by Hyprland while mirroring. It looks up the real
// identity from saved profiles by matching connector names.
func (m *Model) recoverMirroredIdentity() {
	for i, output := range m.editOutputs {
		if output.MirrorOf == "" || strings.TrimSpace(output.Make+" "+output.Model) != "" {
			continue
		}
		for _, prof := range m.profiles {
			for _, saved := range prof.Outputs {
				if saved.Name == output.Name && strings.TrimSpace(saved.Make+" "+saved.Model) != "" {
					m.editOutputs[i].Make = saved.Make
					m.editOutputs[i].Model = saved.Model
					m.editOutputs[i].Serial = saved.Serial
					m.editOutputs[i].Key = saved.Key
					break
				}
			}
			if strings.TrimSpace(m.editOutputs[i].Make+" "+m.editOutputs[i].Model) != "" {
				break
			}
		}
	}
}

func (m *Model) syncSelections() {
	m.selectedOutput = clampIndex(m.selectedOutput, len(m.editOutputs))
	m.selectedProfile = clampIndex(m.selectedProfile, len(m.profiles))
	m.inspectorField = clampIndex(m.inspectorField, len(layoutFields))
	m.workspaceEdit.SelectedField = clampIndex(m.workspaceEdit.SelectedField, len(workspaceFields))
	m.workspaceEdit.SelectedOrder = clampIndex(m.workspaceEdit.SelectedOrder, len(m.workspaceEdit.MonitorOrder))
}

func (m Model) profileExists(name string) bool {
	_, ok := m.profileByName(name)
	return ok
}

func (m Model) profileByName(name string) (profile.Profile, bool) {
	name = strings.TrimSpace(strings.ToLower(name))
	for _, prof := range m.profiles {
		if strings.TrimSpace(strings.ToLower(prof.Name)) == name {
			return prof, true
		}
	}
	return profile.Profile{}, false
}

func (m Model) hasMirroredOutputs() bool {
	for _, output := range m.editOutputs {
		if output.Enabled && output.MirrorOf != "" {
			return true
		}
	}
	return false
}

func (m Model) mirrorSummaryLabels() []string {
	labels := make([]string, 0)
	for _, output := range m.editOutputs {
		if !output.Enabled || output.MirrorOf == "" {
			continue
		}
		labels = append(labels, fmt.Sprintf("%s -> %s", output.Name, m.outputNameForKey(output.MirrorOf)))
	}
	return labels
}

func (m Model) outputNameForKey(key string) string {
	for _, output := range m.editOutputs {
		if output.Key == key {
			return output.Name
		}
	}
	return key
}

func (m *Model) moveSelectedOutput(dx, dy int) {
	if len(m.editOutputs) == 0 {
		return
	}
	m.editOutputs[m.selectedOutput].X += dx
	m.editOutputs[m.selectedOutput].Y += dy
	m.layoutChanged()
}

func (m *Model) toggleSelectedOutput() {
	if len(m.editOutputs) == 0 {
		return
	}
	m.editOutputs[m.selectedOutput].Enabled = !m.editOutputs[m.selectedOutput].Enabled
	m.layoutChanged()
}

func (m Model) analyzeSelectedSnap(threshold int) snapAnalysis {
	analysis := snapAnalysis{
		x: snapAxisCandidate{dist: threshold + 1},
		y: snapAxisCandidate{dist: threshold + 1},
	}
	if len(m.editOutputs) == 0 || m.selectedOutput < 0 || m.selectedOutput >= len(m.editOutputs) {
		return analysis
	}

	selected := m.editOutputs[m.selectedOutput]
	if !selected.Enabled {
		return analysis
	}

	width, height := selected.logicalSize()
	analysis.x.pos = selected.X
	analysis.y.pos = selected.Y

	considerX := func(pos int, marks ...snapMark) {
		dist := abs(selected.X - pos)
		if dist < analysis.x.dist {
			analysis.x = snapAxisCandidate{pos: pos, dist: dist, marks: append([]snapMark(nil), marks...)}
		}
	}
	considerY := func(pos int, marks ...snapMark) {
		dist := abs(selected.Y - pos)
		if dist < analysis.y.dist {
			analysis.y = snapAxisCandidate{pos: pos, dist: dist, marks: append([]snapMark(nil), marks...)}
		}
	}

	for idx, other := range m.editOutputs {
		if idx == m.selectedOutput || !other.Enabled {
			continue
		}

		otherW, otherH := other.logicalSize()
		if spansOverlap(selected.Y, selected.Y+height, other.Y, other.Y+otherH) {
			considerX(other.X-width,
				snapMark{OutputIndex: m.selectedOutput, Edge: snapEdgeRight},
				snapMark{OutputIndex: idx, Edge: snapEdgeLeft},
			)
			considerX(other.X+otherW,
				snapMark{OutputIndex: m.selectedOutput, Edge: snapEdgeLeft},
				snapMark{OutputIndex: idx, Edge: snapEdgeRight},
			)
		}
		considerX(other.X,
			snapMark{OutputIndex: m.selectedOutput, Edge: snapEdgeLeft},
			snapMark{OutputIndex: idx, Edge: snapEdgeLeft},
		)
		considerX(other.X+otherW-width,
			snapMark{OutputIndex: m.selectedOutput, Edge: snapEdgeRight},
			snapMark{OutputIndex: idx, Edge: snapEdgeRight},
		)
		if spansOverlap(selected.X, selected.X+width, other.X, other.X+otherW) {
			considerY(other.Y-height,
				snapMark{OutputIndex: m.selectedOutput, Edge: snapEdgeBottom},
				snapMark{OutputIndex: idx, Edge: snapEdgeTop},
			)
			considerY(other.Y+otherH,
				snapMark{OutputIndex: m.selectedOutput, Edge: snapEdgeTop},
				snapMark{OutputIndex: idx, Edge: snapEdgeBottom},
			)
		}
		considerY(other.Y,
			snapMark{OutputIndex: m.selectedOutput, Edge: snapEdgeTop},
			snapMark{OutputIndex: idx, Edge: snapEdgeTop},
		)
		considerY(other.Y+otherH-height,
			snapMark{OutputIndex: m.selectedOutput, Edge: snapEdgeBottom},
			snapMark{OutputIndex: idx, Edge: snapEdgeBottom},
		)
	}

	considerX(0)
	considerY(0)
	return analysis
}

func (m Model) previewSelectedSnap(threshold int) *snapHintState {
	analysis := m.analyzeSelectedSnap(threshold)
	var marks []snapMark
	if analysis.x.dist <= threshold {
		marks = append(marks, analysis.x.marks...)
	}
	if analysis.y.dist <= threshold {
		marks = append(marks, analysis.y.marks...)
	}
	if len(marks) == 0 {
		return nil
	}
	return &snapHintState{Marks: marks}
}

func (m *Model) applySelectedSnap(threshold int) *snapHintState {
	analysis := m.analyzeSelectedSnap(threshold)
	if len(m.editOutputs) == 0 || m.selectedOutput < 0 || m.selectedOutput >= len(m.editOutputs) {
		return nil
	}

	selected := &m.editOutputs[m.selectedOutput]
	var marks []snapMark
	if analysis.x.dist <= threshold {
		selected.X = analysis.x.pos
		marks = append(marks, analysis.x.marks...)
	}
	if analysis.y.dist <= threshold {
		selected.Y = analysis.y.pos
		marks = append(marks, analysis.y.marks...)
	}
	if len(marks) == 0 {
		return nil
	}
	return &snapHintState{Marks: marks}
}

func spansOverlap(a1, a2, b1, b2 int) bool {
	return a1 < b2 && a2 > b1
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func (m *Model) adjustInspectorField(delta int) {
	if len(m.editOutputs) == 0 {
		return
	}

	output := &m.editOutputs[m.selectedOutput]
	switch m.inspectorField {
	case 0:
		output.Enabled = !output.Enabled
	case 1:
		if len(output.Modes) == 0 {
			return
		}
		output.ModeIndex = wrapIndex(output.ModeIndex+delta, len(output.Modes))
		output.applyMode(output.Modes[output.ModeIndex])
	case 2:
		output.Scale = clampFloat(output.Scale+float64(delta)*0.05, 0.25, 4.0)
	case 3:
		depths := []int{8, 10, 16}
		current := 0
		for i, d := range depths {
			if d == output.Bitdepth {
				current = i
				break
			}
		}
		output.Bitdepth = depths[wrapIndex(current+delta, len(depths))]
	case 4:
		presets := []string{"srgb", "auto", "wide", "hdr", "hdredid", "dcip3", "dp3", "adobe", "edid"}
		current := 0
		for i, p := range presets {
			if p == output.CM {
				current = i
				break
			}
		}
		output.CM = presets[wrapIndex(current+delta, len(presets))]
	case 5:
		output.VRR = wrapValue(output.VRR+delta, 0, 2)
	case 6:
		output.Transform = wrapValue(output.Transform+delta, 0, 7)
	case 7:
		output.X += delta * 10
	case 8:
		output.Y += delta * 10
	case 9:
		targets := []string{""}
		for i, other := range m.editOutputs {
			if i != m.selectedOutput {
				targets = append(targets, other.Key)
			}
		}
		current := 0
		for i, t := range targets {
			if t == output.MirrorOf {
				current = i
				break
			}
		}
		output.MirrorOf = targets[wrapIndex(current+delta, len(targets))]
	case 10:
		output.SDRBrightness = clampFloat(output.SDRBrightness+float64(delta)*0.05, 0, 3.0)
	case 11:
		output.SDRSaturation = clampFloat(output.SDRSaturation+float64(delta)*0.05, 0, 3.0)
	case 12:
		output.SDRMinLuminance = clampFloat(output.SDRMinLuminance+float64(delta)*0.005, 0, 1.0)
	case 13:
		output.SDRMaxLuminance = clampInt(output.SDRMaxLuminance+delta*10, 0, 1000)
	case 14:
		eotfs := []string{"default", "gamma22", "srgb"}
		cur := 0
		for i, e := range eotfs {
			if e == output.SDREOTF {
				cur = i
				break
			}
		}
		output.SDREOTF = eotfs[wrapIndex(cur+delta, len(eotfs))]
	case 15:
		output.MinLuminance = clampFloat(output.MinLuminance+float64(delta)*0.001, 0, 1000.0)
	case 16:
		output.MaxLuminance = clampInt(output.MaxLuminance+delta*10, 0, 2000)
	case 17:
		output.MaxAvgLuminance = clampInt(output.MaxAvgLuminance+delta*10, 0, 2000)
	case 18:
		vals := []int{-1, 0, 1}
		cur := 1
		for i, v := range vals {
			if v == output.SupportsWideColor {
				cur = i
				break
			}
		}
		output.SupportsWideColor = vals[wrapIndex(cur+delta, len(vals))]
	case 19:
		vals := []int{-1, 0, 1}
		cur := 1
		for i, v := range vals {
			if v == output.SupportsHDR {
				cur = i
				break
			}
		}
		output.SupportsHDR = vals[wrapIndex(cur+delta, len(vals))]
	case 20:
		// ICC uses text input via activateInspectorField
	}
	m.layoutChanged()
}

func (m *Model) adjustWorkspaceField(delta int) {
	switch m.workspaceEdit.SelectedField {
	case 0:
		m.workspaceEdit.Enabled = !m.workspaceEdit.Enabled
	case 1:
		strategies := []profile.WorkspaceStrategy{
			profile.WorkspaceStrategyManual,
			profile.WorkspaceStrategySequential,
			profile.WorkspaceStrategyInterleave,
		}
		current := 0
		for idx, strategy := range strategies {
			if strategy == m.workspaceEdit.Strategy {
				current = idx
				break
			}
		}
		if m.workspaceEdit.Strategy == profile.WorkspaceStrategySequential && m.workspaceEdit.GroupSize > 0 {
			m.workspaceEdit.LastSequentialGroupSize = m.workspaceEdit.GroupSize
		}
		next := strategies[wrapIndex(current+delta, len(strategies))]
		if next == profile.WorkspaceStrategySequential && m.workspaceEdit.Strategy != profile.WorkspaceStrategySequential {
			if m.workspaceEdit.LastSequentialGroupSize <= 0 {
				m.workspaceEdit.LastSequentialGroupSize = defaultWorkspaceGroupSize
			}
			m.workspaceEdit.GroupSize = m.workspaceEdit.LastSequentialGroupSize
		}
		m.workspaceEdit.Strategy = next
	case 2:
		m.workspaceEdit.MaxWorkspaces = clampInt(m.workspaceEdit.MaxWorkspaces+delta, 1, 30)
	case 3:
		m.workspaceEdit.GroupSize = clampInt(m.workspaceEdit.GroupSize+delta, 1, 10)
		m.workspaceEdit.LastSequentialGroupSize = m.workspaceEdit.GroupSize
	}
}

func (m *Model) moveWorkspaceOrder(delta int) {
	idx := m.workspaceEdit.SelectedOrder
	next := idx + delta
	if idx < 0 || idx >= len(m.workspaceEdit.MonitorOrder) || next < 0 || next >= len(m.workspaceEdit.MonitorOrder) {
		return
	}
	m.workspaceEdit.MonitorOrder[idx], m.workspaceEdit.MonitorOrder[next] = m.workspaceEdit.MonitorOrder[next], m.workspaceEdit.MonitorOrder[idx]
	m.workspaceEdit.SelectedOrder = next
	m.workspaceEdit.SelectedField = len(workspaceFields) + next
}

func (m Model) currentProfile(name string) profile.Profile {
	p := profile.New(name, m.currentProfileOutputs())
	p.Workspaces = m.workspaceEdit.settings()
	p.Exec = m.currentProfileExec(name)
	p.Normalize()
	return p
}

func (m Model) currentProfileExec(name string) string {
	if exec := strings.TrimSpace(m.draftExec); exec != "" {
		return exec
	}
	if existing, ok := m.profileByName(name); ok {
		return existing.Exec
	}
	return ""
}

func (m Model) currentProfileOutputs() []profile.OutputConfig {
	outputs := make([]profile.OutputConfig, 0, len(m.editOutputs))
	for _, output := range m.editOutputs {
		outputs = append(outputs, output.profileOutput())
	}
	return outputs
}

func (m *Model) revalidate() {
	m.layoutErr = apply.ValidateLayout(m.currentProfileOutputs())
}

func (m *Model) layoutChanged() {
	m.markDirty()
	m.revalidate()
}

func (m *Model) nudgeSelectedOutput(dx, dy int, snapThreshold int) tea.Cmd {
	m.moveSelectedOutput(dx, dy)
	return m.showSnapHint(m.previewSelectedSnap(snapThreshold))
}

func liveConfigSignature(monitors []hypr.Monitor, lidState lid.State) string {
	return profile.MonitorStateHash(monitors) + "|lid=" + string(lidState)
}

func (m Model) liveConfigSignature() string {
	return liveConfigSignature(m.monitors, m.lidState)
}

func (m Model) refreshCmd(background bool) tea.Cmd {
	client := m.client
	store := m.store
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()

		monitors, err := client.Monitors(ctx)
		if err != nil {
			return refreshMsg{background: background, err: err}
		}
		profiles, err := store.List()
		if err != nil {
			return refreshMsg{background: background, err: err}
		}
		workspaceRules, err := client.WorkspaceRules(ctx)
		if err != nil {
			return refreshMsg{background: background, err: err}
		}
		workspaces, err := client.Workspaces(ctx)
		if err != nil {
			return refreshMsg{background: background, err: err}
		}
		lidState, err := lid.ReadState(ctx)
		if err != nil {
			lidState = lid.Unknown
		}

		return refreshMsg{
			monitors:       monitors,
			profiles:       profiles,
			workspaceRules: workspaceRules,
			workspaces:     workspaces,
			lidState:       lidState,
			background:     background,
		}
	}
}

func (m Model) saveCmd(p profile.Profile) tea.Cmd {
	store := m.store
	return func() tea.Msg {
		if err := store.Save(p); err != nil {
			return saveMsg{err: err}
		}
		return saveMsg{name: p.Name}
	}
}

func (m Model) saveProfileCmd(p profile.Profile) tea.Cmd {
	store := m.store
	return func() tea.Msg {
		if err := store.Save(p); err != nil {
			return saveMsg{name: p.Name, err: err, profileTab: true}
		}
		return saveMsg{name: p.Name, profileTab: true}
	}
}

func (m Model) deleteCmd(name string) tea.Cmd {
	store := m.store
	return func() tea.Msg {
		if err := store.Delete(name); err != nil {
			return deleteMsg{name: name, err: err}
		}
		return deleteMsg{name: name}
	}
}

func (m Model) applyCmd(p profile.Profile) tea.Cmd {
	client := m.client
	engine := m.engine
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		monitors, err := client.Monitors(ctx)
		if err != nil {
			return applyMsg{profile: p, err: err}
		}
		applyProfile := p
		if state, err := lid.ReadState(ctx); err == nil && state == lid.Closed {
			applyProfile, _ = profile.ApplyClosedLidPolicy(p, monitors)
		}
		snapshot, err := engine.Apply(ctx, applyProfile, monitors, apply.ApplyModeInteractive)
		if err != nil {
			return applyMsg{profile: p, err: err}
		}
		return applyMsg{profile: applyProfile, snapshot: snapshot}
	}
}

func (m Model) postApply(p profile.Profile) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return m.engine.PostApply(ctx, p)
}

func (m Model) revertCmd(snapshot apply.RevertState, reason string) tea.Cmd {
	engine := m.engine
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := engine.Revert(ctx, snapshot)
		return revertMsg{err: err, reason: reason}
	}
}

func (m Model) layoutFieldValue(output editableOutput, field int) string {
	switch field {
	case 0:
		return boolText(output.Enabled)
	case 1:
		if len(output.Modes) == 0 {
			return output.DisplayMode()
		}
		return fmt.Sprintf("%s (%d/%d)", output.DisplayMode(), output.ModeIndex+1, len(output.Modes))
	case 2:
		return fmt.Sprintf("%.2f", output.Scale)
	case 3:
		return fmt.Sprintf("%d", output.Bitdepth)
	case 4:
		if output.CM == "" {
			return "srgb"
		}
		return output.CM
	case 5:
		return vrrLabel(output.VRR)
	case 6:
		return transformLabel(output.Transform)
	case 7:
		return fmt.Sprintf("%d", output.X)
	case 8:
		return fmt.Sprintf("%d", output.Y)
	case 9:
		if output.MirrorOf == "" {
			return "None"
		}
		for _, other := range m.editOutputs {
			if other.Key == output.MirrorOf {
				return other.displayModelLabel()
			}
		}
		return output.MirrorOf
	case 10:
		return fmt.Sprintf("%.2f", output.SDRBrightness)
	case 11:
		return fmt.Sprintf("%.2f", output.SDRSaturation)
	case 12:
		return fmt.Sprintf("%.3f", output.SDRMinLuminance)
	case 13:
		return fmt.Sprintf("%d", output.SDRMaxLuminance)
	case 14:
		if output.SDREOTF == "" {
			return "default"
		}
		return output.SDREOTF
	case 15:
		return fmt.Sprintf("%.3f", output.MinLuminance)
	case 16:
		return fmt.Sprintf("%d", output.MaxLuminance)
	case 17:
		return fmt.Sprintf("%d", output.MaxAvgLuminance)
	case 18:
		return triStateLabel(output.SupportsWideColor)
	case 19:
		return triStateLabel(output.SupportsHDR)
	case 20:
		if output.ICC == "" {
			return "None"
		}
		return output.ICC
	default:
		return ""
	}
}

func (m Model) workspaceFieldValue(field int) string {
	switch field {
	case 0:
		return boolText(m.workspaceEdit.Enabled)
	case 1:
		return string(blankStrategy(m.workspaceEdit.Strategy))
	case 2:
		return fmt.Sprintf("%d", m.workspaceEdit.MaxWorkspaces)
	case 3:
		return fmt.Sprintf("%d", m.workspaceEdit.GroupSize)
	default:
		return ""
	}
}

func workspacePreviewLines(preview map[string][]string, order []string, outputs []profile.OutputConfig) []string {
	lines := make([]string, 0, len(preview))
	seen := make(map[string]bool, len(preview))

	for _, key := range order {
		displayLabel := outputDisplayLabel(key, outputs)
		connectorName := outputConnector(key, outputs)
		if workspaces, ok := preview[connectorName]; ok {
			lines = append(lines, fmt.Sprintf("%s: %s", displayLabel, strings.Join(workspaces, ", ")))
			seen[connectorName] = true
		}
	}

	for connectorName, workspaces := range preview {
		if seen[connectorName] {
			continue
		}
		displayLabel := connectorName
		for _, o := range outputs {
			if o.Name == connectorName {
				if label := strings.TrimSpace(o.Make + " " + o.Model); label != "" {
					displayLabel = label
				}
				break
			}
		}
		lines = append(lines, fmt.Sprintf("%s: %s", displayLabel, strings.Join(workspaces, ", ")))
	}
	return lines
}

func outputDisplayLabel(key string, outputs []profile.OutputConfig) string {
	for _, o := range outputs {
		if o.Key == key {
			if label := strings.TrimSpace(o.Make + " " + o.Model); label != "" {
				return label
			}
			return o.Name
		}
	}
	return key
}

func outputConnector(key string, outputs []profile.OutputConfig) string {
	for _, o := range outputs {
		if o.Key == key {
			return o.Name
		}
	}
	return key
}

func (m Model) outputLabelForKey(key string) string {
	return outputDisplayLabel(key, m.currentProfileOutputs())
}

func (m Model) findLiveMonitor(output profile.OutputConfig) (hypr.Monitor, bool) {
	return profile.NewMonitorResolver(m.monitors).ResolveOutput(output)
}

func (m *Model) setStatusErr(msg string) {
	m.status = msg
	m.statusErr = true
}

func (m *Model) setStatusOK(msg string) {
	m.status = msg
	m.statusErr = false
}

func (m *Model) notifyUser(msg string, isErr bool) tea.Cmd {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return nil
	}
	m.toastSeq++
	token := m.toastSeq
	m.toast = &toastState{
		message: msg,
		err:     isErr,
		token:   token,
	}
	return clearToastCmd(token)
}

func isDaemonRunning() bool {
	return exec.Command("pgrep", "-x", "hyprmoncfgd").Run() == nil
}

func (m *Model) markDirty() {
	m.dirty = true
	m.draftSaved = false
}

func (m *Model) markClean() {
	m.dirty = false
	m.draftSaved = false
}

func editableOutputFromMonitor(m hypr.Monitor, matchCounts map[string]int) editableOutput {
	output := editableOutput{
		Key:             hypr.MonitorOutputKey(m, matchCounts),
		MatchKey:        m.HardwareKey(),
		Name:            m.Name,
		Description:     m.Description,
		Make:            m.Make,
		Model:           m.Model,
		Serial:          m.Serial,
		PhysicalWidth:   m.PhysicalWidth,
		PhysicalHeight:  m.PhysicalHeight,
		Enabled:         !m.Disabled,
		Width:           m.Width,
		Height:          m.Height,
		Refresh:         m.RefreshRate,
		X:               m.X,
		Y:               m.Y,
		Scale:           clampFloat(m.Scale, 0.25, 4.0),
		VRR:             int(m.VRR),
		Transform:       m.Transform,
		Focused:         m.Focused,
		DPMSStatus:      m.DPMSStatus,
		IsInternal:      m.IsInternal(),
		MirrorOf:        m.MirrorOf,
		ActiveWorkspace: m.ActiveWorkspace.Name,
		Bitdepth:        m.Bitdepth(),
		CM:              m.ColorManagementPreset,
		SDRBrightness:   m.SDRBrightness,
		SDRSaturation:   m.SDRSaturation,
		SDRMinLuminance: m.SDRMinLuminance,
		SDRMaxLuminance: m.SDRMaxLuminance,
	}

	output.Modes = normalizeModes(m.AvailableModes, m.ModeString())
	output.ModeIndex = indexOf(output.Modes, m.ModeString())
	if output.ModeIndex < 0 {
		output.ModeIndex = 0
	}
	if len(output.Modes) > 0 {
		output.applyMode(output.Modes[output.ModeIndex])
	}
	return output
}

func editableOutputFromProfile(saved profile.OutputConfig, live hypr.Monitor, hasLive bool) editableOutput {
	output := editableOutput{
		Key:               saved.Key,
		MatchKey:          saved.MatchIdentity(),
		Name:              saved.Name,
		Make:              saved.Make,
		Model:             saved.Model,
		Serial:            saved.Serial,
		Enabled:           saved.Enabled,
		Width:             saved.Width,
		Height:            saved.Height,
		Refresh:           saved.Refresh,
		X:                 saved.X,
		Y:                 saved.Y,
		Scale:             clampFloat(saved.Scale, 0.25, 4.0),
		VRR:               saved.VRR,
		Transform:         saved.Transform,
		IsInternal:        isInternalOutputName(saved.Name),
		MirrorOf:          saved.MirrorOf,
		Bitdepth:          saved.Bitdepth,
		CM:                saved.CM,
		SDRBrightness:     saved.SDRBrightness,
		SDRSaturation:     saved.SDRSaturation,
		SDRMinLuminance:   saved.SDRMinLuminance,
		SDRMaxLuminance:   saved.SDRMaxLuminance,
		MinLuminance:      saved.MinLuminance,
		MaxLuminance:      saved.MaxLuminance,
		SupportsWideColor: saved.SupportsWideColor,
		SupportsHDR:       saved.SupportsHDR,
		MaxAvgLuminance:   saved.MaxAvgLuminance,
		SDREOTF:           saved.SDREOTF,
		ICC:               saved.ICC,
	}

	mode := saved.NormalizedMode()
	if hasLive {
		output.Description = live.Description
		output.PhysicalWidth = live.PhysicalWidth
		output.PhysicalHeight = live.PhysicalHeight
		output.Focused = live.Focused
		output.DPMSStatus = live.DPMSStatus
		output.IsInternal = live.IsInternal()
		output.ActiveWorkspace = live.ActiveWorkspace.Name
		output.Modes = normalizeModes(live.AvailableModes, mode)
	} else {
		output.Modes = normalizeModes(nil, mode)
	}
	output.ModeIndex = indexOf(output.Modes, mode)
	if output.ModeIndex < 0 {
		output.ModeIndex = 0
	}
	if len(output.Modes) > 0 {
		output.applyMode(output.Modes[output.ModeIndex])
	}
	return output
}

func workspaceEditorFromSettings(settings profile.WorkspaceSettings, outputs []editableOutput) workspaceEditor {
	mirroredKeys := make(map[string]bool)
	for _, output := range outputs {
		if output.MirrorOf != "" {
			mirroredKeys[output.Key] = true
		}
	}

	order := append([]string(nil), settings.MonitorOrder...)
	if len(order) == 0 {
		order = workspaceOrderFromEditorRules(settings.Rules, outputs)
	}
	if len(order) == 0 {
		for _, output := range outputs {
			if output.MirrorOf == "" {
				order = append(order, output.Key)
			}
		}
	}

	seen := make(map[string]bool, len(order))
	normalized := make([]string, 0, len(outputs))
	for _, key := range order {
		if key == "" || seen[key] || mirroredKeys[key] {
			continue
		}
		normalized = append(normalized, key)
		seen[key] = true
	}
	for _, output := range outputs {
		if !seen[output.Key] && !mirroredKeys[output.Key] {
			normalized = append(normalized, output.Key)
			seen[output.Key] = true
		}
	}

	strategy := settings.Strategy
	if strategy == "" {
		if len(settings.Rules) > 0 {
			strategy = profile.WorkspaceStrategyManual
		} else {
			strategy = profile.WorkspaceStrategySequential
		}
	}

	maxWorkspaces := settings.MaxWorkspaces
	if maxWorkspaces <= 0 {
		maxWorkspaces = 9
	}
	groupSize := settings.GroupSize
	if groupSize <= 0 {
		groupSize = defaultWorkspaceGroupSize
	}
	lastSequentialGroupSize := groupSize
	if strategy != profile.WorkspaceStrategySequential && lastSequentialGroupSize <= 1 {
		lastSequentialGroupSize = defaultWorkspaceGroupSize
	}

	return workspaceEditor{
		Enabled:                 settings.Enabled,
		Strategy:                strategy,
		MaxWorkspaces:           maxWorkspaces,
		GroupSize:               groupSize,
		LastSequentialGroupSize: lastSequentialGroupSize,
		MonitorOrder:            normalized,
		Rules:                   append([]profile.WorkspaceRule(nil), settings.Rules...),
	}
}

func workspaceOrderFromEditorRules(rules []profile.WorkspaceRule, outputs []editableOutput) []string {
	if len(rules) == 0 || len(outputs) == 0 {
		return nil
	}

	byName := make(map[string]string, len(outputs))
	byKey := make(map[string]editableOutput, len(outputs))
	for _, output := range outputs {
		byName[output.Name] = output.Key
		byKey[output.Key] = output
	}

	order := make([]string, 0, len(rules))
	seen := make(map[string]bool, len(rules))
	for _, rule := range rules {
		key := strings.TrimSpace(rule.OutputKey)
		if _, ok := byKey[key]; !ok {
			if mapped, ok := byName[strings.TrimSpace(rule.OutputName)]; ok {
				key = mapped
			}
		}
		if key == "" || seen[key] {
			continue
		}
		if output, ok := byKey[key]; ok && output.MirrorOf == "" {
			order = append(order, key)
			seen[key] = true
		}
	}
	return order
}

func (w workspaceEditor) settings() profile.WorkspaceSettings {
	return profile.WorkspaceSettings{
		Enabled:       w.Enabled,
		Strategy:      w.Strategy,
		MaxWorkspaces: w.MaxWorkspaces,
		GroupSize:     w.GroupSize,
		MonitorOrder:  append([]string(nil), w.MonitorOrder...),
		Rules:         append([]profile.WorkspaceRule(nil), w.Rules...),
	}
}

func (o *editableOutput) applyMode(mode string) {
	width, height, refresh, ok := hypr.ParseMode(mode)
	if !ok {
		return
	}
	o.Width = width
	o.Height = height
	o.Refresh = refresh
}

func (o editableOutput) DisplayMode() string {
	if len(o.Modes) > 0 && o.ModeIndex >= 0 && o.ModeIndex < len(o.Modes) {
		return strings.TrimSpace(o.Modes[o.ModeIndex])
	}
	return hypr.FormatMode(o.Width, o.Height, o.Refresh)
}

func (o editableOutput) profileOutput() profile.OutputConfig {
	return profile.OutputConfig{
		Key:               o.Key,
		MatchKey:          o.MatchKey,
		Name:              o.Name,
		Make:              o.Make,
		Model:             o.Model,
		Serial:            o.Serial,
		Enabled:           o.Enabled,
		Mode:              o.DisplayMode(),
		Width:             o.Width,
		Height:            o.Height,
		Refresh:           o.Refresh,
		X:                 o.X,
		Y:                 o.Y,
		Scale:             o.Scale,
		VRR:               o.VRR,
		Transform:         o.Transform,
		MirrorOf:          o.MirrorOf,
		Bitdepth:          o.Bitdepth,
		CM:                o.CM,
		SDRBrightness:     o.SDRBrightness,
		SDRSaturation:     o.SDRSaturation,
		SDRMinLuminance:   o.SDRMinLuminance,
		SDRMaxLuminance:   o.SDRMaxLuminance,
		MinLuminance:      o.MinLuminance,
		MaxLuminance:      o.MaxLuminance,
		SupportsWideColor: o.SupportsWideColor,
		SupportsHDR:       o.SupportsHDR,
		MaxAvgLuminance:   o.MaxAvgLuminance,
		SDREOTF:           o.SDREOTF,
		ICC:               o.ICC,
	}
}

func (o editableOutput) logicalSize() (int, int) {
	scale := o.Scale
	if scale <= 0 {
		scale = 1
	}
	width := int(math.Round(float64(o.Width) / scale))
	height := int(math.Round(float64(o.Height) / scale))
	if o.Transform%2 == 1 {
		width, height = height, width
	}
	return max(1, width), max(1, height)
}

func (o editableOutput) layoutSizeLabel() string {
	width, height := o.logicalSize()
	return fmt.Sprintf("%d x %d", width, height)
}

func (o editableOutput) displayModelLabel() string {
	if label := strings.TrimSpace(o.Make + " " + o.Model); label != "" {
		return label
	}
	if model := strings.TrimSpace(o.Model); model != "" {
		return model
	}
	// Hyprland may report a placeholder description (e.g. "mirror-0") for
	// monitors that are actively mirroring. Skip Description in that case.
	if o.MirrorOf == "" {
		if desc := strings.TrimSpace(o.Description); desc != "" {
			return desc
		}
	}
	return "(unknown)"
}

func isInternalOutputName(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	return strings.HasPrefix(name, "edp") || strings.HasPrefix(name, "lvds") || strings.HasPrefix(name, "dsi")
}

type cardLine struct {
	text string
	fg   string
	bold bool
}

func (o editableOutput) cardModelLabel() string {
	if o.IsInternal {
		return "Internal · " + o.displayModelLabel()
	}
	return o.displayModelLabel()
}

func (o editableOutput) cardLines(maxLines int, fg string, muted string) []cardLine {
	if maxLines <= 0 {
		return nil
	}

	scaleLayout := fmt.Sprintf("%.2fx=%s", o.Scale, strings.ReplaceAll(o.layoutSizeLabel(), " ", ""))
	position := fmt.Sprintf("pos %d,%d", o.X, o.Y)
	full := []cardLine{
		{text: o.Name, fg: fg, bold: true},
		{text: o.cardModelLabel(), fg: muted},
		{text: o.DisplayMode(), fg: muted},
		{text: scaleLayout, fg: muted},
		{text: position, fg: muted},
	}
	if maxLines >= len(full) {
		return full
	}

	switch maxLines {
	case 4:
		return []cardLine{
			full[0],
			full[1],
			full[2],
			{text: scaleLayout + "  " + position, fg: muted},
		}
	case 3:
		return []cardLine{
			full[0],
			full[2],
			{text: scaleLayout + "  " + position, fg: muted},
		}
	case 2:
		return []cardLine{
			full[0],
			full[3],
		}
	default:
		return []cardLine{full[0]}
	}
}

func (m Model) newCanvasCells(width, height int) [][]canvasCell {
	grid := make([][]canvasCell, height)
	p := m.styles.palette
	for y := 0; y < height; y++ {
		row := make([]canvasCell, width)
		for x := 0; x < width; x++ {
			cell := canvasCell{ch: ' ', fg: p.canvasGrid, bg: p.canvasBg}
			switch {
			case y%4 == 0 && x%8 == 0:
				cell.ch = '┼'
				cell.fg = p.canvasAxis
			case y%4 == 0:
				cell.ch = '─'
				cell.fg = p.canvasGrid
			case x%8 == 0:
				cell.ch = '│'
				cell.fg = p.canvasGrid
			}
			row[x] = cell
		}
		grid[y] = row
	}
	return grid
}

func (m Model) canvasCardStyle(output editableOutput, selected bool) canvasCardColors {
	p := m.styles.palette
	colors := canvasCardColors{
		bg:     p.cardBg,
		border: p.cardBorder,
		fg:     p.cardFg,
		muted:  p.cardMuted,
	}
	if !output.Enabled {
		colors = canvasCardColors{
			bg:     p.cardDisabledBg,
			border: p.cardDisabledBorder,
			fg:     p.cardDisabledFg,
			muted:  p.cardDisabledMuted,
		}
	}
	if selected {
		colors = canvasCardColors{
			bg:     p.cardSelectedBg,
			border: p.cardSelectedBorder,
			fg:     p.cardSelectedFg,
			muted:  p.cardSelectedMuted,
		}
	}
	if m.layoutErr != nil && m.isOutputOverlapping(output) {
		colors.border = "#FF0000"
		colors.fg = "#FF0000"
	}
	return colors
}

func renderCanvasLegendItem(label string, colors canvasCardColors) string {
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(0, 1)
	style = withFG(style, colors.fg)
	style = withBG(style, colors.bg)
	if colors.border != "" {
		style = style.BorderForeground(lipgloss.Color(colors.border))
	}
	if colors.bg != "" {
		style = style.BorderBackground(lipgloss.Color(colors.bg))
	}
	return style.Render(label)
}

func paintMonitorCard(grid [][]canvasCell, rect canvasRect, output editableOutput, selected bool, colors canvasCardColors) {
	if len(grid) == 0 || len(grid[0]) == 0 {
		return
	}

	x1 := clampInt(rect.x, 0, len(grid[0])-1)
	y1 := clampInt(rect.y, 0, len(grid)-1)
	x2 := clampInt(rect.x+rect.w-1, 0, len(grid[0])-1)
	y2 := clampInt(rect.y+rect.h-1, 0, len(grid)-1)
	if x2-x1 < 2 || y2-y1 < 2 {
		return
	}

	for y := y1; y <= y2; y++ {
		for x := x1; x <= x2; x++ {
			grid[y][x] = canvasCell{ch: ' ', fg: colors.fg, bg: colors.bg}
		}
	}

	for x := x1 + 1; x < x2; x++ {
		grid[y1][x] = canvasCell{ch: '─', fg: colors.border, bg: colors.bg, bold: selected}
		grid[y2][x] = canvasCell{ch: '─', fg: colors.border, bg: colors.bg, bold: selected}
	}
	for y := y1 + 1; y < y2; y++ {
		grid[y][x1] = canvasCell{ch: '│', fg: colors.border, bg: colors.bg, bold: selected}
		grid[y][x2] = canvasCell{ch: '│', fg: colors.border, bg: colors.bg, bold: selected}
	}
	grid[y1][x1] = canvasCell{ch: '╭', fg: colors.border, bg: colors.bg, bold: selected}
	grid[y1][x2] = canvasCell{ch: '╮', fg: colors.border, bg: colors.bg, bold: selected}
	grid[y2][x1] = canvasCell{ch: '╰', fg: colors.border, bg: colors.bg, bold: selected}
	grid[y2][x2] = canvasCell{ch: '╯', fg: colors.border, bg: colors.bg, bold: selected}

	availableHeight := y2 - y1 - 1
	lines := output.cardLines(max(1, availableHeight), colors.fg, colors.muted)
	startY := y1 + 1 + max(0, (availableHeight-len(lines))/2)
	for idx, line := range lines {
		y := startY + idx
		if y <= y1 || y >= y2 {
			continue
		}
		paintCanvasTextCentered(grid, x1+1, x2-1, y, fitString(line.text, x2-x1-1), line.fg, colors.bg, line.bold)
	}
}

func paintCanvasTextCentered(grid [][]canvasCell, left, right, y int, text string, fg string, bg string, bold bool) {
	if y < 0 || y >= len(grid) || left > right {
		return
	}
	runes := []rune(text)
	width := right - left + 1
	if len(runes) > width {
		runes = []rune(fitString(text, width))
	}
	start := left + max(0, (width-len(runes))/2)
	for idx, r := range runes {
		x := start + idx
		if x < left || x > right || x < 0 || x >= len(grid[y]) {
			continue
		}
		grid[y][x] = canvasCell{ch: r, fg: fg, bg: bg, bold: bold}
	}
}

func paintSnapMark(grid [][]canvasCell, rect canvasRect, edge snapEdge, highlight string) {
	if len(grid) == 0 || len(grid[0]) == 0 {
		return
	}

	x1 := clampInt(rect.x, 0, len(grid[0])-1)
	y1 := clampInt(rect.y, 0, len(grid)-1)
	x2 := clampInt(rect.x+rect.w-1, 0, len(grid[0])-1)
	y2 := clampInt(rect.y+rect.h-1, 0, len(grid)-1)
	switch edge {
	case snapEdgeLeft:
		for y := y1; y <= y2; y++ {
			grid[y][x1] = canvasCell{ch: '┃', fg: highlight, bg: grid[y][x1].bg, bold: true}
		}
	case snapEdgeRight:
		for y := y1; y <= y2; y++ {
			grid[y][x2] = canvasCell{ch: '┃', fg: highlight, bg: grid[y][x2].bg, bold: true}
		}
	case snapEdgeTop:
		for x := x1; x <= x2; x++ {
			grid[y1][x] = canvasCell{ch: '━', fg: highlight, bg: grid[y1][x].bg, bold: true}
		}
	case snapEdgeBottom:
		for x := x1; x <= x2; x++ {
			grid[y2][x] = canvasCell{ch: '━', fg: highlight, bg: grid[y2][x].bg, bold: true}
		}
	}
}

func renderCanvasCells(grid [][]canvasCell) string {
	lines := make([]string, len(grid))
	for y, row := range grid {
		var line strings.Builder
		var run strings.Builder
		cur := canvasCell{}
		have := false
		flush := func() {
			if !have || run.Len() == 0 {
				return
			}
			style := lipgloss.NewStyle()
			if cur.fg != "" {
				style = style.Foreground(lipgloss.Color(cur.fg))
			}
			if cur.bg != "" {
				style = style.Background(lipgloss.Color(cur.bg))
			}
			if cur.bold {
				style = style.Bold(true)
			}
			line.WriteString(style.Render(run.String()))
			run.Reset()
		}
		for _, cell := range row {
			if !have || cell.fg != cur.fg || cell.bg != cur.bg || cell.bold != cur.bold {
				flush()
				cur = cell
				have = true
			}
			run.WriteRune(cell.ch)
		}
		flush()
		lines[y] = line.String()
	}
	return strings.Join(lines, "\n")
}

func normalizeModes(modes []string, current string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(modes)+1)
	add := func(mode string) {
		mode = strings.TrimSpace(mode)
		if mode == "" || seen[mode] {
			return
		}
		seen[mode] = true
		out = append(out, mode)
	}

	add(current)
	for _, mode := range modes {
		add(mode)
	}
	return out
}

func indexOf(values []string, target string) int {
	target = strings.TrimSpace(target)
	for idx, value := range values {
		if strings.TrimSpace(value) == target {
			return idx
		}
	}
	return -1
}

func fitString(value string, width int) string {
	if width <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	if width <= 3 {
		return string(runes[:width])
	}
	return string(runes[:width-3]) + "..."
}

func blankFallback(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func defaultProfileName() string {
	return "profile-" + time.Now().Format("20060102-150405")
}

func (m *Model) showSnapHint(hint *snapHintState) tea.Cmd {
	if hint == nil {
		m.snap = nil
		return nil
	}
	m.snapSeq++
	hint.Token = m.snapSeq
	m.snap = hint
	return clearSnapCmd(hint.Token)
}

func clearSnapCmd(token int) tea.Cmd {
	return tea.Tick(700*time.Millisecond, func(time.Time) tea.Msg {
		return clearSnapMsg{token: token}
	})
}

func clearToastCmd(token int) tea.Cmd {
	return tea.Tick(4*time.Second, func(time.Time) tea.Msg {
		return clearToastMsg{token: token}
	})
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func targetLabel(name string) string {
	if strings.TrimSpace(name) == "" || name == "draft" {
		return "Draft changes"
	}
	return fmt.Sprintf("Profile %q", name)
}

func boolText(v bool) string {
	if v {
		return "on"
	}
	return "off"
}

func vrrLabel(v int) string {
	switch v {
	case 1:
		return "on"
	case 2:
		return "fullscreen"
	default:
		return "off"
	}
}

func triStateLabel(v int) string {
	switch v {
	case -1:
		return "off"
	case 1:
		return "on"
	default:
		return "auto"
	}
}

func transformLabel(v int) string {
	switch v {
	case 0:
		return "normal"
	case 1:
		return "90"
	case 2:
		return "180"
	case 3:
		return "270"
	case 4:
		return "flip"
	case 5:
		return "flip-90"
	case 6:
		return "flip-180"
	case 7:
		return "flip-270"
	default:
		return fmt.Sprintf("%d", v)
	}
}

func blankStrategy(strategy profile.WorkspaceStrategy) profile.WorkspaceStrategy {
	if strategy == "" {
		return profile.WorkspaceStrategySequential
	}
	return strategy
}

func wrapIndex(idx, length int) int {
	if length <= 0 {
		return 0
	}
	for idx < 0 {
		idx += length
	}
	return idx % length
}

func wrapValue(value, minValue, maxValue int) int {
	if maxValue < minValue {
		return minValue
	}
	rangeSize := maxValue - minValue + 1
	for value < minValue {
		value += rangeSize
	}
	for value > maxValue {
		value -= rangeSize
	}
	return value
}

func clampIndex(idx, length int) int {
	if length <= 0 {
		return 0
	}
	if idx < 0 {
		return length - 1
	}
	if idx >= length {
		return 0
	}
	return idx
}

func layoutMoveDelta(key string) (dx, dy int, ok bool) {
	switch key {
	case "left", "h":
		return -100, 0, true
	case "right", "l":
		return 100, 0, true
	case "up", "k":
		return 0, -100, true
	case "down", "j":
		return 0, 100, true
	case "shift+left":
		return -10, 0, true
	case "shift+right":
		return 10, 0, true
	case "shift+up":
		return 0, -10, true
	case "shift+down":
		return 0, 10, true
	case "ctrl+left":
		return -1, 0, true
	case "ctrl+right":
		return 1, 0, true
	case "ctrl+up":
		return 0, -1, true
	case "ctrl+down":
		return 0, 1, true
	case "H":
		return -500, 0, true
	case "L":
		return 500, 0, true
	case "K":
		return 0, -500, true
	case "J":
		return 0, 500, true
	default:
		return 0, 0, false
	}
}

func clampFloat(value, minValue, maxValue float64) float64 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func clampInt(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func focusedOutputIndex(outputs []editableOutput) int {
	for idx, output := range outputs {
		if output.Focused {
			return idx
		}
	}
	return -1
}

func outputIndexByKey(outputs []editableOutput, key string) int {
	for idx, output := range outputs {
		if output.Key == key {
			return idx
		}
	}
	return 0
}

var layoutFields = []string{
	"Enabled",
	"Mode",
	"Scale",
	"Bit Depth",
	"Color Mgmt",
	"VRR",
	"Transform",
	"Position X",
	"Position Y",
	"Mirror",
	"SDR Bright",
	"SDR Sat",
	"SDR Min Lum",
	"SDR Max Lum",
	"SDR Curve",
	"Min Lum",
	"Max Lum",
	"Max Avg Lum",
	"Force Wide",
	"Force HDR",
	"ICC Path",
}

const advancedFieldStart = 10

func layoutFieldShortLabel(field int) string {
	switch field {
	case 0:
		return "On"
	case 6:
		return "Rot"
	case 7:
		return "X"
	case 8:
		return "Y"
	case 9:
		return "Mirror"
	default:
		return layoutFields[field]
	}
}

var workspaceFields = []string{
	"Enabled",
	"Strategy",
	"Max workspaces",
	"Group size",
}

func (m Model) isOutputOverlapping(o editableOutput) bool {
	if !o.Enabled || o.MirrorOf != "" {
		return false
	}
	w1, h1 := o.logicalSize()
	x1_1, y1_1 := o.X, o.Y
	x2_1, y2_1 := o.X+w1, o.Y+h1

	for _, other := range m.editOutputs {
		if other.Name == o.Name || !other.Enabled || other.MirrorOf != "" {
			continue
		}

		w2, h2 := other.logicalSize()
		x1_2, y1_2 := other.X, other.Y
		x2_2, y2_2 := other.X+w2, other.Y+h2

		if x1_1 < x2_2 && x2_1 > x1_2 &&
			y1_1 < y2_2 && y2_1 > y1_2 {
			return true
		}
	}
	return false
}

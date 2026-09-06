package tui

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/crmne/hyprmoncfg/internal/apply"
	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/ipc"
	"github.com/crmne/hyprmoncfg/internal/lid"
	"github.com/crmne/hyprmoncfg/internal/omarchywatch"
	"github.com/crmne/hyprmoncfg/internal/profile"
	"github.com/crmne/hyprmoncfg/internal/profileio"
	"github.com/crmne/hyprmoncfg/internal/scaling"
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
	modeKeybindings
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

type inspectorTab int

const (
	inspectorTabDisplay inspectorTab = iota
	inspectorTabColor
)

type refreshMsg struct {
	monitors        []hypr.Monitor
	profiles        []profile.Profile
	workspaceRules  []hypr.WorkspaceRule
	workspaces      []hypr.WorkspaceState
	lidState        lid.State
	daemonOK        bool
	daemonUnknown   bool
	daemonVersion   string
	profileOverride string
	daemonClient    *ipc.Client
	background      bool
	err             error
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
	profile       profile.Profile
	snapshot      apply.RevertState
	transactionID string
	deadline      time.Time
	remote        bool
	err           error
}

type daemonRestartMsg struct {
	err error
}

type profileAutoMsg struct {
	enabled bool
	err     error
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
	profile       profile.Profile
	snapshot      apply.RevertState
	transactionID string
	deadline      time.Time
	remote        bool
}

type pendingRevertGuard struct {
	mu       sync.Mutex
	armed    bool
	snapshot apply.RevertState
	inFlight int
	idle     chan struct{}
}

type pendingRemoteGuard struct {
	mu            sync.Mutex
	armed         bool
	transactionID string
	inFlight      int
	idle          chan struct{}
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
	ModeUnsupported   bool
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

type snapDirection int

const (
	snapDirectionLeft snapDirection = iota
	snapDirectionRight
	snapDirectionUp
	snapDirectionDown
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
	ManualRulesInitialized  bool
	SelectedField           int
	SelectedOrder           int
}

type Model struct {
	client  *hypr.Client
	store   *profile.Store
	engine  apply.Engine
	ipc     *ipc.Client
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
	inspectorTab    inspectorTab
	selectedProfile int

	pending       *pendingApply
	revertGuard   *pendingRevertGuard
	remoteGuard   *pendingRemoteGuard
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

	resetRequested     bool
	status             string
	statusErr          bool
	dirty              bool
	draftSaved         bool
	draftProfileName   string
	matchedProfileName string
	activeProfileName  string
	draftExec          string
	daemonOK           bool
	daemonVersion      string
	profileOverride    string
	profileModePending bool
	refreshInFlight    bool
	applying           bool
	quitAfterApply     bool
	quitAfterRevert    bool

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
			WakeConfig:         omarchywatch.NewWakeConfig(),
			MonitorsConfPath:   monitorsConfPath,
			HyprlandConfigPath: hyprlandConfigPath,
			Logf: func(format string, args ...any) {
				fmt.Fprintf(os.Stderr, format, args...)
			},
		},
		openURL:     openExternalURL,
		revertGuard: &pendingRevertGuard{},
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

func NewModelWithIPC(client *hypr.Client, store *profile.Store, monitorsConfPath string, hyprlandConfigPath string, ipcClient *ipc.Client) Model {
	model := NewModel(client, store, monitorsConfPath, hyprlandConfigPath)
	model.ipc = ipcClient
	model.remoteGuard = &pendingRemoteGuard{}
	model.daemonOK = ipcClient != nil
	return model
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
		if msg.daemonClient != nil {
			if m.ipc != nil {
				_ = m.ipc.Close()
			}
			m.ipc = msg.daemonClient
		}
		if !msg.daemonUnknown {
			m.daemonOK = msg.daemonOK
			m.daemonVersion = msg.daemonVersion
			m.profileOverride = msg.profileOverride
		}
		if msg.err != nil {
			m.setStatusErr(msg.err.Error())
			return m, nil
		}

		prevSig := m.liveConfigSignature()
		nextSig := liveConfigSignature(msg.monitors, msg.lidState)
		liveChanged := prevSig != nextSig
		wasDirty := m.dirty

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
			m.quitAfterApply = false
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
		m.matchedProfileName = msg.name
		m.draftSaved = true
		m.mode = modeMain
		m.quitAfterApply = false
		if action == saveActionCancel {
			m.setStatusOK("Save cancelled")
			return m, nil
		}
		if action == saveActionSaveQuit {
			m.quitAfterApply = true
			m.applying = true
			return m, m.applyCmd(m.currentProfile(msg.name))
		}
		if action == saveActionApply {
			m.applying = true
			return m, tea.Batch(
				m.refreshCmd(false),
				m.applyCmd(m.currentProfile(msg.name)),
			)
		}
		m.setStatusOK(fmt.Sprintf("Saved profile %q", msg.name))
		return m, m.refreshCmd(false)

	case daemonRestartMsg:
		if msg.err != nil {
			m.setStatusErr(msg.err.Error())
			return m, nil
		}
		m.setStatusOK("Daemon restarted")
		return m, m.refreshCmd(true)

	case profileAutoMsg:
		m.profileModePending = false
		if msg.err != nil {
			m.setStatusErr(msg.err.Error())
			return m, nil
		}
		if msg.enabled {
			m.profileOverride = ""
			m.setStatusOK("Automatic profile selection on")
		} else {
			m.profileOverride = m.activeProfileName
			m.setStatusOK("Automatic profile selection off")
		}
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
		if strings.EqualFold(strings.TrimSpace(msg.name), strings.TrimSpace(m.matchedProfileName)) {
			m.matchedProfileName = ""
		}
		m.setStatusOK(fmt.Sprintf("Deleted profile %q", msg.name))
		m.selectedProfile = clampIndex(m.selectedProfile, len(m.profiles))
		return m, m.refreshCmd(false)

	case applyMsg:
		m.applying = false
		if msg.err != nil {
			if m.quitAfterRevert {
				m.quitAfterRevert = false
				m.quitAfterApply = false
				return m, tea.Quit
			}
			m.quitAfterApply = false
			m.setStatusErr(msg.err.Error())
			m.mode = modeMain
			return m, nil
		}
		deadline := msg.deadline
		if deadline.IsZero() {
			deadline = time.Now().Add(10 * time.Second)
		}
		m.pending = &pendingApply{
			profile:       msg.profile,
			snapshot:      msg.snapshot,
			transactionID: msg.transactionID,
			deadline:      deadline,
			remote:        msg.remote,
		}
		if msg.remote {
			m.armPendingRemote(msg.transactionID)
		} else {
			m.armPendingRevert(msg.snapshot)
		}
		m.mode = modeConfirm
		m.statusErr = false
		m.status = fmt.Sprintf("%s applied. Changes are live until you confirm or revert.", targetLabel(msg.profile.Name))
		if m.quitAfterRevert {
			return m, m.revertCmd(*m.pending, "quit")
		}
		return m, tickCmd()

	case revertMsg:
		quitAfterRevert := m.quitAfterRevert
		m.quitAfterRevert = false
		if msg.err != nil {
			m.mode = modeConfirm
			if m.pending != nil {
				m.pending.deadline = time.Now().Add(10 * time.Second)
			}
			m.setStatusErr(fmt.Sprintf("Revert failed: %v", msg.err))
			return m, nil
		}
		m.mode = modeMain
		m.pending = nil
		m.quitAfterApply = false
		m.disarmPendingRevert()
		m.disarmPendingRemote()
		m.markClean()
		m.draftProfileName = ""
		m.matchedProfileName = ""
		m.draftExec = ""
		m.setStatusOK("Configuration reverted: " + msg.reason)
		if quitAfterRevert {
			return m, tea.Quit
		}
		return m, m.refreshCmd(false)

	case openURLMsg:
		if msg.err != nil {
			m.setStatusErr(fmt.Sprintf("Failed to open %s link: %v", msg.label, msg.err))
		}
		return m, nil

	case tickMsg:
		if m.mode == modeConfirm && m.pending != nil {
			if time.Now().After(m.pending.deadline) {
				return m, m.revertCmd(*m.pending, "timeout")
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
		case modeKeybindings:
			if msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
			m.mode = modeMain
			return m, nil
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
		if m.applying {
			m.quitAfterRevert = true
			m.statusErr = false
			m.status = "Waiting for apply to finish, then restoring the previous configuration..."
			return m, nil
		}
		if m.dirty {
			return m.openQuitSaveDialog()
		}
		return m, tea.Quit
	case "1":
		m.tab = tabLayout
		return m, nil
	case "2":
		m.tab = tabProfiles
		return m, nil
	case "3":
		m.tab = tabWorkspaces
		if m.workspaceEdit.Strategy == profile.WorkspaceStrategyManual && !m.workspaceEdit.ManualRulesInitialized {
			m.workspaceEdit.Rules = m.materializeManualWorkspaceRules()
			m.workspaceEdit.ManualRulesInitialized = len(m.workspaceEdit.Rules) > 0
			if m.workspaceEdit.ManualRulesInitialized {
				m.markDirty()
			}
		}
		return m, nil
	case "?":
		m.mode = modeKeybindings
		return m, nil
	case "R":
		return m, m.restartDaemonCmd()
	case "r":
		m.resetRequested = true
		m.draftProfileName = ""
		m.matchedProfileName = ""
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
		if m.applying {
			m.setStatusErr("A configuration is already being applied")
			return m, nil
		}
		if m.tab == tabProfiles {
			if len(m.profiles) == 0 {
				m.setStatusErr("No profiles available")
				return m, nil
			}
			if m.profileSelectionLocked() {
				m.setStatusErr("Turn off automatic profile selection before choosing a profile")
				return m, nil
			}
			m.applying = true
			target := m.profiles[m.selectedProfile]
			return m, m.applyCmd(target)
		}
		m.applying = true
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
		m.cycleLayoutPane(1)
		return m, nil
	case "shift+tab":
		m.cycleLayoutPane(-1)
		return m, nil
	case "0":
		m.moveSelectedOutputToOrigin()
		return m, nil
	case "[":
		m.selectedOutput = clampIndex(m.selectedOutput-1, len(m.editOutputs))
		return m, nil
	case "]":
		m.selectedOutput = clampIndex(m.selectedOutput+1, len(m.editOutputs))
		return m, nil
	}

	if m.layoutFocus == layoutFocusCanvas {
		if direction, ok := layoutSnapDirection(msg.String()); ok {
			return m, m.snapSelectedOutput(direction)
		}
		if dx, dy, ok := layoutMoveDelta(msg.String()); ok {
			return m, m.nudgeSelectedOutput(dx, dy, 24)
		}
		switch msg.String() {
		case " ":
			m.toggleSelectedOutput()
			return m, nil
		case "enter":
			m.layoutFocus = layoutFocusInspector
			m.normalizeInspectorField()
			return m, nil
		default:
			return m, nil
		}
	}

	switch msg.String() {
	case "up", "k":
		m.moveInspectorField(-1)
	case "down", "j":
		m.moveInspectorField(1)
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
	case " ":
		return m.toggleProfileAutomatic()
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
	case "enter":
		if len(m.profiles) == 0 {
			m.setStatusErr("No profiles available")
			return m, nil
		}
		if m.profileSelectionLocked() {
			m.setStatusErr("Turn off automatic profile selection before choosing a profile")
			return m, nil
		}
		if m.applying {
			m.setStatusErr("A configuration is already being applied")
			return m, nil
		}
		m.applying = true
		return m, m.applyCmd(m.profiles[m.selectedProfile])
	case "l":
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

func (m Model) profileAutomatic() bool {
	return m.daemonOK && strings.TrimSpace(m.profileOverride) == ""
}

func (m Model) profileSelectionLocked() bool {
	return m.profileAutomatic()
}

func (m Model) toggleProfileAutomatic() (tea.Model, tea.Cmd) {
	if m.ipc == nil || !m.daemonOK {
		m.setStatusErr("Automatic profile selection requires the daemon")
		return m, nil
	}
	if m.profileModePending {
		m.setStatusErr("Profile selection mode is already being updated")
		return m, nil
	}
	if m.applying || m.pending != nil {
		m.setStatusErr("Finish the active profile change first")
		return m, nil
	}

	enabled := !m.profileAutomatic()
	m.profileModePending = true
	if enabled {
		m.setStatusOK("Enabling automatic profile selection...")
	} else {
		m.setStatusOK("Turning off automatic profile selection...")
	}
	return m, m.setProfileAutomaticCmd(enabled)
}

func (m Model) updateWorkspaceKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	inItems := m.workspaceEdit.SelectedField >= len(workspaceFields)
	if inItems {
		m.workspaceEdit.SelectedOrder = m.workspaceEdit.SelectedField - len(workspaceFields)
	}

	switch msg.String() {
	case "up":
		m.moveWorkspaceSelection(-1, true)
		return m, nil
	case "down":
		m.moveWorkspaceSelection(1, true)
		return m, nil
	case "pgup":
		m.moveWorkspaceSelection(-m.workspacePageStep(), false)
		return m, nil
	case "pgdown":
		m.moveWorkspaceSelection(m.workspacePageStep(), false)
		return m, nil
	case "home":
		m.setWorkspaceSelection(0)
		return m, nil
	case "end":
		m.setWorkspaceSelection(m.workspaceItemCount() - 1)
		return m, nil
	case "left", "h", "-", "_":
		if inItems {
			m.adjustWorkspaceItem(-1)
		} else {
			m.adjustWorkspaceField(-1)
		}
	case "right", "l", "+", "=":
		if inItems {
			m.adjustWorkspaceItem(1)
		} else {
			m.adjustWorkspaceField(1)
		}
	case "enter":
		if !inItems {
			switch m.workspaceEdit.SelectedField {
			case 2:
				return m, m.openNumericInput(
					numericInputWorkspaceCount,
					-1,
					"Set Workspace Count",
					"Type any positive number. Enter applies. Esc cancels.",
					strconv.Itoa(m.workspaceEdit.MaxWorkspaces),
				)
			case 3:
				if m.workspaceEdit.Strategy == profile.WorkspaceStrategySequential {
					return m, m.openNumericInput(
						numericInputWorkspaceGroupSize,
						-1,
						"Set Workspace Group Size",
						"Type any positive number. Enter applies. Esc cancels.",
						strconv.Itoa(m.workspaceEdit.GroupSize),
					)
				}
			}
		}
		if inItems {
			m.adjustWorkspaceItem(1)
		} else {
			m.adjustWorkspaceField(1)
		}
	case " ":
		if inItems {
			m.adjustWorkspaceItem(1)
		} else {
			m.adjustWorkspaceField(1)
		}
	default:
		return m, nil
	}

	m.markDirty()
	return m, nil
}

func (m Model) updateConfirmKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.pending == nil {
		m.mode = modeMain
		return m, nil
	}

	switch answerKey(msg) {
	case "ctrl+c", "q":
		m.quitAfterRevert = true
		return m, m.revertCmd(*m.pending, "quit")
	case "y", "enter":
		var toastCmd tea.Cmd

		p := m.pending.profile
		confirmErr := m.confirmPending(*m.pending)
		if confirmErr != nil && m.pending.remote {
			if errors.Is(confirmErr, ipc.ErrTransactionUnavailable) {
				m.mode = modeMain
				m.pending = nil
				m.disarmPendingRemote()
				m.markClean()
				m.draftProfileName = ""
				m.matchedProfileName = ""
				m.draftExec = ""
				m.setStatusOK("Configuration reverted: confirmation timeout")
				return m, m.refreshCmd(false)
			}
			m.setStatusErr(fmt.Sprintf("Could not confirm configuration: %v", confirmErr))
			return m, nil
		}
		if target := strings.TrimSpace(p.Name); target != "" && target != "draft" {
			m.draftProfileName = target
			m.matchedProfileName = target
		}

		if confirmErr != nil {
			toastCmd = m.notifyUser(fmt.Sprintf("Post-apply failed for %q: %v", p.Name, confirmErr), true)
		}

		m.mode = modeMain
		m.pending = nil
		m.disarmPendingRevert()
		m.disarmPendingRemote()
		m.markClean()
		m.setStatusOK("Configuration kept")
		if m.quitAfterApply {
			m.quitAfterApply = false
			return m, tea.Quit
		}
		return m, tea.Batch(m.refreshCmd(false), toastCmd)
	case "n", "esc":
		return m, m.revertCmd(*m.pending, "user request")
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
	case modeKeybindings:
		return m.renderModalScreen(m.renderKeybindings())
	default:
		return m.renderMain()
	}
}

func (m Model) renderMain() string {
	tabs := m.renderTabs()
	toast := m.renderToast()
	toastHeight := 0
	if toast != "" {
		toastHeight = lipgloss.Height(toast) + 1
	}

	footerText := m.renderFooterBar()
	bodyHeight := max(3, m.mainBodyHeight(tabs, "", footerText)-toastHeight)

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
	parts := make([]string, 0, len(labels)*2+1)
	lineStyle := withFG(lipgloss.NewStyle(), m.styles.palette.paneBorder)
	parts = append(parts, lineStyle.Render("─"))
	// Only the selected tab carries the accent color, numeral included, so the
	// current tab reads at a glance.
	for idx, label := range labels {
		number := fmt.Sprintf("%d", idx+1)
		if int(m.tab) == idx {
			parts = append(parts, m.styles.tabActive.Render(fmt.Sprintf(" %s %s ", number, label)))
		} else {
			numStyle := withFG(lipgloss.NewStyle().Bold(true), m.styles.palette.tabInactiveFg)
			parts = append(parts, m.styles.tabInactive.Render(fmt.Sprintf(" %s %s ", numStyle.Render(number), label)))
		}
		parts = append(parts, lineStyle.Render("─"))
	}

	left := lipgloss.JoinHorizontal(lipgloss.Center, parts...)
	status := m.renderTopStatus()
	width := m.footerContentWidth()
	availableStatus := max(1, width-lipgloss.Width(left)-2)
	if lipgloss.Width(status) > availableStatus {
		status = m.renderCompactTopStatus()
	}
	if lipgloss.Width(status) > availableStatus {
		status = ansi.Truncate(status, availableStatus, "")
	}
	statusStart := width - lipgloss.Width(status) - 1
	gap := max(1, statusStart-lipgloss.Width(left))
	return left + lineStyle.Render(strings.Repeat("─", gap)) + status + lineStyle.Render("─")
}

func (m Model) renderLayoutView(height int) string {
	if m.useCompactLayout(height) {
		canvasHeight, inspectorHeight := m.compactLayoutHeights(height)
		width := m.terminalWidth() - m.styles.app.GetHorizontalFrameSize()
		canvas := m.renderCanvasPane(width, canvasHeight)
		inspector := m.renderInspectorColumn(width, inspectorHeight, true)
		return lipgloss.JoinVertical(lipgloss.Left, canvas, inspector)
	}

	canvasWidth, inspectorWidth := m.layoutPaneWidths()
	canvas := m.renderCanvasPane(canvasWidth, height)
	inspector := m.renderInspectorColumn(inspectorWidth, height, false)
	return lipgloss.JoinHorizontal(lipgloss.Top, canvas, strings.Repeat(" ", paneGapWidth), inspector)
}

func (m Model) renderCanvasPane(width int, height int) string {
	tone := paneToneIdle
	if m.layoutFocus == layoutFocusCanvas && m.tab == tabLayout {
		tone = paneToneFocused
	}
	panel := m.paneStyle(tone)
	innerWidth := max(1, width-panel.GetHorizontalFrameSize())
	innerHeight := max(1, height-panel.GetVerticalFrameSize())
	body := fitBlock(m.renderCanvas(innerWidth, innerHeight), innerWidth, innerHeight)
	return m.renderTitledPaneWithMeta(tone, "Monitor Layout", m.canvasPaneMeta(), body, width)
}

func (m Model) canvasPaneMeta() string {
	switch m.lidState {
	case lid.Open:
		return "Lid: open"
	case lid.Closed:
		return "Lid: closed"
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
	workspaces := workspacePlanByConnector(profile.ResolveWorkspaceRules(m.currentProfile("draft"), nil))

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
		issue, _ := m.canvasOutputIssue(output)
		colors := m.canvasCardStyle(output, selected)
		paintCard(grid, rect, selected, colors, func(maxLines, maxWidth int) []cardLine {
			return m.monitorCardLines(output, workspaces[output.Name], monitorCardLayout,
				maxLines, maxWidth, colors, issue, m.styles.palette.warning)
		})
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
	paintCanvasSegments(grid, 0, 1, m.hiddenOutputSegments(m.editOutputs, m.selectedOutput, canvasW-2))
	return renderCanvasCells(grid)
}

// inspectorLayout is the single source of truth shared by renderInspectorPane
// and inspectorFieldAt so their row math cannot drift.
type inspectorLayout struct {
	lines     []string
	fieldRows map[int]int // field index → index into lines
}

func (m Model) buildInspectorLayout(output editableOutput, innerWidth int, compact bool) inspectorLayout {
	lines := make([]string, 0, len(layoutFields)+2)

	labelWidth := 0
	shortLabels := compact || innerWidth < 34 || (m.inspectorTab == inspectorTabColor && innerWidth < 50)
	for _, field := range inspectorFieldsForTab(m.inspectorTab) {
		label := layoutFields[field]
		if shortLabels {
			label = layoutFieldShortLabel(field)
		}
		labelWidth = max(labelWidth, lipgloss.Width(label))
	}

	fieldRows := make(map[int]int, len(layoutFields))
	for _, idx := range inspectorFieldsForTab(m.inspectorTab) {
		if idx == advancedFieldStart {
			lines = append(lines, "")
		}
		labelText := layoutFields[idx]
		if shortLabels {
			labelText = layoutFieldShortLabel(idx)
		}
		valueText := fieldOptionLabel(idx, m.layoutFieldValue(output, idx))
		issue, hasIssue := m.layoutFieldIssue(output, idx)
		valueStyle := m.styles.value
		if hasIssue {
			valueStyle = m.styles.warning
		}
		if m.layoutFocus == layoutFocusInspector && idx == m.inspectorField && m.tab == tabLayout {
			valueStyle = m.styles.focused
			if hasIssue {
				valueStyle = withFG(valueStyle, m.styles.palette.warning)
			}
		}
		value := valueStyle.Render(valueText)
		if hasIssue {
			value = lipgloss.JoinHorizontal(lipgloss.Left, value, " ", m.styles.warning.Render("⚠ "+issue))
		}
		label := m.styles.label.Render(fmt.Sprintf("%-*s", labelWidth, labelText))
		fieldRows[idx] = len(lines)
		lines = append(lines, fmt.Sprintf("%s %s", label, value))
	}

	return inspectorLayout{lines: lines, fieldRows: fieldRows}
}

func inspectorFieldsForTab(tab inspectorTab) []int {
	if tab == inspectorTabColor {
		return []int{3, 4, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}
	}
	return []int{0, 1, 2, 5, 6, 7, 8, 9}
}

func (m *Model) normalizeInspectorField() {
	fields := inspectorFieldsForTab(m.inspectorTab)
	for _, field := range fields {
		if m.inspectorField == field {
			return
		}
	}
	if len(fields) > 0 {
		m.inspectorField = fields[0]
	}
}

func (m *Model) moveInspectorField(delta int) {
	fields := inspectorFieldsForTab(m.inspectorTab)
	if len(fields) == 0 {
		return
	}
	position := 0
	for idx, field := range fields {
		if field == m.inspectorField {
			position = idx
			break
		}
	}
	m.inspectorField = fields[clampIndex(position+delta, len(fields))]
}

// cycleLayoutPane walks the layout tab's three panes in the order they appear:
// the canvas, then Display, then Color. Tab moving between panes and the
// bracket keys always cycling monitors keeps each key to one meaning.
func (m *Model) cycleLayoutPane(delta int) {
	position := 0
	if m.layoutFocus == layoutFocusInspector {
		position = 1 + int(m.inspectorTab)
	}

	position = wrapIndex(position+delta, 3)
	if position == 0 {
		m.layoutFocus = layoutFocusCanvas
		return
	}
	m.layoutFocus = layoutFocusInspector
	m.inspectorTab = inspectorTab(position - 1)
	m.normalizeInspectorField()
}

func (m *Model) cycleInspectorTab(delta int) {
	m.inspectorTab = inspectorTab(wrapIndex(int(m.inspectorTab)+delta, 2))
	m.normalizeInspectorField()
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
	tone := paneToneIdle
	if m.layoutFocus == layoutFocusInspector && m.tab == tabLayout {
		tone = paneToneFocused
	}
	panel := m.paneStyle(tone)
	innerWidth := max(1, width-panel.GetHorizontalFrameSize())
	innerHeight := max(1, height-panel.GetVerticalFrameSize())

	if len(m.editOutputs) == 0 {
		body := fitBlock("(none)", innerWidth, innerHeight)
		return m.renderInspectorTabbedPane(tone, body, width)
	}

	layout := m.buildInspectorLayout(m.editOutputs[m.selectedOutput], innerWidth, compact)
	lines := layout.lines

	if m.layoutFocus == layoutFocusInspector && m.tab == tabLayout {
		if row, ok := layout.fieldRows[m.inspectorField]; ok {
			offset := inspectorScrollOffset(len(lines), row, innerHeight)
			lines = lines[offset:]
		}
	}

	body := fitBlock(strings.Join(lines, "\n"), innerWidth, innerHeight)
	return m.renderInspectorTabbedPane(tone, body, width)
}

func (m Model) renderInspectorColumn(width, height int, compact bool) string {
	preferencesHeight, infoHeight := m.inspectorPaneHeights(height)
	info := m.renderInfoPane(width, infoHeight)
	preferences := m.renderInspectorPane(width, preferencesHeight, compact)
	return lipgloss.JoinVertical(lipgloss.Left, info, preferences)
}

func (m Model) inspectorPaneHeights(height int) (int, int) {
	if height <= 8 {
		preferences := max(3, (height+1)/2)
		return preferences, max(2, height-preferences)
	}
	info := clampInt(11, 5, height/2)
	return height - info, info
}

func (m Model) renderInfoPane(width, height int) string {
	panel := m.paneStyle(paneToneStatic)
	innerWidth := max(1, width-panel.GetHorizontalFrameSize())
	innerHeight := max(1, height-panel.GetVerticalFrameSize())
	body := "(none)"
	if len(m.editOutputs) > 0 {
		body = strings.Join(m.inspectorDetailLines(m.editOutputs[m.selectedOutput]), "\n")
	}
	body = fitBlock(body, innerWidth, innerHeight)
	return m.renderTitledPane(paneToneStatic, "Info", body, width)
}

func (m Model) renderInspectorTabbedPane(tone paneTone, body string, width int) string {
	labels := []string{"Display", "Color"}
	parts := make([]string, 0, len(labels))
	plainWidth := 0
	for idx, label := range labels {
		text := label
		plainWidth += lipgloss.Width(text)
		if idx < len(labels)-1 {
			plainWidth += 3
		}
		style := m.styles.subtle
		if int(m.inspectorTab) == idx {
			style = withFG(lipgloss.NewStyle().Bold(true), m.styles.palette.paneActiveBorder)
		}
		parts = append(parts, style.Render(text))
	}
	title := strings.Join(parts, m.styles.subtle.Render(" - "))
	return m.renderPaneWithTitle(tone, title, plainWidth, "", body, width)
}

// paneTone is how loud a pane's chrome should be. Only the pane your keys act
// on takes the accent; a pane that never takes focus still reads clearly; a
// pane you could switch to but have not stays muted.
type paneTone int

const (
	paneToneIdle paneTone = iota
	paneToneFocused
	paneToneStatic
)

func (m Model) paneStyle(tone paneTone) lipgloss.Style {
	switch tone {
	case paneToneFocused:
		return m.styles.activePane
	case paneToneStatic:
		return m.styles.staticPane
	default:
		return m.styles.inactivePane
	}
}

func (m Model) paneBorderColor(tone paneTone) string {
	switch tone {
	case paneToneFocused:
		return m.styles.palette.paneActiveBorder
	case paneToneStatic:
		return m.styles.palette.paneStaticBorder
	default:
		return m.styles.palette.paneBorder
	}
}

// renderTitledPane places the pane label inside its top border, leaving every
// interior row available to the editor. This mirrors the compact pane chrome
// used by terminal applications such as Lazygit.
func (m Model) renderTitledPane(tone paneTone, title, body string, width int) string {
	return m.renderTitledPaneWithMeta(tone, title, "", body, width)
}

func (m Model) renderTitledPaneWithMeta(tone paneTone, title, meta, body string, width int) string {
	styledTitle := withFG(lipgloss.NewStyle().Bold(true), m.paneBorderColor(tone)).Render(title)
	return m.renderPaneWithTitle(tone, styledTitle, lipgloss.Width(title), meta, body, width)
}

func (m Model) renderPaneWithTitle(tone paneTone, styledTitle string, titleWidth int, meta, body string, width int) string {
	panel := m.paneStyle(tone)
	rendered := panel.Width(styleRenderWidth(width, panel)).Render(body)
	lines := strings.Split(rendered, "\n")
	if len(lines) == 0 {
		return rendered
	}

	labelWidth := titleWidth + 2
	topWidth := lipgloss.Width(lines[0])
	start := 2
	if topWidth <= start+labelWidth+1 {
		return rendered
	}

	styledLabel := " " + styledTitle + " "
	lines[0] = ansi.Cut(lines[0], 0, start) + styledLabel + ansi.Cut(lines[0], start+labelWidth, topWidth)

	if meta != "" && len(lines) > 1 {
		bottom := len(lines) - 1
		bottomWidth := lipgloss.Width(lines[bottom])
		metaLabel := " " + meta + " "
		metaWidth := lipgloss.Width(metaLabel)
		metaStart := bottomWidth - metaWidth - 1
		if metaStart > 1 {
			styledMeta := withFG(lipgloss.NewStyle(), m.paneBorderColor(tone)).Render(metaLabel)
			lines[bottom] = ansi.Cut(lines[bottom], 0, metaStart) + styledMeta + ansi.Cut(lines[bottom], metaStart+metaWidth, bottomWidth)
		}
	}
	return strings.Join(lines, "\n")
}

func scrollLinesToFit(lines []string, selectedLine, height int) []string {
	offset := inspectorScrollOffset(len(lines), selectedLine, height)
	return lines[offset:]
}

func (m Model) renderProfilesView(height int) string {
	summaries := m.profileMatchSummaries()

	if m.terminalWidth() < 96 {
		// Compact: stack vertically, list gets enough for profiles, details gets the rest.
		width := m.terminalWidth() - m.styles.app.GetHorizontalFrameSize()
		listHeight := clampInt(len(m.profiles)+profileListHeaderRows+2, profileListHeaderRows+4, height/3)
		left := m.renderProfileListPane(summaries, width, listHeight)
		right := m.renderProfileDetailPanes(summaries, width, max(3, height-listHeight))
		return lipgloss.JoinVertical(lipgloss.Left, left, right)
	}

	listWidth, detailWidth := m.sidePaneWidths(35)
	left := m.renderProfileListPane(summaries, listWidth, height)
	right := m.renderProfileDetailPanes(summaries, detailWidth, height)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, strings.Repeat(" ", paneGapWidth), right)
}

func (m Model) renderProfileListPane(summaries []profileMatchSummary, width, height int) string {
	style := m.paneStyle(paneToneFocused)
	innerWidth := max(1, width-style.GetHorizontalFrameSize())
	innerHeight := max(1, height-style.GetVerticalFrameSize())
	cols := m.profileListColumns(innerWidth)
	rows := m.profileListRows(summaries, cols)
	lines := append([]string{m.profileAutomaticRow(innerWidth), "", m.profileListHeader(cols), ""}, rows[min(m.profileListScroll(innerHeight), len(rows)-1):]...)
	body := fitBlock(strings.Join(lines, "\n"), innerWidth, innerHeight)
	return m.renderTitledPane(paneToneFocused, "Saved Profiles", body, width)
}

// profileCanvasMinHeight is the smallest pane that still shows a readable
// monitor card; below it the details pane keeps the text only.
const profileCanvasMinHeight = 8

func (m Model) renderProfileDetailPanes(summaries []profileMatchSummary, width, height int) string {
	style := m.paneStyle(paneToneStatic)
	innerWidth := max(1, width-style.GetHorizontalFrameSize())

	if len(m.profiles) == 0 {
		body := fitBlock(m.styles.subtle.Render("(no saved profiles)"), innerWidth, max(1, height-style.GetVerticalFrameSize()))
		return m.renderTitledPane(paneToneStatic, "Profile Details", body, width)
	}

	selected := m.profiles[m.selectedProfile]
	infoLines := m.renderDetailRows(m.profileDetailRows(selected, summaries[m.selectedProfile], innerWidth))

	infoHeight := clampInt(len(infoLines)+style.GetVerticalFrameSize(), 3, height)
	canvasHeight := height - infoHeight
	if canvasHeight < profileCanvasMinHeight {
		body := fitBlock(strings.Join(infoLines, "\n"), innerWidth, max(1, height-style.GetVerticalFrameSize()))
		return m.renderTitledPane(paneToneStatic, "Profile Details", body, width)
	}

	info := fitBlock(strings.Join(infoLines, "\n"), innerWidth, max(1, infoHeight-style.GetVerticalFrameSize()))
	canvasInner := max(1, canvasHeight-style.GetVerticalFrameSize())
	canvas := fitBlock(m.renderProfileCanvas(selected, innerWidth, canvasInner), innerWidth, canvasInner)
	return lipgloss.JoinVertical(lipgloss.Left,
		m.renderTitledPane(paneToneStatic, "Profile Details", info, width),
		m.renderTitledPane(paneToneStatic, "Monitor Layout", canvas, width),
	)
}

func (m Model) renderWorkspaceView(height int) string {
	leftStyle := m.paneStyle(paneToneFocused)

	if m.terminalWidth() < 96 {
		// Compact: stack vertically, settings get enough room, preview gets the rest
		width := m.terminalWidth() - m.styles.app.GetHorizontalFrameSize()
		settingsHeight := clampInt(m.workspaceSettingsLineCount()+2, 6, (height*2)/3)
		innerW := max(1, width-leftStyle.GetHorizontalFrameSize())
		innerH := max(1, settingsHeight-leftStyle.GetVerticalFrameSize())
		settings := m.workspaceSettingsVisibleLines(innerH)
		leftBody := fitBlock(strings.Join(settings, "\n"), innerW, innerH)
		left := m.renderTitledPane(paneToneFocused, "Workspace Planner", leftBody, width)
		right := m.renderWorkspacePreviewPanes(width, max(3, height-settingsHeight))
		return lipgloss.JoinVertical(lipgloss.Left, left, right)
	}

	leftWidth, rightWidth := m.sidePaneWidths(35)
	innerH := max(1, height-leftStyle.GetVerticalFrameSize())
	settings := m.workspaceSettingsVisibleLines(innerH)
	leftBody := fitBlock(strings.Join(settings, "\n"), max(1, leftWidth-leftStyle.GetHorizontalFrameSize()), innerH)
	left := m.renderTitledPane(paneToneFocused, "Workspace Planner", leftBody, leftWidth)
	right := m.renderWorkspacePreviewPanes(rightWidth, height)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, strings.Repeat(" ", paneGapWidth), right)
}

// workspaceSettingsLines renders the planner form. Its row order is the one
// workspaceSettingsRect relies on for mouse hits, so keep them in step.
func (m Model) workspaceSettingsLines() []string {
	lines := make([]string, 0, m.workspaceSettingsLineCount())
	for line := 0; line < m.workspaceSettingsLineCount(); line++ {
		lines = append(lines, m.workspaceSettingsLine(line))
	}
	return lines
}

func (m Model) workspaceSettingsLine(line int) string {
	if line >= 0 && line < len(workspaceFields) {
		value := m.workspaceFieldValue(line)
		prefix := "  "
		if line == m.workspaceEdit.SelectedField {
			prefix = m.styles.statusOK.Render("> ")
			value = m.styles.focused.Render(value)
		} else {
			value = m.styles.value.Render(value)
		}
		return prefix + m.styles.label.Render(fmt.Sprintf("%-14s", workspaceFields[line])) + " " + value
	}
	if line == len(workspaceFields) {
		return ""
	}
	if line == len(workspaceFields)+1 {
		if m.workspaceEdit.Strategy == profile.WorkspaceStrategyManual {
			return m.styles.label.Render("Workspace → display") + "  " + m.styles.subtle.Render("←/→ assigns")
		}
		return m.styles.label.Render("Monitor order") + "  " + m.styles.subtle.Render("←/→ reorders")
	}

	item := line - len(workspaceFields) - 2
	if item < 0 || item >= m.workspaceListItemCount() {
		return m.styles.subtle.Render("  (none)")
	}
	if m.workspaceEdit.Strategy == profile.WorkspaceStrategyManual {
		rule := m.workspaceEdit.Rules[item]
		label := m.manualWorkspaceRuleOutputLabel(rule)
		prefix := "  "
		if len(workspaceFields)+item == m.workspaceEdit.SelectedField {
			prefix = m.styles.statusOK.Render("> ")
			label = m.styles.focused.Render(label)
		} else {
			label = m.styles.value.Render(label)
		}
		workspace := m.styles.subtle.Render(fmt.Sprintf("%-14s", "Workspace "+blankFallback(rule.Workspace, "?")))
		return prefix + workspace + " " + label
	}

	label := m.outputLabelForKey(m.workspaceEdit.MonitorOrder[item])
	prefix := "  "
	if len(workspaceFields)+item == m.workspaceEdit.SelectedField {
		prefix = m.styles.statusOK.Render("> ")
		label = m.styles.focused.Render(label)
	} else {
		label = m.styles.value.Render(label)
	}
	return fmt.Sprintf("%s%s %s", prefix, m.styles.subtle.Render(fmt.Sprintf("%d.", item+1)), label)
}

func (m Model) workspaceSelectedLine() int {
	if m.workspaceEdit.SelectedField < len(workspaceFields) {
		return m.workspaceEdit.SelectedField
	}
	return len(workspaceFields) + 2 + m.workspaceEdit.SelectedField - len(workspaceFields)
}

func (m Model) workspaceSettingsScrollOffset(height int) int {
	return inspectorScrollOffset(m.workspaceSettingsLineCount(), m.workspaceSelectedLine(), height)
}

func (m Model) workspaceSettingsVisibleLines(height int) []string {
	offset := m.workspaceSettingsScrollOffset(height)
	end := min(m.workspaceSettingsLineCount(), offset+max(1, height))
	lines := make([]string, 0, max(0, end-offset))
	for line := offset; line < end; line++ {
		lines = append(lines, m.workspaceSettingsLine(line))
	}
	return lines
}

// renderWorkspacePreviewPanes shows the resolved plan as an aligned list and,
// when there is room, the same plan laid over the monitor arrangement.
func (m Model) renderWorkspacePreviewPanes(width, height int) string {
	style := m.paneStyle(paneToneStatic)
	innerWidth := max(1, width-style.GetHorizontalFrameSize())

	settings := m.workspaceEdit.settings()
	disabled := !settings.Enabled
	if disabled {
		settings.Enabled = true
	}
	outputs := m.currentProfileOutputs()
	rules := settings.Rules
	if settings.Strategy != profile.WorkspaceStrategyManual && settings.Strategy != "" {
		rules = m.workspacePlan(outputs, settings)
	}

	planLines := make([]string, 0, len(outputs)+2)
	if disabled {
		planLines = append(planLines, m.styles.warning.Render("(workspace rules disabled; preview only)"), "")
	}
	if rows := m.workspacePlanRows(rules, outputs, innerWidth); len(rows) > 0 {
		planLines = append(planLines, rows...)
	} else {
		planLines = append(planLines, m.styles.subtle.Render("(no workspace rules configured)"))
	}

	planHeight := clampInt(len(planLines)+style.GetVerticalFrameSize(), 3, height)
	canvasHeight := height - planHeight
	if canvasHeight < profileCanvasMinHeight {
		body := fitBlock(strings.Join(planLines, "\n"), innerWidth, max(1, height-style.GetVerticalFrameSize()))
		return m.renderTitledPane(paneToneStatic, "Workspace Plan", body, width)
	}

	plan := fitBlock(strings.Join(planLines, "\n"), innerWidth, max(1, planHeight-style.GetVerticalFrameSize()))
	canvasInner := max(1, canvasHeight-style.GetVerticalFrameSize())
	canvas := fitBlock(m.renderWorkspaceCanvas(workspacePlanByConnector(rules), innerWidth, canvasInner), innerWidth, canvasInner)
	return lipgloss.JoinVertical(lipgloss.Left,
		m.renderTitledPane(paneToneStatic, "Workspace Plan", plan, width),
		m.renderTitledPane(paneToneStatic, "Monitor Layout", canvas, width),
	)
}

func (m Model) renderSavePrompt() string {
	if m.saveDialog == nil {
		return m.renderModalFrame("Save Profile", nil)
	}
	title := "Save Profile"
	body := make([]string, 0, 12)
	if m.saveDialog.Purpose == saveDialogQuit {
		title = "Save Before Quitting"
		body = append(body,
			m.styles.warning.Render("You have unsaved monitor changes."),
			m.styles.subtle.Render("Save and apply them before quitting."),
			"",
		)
	}
	inputBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(m.styles.palette.paneActiveBorder)).
		Padding(0, 1).
		Render(m.saveDialog.Input.View())
	body = append(body,
		m.styles.label.Render("Name"),
		inputBox,
		"",
		m.saveDialog.List.View(),
		"",
		m.styles.label.Render("Action"),
		m.renderSaveActionButtons(),
		"",
	)
	if status := m.renderErrorStatus(); status != "" {
		body = append(body, status, "")
	}
	body = append(body, m.styles.help.MaxWidth(max(20, m.modalMaxWidth()-6)).Render("Type to filter names. Up/Down selects an existing profile. Left/Right or Tab switches action. Enter confirms. Esc cancels."))
	return m.renderModalFrame(title, body)
}

func (m Model) renderSaveConfirm() string {
	consequence := "The existing profile will be replaced with the current draft."
	if m.saveDialog != nil && m.saveDialog.Action == saveActionApply {
		consequence = "The existing profile will be replaced and then applied to the live layout."
	} else if m.saveDialog != nil && m.saveDialog.Action == saveActionSaveQuit {
		consequence = "The existing profile will be replaced, applied, then hyprmoncfg will quit."
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
		m.styles.help.MaxWidth(max(20, m.modalMaxWidth()-6)).Render(m.confirmApplyHelp()),
	}
	return m.renderModalFrame("Confirm Apply", body)
}

// answerKey normalizes a yes/no keypress. A prompt like "Press y" is answered
// with Shift held often enough that a case-sensitive match reads as the dialog
// being broken.
func answerKey(msg tea.KeyMsg) string {
	key := msg.String()
	if len([]rune(key)) == 1 {
		return strings.ToLower(key)
	}
	return key
}

func (m Model) confirmApplyHelp() string {
	if m.quitAfterApply {
		return "Enter or y keeps the change and quits. Esc or n reverts it."
	}
	return "Enter or y keeps the change. Esc or n reverts it."
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

func (m Model) mainBodyHeight(tabs string, status string, help string) int {
	reserved := lipgloss.Height(tabs) + lipgloss.Height(help)
	return max(3, m.terminalHeight()-reserved)
}

func (m Model) useCompactLayout(bodyHeight int) bool {
	return bodyHeight < 14 || m.terminalWidth() < 96
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

// Lipgloss Width includes padding, but adds borders and margins outside it.
// Convert the total pane width allocated by the layout into the value Width
// expects while leaving the pane's internal padding intact.
func styleRenderWidth(total int, style lipgloss.Style) int {
	return max(1, total-style.GetHorizontalMargins()-style.GetHorizontalBorderSize())
}

func (m *Model) loadLiveState() {
	prevOutputs := m.editOutputs
	selectedKey := ""
	if m.selectedOutput >= 0 && m.selectedOutput < len(prevOutputs) {
		selectedKey = prevOutputs[m.selectedOutput].Key
	}
	draft, sourceName, suggestedName := profile.EditorProfileFromState(m.profiles, m.monitors, m.workspaceRules)
	if len(prevOutputs) > 0 && !m.resetRequested {
		previous := profile.Profile{Outputs: make([]profile.OutputConfig, 0, len(prevOutputs))}
		for _, output := range prevOutputs {
			previous.Outputs = append(previous.Outputs, output.profileOutput())
		}
		profile.PreserveUnreportedSettings(&draft, previous)
	}

	m.editOutputs = make([]editableOutput, 0, len(draft.Outputs))
	for _, saved := range draft.Outputs {
		live, ok := m.findLiveMonitor(saved)
		m.editOutputs = append(m.editOutputs, editableOutputFromProfile(saved, live, ok))
	}
	m.recoverMirroredIdentity()
	m.workspaceEdit = workspaceEditorFromSettings(draft.Workspaces, m.editOutputs)
	m.matchedProfileName = ""
	m.activeProfileName = ""
	if sourceName != "" {
		m.draftProfileName = sourceName
		m.matchedProfileName = sourceName
		m.activeProfileName = sourceName
		m.draftExec = draft.Exec
	} else {
		m.draftProfileName = ""
		m.draftExec = ""
		m.matchedProfileName = suggestedName
	}
	m.resetRequested = false
	if idx := focusedOutputIndex(m.editOutputs); idx >= 0 {
		m.selectedOutput = idx
	} else if selectedKey != "" {
		m.selectedOutput = outputIndexByKey(m.editOutputs, selectedKey)
	}
	m.selectedOutput = clampIndex(m.selectedOutput, len(m.editOutputs))
	m.inspectorField = clampIndex(m.inspectorField, len(layoutFields))
	// matchedProfileName is already "the active profile, or the best scoring
	// one", which is the profile worth landing on in the profiles tab.
	if idx := m.profileIndexByName(m.matchedProfileName); idx >= 0 {
		m.selectedProfile = idx
	}
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
	m.matchedProfileName = p.Name
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
	workspaceItems := m.workspaceItemCount()
	m.workspaceEdit.SelectedField = clampIndex(m.workspaceEdit.SelectedField, workspaceItems)
	if m.workspaceEdit.SelectedField >= len(workspaceFields) {
		m.workspaceEdit.SelectedOrder = m.workspaceEdit.SelectedField - len(workspaceFields)
	} else {
		m.workspaceEdit.SelectedOrder = clampIndex(m.workspaceEdit.SelectedOrder, len(m.workspaceEdit.MonitorOrder))
	}
}

func (m Model) profileExists(name string) bool {
	_, ok := m.profileByName(name)
	return ok
}

func (m Model) profileIndexByName(name string) int {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return -1
	}
	for idx, prof := range m.profiles {
		if strings.TrimSpace(strings.ToLower(prof.Name)) == name {
			return idx
		}
	}
	return -1
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
	return outputNameForKeyIn(m.editOutputs, key)
}

func outputNameForKeyIn(outputs []editableOutput, key string) string {
	for _, output := range outputs {
		if output.Key == key {
			return output.Name
		}
	}
	return key
}

// moveSelectedOutputToOrigin puts the selected monitor where Hyprland's own
// `position = auto` would put a first monitor, so the layout can be compared
// against what Hyprland does on its own.
func (m *Model) moveSelectedOutputToOrigin() {
	if len(m.editOutputs) == 0 || !m.canMoveSelectedOutput() {
		return
	}
	if m.editOutputs[m.selectedOutput].X == 0 && m.editOutputs[m.selectedOutput].Y == 0 {
		return
	}

	m.editOutputs[m.selectedOutput].X = 0
	m.editOutputs[m.selectedOutput].Y = 0
	m.layoutChanged()
}

// canMoveSelectedOutput reports whether moving the selection means anything. A
// mirroring display shows its source's image wherever Hyprland decides to put
// it, so accepting a move would only leave the draft permanently different from
// what any apply can produce.
func (m *Model) canMoveSelectedOutput() bool {
	output := m.editOutputs[m.selectedOutput]
	if output.MirrorOf == "" {
		return true
	}
	m.setStatusErr(fmt.Sprintf("%s mirrors %s and follows it; move %s instead",
		output.Name, m.outputNameForKey(output.MirrorOf), m.outputNameForKey(output.MirrorOf)))
	return false
}

func (m *Model) moveSelectedOutput(dx, dy int) {
	if len(m.editOutputs) == 0 || !m.canMoveSelectedOutput() {
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
	shared := profile.AnalyzeSnap(m.currentProfileOutputs(), m.selectedOutput, threshold)
	convertMarks := func(marks []profile.SnapMark) []snapMark {
		converted := make([]snapMark, 0, len(marks))
		for _, mark := range marks {
			converted = append(converted, snapMark{OutputIndex: mark.OutputIndex, Edge: snapEdge(mark.Edge)})
		}
		return converted
	}
	return snapAnalysis{
		x: snapAxisCandidate{pos: shared.X.Pos, dist: shared.X.Dist, marks: convertMarks(shared.X.Marks)},
		y: snapAxisCandidate{pos: shared.Y.Pos, dist: shared.Y.Dist, marks: convertMarks(shared.Y.Marks)},
	}
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

func (m *Model) snapSelectedOutput(direction snapDirection) tea.Cmd {
	if len(m.editOutputs) == 0 || m.selectedOutput < 0 || m.selectedOutput >= len(m.editOutputs) {
		return nil
	}

	selected := &m.editOutputs[m.selectedOutput]
	if !selected.Enabled || selected.MirrorOf != "" {
		m.setStatusErr("Selected monitor must be enabled and not mirrored to snap")
		return nil
	}

	anchorIndex := m.nearestSnapOutput()
	if anchorIndex < 0 {
		m.setStatusErr("No other enabled monitor available for snapping")
		return nil
	}

	anchor := m.editOutputs[anchorIndex]
	selectedW, selectedH := selected.logicalSize()
	anchorW, anchorH := anchor.logicalSize()
	marks := make([]snapMark, 0, 2)

	switch direction {
	case snapDirectionLeft:
		selected.X = anchor.X - selectedW
		selected.Y = anchor.Y + (anchorH-selectedH)/2
		marks = append(marks,
			snapMark{OutputIndex: m.selectedOutput, Edge: snapEdgeRight},
			snapMark{OutputIndex: anchorIndex, Edge: snapEdgeLeft},
		)
	case snapDirectionRight:
		selected.X = anchor.X + anchorW
		selected.Y = anchor.Y + (anchorH-selectedH)/2
		marks = append(marks,
			snapMark{OutputIndex: m.selectedOutput, Edge: snapEdgeLeft},
			snapMark{OutputIndex: anchorIndex, Edge: snapEdgeRight},
		)
	case snapDirectionUp:
		selected.X = anchor.X + (anchorW-selectedW)/2
		selected.Y = anchor.Y - selectedH
		marks = append(marks,
			snapMark{OutputIndex: m.selectedOutput, Edge: snapEdgeBottom},
			snapMark{OutputIndex: anchorIndex, Edge: snapEdgeTop},
		)
	case snapDirectionDown:
		selected.X = anchor.X + (anchorW-selectedW)/2
		selected.Y = anchor.Y + anchorH
		marks = append(marks,
			snapMark{OutputIndex: m.selectedOutput, Edge: snapEdgeTop},
			snapMark{OutputIndex: anchorIndex, Edge: snapEdgeBottom},
		)
	default:
		return nil
	}

	m.layoutChanged()
	m.setStatusOK(fmt.Sprintf("Snapped %s %s %s", selected.Name, direction.relation(), anchor.Name))
	return m.showSnapHint(&snapHintState{Marks: marks})
}

func (m Model) nearestSnapOutput() int {
	if len(m.editOutputs) == 0 || m.selectedOutput < 0 || m.selectedOutput >= len(m.editOutputs) {
		return -1
	}

	selected := m.editOutputs[m.selectedOutput]
	selectedW, selectedH := selected.logicalSize()
	selectedCenterX := int64(selected.X)*2 + int64(selectedW)
	selectedCenterY := int64(selected.Y)*2 + int64(selectedH)

	nearestIndex := -1
	var nearestDistance int64
	for index, output := range m.editOutputs {
		if index == m.selectedOutput || !output.Enabled || output.MirrorOf != "" {
			continue
		}

		width, height := output.logicalSize()
		centerX := int64(output.X)*2 + int64(width)
		centerY := int64(output.Y)*2 + int64(height)
		dx := selectedCenterX - centerX
		dy := selectedCenterY - centerY
		distance := dx*dx + dy*dy
		if nearestIndex < 0 || distance < nearestDistance {
			nearestIndex = index
			nearestDistance = distance
		}
	}
	return nearestIndex
}

func (d snapDirection) relation() string {
	switch d {
	case snapDirectionLeft:
		return "left of"
	case snapDirectionRight:
		return "right of"
	case snapDirectionUp:
		return "above"
	case snapDirectionDown:
		return "below"
	default:
		return ""
	}
}

// reflowAfterResize keeps a layout packed when an output's logical size
// changes. Scale, mode, and transform all move an output's right and bottom
// edges while its top-left corner stays put, so without this the displays
// beside it gap or overlap. Everything past the old right or bottom edge
// moves with that edge: flush neighbors stay flush, deliberate gaps keep
// their width, and a row of displays shifts together.
func (m *Model) reflowAfterResize(index, oldWidth, oldHeight int) {
	outputs := m.currentProfileOutputs()
	profile.ReflowAfterResize(outputs, index, oldWidth, oldHeight)
	for idx := range outputs {
		m.editOutputs[idx].X = outputs[idx].X
		m.editOutputs[idx].Y = outputs[idx].Y
	}
}

func (m *Model) adjustInspectorField(delta int) {
	if len(m.editOutputs) == 0 {
		return
	}

	output := &m.editOutputs[m.selectedOutput]
	oldWidth, oldHeight := output.logicalSize()
	switch m.inspectorField {
	case 0:
		output.Enabled = !output.Enabled
	case 1:
		if len(output.Modes) == 0 {
			return
		}
		output.ModeIndex = wrapIndex(output.ModeIndex+delta, len(output.Modes))
		if output.ModeUnsupported && output.ModeIndex > 0 {
			output.ModeUnsupported = false
		}
		output.applyMode(output.Modes[output.ModeIndex])
	case 2:
		output.Scale = scaling.Round(clampFloat(output.Scale+float64(delta)*0.05, scaling.MinScale, scaling.MaxScale))
	case 3:
		// Hyprland's bitdepth is a boolean in disguise: its parser only asks
		// whether the value is "10", so anything else means 10-bit off. There is
		// no 16-bit path to offer, and offering one would silently hand back 8.
		depths := []int{8, 10}
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
		output.SDRBrightness = clampFloat(sdrMultiplier(output.SDRBrightness)+float64(delta)*0.05, 0, 3.0)
	case 11:
		output.SDRSaturation = clampFloat(sdrMultiplier(output.SDRSaturation)+float64(delta)*0.05, 0, 3.0)
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
	m.reflowAfterResize(m.selectedOutput, oldWidth, oldHeight)
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
		if next == profile.WorkspaceStrategyManual && !m.workspaceEdit.ManualRulesInitialized {
			m.workspaceEdit.Rules = m.materializeManualWorkspaceRules()
			m.workspaceEdit.ManualRulesInitialized = len(m.workspaceEdit.Rules) > 0
		}
		m.workspaceEdit.Strategy = next
	case 2:
		next := adjustPositiveInt(m.workspaceEdit.MaxWorkspaces, delta)
		if m.workspaceEdit.Strategy == profile.WorkspaceStrategyManual {
			m.resizeManualWorkspaceRules(next)
		}
		m.workspaceEdit.MaxWorkspaces = next
	case 3:
		if m.workspaceEdit.Strategy != profile.WorkspaceStrategySequential {
			return
		}
		m.workspaceEdit.GroupSize = adjustPositiveInt(m.workspaceEdit.GroupSize, delta)
		m.workspaceEdit.LastSequentialGroupSize = m.workspaceEdit.GroupSize
	}
}

func (m Model) workspaceItemCount() int {
	return len(workspaceFields) + m.workspaceListItemCount()
}

func (m Model) workspaceListItemCount() int {
	if m.workspaceEdit.Strategy == profile.WorkspaceStrategyManual {
		return len(m.workspaceEdit.Rules)
	}
	return len(m.workspaceEdit.MonitorOrder)
}

func (m *Model) setWorkspaceSelection(selected int) {
	total := m.workspaceItemCount()
	if total <= 0 {
		m.workspaceEdit.SelectedField = 0
		m.workspaceEdit.SelectedOrder = 0
		return
	}
	m.workspaceEdit.SelectedField = clampInt(selected, 0, total-1)
	if m.workspaceEdit.SelectedField >= len(workspaceFields) {
		m.workspaceEdit.SelectedOrder = m.workspaceEdit.SelectedField - len(workspaceFields)
	}
}

func (m *Model) moveWorkspaceSelection(delta int, wrap bool) {
	total := m.workspaceItemCount()
	if total <= 0 {
		return
	}
	next := m.workspaceEdit.SelectedField + delta
	if wrap {
		next = wrapIndex(next, total)
	}
	m.setWorkspaceSelection(next)
}

func (m Model) workspacePageStep() int {
	inner := m.workspaceSettingsRect().inner(m.styles.activePane)
	return max(1, inner.h-2)
}

func (m *Model) adjustWorkspaceItem(delta int) {
	if m.workspaceEdit.Strategy == profile.WorkspaceStrategyManual {
		m.moveManualWorkspaceRule(delta)
		return
	}
	m.moveWorkspaceOrder(delta)
}

func (m Model) materializeManualWorkspaceRules() []profile.WorkspaceRule {
	settings := m.workspaceEdit.settings()
	settings.Enabled = true
	if settings.Strategy == profile.WorkspaceStrategyManual || settings.Strategy == "" {
		settings.Strategy = profile.WorkspaceStrategySequential
	}
	p := profile.Profile{Outputs: m.currentProfileOutputs(), Workspaces: settings}
	return normalizeManualWorkspaceDefaults(profile.ResolveWorkspaceRules(p, nil))
}

func (m Model) manualWorkspaceOutputKeys() []string {
	available := make(map[string]bool, len(m.editOutputs))
	for _, output := range m.editOutputs {
		if output.Enabled && output.MirrorOf == "" {
			available[output.Key] = true
		}
	}

	keys := make([]string, 0, len(available))
	seen := make(map[string]bool, len(available))
	for _, key := range m.workspaceEdit.MonitorOrder {
		if available[key] && !seen[key] {
			keys = append(keys, key)
			seen[key] = true
		}
	}
	for _, output := range m.editOutputs {
		if available[output.Key] && !seen[output.Key] {
			keys = append(keys, output.Key)
			seen[output.Key] = true
		}
	}
	return keys
}

func (m Model) manualWorkspaceRuleOutputLabel(rule profile.WorkspaceRule) string {
	if rule.OutputKey != "" {
		if label := m.outputLabelForKey(rule.OutputKey); label != rule.OutputKey {
			return label
		}
	}
	return blankFallback(rule.OutputName, rule.OutputKey)
}

func (m *Model) moveManualWorkspaceRule(delta int) {
	idx := m.workspaceEdit.SelectedOrder
	if idx < 0 || idx >= len(m.workspaceEdit.Rules) {
		return
	}
	keys := m.manualWorkspaceOutputKeys()
	if len(keys) == 0 {
		return
	}

	rule := &m.workspaceEdit.Rules[idx]
	current := -1
	for pos, key := range keys {
		if key == rule.OutputKey {
			current = pos
			break
		}
	}
	if current < 0 {
		current = 0
		if delta < 0 {
			current = len(keys) - 1
		}
	} else {
		current = wrapIndex(current+delta, len(keys))
	}

	rule.OutputKey = keys[current]
	rule.OutputName = outputConnector(keys[current], m.currentProfileOutputs())
	m.workspaceEdit.Rules = normalizeManualWorkspaceDefaults(m.workspaceEdit.Rules)
}

func (m *Model) resizeManualWorkspaceRules(maximum int) {
	byWorkspace := make(map[string]profile.WorkspaceRule, len(m.workspaceEdit.Rules))
	named := make([]profile.WorkspaceRule, 0, len(m.workspaceEdit.Rules))
	for _, rule := range m.workspaceEdit.Rules {
		number, err := strconv.Atoi(strings.TrimSpace(rule.Workspace))
		if err != nil || number < 1 {
			named = append(named, rule)
			continue
		}
		if number <= maximum {
			byWorkspace[strconv.Itoa(number)] = rule
		}
	}

	keys := m.manualWorkspaceOutputKeys()
	rules := make([]profile.WorkspaceRule, 0, maximum+len(named))
	for number := 1; number <= maximum; number++ {
		workspace := strconv.Itoa(number)
		rule, ok := byWorkspace[workspace]
		if !ok {
			rule.Workspace = workspace
			if len(keys) > 0 {
				rule.OutputKey = keys[0]
				rule.OutputName = outputConnector(keys[0], m.currentProfileOutputs())
			}
		}
		rules = append(rules, rule)
	}
	rules = append(rules, named...)
	m.workspaceEdit.Rules = normalizeManualWorkspaceDefaults(rules)
}

func normalizeManualWorkspaceDefaults(rules []profile.WorkspaceRule) []profile.WorkspaceRule {
	normalized := append([]profile.WorkspaceRule(nil), rules...)
	sort.SliceStable(normalized, func(i, j int) bool {
		left, leftErr := strconv.Atoi(strings.TrimSpace(normalized[i].Workspace))
		right, rightErr := strconv.Atoi(strings.TrimSpace(normalized[j].Workspace))
		if leftErr == nil && rightErr == nil {
			return left < right
		}
		if leftErr == nil {
			return true
		}
		if rightErr == nil {
			return false
		}
		return normalized[i].Workspace < normalized[j].Workspace
	})

	seen := make(map[string]bool, len(normalized))
	for idx := range normalized {
		target := normalized[idx].OutputKey
		if target == "" {
			target = normalized[idx].OutputName
		}
		normalized[idx].Default = false
		normalized[idx].Persistent = false
		if target != "" && !seen[target] {
			normalized[idx].Default = true
			normalized[idx].Persistent = true
			seen[target] = true
		}
	}
	return normalized
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

// restartDaemonCmd hands the running daemon over to the installed build. The
// package manager cannot do this: it installs as root, while the daemon is a
// user service only the session can restart.
func (m *Model) restartDaemonCmd() tea.Cmd {
	if !m.daemonNeedsRestart() {
		return nil
	}
	m.setStatusOK("Restarting the daemon...")

	return tea.Sequence(
		func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			out, err := exec.CommandContext(ctx, "systemctl", "--user", "restart", "hyprmoncfgd.service").CombinedOutput()
			if err != nil {
				detail := strings.TrimSpace(string(out))
				if detail == "" {
					detail = err.Error()
				}
				return daemonRestartMsg{err: fmt.Errorf("restart hyprmoncfgd: %s", detail)}
			}
			return daemonRestartMsg{}
		},
		m.refreshCmd(true),
	)
}

func (m Model) refreshCmd(background bool) tea.Cmd {
	client := m.client
	store := m.store
	ipcClient := m.ipc
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		daemonOK, daemonUnknown, daemonVersion, profileOverride := daemonReachable(ctx, ipcClient)
		// A failure that is not a timeout means the connection is gone, not
		// that the daemon is: reconnect before calling it stopped.
		var replacement *ipc.Client
		if ipcClient != nil && !daemonOK && !daemonUnknown {
			if replacement = redialDaemon(ctx); replacement != nil {
				daemonOK, daemonUnknown, daemonVersion, profileOverride = daemonReachable(ctx, replacement)
			}
		}

		monitors, err := client.Monitors(ctx)
		if err != nil {
			return refreshMsg{daemonOK: daemonOK, daemonUnknown: daemonUnknown, daemonVersion: daemonVersion, profileOverride: profileOverride, daemonClient: replacement, background: background, err: err}
		}
		profiles, err := store.List()
		if err != nil {
			return refreshMsg{daemonOK: daemonOK, daemonUnknown: daemonUnknown, daemonVersion: daemonVersion, profileOverride: profileOverride, daemonClient: replacement, background: background, err: err}
		}
		workspaceRules, err := client.WorkspaceRules(ctx)
		if err != nil {
			return refreshMsg{daemonOK: daemonOK, daemonUnknown: daemonUnknown, daemonVersion: daemonVersion, profileOverride: profileOverride, daemonClient: replacement, background: background, err: err}
		}
		workspaces, err := client.Workspaces(ctx)
		if err != nil {
			return refreshMsg{daemonOK: daemonOK, daemonUnknown: daemonUnknown, daemonVersion: daemonVersion, profileOverride: profileOverride, daemonClient: replacement, background: background, err: err}
		}
		lidState, err := lid.ReadState(ctx)
		if err != nil {
			lidState = lid.Unknown
		}

		return refreshMsg{
			monitors:        monitors,
			profiles:        profiles,
			workspaceRules:  workspaceRules,
			workspaces:      workspaces,
			lidState:        lidState,
			daemonOK:        daemonOK,
			daemonUnknown:   daemonUnknown,
			daemonVersion:   daemonVersion,
			profileOverride: profileOverride,
			daemonClient:    replacement,
			background:      background,
		}
	}
}

// daemonReachable answers whether the daemon is running. A daemon that is busy
// applying a profile can miss the deadline while being perfectly alive, so a
// timeout reports "unknown" and leaves the last answer standing; only a broken
// connection counts as "not running".
func daemonReachable(ctx context.Context, client *ipc.Client) (ok bool, unknown bool, version string, profileOverride string) {
	if client == nil {
		return false, false, "", ""
	}

	probeCtx, cancel := context.WithTimeout(ctx, daemonProbeTimeout)
	defer cancel()
	document, err := client.Status(probeCtx)
	if err != nil {
		return false, isTimeout(err), "", ""
	}
	return true, false, strings.TrimSpace(document.Version), strings.TrimSpace(document.Daemon.ProfileOverride)
}

// redialDaemon reconnects after the daemon was restarted, which the connection
// dialed at startup cannot survive. Without this a restart, an upgrade, or a
// crash would leave the session reporting a stopped daemon until it is
// relaunched.
func redialDaemon(ctx context.Context) *ipc.Client {
	path, err := ipc.SocketPath()
	if err != nil {
		return nil
	}
	dialCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	client, err := ipc.Dial(dialCtx, path)
	if err != nil {
		return nil
	}

	probeCtx, probeCancel := context.WithTimeout(ctx, daemonProbeTimeout)
	defer probeCancel()
	if _, err := client.Status(probeCtx); err != nil {
		_ = client.Close()
		return nil
	}
	return client
}

const daemonProbeTimeout = 3 * time.Second

func isTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func (m Model) saveCmd(p profile.Profile) tea.Cmd {
	if m.ipc != nil {
		client := m.ipc
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := client.Save(ctx, ipc.SaveParams{Profile: p}); err != nil {
				return saveMsg{err: err}
			}
			return saveMsg{name: p.Name}
		}
	}
	store := m.store
	return func() tea.Msg {
		if err := profileio.SaveWithSidecars(store, p); err != nil {
			return saveMsg{err: err}
		}
		return saveMsg{name: p.Name}
	}
}

func (m Model) saveProfileCmd(p profile.Profile) tea.Cmd {
	if m.ipc != nil {
		client := m.ipc
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := client.Save(ctx, ipc.SaveParams{Profile: p}); err != nil {
				return saveMsg{name: p.Name, err: err, profileTab: true}
			}
			return saveMsg{name: p.Name, profileTab: true}
		}
	}
	store := m.store
	return func() tea.Msg {
		if err := profileio.SaveWithSidecars(store, p); err != nil {
			return saveMsg{name: p.Name, err: err, profileTab: true}
		}
		return saveMsg{name: p.Name, profileTab: true}
	}
}

func (m Model) deleteCmd(name string) tea.Cmd {
	if m.ipc != nil {
		client := m.ipc
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := client.Delete(ctx, name); err != nil {
				return deleteMsg{name: name, err: err}
			}
			return deleteMsg{name: name}
		}
	}
	store := m.store
	return func() tea.Msg {
		if err := store.Delete(name); err != nil {
			return deleteMsg{name: name, err: err}
		}
		return deleteMsg{name: name}
	}
}

func (m Model) setProfileAutomaticCmd(enabled bool) tea.Cmd {
	client := m.ipc
	return func() tea.Msg {
		if client == nil {
			return profileAutoMsg{enabled: enabled, err: errors.New("automatic profile selection requires the daemon")}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
		defer cancel()
		return profileAutoMsg{enabled: enabled, err: client.SetProfileAuto(ctx, enabled)}
	}
}

func (m Model) applyCmd(p profile.Profile, allowUnmanagedOverwrite ...bool) tea.Cmd {
	if m.ipc != nil {
		client := m.ipc
		guard := m.remoteGuard
		if guard != nil {
			guard.begin()
		}
		return func() tea.Msg {
			if guard != nil {
				defer guard.finish()
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			transaction, err := client.Preview(ctx, ipc.PreviewParams{
				Profile:        &p,
				TimeoutSeconds: 10,
			})
			if err != nil {
				return applyMsg{profile: p, remote: true, err: err}
			}
			if guard != nil {
				guard.arm(transaction.ID)
			}
			return applyMsg{
				profile:       transaction.Profile,
				transactionID: transaction.ID,
				deadline:      transaction.Deadline,
				remote:        true,
			}
		}
	}

	client := m.client
	engine := m.engine
	guard := m.revertGuard
	if guard != nil {
		guard.begin()
	}
	return func() tea.Msg {
		if guard != nil {
			defer guard.finish()
		}
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
		if guard != nil {
			guard.arm(snapshot)
		}
		return applyMsg{profile: applyProfile, snapshot: snapshot}
	}
}

func (m *Model) armPendingRevert(snapshot apply.RevertState) {
	if m.revertGuard == nil {
		m.revertGuard = &pendingRevertGuard{}
	}
	m.revertGuard.arm(snapshot)
}

func (m *Model) disarmPendingRevert() {
	if m.revertGuard != nil {
		m.revertGuard.disarm()
	}
}

func (m *Model) armPendingRemote(transactionID string) {
	if m.remoteGuard == nil {
		m.remoteGuard = &pendingRemoteGuard{}
	}
	m.remoteGuard.arm(transactionID)
}

func (m *Model) disarmPendingRemote() {
	if m.remoteGuard != nil {
		m.remoteGuard.disarm()
	}
}

func (m Model) RevertPending(ctx context.Context) error {
	if m.remoteGuard != nil {
		transactionID, armed, err := m.remoteGuard.pending(ctx)
		if err != nil {
			return err
		}
		if armed {
			if m.ipc == nil {
				return errors.New("cannot revert daemon transaction without IPC connection")
			}
			if err := m.ipc.Revert(ctx, transactionID); err != nil && !errors.Is(err, ipc.ErrTransactionUnavailable) {
				return err
			}
			m.remoteGuard.disarm()
		}
	}
	if m.revertGuard == nil {
		return nil
	}
	snapshot, armed, err := m.revertGuard.pending(ctx)
	if err != nil || !armed {
		return err
	}
	if err := m.engine.Revert(ctx, snapshot); err != nil {
		return err
	}
	m.revertGuard.disarm()
	return nil
}

func (g *pendingRevertGuard) begin() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.inFlight == 0 {
		g.idle = make(chan struct{})
	}
	g.inFlight++
}

func (g *pendingRevertGuard) finish() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.inFlight--
	if g.inFlight == 0 {
		close(g.idle)
		g.idle = nil
	}
}

func (g *pendingRevertGuard) arm(snapshot apply.RevertState) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.snapshot = snapshot
	g.armed = true
}

func (g *pendingRevertGuard) disarm() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.armed = false
}

func (g *pendingRevertGuard) pending(ctx context.Context) (apply.RevertState, bool, error) {
	for {
		g.mu.Lock()
		if g.inFlight == 0 {
			snapshot := g.snapshot
			armed := g.armed
			g.mu.Unlock()
			return snapshot, armed, nil
		}
		idle := g.idle
		g.mu.Unlock()

		select {
		case <-idle:
		case <-ctx.Done():
			return apply.RevertState{}, false, ctx.Err()
		}
	}
}

func (g *pendingRevertGuard) isArmed() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.armed
}

func (g *pendingRemoteGuard) begin() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.inFlight == 0 {
		g.idle = make(chan struct{})
	}
	g.inFlight++
}

func (g *pendingRemoteGuard) finish() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.inFlight--
	if g.inFlight == 0 {
		close(g.idle)
		g.idle = nil
	}
}

func (g *pendingRemoteGuard) arm(transactionID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.transactionID = transactionID
	g.armed = true
}

func (g *pendingRemoteGuard) disarm() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.armed = false
	g.transactionID = ""
}

func (g *pendingRemoteGuard) pending(ctx context.Context) (string, bool, error) {
	for {
		g.mu.Lock()
		if g.inFlight == 0 {
			transactionID := g.transactionID
			armed := g.armed
			g.mu.Unlock()
			return transactionID, armed, nil
		}
		idle := g.idle
		g.mu.Unlock()

		select {
		case <-idle:
		case <-ctx.Done():
			return "", false, ctx.Err()
		}
	}
}

func (m Model) postApply(p profile.Profile) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return m.engine.PostApply(ctx, p)
}

func (m Model) confirmPending(pending pendingApply) error {
	if pending.remote {
		if m.ipc == nil {
			return errors.New("cannot confirm daemon transaction without IPC connection")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return m.ipc.Confirm(ctx, pending.transactionID)
	}
	return m.postApply(pending.profile)
}

func (m Model) revertCmd(pending pendingApply, reason string) tea.Cmd {
	if pending.remote {
		client := m.ipc
		guard := m.remoteGuard
		if guard != nil {
			guard.begin()
		}
		return func() tea.Msg {
			if guard != nil {
				defer guard.finish()
			}
			if client == nil {
				return revertMsg{err: errors.New("cannot revert daemon transaction without IPC connection"), reason: reason}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			err := client.Revert(ctx, pending.transactionID)
			if errors.Is(err, ipc.ErrTransactionUnavailable) {
				err = nil
			}
			if err == nil && guard != nil {
				guard.disarm()
			}
			return revertMsg{err: err, reason: reason}
		}
	}

	engine := m.engine
	guard := m.revertGuard
	if guard != nil {
		guard.begin()
	}
	return func() tea.Msg {
		if guard != nil {
			defer guard.finish()
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := engine.Revert(ctx, pending.snapshot)
		if err == nil && guard != nil {
			guard.disarm()
		}
		return revertMsg{err: err, reason: reason}
	}
}

func (m Model) layoutFieldValue(output editableOutput, field int) string {
	switch field {
	case 0:
		return boolText(output.Enabled)
	case 1:
		return output.DisplayMode()
	case 2:
		return scaling.Format(output.Scale)
	case 3:
		if output.Bitdepth == 0 {
			return "8"
		}
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
		return fmt.Sprintf("%.2f", sdrMultiplier(output.SDRBrightness))
	case 11:
		return fmt.Sprintf("%.2f", sdrMultiplier(output.SDRSaturation))
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

func (m Model) layoutFieldIssue(output editableOutput, field int) (string, bool) {
	switch field {
	case 1:
		if output.ModeUnsupported || output.ModeIndex < 0 || (len(output.Modes) > 0 && output.ModeIndex >= len(output.Modes)) {
			return "unsupported", true
		}
	case 2:
		if output.Enabled && !scaling.Sharp(output.Width, output.Height, output.Scale) {
			return "fractional px", true
		}
	case 3:
		if output.Bitdepth != 0 && output.Bitdepth != 8 && output.Bitdepth != 10 {
			return "invalid", true
		}
	case 4:
		if !validStringOption(output.CM, "", "srgb", "auto", "wide", "hdr", "hdredid", "dcip3", "dp3", "adobe", "edid") {
			return "invalid", true
		}
	case 5:
		if output.VRR < 0 || output.VRR > 2 {
			return "invalid", true
		}
	case 6:
		if output.Transform < 0 || output.Transform > 7 {
			return "invalid", true
		}
	case 9:
		if output.MirrorOf != "" {
			if output.MirrorOf == output.Key {
				return "self mirror", true
			}
			if !m.outputKeyExists(output.MirrorOf) {
				return "missing target", true
			}
		}
	case 10:
		if output.SDRBrightness < 0 || output.SDRBrightness > 3 {
			return "out of range", true
		}
	case 11:
		if output.SDRSaturation < 0 || output.SDRSaturation > 3 {
			return "out of range", true
		}
	case 12:
		if output.SDRMinLuminance < 0 || output.SDRMinLuminance > 1 {
			return "out of range", true
		}
	case 13:
		if output.SDRMaxLuminance < 0 || output.SDRMaxLuminance > 1000 {
			return "out of range", true
		}
	case 14:
		if !validStringOption(output.SDREOTF, "", "default", "gamma22", "srgb") {
			return "invalid", true
		}
	case 15:
		if output.MinLuminance < 0 || output.MinLuminance > 1000 {
			return "out of range", true
		}
	case 16:
		if output.MaxLuminance < 0 || output.MaxLuminance > 2000 {
			return "out of range", true
		}
	case 17:
		if output.MaxAvgLuminance < 0 || output.MaxAvgLuminance > 2000 {
			return "out of range", true
		}
	case 18:
		if output.SupportsWideColor < -1 || output.SupportsWideColor > 1 {
			return "invalid", true
		}
	case 19:
		if output.SupportsHDR < -1 || output.SupportsHDR > 1 {
			return "invalid", true
		}
	case 20:
		icc := strings.TrimSpace(output.ICC)
		if icc != "" && !filepath.IsAbs(icc) {
			return "needs abs path", true
		}
	}
	return "", false
}

func (m Model) outputKeyExists(key string) bool {
	for _, output := range m.editOutputs {
		if output.Key == key {
			return true
		}
	}
	return false
}

func validStringOption(value string, allowed ...string) bool {
	value = strings.TrimSpace(value)
	for _, option := range allowed {
		if value == option {
			return true
		}
	}
	return false
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
		if m.workspaceEdit.Strategy != profile.WorkspaceStrategySequential {
			return "—"
		}
		return fmt.Sprintf("%d", m.workspaceEdit.GroupSize)
	default:
		return ""
	}
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

func (m *Model) markDirty() {
	m.dirty = true
	m.draftSaved = false
}

func (m *Model) markClean() {
	m.dirty = false
	m.draftSaved = false
}

func editableOutputFromProfile(saved profile.OutputConfig, live hypr.Monitor, hasLive bool) editableOutput {
	output := editableOutput{
		Key:               saved.Key,
		MatchKey:          saved.MatchIdentity(),
		Name:              saved.Name,
		Description:       saved.Description,
		Make:              saved.Make,
		Model:             saved.Model,
		Serial:            saved.Serial,
		Enabled:           saved.Enabled,
		Width:             saved.Width,
		Height:            saved.Height,
		Refresh:           saved.Refresh,
		X:                 saved.X,
		Y:                 saved.Y,
		Scale:             scaling.Round(scaling.Clamp(saved.Scale)),
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
		output.ModeUnsupported = len(live.AvailableModes) > 0 && indexOf(live.AvailableModes, mode) < 0
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
	if strategy == profile.WorkspaceStrategyManual {
		if inferred := numericManualWorkspaceMaximum(settings.Rules); inferred > 0 {
			maxWorkspaces = inferred
		}
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
		ManualRulesInitialized:  strategy == profile.WorkspaceStrategyManual && len(settings.Rules) > 0,
	}
}

func numericManualWorkspaceMaximum(rules []profile.WorkspaceRule) int {
	maximum := 0
	for _, rule := range rules {
		value, err := strconv.Atoi(strings.TrimSpace(rule.Workspace))
		if err == nil && value > maximum {
			maximum = value
		}
	}
	return maximum
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
		Description:       o.Description,
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
		Scale:             scaling.Round(scaling.Clamp(o.Scale)),
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
	scale := scaling.Round(scaling.Clamp(o.Scale))
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

const unknownModelLabel = "(unknown)"

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
	return unknownModelLabel
}

func isInternalOutputName(name string) bool {
	return hypr.IsInternalConnector(name)
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
	return o.cardLinesWithIssue(maxLines, fg, muted, "", "")
}

func (o editableOutput) cardLinesWithIssue(maxLines int, fg string, muted string, issue string, issueFG string) []cardLine {
	if maxLines <= 0 {
		return nil
	}

	scaleLayout := fmt.Sprintf("%sx=%s", scaling.Format(o.Scale), strings.ReplaceAll(o.layoutSizeLabel(), " ", ""))
	position := fmt.Sprintf("pos %d,%d", o.X, o.Y)
	name := o.Name
	if issue != "" {
		name += " ⚠"
	}
	issueLine := cardLine{text: "⚠ " + issue, fg: issueFG, bold: true}
	full := []cardLine{
		{text: name, fg: fg, bold: true},
		{text: o.cardModelLabel(), fg: muted},
		{text: o.DisplayMode(), fg: muted},
		{text: scaleLayout, fg: muted},
		{text: position, fg: muted},
	}
	if issue != "" {
		warnFull := []cardLine{
			full[0],
			issueLine,
			full[1],
			full[2],
			full[3],
			full[4],
		}
		if maxLines >= len(warnFull) {
			return warnFull
		}
		switch maxLines {
		case 5:
			return []cardLine{full[0], issueLine, full[1], full[2], full[3]}
		case 4:
			return []cardLine{full[0], issueLine, full[1], full[2]}
		case 3:
			return []cardLine{full[0], issueLine, cardLine{text: scaleLayout + "  " + position, fg: muted}}
		case 2:
			return []cardLine{full[0], issueLine}
		default:
			return []cardLine{full[0]}
		}
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
			full[1],
			{text: scaleLayout + "  " + position, fg: muted},
		}
	case 2:
		return []cardLine{
			full[0],
			full[1],
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
	if _, ok := m.canvasOutputIssue(output); ok && !selected {
		colors.border = p.warning
	}
	if m.layoutErr != nil && m.isOutputOverlapping(output) && !selected {
		colors.border = "#FF0000"
		colors.fg = "#FF0000"
	}
	return colors
}

func (m Model) canvasOutputIssue(output editableOutput) (string, bool) {
	for idx := range layoutFields {
		if issue, ok := m.layoutFieldIssue(output, idx); ok {
			return issue, true
		}
	}
	if m.layoutErr != nil && m.isOutputOverlapping(output) {
		return "overlap", true
	}
	return "", false
}

// paintCard draws one monitor rectangle. The caller supplies the body lines
// once the card knows how much room it can spare for them.
func paintCard(grid [][]canvasCell, rect canvasRect, emphasized bool, colors canvasCardColors, body func(maxLines, maxWidth int) []cardLine) {
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
		grid[y1][x] = canvasCell{ch: '─', fg: colors.border, bg: colors.bg, bold: emphasized}
		grid[y2][x] = canvasCell{ch: '─', fg: colors.border, bg: colors.bg, bold: emphasized}
	}
	for y := y1 + 1; y < y2; y++ {
		grid[y][x1] = canvasCell{ch: '│', fg: colors.border, bg: colors.bg, bold: emphasized}
		grid[y][x2] = canvasCell{ch: '│', fg: colors.border, bg: colors.bg, bold: emphasized}
	}
	grid[y1][x1] = canvasCell{ch: '╭', fg: colors.border, bg: colors.bg, bold: emphasized}
	grid[y1][x2] = canvasCell{ch: '╮', fg: colors.border, bg: colors.bg, bold: emphasized}
	grid[y2][x1] = canvasCell{ch: '╰', fg: colors.border, bg: colors.bg, bold: emphasized}
	grid[y2][x2] = canvasCell{ch: '╯', fg: colors.border, bg: colors.bg, bold: emphasized}

	availableHeight := y2 - y1 - 1
	lines := body(max(1, availableHeight), max(1, x2-x1-1))
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

// canvasSegment is one styled run in the canvas overlay strip.
type canvasSegment struct {
	text string
	fg   string
	bold bool
}

// paintCanvasSegments writes a styled strip along one canvas row. The layout
// always leaves the first row free of cards, so the strip never hides one.
func paintCanvasSegments(grid [][]canvasCell, y, left int, segments []canvasSegment) {
	if y < 0 || y >= len(grid) {
		return
	}
	x := left
	for _, segment := range segments {
		for _, r := range segment.text {
			if x < 0 || x >= len(grid[y]) {
				return
			}
			grid[y][x] = canvasCell{ch: r, fg: segment.fg, bg: grid[y][x].bg, bold: segment.bold}
			x++
		}
	}
}

// hiddenOutputSegments names the displays a canvas cannot draw: the ones this
// layout turns off and the ones that mirror another display.
func (m Model) hiddenOutputSegments(outputs []editableOutput, selected int, width int) []canvasSegment {
	off := make([]int, 0, len(outputs))
	mirrored := make([]int, 0, len(outputs))
	for idx, output := range outputs {
		switch {
		case !output.Enabled:
			off = append(off, idx)
		case output.MirrorOf != "":
			mirrored = append(mirrored, idx)
		}
	}
	if len(off) == 0 && len(mirrored) == 0 {
		return nil
	}

	p := m.styles.palette
	segments := make([]canvasSegment, 0, len(off)+len(mirrored)+4)
	first := true
	add := func(label string, indexes []int, name func(editableOutput) string) {
		if len(indexes) == 0 {
			return
		}
		if !first {
			segments = append(segments, canvasSegment{text: "   ", fg: p.cardMuted})
		}
		first = false
		segments = append(segments, canvasSegment{text: label, fg: p.cardMuted})
		for pos, idx := range indexes {
			if pos > 0 {
				segments = append(segments, canvasSegment{text: ", ", fg: p.cardMuted})
			}
			fg, bold := p.cardDisabledFg, false
			if idx == selected {
				fg, bold = p.cardSelectedBorder, true
			}
			segments = append(segments, canvasSegment{text: name(outputs[idx]), fg: fg, bold: bold})
		}
	}

	segments = append(segments, canvasSegment{text: " ", fg: p.cardMuted})
	add("Off: ", off, func(o editableOutput) string { return o.Name })
	add("Mirrored: ", mirrored, func(o editableOutput) string {
		return o.Name + " → " + outputNameForKeyIn(outputs, o.MirrorOf)
	})
	segments = append(segments, canvasSegment{text: " ", fg: p.cardMuted})

	used := 0
	for idx, segment := range segments {
		remaining := width - used
		if remaining <= 0 {
			return segments[:idx]
		}
		if lipgloss.Width(segment.text) > remaining {
			segments[idx].text = fitString(segment.text, remaining)
			return segments[:idx+1]
		}
		used += lipgloss.Width(segment.text)
	}
	return segments
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

func layoutSnapDirection(key string) (snapDirection, bool) {
	switch key {
	case "alt+left":
		return snapDirectionLeft, true
	case "alt+right":
		return snapDirectionRight, true
	case "alt+up":
		return snapDirectionUp, true
	case "alt+down":
		return snapDirectionDown, true
	default:
		return snapDirectionLeft, false
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

func adjustPositiveInt(value, delta int) int {
	maxInt := int(^uint(0) >> 1)
	if delta > 0 && value > maxInt-delta {
		return maxInt
	}
	if delta < 0 && value < 1-delta {
		return 1
	}
	return max(1, value+delta)
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
	"Color depth (bpc)",
	"Color space / EOTF",
	"VRR",
	"Rotation",
	"Position X",
	"Position Y",
	"Mirror",
	"SDR luminance scale",
	"SDR saturation scale",
	"SDR black level (cd/m²)",
	"SDR white level (cd/m²)",
	"SDR EOTF",
	"Display black (cd/m²)",
	"Display peak (cd/m²)",
	"Max frame-average (cd/m²)",
	"WCG capability",
	"HDR capability",
	"ICC device profile",
}

const advancedFieldStart = 10

func layoutFieldShortLabel(field int) string {
	switch field {
	case 0:
		return "On"
	case 3:
		return "Depth (bpc)"
	case 4:
		return "Space/EOTF"
	case 10:
		return "SDR lum. x"
	case 11:
		return "SDR sat. x"
	case 12:
		return "SDR black"
	case 13:
		return "SDR white"
	case 15:
		return "Disp. black"
	case 16:
		return "Disp. peak"
	case 17:
		return "Frame avg."
	case 18:
		return "WCG cap."
	case 19:
		return "HDR cap."
	case 20:
		return "ICC profile"
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

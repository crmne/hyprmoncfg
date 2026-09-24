package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/crmne/hyprmoncfg/internal/buildinfo"
	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

func paneTestMonitor(name, make_, model string, width, height int, x, y int) hypr.Monitor {
	return hypr.Monitor{
		Name: name, Description: make_ + " " + model, Make: make_, Model: model,
		Width: width, Height: height, RefreshRate: 60, X: x, Y: y, Scale: 1,
		AvailableModes: []string{hypr.FormatMode(width, height, 60)},
	}
}

var (
	paneTestDesk   = paneTestMonitor("DP-1", "Microstep", "MPG321UR-QD", 3840, 2160, 0, 0)
	paneTestSide   = paneTestMonitor("DP-2", "LG Electronics", "16MR70", 2560, 1600, 3840, 0)
	paneTestLaptop = paneTestMonitor("eDP-1", "BOE", "Panel", 1920, 1080, 0, 2160)
)

func paneTestModel(t *testing.T, tab mainTab, monitors []hypr.Monitor, profiles []profile.Profile) Model {
	t.Helper()

	m := Model{
		styles:      newStyles(),
		mode:        modeMain,
		tab:         tab,
		layoutFocus: layoutFocusCanvas,
		width:       120,
		height:      32,
		monitors:    monitors,
		profiles:    profiles,
	}
	m.loadLiveState()
	m.status = ""
	return m
}

func requireContains(t *testing.T, view string, wants ...string) {
	t.Helper()

	for _, want := range wants {
		if !strings.Contains(view, want) {
			t.Fatalf("expected view to contain %q, got:\n%s", want, view)
		}
	}
}

func TestProfilesListShowsMatchColumnWithBadges(t *testing.T) {
	here := profile.FromState("Desk Solo", []hypr.Monitor{paneTestDesk}, nil)
	away := profile.FromState("Travel", []hypr.Monitor{paneTestLaptop}, nil)

	m := paneTestModel(t, tabProfiles, []hypr.Monitor{paneTestDesk}, []profile.Profile{here, away})
	view := ansi.Strip(m.renderMain())

	requireContains(t, view,
		profileListNameHeader,
		profileListScoreHeader,
		"Desk Solo",
		profileTagActive,
		"100",
	)

	for _, line := range strings.Split(view, "\n") {
		if !strings.Contains(line, "Travel") {
			continue
		}
		if !strings.Contains(line, "–") {
			t.Fatalf("expected the unconnected profile to show no score, got %q", line)
		}
		return
	}
	t.Fatalf("expected a row for the unconnected profile, got:\n%s", view)
}

func TestProfilesListShowsAutomaticSelectionMode(t *testing.T) {
	m := paneTestModel(t, tabProfiles, []hypr.Monitor{paneTestDesk}, []profile.Profile{
		profile.FromState("Desk Solo", []hypr.Monitor{paneTestDesk}, nil),
	})
	m.daemonOK = true

	automatic := ansi.Strip(m.renderMain())
	requireContains(t, automatic, "Automatic profile selection", "on")

	m.profileOverride = "Desk Solo"
	manual := ansi.Strip(m.renderMain())
	requireContains(t, manual, "Automatic profile selection", "off")
}

func TestProfileDetailsExplainMatchScore(t *testing.T) {
	both := profile.FromState("Desk Dual", []hypr.Monitor{paneTestDesk, paneTestSide}, nil)

	m := paneTestModel(t, tabProfiles, []hypr.Monitor{paneTestDesk}, []profile.Profile{both})
	view := ansi.Strip(m.renderMain())

	requireContains(t, view,
		"Match",
		"score 70",
		"+100",
		"1 display connected",
		"-30",
		"1 display not connected",
		"Displays   2 saved · 1 connected",
	)
}

func TestProfileDetailsDrawMonitorLayoutMarkingAbsentDisplays(t *testing.T) {
	both := profile.FromState("Desk Dual", []hypr.Monitor{paneTestDesk, paneTestSide}, nil)
	both.Workspaces = profile.WorkspaceSettings{
		Enabled:       true,
		Strategy:      profile.WorkspaceStrategySequential,
		MaxWorkspaces: 6,
		GroupSize:     3,
		MonitorOrder:  []string{both.Outputs[0].Key, both.Outputs[1].Key},
	}

	m := paneTestModel(t, tabProfiles, []hypr.Monitor{paneTestDesk}, []profile.Profile{both})
	view := ansi.Strip(m.renderMain())

	requireContains(t, view, "Monitor Layout", "DP-1", "DP-2", "not connected", "1, 2, 3", "4, 5, 6")
}

func TestProfileDetailsListDisplaysTheCanvasCannotDraw(t *testing.T) {
	p := profile.FromState("Desk", []hypr.Monitor{paneTestDesk, paneTestSide, paneTestLaptop}, nil)
	for idx := range p.Outputs {
		switch p.Outputs[idx].Name {
		case "eDP-1":
			p.Outputs[idx].Enabled = false
		case "DP-2":
			p.Outputs[idx].MirrorOf = "DP-1"
		}
	}
	p.Normalize()

	deskKey := ""
	for _, output := range p.Outputs {
		if output.Name == "DP-1" {
			deskKey = output.Key
		}
	}
	for idx := range p.Outputs {
		if p.Outputs[idx].Name == "DP-2" {
			p.Outputs[idx].MirrorOf = deskKey
		}
	}

	m := paneTestModel(t, tabProfiles, []hypr.Monitor{paneTestDesk}, []profile.Profile{p})
	view := ansi.Strip(m.renderMain())

	requireContains(t, view, "Kept off", "BOE Panel", "Mirrors", "LG Electronics 16MR70 → Microstep MPG321UR-QD")
}

func TestLayoutCanvasNamesDisplaysItCannotDraw(t *testing.T) {
	off := paneTestLaptop
	off.Disabled = true
	mirror := paneTestSide
	mirror.MirrorOf = "DP-1"

	m := paneTestModel(t, tabLayout, []hypr.Monitor{paneTestDesk, off, mirror}, nil)
	view := ansi.Strip(m.renderMain())

	requireContains(t, view, "eDP-1  Off", "[Enable]", "DP-2  Mirrors DP-1")
}

func TestWorkspacePreviewDrawsPlanOnMonitorLayout(t *testing.T) {
	m := paneTestModel(t, tabWorkspaces, []hypr.Monitor{paneTestDesk, paneTestSide}, nil)
	m.workspaceEdit.Enabled = true
	m.workspaceEdit.Strategy = profile.WorkspaceStrategySequential
	m.workspaceEdit.MaxWorkspaces = 6
	m.workspaceEdit.GroupSize = 3

	view := ansi.Strip(m.renderMain())
	requireContains(t, view, "Workspace Plan", "Monitor Layout", "Microstep MPG321UR-QD  1, 2, 3")

	// The plan also lands on the cards, not just in the list.
	canvas := view[strings.Index(view, "Monitor Layout"):]
	requireContains(t, canvas, "4, 5, 6")
}

func TestFitWorkspaceLinesWrapsAndReportsOverflow(t *testing.T) {
	workspaces := []string{"1", "2", "3", "4", "5", "6"}

	if got := fitWorkspaceLines(workspaces, 20, 1); len(got) != 1 || got[0] != "1, 2, 3, 4, 5, 6" {
		t.Fatalf("expected a single unwrapped row, got %q", got)
	}

	got := fitWorkspaceLines(workspaces, 6, 2)
	want := []string{"1, 2,", "3 +3"}
	if len(got) != len(want) {
		t.Fatalf("expected %d rows, got %q", len(want), got)
	}
	for idx := range want {
		if got[idx] != want[idx] {
			t.Fatalf("row %d = %q, want %q", idx, got[idx], want[idx])
		}
	}

	if got := fitWorkspaceLines(workspaces, 4, 1); len(got) != 1 || !strings.HasSuffix(got[0], "+5") {
		t.Fatalf("expected a single row reporting the overflow, got %q", got)
	}
}

func TestProfileListColumnsShrinkOnNarrowPanes(t *testing.T) {
	m := Model{styles: newStyles(), profiles: []profile.Profile{profile.New("Desk", nil)}}

	wide := m.profileListColumns(40)
	if wide.tag != profileListTagWidth || wide.score != profileListScoreWidth {
		t.Fatalf("expected a wide pane to keep every column, got %+v", wide)
	}

	narrow := m.profileListColumns(20)
	if narrow.tag != 0 || narrow.score != profileListScoreWidth {
		t.Fatalf("expected a narrow pane to drop the badge column first, got %+v", narrow)
	}

	tiny := m.profileListColumns(12)
	if tiny.tag != 0 || tiny.score != 0 || tiny.name < 1 {
		t.Fatalf("expected a tiny pane to keep only names, got %+v", tiny)
	}
}

func TestPaneTonesSeparateFocusFromPresence(t *testing.T) {
	m := Model{styles: newStyles()}

	focused := m.paneBorderColor(paneToneFocused)
	static := m.paneBorderColor(paneToneStatic)
	idle := m.paneBorderColor(paneToneIdle)

	if focused != m.styles.palette.paneActiveBorder {
		t.Fatalf("focused panes should carry the accent, got %q", focused)
	}
	if static == focused || static == idle {
		t.Fatalf("static panes need their own tone, got %q between %q and %q", static, focused, idle)
	}
	if m.paneStyle(paneToneStatic).GetVerticalFrameSize() != m.paneStyle(paneToneIdle).GetVerticalFrameSize() ||
		m.paneStyle(paneToneStatic).GetHorizontalFrameSize() != m.paneStyle(paneToneIdle).GetHorizontalFrameSize() {
		t.Fatal("every pane tone must keep the same frame so layout and hit-testing hold")
	}
}

func TestPreviewCanvasesDoNotClaimTheAccent(t *testing.T) {
	m := paneTestModel(t, tabProfiles, []hypr.Monitor{paneTestDesk}, nil)

	static := m.staticCardStyle()
	if static.border == m.styles.palette.cardSelectedBorder {
		t.Fatal("a monitor nobody is dragging should not wear the selected border")
	}
	if static.border == m.styles.palette.cardBorder {
		t.Fatal("a monitor on a read-only canvas should read clearer than a monitor you could select")
	}
}

func TestEveryTabUsesTheSameAdaptiveMonitorCardVocabulary(t *testing.T) {
	m := Model{styles: newStyles()}
	output := editableOutput{
		Name:      "DP-1",
		Make:      "Microstep",
		Model:     "MPG321UR-QD",
		Enabled:   true,
		Modes:     []string{"3840x2160@143.99Hz"},
		ModeIndex: 0,
		Width:     3840,
		Height:    2160,
		Scale:     1.33333,
		X:         10,
		Y:         20,
	}
	colors := m.staticCardStyle()
	emphases := []monitorCardEmphasis{monitorCardLayout, monitorCardProfile, monitorCardWorkspaces}
	var expected []string
	for idx, emphasis := range emphases {
		lines := m.monitorCardLines(output, []string{"1", "2", "3", "4"}, emphasis,
			6, 40, colors, "", m.styles.palette.warning)
		texts := make([]string, len(lines))
		for line := range lines {
			texts[line] = lines[line].text
		}
		if idx == 0 {
			expected = texts
		} else if strings.Join(texts, "\n") != strings.Join(expected, "\n") {
			t.Fatalf("emphasis %d changed monitor content: got %q want %q", emphasis, texts, expected)
		}
		if got := lines[len(lines)-1]; got.text != "1, 2, 3, 4" || got.bold != (emphasis != monitorCardLayout) {
			t.Fatalf("workspace emphasis %d = %+v", emphasis, got)
		}
	}

	requireContains(t, strings.Join(expected, "\n"),
		"DP-1", "Microstep MPG321UR-QD", "3840x2160@144Hz", "Position 10,20", "1, 2, 3, 4")

	compact := m.monitorCardLines(output, []string{"1", "2", "3", "4"}, monitorCardLayout,
		3, 40, colors, "", m.styles.palette.warning)
	if got := []string{compact[0].text, compact[1].text, compact[2].text}; strings.Join(got, "|") != "DP-1|Microstep MPG321UR-QD|1, 2, 3, 4" {
		t.Fatalf("compact monitor identity/workspace priority = %q", got)
	}
}

func TestLayoutCanvasIncludesTheDraftWorkspacePlan(t *testing.T) {
	m := paneTestModel(t, tabLayout, []hypr.Monitor{paneTestDesk, paneTestSide}, nil)
	m.workspaceEdit.Enabled = true
	m.workspaceEdit.Strategy = profile.WorkspaceStrategySequential
	m.workspaceEdit.MaxWorkspaces = 6
	m.workspaceEdit.GroupSize = 3

	view := ansi.Strip(m.renderCanvas(100, 20))
	requireContains(t, view, "1, 2, 3", "4, 5, 6")
}

func TestLoadLiveStateSelectsTheActiveProfile(t *testing.T) {
	desk := paneTestDesk
	desk.Focused = true
	profiles := []profile.Profile{
		profile.FromState("Away", []hypr.Monitor{paneTestLaptop}, nil),
		profile.FromState("Desk Dual", []hypr.Monitor{paneTestDesk, paneTestSide}, nil),
		profile.FromState("Desk Solo", []hypr.Monitor{desk}, nil),
	}

	m := paneTestModel(t, tabProfiles, []hypr.Monitor{desk}, profiles)
	if got := m.profiles[m.selectedProfile].Name; got != "Desk Solo" {
		t.Fatalf("expected the profile already on screen to be selected, got %q", got)
	}
}

func TestLoadLiveStateFallsBackToTheHighestScoringProfile(t *testing.T) {
	desk := paneTestDesk
	desk.Focused = true
	// No profile reproduces the live state exactly, so the best score wins.
	dual := profile.FromState("Desk Dual", []hypr.Monitor{paneTestDesk, paneTestSide}, nil)
	away := profile.FromState("Away", []hypr.Monitor{paneTestLaptop}, nil)

	m := paneTestModel(t, tabProfiles, []hypr.Monitor{desk}, []profile.Profile{away, dual})
	if got := m.profiles[m.selectedProfile].Name; got != "Desk Dual" {
		t.Fatalf("expected the highest scoring profile to be selected, got %q", got)
	}

	summaries := m.profileMatchSummaries()
	if summaries[m.selectedProfile].active {
		t.Fatal("expected the fallback selection to be a recommendation, not an active profile")
	}
}

func TestProfileRecommendationIncludesPartialBasesAndExplicitStrictProfiles(t *testing.T) {
	saved := profile.FromMonitors("Laptop", []hypr.Monitor{paneTestLaptop})
	m := Model{profiles: []profile.Profile{saved}, monitors: []hypr.Monitor{paneTestLaptop, paneTestDesk}}
	summaries := m.profileMatchSummaries()
	if !summaries[0].matches() || !summaries[0].recommended || summaries[0].active {
		t.Fatalf("partial match must recommend the extension base without claiming it is active: %+v", summaries[0])
	}
	m.profiles[0].DisableUnknownOutputs = true
	if summaries = m.profileMatchSummaries(); !summaries[0].recommended || summaries[0].active {
		t.Fatalf("explicit strict profile must remain a recommendation: %+v", summaries[0])
	}

	// Removing the unfamiliar display retains the ordinary undocked fallback.
	m.monitors = []hypr.Monitor{paneTestLaptop}
	if summaries = m.profileMatchSummaries(); !summaries[0].recommended {
		t.Fatalf("known laptop was not recommended after undocking: %+v", summaries[0])
	}
}

func TestBusyDaemonIsNotReportedAsStopped(t *testing.T) {
	m := Model{styles: newStyles(), daemonOK: true, profileOverride: "Desk Solo"}

	busy, _ := m.Update(refreshMsg{daemonOK: false, daemonUnknown: true, background: true})
	busyModel := mustModel(t, busy)
	if !busyModel.daemonOK {
		t.Fatal("a probe that timed out must leave the last known daemon state alone")
	}
	if busyModel.profileOverride != "Desk Solo" {
		t.Fatalf("a timed-out probe must preserve manual profile mode, got %q", busyModel.profileOverride)
	}

	stopped, _ := m.Update(refreshMsg{daemonOK: false, background: true})
	if mustModel(t, stopped).daemonOK {
		t.Fatal("a conclusive probe failure must report the daemon as stopped")
	}
}

func TestTopStatusNamesTheProfileOnScreen(t *testing.T) {
	desk := paneTestDesk
	desk.Focused = true
	solo := profile.FromState("Desk Solo", []hypr.Monitor{desk}, nil)
	dual := profile.FromState("Desk Dual", []hypr.Monitor{paneTestDesk, paneTestSide}, nil)

	matched := paneTestModel(t, tabProfiles, []hypr.Monitor{desk}, []profile.Profile{solo})
	if got := ansi.Strip(matched.renderTopStatus()); !strings.Contains(got, "Desk Solo") {
		t.Fatalf("expected the active profile to be named, got %q", got)
	}

	unmatched := paneTestModel(t, tabProfiles, []hypr.Monitor{desk}, []profile.Profile{dual})
	if got := ansi.Strip(unmatched.renderTopStatus()); !strings.Contains(got, "no match") {
		t.Fatalf("expected an unsaved layout to say so, got %q", got)
	}
}

func TestConfirmationDialogAcceptsAShiftedYes(t *testing.T) {
	// "Keep this configuration? y/n" gets answered with Shift held, and a case
	// sensitive match makes the dialog look broken.
	m := Model{
		styles:  newStyles(),
		mode:    modeConfirm,
		pending: &pendingApply{profile: profile.New("desk", nil)},
	}

	updated, cmd := m.updateConfirmKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'Y'}})
	if cmd == nil {
		t.Fatal("expected an uppercase Y to confirm")
	}
	if mustModel(t, updated).mode != modeMain {
		t.Fatal("expected the dialog to close on an uppercase Y")
	}
}
func TestOriginShortcutMovesTheSelectedMonitorToZeroZero(t *testing.T) {
	m := Model{
		styles:      newStyles(),
		tab:         tabLayout,
		layoutFocus: layoutFocusCanvas,
		editOutputs: []editableOutput{
			{Key: "a", Name: "DP-1", Enabled: true, Scale: 1, Width: 3840, Height: 2160, X: 3820, Y: 927},
			{Key: "b", Name: "DP-2", Enabled: true, Scale: 1, Width: 2560, Height: 1440, X: 0, Y: 0},
		},
	}

	updated, cmd := m.updateLayoutKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'0'}})
	got := mustModel(t, updated)
	if got.editOutputs[0].X != 0 || got.editOutputs[0].Y != 0 {
		t.Fatalf("expected the selected monitor at 0,0, got %d,%d", got.editOutputs[0].X, got.editOutputs[0].Y)
	}
	if got.editOutputs[1].X != 0 || got.editOutputs[1].Y != 0 {
		t.Fatal("expected the other monitor to be left where it was")
	}
	if !got.dirty {
		t.Fatal("expected the layout to be dirty")
	}
	// Moving a monitor is not an event worth a popup; the arrows do not raise
	// one either.
	if cmd != nil {
		t.Fatal("expected no toast for an ordinary move")
	}
}

func TestAMirroredDisplayCannotBeMovedOnItsOwn(t *testing.T) {
	m := Model{
		styles:      newStyles(),
		tab:         tabLayout,
		layoutFocus: layoutFocusCanvas,
		editOutputs: []editableOutput{
			{Key: "tv", Name: "HDMI-A-1", Enabled: true, Scale: 1, X: 3820, Y: 927, MirrorOf: "desk"},
			{Key: "desk", Name: "DP-1", Enabled: true, Scale: 1, X: 0, Y: 0},
		},
	}

	m.updateLayoutKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'0'}})
	if m.editOutputs[0].X != 3820 || m.editOutputs[0].Y != 927 {
		t.Fatalf("expected the mirror to stay where Hyprland put it, got %d,%d", m.editOutputs[0].X, m.editOutputs[0].Y)
	}
	if m.dirty {
		t.Fatal("expected no unsaved change that no apply could ever produce")
	}
	if !m.statusErr || !strings.Contains(m.status, "DP-1") {
		t.Fatalf("expected the status to point at the source display, got %q", m.status)
	}
}

func TestTabWalksThePanesAndBracketsAlwaysCycleMonitors(t *testing.T) {
	m := Model{
		styles:      newStyles(),
		tab:         tabLayout,
		layoutFocus: layoutFocusCanvas,
		editOutputs: []editableOutput{
			{Key: "a", Name: "DP-1", Enabled: true, Scale: 1},
			{Key: "b", Name: "DP-2", Enabled: true, Scale: 1},
		},
	}

	for _, want := range []struct {
		focus layoutFocus
		tab   inspectorTab
	}{
		{layoutFocusInspector, inspectorTabDisplay},
		{layoutFocusInspector, inspectorTabColor},
		{layoutFocusCanvas, inspectorTabColor},
		{layoutFocusInspector, inspectorTabDisplay},
	} {
		m.updateLayoutKeys(tea.KeyMsg{Type: tea.KeyTab})
		if m.layoutFocus != want.focus || (want.focus == layoutFocusInspector && m.inspectorTab != want.tab) {
			t.Fatalf("Tab landed on focus=%v tab=%v, want focus=%v tab=%v", m.layoutFocus, m.inspectorTab, want.focus, want.tab)
		}
	}

	// Whatever pane has focus, the brackets mean monitors and nothing else.
	before := m.inspectorTab
	m.updateLayoutKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	if m.selectedOutput != 1 {
		t.Fatalf("expected ] to select the next monitor, got %d", m.selectedOutput)
	}
	if m.inspectorTab != before {
		t.Fatal("expected ] to leave the Display/Color pane alone")
	}
	m.updateLayoutKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}})
	if m.selectedOutput != 0 {
		t.Fatalf("expected [ to select the previous monitor, got %d", m.selectedOutput)
	}
}

func TestQuestionMarkShowsTheKeysForTheCurrentTab(t *testing.T) {
	m := Model{styles: newStyles(), width: 100, height: 34, tab: tabWorkspaces}

	updated, _ := m.updateMainKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	got := mustModel(t, updated)
	if got.mode != modeKeybindings {
		t.Fatalf("expected ? to open the keys dialog, got mode %v", got.mode)
	}

	view := ansi.Strip(got.View())
	requireContains(t, view, "Keys", "Workspaces", "Adjust it", "PgUp PgDn", "Home End", "Anywhere", "Switch tabs")
	if strings.Contains(view, "Snap beside") {
		t.Fatalf("expected only the current tab's keys, got:\n%s", view)
	}

	closed, _ := got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if mustModel(t, closed).mode != modeMain {
		t.Fatal("expected any key to close the dialog")
	}
}

func TestATopStatusAsksForARestartWhenTheDaemonIsBehind(t *testing.T) {
	// Upgrading the package leaves the old daemon binary serving until someone
	// restarts the user service, and nothing in packaging can do that.
	behind := Model{styles: newStyles(), daemonOK: true, daemonVersion: "1.13.0"}
	if !behind.daemonNeedsRestart() {
		t.Fatal("expected an older running daemon to ask for a restart")
	}
	if got := ansi.Strip(behind.renderTopStatus()); !strings.Contains(got, "1.13.0") || !strings.Contains(got, "restart: R") {
		t.Fatalf("expected the status to name the running version and offer a restart, got %q", got)
	}

	// The tab row swaps in a compact status when the full one does not fit, and
	// a laptop terminal is narrow enough to reach that. Dropping the one thing
	// the reader can act on is the wrong thing to drop.
	for _, width := range []int{160, 120, 100, 90, 80} {
		narrow := Model{styles: newStyles(), width: width, height: 32, daemonOK: true, daemonVersion: "1.13.0"}
		if got := ansi.Strip(narrow.renderTabs()); !strings.Contains(got, "restart: R") {
			t.Fatalf("width %d dropped the restart hint: %q", width, strings.TrimSpace(got))
		}
	}

	// The message is the click target, so it has to be reachable by mouse too.
	behind.width, behind.height = 120, 32
	x := behind.footerContentWidth() - lipgloss.Width(behind.restartHint())
	if !behind.restartHintAt(behind.appContentX()+x, behind.appContentY()) {
		t.Fatal("expected the message to be clickable")
	}
	if behind.restartHintAt(behind.appContentX(), behind.appContentY()) {
		t.Fatal("expected a click on the tabs to be left alone")
	}

	current := Model{styles: newStyles(), daemonOK: true, daemonVersion: buildinfo.Version}
	if current.daemonNeedsRestart() {
		t.Fatal("expected a matching daemon to be left alone")
	}
	// A daemon that never answered says nothing about its version.
	unknown := Model{styles: newStyles(), daemonOK: true}
	if unknown.daemonNeedsRestart() {
		t.Fatal("expected no restart nag without a reported version")
	}
}

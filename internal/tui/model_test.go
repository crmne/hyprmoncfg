package tui

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/crmne/hyprmoncfg/internal/apply"
	"github.com/crmne/hyprmoncfg/internal/buildinfo"
	"github.com/crmne/hyprmoncfg/internal/config"
	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/ipc"
	"github.com/crmne/hyprmoncfg/internal/lid"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

func TestRenderMainUsesPaneTitlesWithoutMastheadOrCanvasChrome(t *testing.T) {
	m := Model{
		styles:      newStyles(),
		mode:        modeMain,
		tab:         tabLayout,
		layoutFocus: layoutFocusInspector,
		width:       120,
		height:      36,
		editOutputs: []editableOutput{{
			Key:             "microstep|mpg321ur-qd",
			Name:            "DP-1",
			Description:     "Microstep MPG321UR-QD",
			Enabled:         true,
			Modes:           []string{"3840x2160@143.99Hz"},
			ModeIndex:       0,
			Width:           3840,
			Height:          2160,
			Refresh:         143.99,
			X:               0,
			Y:               0,
			Scale:           1.33,
			ActiveWorkspace: "1",
		}},
		workspaceEdit: workspaceEditor{
			Enabled:       true,
			Strategy:      profile.WorkspaceStrategySequential,
			MaxWorkspaces: 9,
			GroupSize:     3,
		},
	}

	view := m.renderMain()
	for _, unwanted := range []string{"Hyprland monitor layout and workspace planner", "Lid: open", "Legend"} {
		if strings.Contains(view, unwanted) {
			t.Fatalf("expected %q to be removed from editor chrome, got:\n%s", unwanted, view)
		}
	}
	for _, want := range []string{"Monitor Layout", "Display", "Color", "Info"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected pane border title %q in view, got:\n%s", want, view)
		}
	}
}

func TestLayoutPanesReachTerminalEdges(t *testing.T) {
	m := Model{
		styles:      newStyles(),
		mode:        modeMain,
		tab:         tabLayout,
		layoutFocus: layoutFocusCanvas,
		width:       120,
		height:      36,
	}

	view := ansi.Strip(m.renderLayoutView(24))
	firstLine, _, _ := strings.Cut(view, "\n")
	if width := lipgloss.Width(firstLine); width != m.width {
		t.Fatalf("expected panes to fill terminal width %d, got %d", m.width, width)
	}
	if !strings.HasPrefix(firstLine, "╭") || !strings.HasSuffix(firstLine, "╮") {
		t.Fatalf("expected pane borders at both terminal edges, got %q", firstLine)
	}
}

func TestSplitPanesDoNotReserveGap(t *testing.T) {
	left, right := splitPaneWidths(120, 66, 18)
	if got := 120 - left - right; got != paneGapWidth {
		t.Fatalf("expected %d-column pane gap, got %d", paneGapWidth, got)
	}
}

func TestTabsUseSingleLineRailWithStatusAtRight(t *testing.T) {
	m := Model{styles: newStyles(), tab: tabLayout}
	tabs := ansi.Strip(m.renderTabs())
	if got := lipgloss.Height(tabs); got != 1 {
		t.Fatalf("expected a single navigation rail, got %d rows", got)
	}
	if !strings.Contains(tabs, "1 Layout") || !strings.Contains(tabs, "Current setup") || strings.Index(tabs, "Current setup") < lipgloss.Width(tabs)/2 {
		t.Fatalf("expected tabs at left and setup status at right, got %q", tabs)
	}
	if width := lipgloss.Width(tabs); width != m.terminalWidth() {
		t.Fatalf("expected navigation rail to fill width %d, got %d", m.terminalWidth(), width)
	}
}

func TestCurrentSetupIsPlainTopRailMetadata(t *testing.T) {
	m := Model{styles: newStyles(), tab: tabLayout, width: 120, height: 30, daemonOK: true}
	status := m.renderTopStatus()
	if ansi.Strip(status) != "Current setup" {
		t.Fatalf("expected neutral setup label, got %q", ansi.Strip(status))
	}
	if strings.Contains(status, "\x1b[48;") {
		t.Fatalf("expected Current setup without background color, got %q", status)
	}
}

func TestTopRailShowsDaemonFailureAndLinksToSetup(t *testing.T) {
	var openedURL string
	m := Model{
		styles: newStyles(),
		tab:    tabLayout,
		width:  160,
		height: 30,
		openURL: func(url string) error {
			openedURL = url
			return nil
		},
	}
	plain := ansi.Strip(m.renderTabs())
	start, found := visibleTextColumn(plain, "Daemon not running")
	if !found {
		t.Fatalf("expected daemon failure at top right, got %q", plain)
	}

	_, cmd := m.updateMouse(mousePressAt(start, m.appContentY()))
	if cmd == nil {
		t.Fatal("expected daemon failure click to open setup instructions")
	}
	msg := cmd()
	open, ok := msg.(openURLMsg)
	if !ok || open.url != daemonURL {
		t.Fatalf("expected daemon setup open command, got %#v", msg)
	}
	if openedURL != daemonURL {
		t.Fatalf("expected fake opener to receive %q, got %q", daemonURL, openedURL)
	}
}

func TestVisibleTextColumnUsesTerminalCellsForUnicodeRail(t *testing.T) {
	line := "─ 1 Layout ─ hyprmoncfg"
	got, ok := visibleTextColumn(line, "hyprmoncfg")
	if !ok {
		t.Fatal("expected label to be found")
	}
	want := lipgloss.Width("─ 1 Layout ─ ")
	if got != want {
		t.Fatalf("expected cell column %d, got byte-derived column %d", want, got)
	}
}

func TestRenderMainShowsFooterProjectLinks(t *testing.T) {
	prevVersion := buildinfo.Version
	buildinfo.Version = "1.2.3"
	defer func() { buildinfo.Version = prevVersion }()

	m := Model{
		styles:      newStyles(),
		mode:        modeMain,
		tab:         tabLayout,
		layoutFocus: layoutFocusInspector,
		width:       200,
		height:      30,
		editOutputs: []editableOutput{{
			Key:       "microstep|mpg321ur-qd",
			Name:      "DP-1",
			Enabled:   true,
			Modes:     []string{"3840x2160@143.99Hz"},
			ModeIndex: 0,
			Width:     3840,
			Height:    2160,
			Refresh:   143.99,
			Scale:     1,
		}},
	}

	view := m.renderMain()
	for _, want := range []string{"Ask", "Donate"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected footer to include %q, got:\n%s", want, view)
		}
	}
}

func TestNotifyUserRendersToastInMainView(t *testing.T) {
	m := Model{
		styles:          newStyles(),
		mode:            modeMain,
		tab:             tabProfiles,
		width:           120,
		height:          30,
		selectedProfile: 0,
		profiles:        []profile.Profile{{Name: "Desk Dock"}},
	}

	cmd := m.notifyUser("Post-apply failed", true)
	if cmd == nil {
		t.Fatal("expected notifyUser to return a clear command")
	}

	view := ansi.Strip(m.View())
	if !strings.Contains(view, "Post-apply failed") {
		t.Fatalf("expected toast message in rendered view, got:\n%s", view)
	}
}

func TestClearToastMsgRemovesToast(t *testing.T) {
	m := Model{
		styles: newStyles(),
		toast: &toastState{
			message: "Post-apply failed",
			err:     true,
			token:   3,
		},
	}

	updated, _ := m.Update(clearToastMsg{token: 3})
	got := updated.(Model)
	if got.toast != nil {
		t.Fatalf("expected toast to be cleared, got %+v", got.toast)
	}
}

func TestRenderFooterInfoIncludesVersion(t *testing.T) {
	prevVersion := buildinfo.Version
	buildinfo.Version = "1.2.3"
	defer func() { buildinfo.Version = prevVersion }()

	m := Model{styles: newStyles(), width: 120}
	info := m.renderFooterInfo(118)
	for _, want := range []string{"Ask", "Donate", "v1.2.3"} {
		if !strings.Contains(info, want) {
			t.Fatalf("expected footer info to include %q, got %q", want, info)
		}
	}
}

func TestRenderFooterBarFitsVersionWithinLineWidth(t *testing.T) {
	prevVersion := buildinfo.Version
	buildinfo.Version = "1.2.3"
	defer func() { buildinfo.Version = prevVersion }()

	m := Model{styles: newStyles(), width: 120}
	bar := m.renderFooterBar()
	if !strings.Contains(bar, "v1.2.3") {
		t.Fatalf("expected footer bar to include version, got %q", bar)
	}
	if width := lipgloss.Width(bar); width > m.footerContentWidth() {
		t.Fatalf("expected footer bar to fit width %d, got %d", m.footerContentWidth(), width)
	}
}

func TestRenderFooterInfoCollapsesToVersionOnNarrowWidth(t *testing.T) {
	prevVersion := buildinfo.Version
	buildinfo.Version = "1.2.3"
	defer func() { buildinfo.Version = prevVersion }()

	m := Model{styles: newStyles(), width: 32}

	info := m.renderFooterInfo(m.footerContentWidth())
	if info != "v1.2.3" {
		t.Fatalf("expected narrow footer info to collapse to version, got %q", info)
	}
}

func TestFooterLinkAtReturnsClickableRegionsOnly(t *testing.T) {
	prevVersion := buildinfo.Version
	buildinfo.Version = "1.2.3"
	defer func() { buildinfo.Version = prevVersion }()

	m := Model{styles: newStyles(), width: 160, height: 24}
	layout := m.footerLayout()
	if len(layout.links) < 3 {
		t.Fatalf("expected at least 3 clickable footer links, got %+v", layout.links)
	}

	// Find the Ask link: simulate a real click on the rendered footer,
	// which is shifted right by the badge padding added during decoration.
	var askFound bool
	for _, link := range layout.links {
		if link.label == "Ask" && link.url == communityURL {
			lx := m.footerColumnX() + link.start
			hit, ok := m.footerLinkAt(lx, m.footerRowY())
			if !ok || hit.label != "Ask" {
				t.Fatalf("expected Ask hit at x=%d, got ok=%v link=%+v", lx, ok, hit)
			}
			askFound = true
			break
		}
	}
	if !askFound {
		t.Fatalf("expected Ask link in footer, got %+v", layout.links)
	}
}

func TestFooterLinkAtMatchesVisibleFooterTextPosition(t *testing.T) {
	prevVersion := buildinfo.Version
	buildinfo.Version = "1.2.3"
	defer func() { buildinfo.Version = prevVersion }()

	m := Model{
		styles: newStyles(),
		width:  200,
		height: 24,
		tab:    tabLayout,
	}

	footer := m.renderFooterBar()
	for _, want := range []struct {
		label string
		url   string
	}{
		{label: "Ask", url: communityURL},
		{label: "Donate", url: sponsorURL},
	} {
		offset := strings.Index(footer, want.label)
		if offset < 0 {
			t.Fatalf("expected footer text to contain %q, got %q", want.label, footer)
		}

		x := m.footerColumnX() + offset
		hit, ok := m.footerLinkAt(x, m.footerRowY())
		if !ok {
			t.Fatalf("expected click on visible %q at x=%d to resolve to a link", want.label, x)
		}
		if hit.label != want.label || hit.url != want.url {
			t.Fatalf("expected %q link at x=%d, got %+v", want.label, x, hit)
		}
	}
}

func TestFooterClickRunsBrowserOpenCommand(t *testing.T) {
	prevVersion := buildinfo.Version
	buildinfo.Version = "1.2.3"
	defer func() { buildinfo.Version = prevVersion }()

	m := Model{
		styles: newStyles(),
		width:  200,
		height: 24,
		tab:    tabLayout,
	}

	layout := m.footerLayout()
	var donateFound bool
	for _, link := range layout.links {
		if link.label == "Donate" && link.url == sponsorURL {
			donateFound = true
			break
		}
	}
	if !donateFound {
		t.Fatalf("expected Donate link in footer, got %+v", layout.links)
	}
}

func TestOpenURLMsgSetsErrorStatus(t *testing.T) {
	m := Model{styles: newStyles()}

	updated, _ := m.Update(openURLMsg{label: "Ask", url: communityURL, err: errors.New("boom")})
	got := updated.(Model)
	if !got.statusErr {
		t.Fatal("expected failed open-url status to be marked as error")
	}
	if !strings.Contains(got.status, "Failed to open Ask link") {
		t.Fatalf("expected open-url failure in status, got %q", got.status)
	}
}

func TestProfilesMouseSelectsVisibleRow(t *testing.T) {
	m := Model{
		styles:   newStyles(),
		width:    120,
		height:   24,
		tab:      tabProfiles,
		profiles: []profile.Profile{testProfile("Laptop Home", 1), testProfile("Desk Dock", 2)},
	}

	x, y := findVisiblePosition(t, m.renderMain(), "Desk Dock")
	updated, _ := m.updateMouse(mousePressAt(x, y))
	got := updated.(Model)
	if got.selectedProfile != 1 {
		t.Fatalf("expected visible click on Desk Dock to select row 1, got %d", got.selectedProfile)
	}
}

func TestAutomaticProfileSelectionStillAllowsProfileBrowsing(t *testing.T) {
	m := Model{
		styles:          newStyles(),
		tab:             tabProfiles,
		daemonOK:        true,
		profiles:        []profile.Profile{testProfile("Laptop Home", 1), testProfile("Desk Dock", 2)},
		selectedProfile: 0,
	}

	updated, cmd := m.updateProfileKeys(tea.KeyMsg{Type: tea.KeyDown})
	got := updated.(Model)
	if cmd != nil || got.selectedProfile != 1 {
		t.Fatalf("automatic selection should allow browsing profile previews, got index %d", got.selectedProfile)
	}

	updated, cmd = got.updateProfileKeys(tea.KeyMsg{Type: tea.KeyEnter})
	previewing := updated.(Model)
	if cmd == nil || !previewing.applying {
		t.Fatalf("explicit use must start a preview, applying=%v cmd=%v", previewing.applying, cmd != nil)
	}
}

func TestProfileDeletionRequiresExplicitConfirmation(t *testing.T) {
	m := Model{styles: newStyles(), tab: tabProfiles, profiles: []profile.Profile{testProfile("Laptop", 1)}}
	updated, cmd := m.updateProfileKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	prompt := updated.(Model)
	if cmd != nil || prompt.mode != modeDeleteConfirm || prompt.deleteProfileName != "Laptop" {
		t.Fatal("delete must open confirmation without touching storage")
	}
	for _, key := range []tea.KeyMsg{{Type: tea.KeyEnter}, {Type: tea.KeyEsc}, {Type: tea.KeyRunes, Runes: []rune{'n'}}} {
		updated, cmd = prompt.Update(key)
		if cmd != nil || updated.(Model).mode != modeMain || updated.(Model).deleteProfileName != "" {
			t.Fatal("cancel must clear the pending deletion")
		}
	}
	updated, cmd = prompt.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if cmd == nil || updated.(Model).mode != modeMain {
		t.Fatal("explicit yes must schedule deletion")
	}
}

func TestProfilesMouseBrowsesWithoutApplyingInEitherSelectionMode(t *testing.T) {
	m := Model{
		styles:            newStyles(),
		width:             120,
		height:            24,
		tab:               tabProfiles,
		daemonOK:          true,
		activeProfileName: "Laptop Home",
		profiles:          []profile.Profile{testProfile("Laptop Home", 1), testProfile("Desk Dock", 2)},
	}

	x, y := findVisiblePosition(t, m.renderMain(), "Desk Dock")
	updated, cmd := m.updateMouse(mousePressAt(x, y))
	locked := updated.(Model)
	if cmd != nil || locked.selectedProfile != 1 || locked.applying {
		t.Fatalf("automatic selection should browse without applying, got index=%d applying=%v", locked.selectedProfile, locked.applying)
	}

	m.profileOverride = "Laptop Home"
	x, y = findVisiblePosition(t, m.renderMain(), "Desk Dock")
	updated, cmd = m.updateMouse(mousePressAt(x, y))
	manual := updated.(Model)
	if cmd != nil || manual.selectedProfile != 1 || manual.applying {
		t.Fatalf("manual profile click should browse without applying, got index=%d applying=%v cmd=%v", manual.selectedProfile, manual.applying, cmd != nil)
	}

	updated, cmd = manual.updateProfileKeys(tea.KeyMsg{Type: tea.KeyEnter})
	applying := updated.(Model)
	if cmd == nil || !applying.applying {
		t.Fatalf("manual Enter should begin safe apply, applying=%v cmd=%v", applying.applying, cmd != nil)
	}
}

func TestSpaceTogglesProfileSelectionModeThroughDaemon(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() {
		_ = clientConn.Close()
		_ = serverConn.Close()
	})
	m := Model{
		styles:   newStyles(),
		tab:      tabProfiles,
		daemonOK: true,
		ipc:      ipc.NewClient(clientConn),
	}

	updated, cmd := m.updateProfileKeys(tea.KeyMsg{Type: tea.KeySpace})
	got := updated.(Model)
	if cmd == nil || !got.profileModePending {
		t.Fatalf("expected automatic-selection toggle to start a daemon request, pending=%v cmd=%v", got.profileModePending, cmd != nil)
	}
}

func TestProfilesMouseIgnoresDetailsPaneInCompactLayout(t *testing.T) {
	m := Model{
		styles: newStyles(),
		width:  80,
		height: 24,
		tab:    tabProfiles,
		profiles: []profile.Profile{
			testProfile("Laptop Home", 1),
			testProfile("Desk Dock", 2),
			testProfile("Travel Dock", 1),
			testProfile("Office Desk", 2),
			testProfile("Studio", 1),
			testProfile("Projector", 1),
		},
		selectedProfile: 5,
	}

	x, y := findVisiblePosition(t, m.renderMain(), "Updated")
	updated, _ := m.updateMouse(mousePressAt(x, y))
	got := updated.(Model)
	if got.selectedProfile != 5 {
		t.Fatalf("expected compact details-pane click to keep selected profile 5, got %d", got.selectedProfile)
	}
}

func TestWorkspaceMouseSelectsVisibleField(t *testing.T) {
	m := Model{
		styles: newStyles(),
		width:  120,
		height: 24,
		tab:    tabWorkspaces,
		editOutputs: []editableOutput{
			{Key: "mon-a", Name: "DP-1", Enabled: true, Scale: 1},
			{Key: "mon-b", Name: "HDMI-A-1", Enabled: true, Scale: 1},
		},
		workspaceEdit: workspaceEditor{
			Enabled:       true,
			Strategy:      profile.WorkspaceStrategySequential,
			MaxWorkspaces: 6,
			GroupSize:     3,
			MonitorOrder:  []string{"mon-a", "mon-b"},
		},
	}

	x, y := findVisiblePosition(t, m.renderMain(), "Max workspaces")
	updated, _ := m.updateMouse(mousePressAt(x, y))
	got := updated.(Model)
	if got.workspaceEdit.SelectedField != 1 {
		t.Fatalf("expected visible click on Max workspaces to select field 1, got %d", got.workspaceEdit.SelectedField)
	}
	if got.workspaceEdit.MaxWorkspaces != 7 {
		t.Fatalf("expected click on Max workspaces to increment value to 7, got %d", got.workspaceEdit.MaxWorkspaces)
	}
}

func TestWorkspaceMouseIgnoresPreviewPaneInCompactLayout(t *testing.T) {
	m := Model{
		styles: newStyles(),
		width:  80,
		height: 24,
		tab:    tabWorkspaces,
		editOutputs: []editableOutput{
			{Key: "mon-a", Name: "DP-1", Enabled: true, Scale: 1},
			{Key: "mon-b", Name: "HDMI-A-1", Enabled: true, Scale: 1},
		},
		workspaceEdit: workspaceEditor{
			Enabled:       true,
			Strategy:      profile.WorkspaceStrategySequential,
			MaxWorkspaces: 6,
			GroupSize:     3,
			MonitorOrder:  []string{"mon-a", "mon-b"},
			SelectedField: 1,
		},
	}

	x, y := findVisiblePosition(t, m.renderMain(), "HDMI-A-1  4, 5, 6")
	updated, _ := m.updateMouse(mousePressAt(x, y))
	got := updated.(Model)
	if got.workspaceEdit.SelectedField != 1 {
		t.Fatalf("expected compact preview-pane click to keep selected field 1, got %d", got.workspaceEdit.SelectedField)
	}
	if got.workspaceEdit.MaxWorkspaces != 6 {
		t.Fatalf("expected compact preview-pane click to leave workspace settings unchanged, got %d", got.workspaceEdit.MaxWorkspaces)
	}
}

func TestSyncSelectionsPreservesWorkspaceOrderSelection(t *testing.T) {
	m := Model{
		styles: newStyles(),
		workspaceEdit: workspaceEditor{
			Enabled:       true,
			MonitorOrder:  []string{"mon-a", "mon-b"},
			SelectedField: len(workspaceFields) + 1,
			SelectedOrder: 1,
		},
	}

	m.syncSelections()

	if m.workspaceEdit.SelectedField != len(workspaceFields)+1 {
		t.Fatalf("expected monitor-order field selection to survive sync, got %d", m.workspaceEdit.SelectedField)
	}
	if m.workspaceEdit.SelectedOrder != 1 {
		t.Fatalf("expected selected order 1 to survive sync, got %d", m.workspaceEdit.SelectedOrder)
	}
}

func TestLayoutMouseOpensScaleEditorAtVisibleField(t *testing.T) {
	m := Model{
		styles:         newStyles(),
		width:          150,
		height:         28,
		tab:            tabLayout,
		layoutFocus:    layoutFocusCanvas,
		inspectorField: 0,
		editOutputs: []editableOutput{{
			Key:       "main",
			Name:      "DP-1",
			Enabled:   true,
			Modes:     []string{"3840x2160@143.99Hz", "2560x1440@143.97Hz"},
			ModeIndex: 0,
			Width:     3840,
			Height:    2160,
			Refresh:   143.99,
			Scale:     1.33,
		}},
	}

	x, y := findVisiblePosition(t, m.renderMain(), "Scale")
	updated, cmd := m.updateMouse(mousePressAt(x, y))
	if cmd != nil {
		if msg := cmd(); msg != nil {
			updated = runModelUpdate(t, updated, msg)
		}
	}
	got := mustModel(t, updated)
	if got.mode != modeNumericInput || got.input == nil || got.input.Kind != numericInputScale {
		t.Fatalf("expected visible click on Scale to open numeric scale editor, got mode=%v input=%+v", got.mode, got.input)
	}
}

func TestLayoutMouseOpensScaleEditorAtVisibleFieldInCompactLayout(t *testing.T) {
	m := Model{
		styles:         newStyles(),
		width:          90,
		height:         24,
		tab:            tabLayout,
		layoutFocus:    layoutFocusCanvas,
		inspectorField: 0,
		editOutputs: []editableOutput{{
			Key:       "main",
			Name:      "DP-1",
			Enabled:   true,
			Modes:     []string{"3840x2160@143.99Hz", "2560x1440@143.97Hz"},
			ModeIndex: 0,
			Width:     3840,
			Height:    2160,
			Refresh:   143.99,
			Scale:     1.33,
		}},
	}

	x, y := findVisiblePosition(t, m.renderMain(), "Scale")
	updated, cmd := m.updateMouse(mousePressAt(x, y))
	if cmd != nil {
		if msg := cmd(); msg != nil {
			updated = runModelUpdate(t, updated, msg)
		}
	}
	got := mustModel(t, updated)
	if got.mode != modeNumericInput || got.input == nil || got.input.Kind != numericInputScale {
		t.Fatalf("expected compact visible click on Scale to open numeric scale editor, got mode=%v input=%+v", got.mode, got.input)
	}
}

func TestCanvasPaneUsesEntireInteriorForCanvas(t *testing.T) {
	m := Model{
		styles: newStyles(),
		tab:    tabLayout,
		editOutputs: []editableOutput{{
			Name:    "DP-1",
			Enabled: true,
			Width:   3840,
			Height:  2160,
			Scale:   1,
		}},
	}

	view := ansi.Strip(m.renderCanvasPane(80, 12))
	for _, unwanted := range []string{"Legend", "Lid:", "Disabled:", "Mirrors:"} {
		if strings.Contains(view, unwanted) {
			t.Fatalf("expected canvas chrome %q to be removed, got:\n%s", unwanted, view)
		}
	}
	if !strings.Contains(view, "Monitor Layout") || !strings.Contains(view, "DP-1") {
		t.Fatalf("expected border title and monitor card, got:\n%s", view)
	}
}

func TestCanvasPaneShowsLidStateInBottomBorder(t *testing.T) {
	m := Model{styles: newStyles(), tab: tabLayout, lidState: lid.Open}
	view := ansi.Strip(m.renderCanvasPane(80, 12))
	lines := strings.Split(view, "\n")
	if !strings.Contains(lines[len(lines)-1], "Lid: open") {
		t.Fatalf("expected lid state in bottom border, got:\n%s", view)
	}
	for _, line := range lines[1 : len(lines)-1] {
		if strings.Contains(line, "Lid:") {
			t.Fatalf("expected no lid state inside canvas content, got:\n%s", view)
		}
	}

	m.lidState = lid.Unknown
	if view := ansi.Strip(m.renderCanvasPane(80, 12)); strings.Contains(view, "Lid:") {
		t.Fatalf("expected unknown lid state to stay hidden, got:\n%s", view)
	}
}

func TestCanvasShowsWarningsOnMonitorCards(t *testing.T) {
	m := Model{
		styles: newStyles(),
		tab:    tabLayout,
		editOutputs: []editableOutput{
			{
				Name:    "DP-1",
				Enabled: true,
				Width:   3840,
				Height:  2160,
				Scale:   1.33,
				X:       0,
				Y:       0,
			},
			{
				Name:    "eDP-1",
				Enabled: true,
				Width:   2880,
				Height:  1800,
				Scale:   1.67,
				X:       2887,
				Y:       546,
			},
		},
	}

	view := ansi.Strip(m.renderCanvas(100, 20))
	for _, want := range []string{"DP-1 ⚠", "eDP-1 ⚠"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected canvas warning marker %q, got:\n%s", want, view)
		}
	}
	if got := strings.Count(view, "⚠ fractional px"); got < 2 {
		t.Fatalf("expected both canvas cards to show fractional warnings, got %d:\n%s", got, view)
	}
}

func TestActivateInspectorFieldOpensEditors(t *testing.T) {
	base := Model{
		styles:      newStyles(),
		mode:        modeMain,
		tab:         tabLayout,
		layoutFocus: layoutFocusInspector,
		editOutputs: []editableOutput{{
			Name:      "DP-1",
			Enabled:   true,
			Modes:     []string{"3840x2160@143.99Hz", "2560x1440@143.97Hz"},
			ModeIndex: 0,
			Scale:     1.33,
		}},
	}

	base.activateInspectorField()
	if base.mode != modeMain {
		t.Fatalf("enabled row should toggle inline, got mode %v", base.mode)
	}

	base.inspectorField = 1
	base.activateInspectorField()
	if base.mode != modeModePicker || base.picker == nil {
		t.Fatalf("expected mode picker to open, got mode %v picker %+v", base.mode, base.picker)
	}

	base.inspectorField = 2
	base.activateInspectorField()
	if base.mode != modeNumericInput || base.input == nil {
		t.Fatalf("expected numeric input to open, got mode %v input %+v", base.mode, base.input)
	}

	base.inspectorField = 7
	base.activateInspectorField()
	if base.mode != modeNumericInput || base.input == nil || base.input.Kind != numericInputPositionX {
		t.Fatalf("expected position X input to open, got mode %v input %+v", base.mode, base.input)
	}

	base.inspectorField = 8
	base.activateInspectorField()
	if base.mode != modeNumericInput || base.input == nil || base.input.Kind != numericInputPositionY {
		t.Fatalf("expected position Y input to open, got mode %v input %+v", base.mode, base.input)
	}
}

func TestScaleInputShowsClosestSharpSuggestion(t *testing.T) {
	m := Model{
		styles: newStyles(),
		width:  120,
		height: 28,
		mode:   modeNumericInput,
		input: &numericInputState{
			Kind:        numericInputScale,
			OutputIndex: 0,
			Title:       "Set Scale for DP-1",
			Hint:        "Scale hint",
			Input:       textInputWithValue("1.33"),
		},
		editOutputs: []editableOutput{{
			Name:    "DP-1",
			Enabled: true,
			Width:   3840,
			Height:  2160,
			Scale:   1,
		}},
	}

	view := ansi.Strip(m.renderNumericInput())
	for _, want := range []string{
		"⚠ Not sharp:",
		"3840 / 1.33 = 2887.22",
		"2160 / 1.33 = 1624.06",
		"fractional px",
		"Closest sharp: 1.33333 -> 2880 x 1620 logical px",
		"Enter applies it",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected scale input feedback to include %q, got:\n%s", want, view)
		}
	}
}

func TestInspectorHighlightsNonSharpScale(t *testing.T) {
	m := Model{
		styles:         newStyles(),
		tab:            tabLayout,
		layoutFocus:    layoutFocusInspector,
		inspectorField: 2,
		editOutputs: []editableOutput{{
			Name:      "DP-1",
			Enabled:   true,
			Modes:     []string{"3840x2160@143.99Hz"},
			ModeIndex: 0,
			Width:     3840,
			Height:    2160,
			Refresh:   143.99,
			Scale:     1.33,
		}},
	}

	view := ansi.Strip(m.renderInspectorPane(64, 30, false))
	for _, want := range []string{"Scale", "1.33", "⚠ fractional px"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected inspector warning to include %q, got:\n%s", want, view)
		}
	}
}

func TestInspectorFieldWarningsCoverInvalidValues(t *testing.T) {
	m := Model{
		editOutputs: []editableOutput{{
			Key:             "dp-1",
			Name:            "DP-1",
			Enabled:         true,
			ModeUnsupported: true,
			Bitdepth:        12,
			MirrorOf:        "missing",
			Scale:           1,
		}},
	}
	output := m.editOutputs[0]

	for _, tt := range []struct {
		field int
		want  string
	}{
		{field: 1, want: "unsupported"},
		{field: 3, want: "invalid"},
		{field: 9, want: "missing target"},
	} {
		got, ok := m.layoutFieldIssue(output, tt.field)
		if !ok || got != tt.want {
			t.Fatalf("field %d issue = %q, %v; want %q, true", tt.field, got, ok, tt.want)
		}
	}
}

func TestCommitScaleInputAppliesShownSharpSuggestion(t *testing.T) {
	m := Model{
		styles: newStyles(),
		mode:   modeNumericInput,
		input: &numericInputState{
			Kind:        numericInputScale,
			OutputIndex: 0,
			Input:       textInputWithValue("1.33"),
		},
		editOutputs: []editableOutput{{
			Name:    "DP-1",
			Enabled: true,
			Width:   3840,
			Height:  2160,
			Scale:   1,
		}},
	}

	cmd := m.commitNumericInput()
	if cmd != nil {
		t.Fatal("expected scale commit not to return a command")
	}
	if got := m.editOutputs[0].Scale; got != 1.33333 {
		t.Fatalf("expected 1.33 scale input to apply shown 1.33333 suggestion, got %v", got)
	}
	if !strings.Contains(m.status, "closest sharp scale for 1.33") {
		t.Fatalf("expected status to explain sharp suggestion, got %q", m.status)
	}
}

func TestScaleInputShowsInvalidIncompleteScale(t *testing.T) {
	m := Model{
		styles: newStyles(),
		width:  120,
		height: 28,
		mode:   modeNumericInput,
		input: &numericInputState{
			Kind:        numericInputScale,
			OutputIndex: 0,
			Title:       "Set Scale for DP-1",
			Hint:        "Scale hint",
			Input:       textInputWithValue("1."),
		},
		editOutputs: []editableOutput{{
			Name:    "DP-1",
			Enabled: true,
			Width:   3840,
			Height:  2160,
			Scale:   1,
		}},
	}

	view := ansi.Strip(m.renderNumericInput())
	if !strings.Contains(view, "scale must be a number") {
		t.Fatalf("expected incomplete scale to render validation feedback, got:\n%s", view)
	}
}

func TestModePickerMouseSelectsVisibleMode(t *testing.T) {
	base := Model{
		styles:         newStyles(),
		width:          120,
		height:         28,
		tab:            tabLayout,
		layoutFocus:    layoutFocusInspector,
		inspectorField: 1,
		editOutputs: []editableOutput{{
			Key:       "main",
			Name:      "DP-1",
			Enabled:   true,
			Modes:     []string{"3840x2160@143.99Hz", "2560x1440@143.97Hz"},
			ModeIndex: 0,
			Width:     3840,
			Height:    2160,
			Refresh:   143.99,
			Scale:     1.33,
		}},
	}

	base.activateInspectorField()
	if base.mode != modeModePicker || base.picker == nil {
		t.Fatalf("expected mode picker to be active, got mode=%v picker=%+v", base.mode, base.picker)
	}
	m := base

	x, y := findVisiblePosition(t, m.View(), "2560x1440@144Hz")
	updated, cmd := m.updateMouse(mousePressAt(x, y))
	if cmd != nil {
		if msg := cmd(); msg != nil {
			updated = runModelUpdate(t, updated, msg)
		}
	}
	got := updated.(*Model)
	if got.mode != modeMain {
		t.Fatalf("expected mode picker click to close dialog, got mode %v", got.mode)
	}
	if got.editOutputs[0].ModeIndex != 1 {
		t.Fatalf("expected mode picker click to select second mode, got index %d", got.editOutputs[0].ModeIndex)
	}
}

func TestModePickerRendersOptionsWithoutBlankRows(t *testing.T) {
	m := Model{
		styles:         newStyles(),
		width:          120,
		height:         28,
		tab:            tabLayout,
		layoutFocus:    layoutFocusInspector,
		inspectorField: 1,
		editOutputs: []editableOutput{{
			Name:      "DP-1",
			Enabled:   true,
			Modes:     []string{"3840x2160@143.99Hz", "2560x1440@143.97Hz"},
			ModeIndex: 0,
			Scale:     1,
		}},
	}

	m.activateInspectorField()
	if m.picker == nil {
		t.Fatal("expected mode picker to be active")
	}

	lines := strings.Split(ansi.Strip(m.picker.List.View()), "\n")
	first, second := -1, -1
	for i, line := range lines {
		if strings.Contains(line, "3840x2160@144Hz") {
			first = i
		}
		if strings.Contains(line, "2560x1440@144Hz") {
			second = i
		}
	}
	if first < 0 || second < 0 {
		t.Fatalf("expected both mode options in picker view, got:\n%s", strings.Join(lines, "\n"))
	}
	if second != first+1 {
		t.Fatalf("expected mode options on adjacent rows, got rows %d and %d:\n%s", first, second, strings.Join(lines, "\n"))
	}
}

func TestCanvasLayoutPreservesWideMonitorAspect(t *testing.T) {
	m := Model{
		editOutputs: []editableOutput{
			{
				Name:    "DP-1",
				Enabled: true,
				Width:   3840,
				Height:  2160,
				Scale:   1,
				X:       0,
				Y:       0,
			},
		},
	}

	layout := m.canvasLayout(90, 24)
	if !layout.ok || len(layout.rects) != 1 {
		t.Fatalf("expected one visible rect, got %+v", layout)
	}

	rect := layout.rects[0]
	physicalRatio := float64(rect.w) / (float64(rect.h) * layout.cellW)
	if physicalRatio < 1.6 || physicalRatio > 1.95 {
		t.Fatalf("expected wide physical ratio near 16:9, got %.2f (rect=%+v cellW=%.2f)", physicalRatio, rect, layout.cellW)
	}
}

func TestCanvasLayoutSkipsMirroredOutputs(t *testing.T) {
	m := Model{
		editOutputs: []editableOutput{
			{
				Key:      "main",
				Name:     "DP-1",
				Enabled:  true,
				Width:    3840,
				Height:   2160,
				Scale:    1,
				X:        0,
				Y:        0,
				MirrorOf: "",
			},
			{
				Key:      "mirror",
				Name:     "HDMI-A-1",
				Enabled:  true,
				Width:    1920,
				Height:   1080,
				Scale:    1,
				X:        0,
				Y:        0,
				MirrorOf: "main",
			},
		},
	}

	layout := m.canvasLayout(90, 24)
	if !layout.ok || len(layout.rects) != 1 {
		t.Fatalf("expected mirrored output to be omitted from canvas rects, got %+v", layout)
	}
	if got := m.editOutputs[layout.rects[0].index].Name; got != "DP-1" {
		t.Fatalf("expected only independent monitor rect, got %q", got)
	}
}

func TestRenderCanvasPaneDoesNotSpendCanvasRowsOnMirrorSummary(t *testing.T) {
	m := Model{
		styles: newStyles(),
		editOutputs: []editableOutput{
			{
				Key:     "main",
				Name:    "DP-1",
				Enabled: true,
				Width:   3840,
				Height:  2160,
				Scale:   1,
			},
			{
				Key:      "mirror",
				Name:     "HDMI-A-1",
				Enabled:  true,
				Width:    1920,
				Height:   1080,
				Scale:    1,
				MirrorOf: "main",
			},
		},
	}

	view := ansi.Strip(m.renderCanvasPane(80, 12))
	if strings.Contains(view, "Mirrors:") {
		t.Fatalf("expected mirror summary to stay out of the canvas, got:\n%s", view)
	}
	if !strings.Contains(view, "DP-1") {
		t.Fatalf("expected independent monitor to remain visible, got:\n%s", view)
	}
}

func TestCardLinesShowMakeModelAndPosition(t *testing.T) {
	output := editableOutput{
		Name:   "DP-1",
		Make:   "Microstep",
		Model:  "MPG321UR-QD",
		Width:  3840,
		Height: 2160,
		Scale:  1.33,
		X:      0,
		Y:      0,
	}

	lines := output.cardLines(5, "", "")
	if len(lines) != 4 {
		t.Fatalf("expected 4 card lines, got %d", len(lines))
	}
	if lines[1].text != "Microstep MPG321UR-QD" {
		t.Fatalf("expected make+model on card, got %q", lines[1].text)
	}
	if lines[3].text != "Scale 1.33x  Position 0,0" {
		t.Fatalf("expected scale and position on card, got %q", lines[3].text)
	}
}

func TestCardLinesPrioritizeWarningWhenPresent(t *testing.T) {
	output := editableOutput{
		Name:   "DP-1",
		Width:  3840,
		Height: 2160,
		Scale:  1.33,
	}

	lines := output.cardLinesWithIssue(3, "", "", "fractional px", "3")
	if len(lines) != 3 {
		t.Fatalf("expected 3 card lines, got %d", len(lines))
	}
	if lines[0].text != "DP-1 ⚠" {
		t.Fatalf("expected connector warning marker, got %q", lines[0].text)
	}
	if lines[1].text != "⚠ fractional px" {
		t.Fatalf("expected warning line, got %q", lines[1].text)
	}
}

func TestOpenSaveDialogShowsExistingProfiles(t *testing.T) {
	m := Model{
		styles:   newStyles(),
		height:   30,
		profiles: []profile.Profile{{Name: "Laptop Home"}, {Name: "Desk Dock"}},
	}

	updatedModel, _ := m.openSaveDialog()
	got := updatedModel.(*Model)
	if got.saveDialog == nil {
		t.Fatal("expected save dialog to be initialized")
	}
	if len(got.saveDialog.List.Items()) != 2 {
		t.Fatalf("expected 2 visible profiles, got %d", len(got.saveDialog.List.Items()))
	}
}

func TestOpenSaveDialogPrefillsCurrentDraftProfileName(t *testing.T) {
	m := Model{
		styles:           newStyles(),
		height:           30,
		draftProfileName: "Desk Dock",
		profiles:         []profile.Profile{{Name: "Laptop Home"}, {Name: "Desk Dock"}},
	}

	updatedModel, _ := m.openSaveDialog()
	got := updatedModel.(*Model)
	if got.saveDialog == nil {
		t.Fatal("expected save dialog to be initialized")
	}
	if got.saveDialog.Input.Value() != "Desk Dock" {
		t.Fatalf("expected save dialog to prefill current draft profile name, got %q", got.saveDialog.Input.Value())
	}
	if len(got.saveDialog.List.Items()) != 2 {
		t.Fatalf("expected prefilled save dialog to keep the full profile list visible, got %d items", len(got.saveDialog.List.Items()))
	}
	if got.saveDialog.Action != saveActionApply {
		t.Fatalf("expected save dialog to default to Save & Apply, got %v", got.saveDialog.Action)
	}
}

func TestOpenSaveDialogPrefillsMatchedProfileNameAndMovesItToTop(t *testing.T) {
	m := Model{
		styles:             newStyles(),
		height:             30,
		matchedProfileName: "Laptop Home",
		profiles:           []profile.Profile{{Name: "Desk Dock"}, {Name: "Laptop Home"}},
	}

	updatedModel, _ := m.openSaveDialog()
	got := updatedModel.(*Model)
	if got.saveDialog == nil {
		t.Fatal("expected save dialog to be initialized")
	}
	if got.saveDialog.Input.Value() != "Laptop Home" {
		t.Fatalf("expected save dialog to prefill matched profile name, got %q", got.saveDialog.Input.Value())
	}
	if len(got.saveDialog.List.Items()) != 2 {
		t.Fatalf("expected 2 visible profiles, got %d", len(got.saveDialog.List.Items()))
	}
	first, ok := got.saveDialog.List.Items()[0].(profileListItem)
	if !ok {
		t.Fatalf("expected first save-list item to be a profileListItem, got %T", got.saveDialog.List.Items()[0])
	}
	if first.name != "Laptop Home" {
		t.Fatalf("expected matched profile to be first in save dialog, got %q", first.name)
	}
	if got.saveDialog.List.Index() != 0 {
		t.Fatalf("expected matched profile to be selected, got index %d", got.saveDialog.List.Index())
	}
}

func TestLoadLiveStateInfersDraftProfileNameFromExactCurrentProfile(t *testing.T) {
	monitors := []hypr.Monitor{{
		Name:        "DP-1",
		Make:        "Dell",
		Model:       "U2720Q",
		Serial:      "A1",
		Width:       2560,
		Height:      1440,
		RefreshRate: 144,
		X:           0,
		Y:           0,
		Scale:       1,
		Focused:     true,
		DPMSStatus:  true,
	}}

	m := Model{
		styles:   newStyles(),
		monitors: monitors,
		profiles: []profile.Profile{profile.FromState("Desk Dock", monitors, nil)},
	}

	m.loadLiveState()

	if m.draftProfileName != "Desk Dock" {
		t.Fatalf("expected live state to infer current profile name, got %q", m.draftProfileName)
	}
}

func TestLoadLiveStateRecoversPreciseScaleFromRoundedHyprlandReadback(t *testing.T) {
	monitors := []hypr.Monitor{{
		Name:                  "DP-1",
		Make:                  "Microstep",
		Model:                 "MPG321UR-QD",
		Serial:                "A1",
		Width:                 3840,
		Height:                2160,
		RefreshRate:           144,
		X:                     0,
		Y:                     0,
		Scale:                 1.33,
		CurrentFormat:         "XRGB8888",
		ColorManagementPreset: "srgb",
		SDRBrightness:         1,
		SDRSaturation:         1,
		Focused:               true,
		DPMSStatus:            true,
	}}
	saved := profile.FromState("Laptop Home", []hypr.Monitor{{
		Name:        "DP-1",
		Make:        "Microstep",
		Model:       "MPG321UR-QD",
		Serial:      "A1",
		Width:       3840,
		Height:      2160,
		RefreshRate: 144,
		X:           0,
		Y:           0,
		Scale:       1.33333,
	}}, nil)
	saved.Outputs[0].Bitdepth = 0
	saved.Outputs[0].CM = ""
	saved.Outputs[0].SDRBrightness = 0
	saved.Outputs[0].SDRSaturation = 0

	m := Model{
		styles:   newStyles(),
		monitors: monitors,
		profiles: []profile.Profile{saved},
	}

	m.loadLiveState()

	if m.draftProfileName != "Laptop Home" {
		t.Fatalf("expected rounded live readback to match profile, got %q", m.draftProfileName)
	}
	if got := m.editOutputs[0].Scale; got != 1.33333 {
		t.Fatalf("expected editor to recover precise saved scale, got %v", got)
	}
	if issue, ok := m.layoutFieldIssue(m.editOutputs[0], 2); ok {
		t.Fatalf("expected recovered precise scale to clear warning, got %q", issue)
	}
}

func TestLoadLiveStateKeepsBestMatchForSaveDialogWhenStateDiffers(t *testing.T) {
	monitors := []hypr.Monitor{{
		Name:        "DP-1",
		Make:        "Microstep",
		Model:       "MPG321UR-QD",
		Serial:      "A1",
		Width:       3840,
		Height:      2160,
		RefreshRate: 144,
		X:           100,
		Scale:       1.33,
		Focused:     true,
		DPMSStatus:  true,
	}}
	prof := profile.FromState("Laptop Home", []hypr.Monitor{{
		Name:        "DP-1",
		Make:        "Microstep",
		Model:       "MPG321UR-QD",
		Serial:      "A1",
		Width:       3840,
		Height:      2160,
		RefreshRate: 144,
		X:           0,
		Scale:       1.33333,
	}}, nil)

	m := Model{
		styles:   newStyles(),
		monitors: monitors,
		profiles: []profile.Profile{prof},
	}

	m.loadLiveState()

	if m.draftProfileName != "" {
		t.Fatalf("expected shifted live state not to be exact draft match, got %q", m.draftProfileName)
	}
	if m.matchedProfileName != "Laptop Home" {
		t.Fatalf("expected best hardware match to be remembered, got %q", m.matchedProfileName)
	}

	updatedModel, _ := m.openSaveDialog()
	got := updatedModel.(*Model)
	if got.saveDialog.Input.Value() != "Laptop Home" {
		t.Fatalf("expected save dialog to fall back to best matched profile, got %q", got.saveDialog.Input.Value())
	}
}

func TestProfileExecEditorOpensWithCurrentValue(t *testing.T) {
	m := Model{
		styles:   newStyles(),
		tab:      tabProfiles,
		profiles: []profile.Profile{{Name: "Desk Dock", Exec: "/path/to/script.sh"}},
	}

	updatedModel, cmd := m.updateProfileKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	got := updatedModel.(Model)

	if cmd == nil {
		t.Fatal("expected exec editor to focus its input")
	}
	if got.mode != modeProfileExecInput {
		t.Fatalf("expected exec editor mode, got %v", got.mode)
	}
	if got.execInput == nil {
		t.Fatal("expected exec input state to be initialized")
	}
	if got.execInput.Input.Value() != "/path/to/script.sh" {
		t.Fatalf("expected exec editor to prefill current value, got %q", got.execInput.Input.Value())
	}
}

func TestProfileExecEditorEnterSavesExecutableCommand(t *testing.T) {
	store := profile.NewStore(t.TempDir())
	savedProfile := testProfile("Desk Dock", 1)
	if err := store.Save(savedProfile); err != nil {
		t.Fatalf("save initial profile: %v", err)
	}
	execPath := filepath.Join(t.TempDir(), "post-apply.sh")
	if err := os.WriteFile(execPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write executable script: %v", err)
	}

	m := Model{
		styles:   newStyles(),
		tab:      tabProfiles,
		store:    store,
		profiles: []profile.Profile{savedProfile},
	}

	updatedModel, _ := m.updateProfileKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	got := updatedModel.(Model)
	got.execInput.Input.SetValue(execPath)

	nextModel, cmd := got.updateProfileExecInputKeys(tea.KeyMsg{Type: tea.KeyEnter})
	next := nextModel.(*Model)
	if cmd == nil {
		t.Fatal("expected exec editor to return an auto-save command")
	}

	if next.mode != modeMain {
		t.Fatalf("expected exec editor to close after Enter, got %v", next.mode)
	}
	if next.execInput != nil {
		t.Fatalf("expected exec input to be cleared, got %+v", next.execInput)
	}
	if next.profiles[0].Exec != execPath {
		t.Fatalf("expected exec to update in memory, got %q", next.profiles[0].Exec)
	}
	msg := cmd()
	if _, ok := msg.(saveMsg); !ok {
		t.Fatalf("expected saveMsg from auto-save command, got %T", msg)
	}
	saved, err := store.Load("Desk Dock")
	if err != nil {
		t.Fatalf("load saved profile: %v", err)
	}
	if saved.Exec != execPath {
		t.Fatalf("expected exec to be saved, got %q", saved.Exec)
	}
}

func TestProfileExecEditorRejectsNonExecutablePath(t *testing.T) {
	longDir := filepath.Join(t.TempDir(), strings.Repeat("very-long-build-path-", 7))
	if err := os.MkdirAll(longDir, 0o755); err != nil {
		t.Fatalf("create long temp path: %v", err)
	}
	execPath := filepath.Join(longDir, "post-apply.sh")
	if err := os.WriteFile(execPath, []byte("#!/bin/sh\nexit 0\n"), 0o644); err != nil {
		t.Fatalf("write non-executable script: %v", err)
	}

	m := Model{
		styles:   newStyles(),
		tab:      tabProfiles,
		profiles: []profile.Profile{{Name: "Desk Dock"}},
	}

	updatedModel, _ := m.updateProfileKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	got := updatedModel.(Model)
	got.execInput.Input.SetValue(execPath)

	nextModel, cmd := got.updateProfileExecInputKeys(tea.KeyMsg{Type: tea.KeyEnter})
	next := nextModel.(*Model)
	if cmd != nil {
		t.Fatal("expected invalid exec to skip save command")
	}
	if next.mode != modeProfileExecInput {
		t.Fatalf("expected exec editor to stay open, got %v", next.mode)
	}
	if next.execInput == nil || next.execInput.Err == nil {
		t.Fatal("expected exec editor to retain validation error")
	}
	if !strings.Contains(next.execInput.Err.Error(), "not executable") {
		t.Fatalf("expected validation error to mention executable bit, got %v", next.execInput.Err)
	}
	if next.profiles[0].Exec != "" {
		t.Fatalf("expected invalid exec not to update profile, got %q", next.profiles[0].Exec)
	}
	view := ansi.Strip(next.View())
	if !strings.Contains(view, "not ex") {
		t.Fatalf("expected validation error in modal, got:\n%s", view)
	}
}

func TestProfileExecEditorEscDiscardsChange(t *testing.T) {
	m := Model{
		styles:   newStyles(),
		tab:      tabProfiles,
		profiles: []profile.Profile{{Name: "Desk Dock", Exec: "/path/to/script.sh"}},
	}

	updatedModel, _ := m.updateProfileKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	got := updatedModel.(Model)
	got.execInput.Input.SetValue("/other/script.sh")

	nextModel, _ := got.updateProfileExecInputKeys(tea.KeyMsg{Type: tea.KeyEsc})
	next := nextModel.(*Model)

	if next.profiles[0].Exec != "/path/to/script.sh" {
		t.Fatalf("expected Esc to discard changes, got %q", next.profiles[0].Exec)
	}
	if next.mode != modeMain {
		t.Fatalf("expected exec editor to close on Esc, got %v", next.mode)
	}
}

func TestProfilesTabSavePersistsSelectedProfileExec(t *testing.T) {
	store := profile.NewStore(t.TempDir())
	savedProfile := testProfile("Desk Dock", 1)
	savedProfile.Exec = "/path/to/script.sh"
	m := Model{
		styles:           newStyles(),
		tab:              tabProfiles,
		store:            store,
		profiles:         []profile.Profile{savedProfile},
		draftProfileName: "Draft Name",
	}

	updatedModel, cmd := m.updateMainKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	got := updatedModel.(Model)
	if cmd == nil {
		t.Fatal("expected Profiles-tab save to return a command")
	}

	msg := cmd()
	finalModel, _ := got.Update(msg)
	final := finalModel.(Model)

	saved, err := store.Load("Desk Dock")
	if err != nil {
		t.Fatalf("expected selected profile to be saved: %v", err)
	}
	if saved.Exec != savedProfile.Exec {
		t.Fatalf("expected saved exec %q, got %q", savedProfile.Exec, saved.Exec)
	}
	if final.draftProfileName != "Draft Name" {
		t.Fatalf("expected Profiles-tab save not to rewrite draft name, got %q", final.draftProfileName)
	}
}

func TestSaveDialogMouseSelectsVisibleProfile(t *testing.T) {
	m := Model{
		styles:   newStyles(),
		width:    120,
		height:   28,
		profiles: []profile.Profile{testProfile("Laptop Home", 1), testProfile("Desk Dock", 2)},
	}

	updatedModel, _ := m.openSaveDialog()
	got := updatedModel.(*Model)

	x, y := findVisiblePosition(t, got.View(), "Desk Dock")
	updated, _ := got.updateMouse(mousePressAt(x, y))
	next := updated.(Model)
	if next.saveDialog == nil {
		t.Fatal("expected save dialog to remain open after profile click")
	}
	if next.saveDialog.List.Index() != 1 {
		t.Fatalf("expected visible click on Desk Dock to select row 1, got %d", next.saveDialog.List.Index())
	}
	if next.saveDialog.Input.Value() != "Desk Dock" {
		t.Fatalf("expected save dialog click to sync name input to Desk Dock, got %q", next.saveDialog.Input.Value())
	}
}

func TestSaveDialogAllowsJKInProfileName(t *testing.T) {
	m := Model{
		styles:   newStyles(),
		height:   30,
		profiles: []profile.Profile{{Name: "Laptop Home"}, {Name: "Desk Dock"}},
	}

	updatedModel, _ := m.openSaveDialog()
	got := updatedModel.(*Model)
	got.saveDialog.Input.SetValue("")
	got.saveDialog.Filter = ""
	got.rebuildSaveList(false)

	for _, r := range "desk job" {
		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
		nextModel, _ := got.updateSaveKeys(msg)
		next := nextModel.(Model)
		got = &next
	}

	if value := got.saveDialog.Input.Value(); value != "desk job" {
		t.Fatalf("expected typed name to include j/k, got %q", value)
	}
	if got.saveDialog.Filter != "desk job" {
		t.Fatalf("expected filter to track typed name, got %q", got.saveDialog.Filter)
	}
}

func TestSaveMarksDraftAsSavedWithoutDiscardingEditorState(t *testing.T) {
	m := Model{
		styles: newStyles(),
		mode:   modeSave,
		dirty:  true,
	}

	updatedModel, _ := m.Update(saveMsg{name: "Desk Home"})
	got := updatedModel.(Model)

	if !got.dirty {
		t.Fatal("expected saved draft to remain editable")
	}
	if !got.draftSaved {
		t.Fatal("expected draft to be marked as saved")
	}
	if !strings.Contains(got.unsavedBadge(), "Saved Draft") {
		t.Fatalf("expected badge to show saved draft, got %q", got.unsavedBadge())
	}
}

func TestSaveDialogTabCyclesExplicitActions(t *testing.T) {
	m := Model{
		styles: newStyles(),
		height: 30,
	}

	updatedModel, _ := m.openSaveDialog()
	got := updatedModel.(*Model)

	nextModel, _ := got.updateSaveKeys(tea.KeyMsg{Type: tea.KeyTab})
	next := nextModel.(Model)
	if next.saveDialog == nil {
		t.Fatal("expected save dialog to remain open")
	}
	if next.saveDialog.Action != saveActionCancel {
		t.Fatalf("expected Tab to cycle Save & Apply to Cancel, got %v", next.saveDialog.Action)
	}

	backModel, _ := next.updateSaveKeys(tea.KeyMsg{Type: tea.KeyShiftTab})
	back := backModel.(Model)
	if back.saveDialog == nil {
		t.Fatal("expected save dialog to remain open after Shift+Tab")
	}
	if back.saveDialog.Action != saveActionApply {
		t.Fatalf("expected Shift+Tab to cycle back to Save & Apply, got %v", back.saveDialog.Action)
	}
}

func TestSaveDialogArrowKeysCycleExplicitActions(t *testing.T) {
	for _, tc := range []struct {
		name    string
		purpose saveDialogPurpose
		start   saveAction
		key     tea.KeyType
		want    saveAction
	}{
		{
			name:    "right advances profile action",
			purpose: saveDialogProfile,
			start:   saveActionApply,
			key:     tea.KeyRight,
			want:    saveActionCancel,
		},
		{
			name:    "left wraps profile action",
			purpose: saveDialogProfile,
			start:   saveActionOnly,
			key:     tea.KeyLeft,
			want:    saveActionCancel,
		},
		{
			name:    "right advances quit action",
			purpose: saveDialogQuit,
			start:   saveActionSaveQuit,
			key:     tea.KeyRight,
			want:    saveActionDiscardQuit,
		},
		{
			name:    "left wraps quit action",
			purpose: saveDialogQuit,
			start:   saveActionSaveQuit,
			key:     tea.KeyLeft,
			want:    saveActionCancel,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := Model{
				styles: newStyles(),
				mode:   modeSave,
				saveDialog: &saveDialogState{
					Action:  tc.start,
					Purpose: tc.purpose,
				},
			}

			updatedModel, _ := m.updateSaveKeys(tea.KeyMsg{Type: tc.key})
			got := updatedModel.(Model)
			if got.saveDialog == nil {
				t.Fatal("expected save dialog to remain open")
			}
			if got.saveDialog.Action != tc.want {
				t.Fatalf("expected action %v, got %v", tc.want, got.saveDialog.Action)
			}
		})
	}
}

func TestDirtyQuitOpensSaveBeforeQuitDialog(t *testing.T) {
	m := Model{
		styles:           newStyles(),
		mode:             modeMain,
		dirty:            true,
		height:           30,
		draftProfileName: "Laptop Home",
		profiles:         []profile.Profile{{Name: "Laptop Home"}, {Name: "Desk Dock"}},
	}

	updatedModel, _ := m.updateMainKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	got := updatedModel.(*Model)
	if got.mode != modeSave || got.saveDialog == nil {
		t.Fatalf("expected dirty quit to open save dialog, mode=%v dialog=%+v", got.mode, got.saveDialog)
	}
	if got.saveDialog.Purpose != saveDialogQuit {
		t.Fatalf("expected quit save dialog purpose, got %v", got.saveDialog.Purpose)
	}
	if got.saveDialog.Action != saveActionSaveQuit {
		t.Fatalf("expected quit dialog to default to Save, Apply & Quit, got %v", got.saveDialog.Action)
	}
	if got.saveDialog.Input.Value() != "Laptop Home" {
		t.Fatalf("expected quit dialog to reuse save suggestion, got %q", got.saveDialog.Input.Value())
	}

	view := got.View()
	for _, want := range []string{"Save Before Quitting", "Save, Apply & Quit", "Quit Without Saving", "Cancel"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected quit dialog to include %q, got:\n%s", want, view)
		}
	}
}

func TestQuitSaveDialogDiscardQuitsWithoutSaving(t *testing.T) {
	m := Model{
		styles: newStyles(),
		mode:   modeSave,
		saveDialog: &saveDialogState{
			Action:  saveActionDiscardQuit,
			Purpose: saveDialogQuit,
		},
	}

	_, cmd := m.updateSaveKeys(tea.KeyMsg{Type: tea.KeyEnter})
	assertQuitCmd(t, cmd)
}

func TestSaveMsgWithQuitActionAppliesBeforeQuit(t *testing.T) {
	m := Model{
		styles: newStyles(),
		mode:   modeSave,
		dirty:  true,
		saveDialog: &saveDialogState{
			Action:  saveActionSaveQuit,
			Purpose: saveDialogQuit,
		},
	}

	updatedModel, cmd := m.Update(saveMsg{name: "Desk Home"})
	got := updatedModel.(Model)
	if got.mode != modeMain {
		t.Fatalf("expected saved quit to return to main mode before applying, got %v", got.mode)
	}
	if got.saveDialog != nil {
		t.Fatalf("expected saved quit to clear dialog, got %+v", got.saveDialog)
	}
	if !got.quitAfterApply {
		t.Fatal("expected saved quit to mark quit after apply")
	}
	if cmd == nil {
		t.Fatal("expected saved quit to start apply command")
	}
}

func TestQuitAfterApplyQuitsOnlyAfterConfirmation(t *testing.T) {
	m := Model{
		styles:         newStyles(),
		mode:           modeConfirm,
		quitAfterApply: true,
		pending: &pendingApply{
			profile:  profile.New("Desk Home", nil),
			deadline: time.Now().Add(10 * time.Second),
		},
	}

	view := m.renderConfirm()
	if !strings.Contains(view, "keeps the change and quits") {
		t.Fatalf("expected confirm dialog to explain keep-and-quit, got:\n%s", view)
	}

	updatedModel, cmd := m.updateConfirmKeys(tea.KeyMsg{Type: tea.KeyEnter})
	got := updatedModel.(Model)
	if got.quitAfterApply {
		t.Fatal("expected quit-after-apply flag to clear after confirmation")
	}
	if got.pending != nil {
		t.Fatalf("expected pending apply to clear, got %+v", got.pending)
	}
	assertQuitCmd(t, cmd)
}

func TestQuitDuringApplyConfirmationRevertsBeforeQuitting(t *testing.T) {
	m := Model{
		styles: newStyles(),
		mode:   modeConfirm,
		pending: &pendingApply{
			profile:  profile.New("Desk", nil),
			deadline: time.Now().Add(10 * time.Second),
		},
	}

	updated, cmd := m.updateConfirmKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	got := updated.(Model)
	if cmd == nil || !got.quitAfterRevert || got.pending == nil {
		t.Fatalf("expected quit to request revert first, pending=%+v quitAfterRevert=%v", got.pending, got.quitAfterRevert)
	}
	if _, ok := cmd().(tea.QuitMsg); ok {
		t.Fatal("quit command ran before revert")
	}
}

func TestQuitWhileApplyIsRunningWaitsForResult(t *testing.T) {
	m := Model{styles: newStyles(), mode: modeMain, applying: true}

	updated, cmd := m.updateMainKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	got := updated.(Model)
	if cmd != nil {
		t.Fatal("expected quit to wait instead of exiting during apply")
	}
	if !got.quitAfterRevert || !got.applying {
		t.Fatalf("expected deferred quit while apply runs, applying=%v quitAfterRevert=%v", got.applying, got.quitAfterRevert)
	}
	if !strings.Contains(got.status, "restoring") {
		t.Fatalf("expected deferred quit status, got %q", got.status)
	}
}

func TestPendingRevertGuardWaitsForInflightWork(t *testing.T) {
	guard := &pendingRevertGuard{}
	guard.begin()
	type result struct {
		snapshot apply.RevertState
		armed    bool
		err      error
	}
	resultCh := make(chan result, 1)
	go func() {
		snapshot, armed, err := guard.pending(context.Background())
		resultCh <- result{snapshot: snapshot, armed: armed, err: err}
	}()

	select {
	case <-resultCh:
		t.Fatal("guard returned while apply work was still in flight")
	default:
	}

	want := apply.RevertState{MonitorsConf: config.FileSnapshot{Path: "/tmp/monitors.conf"}}
	guard.arm(want)
	guard.finish()
	got := <-resultCh
	if got.err != nil || !got.armed || got.snapshot.MonitorsConf.Path != want.MonitorsConf.Path {
		t.Fatalf("unexpected pending guard result: %+v", got)
	}
}

func TestSuccessfulQuitRevertDisarmsGuardAndQuits(t *testing.T) {
	guard := &pendingRevertGuard{armed: true}
	m := Model{
		styles:          newStyles(),
		mode:            modeConfirm,
		pending:         &pendingApply{deadline: time.Now().Add(10 * time.Second)},
		revertGuard:     guard,
		quitAfterRevert: true,
	}

	updated, cmd := m.Update(revertMsg{reason: "quit"})
	got := updated.(Model)
	if got.pending != nil || guard.isArmed() {
		t.Fatalf("expected successful revert to clear pending state, pending=%+v armed=%v", got.pending, guard.isArmed())
	}
	assertQuitCmd(t, cmd)
}

func TestFailedRevertKeepsPendingConfigurationArmed(t *testing.T) {
	guard := &pendingRevertGuard{armed: true}
	pending := &pendingApply{deadline: time.Now().Add(10 * time.Second)}
	m := Model{
		styles:          newStyles(),
		mode:            modeConfirm,
		pending:         pending,
		revertGuard:     guard,
		quitAfterRevert: true,
	}

	updated, cmd := m.Update(revertMsg{reason: "quit", err: errors.New("reload failed")})
	got := updated.(Model)
	if cmd != nil || got.pending == nil || !guard.isArmed() || got.mode != modeConfirm {
		t.Fatalf("expected failed revert to remain pending, mode=%v pending=%+v armed=%v", got.mode, got.pending, guard.isArmed())
	}
	if got.quitAfterRevert {
		t.Fatal("expected failed quit-revert attempt to clear the immediate quit request")
	}
}

func TestSaveMsgWithApplyActionSkipsSecondPrompt(t *testing.T) {
	m := Model{
		styles: newStyles(),
		mode:   modeSave,
		dirty:  true,
		saveDialog: &saveDialogState{
			Action: saveActionApply,
		},
	}

	updatedModel, cmd := m.Update(saveMsg{name: "Desk Home"})
	got := updatedModel.(Model)

	if cmd == nil {
		t.Fatal("expected save with Save & Apply selected to return follow-up commands")
	}
	if got.mode != modeMain {
		t.Fatalf("expected save with Save & Apply selected to return to main mode, got %v", got.mode)
	}
	if got.saveDialog != nil {
		t.Fatalf("expected save with Save & Apply selected to clear dialog state, got %+v", got.saveDialog)
	}
	if !got.dirty || !got.draftSaved {
		t.Fatal("expected save with Save & Apply selected to keep the saved draft intact")
	}
	if got.draftProfileName != "Desk Home" {
		t.Fatalf("expected save with Save & Apply selected to remember profile name, got %q", got.draftProfileName)
	}
}

func TestSaveDialogDoesNotShowStaleSuccessStatus(t *testing.T) {
	m := Model{
		styles:   newStyles(),
		height:   30,
		profiles: []profile.Profile{{Name: "Laptop Home"}},
	}

	updatedModel, _ := m.openSaveDialog()
	got := updatedModel.(*Model)
	got.setStatusOK("Loaded 2 monitors and 1 profiles")

	view := got.renderSavePrompt()
	if strings.Contains(view, "Loaded 2 monitors and 1 profiles") {
		t.Fatalf("expected save dialog to hide stale success status, got:\n%s", view)
	}

	got.setStatusErr("Profile name cannot be empty")
	view = got.renderSavePrompt()
	if !strings.Contains(view, "Profile name cannot be empty") {
		t.Fatalf("expected save dialog to show errors, got:\n%s", view)
	}
}

func TestRefreshMsgBackgroundReloadsEditorWhenLiveConfigChanges(t *testing.T) {
	monitorsA := []hypr.Monitor{{
		Name:        "DP-1",
		Make:        "Dell",
		Model:       "U2720Q",
		Serial:      "A1",
		Width:       2560,
		Height:      1440,
		RefreshRate: 144,
		Scale:       1,
	}}
	monitorsB := append(append([]hypr.Monitor(nil), monitorsA...), hypr.Monitor{
		Name:        "HDMI-A-1",
		Make:        "LG",
		Model:       "27GP850",
		Serial:      "B2",
		Width:       2560,
		Height:      1440,
		RefreshRate: 144,
		X:           2560,
		Scale:       1,
	})

	m := Model{
		styles:   newStyles(),
		monitors: monitorsA,
		lidState: lid.Open,
	}
	m.loadLiveState()
	m.editOutputs[0].X = 999
	m.dirty = true

	updatedModel, _ := m.Update(refreshMsg{
		monitors:   monitorsB,
		lidState:   lid.Open,
		background: true,
	})
	got := updatedModel.(Model)

	if got.dirty {
		t.Fatal("expected live config change to reset dirty draft")
	}
	if len(got.editOutputs) != 2 {
		t.Fatalf("expected editor to reload two outputs, got %d", len(got.editOutputs))
	}
	if got.editOutputs[0].X == 999 {
		t.Fatal("expected stale draft position to be discarded after live config change")
	}
	if !strings.Contains(got.status, "Monitor configuration changed") {
		t.Fatalf("expected live reload status, got %q", got.status)
	}
}

func TestRefreshMsgBackgroundPreservesDirtyDraftWhenLiveConfigUnchanged(t *testing.T) {
	monitors := []hypr.Monitor{{
		Name:        "DP-1",
		Make:        "Dell",
		Model:       "U2720Q",
		Serial:      "A1",
		Width:       2560,
		Height:      1440,
		RefreshRate: 144,
		Scale:       1,
	}}

	m := Model{
		styles:   newStyles(),
		monitors: monitors,
		lidState: lid.Open,
	}
	m.loadLiveState()
	m.editOutputs[0].X = 999
	m.dirty = true

	updatedModel, _ := m.Update(refreshMsg{
		monitors:   monitors,
		lidState:   lid.Open,
		background: true,
	})
	got := updatedModel.(Model)

	if !got.dirty {
		t.Fatal("expected unchanged live config to preserve dirty draft")
	}
	if len(got.editOutputs) != 1 {
		t.Fatalf("expected one output to remain in editor, got %d", len(got.editOutputs))
	}
	if got.editOutputs[0].X != 999 {
		t.Fatalf("expected draft position to remain intact, got %d", got.editOutputs[0].X)
	}
}

func TestTickMsgStartsBackgroundRefreshWhenIdle(t *testing.T) {
	m := Model{styles: newStyles()}

	updatedModel, cmd := m.Update(tickMsg(time.Now()))
	got := updatedModel.(Model)

	if cmd == nil {
		t.Fatal("expected tick to schedule follow-up work")
	}
	if !got.refreshInFlight {
		t.Fatal("expected tick to queue a background refresh when idle")
	}
}

func TestRenderMainFitsNarrowTerminalWidth(t *testing.T) {
	m := Model{
		styles:      newStyles(),
		mode:        modeMain,
		tab:         tabLayout,
		layoutFocus: layoutFocusInspector,
		width:       60,
		height:      24,
		editOutputs: []editableOutput{{
			Key:             "microstep|mpg321ur-qd",
			Name:            "DP-1",
			Description:     "Microstep MPG321UR-QD",
			Enabled:         true,
			Modes:           []string{"3840x2160@143.99Hz"},
			ModeIndex:       0,
			Width:           3840,
			Height:          2160,
			Refresh:         143.99,
			X:               0,
			Y:               0,
			Scale:           1.33,
			ActiveWorkspace: "1",
		}},
		workspaceEdit: workspaceEditor{
			Enabled:       true,
			Strategy:      profile.WorkspaceStrategySequential,
			MaxWorkspaces: 9,
			GroupSize:     3,
		},
	}

	if width := maxRenderedLineWidth(m.renderMain()); width > m.width {
		t.Fatalf("expected main view to fit width %d, got max line width %d", m.width, width)
	}
	if height := lipgloss.Height(m.renderMain()); height != m.height {
		t.Fatalf("expected main view to fill height %d, got %d", m.height, height)
	}
}

func TestSaveModalFitsNarrowTerminalWidth(t *testing.T) {
	m := Model{
		styles:   newStyles(),
		width:    60,
		height:   24,
		profiles: []profile.Profile{{Name: "Laptop Home"}, {Name: "Desk Dock"}},
	}

	updatedModel, _ := m.openSaveDialog()
	got := updatedModel.(*Model)

	if width := maxRenderedLineWidth(got.View()); width > got.width {
		t.Fatalf("expected save modal to fit width %d, got max line width %d", got.width, width)
	}
}

func TestRenderMainFitsShortTerminalHeight(t *testing.T) {
	m := Model{
		styles:      newStyles(),
		mode:        modeMain,
		tab:         tabLayout,
		layoutFocus: layoutFocusInspector,
		width:       80,
		height:      16,
		editOutputs: []editableOutput{{
			Key:             "microstep|mpg321ur-qd",
			Name:            "DP-1",
			Description:     "Microstep MPG321UR-QD",
			Enabled:         true,
			Modes:           []string{"3840x2160@143.99Hz"},
			ModeIndex:       0,
			Width:           3840,
			Height:          2160,
			Refresh:         143.99,
			X:               0,
			Y:               0,
			Scale:           1.33,
			ActiveWorkspace: "1",
		}},
		workspaceEdit: workspaceEditor{
			Enabled:       true,
			Strategy:      profile.WorkspaceStrategySequential,
			MaxWorkspaces: 9,
			GroupSize:     3,
		},
	}

	view := m.renderMain()
	if width := maxRenderedLineWidth(view); width > m.width {
		t.Fatalf("expected short main view to fit width %d, got max line width %d", m.width, width)
	}
	if height := lipgloss.Height(view); height != m.height {
		t.Fatalf("expected short main view to fill height %d, got %d", m.height, height)
	}
	if !strings.Contains(view, "Display") || !strings.Contains(view, "Info") {
		t.Fatalf("expected display and info panes to be visible, got:\n%s", view)
	}
}

func TestRenderInspectorPaneCompactsFieldsOnShortHeight(t *testing.T) {
	m := Model{
		styles:         newStyles(),
		tab:            tabLayout,
		layoutFocus:    layoutFocusInspector,
		inspectorField: 2,
		editOutputs: []editableOutput{{
			Key:             "microstep|mpg321ur-qd",
			Name:            "DP-1",
			Description:     "Microstep MPG321UR-QD",
			Enabled:         true,
			Modes:           []string{"3840x2160@143.99Hz"},
			ModeIndex:       0,
			Width:           3840,
			Height:          2160,
			Refresh:         143.99,
			X:               0,
			Y:               120,
			Scale:           1.33,
			VRR:             1,
			Transform:       0,
			ActiveWorkspace: "1",
		}},
	}

	view := m.renderInspectorPane(48, 30, false)
	for _, want := range []string{"Mode", "3840x2160@144Hz", "Scale", "VRR", "Rotation", "Position X", "Position Y"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected inspector to include %q, got:\n%s", want, view)
		}
	}
}

func TestInspectorModeOmitsPickerPosition(t *testing.T) {
	m := Model{styles: newStyles()}
	output := editableOutput{
		Modes:     []string{"3840x2160@143.99Hz", "2560x1440@144Hz"},
		ModeIndex: 0,
		Width:     3840,
		Height:    2160,
		Refresh:   143.99,
	}
	got := m.layoutFieldValue(output, 1)
	if got != "3840x2160@144Hz" {
		t.Fatalf("expected clean mode value, got %q", got)
	}
}

func TestCompactLayoutHeightsReserveSpaceForInspector(t *testing.T) {
	m := Model{}

	canvas, inspector := m.compactLayoutHeights(18)
	if inspector < 10 {
		t.Fatalf("expected compact layout to reserve at least 10 rows for the inspector, got canvas=%d inspector=%d", canvas, inspector)
	}
	if canvas < 4 {
		t.Fatalf("expected compact layout to preserve a usable canvas, got canvas=%d inspector=%d", canvas, inspector)
	}
	if canvas+inspector != 18 {
		t.Fatalf("expected compact layout heights to add up to 18, got canvas=%d inspector=%d", canvas, inspector)
	}
}

func TestRenderMainFitsTallMediumWidth(t *testing.T) {
	m := Model{
		styles:      newStyles(),
		mode:        modeMain,
		tab:         tabLayout,
		layoutFocus: layoutFocusInspector,
		width:       100,
		height:      40,
		editOutputs: []editableOutput{{
			Key:             "samsung display corp.|atna60cl10-0",
			Name:            "eDP-1",
			Description:     "Samsung Display Corp. ATNA60CL10-0",
			Make:            "Samsung Display Corp.",
			Model:           "ATNA60CL10-0",
			Enabled:         true,
			Modes:           []string{"2880x1800@120.00Hz", "2560x1600@90.00Hz"},
			ModeIndex:       0,
			Width:           2880,
			Height:          1800,
			Refresh:         120,
			X:               0,
			Y:               0,
			Scale:           1.50,
			Focused:         true,
			DPMSStatus:      true,
			PhysicalWidth:   340,
			PhysicalHeight:  220,
			ActiveWorkspace: "1",
		}},
	}

	view := m.renderMain()
	if width := maxRenderedLineWidth(view); width > m.width {
		t.Fatalf("expected tall medium-width view to fit width %d, got max line width %d", m.width, width)
	}
	if height := lipgloss.Height(view); height != m.height {
		t.Fatalf("expected tall medium-width view to fill height %d, got %d", m.height, height)
	}
	if !strings.Contains(view, "Display") || !strings.Contains(view, "Info") {
		t.Fatalf("expected display and info panes visible, got:\n%s", view)
	}
}

func TestFitBlockAccountsForWrappedLines(t *testing.T) {
	text := strings.Join([]string{
		"Selected Monitor",
		"Enter opens the active editor. Mouse click selects fields.",
		"Samsung Display Corp. ATNA60CL10-0",
		"Mode 2880x1800@120.00Hz (1/13)",
	}, "\n")

	got := fitBlock(text, 20, 6)
	if width := maxRenderedLineWidth(got); width > 20 {
		t.Fatalf("expected wrapped block to fit width 20, got %d", width)
	}
	if height := lipgloss.Height(got); height != 6 {
		t.Fatalf("expected wrapped block to fit height 6, got %d", height)
	}
}

func TestUseCompactLayoutForMediumWideTallTerminals(t *testing.T) {
	m := Model{width: 95}
	if !m.useCompactLayout(30) {
		t.Fatal("expected a terminal below 96 columns to stay compact")
	}

	m.width = 100
	if m.useCompactLayout(30) {
		t.Fatal("expected an Omarchy-sized terminal to allow side-by-side layout")
	}
}

func TestPreviewSelectedSnapShowsAlignedBottomEdgeWithoutMoving(t *testing.T) {
	m := Model{
		selectedOutput: 1,
		editOutputs: []editableOutput{
			{
				Name:    "DP-1",
				Enabled: true,
				Width:   3840,
				Height:  2160,
				Scale:   1,
				X:       0,
				Y:       0,
			},
			{
				Name:    "eDP-1",
				Enabled: true,
				Width:   1920,
				Height:  1200,
				Scale:   1,
				X:       4000,
				Y:       950,
			},
		},
	}

	hint := m.previewSelectedSnap(24)
	if hint == nil {
		t.Fatal("expected aligned-edge snap hint")
	}
	if m.editOutputs[1].Y != 950 {
		t.Fatalf("preview should not mutate output position, got %d", m.editOutputs[1].Y)
	}
	if !hasSnapMark(hint.Marks, 1, snapEdgeBottom) || !hasSnapMark(hint.Marks, 0, snapEdgeBottom) {
		t.Fatalf("expected bottom-edge marks for both monitors, got %+v", hint.Marks)
	}
}

func TestApplySelectedSnapAlignsBottomEdge(t *testing.T) {
	m := Model{
		selectedOutput: 1,
		editOutputs: []editableOutput{
			{
				Name:    "DP-1",
				Enabled: true,
				Width:   3840,
				Height:  2160,
				Scale:   1,
				X:       0,
				Y:       0,
			},
			{
				Name:    "eDP-1",
				Enabled: true,
				Width:   1920,
				Height:  1200,
				Scale:   1,
				X:       4000,
				Y:       950,
			},
		},
	}

	hint := m.applySelectedSnap(24)
	if hint == nil {
		t.Fatal("expected aligned-edge snap application")
	}
	if m.editOutputs[1].Y != 960 {
		t.Fatalf("expected Y to snap to 960, got %d", m.editOutputs[1].Y)
	}
}

func TestSnapSelectedOutputPlacesItAroundNearestMonitor(t *testing.T) {
	tests := []struct {
		name         string
		direction    snapDirection
		wantX        int
		wantY        int
		selectedEdge snapEdge
		anchorEdge   snapEdge
	}{
		{name: "left", direction: snapDirectionLeft, wantX: -1180, wantY: 380, selectedEdge: snapEdgeRight, anchorEdge: snapEdgeLeft},
		{name: "right", direction: snapDirectionRight, wantX: 2020, wantY: 380, selectedEdge: snapEdgeLeft, anchorEdge: snapEdgeRight},
		{name: "up", direction: snapDirectionUp, wantX: 420, wantY: -520, selectedEdge: snapEdgeBottom, anchorEdge: snapEdgeTop},
		{name: "down", direction: snapDirectionDown, wantX: 420, wantY: 1280, selectedEdge: snapEdgeTop, anchorEdge: snapEdgeBottom},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := Model{
				selectedOutput: 1,
				editOutputs: []editableOutput{
					{
						Key:     "anchor",
						Name:    "DP-1",
						Enabled: true,
						Width:   1920,
						Height:  1080,
						Scale:   1,
						X:       100,
						Y:       200,
					},
					{
						Key:     "selected",
						Name:    "eDP-1",
						Enabled: true,
						Width:   2560,
						Height:  1440,
						Scale:   2,
						X:       900,
						Y:       700,
					},
				},
			}

			cmd := m.snapSelectedOutput(test.direction)
			if cmd == nil {
				t.Fatal("expected snap hint command")
			}
			selected := m.editOutputs[m.selectedOutput]
			if selected.X != test.wantX || selected.Y != test.wantY {
				t.Fatalf("snapped position = %d,%d, want %d,%d", selected.X, selected.Y, test.wantX, test.wantY)
			}
			if !m.dirty {
				t.Fatal("expected snap to mark the layout dirty")
			}
			if m.snap == nil || !hasSnapMark(m.snap.Marks, 1, test.selectedEdge) || !hasSnapMark(m.snap.Marks, 0, test.anchorEdge) {
				t.Fatalf("expected touching-edge snap marks, got %+v", m.snap)
			}
		})
	}
}

func TestSnapSelectedOutputUsesNearestEligibleMonitor(t *testing.T) {
	m := Model{
		selectedOutput: 0,
		editOutputs: []editableOutput{
			{Key: "selected", Name: "DP-1", Enabled: true, Width: 1000, Height: 800, Scale: 1, X: 900, Y: 100},
			{Key: "disabled", Name: "DP-2", Enabled: false, Width: 1000, Height: 800, Scale: 1, X: 950, Y: 100},
			{Key: "mirror", Name: "DP-3", Enabled: true, MirrorOf: "anchor", Width: 1000, Height: 800, Scale: 1, X: 1000, Y: 100},
			{Key: "anchor", Name: "DP-4", Enabled: true, Width: 1000, Height: 800, Scale: 1, X: 4000, Y: 100},
		},
	}

	m.snapSelectedOutput(snapDirectionRight)
	if got := m.editOutputs[0].X; got != 5000 {
		t.Fatalf("expected selected output to snap beside nearest eligible monitor at X=5000, got %d", got)
	}
	if !strings.Contains(m.status, "DP-4") {
		t.Fatalf("expected status to name the anchor monitor, got %q", m.status)
	}
}

func TestSnapSelectedOutputWithoutAnchorDoesNotChangeLayout(t *testing.T) {
	m := Model{
		editOutputs: []editableOutput{{
			Key:     "selected",
			Name:    "DP-1",
			Enabled: true,
			Width:   1920,
			Height:  1080,
			Scale:   1,
			X:       10,
			Y:       20,
		}},
	}

	if cmd := m.snapSelectedOutput(snapDirectionRight); cmd != nil {
		t.Fatal("did not expect a command without an eligible anchor")
	}
	if m.editOutputs[0].X != 10 || m.editOutputs[0].Y != 20 || m.dirty {
		t.Fatalf("expected unchanged layout, got output=%+v dirty=%v", m.editOutputs[0], m.dirty)
	}
	if !m.statusErr || !strings.Contains(m.status, "No other enabled monitor") {
		t.Fatalf("expected helpful error status, got %q", m.status)
	}
}

// Off keeps the saved plan but writes no rules (#70), so the preview must not
// suggest any workspace placement.
func TestRenderWorkspaceViewWhenOff(t *testing.T) {
	m := Model{
		styles: newStyles(),
		tab:    tabWorkspaces,
		editOutputs: []editableOutput{
			{Key: "mon-a", Name: "DP-1", Enabled: true, Scale: 1},
			{Key: "mon-b", Name: "HDMI-A-1", Enabled: true, Scale: 1},
		},
		workspaceEdit: workspaceEditor{
			Enabled:       false,
			Strategy:      profile.WorkspaceStrategySequential,
			MaxWorkspaces: 6,
			GroupSize:     3,
			MonitorOrder:  []string{"mon-a", "mon-b"},
		},
	}

	view := ansi.Strip(m.renderWorkspaceView(16))
	requireContains(t, view, "Strategy        Off", "Off: hyprmoncfg writes no workspace rules.")
	for _, unwanted := range []string{"1, 2, 3", "4, 5, 6", "Monitor order", "Group size     3"} {
		if strings.Contains(view, unwanted) {
			t.Fatalf("Off view should not show %q:\n%s", unwanted, view)
		}
	}
}

func TestAdjustWorkspaceFieldRestoresSequentialPreviewAfterInterleave(t *testing.T) {
	m := Model{
		styles: newStyles(),
		tab:    tabWorkspaces,
		editOutputs: []editableOutput{
			{Key: "mon-a", Name: "DP-1", Enabled: true, Scale: 1},
			{Key: "mon-b", Name: "HDMI-A-1", Enabled: true, Scale: 1},
		},
		workspaceEdit: workspaceEditor{
			Enabled:                 true,
			Strategy:                profile.WorkspaceStrategyInterleave,
			MaxWorkspaces:           6,
			GroupSize:               1,
			LastSequentialGroupSize: defaultWorkspaceGroupSize,
			MonitorOrder:            []string{"mon-a", "mon-b"},
			SelectedField:           0,
		},
	}

	m.adjustWorkspaceField(-1)
	if m.workspaceEdit.Strategy != profile.WorkspaceStrategySequential {
		t.Fatalf("expected sequential strategy after moving left from interleave, got %q", m.workspaceEdit.Strategy)
	}
	if m.workspaceEdit.GroupSize != defaultWorkspaceGroupSize {
		t.Fatalf("expected sequential to restore default group size %d, got %d", defaultWorkspaceGroupSize, m.workspaceEdit.GroupSize)
	}

	view := m.renderWorkspaceView(16)
	for _, want := range []string{"DP-1      1, 2, 3", "HDMI-A-1  4, 5, 6"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected sequential preview to include %q after strategy switch, got:\n%s", want, view)
		}
	}
}

func TestAdjustWorkspaceFieldPreservesCustomSequentialGroupSize(t *testing.T) {
	m := Model{
		workspaceEdit: workspaceEditor{
			Enabled:                 true,
			Strategy:                profile.WorkspaceStrategySequential,
			GroupSize:               2,
			LastSequentialGroupSize: 2,
			SelectedField:           0,
		},
	}

	m.adjustWorkspaceField(1)
	if m.workspaceEdit.Strategy != profile.WorkspaceStrategyInterleave {
		t.Fatalf("expected interleave strategy after moving right from sequential, got %q", m.workspaceEdit.Strategy)
	}

	m.adjustWorkspaceField(-1)
	if m.workspaceEdit.Strategy != profile.WorkspaceStrategySequential {
		t.Fatalf("expected sequential strategy after moving left from interleave, got %q", m.workspaceEdit.Strategy)
	}
	if m.workspaceEdit.GroupSize != 2 {
		t.Fatalf("expected custom sequential group size to be preserved, got %d", m.workspaceEdit.GroupSize)
	}
}

func TestWorkspaceEditorFromInterleaveSettingsSeedsSequentialGroupSize(t *testing.T) {
	editor := workspaceEditorFromSettings(profile.WorkspaceSettings{
		Enabled:       true,
		Strategy:      profile.WorkspaceStrategyInterleave,
		MaxWorkspaces: 6,
		GroupSize:     1,
		MonitorOrder:  []string{"mon-a", "mon-b"},
	}, []editableOutput{
		{Key: "mon-a", Name: "DP-1", Enabled: true, Scale: 1},
		{Key: "mon-b", Name: "HDMI-A-1", Enabled: true, Scale: 1},
	})

	if editor.GroupSize != 1 {
		t.Fatalf("expected interleave settings to keep stored group size 1, got %d", editor.GroupSize)
	}
	if editor.LastSequentialGroupSize != defaultWorkspaceGroupSize {
		t.Fatalf("expected interleave settings to seed sequential group size %d, got %d", defaultWorkspaceGroupSize, editor.LastSequentialGroupSize)
	}
}

func TestWorkspaceEditorFromSettingsFallsBackToManualRuleOrder(t *testing.T) {
	editor := workspaceEditorFromSettings(profile.WorkspaceSettings{
		Enabled:       true,
		Strategy:      profile.WorkspaceStrategySequential,
		MaxWorkspaces: 6,
		GroupSize:     3,
		Rules: []profile.WorkspaceRule{
			{Workspace: "1", OutputName: "DP-1"},
			{Workspace: "2", OutputName: "DP-1"},
			{Workspace: "3", OutputName: "DP-1"},
			{Workspace: "4", OutputName: "eDP-1"},
			{Workspace: "5", OutputName: "eDP-1"},
			{Workspace: "6", OutputName: "eDP-1"},
		},
	}, []editableOutput{
		{Key: "dp-key", Name: "DP-1", Enabled: true, Scale: 1},
		{Key: "edp-key", Name: "eDP-1", Enabled: true, Scale: 1},
	})

	if len(editor.MonitorOrder) != 2 {
		t.Fatalf("expected monitor order from manual rules, got %v", editor.MonitorOrder)
	}
	if editor.MonitorOrder[0] != "dp-key" || editor.MonitorOrder[1] != "edp-key" {
		t.Fatalf("expected DP-1 then eDP-1, got %v", editor.MonitorOrder)
	}
}

func TestEnteringManualWorkspaceStrategyMaterializesVisiblePlan(t *testing.T) {
	m := Model{
		styles: newStyles(),
		editOutputs: []editableOutput{
			{Key: "dell|desk", Name: "DP-1", Make: "Dell", Model: "Desk", Enabled: true, Scale: 1},
			{Key: "lg|side", Name: "HDMI-A-1", Make: "LG", Model: "Side", Enabled: true, Scale: 1},
		},
		workspaceEdit: workspaceEditor{
			Enabled:       true,
			Strategy:      profile.WorkspaceStrategySequential,
			MaxWorkspaces: 6,
			GroupSize:     3,
			MonitorOrder:  []string{"dell|desk", "lg|side"},
			SelectedField: 0,
		},
	}

	m.adjustWorkspaceField(-1)

	if m.workspaceEdit.Strategy != profile.WorkspaceStrategyManual {
		t.Fatalf("expected manual strategy, got %q", m.workspaceEdit.Strategy)
	}
	if !m.workspaceEdit.ManualRulesInitialized || len(m.workspaceEdit.Rules) != 6 {
		t.Fatalf("expected the visible six-workspace plan to become manual rules, got %+v", m.workspaceEdit)
	}
	if m.workspaceEdit.Rules[0].OutputKey != "dell|desk" || m.workspaceEdit.Rules[3].OutputKey != "lg|side" {
		t.Fatalf("unexpected materialized assignments: %+v", m.workspaceEdit.Rules)
	}
	if !m.workspaceEdit.Rules[0].Default || !m.workspaceEdit.Rules[3].Default {
		t.Fatalf("expected one default workspace per display: %+v", m.workspaceEdit.Rules)
	}
	view := strings.Join(m.workspaceSettingsLines(), "\n")
	for _, want := range []string{"Workspace → display", "Workspace 1", "Dell Desk", "Workspace 4", "LG Side"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected manual planner to include %q, got:\n%s", want, view)
		}
	}
}

func TestOpeningWorkspaceTabRepairsAnEmptyManualDraft(t *testing.T) {
	m := Model{
		styles: newStyles(),
		editOutputs: []editableOutput{
			{Key: "mon-a", Name: "DP-1", Enabled: true, Scale: 1},
			{Key: "mon-b", Name: "HDMI-A-1", Enabled: true, Scale: 1},
		},
		workspaceEdit: workspaceEditor{
			Enabled:       true,
			Strategy:      profile.WorkspaceStrategyManual,
			MaxWorkspaces: 4,
			GroupSize:     2,
			MonitorOrder:  []string{"mon-a", "mon-b"},
		},
	}

	updated, _ := m.updateMainKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	got := updated.(Model)

	if got.tab != tabWorkspaces || len(got.workspaceEdit.Rules) != 4 {
		t.Fatalf("expected an editable four-workspace manual plan, got tab=%d rules=%+v", got.tab, got.workspaceEdit.Rules)
	}
	if !got.workspaceEdit.ManualRulesInitialized || !got.dirty {
		t.Fatalf("expected the repaired draft to be initialized and visibly dirty: %+v", got.workspaceEdit)
	}
}

func TestManualWorkspaceAssignmentCyclesSelectedWorkspaceTarget(t *testing.T) {
	m := Model{
		styles: newStyles(),
		editOutputs: []editableOutput{
			{Key: "mon-a", Name: "DP-1", Enabled: true, Scale: 1},
			{Key: "mon-b", Name: "HDMI-A-1", Enabled: true, Scale: 1},
		},
		workspaceEdit: workspaceEditor{
			Enabled:                true,
			Strategy:               profile.WorkspaceStrategyManual,
			MaxWorkspaces:          2,
			MonitorOrder:           []string{"mon-a", "mon-b"},
			ManualRulesInitialized: true,
			Rules: []profile.WorkspaceRule{
				{Workspace: "1", OutputKey: "mon-a", OutputName: "DP-1", Default: true, Persistent: true},
				{Workspace: "2", OutputKey: "mon-a", OutputName: "DP-1"},
			},
			SelectedField: len(workspaceFields) + 1,
			SelectedOrder: 1,
		},
	}

	m.moveManualWorkspaceRule(1)

	rules := m.workspaceEdit.Rules
	if rules[1].OutputKey != "mon-b" || rules[1].OutputName != "HDMI-A-1" {
		t.Fatalf("expected workspace 2 on mon-b, got %+v", rules[1])
	}
	if !rules[0].Default || !rules[0].Persistent || !rules[1].Default || !rules[1].Persistent {
		t.Fatalf("expected the first workspace on each display to remain default and persistent: %+v", rules)
	}
}

func TestManualWorkspaceCountAddsAndRemovesNumberedAssignments(t *testing.T) {
	m := Model{
		editOutputs: []editableOutput{
			{Key: "mon-a", Name: "DP-1", Enabled: true, Scale: 1},
			{Key: "mon-b", Name: "HDMI-A-1", Enabled: true, Scale: 1},
		},
		workspaceEdit: workspaceEditor{
			Enabled:       true,
			Strategy:      profile.WorkspaceStrategyManual,
			MaxWorkspaces: 2,
			MonitorOrder:  []string{"mon-a", "mon-b"},
			SelectedField: 1,
			Rules: []profile.WorkspaceRule{
				{Workspace: "1", OutputKey: "mon-a", OutputName: "DP-1"},
				{Workspace: "2", OutputKey: "mon-b", OutputName: "HDMI-A-1"},
				{Workspace: "special:music", OutputKey: "mon-b", OutputName: "HDMI-A-1"},
			},
		},
	}

	m.adjustWorkspaceField(1)
	if m.workspaceEdit.MaxWorkspaces != 3 || len(m.workspaceEdit.Rules) != 4 {
		t.Fatalf("expected workspace 3 plus the named rule, got max=%d rules=%+v", m.workspaceEdit.MaxWorkspaces, m.workspaceEdit.Rules)
	}
	if m.workspaceEdit.Rules[2].Workspace != "3" || m.workspaceEdit.Rules[2].OutputKey != "mon-a" {
		t.Fatalf("unexpected new workspace assignment: %+v", m.workspaceEdit.Rules[2])
	}

	m.adjustWorkspaceField(-1)
	if m.workspaceEdit.MaxWorkspaces != 2 || len(m.workspaceEdit.Rules) != 3 {
		t.Fatalf("expected workspace 3 to be removed while preserving the named rule, got %+v", m.workspaceEdit.Rules)
	}
	if m.workspaceEdit.Rules[2].Workspace != "special:music" {
		t.Fatalf("expected named rule to survive resizing, got %+v", m.workspaceEdit.Rules)
	}
}

func TestWorkspaceCountsHaveNoLegacyCeilingAndAcceptExactInput(t *testing.T) {
	m := Model{
		styles: newStyles(),
		workspaceEdit: workspaceEditor{
			Enabled:       true,
			Strategy:      profile.WorkspaceStrategySequential,
			MaxWorkspaces: 30,
			GroupSize:     10,
			SelectedField: 1,
		},
	}

	m.adjustWorkspaceField(1)
	if m.workspaceEdit.MaxWorkspaces != 31 {
		t.Fatalf("workspace count should move past the old ceiling: got %d", m.workspaceEdit.MaxWorkspaces)
	}
	m.workspaceEdit.SelectedField = 2
	m.adjustWorkspaceField(1)
	if m.workspaceEdit.GroupSize != 11 {
		t.Fatalf("group size should move past the old ceiling: got %d", m.workspaceEdit.GroupSize)
	}

	m.workspaceEdit.SelectedField = 1
	updated, _ := m.updateWorkspaceKeys(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(Model)
	if got.mode != modeNumericInput || got.input == nil || got.input.Kind != numericInputWorkspaceCount {
		t.Fatalf("Enter should open exact workspace count input, got mode=%v input=%+v", got.mode, got.input)
	}
	if got.dirty {
		t.Fatal("opening exact count input should not dirty the draft")
	}
	got.input.Input.SetValue("128")
	got.commitNumericInput()
	if got.workspaceEdit.MaxWorkspaces != 128 || !got.dirty || got.mode != modeMain {
		t.Fatalf("expected exact workspace count 128, got editor=%+v dirty=%v mode=%v", got.workspaceEdit, got.dirty, got.mode)
	}

	got.dirty = false
	got.workspaceEdit.SelectedField = 2
	updated, _ = got.updateWorkspaceKeys(tea.KeyMsg{Type: tea.KeyEnter})
	got = updated.(Model)
	if got.mode != modeNumericInput || got.input == nil || got.input.Kind != numericInputWorkspaceGroupSize {
		t.Fatalf("Enter should open exact group size input, got mode=%v input=%+v", got.mode, got.input)
	}
	got.input.Input.SetValue("64")
	got.commitNumericInput()
	if got.workspaceEdit.GroupSize != 64 || got.workspaceEdit.LastSequentialGroupSize != 64 || !got.dirty {
		t.Fatalf("expected exact group size 64, got editor=%+v dirty=%v", got.workspaceEdit, got.dirty)
	}
}

func TestLongManualWorkspaceListHasPageAndBoundaryNavigation(t *testing.T) {
	rules := make([]profile.WorkspaceRule, 1000)
	for idx := range rules {
		rules[idx] = profile.WorkspaceRule{
			Workspace: strconv.Itoa(idx + 1),
			OutputKey: "mon-a",
		}
	}
	m := Model{
		styles: newStyles(),
		width:  120,
		height: 24,
		tab:    tabWorkspaces,
		editOutputs: []editableOutput{
			{Key: "mon-a", Name: "DP-1", Enabled: true, Scale: 1},
		},
		workspaceEdit: workspaceEditor{
			Enabled:       true,
			Strategy:      profile.WorkspaceStrategyManual,
			SelectedField: len(workspaceFields),
			Rules:         rules,
		},
	}

	pageStep := m.workspacePageStep()
	updated, _ := m.updateWorkspaceKeys(tea.KeyMsg{Type: tea.KeyPgDown})
	got := updated.(Model)
	if got.workspaceEdit.SelectedField != len(workspaceFields)+pageStep {
		t.Fatalf("Page Down selected %d, want %d", got.workspaceEdit.SelectedField, len(workspaceFields)+pageStep)
	}
	if got.dirty {
		t.Fatal("navigation should not dirty the workspace draft")
	}

	updated, _ = got.updateWorkspaceKeys(tea.KeyMsg{Type: tea.KeyEnd})
	got = updated.(Model)
	if got.workspaceEdit.SelectedField != len(workspaceFields)+len(rules)-1 || got.workspaceEdit.SelectedOrder != len(rules)-1 {
		t.Fatalf("End should select the final workspace, got field=%d order=%d", got.workspaceEdit.SelectedField, got.workspaceEdit.SelectedOrder)
	}
	visible := ansi.Strip(strings.Join(got.workspaceSettingsVisibleLines(10), "\n"))
	if !strings.Contains(visible, "Workspace 1000") || strings.Contains(visible, "Workspace 1 ") {
		t.Fatalf("expected a virtualized window around the final workspace, got:\n%s", visible)
	}
	if lines := len(strings.Split(visible, "\n")); lines > 10 {
		t.Fatalf("visible workspace window rendered %d lines, want at most 10", lines)
	}

	updated, _ = got.updateWorkspaceKeys(tea.KeyMsg{Type: tea.KeyHome})
	got = updated.(Model)
	if got.workspaceEdit.SelectedField != 0 {
		t.Fatalf("Home should select the first setting, got %d", got.workspaceEdit.SelectedField)
	}
}

func TestWorkspaceMouseWheelScrollsWithoutEditingAssignments(t *testing.T) {
	rules := make([]profile.WorkspaceRule, 100)
	for idx := range rules {
		rules[idx] = profile.WorkspaceRule{
			Workspace: strconv.Itoa(idx + 1),
			OutputKey: "mon-a",
		}
	}
	m := Model{
		styles: newStyles(),
		width:  120,
		height: 24,
		tab:    tabWorkspaces,
		editOutputs: []editableOutput{
			{Key: "mon-a", Name: "DP-1", Enabled: true, Scale: 1},
			{Key: "mon-b", Name: "DP-2", Enabled: true, Scale: 1},
		},
		workspaceEdit: workspaceEditor{
			Enabled:       true,
			Strategy:      profile.WorkspaceStrategyManual,
			SelectedField: len(workspaceFields) + 20,
			SelectedOrder: 20,
			Rules:         rules,
		},
	}
	inner := m.workspaceSettingsRect().inner(m.styles.activePane)
	wheel := tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonWheelDown,
		X:      inner.x,
		Y:      inner.y,
	}

	updated, _ := m.updateWorkspaceMouse(wheel)
	got := updated.(Model)
	if got.workspaceEdit.SelectedField != len(workspaceFields)+23 || got.workspaceEdit.SelectedOrder != 23 {
		t.Fatalf("mouse wheel should move three rows, got field=%d order=%d", got.workspaceEdit.SelectedField, got.workspaceEdit.SelectedOrder)
	}
	if got.workspaceEdit.Rules[20].OutputKey != "mon-a" || got.dirty {
		t.Fatalf("mouse wheel should not edit the hovered assignment, got rule=%+v dirty=%v", got.workspaceEdit.Rules[20], got.dirty)
	}
}

func hasSnapMark(marks []snapMark, outputIndex int, edge snapEdge) bool {
	for _, mark := range marks {
		if mark.OutputIndex == outputIndex && mark.Edge == edge {
			return true
		}
	}
	return false
}

func maxRenderedLineWidth(view string) int {
	maxWidth := 0
	for _, line := range strings.Split(view, "\n") {
		maxWidth = max(maxWidth, lipgloss.Width(line))
	}
	return maxWidth
}

func findVisiblePosition(t *testing.T, view string, text string) (int, int) {
	t.Helper()

	for y, line := range strings.Split(ansi.Strip(view), "\n") {
		idx := strings.Index(line, text)
		if idx >= 0 {
			return lipgloss.Width(line[:idx]), y
		}
	}

	t.Fatalf("expected rendered view to contain %q, got:\n%s", text, ansi.Strip(view))
	return 0, 0
}

func mousePressAt(x, y int) tea.MouseMsg {
	return tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
		X:      x,
		Y:      y,
	}
}

func runModelUpdate(t *testing.T, model tea.Model, msg tea.Msg) tea.Model {
	t.Helper()

	switch m := model.(type) {
	case Model:
		updated, _ := m.Update(msg)
		return updated
	case *Model:
		updated, _ := m.Update(msg)
		return updated
	default:
		t.Fatalf("unexpected model type %T", model)
		return nil
	}
}

func assertQuitCmd(t *testing.T, cmd tea.Cmd) {
	t.Helper()

	if cmd == nil {
		t.Fatal("expected quit command, got nil")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("expected quit command to return tea.QuitMsg")
	}
}

func mustModel(t *testing.T, model tea.Model) Model {
	t.Helper()

	switch m := model.(type) {
	case Model:
		return m
	case *Model:
		return *m
	default:
		t.Fatalf("unexpected model type %T", model)
		return Model{}
	}
}

func textInputWithValue(value string) textinput.Model {
	input := textinput.New()
	input.SetValue(value)
	return input
}

func testProfile(name string, outputCount int) profile.Profile {
	outputs := make([]profile.OutputConfig, 0, outputCount)
	for idx := 0; idx < outputCount; idx++ {
		outputs = append(outputs, profile.OutputConfig{
			Key:     fmt.Sprintf("%s-%d", name, idx),
			Name:    fmt.Sprintf("DP-%d", idx+1),
			Enabled: true,
			Width:   1920,
			Height:  1080,
			Refresh: 60,
			Scale:   1,
		})
	}
	return profile.New(name, outputs)
}

func TestIsOutputOverlapping(t *testing.T) {
	m := Model{
		editOutputs: []editableOutput{
			{
				Name:    "DP-1",
				Enabled: true,
				X:       0,
				Y:       0,
				Width:   1920,
				Height:  1080,
			},
			{
				Name:    "DP-2",
				Enabled: true,
				X:       500,
				Y:       0,
				Width:   1920,
				Height:  1080,
			},
			{
				Name:    "DP-3",
				Enabled: true,
				X:       4000,
				Y:       0,
				Width:   1920,
				Height:  1080,
			},
		},
	}

	if !m.isOutputOverlapping(m.editOutputs[0]) {
		t.Errorf("Expected DP-1 to be marked as overlapping (collides with DP-2)")
	}

	if !m.isOutputOverlapping(m.editOutputs[1]) {
		t.Errorf("Expected DP-2 to be marked as overlapping (collides with DP-1)")
	}

	if m.isOutputOverlapping(m.editOutputs[2]) {
		t.Errorf("Expected DP-3 to NOT be marked as overlapping")
	}
}

func TestLayoutMoveVimKeys(t *testing.T) {
	tests := []struct {
		key    string
		wantDx int
		wantDy int
	}{
		{"h", -100, 0},
		{"j", 0, 100},
		{"k", 0, -100},
		{"l", 100, 0},
		{"H", -500, 0},
		{"J", 0, 500},
		{"K", 0, -500},
		{"L", 500, 0},
	}
	for _, tt := range tests {
		dx, dy, ok := layoutMoveDelta(tt.key)
		if !ok {
			t.Errorf("layoutMoveDelta(%q) returned ok=false", tt.key)
			continue
		}
		if dx != tt.wantDx || dy != tt.wantDy {
			t.Errorf("layoutMoveDelta(%q) = (%d, %d), want (%d, %d)", tt.key, dx, dy, tt.wantDx, tt.wantDy)
		}
	}
}

func TestInspectorVimNavigation(t *testing.T) {
	m := Model{
		styles:      newStyles(),
		mode:        modeMain,
		tab:         tabLayout,
		layoutFocus: layoutFocusInspector,
		editOutputs: []editableOutput{{
			Name:    "DP-1",
			Enabled: true,
			Scale:   1,
		}},
		inspectorField: 2,
	}

	// j moves down
	m.updateLayoutKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.inspectorField != 5 {
		t.Errorf("j: inspectorField = %d, want 5", m.inspectorField)
	}

	// k moves up
	m.updateLayoutKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if m.inspectorField != 2 {
		t.Errorf("k: inspectorField = %d, want 2", m.inspectorField)
	}
}

func TestLayoutMoveVimKeysMatchArrows(t *testing.T) {
	pairs := [][2]string{
		{"h", "left"},
		{"l", "right"},
		{"k", "up"},
		{"j", "down"},
	}
	for _, pair := range pairs {
		vdx, vdy, vok := layoutMoveDelta(pair[0])
		adx, ady, aok := layoutMoveDelta(pair[1])
		if !vok || !aok {
			t.Errorf("%q or %q returned ok=false", pair[0], pair[1])
			continue
		}
		if vdx != adx || vdy != ady {
			t.Errorf("%q=(%d,%d) != %q=(%d,%d)", pair[0], vdx, vdy, pair[1], adx, ady)
		}
	}
}

func TestLayoutSnapDirectionKeys(t *testing.T) {
	tests := []struct {
		key  string
		want snapDirection
	}{
		{key: "alt+left", want: snapDirectionLeft},
		{key: "alt+right", want: snapDirectionRight},
		{key: "alt+up", want: snapDirectionUp},
		{key: "alt+down", want: snapDirectionDown},
	}

	for _, test := range tests {
		got, ok := layoutSnapDirection(test.key)
		if !ok || got != test.want {
			t.Errorf("layoutSnapDirection(%q) = (%v, %v), want (%v, true)", test.key, got, ok, test.want)
		}
	}
	if _, ok := layoutSnapDirection("right"); ok {
		t.Fatal("expected an unmodified arrow not to be a directional snap")
	}
}

func TestAltArrowSnapsSelectedOutputFromCanvas(t *testing.T) {
	m := Model{
		tab:            tabLayout,
		layoutFocus:    layoutFocusCanvas,
		selectedOutput: 1,
		editOutputs: []editableOutput{
			{Key: "anchor", Name: "DP-1", Enabled: true, Width: 1920, Height: 1080, Scale: 1, X: 0, Y: 0},
			{Key: "selected", Name: "DP-2", Enabled: true, Width: 1280, Height: 720, Scale: 1, X: 3000, Y: 1000},
		},
	}

	_, cmd := m.updateLayoutKeys(tea.KeyMsg{Type: tea.KeyRight, Alt: true})
	if cmd == nil {
		t.Fatal("expected snap hint command")
	}
	if m.editOutputs[1].X != 1920 || m.editOutputs[1].Y != 180 {
		t.Fatalf("expected Alt+Right to snap at 1920,180, got %d,%d", m.editOutputs[1].X, m.editOutputs[1].Y)
	}
}

func TestNumericInputWidthFor(t *testing.T) {
	m := Model{width: 120}
	iccWidth := m.numericInputWidthFor(numericInputICC)
	scaleWidth := m.numericInputWidthFor(numericInputScale)
	floatWidth := m.numericInputWidthFor(numericInputFloat)
	intWidth := m.numericInputWidthFor(numericInputInt)

	if iccWidth <= scaleWidth {
		t.Errorf("ICC width (%d) should be wider than scale width (%d)", iccWidth, scaleWidth)
	}
	if iccWidth < 20 || iccWidth > 60 {
		t.Errorf("ICC width %d outside expected range [20, 60]", iccWidth)
	}
	if scaleWidth < 8 || scaleWidth > 12 {
		t.Errorf("Scale width %d outside expected range [8, 12]", scaleWidth)
	}
	if floatWidth != scaleWidth || intWidth != scaleWidth {
		t.Errorf("float/int widths should match scale: float=%d int=%d scale=%d", floatWidth, intWidth, scaleWidth)
	}
}

func TestScrollLinesToFit(t *testing.T) {
	lines := []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9"}

	tests := []struct {
		name         string
		selectedLine int
		height       int
		wantFirst    string
		wantLen      int
	}{
		{"selected at top, fits", 0, 10, "0", 10},
		{"selected inside viewport", 3, 10, "0", 10},
		{"selected at last visible row", 9, 10, "0", 10},
		{"selected just past viewport", 5, 5, "1", 9},
		{"selected at end", 9, 5, "5", 5},
		{"height zero returns unchanged", 9, 0, "0", 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := scrollLinesToFit(lines, tt.selectedLine, tt.height)
			if len(got) != tt.wantLen {
				t.Errorf("len = %d, want %d", len(got), tt.wantLen)
			}
			if got[0] != tt.wantFirst {
				t.Errorf("first line = %q, want %q", got[0], tt.wantFirst)
			}
		})
	}
}

func TestBuildInspectorTabsTogetherMapAllFields(t *testing.T) {
	m := Model{
		styles: newStyles(),
		editOutputs: []editableOutput{{
			Key:             "test",
			Name:            "DP-1",
			Enabled:         true,
			Modes:           []string{"3840x2160@144Hz"},
			ModeIndex:       0,
			Width:           3840,
			Height:          2160,
			Refresh:         144,
			Scale:           1,
			ActiveWorkspace: "1",
		}},
	}

	seen := make(map[int]bool, len(layoutFields))
	for _, tab := range []inspectorTab{inspectorTabDisplay, inspectorTabColor} {
		m.inspectorTab = tab
		layout := m.buildInspectorLayout(m.editOutputs[0], 60, false)
		for _, field := range inspectorFieldsForTab(tab) {
			row, ok := layout.fieldRows[field]
			if !ok || row < 0 || row >= len(layout.lines) {
				t.Errorf("tab %d field %d (%s) has invalid row %d", tab, field, layoutFields[field], row)
			}
			seen[field] = true
		}
	}
	if len(seen) != len(layoutFields) {
		t.Fatalf("tabs cover %d fields, want %d", len(seen), len(layoutFields))
	}
}

func TestInspectorTabsGroupDisplayAndColorFields(t *testing.T) {
	m := Model{
		styles: newStyles(),
		editOutputs: []editableOutput{{
			Key:     "test",
			Name:    "DP-1",
			Enabled: true,
			Scale:   1,
		}},
	}

	m.inspectorTab = inspectorTabDisplay
	display := m.buildInspectorLayout(m.editOutputs[0], 60, false)
	for _, field := range []int{0, 1, 2, 5, 6, 7, 8, 9} {
		if _, ok := display.fieldRows[field]; !ok {
			t.Errorf("expected display tab to contain %s", layoutFields[field])
		}
	}

	m.inspectorTab = inspectorTabColor
	color := m.buildInspectorLayout(m.editOutputs[0], 60, false)
	for _, field := range []int{3, 4, 10, 14, 18, 19, 20} {
		if _, ok := color.fieldRows[field]; !ok {
			t.Errorf("expected color tab to contain %s", layoutFields[field])
		}
	}
}

func TestInspectorTabsSwitchByKeyboardAndMouse(t *testing.T) {
	m := Model{
		styles:      newStyles(),
		tab:         tabLayout,
		layoutFocus: layoutFocusInspector,
		width:       120,
		height:      32,
		editOutputs: []editableOutput{{Name: "DP-1", Enabled: true, Scale: 1}},
	}

	// Tab walks the panes in the order they appear, so from Display it opens Color.
	m.updateLayoutKeys(tea.KeyMsg{Type: tea.KeyTab})
	if m.inspectorTab != inspectorTabColor || m.inspectorField != inspectorFieldsForTab(inspectorTabColor)[0] {
		t.Fatalf("expected Tab to open Color and select its first field, got tab=%d field=%d", m.inspectorTab, m.inspectorField)
	}

	rect, _ := m.layoutInspectorRect()
	colorX := rect.x + 3 + lipgloss.Width("Display") + 3
	updated, _ := m.updateMouse(mousePressAt(colorX, rect.y))
	got := mustModel(t, updated)
	if got.inspectorTab != inspectorTabColor {
		t.Fatalf("expected Color border label click to select Color, got %d", got.inspectorTab)
	}

	displayX := rect.x + 3
	updated, _ = got.updateMouse(mousePressAt(displayX, rect.y))
	got = mustModel(t, updated)
	if got.inspectorTab != inspectorTabDisplay {
		t.Fatalf("expected Display border label click to select Display, got %d", got.inspectorTab)
	}
}

func TestPaneTitlesSelectTheirLayoutFocus(t *testing.T) {
	m := Model{
		styles:         newStyles(),
		tab:            tabLayout,
		layoutFocus:    layoutFocusCanvas,
		width:          120,
		height:         32,
		editOutputs:    []editableOutput{{Name: "DP-1", Enabled: true, Scale: 1}},
		inspectorField: 0,
	}

	inspectorRect, _ := m.layoutInspectorRect()
	updated, _ := m.updateMouse(mousePressAt(inspectorRect.x+3, inspectorRect.y))
	got := mustModel(t, updated)
	if got.layoutFocus != layoutFocusInspector || got.inspectorTab != inspectorTabDisplay {
		t.Fatalf("expected Display title to focus inspector Display tab, got focus=%d tab=%d", got.layoutFocus, got.inspectorTab)
	}

	canvasRect, _ := got.layoutCanvasRect()
	updated, _ = got.updateMouse(mousePressAt(canvasRect.x+3, canvasRect.y))
	got = mustModel(t, updated)
	if got.layoutFocus != layoutFocusCanvas {
		t.Fatalf("expected Monitor Layout title to focus canvas, got %d", got.layoutFocus)
	}
}

func TestInspectorColumnPlacesInfoAbovePreferences(t *testing.T) {
	m := Model{
		styles:      newStyles(),
		tab:         tabLayout,
		layoutFocus: layoutFocusInspector,
		editOutputs: []editableOutput{{Name: "DP-1", Enabled: true, Scale: 1}},
	}
	view := ansi.Strip(m.renderInspectorColumn(48, 30, false))
	info := strings.Index(view, "Info")
	display := strings.Index(view, "Display - Color")
	if info < 0 || display < 0 || info > display {
		t.Fatalf("expected Info above Display - Color, got:\n%s", view)
	}
}

func TestBuildInspectorLayoutUniqueRows(t *testing.T) {
	m := Model{
		styles: newStyles(),
		editOutputs: []editableOutput{{
			Key:     "test",
			Name:    "DP-1",
			Enabled: true,
			Scale:   1,
		}},
	}
	for _, tab := range []inspectorTab{inspectorTabDisplay, inspectorTabColor} {
		m.inspectorTab = tab
		layout := m.buildInspectorLayout(m.editOutputs[0], 60, false)
		seen := make(map[int]int)
		for _, idx := range inspectorFieldsForTab(tab) {
			row := layout.fieldRows[idx]
			if other, exists := seen[row]; exists {
				t.Errorf("tab=%d: field %d and %d share row %d", tab, other, idx, row)
			}
			seen[row] = idx
		}
	}
}

func TestAdjustInspectorScaleKeepsNeighborsFlush(t *testing.T) {
	m := Model{
		selectedOutput: 0,
		inspectorField: 2,
		editOutputs: []editableOutput{
			{Key: "internal", Name: "eDP-1", Enabled: true, Width: 1920, Height: 1080, Scale: 2, X: 0, Y: 0},
			{Key: "middle", Name: "DP-1", Enabled: true, Width: 1920, Height: 1080, Scale: 1, X: 960, Y: 0},
			{Key: "right", Name: "DP-2", Enabled: true, Width: 1280, Height: 720, Scale: 1, X: 2880, Y: 0},
		},
	}

	before, _ := m.editOutputs[0].logicalSize()
	m.adjustInspectorField(-1)
	after, _ := m.editOutputs[0].logicalSize()
	if after == before {
		t.Fatalf("expected the scale change to resize the output, still %d wide", after)
	}
	shift := after - before

	if got, want := m.editOutputs[1].X, 960+shift; got != want {
		t.Fatalf("flush neighbor X = %d, want %d", got, want)
	}
	if got, want := m.editOutputs[2].X, 2880+shift; got != want {
		t.Fatalf("downstream neighbor X = %d, want %d", got, want)
	}
	if got, want := m.editOutputs[1].X, m.editOutputs[0].X+after; got != want {
		t.Fatalf("neighbor no longer flush against the resized output: X = %d, right edge = %d", got, want)
	}
}

func TestAdjustInspectorScalePreservesGapsAndLeavesEarlierOutputsAlone(t *testing.T) {
	m := Model{
		selectedOutput: 1,
		inspectorField: 2,
		editOutputs: []editableOutput{
			{Key: "left", Name: "DP-1", Enabled: true, Width: 1920, Height: 1080, Scale: 1, X: 0, Y: 0},
			{Key: "selected", Name: "eDP-1", Enabled: true, Width: 1920, Height: 1080, Scale: 2, X: 1920, Y: 0},
			{Key: "spaced", Name: "DP-2", Enabled: true, Width: 1280, Height: 720, Scale: 1, X: 3000, Y: 0},
		},
	}

	before, _ := m.editOutputs[1].logicalSize()
	m.adjustInspectorField(-1)
	after, _ := m.editOutputs[1].logicalSize()
	shift := after - before
	if shift == 0 {
		t.Fatal("expected the scale change to resize the output")
	}

	if got := m.editOutputs[0].X; got != 0 {
		t.Fatalf("output left of the resized one moved to %d", got)
	}
	gapBefore := 3000 - (1920 + before)
	gapAfter := m.editOutputs[2].X - (m.editOutputs[1].X + after)
	if gapAfter != gapBefore {
		t.Fatalf("gap changed from %d to %d", gapBefore, gapAfter)
	}
}

func TestAdjustInspectorScaleReflowsVertically(t *testing.T) {
	m := Model{
		selectedOutput: 0,
		inspectorField: 2,
		editOutputs: []editableOutput{
			{Key: "top", Name: "DP-1", Enabled: true, Width: 1920, Height: 1080, Scale: 2, X: 0, Y: 0},
			{Key: "below", Name: "DP-2", Enabled: true, Width: 1920, Height: 1080, Scale: 1, X: 0, Y: 540},
		},
	}

	_, before := m.editOutputs[0].logicalSize()
	m.adjustInspectorField(-1)
	_, after := m.editOutputs[0].logicalSize()
	if after == before {
		t.Fatal("expected the scale change to resize the output")
	}

	if got, want := m.editOutputs[1].Y, m.editOutputs[0].Y+after; got != want {
		t.Fatalf("output below is no longer flush: Y = %d, bottom edge = %d", got, want)
	}
}

func TestAdjustInspectorModeReflowsNeighbors(t *testing.T) {
	m := Model{
		selectedOutput: 0,
		inspectorField: 1,
		editOutputs: []editableOutput{
			{
				Key: "selected", Name: "DP-1", Enabled: true,
				Width: 1920, Height: 1080, Refresh: 60, Scale: 1, X: 0, Y: 0,
				Modes: []string{"1920x1080@60.00", "1280x720@60.00"}, ModeIndex: 0,
			},
			{Key: "neighbor", Name: "DP-2", Enabled: true, Width: 1920, Height: 1080, Scale: 1, X: 1920, Y: 0},
		},
	}

	m.adjustInspectorField(1)

	if got := m.editOutputs[0].Width; got != 1280 {
		t.Fatalf("mode change did not apply, width = %d", got)
	}
	if got := m.editOutputs[1].X; got != 1280 {
		t.Fatalf("neighbor X = %d, want 1280", got)
	}
}

func TestAdjustInspectorScaleIgnoresDisabledOutputs(t *testing.T) {
	m := Model{
		selectedOutput: 0,
		inspectorField: 2,
		editOutputs: []editableOutput{
			{Key: "selected", Name: "eDP-1", Enabled: true, Width: 1920, Height: 1080, Scale: 2, X: 0, Y: 0},
			{Key: "off", Name: "DP-1", Enabled: false, Width: 1920, Height: 1080, Scale: 1, X: 960, Y: 0},
		},
	}

	m.adjustInspectorField(-1)

	if got := m.editOutputs[1].X; got != 960 {
		t.Fatalf("disabled output moved to %d", got)
	}
}

func TestAdjustInspectorPositionDoesNotReflow(t *testing.T) {
	m := Model{
		selectedOutput: 0,
		inspectorField: 7,
		editOutputs: []editableOutput{
			{Key: "selected", Name: "DP-1", Enabled: true, Width: 1920, Height: 1080, Scale: 1, X: 0, Y: 0},
			{Key: "neighbor", Name: "DP-2", Enabled: true, Width: 1920, Height: 1080, Scale: 1, X: 1920, Y: 0},
		},
	}

	m.adjustInspectorField(1)

	if got := m.editOutputs[0].X; got != 10 {
		t.Fatalf("selected output X = %d, want 10", got)
	}
	if got := m.editOutputs[1].X; got != 1920 {
		t.Fatalf("moving an output shifted its neighbor to %d", got)
	}
}

func TestCommitNumericScaleReflowsNeighbors(t *testing.T) {
	m := Model{
		styles: newStyles(),
		width:  120,
		height: 28,
		mode:   modeNumericInput,
		input: &numericInputState{
			Kind:        numericInputScale,
			OutputIndex: 0,
			Input:       textInputWithValue("2"),
		},
		editOutputs: []editableOutput{
			{Key: "selected", Name: "eDP-1", Enabled: true, Width: 1920, Height: 1080, Scale: 1, X: 0, Y: 0},
			{Key: "neighbor", Name: "DP-1", Enabled: true, Width: 1920, Height: 1080, Scale: 1, X: 1920, Y: 0},
		},
	}

	if cmd := m.commitNumericInput(); cmd != nil {
		t.Fatal("expected scale commit not to return a command")
	}

	if got := m.editOutputs[0].Scale; got != 2 {
		t.Fatalf("typed scale = %v, want 2", got)
	}
	if got := m.editOutputs[1].X; got != 960 {
		t.Fatalf("neighbor X = %d, want 960: typing a scale should reflow like the arrows do", got)
	}
}

func TestCommitNumericPositionDoesNotReflow(t *testing.T) {
	m := Model{
		styles: newStyles(),
		width:  120,
		height: 28,
		mode:   modeNumericInput,
		input: &numericInputState{
			Kind:        numericInputPositionX,
			OutputIndex: 0,
			Input:       textInputWithValue("500"),
		},
		editOutputs: []editableOutput{
			{Key: "selected", Name: "eDP-1", Enabled: true, Width: 1920, Height: 1080, Scale: 1, X: 0, Y: 0},
			{Key: "neighbor", Name: "DP-1", Enabled: true, Width: 1920, Height: 1080, Scale: 1, X: 1920, Y: 0},
		},
	}

	if cmd := m.commitNumericInput(); cmd != nil {
		t.Fatal("expected position commit not to return a command")
	}

	if got := m.editOutputs[0].X; got != 500 {
		t.Fatalf("typed position = %d, want 500", got)
	}
	if got := m.editOutputs[1].X; got != 1920 {
		t.Fatalf("neighbor moved to %d: repositioning is not a resize", got)
	}
}

func TestFooterTellsTheInspectorHowToChangeAValue(t *testing.T) {
	inspector := Model{tab: tabLayout, layoutFocus: layoutFocusInspector}.footerHelpText()
	for _, want := range []string{"`←→` adjust", "`Enter` type", "`[ ]` monitors"} {
		if !strings.Contains(inspector, want) {
			t.Fatalf("inspector footer missing %q: %s", want, inspector)
		}
	}

	canvas := Model{tab: tabLayout, layoutFocus: layoutFocusCanvas}.footerHelpText()
	if !strings.Contains(canvas, "`drag/arrows` move") {
		t.Fatalf("canvas footer should still describe moving monitors: %s", canvas)
	}

	workspaces := Model{
		tab: tabWorkspaces,
		workspaceEdit: workspaceEditor{
			Enabled:  true,
			Strategy: profile.WorkspaceStrategySequential,
		},
	}.footerHelpText()
	if !strings.Contains(workspaces, "`Enter` type count") {
		t.Fatalf("workspace footer should advertise exact count entry: %s", workspaces)
	}
}

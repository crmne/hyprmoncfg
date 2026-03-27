package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/crmne/hyprmoncfg/internal/buildinfo"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

func TestRenderMainIncludesRefreshedChrome(t *testing.T) {
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
	if !strings.Contains(view, "Hyprland monitor layout and workspace planner") {
		t.Fatalf("expected refreshed title bar in view, got:\n%s", view)
	}
	if !strings.Contains(view, "Drag cards to reposition monitors.") {
		t.Fatalf("expected layout guidance in view, got:\n%s", view)
	}
	if !strings.Contains(view, "Current setup") {
		t.Fatalf("expected current-setup badge in view, got:\n%s", view)
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
		width:       120,
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
	for _, want := range []string{"Ask", "Donate", "v1.2.3", sponsorURL, communityURL, releasesURL} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected footer to include %q, got:\n%s", want, view)
		}
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
	if width := lipgloss.Width(bar); width > 118 {
		t.Fatalf("expected footer bar to fit width 118, got %d", width)
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

	m := Model{styles: newStyles(), width: 120, height: 24}
	layout := m.footerLayout()
	if len(layout.links) != 3 {
		t.Fatalf("expected 3 clickable footer links, got %+v", layout.links)
	}

	askX := m.footerColumnX() + layout.links[0].start
	link, ok := m.footerLinkAt(askX, m.footerRowY())
	if !ok || link.label != "Ask" || link.url != communityURL {
		t.Fatalf("expected Ask hit, got ok=%v link=%+v", ok, link)
	}

	versionX := m.footerColumnX() + strings.LastIndex(layout.text, "v1.2.3")
	link, ok = m.footerLinkAt(versionX, m.footerRowY())
	if !ok || link.label != "v1.2.3" || link.url != releasesURL {
		t.Fatalf("expected version hit, got ok=%v link=%+v", ok, link)
	}
}

func TestFooterClickRunsBrowserOpenCommand(t *testing.T) {
	prevVersion := buildinfo.Version
	buildinfo.Version = "1.2.3"
	defer func() { buildinfo.Version = prevVersion }()

	var opened string
	m := Model{
		styles:  newStyles(),
		width:   120,
		height:  24,
		tab:     tabLayout,
		openURL: func(url string) error { opened = url; return nil },
	}

	layout := m.footerLayout()
	donate := layout.links[1]
	msg := tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
		X:      m.footerColumnX() + donate.start,
		Y:      m.footerRowY(),
	}

	nextModel, cmd := m.updateMouse(msg)
	if cmd == nil {
		t.Fatal("expected footer click to return open-url command")
	}

	result := cmd()
	openMsg, ok := result.(openURLMsg)
	if !ok {
		t.Fatalf("expected openURLMsg, got %T", result)
	}
	if opened != sponsorURL {
		t.Fatalf("expected donate click to open %q, got %q", sponsorURL, opened)
	}

	updated, _ := nextModel.(Model).Update(openMsg)
	got := updated.(Model)
	if got.status != "Opened Donate in browser" || got.statusErr {
		t.Fatalf("expected success status after opening link, got status=%q err=%v", got.status, got.statusErr)
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

func TestCanvasLegendMatchesCanvasCardColors(t *testing.T) {
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

	view := m.renderCanvasPane(80, 12)

	expected := []string{
		renderCanvasLegendItem("Selected", m.canvasCardStyle(editableOutput{Enabled: true}, true)),
		renderCanvasLegendItem("Enabled", m.canvasCardStyle(editableOutput{Enabled: true}, false)),
		renderCanvasLegendItem("Disabled", m.canvasCardStyle(editableOutput{Enabled: false}, false)),
	}
	for _, item := range expected {
		if !strings.Contains(view, item) {
			t.Fatalf("expected legend to include canvas-matched item %q, got:\n%s", item, view)
		}
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

	modeModel, _ := base.activateInspectorField()
	gotMode := modeModel.(Model)
	if gotMode.mode != modeMain {
		t.Fatalf("enabled row should toggle inline, got mode %v", gotMode.mode)
	}

	base.inspectorField = 1
	modeModel, _ = base.activateInspectorField()
	gotMode = modeModel.(Model)
	if gotMode.mode != modeModePicker || gotMode.picker == nil {
		t.Fatalf("expected mode picker to open, got mode %v picker %+v", gotMode.mode, gotMode.picker)
	}

	base.inspectorField = 2
	scaleModel, _ := base.activateInspectorField()
	gotScale := scaleModel.(Model)
	if gotScale.mode != modeNumericInput || gotScale.input == nil {
		t.Fatalf("expected numeric input to open, got mode %v input %+v", gotScale.mode, gotScale.input)
	}

	base.inspectorField = 5
	posXModel, _ := base.activateInspectorField()
	gotPosX := posXModel.(Model)
	if gotPosX.mode != modeNumericInput || gotPosX.input == nil || gotPosX.input.Kind != numericInputPositionX {
		t.Fatalf("expected position X input to open, got mode %v input %+v", gotPosX.mode, gotPosX.input)
	}

	base.inspectorField = 6
	posYModel, _ := base.activateInspectorField()
	gotPosY := posYModel.(Model)
	if gotPosY.mode != modeNumericInput || gotPosY.input == nil || gotPosY.input.Kind != numericInputPositionY {
		t.Fatalf("expected position Y input to open, got mode %v input %+v", gotPosY.mode, gotPosY.input)
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
	if len(lines) != 5 {
		t.Fatalf("expected 5 card lines, got %d", len(lines))
	}
	if lines[1].text != "Microstep MPG321UR-QD" {
		t.Fatalf("expected make+model on card, got %q", lines[1].text)
	}
	if lines[4].text != "pos 0,0" {
		t.Fatalf("expected position line on card, got %q", lines[4].text)
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
		status: "Loaded 2 monitors and 3 profiles",
	}

	view := m.renderMain()
	if width := maxRenderedLineWidth(view); width > m.width {
		t.Fatalf("expected short main view to fit width %d, got max line width %d", m.width, width)
	}
	if height := lipgloss.Height(view); height != m.height {
		t.Fatalf("expected short main view to fill height %d, got %d", m.height, height)
	}
	if !strings.Contains(view, "Loaded 2 monitors and 3 profiles") {
		t.Fatalf("expected status to remain visible below the body, got:\n%s", view)
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

	view := m.renderInspectorPane(48, 8)
	for _, want := range []string{"Mode", "3840x2160@143.99Hz", "Scale", "VRR", "X", "Y"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected compact inspector to include %q, got:\n%s", want, view)
		}
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
		status: "Loaded 1 monitors and 3 profiles",
	}

	view := m.renderMain()
	if width := maxRenderedLineWidth(view); width > m.width {
		t.Fatalf("expected tall medium-width view to fit width %d, got max line width %d", m.width, width)
	}
	if height := lipgloss.Height(view); height != m.height {
		t.Fatalf("expected tall medium-width view to fill height %d, got %d", m.height, height)
	}
	if !strings.Contains(view, "Loaded 1 monitors and 3 profiles") {
		t.Fatalf("expected status to remain visible below the body, got:\n%s", view)
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
	m := Model{width: 140}
	if !m.useCompactLayout(30) {
		t.Fatal("expected 140-column terminal to stay in compact layout")
	}

	m.width = 150
	if m.useCompactLayout(30) {
		t.Fatal("expected 150-column terminal to allow side-by-side layout")
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

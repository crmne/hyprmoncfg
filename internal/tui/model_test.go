package tui

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/crmne/hyprmoncfg/internal/buildinfo"
	"github.com/crmne/hyprmoncfg/internal/hypr"
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
	if !strings.Contains(view, "Monitor Layout") {
		t.Fatalf("expected Monitor Layout header in view, got:\n%s", view)
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

	m := Model{styles: newStyles(), width: 160, height: 24}
	layout := m.footerLayout()
	if len(layout.links) < 3 {
		t.Fatalf("expected at least 3 clickable footer links, got %+v", layout.links)
	}

	// Find the Ask link — simulate a real click on the rendered footer,
	// which is shifted right by the badge padding added during decoration.
	var askFound bool
	for _, link := range layout.links {
		if link.label == "Ask" && link.url == communityURL {
			lx := m.footerColumnX() + m.badgeExtraWidth() + link.start
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

		x := m.footerColumnX() + m.badgeExtraWidth() + offset
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

	x, y := findVisiblePosition(t, m.renderMain(), "Updated:")
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
	if got.workspaceEdit.SelectedField != 2 {
		t.Fatalf("expected visible click on Max workspaces to select field 2, got %d", got.workspaceEdit.SelectedField)
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

	x, y := findVisiblePosition(t, m.renderMain(), "HDMI-A-1: 4, 5, 6")
	updated, _ := m.updateMouse(mousePressAt(x, y))
	got := updated.(Model)
	if got.workspaceEdit.SelectedField != 1 {
		t.Fatalf("expected compact preview-pane click to keep selected field 1, got %d", got.workspaceEdit.SelectedField)
	}
	if got.workspaceEdit.MaxWorkspaces != 6 {
		t.Fatalf("expected compact preview-pane click to leave workspace settings unchanged, got %d", got.workspaceEdit.MaxWorkspaces)
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
		width:          100,
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

	for _, label := range []string{"Legend", "Selected", "Enabled"} {
		if !strings.Contains(view, label) {
			t.Fatalf("expected legend to include %q, got:\n%s", label, view)
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

	x, y := findVisiblePosition(t, m.View(), "2560x1440@143.97Hz")
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

func TestRenderCanvasPaneShowsMirrorSummary(t *testing.T) {
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
	if !strings.Contains(view, "Mirrors: HDMI-A-1 -> DP-1") {
		t.Fatalf("expected mirror summary in canvas pane, got:\n%s", view)
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

func TestProfileExecEditorEnterUpdatesSelectedProfileInMemory(t *testing.T) {
	m := Model{
		styles:   newStyles(),
		tab:      tabProfiles,
		profiles: []profile.Profile{{Name: "Desk Dock"}},
	}

	updatedModel, _ := m.updateProfileKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	got := updatedModel.(Model)
	got.execInput.Input.SetValue("/path/to/script.sh")

	nextModel, _ := got.updateProfileExecInputKeys(tea.KeyMsg{Type: tea.KeyEnter})
	next := nextModel.(*Model)

	if next.mode != modeMain {
		t.Fatalf("expected exec editor to close after Enter, got %v", next.mode)
	}
	if next.execInput != nil {
		t.Fatalf("expected exec input to be cleared, got %+v", next.execInput)
	}
	if next.profiles[0].Exec != "/path/to/script.sh" {
		t.Fatalf("expected exec to update in memory, got %q", next.profiles[0].Exec)
	}
	if !strings.Contains(next.status, "Press s to save") {
		t.Fatalf("expected status to mention explicit save, got %q", next.status)
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
	if !strings.Contains(view, "Preferences") {
		t.Fatalf("expected Preferences section to be visible in inspector, got:\n%s", view)
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
	for _, want := range []string{"Mode", "3840x2160@143.99Hz", "Scale", "VRR", "Position X", "Position Y"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected inspector to include %q, got:\n%s", want, view)
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
	}

	view := m.renderMain()
	if width := maxRenderedLineWidth(view); width > m.width {
		t.Fatalf("expected tall medium-width view to fit width %d, got max line width %d", m.width, width)
	}
	if height := lipgloss.Height(view); height != m.height {
		t.Fatalf("expected tall medium-width view to fill height %d, got %d", m.height, height)
	}
	if !strings.Contains(view, "Preferences") {
		t.Fatalf("expected Preferences section visible in view, got:\n%s", view)
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

func TestRenderWorkspaceViewShowsPreviewWhenDisabled(t *testing.T) {
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

	view := m.renderWorkspaceView(16)
	for _, want := range []string{
		"(workspace rules disabled; preview only)",
		"DP-1: 1, 2, 3",
		"HDMI-A-1: 4, 5, 6",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected workspace view to include %q, got:\n%s", want, view)
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
			SelectedField:           1,
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
	for _, want := range []string{"DP-1: 1, 2, 3", "HDMI-A-1: 4, 5, 6"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected sequential preview to include %q after strategy switch, got:\n%s", want, view)
		}
	}
}

func TestAdjustWorkspaceFieldPreservesCustomSequentialGroupSize(t *testing.T) {
	m := Model{
		workspaceEdit: workspaceEditor{
			Strategy:                profile.WorkspaceStrategySequential,
			GroupSize:               2,
			LastSequentialGroupSize: 2,
			SelectedField:           1,
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
		inspectorField: 3,
	}

	// j moves down
	m.updateLayoutKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.inspectorField != 4 {
		t.Errorf("j: inspectorField = %d, want 4", m.inspectorField)
	}

	// k moves up
	m.updateLayoutKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if m.inspectorField != 3 {
		t.Errorf("k: inspectorField = %d, want 3", m.inspectorField)
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

func TestBuildInspectorLayoutMapsAllFields(t *testing.T) {
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

	for _, compact := range []bool{false, true} {
		name := "full"
		if compact {
			name = "compact"
		}
		t.Run(name, func(t *testing.T) {
			layout := m.buildInspectorLayout(m.editOutputs[0], 60, compact)
			if len(layout.fieldRows) != len(layoutFields) {
				t.Fatalf("fieldRows has %d entries, want %d", len(layout.fieldRows), len(layoutFields))
			}
			for idx := range layoutFields {
				row, ok := layout.fieldRows[idx]
				if !ok {
					t.Errorf("field %d (%s) missing from fieldRows", idx, layoutFields[idx])
					continue
				}
				if row < 0 || row >= len(layout.lines) {
					t.Errorf("field %d row %d out of range [0, %d)", idx, row, len(layout.lines))
				}
			}
		})
	}
}

func TestBuildInspectorLayoutSpacerBeforeAdvanced(t *testing.T) {
	m := Model{
		styles: newStyles(),
		editOutputs: []editableOutput{{
			Key:     "test",
			Name:    "DP-1",
			Enabled: true,
			Scale:   1,
		}},
	}
	layout := m.buildInspectorLayout(m.editOutputs[0], 60, false)

	lastBase := layout.fieldRows[advancedFieldStart-1]
	firstAdvanced := layout.fieldRows[advancedFieldStart]
	if firstAdvanced-lastBase < 2 {
		t.Errorf("expected spacer row between field %d and %d: got rows %d and %d", advancedFieldStart-1, advancedFieldStart, lastBase, firstAdvanced)
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
	for _, compact := range []bool{false, true} {
		layout := m.buildInspectorLayout(m.editOutputs[0], 60, compact)
		seen := make(map[int]int)
		for idx := range layoutFields {
			row := layout.fieldRows[idx]
			if other, exists := seen[row]; exists {
				t.Errorf("compact=%v: field %d and %d share row %d", compact, other, idx, row)
			}
			seen[row] = idx
		}
	}
}

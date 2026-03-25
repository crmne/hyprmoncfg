package tui

import (
	"strings"
	"testing"

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

package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

func TestOffDisplayRowsSelectAndEnableDraftOnly(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {113, 33}} {
		for _, allOff := range []bool{false, true} {
			off := paneTestSide
			off.Disabled = true
			desk := paneTestDesk
			desk.Disabled = allOff
			m := paneTestModel(t, tabLayout, []hypr.Monitor{desk, off}, nil)
			m.width, m.height = size[0], size[1]
			x, y := findVisiblePosition(t, m.View(), "DP-2  Off")
			next, cmd := m.updateMouse(mousePressAt(x, y))
			selected := next.(*Model)
			if cmd != nil || selected.editOutputs[selected.selectedOutput].Name != "DP-2" || selected.editOutputs[selected.selectedOutput].Enabled || selected.drag != nil {
				t.Fatal("selection must only inspect")
			}
			rows := selected.hiddenDisplayRows(70, 20)
			layout := selected.canvasLayout(70, 20)
			for _, rect := range layout.rects {
				if rect.y < len(rows) {
					t.Fatal("rows overlap geometry")
				}
			}
			// Find this output's action on its row, not another disabled display.
			viewLines := strings.Split(ansi.Strip(selected.View()), "\n")
			x = strings.Index(viewLines[y], "[Enable]")
			next, cmd = selected.updateMouse(mousePressAt(x, y))
			enabled := next.(*Model)
			if cmd != nil || !enabled.editOutputs[enabled.selectedOutput].Enabled || !enabled.monitors[1].Disabled {
				t.Fatal("enable must edit only the draft")
			}
		}
	}
}

func TestDisconnectedRowHasNoEnableAction(t *testing.T) {
	m := paneTestModel(t, tabLayout, []hypr.Monitor{paneTestDesk, paneTestSide}, nil)
	for i := range m.editOutputs {
		if m.editOutputs[i].Name == paneTestSide.Name {
			m.editOutputs[i].Enabled = false
		}
	}
	m.monitors = m.monitors[:1]
	rows := m.hiddenDisplayRows(70, 20)
	if len(rows) != 1 || rows[0].action != "" || !strings.Contains(rows[0].label, "Not connected") {
		t.Fatal(rows)
	}
}

func TestProfileActionButtonsAndCommandAreDiscoverable(t *testing.T) {
	p := profile.FromState("Desk", []hypr.Monitor{paneTestDesk}, nil)
	for _, size := range [][2]int{{80, 24}, {113, 33}} {
		m := paneTestModel(t, tabProfiles, []hypr.Monitor{paneTestDesk}, []profile.Profile{p})
		m.width, m.height = size[0], size[1]
		requireContains(t, ansi.Strip(m.View()), "[Preview]", "[Edit]", "[Delete]", "Status", "[Edit command]")
		_, autoY := findVisiblePosition(t, m.View(), "Automatic profile selection")
		_, listY := findVisiblePosition(t, m.View(), "Saved Profiles")
		rowY := m.profilesListRect().inner(m.styles.activePane).y + profileListHeaderRows
		_, actionY := findVisiblePosition(t, m.View(), "[Preview]")
		if listY != autoY+profileAutomaticPaneHeight || actionY <= rowY {
			t.Fatal("separate automatic pane and actions below rows required")
		}
		if lines := strings.Count(ansi.Strip(m.View()), "\n") + 1; lines > m.height {
			t.Fatal("profile view overflow", lines, m.height)
		}
		listRect := m.profilesListRect()
		if m.profileAutomaticRect().w != listRect.w {
			t.Fatal("automatic selection must match list width")
		}
		if m.terminalWidth() >= 96 {
			_, detailY := findVisiblePosition(t, m.View(), "Profile Details")
			if detailY != autoY {
				t.Fatal("details must start beside automatic selection")
			}
			if m.profileAutomaticRect().contains(listRect.x+listRect.w+2, autoY+1) {
				t.Fatal("automatic click target extends into details")
			}
		}
		if !listRect.contains(listRect.x, rowY) {
			t.Fatal("list pointer geometry missed visible row")
		}
		x, y := findVisiblePosition(t, m.View(), "[Delete]")
		next, cmd := m.updateMouse(mousePressAt(x, y))
		confirm := next.(Model)
		if cmd != nil || confirm.mode != modeDeleteConfirm {
			t.Fatal("delete must ask first")
		}
		closed, _ := confirm.Update(tea.KeyMsg{Type: tea.KeyEsc})
		if len(closed.(Model).profiles) != 1 {
			t.Fatal("cancel removed profile")
		}
	}
}

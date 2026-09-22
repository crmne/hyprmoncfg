package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

// Issue #60: exercise terminal cell sizes, not a claimed physical pixel mode.
func TestIssue60PageAndFooterSizeMatrix(t *testing.T) {
	monitors := []hypr.Monitor{paneTestDesk, paneTestSide}
	var profiles []profile.Profile
	for i := 0; i < 20; i++ {
		profiles = append(profiles, profile.FromState(fmt.Sprintf("Saved arrangement %02d", i), monitors, nil))
	}
	for _, size := range [][2]int{{80, 24}, {90, 24}, {96, 24}, {113, 33}, {136, 38}, {80, 16}} {
		for _, tab := range []mainTab{tabLayout, tabWorkspaces, tabProfiles} {
			t.Run(fmt.Sprintf("%dx%d/page%d", size[0], size[1], tab), func(t *testing.T) {
				m := paneTestModel(t, tab, monitors, profiles)
				m.width, m.height = size[0], size[1]
				m.selectedProfile = len(profiles) - 1
				view := m.View()
				if w := maxRenderedLineWidth(view); w > m.width {
					t.Errorf("width %d exceeds %d", w, m.width)
				}
				if h := lipgloss.Height(view); h > m.height {
					t.Errorf("height %d exceeds %d", h, m.height)
				}
				lines := strings.Split(ansi.Strip(view), "\n")
				if size == [2]int{80, 24} {
					t.Logf("80x24 footer: %s", lines[len(lines)-1])
				}
				footer := lines[len(lines)-1]
				if !strings.Contains(footer, "? keys") {
					t.Fatal("help shortcut hidden", footer)
				}
				if tab != tabProfiles && !strings.Contains(footer, "s save") {
					t.Fatal("save shortcut hidden", footer)
				}
				if !strings.Contains(lines[len(lines)-1], "Donate") {
					t.Errorf("footer missing from last line: %q", lines[len(lines)-1])
				}
				if tab == tabProfiles {
					requireContains(t, ansi.Strip(view), profiles[m.selectedProfile].Name, "[Preview]", "[Edit]", "[Delete]")
				}
			})
		}
	}
}

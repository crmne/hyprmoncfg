package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/crmne/hyprmoncfg/internal/profile"
)

type keyBinding struct {
	keys   string
	action string
}

type keyGroup struct {
	title    string
	bindings []keyBinding
}

// keyGroupsFor lists what the keys do, starting with the tab in front of you.
// The footer only has room for the handful you reach for constantly, so this is
// where the rest lives.
func (m Model) keyGroupsFor(tab mainTab) []keyGroup {
	groups := make([]keyGroup, 0, 3)

	switch tab {
	case tabLayout:
		// The heading names what these act on, so each line can stay parallel
		// instead of the first one carrying the subject for all of them.
		groups = append(groups,
			keyGroup{
				title: "Selected monitor",
				bindings: []keyBinding{
					{"drag, arrows", "Move by 100px"},
					{"Shift+arrows", "Move by 10px"},
					{"Ctrl+arrows", "Move by 1px"},
					{"Alt+arrows", "Snap beside the nearest monitor"},
					{"0", "Move to 0,0"},
					{"[ ]", "Select the previous or next monitor"},
					{"Space", "Enable or disable the selected monitor in the draft"},
				},
			},
			keyGroup{
				title: "Layout",
				bindings: []keyBinding{
					{"Tab, Shift+Tab", "Move between the canvas, Display, and Color"},
					{"Enter", "Edit the selected field"},
				},
			},
		)
	case tabProfiles:
		groups = append(groups, keyGroup{
			title: "Selected profile",
			bindings: []keyBinding{
				{"↑ ↓, click", "Browse profiles and preview their saved setup"},
				{"Enter, a", "Preview this profile"},
				{"l", "Load it into the layout editor"},
				{"e", "Edit post-apply command"},
				{"d", "Delete it"},
			},
		})
	case tabWorkspaces:
		adjustAction := "Adjust it, or reorder monitors"
		if m.workspaceEdit.Strategy == profile.WorkspaceStrategyManual {
			adjustAction = "Adjust it, or assign a workspace to a monitor"
		}
		groups = append(groups, keyGroup{
			title: "Workspaces",
			bindings: []keyBinding{
				{"↑ ↓", "Select a setting, workspace, or monitor"},
				{"PgUp PgDn", "Move one visible page"},
				{"Home End", "Jump to the first or last row"},
				{"← →", adjustAction},
				{"Enter", "Type an exact workspace or group count"},
			},
		})
	}

	return append(groups, keyGroup{
		title: "Anywhere",
		bindings: []keyBinding{
			{"1 2 3", "Switch tabs"},
			{"a", "Apply the current draft or selected profile"},
			{"s", "Save the current draft as a profile"},
			{"U", fmt.Sprintf("Disable displays outside the draft: %t (toggle)", m.disableUnknownOutputs)},
			{"r", "Reset from live Hyprland state"},
			{"?", "Show these keys"},
			{"R", "Restart the daemon after an upgrade"},
			{"q", "Quit"},
		},
	})
}

func (m Model) renderKeybindings() string {
	groups := m.keyGroupsFor(m.tab)

	width := 0
	for _, group := range groups {
		for _, binding := range group.bindings {
			width = max(width, lipgloss.Width(binding.keys))
		}
	}

	body := make([]string, 0, 16)
	keyStyle := withFG(lipgloss.NewStyle().Bold(true), "2")
	for idx, group := range groups {
		if idx > 0 {
			body = append(body, "")
		}
		body = append(body, m.styles.label.Render(group.title))
		for _, binding := range group.bindings {
			body = append(body, fmt.Sprintf("  %s  %s",
				keyStyle.Render(fmt.Sprintf("%-*s", width, binding.keys)),
				m.styles.value.Render(binding.action),
			))
		}
	}
	body = append(body, "", m.styles.help.Render("Any key closes this."))

	return m.renderModalFrame("Keys", []string{strings.Join(body, "\n")})
}

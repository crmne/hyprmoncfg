package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/crmne/hyprmoncfg/internal/buildinfo"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

const (
	daemonURL      = "https://hyprmoncfg.dev/daemon/"
	repoURL        = "https://github.com/crmne/hyprmoncfg"
	releasesURL    = repoURL + "/releases"
	sponsorURL     = "https://github.com/sponsors/crmne"
	communityURL   = repoURL + "/discussions"
	footerMinHelpW = 20
)

type footerItem struct {
	label string
	url   string
}

type footerLinkRegion struct {
	label string
	url   string
	start int
	end   int
}

type footerLayout struct {
	text  string
	links []footerLinkRegion
}

func (m Model) footerHelpText() string {
	// One key, one meaning. Everything else lives behind `?`.
	switch m.tab {
	case tabLayout:
		// The inspector is where scale, mode, and position are edited, so say
		// how to change a value rather than how to move a monitor around.
		if m.layoutFocus == layoutFocusInspector {
			return "`↑↓` field | `←→` adjust | `Enter` type | `[ ]` monitors | `Tab` pane | `a` apply | `s` save | `?` keys"
		}
		return "`drag/arrows` move | `[ ]` monitors | `Tab` pane | `Enter` edit | `a` apply | `s` save | `?` keys"
	case tabProfiles:
		if m.profileAutomatic() {
			return "`↑↓` browse | `Enter` preview | `l` edit | `e` post-apply command | `d` delete | `?` keys"
		}
		return "`↑↓` browse | `Enter` preview | `l` edit | `e` post-apply command | `d` delete | `?` keys"
	case tabWorkspaces:
		if !m.workspaceEdit.Enabled {
			return "`↑↓` select | `←→` adjust | `a` apply | `s` save | `?` keys"
		}
		if m.workspaceEdit.Strategy == profile.WorkspaceStrategyManual {
			return "`↑↓` select | `←→` assign | `Enter` type count | `a` apply | `s` save | `?` keys"
		}
		return "`↑↓` select | `←→` adjust | `Enter` type count | `a` apply | `s` save | `?` keys"
	default:
		return ""
	}
}

func joinFooterItems(items []footerItem) string {
	if len(items) == 0 {
		return ""
	}
	labels := make([]string, 0, len(items))
	for _, item := range items {
		labels = append(labels, item.label)
	}
	return strings.Join(labels, " ")
}

func (m Model) footerInfoItems(width int) []footerItem {
	version := footerVersionLabel()
	variants := [][]footerItem{
		{
			{label: "Ask", url: communityURL},
			{label: "Donate", url: sponsorURL},
			{label: version, url: releasesURL},
		},
		{
			{label: "Donate", url: sponsorURL},
			{label: version, url: releasesURL},
		},
		{
			{label: version, url: releasesURL},
		},
	}

	maxInfoWidth := max(0, width-footerMinHelpW)
	for idx, variant := range variants {
		if lipgloss.Width(joinFooterItems(variant)) <= maxInfoWidth || idx == len(variants)-1 {
			return variant
		}
	}
	return nil
}

func (m Model) unsavedLabel() string {
	if m.dirty && !m.draftSaved {
		return "Unsaved Changes"
	}
	if m.dirty && m.draftSaved {
		return "Saved Draft"
	}
	return "Current setup"
}

// activeProfileLabel answers "which saved profile am I looking at right now?"
// on every tab, including when the answer is none of them.
func (m Model) activeProfileLabel() string {
	if m.dirty || len(m.profiles) == 0 {
		return ""
	}
	if m.activeProfileName == "" {
		return m.styles.subtle.Render("no match")
	}
	return m.styles.statusOK.Render(m.activeProfileName)
}

// daemonNeedsRestart reports a daemon still running an older build than the one
// that just got installed. Nothing restarts a user service on upgrade, so the
// old binary keeps serving until someone says so.
func (m Model) daemonNeedsRestart() bool {
	installed := strings.TrimSpace(buildinfo.Version)
	running := strings.TrimSpace(m.daemonVersion)
	if !m.daemonOK || installed == "" || running == "" {
		return false
	}
	return installed != running
}

// restartHint is one clickable run of text, so its width can be hit-tested.
// It stays short because the tab row drops the whole status when it does not
// fit, and this is the part worth keeping.
func (m Model) restartHint() string {
	return fmt.Sprintf("Daemon %s · restart: R", m.daemonVersion)
}

func (m Model) renderTopStatus() string {
	parts := []string{m.unsavedBadge()}
	if label := m.activeProfileLabel(); label != "" {
		parts[0] += m.styles.subtle.Render(" · ") + label
	}
	if !m.daemonOK {
		daemon := m.styles.statusError.Underline(true).Render("Daemon not running")
		parts = append(parts, osc8Link(daemonURL, daemon))
	} else if m.daemonNeedsRestart() {
		parts = append(parts, m.styles.warning.Render(m.restartHint()))
	}
	if m.layoutErr != nil {
		parts = append(parts, m.styles.statusError.Render(m.layoutErr.Error()))
	} else if m.status != "" {
		style := m.styles.statusOK
		if m.statusErr {
			style = m.styles.statusError
		}
		parts = append(parts, style.Render(m.status))
	}
	return strings.Join(parts, "  ")
}

// renderCompactTopStatus keeps whatever the reader can act on. A narrow window
// has room for one thing, and "Current setup" is not it.
func (m Model) renderCompactTopStatus() string {
	if !m.daemonOK {
		daemon := m.styles.statusError.Underline(true).Render("Daemon not running")
		return osc8Link(daemonURL, daemon)
	}
	if m.daemonNeedsRestart() {
		return m.styles.warning.Render(m.restartHint())
	}
	return m.unsavedBadge()
}

func (m Model) footerLayout() footerLayout {
	width := max(20, m.footerContentWidth())
	help := m.footerHelpText()
	items := m.footerInfoItems(width)
	info := joinFooterItems(items)

	// Commands live on the left; project links live on the right.
	helpClean := strings.ReplaceAll(help, "`", "")
	if lipgloss.Width(helpClean)+lipgloss.Width(info)+1 > width {
		switch m.tab {
		case tabProfiles:
			helpClean = "Enter preview | l edit | d delete | ? keys"
		default:
			helpClean = "a apply | s save | ? keys"
		}
		// Essential actions take precedence over project links and version.
		for len(items) > 0 && lipgloss.Width(helpClean)+lipgloss.Width(joinFooterItems(items))+1 > width {
			items = items[1:]
		}
		info = joinFooterItems(items)
	}
	maxHelp := max(0, width-lipgloss.Width(info)-1)
	if lipgloss.Width(helpClean) > maxHelp {
		helpClean = fitString(helpClean, maxHelp)
	}
	gap := max(1, width-lipgloss.Width(helpClean)-lipgloss.Width(info))

	layout := footerLayout{
		text:  helpClean + strings.Repeat(" ", gap) + info,
		links: make([]footerLinkRegion, 0, len(items)),
	}

	cursor := lipgloss.Width(helpClean) + gap
	for idx, item := range items {
		labelWidth := lipgloss.Width(item.label)
		if strings.TrimSpace(item.url) != "" {
			layout.links = append(layout.links, footerLinkRegion{
				label: item.label,
				url:   item.url,
				start: cursor,
				end:   cursor + labelWidth,
			})
		}
		cursor += labelWidth
		if idx < len(items)-1 {
			cursor++
		}
	}

	return layout
}

func (m Model) renderFooterBar() string {
	return m.footerLayout().text
}

func (m Model) footerRowY() int {
	body := m.bodyRect()
	return body.y + body.h
}

func (m Model) footerColumnX() int {
	return m.appContentX()
}

func (m Model) footerLinkAt(x, y int) (footerLinkRegion, bool) {
	if y != m.footerRowY() {
		return footerLinkRegion{}, false
	}

	localX := x - m.footerColumnX()
	if localX < 0 || localX >= m.footerContentWidth() {
		return footerLinkRegion{}, false
	}

	layout := m.footerLayout()
	for _, link := range layout.links {
		if localX >= link.start && localX < link.end {
			return link, true
		}
	}

	return footerLinkRegion{}, false
}

func (m Model) footerContentWidth() int {
	app := m.styles.app
	frame := app.GetHorizontalFrameSize()
	// renderMain sets app.Width(terminalWidth - frame). Lipgloss treats Width
	// as total output width (including padding), so the actual content area is
	// Width - frame = terminalWidth - 2*frame.
	return max(1, m.terminalWidth()-2*frame)
}

func (m Model) renderFooterInfo(width int) string {
	return joinFooterItems(m.footerInfoItems(width))
}

func footerVersionLabel() string {
	version := strings.TrimSpace(buildinfo.Version)
	switch version {
	case "", "none":
		return "dev"
	case "dev":
		return version
	default:
		if strings.HasPrefix(version, "v") {
			return version
		}
		return "v" + version
	}
}

func replaceLastOccurrence(s, old, new string) string {
	idx := strings.LastIndex(s, old)
	if idx < 0 {
		return s
	}
	return s[:idx] + new + s[idx+len(old):]
}

func osc8Link(url, label string) string {
	return ansi.SetHyperlink(url) + label + ansi.ResetHyperlink()
}

func (m Model) decorateFooterBar(footer string) string {
	if strings.TrimSpace(footer) == "" {
		return footer
	}

	styled := m.styles.help.Render(footer)

	// Highlight keyboard shortcuts using backtick markers from the raw help text.
	keyStyle := withFG(lipgloss.NewStyle().Bold(true), "2")
	help := m.footerHelpText()
	for {
		start := strings.Index(help, "`")
		if start < 0 {
			break
		}
		end := strings.Index(help[start+1:], "`")
		if end < 0 {
			break
		}
		end += start + 1
		key := help[start+1 : end]
		rest := help[end+1:]
		ctxEnd := strings.Index(rest, "|")
		if ctxEnd < 0 {
			ctxEnd = len(rest)
		}
		ctx := rest[:ctxEnd]
		styled = strings.Replace(styled, key+ctx, keyStyle.Render(key)+ctx, 1)
		help = rest
	}

	// Version: replace before inserting URLs that might contain "dev"
	version := footerVersionLabel()
	styled = replaceLastOccurrence(styled, version, osc8Link(releasesURL, m.styles.footerVersion.Render(version)))

	// Links
	styled = strings.ReplaceAll(styled, "Donate", osc8Link(sponsorURL, m.styles.footerLinkAccent.Render("Donate")))
	styled = strings.ReplaceAll(styled, "Ask", osc8Link(communityURL, m.styles.footerLinkWarm.Render("Ask")))

	return styled
}

package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/crmne/hyprmoncfg/internal/buildinfo"
)

const (
	repoURL        = "https://github.com/crmne/hyprmoncfg"
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
	switch m.tab {
	case tabLayout:
		return "mouse drag monitors | arrows move 100px | Shift+arrows move 10px | Ctrl+arrows move 1px | Enter opens pickers | [ ] cycle | Tab switches panes | a apply | s save | r reset"
	case tabProfiles:
		return "mouse click selects profiles | Enter loads into the draft editor | a apply selected | d delete | s save current draft"
	case tabWorkspaces:
		return "click to focus fields and monitor order | left/right adjust | [ ] order select | u/n reorder | a apply | s save"
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
	return strings.Join(labels, "  ")
}

func (m Model) footerInfoItems(width int) []footerItem {
	version := footerVersionLabel()
	variants := [][]footerItem{
		{
			{label: "Ask", url: communityURL},
			{label: "Donate", url: sponsorURL},
			{label: version},
		},
		{
			{label: "Donate", url: sponsorURL},
			{label: version},
		},
		{
			{label: version},
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

func (m Model) footerLayout() footerLayout {
	width := max(20, m.footerContentWidth())
	help := m.footerHelpText()
	items := m.footerInfoItems(width)
	info := joinFooterItems(items)
	if info == "" {
		return footerLayout{text: fitString(help, width)}
	}

	infoWidth := lipgloss.Width(info)
	helpWidth := max(0, width-infoWidth-2)
	left := fitString(help, helpWidth)
	gap := max(2, width-lipgloss.Width(left)-infoWidth)
	infoStart := lipgloss.Width(left) + gap

	layout := footerLayout{
		text:  left + strings.Repeat(" ", gap) + info,
		links: make([]footerLinkRegion, 0, len(items)),
	}

	cursor := infoStart
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
			cursor += 2
		}
	}

	return layout
}

func (m Model) renderFooterBar() string {
	return m.footerLayout().text
}

func (m Model) footerRowY() int {
	return m.terminalHeight() - 1
}

func (m Model) footerColumnX() int {
	return 1
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
	outerWidth := max(1, m.terminalWidth()-app.GetHorizontalFrameSize())
	return max(1, outerWidth-app.GetHorizontalFrameSize())
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

func osc8Link(url, label string) string {
	const osc8 = "\x1b]8;;"
	const st = "\x1b\\"
	return osc8 + url + st + label + osc8 + st
}

func (m Model) decorateFooterBar(view, footer string) string {
	if strings.TrimSpace(footer) == "" {
		return view
	}

	styled := m.styles.help.Render(footer)
	styled = strings.ReplaceAll(styled, "Ask", osc8Link(communityURL, m.styles.footerLinkWarm.Render("Ask")))
	styled = strings.ReplaceAll(styled, "Donate", osc8Link(sponsorURL, m.styles.footerLinkAccent.Render("Donate")))
	version := footerVersionLabel()
	styled = strings.ReplaceAll(styled, version, m.styles.footerVersion.Render(version))
	return strings.Replace(view, footer, styled, 1)
}

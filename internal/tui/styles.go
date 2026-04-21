package tui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

var (
	activeTabBorder = lipgloss.Border{
		Top:         "─",
		Bottom:      " ",
		Left:        "│",
		Right:       "│",
		TopLeft:     "╭",
		TopRight:    "╮",
		BottomLeft:  "┘",
		BottomRight: "└",
	}
	inactiveTabBorder = lipgloss.Border{
		Top:         " ",
		Bottom:      " ",
		Left:        " ",
		Right:       " ",
		TopLeft:     " ",
		TopRight:    " ",
		BottomLeft:  " ",
		BottomRight: " ",
	}
)

type palette struct {
	text               string
	titleFg            string
	titleBg            string
	subtitle           string
	header             string
	subtle             string
	label              string
	value              string
	field              string
	fieldSelectedFg    string
	fieldSelectedBg    string
	groupTitle         string
	paneBorder         string
	paneActiveBorder   string
	tabBorder          string
	tabActiveBorder    string
	tabInactiveFg      string
	tabActiveFg        string
	tabActiveBg        string
	statusOK           string
	statusError        string
	help               string
	footerWarm         string
	footerAccent       string
	footerVersion      string
	warning            string
	badgeAccentFg      string
	badgeAccentBg      string
	badgeOnFg          string
	badgeOnBg          string
	badgeOffFg         string
	badgeOffBg         string
	badgeMutedFg       string
	badgeMutedBg       string
	modalBorder        string
	modalBg            string
	modalTitle         string
	selectedDesc       string
	panelBg            string
	canvasBg           string
	canvasGrid         string
	canvasAxis         string
	cardBorder         string
	cardBg             string
	cardFg             string
	cardMuted          string
	cardDisabledBorder string
	cardDisabledBg     string
	cardDisabledFg     string
	cardDisabledMuted  string
	cardSelectedBorder string
	cardSelectedBg     string
	cardSelectedFg     string
	cardSelectedMuted  string
	snapHighlight      string
}

type styles struct {
	palette          palette
	app              lipgloss.Style
	title            lipgloss.Style
	subtitle         lipgloss.Style
	header           lipgloss.Style
	subtle           lipgloss.Style
	label            lipgloss.Style
	value            lipgloss.Style
	field            lipgloss.Style
	fieldSelected    lipgloss.Style
	group            lipgloss.Style
	groupTitle       lipgloss.Style
	focused          lipgloss.Style
	activePane       lipgloss.Style
	inactivePane     lipgloss.Style
	tabActive        lipgloss.Style
	tabInactive      lipgloss.Style
	statusOK         lipgloss.Style
	statusError      lipgloss.Style
	toast            lipgloss.Style
	toastError       lipgloss.Style
	help             lipgloss.Style
	selectedDesc     lipgloss.Style
	footerLinkWarm   lipgloss.Style
	footerLinkAccent lipgloss.Style
	footerVersion    lipgloss.Style
	warning          lipgloss.Style
	badgeAccent      lipgloss.Style
	badgeOn          lipgloss.Style
	badgeOff         lipgloss.Style
	badgeMuted       lipgloss.Style
	modalBackdrop    lipgloss.Style
	modal            lipgloss.Style
	modalTitle       lipgloss.Style
	canvas           lipgloss.Style
}

func newStyles() styles {
	p := newPalette()

	return styles{
		palette:          p,
		app:              withFG(lipgloss.NewStyle().Padding(0, 1), p.text),
		title:            withBG(withFG(lipgloss.NewStyle().Bold(true).Padding(0, 1), p.titleFg), p.titleBg),
		subtitle:         withFG(lipgloss.NewStyle(), p.subtitle),
		header:           withFG(lipgloss.NewStyle().Bold(true), p.header),
		subtle:           withFG(lipgloss.NewStyle(), p.subtle),
		label:            withFG(lipgloss.NewStyle(), p.label),
		value:            withFG(lipgloss.NewStyle(), p.value),
		field:            withFG(lipgloss.NewStyle().Padding(0, 1), p.field),
		fieldSelected:    withBG(withFG(lipgloss.NewStyle().Padding(0, 1).Bold(true), p.fieldSelectedFg), p.fieldSelectedBg),
		group:            withBG(lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(p.paneBorder)).Padding(0, 1), p.panelBg),
		groupTitle:       withFG(lipgloss.NewStyle().Bold(true), p.groupTitle),
		focused:          withBG(withFG(lipgloss.NewStyle().Bold(true).Padding(0, 1), p.fieldSelectedFg), p.fieldSelectedBg),
		activePane:       withBG(lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(p.paneActiveBorder)).Padding(0, 1), p.panelBg),
		inactivePane:     withBG(lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(p.paneBorder)).Padding(0, 1), p.panelBg),
		tabActive:        lipgloss.NewStyle().Border(activeTabBorder).BorderForeground(lipgloss.Color(p.tabActiveBorder)).Padding(0, 1).Bold(true),
		tabInactive:      withFG(lipgloss.NewStyle().Border(inactiveTabBorder).BorderForeground(lipgloss.Color(p.tabBorder)).Padding(0, 1), p.tabInactiveFg),
		statusOK:         withFG(lipgloss.NewStyle().Bold(true), p.statusOK),
		statusError:      withFG(lipgloss.NewStyle().Bold(true), p.statusError),
		toast:            withBG(withFG(lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(p.paneActiveBorder)).Padding(0, 2).Bold(true), p.badgeOnFg), p.badgeOnBg),
		toastError:       withBG(withFG(lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("15")).Padding(0, 2).Bold(true), "0"), "9"),
		help:             withFG(lipgloss.NewStyle(), p.help),
		selectedDesc:     withBG(withFG(lipgloss.NewStyle(), p.selectedDesc), p.fieldSelectedBg),
		footerLinkWarm:   withFG(lipgloss.NewStyle().Underline(true), p.footerWarm),
		footerLinkAccent: withFG(lipgloss.NewStyle().Underline(true), p.footerAccent),
		footerVersion:    withFG(lipgloss.NewStyle().Underline(true), p.footerVersion),
		warning:          withFG(lipgloss.NewStyle().Bold(true), p.warning),
		badgeAccent:      withBG(withFG(lipgloss.NewStyle().Padding(0, 1).Bold(true), p.badgeAccentFg), p.badgeAccentBg),
		badgeOn:          withBG(withFG(lipgloss.NewStyle().Padding(0, 1).Bold(true), p.badgeOnFg), p.badgeOnBg),
		badgeOff:         withBG(withFG(lipgloss.NewStyle().Padding(0, 1), p.badgeOffFg), p.badgeOffBg),
		badgeMuted:       withBG(withFG(lipgloss.NewStyle().Padding(0, 1), p.badgeMutedFg), p.badgeMutedBg),
		modalBackdrop:    lipgloss.NewStyle().Padding(0, 1),
		modal:            withBG(lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(p.modalBorder)).Padding(1, 2), p.modalBg),
		modalTitle:       withFG(lipgloss.NewStyle().Bold(true), p.modalTitle),
		canvas:           withBG(lipgloss.NewStyle().Padding(0), p.canvasBg),
	}
}

func newPalette() palette {
	fgColor := termenv.ForegroundColor()
	bgColor := termenv.BackgroundColor()
	defaultFG := terminalColorString(fgColor, "7")
	defaultBG := terminalColorString(bgColor, "0")
	supportText := blendedTerminalColor(fgColor, bgColor, 0.42, "7")
	chrome := blendedTerminalColor(fgColor, bgColor, 0.68, "8")
	softFill := blendedTerminalColor(fgColor, bgColor, 0.82, "8")
	canvasAxis := blendedTerminalColor(fgColor, bgColor, 0.55, "7")

	return palette{
		text:               "",
		titleFg:            "",
		titleBg:            softFill,
		subtitle:           supportText,
		header:             "",
		subtle:             supportText,
		label:              supportText,
		value:              "",
		field:              "",
		fieldSelectedFg:    defaultBG,
		fieldSelectedBg:    defaultFG,
		groupTitle:         "3",
		paneBorder:         chrome,
		paneActiveBorder:   "2",
		tabBorder:          chrome,
		tabActiveBorder:    "2",
		tabInactiveFg:      supportText,
		tabActiveFg:        defaultBG,
		tabActiveBg:        defaultFG,
		statusOK:           "2",
		statusError:        "1",
		help:               supportText,
		footerWarm:         "3",
		footerAccent:       "6",
		footerVersion:      supportText,
		warning:            "3",
		badgeAccentFg:      "0",
		badgeAccentBg:      "3",
		badgeOnFg:          "15",
		badgeOnBg:          "2",
		badgeOffFg:         "",
		badgeOffBg:         softFill,
		badgeMutedFg:       "",
		badgeMutedBg:       softFill,
		modalBorder:        "2",
		modalBg:            "",
		modalTitle:         "",
		selectedDesc:       defaultBG,
		panelBg:            "",
		canvasBg:           "",
		canvasGrid:         chrome,
		canvasAxis:         canvasAxis,
		cardBorder:         chrome,
		cardBg:             "",
		cardFg:             "",
		cardMuted:          supportText,
		cardDisabledBorder: "1",
		cardDisabledBg:     "",
		cardDisabledFg:     supportText,
		cardDisabledMuted:  supportText,
		cardSelectedBorder: "3",
		cardSelectedBg:     defaultFG,
		cardSelectedFg:     defaultBG,
		cardSelectedMuted:  defaultBG,
		snapHighlight:      "3",
	}
}

func terminalColorString(color termenv.Color, fallback string) string {
	if color == nil {
		return fallback
	}
	if value := fmt.Sprint(color); value != "" {
		return value
	}
	return fallback
}

func blendedTerminalColor(fgColor, bgColor termenv.Color, bgWeight float64, fallback string) string {
	if fmt.Sprint(fgColor) == "" || fmt.Sprint(bgColor) == "" {
		return fallback
	}
	return termenv.ConvertToRGB(fgColor).BlendLab(termenv.ConvertToRGB(bgColor), bgWeight).Clamped().Hex()
}

func withFG(style lipgloss.Style, value string) lipgloss.Style {
	if value == "" {
		return style
	}
	return style.Foreground(lipgloss.Color(value))
}

func withBG(style lipgloss.Style, value string) lipgloss.Style {
	if value == "" {
		return style
	}
	return style.Background(lipgloss.Color(value))
}

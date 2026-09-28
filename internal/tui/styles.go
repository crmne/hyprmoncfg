package tui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

type palette struct {
	text               string
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
	paneStaticBorder   string
	tabInactiveFg      string
	tabActiveFg        string
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
	cardStaticBorder   string
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
	stageDot           string
	cardFill           string
	cardSelectedFill   string
	chipFg             string
	chipBg             string
	chipStrongFg       string
	chipStrongBg       string
	tabPillBg          string
	meterEmpty         string
}

type styles struct {
	palette          palette
	app              lipgloss.Style
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
	staticPane       lipgloss.Style
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
	return newStylesFrom(termenv.ForegroundColor(), termenv.BackgroundColor())
}

// newStylesFrom derives every style from the terminal's own foreground and
// background, so light and dark themes get the same hierarchy.
func newStylesFrom(fgColor, bgColor termenv.Color) styles {
	p := newPaletteFrom(fgColor, bgColor)

	return styles{
		palette:          p,
		app:              withFG(lipgloss.NewStyle(), p.text),
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
		staticPane:       withBG(lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(p.paneStaticBorder)).Padding(0, 1), p.panelBg),
		tabActive:        withFG(lipgloss.NewStyle().Bold(true), p.tabActiveFg),
		tabInactive:      withFG(lipgloss.NewStyle(), p.tabInactiveFg),
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

func newPaletteFrom(fgColor, bgColor termenv.Color) palette {
	defaultFG := terminalColorString(fgColor, "7")
	defaultBG := terminalColorString(bgColor, "0")
	supportText := blendedTerminalColor(fgColor, bgColor, 0.42, "7")
	chrome := blendedTerminalColor(fgColor, bgColor, 0.68, "8")
	softFill := blendedTerminalColor(fgColor, bgColor, 0.82, "8")
	// Panes and cards that only show what is there sit between the muted
	// chrome and the body text, so they read clearly without taking the accent.
	presence := blendedTerminalColor(fgColor, bgColor, 0.5, "7")
	canvasAxis := blendedTerminalColor(fgColor, bgColor, 0.55, "7")
	// The stage is a quiet dotted field; cards are faintly lit screens on it,
	// so the arrangement reads before any text does. Without a known terminal
	// background the fills fall back to none rather than guessing a color.
	stageDot := blendedTerminalColor(fgColor, bgColor, 0.74, "8")
	cardFill := blendedTerminalColor(fgColor, bgColor, 0.94, "")
	cardSelectedFill := blendedTerminalColor(fgColor, bgColor, 0.89, "")

	return palette{
		text:               "",
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
		paneStaticBorder:   presence,
		tabInactiveFg:      supportText,
		tabActiveFg:        "2",
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
		cardStaticBorder:   presence,
		cardBg:             cardFill,
		cardFg:             "",
		cardMuted:          supportText,
		cardDisabledBorder: "1",
		cardDisabledBg:     "",
		cardDisabledFg:     supportText,
		cardDisabledMuted:  supportText,
		cardSelectedBorder: "2",
		cardSelectedBg:     cardSelectedFill,
		cardSelectedFg:     "",
		cardSelectedMuted:  supportText,
		snapHighlight:      "3",
		stageDot:           stageDot,
		cardFill:           cardFill,
		cardSelectedFill:   cardSelectedFill,
		chipFg:             "2",
		chipBg:             softFill,
		chipStrongFg:       defaultBG,
		chipStrongBg:       "2",
		tabPillBg:          softFill,
		meterEmpty:         chrome,
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

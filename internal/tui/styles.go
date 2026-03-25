package tui

import "github.com/charmbracelet/lipgloss"

type styles struct {
	app           lipgloss.Style
	title         lipgloss.Style
	subtitle      lipgloss.Style
	header        lipgloss.Style
	subtle        lipgloss.Style
	label         lipgloss.Style
	value         lipgloss.Style
	field         lipgloss.Style
	fieldSelected lipgloss.Style
	group         lipgloss.Style
	groupTitle    lipgloss.Style
	focused       lipgloss.Style
	activePane    lipgloss.Style
	inactivePane  lipgloss.Style
	tabActive     lipgloss.Style
	tabInactive   lipgloss.Style
	statusOK      lipgloss.Style
	statusError   lipgloss.Style
	help          lipgloss.Style
	warning       lipgloss.Style
	badgeAccent   lipgloss.Style
	badgeOn       lipgloss.Style
	badgeOff      lipgloss.Style
	badgeMuted    lipgloss.Style
	modalBackdrop lipgloss.Style
	modal         lipgloss.Style
	modalTitle    lipgloss.Style
	canvas        lipgloss.Style
}

func newStyles() styles {
	return styles{
		app:           lipgloss.NewStyle().Padding(0, 1).Foreground(lipgloss.Color("#E7E9EE")),
		title:         lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F5E7CF")).Background(lipgloss.Color("#2A2020")).Padding(0, 1),
		subtitle:      lipgloss.NewStyle().Foreground(lipgloss.Color("#8E94A4")),
		header:        lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F3F4F7")),
		subtle:        lipgloss.NewStyle().Foreground(lipgloss.Color("#7F8594")),
		label:         lipgloss.NewStyle().Foreground(lipgloss.Color("#A6ADBB")),
		value:         lipgloss.NewStyle().Foreground(lipgloss.Color("#F3F4F7")),
		field:         lipgloss.NewStyle().Foreground(lipgloss.Color("#E7E9EE")).Padding(0, 1),
		fieldSelected: lipgloss.NewStyle().Foreground(lipgloss.Color("#F8FBFF")).Background(lipgloss.Color("#21344A")).Padding(0, 1).Bold(true),
		group:         lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#2C3140")).Background(lipgloss.Color("#17191F")).Padding(0, 1),
		groupTitle:    lipgloss.NewStyle().Foreground(lipgloss.Color("#D2A56E")).Bold(true),
		focused:       lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F8FBFF")).Background(lipgloss.Color("#355C8A")).Padding(0, 1),
		activePane:    lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#4C76A8")).Background(lipgloss.Color("#13161C")).Padding(0, 1),
		inactivePane:  lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#2A2F3A")).Background(lipgloss.Color("#111318")).Padding(0, 1),
		tabActive:     lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#679BF4")).Foreground(lipgloss.Color("#F8FBFF")).Background(lipgloss.Color("#203248")).Padding(0, 1).Bold(true),
		tabInactive:   lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#2A2F3A")).Foreground(lipgloss.Color("#B0B6C3")).Background(lipgloss.Color("#17191F")).Padding(0, 1),
		statusOK:      lipgloss.NewStyle().Foreground(lipgloss.Color("#9CD67B")).Bold(true),
		statusError:   lipgloss.NewStyle().Foreground(lipgloss.Color("#F08A86")).Bold(true),
		help:          lipgloss.NewStyle().Foreground(lipgloss.Color("#6D7382")),
		warning:       lipgloss.NewStyle().Foreground(lipgloss.Color("#E6BF6C")).Bold(true),
		badgeAccent:   lipgloss.NewStyle().Foreground(lipgloss.Color("#0D1620")).Background(lipgloss.Color("#D2A56E")).Padding(0, 1).Bold(true),
		badgeOn:       lipgloss.NewStyle().Foreground(lipgloss.Color("#0C1A11")).Background(lipgloss.Color("#9CD67B")).Padding(0, 1).Bold(true),
		badgeOff:      lipgloss.NewStyle().Foreground(lipgloss.Color("#D7DCE5")).Background(lipgloss.Color("#2C3140")).Padding(0, 1),
		badgeMuted:    lipgloss.NewStyle().Foreground(lipgloss.Color("#D7DCE5")).Background(lipgloss.Color("#3B414C")).Padding(0, 1),
		modalBackdrop: lipgloss.NewStyle().Padding(0, 1),
		modal:         lipgloss.NewStyle().Border(lipgloss.DoubleBorder()).BorderForeground(lipgloss.Color("#7EA8FF")).Background(lipgloss.Color("#161A22")).Padding(1, 2),
		modalTitle:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F5E7CF")),
		canvas:        lipgloss.NewStyle().Background(lipgloss.Color("#16181D")).Padding(0),
	}
}

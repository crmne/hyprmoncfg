package tui

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/crmne/hyprmoncfg/internal/hypr"
)

// Resolve pointer targets from the same rendered text that the user sees.
func (m Model) visibleActionAt(x, y int, label string) bool {
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	if y < 0 || y >= len(lines) {
		return false
	}
	line := lines[y]
	index := strings.Index(line, label)
	if index < 0 {
		return false
	}
	start := lipgloss.Width(line[:index])
	end := start + lipgloss.Width(label)
	return x >= start && x < end
}

// Display formatting is presentation-only. Never round the stored mode or scale.
// Keep this vocabulary aligned with the panel's Model.displaySummary.
func displayNumber(value float64, precision int) string {
	return strings.TrimRight(strings.TrimRight(strconv.FormatFloat(value, 'f', precision, 64), "0"), ".")
}

func displayModeLabel(mode string) string {
	w, h, hz, ok := hypr.ParseMode(mode)
	if !ok {
		return mode
	}
	return fmt.Sprintf("%dx%d@%sHz", w, h, displayNumber(hz, 1))
}

func (o editableOutput) modelSizeLabel() string {
	label := o.displayModelLabel()
	if o.PhysicalWidth > 0 && o.PhysicalHeight > 0 {
		size := math.Round(math.Hypot(float64(o.PhysicalWidth), float64(o.PhysicalHeight)) / 25.4)
		label += fmt.Sprintf(" %.0f\"", size)
	}
	return label
}

func (o editableOutput) placementLabel() string {
	return fmt.Sprintf("Scale %sx  Position %d,%d", displayNumber(o.Scale, 2), o.X, o.Y)
}

func (o editableOutput) maximumResolutionLabel() string {
	w, h := 0, 0
	for _, mode := range o.HardwareModes {
		mw, mh, _, ok := hypr.ParseMode(mode)
		if ok && mw*mh > w*h {
			w, h = mw, mh
		}
	}
	if w == 0 {
		return "Not reported"
	}
	return fmt.Sprintf("%dx%d", w, h)
}

func (m Model) hardwareDetailLines(output editableOutput) []string {
	panelSize := "Not reported"
	if output.PhysicalWidth > 0 && output.PhysicalHeight > 0 {
		size := math.Round(math.Hypot(float64(output.PhysicalWidth), float64(output.PhysicalHeight)) / 25.4)
		panelSize = fmt.Sprintf("%.0f\" (%dx%dmm)", size, output.PhysicalWidth, output.PhysicalHeight)
	}
	return m.renderDetailRows([]detailRow{
		{label: "Connector", value: output.Name},
		{label: "Model", value: output.displayModelLabel()},
		{label: "Max resolution", value: output.maximumResolutionLabel()},
		{label: "Panel size", value: panelSize},
		{label: "Type", value: outputTypeLabel(output)},
		{label: "Serial", value: blankFallback(strings.TrimSpace(output.Serial), "Not reported")},
	})
}

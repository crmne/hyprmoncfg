package omarchywatch

import (
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

// PanelEdit holds the fields of one monitor that changed in Omarchy's display
// panel. Empty fields did not change.
type PanelEdit struct {
	Mode      string
	Position  string
	Scale     string
	Transform string
}

// PanelSettings follows the settings file Omarchy's display panel writes, so an
// edit made there lands in the active profile instead of being reverted.
type PanelSettings struct {
	path  string
	known bool
	last  map[string]PanelEdit
	// held keeps edits for monitors that are unplugged, until they return.
	held map[string]PanelEdit
}

// NewPanelSettings returns nil outside Omarchy; a nil PanelSettings reports no edits.
func NewPanelSettings() *PanelSettings {
	if _, err := exec.LookPath(clamshellCommand); err != nil {
		return nil
	}
	state := os.Getenv("XDG_STATE_HOME")
	if state == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil
		}
		state = filepath.Join(home, ".local", "state")
	}
	return &PanelSettings{path: filepath.Join(state, "omarchy", "monitors", "settings")}
}

// Changes reports the fields that differ from the previous read, by monitor
// identity. The first read only records a baseline: settings written before the
// daemon started are not fresh edits.
func (p *PanelSettings) Changes() (map[string]PanelEdit, error) {
	if p == nil {
		return nil, nil
	}
	current, err := readPanelSettings(p.path)
	if err != nil {
		return nil, err
	}
	if !p.known {
		p.known, p.last = true, current
		return nil, nil
	}
	changes := p.held
	p.held = nil
	if changes == nil {
		changes = map[string]PanelEdit{}
	}
	for identity, now := range current {
		was := p.last[identity]
		var edit PanelEdit
		if now.Mode != was.Mode {
			edit.Mode = now.Mode
		}
		if now.Position != was.Position {
			edit.Position = now.Position
		}
		if now.Scale != was.Scale {
			edit.Scale = now.Scale
		}
		if now.Transform != was.Transform {
			edit.Transform = now.Transform
		}
		if edit != (PanelEdit{}) {
			changes[identity] = mergePanelEdit(changes[identity], edit)
		}
	}
	p.last = current
	return changes, nil
}

// Hold keeps an edit for the next read, for a monitor that is not connected now.
func (p *PanelSettings) Hold(edits map[string]PanelEdit) {
	if p == nil || len(edits) == 0 {
		return
	}
	if p.held == nil {
		p.held = map[string]PanelEdit{}
	}
	for identity, edit := range edits {
		p.held[identity] = mergePanelEdit(p.held[identity], edit)
	}
}

func mergePanelEdit(older, newer PanelEdit) PanelEdit {
	if newer.Mode != "" {
		older.Mode = newer.Mode
	}
	if newer.Position != "" {
		older.Position = newer.Position
	}
	if newer.Scale != "" {
		older.Scale = newer.Scale
	}
	if newer.Transform != "" {
		older.Transform = newer.Transform
	}
	return older
}

// Each line is mode, position, scale, transform, identity; older lines have no
// transform.
func readPanelSettings(path string) (map[string]PanelEdit, error) {
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]PanelEdit{}, nil
	}
	if err != nil {
		return nil, err
	}
	settings := map[string]PanelEdit{}
	for _, line := range strings.Split(string(content), "\n") {
		fields := strings.Split(line, "\t")
		switch len(fields) {
		case 4:
			settings[fields[3]] = PanelEdit{Mode: fields[0], Position: fields[1], Scale: fields[2]}
		case 5:
			settings[fields[4]] = PanelEdit{Mode: fields[0], Position: fields[1], Scale: fields[2], Transform: fields[3]}
		}
	}
	return settings, nil
}

var errPanelMonitorAbsent = errors.New("monitor not connected")

var (
	panelMode        = regexp.MustCompile(`^(\d+)x(\d+)(?:@([\d.]+))?$`)
	panelPosition    = regexp.MustCompile(`^(-?\d+)x(-?\d+)$`)
	panelOrdinal     = regexp.MustCompile(` #\d+$`)
	refreshTolerance = 0.5
	scaleTolerance   = 0.001
)

// ImportPanelEdits folds Omarchy panel edits into a profile. It returns the
// connectors it changed, a reason for each edit it dropped, and the edits for
// monitors that are not connected, which can still apply when they return.
func ImportPanelEdits(p profile.Profile, monitors []hypr.Monitor, changes map[string]PanelEdit) (profile.Profile, []string, []string, map[string]PanelEdit) {
	resolver := profile.NewMonitorResolver(monitors)
	outputs := append([]profile.OutputConfig(nil), p.Outputs...)
	var imported, skipped []string
	absent := map[string]PanelEdit{}
	for identity, edit := range changes {
		monitor, err := panelMonitor(identity, monitors)
		if errors.Is(err, errPanelMonitorAbsent) {
			absent[identity] = edit
			continue
		}
		if err != nil {
			skipped = append(skipped, err.Error())
			continue
		}
		for i := range outputs {
			resolved, ok := resolver.ResolveOutput(outputs[i])
			if !ok || resolved.Name != monitor.Name || outputs[i].MirrorOf != "" {
				continue
			}
			if applyPanelEdit(&outputs[i], edit, monitor) {
				imported = append(imported, monitor.Name)
			}
		}
	}
	p.Outputs = outputs
	return p, imported, skipped, absent
}

// panelMonitor finds the monitor an identity names. Omarchy numbers identical
// monitors in its own order, so those cannot be told apart from here.
func panelMonitor(identity string, monitors []hypr.Monitor) (hypr.Monitor, error) {
	description := panelOrdinal.ReplaceAllString(identity, "")
	var found []hypr.Monitor
	for _, monitor := range monitors {
		if monitor.Name == identity || (monitor.Description != "" && monitor.Description == description) {
			found = append(found, monitor)
		}
	}
	switch len(found) {
	case 0:
		return hypr.Monitor{}, errPanelMonitorAbsent
	case 1:
		return found[0], nil
	}
	return hypr.Monitor{}, fmt.Errorf("%q matches %d identical monitors", identity, len(found))
}

func applyPanelEdit(out *profile.OutputConfig, edit PanelEdit, monitor hypr.Monitor) bool {
	changed := false
	if parts := panelMode.FindStringSubmatch(edit.Mode); parts != nil {
		width, _ := strconv.Atoi(parts[1])
		height, _ := strconv.Atoi(parts[2])
		refresh := monitor.RefreshRate
		if parts[3] != "" {
			refresh, _ = strconv.ParseFloat(parts[3], 64)
		}
		if width != out.Width || height != out.Height || math.Abs(refresh-out.Refresh) > refreshTolerance {
			out.Width, out.Height, out.Refresh = width, height, refresh
			out.Mode = hypr.FormatMode(width, height, refresh)
			changed = true
		}
	}
	if parts := panelPosition.FindStringSubmatch(edit.Position); parts != nil {
		x, _ := strconv.Atoi(parts[1])
		y, _ := strconv.Atoi(parts[2])
		if x != out.X || y != out.Y {
			out.X, out.Y = x, y
			changed = true
		}
	}
	if scale, err := strconv.ParseFloat(edit.Scale, 64); err == nil && scale > 0 && math.Abs(scale-out.Scale) > scaleTolerance {
		out.Scale = scale
		changed = true
	}
	// Automatic rotation owns the transform of a panel that follows the sensor.
	if transform, err := strconv.Atoi(edit.Transform); err == nil && transform >= 0 && transform <= 7 &&
		!out.RotationAuto && transform != out.Transform {
		out.Transform = transform
		changed = true
	}
	return changed
}

package profile

import (
	"math"
	"sort"

	"github.com/crmne/hyprmoncfg/internal/hypr"
)

// ExtendConnected adds displays absent from a saved profile to its right edge.
// The returned layout is ephemeral; the saved profile is never modified.
func ExtendConnected(p Profile, monitors []hypr.Monitor) Profile {
	if p.DisableUnknownOutputs {
		return p
	}
	p.Outputs = append([]OutputConfig(nil), p.Outputs...)
	p.Workspaces.MonitorOrder = append([]string(nil), p.Workspaces.MonitorOrder...)
	p.Workspaces.Rules = append([]WorkspaceRule(nil), p.Workspaces.Rules...)
	omitted := OmittedMonitors(p, monitors)
	if len(omitted) == 0 {
		return p
	}
	resolver := NewMonitorResolver(monitors)
	right, top, adjacentHeight, found := 0, 0, 0, false
	connected := make([]OutputConfig, 0, len(p.Outputs))
	for _, out := range p.Outputs {
		live, ok := resolver.ResolveOutput(out)
		if !ok {
			continue
		}
		connected = append(connected, out)
		if !out.Enabled || out.MirrorOf != "" {
			continue
		}
		m := hypr.Monitor{Width: out.Width, Height: out.Height, Scale: out.Scale, Transform: out.Transform}
		if m.Width <= 0 || m.Height <= 0 {
			m.Width, m.Height = live.Width, live.Height
		}
		w, h := m.LogicalSize()
		if !found || out.X+w > right {
			right, top, adjacentHeight, found = out.X+w, out.Y, h, true
		}
	}
	p.Outputs = connected
	if len(p.Workspaces.MonitorOrder) == 0 {
		for _, idx := range outputIndicesByLayout(p.Outputs) {
			if p.Outputs[idx].Enabled && p.Outputs[idx].MirrorOf == "" {
				p.Workspaces.MonitorOrder = append(p.Workspaces.MonitorOrder, p.Outputs[idx].Key)
			}
		}
	}
	sort.SliceStable(omitted, func(i, j int) bool {
		if omitted[i].IsInternal() != omitted[j].IsInternal() {
			return omitted[i].IsInternal()
		}
		return omitted[i].Name < omitted[j].Name
	})
	counts := hypr.MonitorMatchCounts(monitors)
	for _, m := range omitted {
		// Choose a supported pair, not independent resolution/refresh maxima.
		bestArea, bestRefresh := 0, 0.0
		for _, mode := range m.AvailableModes {
			if w, h, hz, ok := hypr.ParseMode(mode); ok && w > 0 && h > 0 && hz > 0 {
				if area := w * h; area > bestArea || (area == bestArea && hz > bestRefresh) {
					m.Width, m.Height, m.RefreshRate = w, h, hz
					bestArea, bestRefresh = area, hz
				}
			}
		}
		if m.Scale <= 0 {
			m.Scale = 1
		}
		m.Disabled, m.MirrorOf, m.Transform = false, "", 0
		m.X, m.Y = right, top
		w, h := m.LogicalSize()
		if found {
			m.Y += int(math.Round(float64(adjacentHeight-h) / 2))
		}
		out := FromMonitors("draft", []hypr.Monitor{m}).Outputs[0]
		out.VRR, out.Bitdepth, out.CM = 0, 8, "srgb"
		out.Key = hypr.MonitorOutputKey(m, counts)
		p.Outputs = append(p.Outputs, out)
		p.Workspaces.MonitorOrder = append(p.Workspaces.MonitorOrder, out.Key)
		right += w
		top, adjacentHeight, found = m.Y, h, true
	}
	if !p.Workspaces.Enabled {
		p.Workspaces.Enabled = true
		p.Workspaces.Strategy = WorkspaceStrategySequential
		p.Workspaces.GroupSize = 3
		p.Workspaces.MaxWorkspaces = 9
	}
	p.Normalize()
	return p
}

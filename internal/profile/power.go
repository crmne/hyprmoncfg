package profile

import (
	"github.com/crmne/hyprmoncfg/internal/hypr"
	"math"
)

// WithPowerRefresh changes only the ephemeral internal-panel mode. The caller
// must establish a known power source and explicit opt-in before using it.
func WithPowerRefresh(p Profile, monitors []hypr.Monitor, battery bool) Profile {
	p.Outputs = append([]OutputConfig(nil), p.Outputs...)
	resolver := NewMonitorResolver(monitors)
	for i, out := range p.Outputs {
		m, ok := resolver.ResolveOutput(out)
		if !ok || !m.IsInternal() || !out.Enabled || out.MirrorOf != "" {
			continue
		}
		width, height := out.Width, out.Height
		if w, h, _, ok := hypr.ParseMode(out.Mode); ok {
			width, height = w, h
		}
		best, lowest := 0.0, math.Inf(1)
		bestMode, lowestMode := "", ""
		for _, mode := range m.AvailableModes {
			w, h, hz, ok := hypr.ParseMode(mode)
			if !ok || w != width || h != height || hz <= 0 {
				continue
			}
			if hz < lowest {
				lowest, lowestMode = hz, mode
			}
			if hz > best && (!battery || hz <= 60.01) {
				best, bestMode = hz, mode
			}
		}
		if bestMode == "" && battery {
			best, bestMode = lowest, lowestMode
		}
		if bestMode == "" {
			continue
		}
		p.Outputs[i].Mode, p.Outputs[i].Refresh = bestMode, best
		p.Outputs[i].Width, p.Outputs[i].Height = width, height
	}
	return p
}

package daemon

import (
	"errors"

	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

func setLaptopDisplay(p profile.Profile, monitors []hypr.Monitor, enabled bool) (profile.Profile, error) {
	p.Outputs = append([]profile.OutputConfig(nil), p.Outputs...)
	resolver := profile.NewMonitorResolver(monitors)
	found := false
	for i, output := range p.Outputs {
		if monitor, ok := resolver.ResolveOutput(output); ok && monitor.IsInternal() {
			p.Outputs[i].Enabled = enabled
			found = true
		}
	}
	if enabled && !found {
		// Older external-only profiles may omit the laptop entirely. There
		// is no saved placement to restore: add it beside the existing desk.
		for _, output := range profile.FromMonitors("", monitors).Outputs {
			monitor, ok := resolver.ResolveOutput(output)
			if !ok || !monitor.IsInternal() {
				continue
			}
			if output.Width <= 0 || output.Height <= 0 {
				for _, mode := range monitor.AvailableModes {
					if width, height, refresh, ok := hypr.ParseMode(mode); ok {
						output.Mode, output.Width, output.Height, output.Refresh = mode, width, height, refresh
						break
					}
				}
			}
			if output.Width <= 0 || output.Height <= 0 {
				return profile.Profile{}, errors.New("laptop display has no usable mode; configure it in the editor first")
			}
			output.Enabled, output.MirrorOf = true, ""
			output.X, output.Y = 0, 0
			if output.Scale <= 0 {
				output.Scale = 1
			}
			for _, existing := range p.Outputs {
				if _, connected := resolver.ResolveOutput(existing); connected && existing.Enabled && existing.MirrorOf == "" {
					width, _ := existing.LogicalSize()
					output.X = max(output.X, existing.X+width)
				}
			}
			p.Outputs = append(p.Outputs, output)
		}
	}
	if profile.EvaluateMatch(p, monitors).ConnectedEnabledOutputs == 0 {
		return profile.Profile{}, errors.New("laptop display toggle would disable every connected display")
	}
	p.Normalize()
	return p, p.Validate()
}

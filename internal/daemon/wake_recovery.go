package daemon

import (
	"context"
	"os"
	"slices"
	"time"

	"github.com/crmne/hyprmoncfg/internal/config"
	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/lid"
	"github.com/crmne/hyprmoncfg/internal/profile"
	"github.com/crmne/hyprmoncfg/internal/render"
)

// Read the saved (open-lid) choice rather than the generated clamshell rules.
// An explicitly disabled panel stays disabled. No profile hooks are executed.
// The caller holds writeMu, as with any other profile-based display operation.
func (s *Service) wakeProfile() profile.Profile {
	p := s.lastProfile
	if p.Name != "" {
		if saved, err := s.store.Load(p.Name); err == nil {
			p = saved
		}
	}
	// The profile may have been saved on a different connector name. Reuse the
	// last successful hardware resolution without querying a stalled compositor.
	p.Outputs = slices.Clone(p.Outputs)
	if s.applied != nil {
		resolver := profile.NewMonitorResolver(render.ProfileMonitors(s.applied.live))
		for i, output := range p.Outputs {
			if monitor, ok := resolver.ResolveOutput(output); ok {
				p.Outputs[i].Name = monitor.Name
			}
		}
	}
	return p
}

func openInternalProfile(p profile.Profile, state lid.State) profile.Profile {
	result := profile.Profile{Name: p.Name}
	if state != lid.Open {
		return result
	}
	for _, output := range p.Outputs {
		if !output.Enabled || !hypr.IsInternalConnector(output.Name) {
			continue
		}
		// An absent HDMI mirror source must not prevent emergency panel recovery.
		output.MirrorOf = ""
		result.Outputs = append(result.Outputs, output)
	}
	return result
}

func (s *Service) restoreOpenInternal(ctx context.Context) error {
	p := openInternalProfile(s.wakeProfile(), s.lidState)
	if len(p.Outputs) == 0 {
		return nil
	}
	restoreCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	resolved, err := s.wakeHyprConfig(restoreCtx)
	if err != nil {
		return err
	}
	if resolved.Format == config.HyprConfigLua {
		code, err := render.RenderConfig(p, render.ProfileMonitors(p), render.Options{Format: config.HyprConfigLua, UseMonitorV2: true})
		if err != nil {
			return err
		}
		_, err = s.client.Eval(restoreCtx, "do\n"+code+"\nend")
		return err
	}
	for _, output := range p.Outputs {
		if err := s.client.KeywordMonitor(restoreCtx, render.CommandForOutput(output.Name, output, "")); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) wakeRecovered(monitors []hypr.Monitor) bool {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if displayPowerState(monitors) != displayPowerAwake {
		return false
	}
	for _, output := range openInternalProfile(s.wakeProfile(), s.lidState).Outputs {
		found := false
		for _, monitor := range monitors {
			if monitor.Name == output.Name && !monitor.Disabled && monitor.DPMSStatus {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	for _, monitor := range monitors {
		if !monitor.Disabled && !monitor.DPMSStatus {
			return false
		}
	}
	return true
}

// Startup discovers the running dialect while IPC is healthy. Reuse it during
// recovery rather than letting an empty version silently select legacy syntax.
func (s *Service) wakeHyprConfig(ctx context.Context) (config.ResolvedHyprConfig, error) {
	s.wakeConfigMu.RLock()
	cached := s.wakeConfig
	s.wakeConfigMu.RUnlock()
	if cached != nil {
		return *cached, nil
	}
	if s.cfg.HyprConfig != "" || os.Getenv("HYPRLAND_CONFIG") != "" {
		return config.ResolveHyprlandConfig("", s.cfg.MonitorsConf, s.cfg.HyprConfig)
	}
	return s.resolveHyprConfig(ctx)
}

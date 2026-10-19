package apply

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

// enableWakeCheck belongs to one apply, not the daemon's background wake state.
// Only an off-to-on transition requests power: ordinary sleep on an already
// enabled output must survive unrelated layout edits and reconciliation.
func (e Engine) enableWakeCheck(p profile.Profile, before []hypr.Monitor, lua bool) func(context.Context, []hypr.Monitor) error {
	resolver := profile.NewMonitorResolver(before)
	var targets []hypr.Monitor
	for _, output := range p.Outputs {
		if monitor, ok := resolver.ResolveOutput(output); ok && output.Enabled && monitor.Disabled {
			targets = append(targets, monitor)
		}
	}
	attempts := make(map[string]int)
	last := make(map[string]time.Time)
	return func(ctx context.Context, after []hypr.Monitor) error {
		current := profile.NewMonitorResolver(after)
		var problems []error
		for _, target := range targets {
			monitor, ok := current.Resolve(target.HardwareKey(), target.Name)
			if !ok || monitor.Disabled {
				problems = append(problems, fmt.Errorf("%s has not become enabled", target.Name))
				continue
			}
			if monitor.DPMSStatus {
				continue
			}
			problems = append(problems, fmt.Errorf("%s remains asleep after enabling", monitor.Name))
			// Mode application can expose an output before its first wake can take
			// effect. Retry a bounded number of times within the normal apply deadline.
			if attempts[target.Name] >= 3 || time.Since(last[target.Name]) < 500*time.Millisecond {
				continue
			}
			attempts[target.Name]++
			last[target.Name] = time.Now()
			command := fmt.Sprintf("dispatch dpms on %s", monitor.Name)
			if lua {
				command = fmt.Sprintf("dispatch hl.dsp.dpms({ action = %q, monitor = %q })", "enable", monitor.Name)
			}
			if !hypr.BatchSafe(command) {
				problems = append(problems, fmt.Errorf("unsafe display name for wake: %q", monitor.Name))
				continue
			}
			if err := e.applyLiveCommands(ctx, []string{command}); err != nil {
				problems = append(problems, fmt.Errorf("wake %s: %w", monitor.Name, err))
			}
		}
		return errors.Join(problems...)
	}
}

package daemon

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/crmne/hyprmoncfg/internal/config"
	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

// Some displays will not come up in the settings a profile saved for them.
// Some cannot wake straight into a demanding mode such as 4K at 144 Hz: they
// drop off the link a second after connecting, and when they come back they
// sit on "No signal" and go back to sleep. Woken at about 60 Hz and switched
// to the saved mode once awake, they work. So a display that drops right after
// connecting at its saved settings is remembered as needing a gentle wake:
// from then on its rule wakes it at about 60 Hz, and the daemon switches it to
// the saved settings once it has stayed connected for a few seconds.
//
// If that switch makes it drop again, or it keeps coming back from a wake
// without a mode, the daemon steps its settings down one at a time (VRR off,
// then a lower refresh rate, then about 60 Hz) and keeps the first that
// sticks. The saved profile never changes. Both are live state, remembered per
// display so a restart does not start the struggle over. Applying a layout by
// hand drops the steps; the gentle wake stays, because it still ends at the
// saved settings.
//
// What this cannot see is a display that stays connected but shows nothing.
// Nothing Hyprland or the kernel reports tells that apart from a working one.
const (
	// A connection this short that ends on its own is a drop, not someone
	// turning the display off after using it.
	fallbackShortConnection = 8 * time.Second
	// The bounce that marks a display for a gentle wake comes a second or two
	// after connecting. Unplugging and replugging by hand takes longer, and
	// must not slow down every future wake of a display that works.
	fallbackBounce = 4 * time.Second
	// How long a gently woken display stays connected before it gets its
	// saved settings. Switching a second after connecting knocked the
	// display that needed this off again; ten seconds held.
	fallbackSettle = 10 * time.Second
	// Applies that left the display enabled but without a mode.
	fallbackNoModeThreshold = 2
	fallbackStateFile       = "display-fallbacks.json"
)

// fallbackTriggerPrefix marks a reconciliation that writes a new step.
const fallbackTriggerPrefix = "display-fallback:"

const (
	fallbackReasonDropping = "dropping"
	fallbackReasonNoMode   = "no_mode"
)

type displayFallback struct {
	Step   profile.FallbackStep `json:"step"`
	Reason string               `json:"reason"`
	// Saved is the mode and VRR this step replaces. A different saved
	// setting means the profile changed and the step no longer applies.
	Saved string `json:"saved"`
	// Running says how the display runs now, in words.
	Running string    `json:"running,omitempty"`
	Since   time.Time `json:"since"`
	// GentleWake wakes the display at about 60 Hz before its settings.
	GentleWake bool `json:"gentle_wake,omitempty"`
}

type displayFallbacks struct {
	path string
	logf func(string, ...any)

	now    func() time.Time
	settle time.Duration

	mu          sync.Mutex
	connectedAt map[string]time.Time
	switchedAt  map[string]time.Time
	noMode      map[string]int
	due         map[string]string
	lastSeen    map[string]hypr.Monitor
	active      map[string]displayFallback
}

func newDisplayFallbacks(configDir string, logf func(string, ...any)) *displayFallbacks {
	f := &displayFallbacks{
		logf:        logf,
		now:         time.Now,
		settle:      fallbackSettle,
		connectedAt: map[string]time.Time{},
		switchedAt:  map[string]time.Time{},
		noMode:      map[string]int{},
		due:         map[string]string{},
		lastSeen:    map[string]hypr.Monitor{},
		active:      map[string]displayFallback{},
	}
	if configDir != "" {
		f.path = filepath.Join(configDir, fallbackStateFile)
		if content, err := os.ReadFile(f.path); err == nil {
			if err := json.Unmarshal(content, &f.active); err != nil {
				logf("ignoring unreadable %s: %v", f.path, err)
				f.active = map[string]displayFallback{}
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			logf("could not read %s: %v", f.path, err)
		}
	}
	return f
}

func fallbackKey(description string) string {
	return strings.ToLower(strings.TrimSpace(description))
}

// observeEvent watches displays drop right after connecting. It returns true
// when that calls for a new rule while the display is gone: a first drop at
// the saved settings asks for a gentle wake from now on, and a drop right after
// the switch from a gentle wake asks for a gentler setting. Only Hyprland's v2
// events name the display, so the v1 duplicate is ignored.
func (f *displayFallbacks) observeEvent(ev hypr.Event, now time.Time) bool {
	parts := strings.SplitN(ev.Value, ",", 3)
	if len(parts) != 3 {
		return false
	}
	key := fallbackKey(parts[2])
	if key == "" {
		return false
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	switch ev.Type {
	case hypr.EventMonitorAdded:
		f.connectedAt[key] = now
		delete(f.switchedAt, key)
	case hypr.EventMonitorRemoved:
		connected, wasConnected := f.connectedAt[key]
		switched, wasSwitched := f.switchedAt[key]
		delete(f.connectedAt, key)
		delete(f.switchedAt, key)
		entry := f.active[key]
		switch {
		case !entry.GentleWake:
			if !wasConnected || now.Sub(connected) > fallbackBounce {
				return false
			}
			entry.GentleWake = true
			if entry.Since.IsZero() {
				entry.Since = now
			}
			f.active[key] = entry
			f.save()
			f.logf("%s dropped right after connecting at its saved settings; it will wake at about 60 Hz first from now on", parts[1]+" ("+parts[2]+")")
			return true
		case wasSwitched && now.Sub(switched) <= fallbackShortConnection:
			f.due[key] = fallbackReasonDropping
			return true
		}
		// A drop while still waking gently changes nothing: the rule for
		// the next connect is the gentle one already.
	}
	return false
}

// settleDelay is how long until a gently woken display is due its saved
// settings, and false when none is waiting.
func (f *displayFallbacks) settleDelay() (time.Duration, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := f.now()
	var wait time.Duration
	waiting := false
	for key, connected := range f.connectedAt {
		if !f.active[key].GentleWake {
			continue
		}
		if _, done := f.switchedAt[key]; done {
			continue
		}
		remaining := max(0, connected.Add(f.settle).Sub(now))
		if !waiting || remaining < wait {
			wait, waiting = remaining, true
		}
	}
	return wait, waiting
}

// observeFailedApply counts enabled, awake displays left without a mode by an
// apply that asked for one.
func (f *displayFallbacks) observeFailedApply(effective profile.Profile, monitors []hypr.Monitor) {
	resolver := profile.NewMonitorResolver(monitors)
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, output := range effective.Outputs {
		monitor, ok := resolver.ResolveOutput(output)
		if !ok || !output.Enabled || output.MirrorOf != "" || monitor.Disabled || !monitor.DPMSStatus ||
			(monitor.Width > 0 && monitor.Height > 0) {
			continue
		}
		key := fallbackKey(monitor.Description)
		if key == "" {
			continue
		}
		f.noMode[key]++
		if f.noMode[key] >= fallbackNoModeThreshold {
			delete(f.noMode, key)
			f.due[key] = fallbackReasonNoMode
		}
	}
}

// observeApplied forgets modeless strikes once an apply verified the layout.
func (f *displayFallbacks) observeApplied() {
	f.mu.Lock()
	clear(f.noMode)
	f.mu.Unlock()
}

// reset drops every step, for when someone applies a layout by hand: that is
// a request for exactly those settings. A gentle wake stays, because it still
// ends at the saved settings.
func (f *displayFallbacks) reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	clear(f.noMode)
	clear(f.due)
	changed := false
	for key, entry := range f.active {
		if entry.Step == profile.FallbackNone {
			continue
		}
		changed = true
		if entry.GentleWake {
			f.active[key] = displayFallback{GentleWake: true, Since: entry.Since}
		} else {
			delete(f.active, key)
		}
	}
	if changed {
		f.save()
		f.logf("display fallbacks cleared; using saved settings again")
	}
}

// apply takes any step that has become due and returns the profile with every
// remembered step in place. Displays that are not connected right now are
// stepped too, using their last known modes, because the generated config
// holds their rule for the next time they connect.
func (f *displayFallbacks) apply(p profile.Profile, monitors []hypr.Monitor) profile.Profile {
	f.mu.Lock()
	defer f.mu.Unlock()

	descriptions := map[string]int{}
	for _, monitor := range monitors {
		descriptions[fallbackKey(monitor.Description)]++
	}
	saved := map[string]int{}
	for _, output := range p.Outputs {
		saved[fallbackKey(output.Description)]++
	}
	for _, monitor := range monitors {
		if key := fallbackKey(monitor.Description); key != "" && descriptions[key] == 1 && len(monitor.AvailableModes) > 0 {
			f.lastSeen[key] = monitor
		}
	}

	resolver := profile.NewMonitorResolver(monitors)
	changed := false
	p.Outputs = append([]profile.OutputConfig(nil), p.Outputs...)
	for i, output := range p.Outputs {
		key := fallbackKey(output.Description)
		if monitor, ok := resolver.ResolveOutput(output); ok {
			key = fallbackKey(monitor.Description)
		}
		// Two displays with one description cannot be told apart here.
		if key == "" || descriptions[key] > 1 || saved[fallbackKey(output.Description)] > 1 {
			continue
		}
		monitor, known := f.lastSeen[key]
		settings := output.NormalizedMode() + " vrr " + strconv.Itoa(output.VRR)
		current := f.active[key]
		stepped := current.Step != profile.FallbackNone
		if stepped && current.Saved != settings {
			current = displayFallback{GentleWake: current.GentleWake, Since: current.Since}
			if current.GentleWake {
				f.active[key] = current
			} else {
				delete(f.active, key)
			}
			stepped, changed = false, true
		}
		if reason, due := f.due[key]; due && known {
			delete(f.due, key)
			from := profile.FallbackNone
			if stepped {
				from = current.Step
			}
			if next, ok := profile.NextFallback(output, monitor, from); ok {
				current = displayFallback{Step: next, Reason: reason, Saved: settings,
					Running: profile.DescribeFallback(output, monitor, next), Since: f.now(), GentleWake: current.GentleWake}
				f.active[key] = current
				stepped, changed = true, true
				f.logf("%s %s at its saved settings; running it %s (the saved profile is unchanged)",
					displayLabel(monitor), fallbackReasonText(reason), current.Running)
			} else {
				f.logf("%s %s, and it is already at its gentlest settings", displayLabel(monitor), fallbackReasonText(reason))
			}
		}
		if !known {
			continue
		}
		applied := output
		if stepped {
			applied = profile.OutputWithFallback(output, monitor, current.Step)
		}
		if current.GentleWake {
			_, connected := resolver.ResolveOutput(output)
			connectedAt, recent := f.connectedAt[key]
			// Connected with no connect seen means it was already on when
			// the daemon started, so it is awake.
			settled := connected && (!recent || f.now().Sub(connectedAt) >= f.settle)
			if !settled {
				applied = profile.OutputWithFallback(output, monitor, profile.FallbackSafeRefresh)
			} else if _, done := f.switchedAt[key]; !done && recent {
				f.switchedAt[key] = f.now()
			}
		}
		p.Outputs[i] = applied
	}
	if changed {
		f.save()
	}
	return p
}

// describe returns the remembered step of each connected display, by connector.
// A gentle wake alone is not reported: the display still runs as saved.
func (f *displayFallbacks) describe(monitors []hypr.Monitor) map[string]displayFallback {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.active) == 0 {
		return nil
	}
	steps := map[string]displayFallback{}
	for _, monitor := range monitors {
		if step, ok := f.active[fallbackKey(monitor.Description)]; ok && step.Step != profile.FallbackNone {
			steps[monitor.Name] = step
		}
	}
	return steps
}

// save writes the remembered steps. The caller holds f.mu.
func (f *displayFallbacks) save() {
	if f.path == "" {
		return
	}
	if len(f.active) == 0 {
		if err := os.Remove(f.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			f.logf("could not remove %s: %v", f.path, err)
		}
		return
	}
	content, err := json.MarshalIndent(f.active, "", "  ")
	if err == nil {
		err = config.WriteFileAtomic(f.path, append(content, '\n'), 0o644)
	}
	if err != nil {
		f.logf("could not save display fallbacks to %s: %v", f.path, err)
	}
}

func fallbackReasonText(reason string) string {
	if reason == fallbackReasonNoMode {
		return "kept coming back without a mode"
	}
	return "kept disconnecting right after connecting"
}

func displayLabel(monitor hypr.Monitor) string {
	if monitor.Description == "" {
		return monitor.Name
	}
	return monitor.Name + " (" + monitor.Description + ")"
}

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

// Some displays will not stay on in the settings a profile saved for them.
// They drop off the link moments after every connect, or they come back from
// a wake without a mode. Rather than retry the same thing forever, the daemon
// steps such a display down one setting at a time (VRR off, then a lower
// refresh rate, then about 60 Hz) and keeps the first one that sticks. The
// saved profile never changes: the step is live state, remembered per display
// so a restart does not start the struggle over, and it is dropped as soon as
// someone applies a layout by hand or the saved settings change.
//
// What this cannot see is a display that stays connected but shows nothing.
// Nothing Hyprland or the kernel reports tells that apart from a working one.
const (
	// A normal display connects once. Some bounce a single time while
	// powering up, so one short connection is not a failure.
	fallbackShortConnection = 8 * time.Second
	fallbackDropWindow      = 3 * time.Minute
	fallbackDropThreshold   = 3
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
	Running string    `json:"running"`
	Since   time.Time `json:"since"`
}

type displayFallbacks struct {
	path string
	logf func(string, ...any)

	mu          sync.Mutex
	connectedAt map[string]time.Time
	drops       map[string][]time.Time
	noMode      map[string]int
	due         map[string]string
	lastSeen    map[string]hypr.Monitor
	active      map[string]displayFallback
}

func newDisplayFallbacks(configDir string, logf func(string, ...any)) *displayFallbacks {
	f := &displayFallbacks{
		logf:        logf,
		connectedAt: map[string]time.Time{},
		drops:       map[string][]time.Time{},
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

// observeEvent counts a display dropping off shortly after it connected. It
// returns true when that makes the display due for a gentler setting. Only
// Hyprland's v2 events name the display, so the v1 duplicate is ignored.
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
	case hypr.EventMonitorRemoved:
		connected, ok := f.connectedAt[key]
		delete(f.connectedAt, key)
		if !ok || now.Sub(connected) > fallbackShortConnection {
			return false
		}
		recent := f.drops[key][:0]
		for _, drop := range f.drops[key] {
			if now.Sub(drop) <= fallbackDropWindow {
				recent = append(recent, drop)
			}
		}
		f.drops[key] = append(recent, now)
		if len(f.drops[key]) >= fallbackDropThreshold {
			delete(f.drops, key)
			f.due[key] = fallbackReasonDropping
			return true
		}
	}
	return false
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
// a request for exactly those settings.
func (f *displayFallbacks) reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	clear(f.drops)
	clear(f.noMode)
	clear(f.due)
	if len(f.active) == 0 {
		return
	}
	clear(f.active)
	f.save()
	f.logf("display fallbacks cleared; using saved settings again")
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
		current, stepped := f.active[key]
		if stepped && current.Saved != settings {
			delete(f.active, key)
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
					Running: profile.DescribeFallback(output, monitor, next), Since: time.Now()}
				f.active[key] = current
				stepped, changed = true, true
				f.logf("%s %s at its saved settings; running it %s (the saved profile is unchanged)",
					displayLabel(monitor), fallbackReasonText(reason), current.Running)
			} else {
				f.logf("%s %s, and it is already at its gentlest settings", displayLabel(monitor), fallbackReasonText(reason))
			}
		}
		if stepped && known {
			p.Outputs[i] = profile.OutputWithFallback(output, monitor, current.Step)
		}
	}
	if changed {
		f.save()
	}
	return p
}

// describe returns the remembered step of each connected display, by connector.
func (f *displayFallbacks) describe(monitors []hypr.Monitor) map[string]displayFallback {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.active) == 0 {
		return nil
	}
	steps := map[string]displayFallback{}
	for _, monitor := range monitors {
		if step, ok := f.active[fallbackKey(monitor.Description)]; ok {
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

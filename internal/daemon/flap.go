package daemon

import (
	"time"

	"github.com/crmne/hyprmoncfg/internal/hypr"
)

// displayFlapWindow bounds how long a sleeping connector's link drop still
// explains a relight. Monitors with input auto-switching scan their other
// inputs a second or two after the source stops sending signal and return
// just as quickly; unplugging and replugging by hand takes longer.
const displayFlapWindow = 15 * time.Second

// linkFlaps tracks connectors that dropped their link while the displays were
// asleep. Such a drop comes back lit because Hyprland starts a re-added output
// with DPMS on, and that must not be mistaken for a person asking for the
// displays back: an input-auto-switching monitor scans its other inputs when a
// sleeping source goes quiet, which hot-unplugs the output and lights it again
// on the way back to the same input.
type linkFlaps struct {
	removedAt map[string]time.Time
}

func newLinkFlaps() *linkFlaps {
	return &linkFlaps{removedAt: make(map[string]time.Time)}
}

// Removed records a connector whose link dropped at now. Only call this while
// the displays are asleep: a drop while they are lit is a real unplug.
func (f *linkFlaps) Removed(name string, now time.Time) {
	f.removedAt[name] = now
}

// RelitDuringSleep returns the flapped connectors when every lit output is one
// that dropped its link within the flap window and came back. An output lit
// without having dropped is a real wake — a new connection, or the user
// asking for light — and consumes nothing.
func (f *linkFlaps) RelitDuringSleep(monitors []hypr.Monitor, now time.Time) ([]string, bool) {
	var relit []string
	for _, monitor := range monitors {
		if monitor.Disabled || !monitor.DPMSStatus {
			continue
		}
		droppedAt, ok := f.removedAt[monitor.Name]
		if !ok || now.Sub(droppedAt) > displayFlapWindow {
			return nil, false
		}
		relit = append(relit, monitor.Name)
	}
	if len(relit) == 0 {
		return nil, false
	}
	return relit, true
}

// Consume drops the flap records behind one explained relight so they cannot
// explain another. A later relight without fresh drops is a real wake.
func (f *linkFlaps) Consume(names []string) {
	for _, name := range names {
		delete(f.removedAt, name)
	}
}

// Reset forgets the episode: a fresh sleep or a real wake.
func (f *linkFlaps) Reset() {
	f.removedAt = make(map[string]time.Time)
}

package daemon

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/crmne/hyprmoncfg/internal/hypr"
)

// aquamarine 0.15.0 never disables the CRTC of a display that disconnects,
// so the kernel keeps it routed to the connector that is gone. A display
// that reconnects and is handed that CRTC fails every modeset and stays
// enabled at 0x0; no reload or lower mode helps (hyprwm/aquamarine#386,
// fixed in 0.15.1). Switching to another console and back makes the
// compositor reset the kernel state, and a reload then sets the mode.
const (
	// How long to stay on the other console. Two seconds was enough on an
	// RTX 3090 with two DisplayPort monitors.
	staleCRTCConsoleAway = 2 * time.Second
	// How long the compositor gets to take the displays back before the
	// reload.
	staleCRTCConsoleReturn = 2 * time.Second
	// The first recovery runs on the first failed apply with the signature;
	// later ones wait this long, doubling, because each one blanks every
	// display.
	staleCRTCRetryWait    = 30 * time.Second
	staleCRTCRetryMaxWait = 10 * time.Minute
)

type staleCRTCRecovery struct {
	mu         sync.Mutex
	now        func() time.Time
	away       time.Duration
	returnWait time.Duration
	attempts   int
	next       time.Time
	reported   bool
}

// staleCRTCStuck lists the awake, enabled outputs left without a mode
// although they advertise modes, which a stale CRTC causes.
func staleCRTCStuck(monitors []hypr.Monitor) []string {
	var stuck []string
	for _, monitor := range monitors {
		if monitor.Name == "FALLBACK" || monitor.Disabled || !monitor.DPMSStatus {
			continue
		}
		if (monitor.Width == 0 || monitor.Height == 0) && len(monitor.AvailableModes) > 0 {
			stuck = append(stuck, monitor.Name)
		}
	}
	return stuck
}

// staleCRTCEvidence sorts the connectors whose kernel CRTC the compositor
// cannot use. released lists disconnected connectors still enabled: a
// console switch makes the compositor release those. swapped lists stuck
// outputs whose own connector is still enabled while every connector is
// connected, as when two reconnecting displays are handed each other's
// CRTC (hyprwm/aquamarine#428). A console switch does not clear that: the
// compositor keeps its swapped records, the restore fails and the screens
// keep the console's last image. Only a display disconnecting clears it.
func staleCRTCEvidence(stuck []string, states []hypr.DRMConnectorState) (released, swapped []string) {
	isStuck := map[string]bool{}
	for _, name := range stuck {
		isStuck[name] = true
	}
	for _, state := range states {
		switch {
		case state.Enabled && !state.Connected:
			released = append(released, state.Name)
		case state.Enabled && isStuck[state.Name]:
			swapped = append(swapped, state.Name)
		}
	}
	return released, swapped
}

// observe records one failed apply. It reports whether the failure carries
// the stale-CRTC signature, and whether a console switch is due now.
func (r *staleCRTCRecovery) observe(stuck, stale []string) (matched, due bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(stuck) == 0 || len(stale) == 0 {
		r.attempts, r.next, r.reported = 0, time.Time{}, false
		return false, false
	}
	if r.now().Before(r.next) {
		return true, false
	}
	wait := staleCRTCRetryWait
	for i := 0; i < r.attempts && wait < staleCRTCRetryMaxWait; i++ {
		wait *= 2
	}
	r.attempts++
	r.next = r.now().Add(min(wait, staleCRTCRetryMaxWait))
	return true, true
}

func (r *staleCRTCRecovery) firstReport() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	first := !r.reported
	r.reported = true
	return first
}

// recoverStaleCRTC handles a failed apply whose readback shows the
// stale-CRTC signature. It reports whether it did, so the failure is not
// taken as evidence against the display's settings.
func (s *Service) recoverStaleCRTC(ctx context.Context, after []hypr.Monitor) bool {
	if s.cfg.ConnectorStates == nil {
		return false
	}
	stuck := staleCRTCStuck(after)
	if len(stuck) == 0 {
		s.staleCRTC.observe(nil, nil)
		return false
	}
	released, swapped := staleCRTCEvidence(stuck, s.cfg.ConnectorStates())
	if len(released) == 0 {
		matched, _ := s.staleCRTC.observe(stuck, swapped)
		if matched && s.staleCRTC.firstReport() {
			s.cfg.Logf("%s kept failing without a mode while each still holds a display pipe the compositor gave another monitor: a display stack bug (aquamarine 0.15.0, fixed in 0.15.1); turn one of these monitors off, wait 5 seconds, and turn it on again",
				strings.Join(stuck, ", "))
		}
		return matched
	}
	matched, due := s.staleCRTC.observe(stuck, released)
	if !matched {
		return false
	}
	if s.staleCRTC.firstReport() {
		s.cfg.Logf("%s kept failing without a mode while the kernel still drives disconnected %s: a display stack bug (aquamarine 0.15.0, fixed in 0.15.1) that switching to another console and back clears",
			strings.Join(stuck, ", "), strings.Join(released, ", "))
	}
	if !due {
		return true
	}
	if s.cfg.SwitchConsole == nil {
		return true
	}
	s.cfg.Logf("switching to another console and back to reset the displays")
	if err := s.cfg.SwitchConsole(ctx, s.staleCRTC.away); err != nil {
		s.cfg.Logf("console switch failed: %v; switch consoles by hand (for example sudo chvt 2, then sudo chvt 1) or update aquamarine", err)
		return true
	}
	select {
	case <-ctx.Done():
		return true
	case <-time.After(s.staleCRTC.returnWait):
	}
	if err := s.client.Reload(ctx); err != nil {
		s.cfg.Logf("reload after the console switch failed: %v", err)
	}
	return true
}

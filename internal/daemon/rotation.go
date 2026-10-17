package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/crmne/hyprmoncfg/internal/appstatus"
	"github.com/crmne/hyprmoncfg/internal/ipc"
	"github.com/crmne/hyprmoncfg/internal/lid"
	"github.com/crmne/hyprmoncfg/internal/orientation"
)

// autoRotation turns built-in panels set to rotate automatically with the
// device. The saved profile stays upright; the turn is live state applied on
// top. A convertible with a tablet-mode switch turns only in tablet mode, as
// GNOME does, so a laptop tilted on a lap stays put. The rotation lock holds
// the current turn, and survives restarts.
type autoRotation struct {
	mu   sync.Mutex
	path string
	logf func(string, ...any)

	available   bool
	orientation string
	quarters    int
	// tabletSwitch is true when the device has a tablet-mode switch that
	// can be read; tabletMode is its state.
	tabletSwitch bool
	tabletMode   bool
	reportedRead bool

	locked         bool
	lockedQuarters int
	// reversed turns panels the other way, for a device whose accelerometer
	// reports the opposite direction.
	reversed bool
}

const (
	rotationStateFile = "rotation.json"
	// How long a new orientation must hold before the panel turns.
	rotationSettle = 300 * time.Millisecond
	// How often the tablet-mode switch is read. It has no event the daemon
	// can wait on without opening the input device for good.
	rotationTabletModePoll = time.Second
)

type rotationState struct {
	Locked   bool `json:"locked"`
	Quarters int  `json:"quarters"`
	Reversed bool `json:"reversed,omitempty"`
}

func newAutoRotation(configDir string, logf func(string, ...any)) *autoRotation {
	r := &autoRotation{logf: logf}
	if configDir == "" {
		return r
	}
	r.path = filepath.Join(configDir, rotationStateFile)
	if data, err := os.ReadFile(r.path); err == nil {
		var state rotationState
		if json.Unmarshal(data, &state) == nil {
			r.locked, r.lockedQuarters = state.Locked, ((state.Quarters%4)+4)%4
			r.reversed = state.Reversed
		}
	}
	return r
}

// turnLocked is the quarter turns to apply. The caller holds r.mu.
func (r *autoRotation) turnLocked() int {
	switch {
	case !r.available:
		return 0
	case r.locked:
		return r.lockedQuarters
	case r.tabletSwitch && !r.tabletMode:
		return 0
	case r.reversed:
		return (4 - r.quarters) % 4
	}
	return r.quarters
}

// turn is the quarter turns the daemon applies to panels that rotate.
func (r *autoRotation) turn() int {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.turnLocked()
}

// observe records a sensor reading. It reports whether the status changed
// and whether the applied turn did.
func (r *autoRotation) observe(reading orientation.Reading) (statusChanged, turnChanged bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	before, beforeAvailable, beforeOrientation := r.turnLocked(), r.available, r.orientation
	r.available = reading.Available
	r.orientation = ""
	if reading.Available {
		r.orientation = reading.Orientation
		// Lying flat keeps the last turn.
		if quarters, ok := orientation.Quarters(reading.Orientation); ok {
			r.quarters = quarters
		}
	}
	turnChanged = r.turnLocked() != before
	statusChanged = turnChanged || r.available != beforeAvailable || r.orientation != beforeOrientation
	return statusChanged, turnChanged
}

// observeTabletMode records a read of the tablet-mode switch.
func (r *autoRotation) observeTabletMode(on bool, err error) (turnChanged bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	before := r.turnLocked()
	switch {
	case err == nil:
		r.tabletSwitch, r.tabletMode = true, on
	case errors.Is(err, lid.ErrNoTabletModeSwitch):
		r.tabletSwitch = false
	default:
		// An unreadable switch must not stop rotation for good.
		if !r.reportedRead {
			r.reportedRead = true
			r.logf("tablet mode unknown (%v); rotating in any mode", err)
		}
		r.tabletSwitch = false
	}
	return r.turnLocked() != before
}

// set holds or releases the current turn and sets the direction; nil
// leaves either as it is. It reports whether the applied turn changed, as it
// does when releasing a lock taken in another orientation or reversing.
func (r *autoRotation) set(locked, reversed *bool) (turnChanged bool, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	before := r.turnLocked()
	if reversed != nil {
		r.reversed = *reversed
	}
	if locked != nil {
		if *locked && !r.locked {
			r.lockedQuarters = before
		}
		r.locked = *locked
	}
	if r.path != "" {
		data, _ := json.Marshal(rotationState{Locked: r.locked, Quarters: r.lockedQuarters, Reversed: r.reversed})
		if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
			return false, err
		}
		if err := os.WriteFile(r.path, data, 0o644); err != nil {
			return false, err
		}
	}
	return r.turnLocked() != before, nil
}

func (r *autoRotation) status() *appstatus.Rotation {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return &appstatus.Rotation{
		Available:   r.available,
		Orientation: r.orientation,
		Quarters:    r.turnLocked(),
		Locked:      r.locked,
		Reversed:    r.reversed,
		TabletMode:  r.tabletSwitch && r.tabletMode,
		Paused:      r.available && !r.locked && r.tabletSwitch && !r.tabletMode,
	}
}

func (s *Service) editorRotation() appstatus.EditorRotation {
	if s.cfg.WatchOrientation == nil {
		return appstatus.EditorRotation{}
	}
	status := s.rotation.status()
	return appstatus.EditorRotation{Available: status.Available, Quarters: status.Quarters}
}

// SetRotation holds or releases the turn the orientation sensor gives
// panels that rotate automatically, or reverses its direction. A change that
// turns the panels applies right away.
func (s *Service) SetRotation(params ipc.RotationParams) error {
	if s.cfg.WatchOrientation == nil {
		return errors.New("automatic rotation is not available")
	}
	turnChanged, err := s.rotation.set(params.Locked, params.Reversed)
	if err != nil {
		return err
	}
	if params.Locked != nil {
		if *params.Locked {
			s.cfg.Logf("rotation locked")
		} else {
			s.cfg.Logf("rotation unlocked")
		}
	}
	if params.Reversed != nil {
		if *params.Reversed {
			s.cfg.Logf("rotation direction reversed")
		} else {
			s.cfg.Logf("rotation direction normal")
		}
	}
	if turnChanged {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := s.applyAutomatic(ctx); err != nil {
			s.cfg.Logf("rotate after a rotation change: %v", err)
		}
	}
	s.signalChange()
	return nil
}

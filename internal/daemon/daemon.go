package daemon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/crmne/hyprmoncfg/internal/apply"
	"github.com/crmne/hyprmoncfg/internal/config"
	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/lid"
	"github.com/crmne/hyprmoncfg/internal/omarchywatch"
	"github.com/crmne/hyprmoncfg/internal/profile"
	"github.com/crmne/hyprmoncfg/internal/suspend"
)

type Config struct {
	Debounce        time.Duration
	WakeSettle      time.Duration
	PollInterval    time.Duration
	LidPollInterval time.Duration
	EventRetry      time.Duration
	ForcedProfile   string
	MonitorsConf    string
	HyprConfig      string
	Logf            func(format string, args ...any)
}

type Service struct {
	client        *hypr.Client
	store         *profile.Store
	engine        apply.Engine
	cfg           Config
	writeMu       sync.Mutex
	pendingMu     sync.Mutex
	pending       *pendingTransaction
	manualMu      sync.Mutex
	manualSet     string
	manualProfile profile.Profile
	notifyMu      sync.RWMutex
	notify        func()
	applied       string
	lastSeenHash  string
	lidState      lid.State
	lidSupported  bool

	readLid      func(context.Context) (lid.State, error)
	watchLid     func(context.Context, time.Duration) (<-chan lid.State, <-chan error)
	watchSuspend func(context.Context) <-chan bool
}

var errDisplaysSleeping = errors.New("displays are sleeping")

type displaySleepTransition uint8

const (
	displaySleepUnchanged displaySleepTransition = iota
	displaySleepEntered
	displaySleepExited
)

type displaySleepGuard struct {
	sleeping bool
}

func (g *displaySleepGuard) Observe(monitors []hypr.Monitor) displaySleepTransition {
	switch displayPowerState(monitors) {
	case displayPowerAsleep:
		if !g.sleeping {
			g.sleeping = true
			return displaySleepEntered
		}
	case displayPowerAwake:
		if g.sleeping {
			g.sleeping = false
			return displaySleepExited
		}
	}
	return displaySleepUnchanged
}

func (g *displaySleepGuard) MarkSleeping() bool {
	if g.sleeping {
		return false
	}
	g.sleeping = true
	return true
}

type displayPower uint8

const (
	displayPowerUnknown displayPower = iota
	displayPowerAwake
	displayPowerAsleep
)

func displayPowerState(monitors []hypr.Monitor) displayPower {
	enabled := 0
	for _, monitor := range monitors {
		if monitor.Disabled {
			continue
		}
		enabled++
		if monitor.DPMSStatus {
			return displayPowerAwake
		}
	}
	if enabled > 0 {
		return displayPowerAsleep
	}
	return displayPowerUnknown
}

func New(client *hypr.Client, store *profile.Store, cfg Config) *Service {
	if cfg.Debounce <= 0 {
		cfg.Debounce = 1200 * time.Millisecond
	}
	if cfg.WakeSettle <= 0 {
		cfg.WakeSettle = 2 * time.Second
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 5 * time.Second
	}
	if cfg.LidPollInterval <= 0 {
		cfg.LidPollInterval = lid.DefaultPollInterval
	}
	if cfg.EventRetry <= 0 {
		cfg.EventRetry = 5 * time.Second
	}
	if cfg.Logf == nil {
		cfg.Logf = func(string, ...any) {}
	}
	return &Service{
		client: client,
		store:  store,
		engine: apply.Engine{
			Client:             client,
			MonitorsConfPath:   cfg.MonitorsConf,
			HyprlandConfigPath: cfg.HyprConfig,
			Logf:               cfg.Logf,
		},
		cfg:          cfg,
		lidState:     lid.Unknown,
		readLid:      lid.ReadState,
		watchLid:     lid.Watch,
		watchSuspend: suspend.Watch,
	}
}

func (s *Service) Run(ctx context.Context) error {
	if s.client == nil || s.store == nil {
		return fmt.Errorf("daemon not initialized")
	}
	if err := s.store.Ensure(); err != nil {
		return err
	}
	s.ensureConfigInclude(ctx)

	type trigger struct {
		reason string
		delay  time.Duration
	}
	triggerCh := make(chan trigger, 8)
	pushTrigger := func(reason string, delay time.Duration) {
		select {
		case triggerCh <- trigger{reason: reason, delay: delay}:
		default:
		}
	}

	pushTrigger("startup", s.cfg.Debounce)

	var lidStates <-chan lid.State
	var lidErrs <-chan error
	if state, err := s.readLid(ctx); err != nil {
		s.cfg.Logf("lid events disabled: %v", err)
	} else {
		s.lidSupported = true
		s.lidState = state
		s.cfg.Logf("lid state: %s", state)
		lidStates, lidErrs = s.watchLid(ctx, s.cfg.LidPollInterval)
	}

	suspendEvents := s.watchSuspend(ctx)

	events, eventErrs := s.client.SubscribeMonitorEvents(ctx)
	var eventRetry <-chan time.Time
	scheduleEventRetry := func() {
		if eventRetry == nil {
			eventRetry = time.After(s.cfg.EventRetry)
		}
	}

	pollTicker := time.NewTicker(s.cfg.PollInterval)
	defer pollTicker.Stop()

	debounceTimer := time.NewTimer(s.cfg.Debounce)
	if !debounceTimer.Stop() {
		<-debounceTimer.C
	}

	pending := false
	settlingAfterWake := false
	displayGuard := displaySleepGuard{}
	stopDebounce := func() {
		if !debounceTimer.Stop() {
			select {
			case <-debounceTimer.C:
			default:
			}
		}
	}
	deferForDisplaySleep := func(reason string) {
		pending = true
		settlingAfterWake = false
		stopDebounce()
		if reason != "" {
			s.cfg.Logf("automatic switching deferred while displays sleep: %s", reason)
		}
	}
	scheduleMonitorTrigger := func(reason string) {
		delay := s.cfg.Debounce
		if settlingAfterWake {
			delay = s.cfg.WakeSettle
		}
		pushTrigger(reason, delay)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case err, ok := <-eventErrs:
			if !ok {
				eventErrs = nil
				if events == nil {
					scheduleEventRetry()
				}
				continue
			}
			if err != nil {
				s.cfg.Logf("socket2 unavailable: %v; retrying in %s", err, s.cfg.EventRetry)
			}
		case ev, ok := <-events:
			if !ok {
				events = nil
				if eventErrs == nil {
					scheduleEventRetry()
				}
				continue
			}
			reason := string(ev.Type) + ":" + ev.Value
			monitors, err := s.client.Monitors(ctx)
			if err != nil {
				if displayGuard.sleeping {
					deferForDisplaySleep(reason)
					continue
				}
			} else {
				switch displayGuard.Observe(monitors) {
				case displaySleepEntered:
					s.cfg.Logf("display sleep detected; pausing automatic switching")
					deferForDisplaySleep(reason)
					continue
				case displaySleepExited:
					settlingAfterWake = true
					s.cfg.Logf("display wake detected; waiting %s for monitors to settle", s.cfg.WakeSettle)
					scheduleMonitorTrigger("display-wake:" + reason)
					continue
				}
				if displayGuard.sleeping {
					deferForDisplaySleep(reason)
					continue
				}
			}
			scheduleMonitorTrigger(reason)
		case <-eventRetry:
			eventRetry = nil
			if events == nil && eventErrs == nil {
				events, eventErrs = s.client.SubscribeMonitorEvents(ctx)
			}
		case sleeping, ok := <-suspendEvents:
			if !ok {
				suspendEvents = nil
				continue
			}
			if sleeping {
				// A lid close that suspends the machine must not be applied on
				// resume: by then the lid is usually open again, and honoring
				// the stale close would turn the panel off in the user's face.
				if pending {
					s.cfg.Logf("suspending; dropped the pending trigger")
				}
				pending = false
				settlingAfterWake = false
				stopDebounce()
				continue
			}
			s.cfg.Logf("resumed from sleep; waking displays")
			s.refreshLidState(ctx)
			s.wakeDisplays(ctx)
			displayGuard.sleeping = false
			settlingAfterWake = true
			pushTrigger("resume", s.cfg.WakeSettle)
		case state, ok := <-lidStates:
			if !ok {
				lidStates = nil
				continue
			}
			if state != s.lidState {
				s.lidState = state
				s.clearManualOverride()
				reason := "lid:" + string(state)
				if state == lid.Open {
					// Opening the lid is an explicit ask for light. Wake the
					// displays instead of waiting for a keypress to do it.
					s.wakeDisplays(ctx)
					if displayGuard.sleeping {
						displayGuard.sleeping = false
						settlingAfterWake = true
					}
				}
				if displayGuard.sleeping {
					deferForDisplaySleep(reason)
				} else {
					scheduleMonitorTrigger(reason)
				}
			}
		case err, ok := <-lidErrs:
			if !ok {
				lidErrs = nil
				continue
			}
			if err != nil {
				s.cfg.Logf("lid state unavailable: %v", err)
			}
		case <-pollTicker.C:
			monitors, err := s.client.Monitors(ctx)
			if err != nil {
				s.cfg.Logf("poll monitors failed: %v", err)
				continue
			}
			switch displayGuard.Observe(monitors) {
			case displaySleepEntered:
				s.cfg.Logf("display sleep detected; pausing automatic switching")
				deferForDisplaySleep("")
				continue
			case displaySleepExited:
				settlingAfterWake = true
				s.cfg.Logf("display wake detected; waiting %s for monitors to settle", s.cfg.WakeSettle)
				scheduleMonitorTrigger("display-wake")
				continue
			}
			if displayGuard.sleeping {
				continue
			}

			h := profile.MonitorStateHash(monitors)
			if h != s.lastSeenHash {
				s.lastSeenHash = h
				scheduleMonitorTrigger("poll-change")
			}
		case next := <-triggerCh:
			if displayGuard.sleeping {
				deferForDisplaySleep(next.reason)
				continue
			}
			s.cfg.Logf("triggered: %s", next.reason)
			pending = true
			stopDebounce()
			debounceTimer.Reset(next.delay)
		case <-debounceTimer.C:
			if !pending {
				continue
			}
			err := s.applyBest(ctx)
			if errors.Is(err, errDisplaysSleeping) {
				if displayGuard.MarkSleeping() {
					s.cfg.Logf("display sleep detected; pausing automatic switching")
				}
				deferForDisplaySleep("")
				continue
			}
			pending = false
			settlingAfterWake = false
			if err != nil {
				s.cfg.Logf("apply failed: %v", err)
			}
			s.signalChange()
		}
	}
}

// ensureConfigInclude makes the generated monitor config the last thing the
// root Hyprland config loads, before any profile is applied. Applying does this
// too, so this only covers the window before the first one.
func (s *Service) ensureConfigInclude(ctx context.Context) {
	version := ""
	versionCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	if info, err := s.client.Version(versionCtx); err == nil {
		version = info.Version
	}
	cancel()

	resolved, err := config.ResolveHyprlandConfig(version, s.cfg.MonitorsConf, s.cfg.HyprConfig)
	if err != nil {
		s.cfg.Logf("could not resolve the Hyprland config: %v", err)
		return
	}

	// Before the first apply the generated file does not exist yet, and an
	// include naming a missing file is a config error on the next reload. The
	// apply that creates it adds the include in the same breath.
	if _, err := os.Stat(resolved.MonitorsPath); err != nil {
		return
	}

	result, err := config.EnsureIncluded(resolved.RootPath, resolved.Format, resolved.MonitorsPath)
	if err != nil {
		s.cfg.Logf("could not load %s from %s: %v", resolved.MonitorsPath, resolved.RootPath, err)
		return
	}
	if result.Changed() {
		action := "moved to the end of"
		if result.Added {
			action = "added to"
		}
		s.cfg.Logf("%s %s: %s", action, result.RootPath, result.Line)
	}
}

// refreshLidState re-reads the lid switch so decisions made now use the lid as
// it is, not as it was when the triggering event fired. The two diverge across
// a suspend: the close that suspended the machine is still the cached state
// when the resume releases the deferred apply, and honoring it would disable
// the internal panel right as the user opens the laptop.
func (s *Service) refreshLidState(ctx context.Context) {
	if !s.lidSupported {
		return
	}
	state, err := s.readLid(ctx)
	if err != nil || !state.Known() || state == s.lidState {
		return
	}
	s.cfg.Logf("lid state: %s", state)
	s.lidState = state
	s.clearManualOverride()
}

// wakeDisplays turns every output's DPMS on. Opening the lid or resuming from
// sleep is the user asking for light; without this the screens stay dark until
// a keypress, and an external monitor left undriven can take half a minute to
// come back on its own.
func (s *Service) wakeDisplays(ctx context.Context) {
	wakeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	version := ""
	if info, err := s.client.Version(wakeCtx); err == nil {
		version = info.Version
	}
	luaDispatch := false
	if resolved, err := config.ResolveHyprlandConfig(version, s.cfg.MonitorsConf, s.cfg.HyprConfig); err == nil {
		luaDispatch = resolved.Format == config.HyprConfigLua
	}
	if err := s.client.WakeDisplays(wakeCtx, luaDispatch); err != nil {
		s.cfg.Logf("could not wake displays: %v", err)
	}
}

func (s *Service) applyBest(ctx context.Context) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	s.pendingMu.Lock()
	interactive := s.pending != nil
	s.pendingMu.Unlock()
	if interactive {
		s.cfg.Logf("automatic switching paused during interactive preview")
		return nil
	}

	s.refreshLidState(ctx)

	monitors, err := s.client.Monitors(ctx)
	if err != nil {
		return err
	}
	if len(monitors) == 0 {
		return nil
	}
	if displayPowerState(monitors) == displayPowerAsleep {
		return errDisplaysSleeping
	}

	hash := profile.MonitorStateHash(monitors)
	monitorSet := profile.MonitorSetHash(monitors)

	var target profile.Profile
	manualHold := false
	if s.cfg.ForcedProfile != "" {
		target, err = s.store.Load(s.cfg.ForcedProfile)
		if err != nil {
			return fmt.Errorf("forced profile %q not found: %w", s.cfg.ForcedProfile, err)
		}
	} else if manual, ok := s.manualOverride(monitorSet); ok {
		// A profile chosen by hand outranks matching until the hardware
		// changes. Standing down here instead would hand the displays to
		// whatever moved them, so re-assert the choice rather than abandon it.
		target = manual
		manualHold = true
	} else {
		profiles, err := s.store.List()
		if err != nil {
			return err
		}
		best, score, ok := profile.BestMatch(profiles, monitors)
		if !ok {
			fallback, fallbackOK := internalOnlyFallbackProfile(monitors)
			if fallbackOK {
				s.cfg.Logf("no matching profile for monitor set %s; enabling internal output", hash)
				target = fallback
			} else {
				s.cfg.Logf("no matching profile for monitor set %s", hash)
				return nil
			}
		} else {
			if s.lidState.Known() {
				s.cfg.Logf("best profile %q score=%d lid=%s", best.Name, score, s.lidState)
			} else {
				s.cfg.Logf("best profile %q score=%d", best.Name, score)
			}
			target = best
		}
	}

	target = s.absorbOmarchyDisplayScale(target, monitors, monitorSet, manualHold)

	effective := target
	if s.lidState == lid.Closed {
		adjusted, adjustment := profile.ApplyClosedLidPolicy(target, monitors)
		effective = adjusted
		if adjustment.Applied {
			disabled := strings.Join(adjustment.DisabledOutputNames, ",")
			if disabled == "" {
				disabled = "already disabled"
			}
			workspaceTarget := adjustment.WorkspaceTargetName
			if workspaceTarget == "" {
				workspaceTarget = "none"
			}
			s.cfg.Logf(
				"lid closed: forced internal outputs off (%s), workspace target=%s retargeted=%d",
				disabled,
				workspaceTarget,
				adjustment.RetargetedWorkspaces,
			)
		}
	}

	applyKey := target.Name + "|" + hash + "|lid=" + string(s.lidState)
	if applyKey == s.applied {
		return nil
	}
	if manualHold {
		s.cfg.Logf("restoring manually selected profile %q after an external change", target.Name)
	}

	if _, err := s.engine.Apply(ctx, effective, monitors); err != nil {
		return err
	}

	appliedHash := hash
	appliedMonitors, err := s.client.Monitors(ctx)
	if err != nil {
		s.cfg.Logf("refresh monitors after apply failed: %v", err)
	} else {
		appliedHash = profile.MonitorStateHash(appliedMonitors)
	}

	s.applied = target.Name + "|" + appliedHash + "|lid=" + string(s.lidState)
	s.lastSeenHash = appliedHash
	s.cfg.Logf("applied profile: %s", target.Name)
	s.signalChange()
	return nil
}

// absorbOmarchyDisplayScale keeps a scale the Omarchy Display panel just
// set. That panel talks to Hyprland directly, so without this the next
// poll would put the saved profile back and undo the click. Clamshell
// resets do not write Omarchy's scaling log, so they still get reverted.
func (s *Service) absorbOmarchyDisplayScale(target profile.Profile, monitors []hypr.Monitor, monitorSet string, manualHold bool) profile.Profile {
	if target.Name == "" {
		return target
	}
	if _, err := s.store.Load(target.Name); err != nil {
		return target
	}

	updated, changed := profile.AbsorbRequestedScales(target, monitors, omarchywatch.RecentRequestedScales(time.Now()))
	if len(changed) == 0 {
		return target
	}
	if err := s.store.Save(updated); err != nil {
		s.cfg.Logf("could not persist Omarchy display scale into %q: %v", target.Name, err)
		return target
	}
	if manualHold {
		s.setManualOverride(monitorSet, updated)
	}
	s.cfg.Logf("persisted Omarchy display scale for %s into profile %q", strings.Join(changed, ","), updated.Name)
	return updated
}

func (s *Service) SetNotifier(notify func()) {
	s.notifyMu.Lock()
	s.notify = notify
	s.notifyMu.Unlock()
}

func (s *Service) signalChange() {
	s.notifyMu.RLock()
	notify := s.notify
	s.notifyMu.RUnlock()
	if notify != nil {
		notify()
	}
}

func (s *Service) setManualOverride(monitorSet string, chosen profile.Profile) {
	s.manualMu.Lock()
	s.manualSet = monitorSet
	s.manualProfile = chosen
	s.manualMu.Unlock()
}

func (s *Service) clearManualOverride() {
	s.manualMu.Lock()
	s.manualSet = ""
	s.manualProfile = profile.Profile{}
	s.manualMu.Unlock()
}

// manualOverride returns the profile a person chose by hand. The choice holds
// until the monitor set changes, and it is a profile rather than a flag so the
// daemon can put it back when something else moves the displays.
func (s *Service) manualOverride(monitorSet string) (profile.Profile, bool) {
	s.manualMu.Lock()
	defer s.manualMu.Unlock()
	if s.manualSet == "" {
		return profile.Profile{}, false
	}
	if s.manualSet != monitorSet {
		s.manualSet = ""
		s.manualProfile = profile.Profile{}
		return profile.Profile{}, false
	}
	return s.manualProfile, true
}

func internalOnlyFallbackProfile(monitors []hypr.Monitor) (profile.Profile, bool) {
	if len(monitors) == 0 {
		return profile.Profile{}, false
	}

	internalIndex := -1
	for idx, monitor := range monitors {
		if !monitor.Disabled {
			return profile.Profile{}, false
		}
		if internalIndex < 0 && monitor.IsInternal() {
			internalIndex = idx
		}
	}
	if internalIndex < 0 {
		return profile.Profile{}, false
	}

	fallback := profile.FromMonitors("internal-fallback", monitors)
	internalKey := hypr.MonitorOutputKey(monitors[internalIndex], hypr.MonitorMatchCounts(monitors))
	for idx := range fallback.Outputs {
		fallback.Outputs[idx].Enabled = fallback.Outputs[idx].Key == internalKey
		fallback.Outputs[idx].MirrorOf = ""
		if fallback.Outputs[idx].Key != internalKey {
			continue
		}
		fallback.Outputs[idx].X = 0
		fallback.Outputs[idx].Y = 0
		if fallback.Outputs[idx].Scale <= 0 {
			fallback.Outputs[idx].Scale = 1
		}
	}
	fallback.Workspaces = profile.WorkspaceSettings{}
	fallback.Normalize()
	return fallback, true
}

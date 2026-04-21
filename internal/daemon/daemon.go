package daemon

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/crmne/hyprmoncfg/internal/apply"
	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/lid"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

type Config struct {
	Debounce        time.Duration
	PollInterval    time.Duration
	LidPollInterval time.Duration
	ForcedProfile   string
	MonitorsConf    string
	HyprConfig      string
	Logf            func(format string, args ...any)
}

type Service struct {
	client       *hypr.Client
	store        *profile.Store
	engine       apply.Engine
	cfg          Config
	applied      string
	lastSeenHash string
	lidState     lid.State
}

func New(client *hypr.Client, store *profile.Store, cfg Config) *Service {
	if cfg.Debounce <= 0 {
		cfg.Debounce = 1200 * time.Millisecond
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 5 * time.Second
	}
	if cfg.LidPollInterval <= 0 {
		cfg.LidPollInterval = lid.DefaultPollInterval
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
		cfg:      cfg,
		lidState: lid.Unknown,
	}
}

func (s *Service) Run(ctx context.Context) error {
	if s.client == nil || s.store == nil {
		return fmt.Errorf("daemon not initialized")
	}
	if err := s.store.Ensure(); err != nil {
		return err
	}

	triggerCh := make(chan string, 8)
	pushTrigger := func(reason string) {
		select {
		case triggerCh <- reason:
		default:
		}
	}

	pushTrigger("startup")

	var lidStates <-chan lid.State
	var lidErrs <-chan error
	if state, err := lid.ReadState(ctx); err != nil {
		s.cfg.Logf("lid events disabled: %v", err)
	} else {
		s.lidState = state
		s.cfg.Logf("lid state: %s", state)
		lidStates, lidErrs = lid.Watch(ctx, s.cfg.LidPollInterval)
	}

	events, eventErrs := s.client.SubscribeMonitorEvents(ctx)
	go func() {
		for ev := range events {
			pushTrigger(string(ev.Type) + ":" + ev.Value)
		}
	}()

	pollTicker := time.NewTicker(s.cfg.PollInterval)
	defer pollTicker.Stop()

	debounceTimer := time.NewTimer(s.cfg.Debounce)
	if !debounceTimer.Stop() {
		<-debounceTimer.C
	}

	pending := false

	for {
		select {
		case <-ctx.Done():
			return nil
		case err, ok := <-eventErrs:
			if !ok {
				eventErrs = nil
				continue
			}
			if err != nil {
				s.cfg.Logf("socket2 disabled: %v", err)
				eventErrs = nil
			}
		case state, ok := <-lidStates:
			if !ok {
				lidStates = nil
				continue
			}
			if state != s.lidState {
				s.lidState = state
				pushTrigger("lid:" + string(state))
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
			h := profile.MonitorStateHash(monitors)
			if h != s.lastSeenHash {
				s.lastSeenHash = h
				pushTrigger("poll-change")
			}
		case reason := <-triggerCh:
			s.cfg.Logf("triggered: %s", reason)
			pending = true
			if !debounceTimer.Stop() {
				select {
				case <-debounceTimer.C:
				default:
				}
			}
			debounceTimer.Reset(s.cfg.Debounce)
		case <-debounceTimer.C:
			if !pending {
				continue
			}
			pending = false
			if err := s.applyBest(ctx); err != nil {
				s.cfg.Logf("apply failed: %v", err)
			}
		}
	}
}

func (s *Service) applyBest(ctx context.Context) error {
	monitors, err := s.client.Monitors(ctx)
	if err != nil {
		return err
	}
	if len(monitors) == 0 {
		return nil
	}

	hash := profile.MonitorStateHash(monitors)

	var target profile.Profile
	if s.cfg.ForcedProfile != "" {
		target, err = s.store.Load(s.cfg.ForcedProfile)
		if err != nil {
			return fmt.Errorf("forced profile %q not found: %w", s.cfg.ForcedProfile, err)
		}
	} else {
		profiles, err := s.store.List()
		if err != nil {
			return err
		}
		best, score, ok := profile.BestMatch(profiles, monitors)
		if !ok {
			s.cfg.Logf("no matching profile for monitor set %s", hash)
			return nil
		}
		if s.lidState.Known() {
			s.cfg.Logf("best profile %q score=%d lid=%s", best.Name, score, s.lidState)
		} else {
			s.cfg.Logf("best profile %q score=%d", best.Name, score)
		}
		target = best
	}

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
	return nil
}

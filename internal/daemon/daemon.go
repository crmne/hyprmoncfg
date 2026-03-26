package daemon

import (
	"context"
	"fmt"
	"time"

	"github.com/crmne/hyprmoncfg/internal/apply"
	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

type Config struct {
	Debounce      time.Duration
	PollInterval  time.Duration
	ForcedProfile string
	MonitorsConf  string
	HyprConfig    string
	Logf          func(format string, args ...any)
}

type Service struct {
	client  *hypr.Client
	store   *profile.Store
	engine  apply.Engine
	cfg     Config
	applied string
}

func New(client *hypr.Client, store *profile.Store, cfg Config) *Service {
	if cfg.Debounce <= 0 {
		cfg.Debounce = 1200 * time.Millisecond
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 5 * time.Second
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
		},
		cfg: cfg,
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
	lastSeenHash := ""

	for {
		select {
		case <-ctx.Done():
			return nil
		case err, ok := <-eventErrs:
			if ok && err != nil {
				s.cfg.Logf("socket2 disabled: %v", err)
				eventErrs = nil
			}
		case <-pollTicker.C:
			monitors, err := s.client.Monitors(ctx)
			if err != nil {
				s.cfg.Logf("poll monitors failed: %v", err)
				continue
			}
			h := profile.MonitorStateHash(monitors)
			if h != lastSeenHash {
				lastSeenHash = h
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
		s.cfg.Logf("best profile %q score=%d", best.Name, score)
		target = best
	}

	applyKey := target.Name + "|" + hash
	if applyKey == s.applied {
		return nil
	}

	if _, err := s.engine.Apply(ctx, target, monitors); err != nil {
		return err
	}

	appliedHash := hash
	appliedMonitors, err := s.client.Monitors(ctx)
	if err != nil {
		s.cfg.Logf("refresh monitors after apply failed: %v", err)
	} else {
		appliedHash = profile.MonitorStateHash(appliedMonitors)
	}

	s.applied = target.Name + "|" + appliedHash
	s.cfg.Logf("applied profile: %s", target.Name)
	return nil
}

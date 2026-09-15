package daemon

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/ipc"
)

func (s *Service) queryMonitors(ctx context.Context) ([]hypr.Monitor, error) {
	queryCtx, cancel := context.WithTimeout(ctx, s.cfg.QueryTimeout)
	defer cancel()
	started := time.Now()
	monitors, err := s.client.Monitors(queryCtx)
	if errors.Is(queryCtx.Err(), context.DeadlineExceeded) {
		err = fmt.Errorf("%w (monitors)", ipc.ErrCompositorBusy)
	}
	s.logQuery("monitors", started, err)
	return monitors, err
}

func (s *Service) queryWorkspaceRules(ctx context.Context) ([]hypr.WorkspaceRule, error) {
	queryCtx, cancel := context.WithTimeout(ctx, s.cfg.QueryTimeout)
	defer cancel()
	started := time.Now()
	rules, err := s.client.WorkspaceRules(queryCtx)
	if errors.Is(queryCtx.Err(), context.DeadlineExceeded) {
		err = fmt.Errorf("%w (workspace rules)", ipc.ErrCompositorBusy)
	}
	s.logQuery("workspace-rules", started, err)
	return rules, err
}

func (s *Service) logQuery(operation string, started time.Time, err error) {
	elapsed := time.Since(started)
	if err != nil || elapsed >= 100*time.Millisecond {
		s.cfg.Logf("compositor query operation=%s elapsed=%s error=%v", operation, elapsed.Round(time.Millisecond), err)
	}
}

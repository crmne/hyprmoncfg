package hypr

import (
	"context"
	"fmt"
	"slices"
)

// DRM ioctls cannot be canceled by a Go context. Share one slot across clients
// so a stalled driver leaves at most one worker behind, rather than one per
// timed-out status/editor request. No result is cached across monitor snapshots.
var sharedMonitorConnectorEnricher = newMonitorConnectorEnricher(enrichMonitorConnectorPaths)

type monitorConnectorEnricher struct {
	slot  chan struct{}
	probe func([]Monitor)
}

func newMonitorConnectorEnricher(probe func([]Monitor)) *monitorConnectorEnricher {
	return &monitorConnectorEnricher{slot: make(chan struct{}, 1), probe: probe}
}

func (e *monitorConnectorEnricher) enrich(ctx context.Context, monitors []Monitor) ([]Monitor, error) {
	if err := ctx.Err(); err != nil {
		return nil, connectorEnrichmentError(err)
	}
	// Only ambiguous hardware identities use ConnectorPath in MonitorOutputKey.
	// Avoid touching DRM at all for ordinary distinct-serial monitor sets, even
	// when a previous ambiguous setup has left the shared worker blocked.
	counts := MonitorMatchCounts(monitors)
	var indices []int
	for i, monitor := range monitors {
		if key := monitor.HardwareKey(); key == "" || counts[key] > 1 {
			indices = append(indices, i)
		}
	}
	if len(indices) == 0 {
		if err := ctx.Err(); err != nil {
			return nil, connectorEnrichmentError(err)
		}
		return monitors, nil
	}
	select {
	case e.slot <- struct{}{}:
	case <-ctx.Done():
		return nil, connectorEnrichmentError(ctx.Err())
	}
	// A ready slot and an expired context can both win select. Never start fresh
	// kernel work for a caller that has already given up.
	if err := ctx.Err(); err != nil {
		<-e.slot
		return nil, connectorEnrichmentError(err)
	}
	snapshot := slices.Clone(monitors)
	for i := range snapshot {
		snapshot[i].AvailableModes = slices.Clone(snapshot[i].AvailableModes)
	}
	ambiguous := make([]Monitor, len(indices))
	for i, index := range indices {
		ambiguous[i] = snapshot[index]
	}
	done := make(chan []Monitor, 1)
	go func() {
		defer func() { <-e.slot }()
		e.probe(ambiguous)
		for i, index := range indices {
			snapshot[index].ConnectorPath = ambiguous[i].ConnectorPath
		}
		// Buffered delivery lets an abandoned worker finish and free the gate.
		done <- snapshot
	}()
	select {
	case <-ctx.Done():
		return nil, connectorEnrichmentError(ctx.Err())
	case result := <-done:
		if err := ctx.Err(); err != nil {
			return nil, connectorEnrichmentError(err)
		}
		return result, nil
	}
}

func connectorEnrichmentError(err error) error {
	return fmt.Errorf("failed to query monitor connector identities: %w", err)
}

package server

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/event"
	fwfmp "codeburg.org/lexbit/relurpify/relurpnet/fmp"
)

// ReconciliationScanner periodically scans expired FMP leases and orphans unresolved attempts.
type ReconciliationScanner struct {
	Service  *fwfmp.Service
	Interval time.Duration
	Log      event.Log

	mu     sync.Mutex
	done   chan struct{}
	ticker *time.Ticker
}

// Start launches the reconciliation loop. Safe to call multiple times.
func (s *ReconciliationScanner) Start(ctx context.Context) {
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.done != nil {
		s.mu.Unlock()
		return
	}
	s.done = make(chan struct{})
	if s.Interval <= 0 {
		s.Interval = 2 * time.Minute
	}
	s.mu.Unlock()

	go s.loop(ctx)
}

// Stop terminates the reconciliation loop.
func (s *ReconciliationScanner) Stop() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done != nil {
		close(s.done)
		s.done = nil
	}
	if s.ticker != nil {
		s.ticker.Stop()
	}
}

func (s *ReconciliationScanner) loop(ctx context.Context) {
	s.mu.Lock()
	ticker := time.NewTicker(s.Interval)
	s.ticker = ticker
	s.mu.Unlock()
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.done:
			return
		case <-ticker.C:
			result, err := s.Service.ReconcileExpiredLeases(ctx)
			if err != nil {
				// Reconciliation error; log is handled within ReconcileExpiredLeases via service.emit
				_ = err // Silence lint
			} else if result.Scanned > 0 || result.Errors > 0 {
				// Reconciliation ran; events are emitted by the service during orphaning
				// Log summary if there were changes
				if s.Log != nil && (result.Orphaned > 0 || result.Errors > 0) {
					payload, _ := json.Marshal(map[string]interface{}{
						"scanned":  result.Scanned,
						"orphaned": result.Orphaned,
						"skipped":  result.Skipped,
						"errors":   result.Errors,
					})
					_, _ = s.Log.Append(ctx, s.Service.Partition, []core.FrameworkEvent{{
						Type:      "fmp.reconciliation.complete.v1",
						Timestamp: time.Now().UTC(),
						Payload:   payload,
					}})
				}
			}
		}
	}
}

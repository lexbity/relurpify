package server

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/event"
	fwfmp "github.com/lexcodex/relurpify/framework/middleware/fmp"
)

// GCScanner periodically enforces TTL-based garbage collection on FMP advertisements and context objects.
// It maintains two independent scan intervals: one for discovery TTL enforcement and one for context object GC.
type GCScanner struct {
	Service         *fwfmp.Service
	DiscoveryExpiry time.Duration
	ContextGCExpiry time.Duration
	Log             event.Log

	mu              sync.Mutex
	done            chan struct{}
	discoveryTicker *time.Ticker
	contextTicker   *time.Ticker
}

// Start launches the GC loop. Safe to call multiple times.
func (s *GCScanner) Start(ctx context.Context) {
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.done != nil {
		s.mu.Unlock()
		return
	}
	s.done = make(chan struct{})
	if s.DiscoveryExpiry <= 0 {
		s.DiscoveryExpiry = 5 * time.Minute
	}
	if s.ContextGCExpiry <= 0 {
		s.ContextGCExpiry = 15 * time.Minute
	}
	s.mu.Unlock()

	go s.loop(ctx)
}

// Stop terminates the GC loop.
func (s *GCScanner) Stop() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done != nil {
		close(s.done)
		s.done = nil
	}
	if s.discoveryTicker != nil {
		s.discoveryTicker.Stop()
	}
	if s.contextTicker != nil {
		s.contextTicker.Stop()
	}
}

func (s *GCScanner) loop(ctx context.Context) {
	s.mu.Lock()
	discoveryTicker := time.NewTicker(s.DiscoveryExpiry)
	contextTicker := time.NewTicker(s.ContextGCExpiry)
	s.discoveryTicker = discoveryTicker
	s.contextTicker = contextTicker
	s.mu.Unlock()
	defer discoveryTicker.Stop()
	defer contextTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.done:
			return
		case <-discoveryTicker.C:
			// Run discovery TTL enforcement
			now := time.Now().UTC()
			if err := s.Service.Discovery.DeleteExpired(ctx, now); err != nil {
				// Log GC error
				if s.Log != nil {
					payload, _ := json.Marshal(map[string]interface{}{
						"type":  "discovery_expiry",
						"error": err.Error(),
					})
					_, _ = s.Log.Append(ctx, s.Service.Partition, []core.FrameworkEvent{{
						Type:      "fmp.gc.error.v1",
						Timestamp: now,
						Payload:   payload,
					}})
				}
			} else {
				// Log successful GC run
				if s.Log != nil {
					payload, _ := json.Marshal(map[string]interface{}{
						"type": "discovery_expiry",
					})
					_, _ = s.Log.Append(ctx, s.Service.Partition, []core.FrameworkEvent{{
						Type:      "fmp.gc.complete.v1",
						Timestamp: now,
						Payload:   payload,
					}})
				}
			}

		case <-contextTicker.C:
			// Run context object GC
			now := time.Now().UTC()
			result, err := s.Service.GCContextObjects(ctx)
			if err != nil {
				// Log GC error
				if s.Log != nil {
					payload, _ := json.Marshal(map[string]interface{}{
						"type":            "context_gc",
						"scanned_objects": result.ScannedObjects,
						"deleted_objects": result.DeletedObjects,
						"error":           err.Error(),
					})
					_, _ = s.Log.Append(ctx, s.Service.Partition, []core.FrameworkEvent{{
						Type:      "fmp.gc.error.v1",
						Timestamp: now,
						Payload:   payload,
					}})
				}
			} else if result.ScannedObjects > 0 || result.Errors > 0 {
				// Log successful GC run
				if s.Log != nil {
					payload, _ := json.Marshal(map[string]interface{}{
						"type":            "context_gc",
						"scanned_objects": result.ScannedObjects,
						"deleted_objects": result.DeletedObjects,
					})
					_, _ = s.Log.Append(ctx, s.Service.Partition, []core.FrameworkEvent{{
						Type:      "fmp.gc.complete.v1",
						Timestamp: now,
						Payload:   payload,
					}})
				}
			}
		}
	}
}

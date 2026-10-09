package knowledge

import (
	"context"
	"fmt"
	"sync"
)

// KnowledgeHealth aggregates knowledge-domain health signals into one
// operator-visible condition. It subscribes to the composition bus and latches
// degraded state when invalidation or the store reports trouble. The condition
// is intentionally latched: a recovered subsystem is observable through its own
// telemetry, but the named degraded condition persists until the process (and
// therefore this aggregator) restarts, so it can never be silently forgotten.
type KnowledgeHealth struct {
	mu       sync.Mutex
	degraded bool
	reason   string
	cancel   context.CancelFunc
	unsub    func()
	wg       sync.WaitGroup
}

// NewKnowledgeHealth subscribes to a bus and begins aggregating health events.
// A nil bus yields a valid, permanently-healthy aggregator.
func NewKnowledgeHealth(bus *EventBus) *KnowledgeHealth {
	h := &KnowledgeHealth{}
	if bus == nil {
		return h
	}
	ctx, cancel := context.WithCancel(context.Background())
	events, unsub := bus.Subscribe(64)
	h.mu.Lock()
	h.cancel = cancel
	h.unsub = unsub
	h.mu.Unlock()
	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-events:
				if !ok {
					return
				}
				h.observe(event)
			}
		}
	}()
	return h
}

// observe folds one knowledge event into the health condition.
func (h *KnowledgeHealth) observe(event Event) {
	switch event.Kind {
	case EventInvalidationDegraded:
		payload, ok := event.Payload.(InvalidationDegradedPayload)
		if !ok {
			return
		}
		h.mark(fmt.Sprintf("invalidation degraded after %d consecutive failures: %s", payload.FailureCount, payload.Error))
	case EventStoreDegraded:
		payload, ok := event.Payload.(StoreDegradedPayload)
		if !ok {
			return
		}
		h.mark("store degraded: " + payload.Error)
	}
}

func (h *KnowledgeHealth) mark(reason string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.degraded = true
	h.reason = reason
}

// Degraded reports the latched knowledge-health condition and its reason.
func (h *KnowledgeHealth) Degraded() (bool, string) {
	if h == nil {
		return false, ""
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.degraded, h.reason
}

// Close unsubscribes and stops the aggregation goroutine. Safe to call more
// than once and on a nil receiver.
func (h *KnowledgeHealth) Close() {
	if h == nil {
		return
	}
	h.mu.Lock()
	cancel := h.cancel
	unsub := h.unsub
	h.cancel = nil
	h.unsub = nil
	h.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if unsub != nil {
		unsub()
	}
	h.wg.Wait()
}

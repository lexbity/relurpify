package contextbudget

import (
	"context"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/platform/contracts"
)

type snapshotTelemetry interface {
	Emit(event contracts.Event)
}

// SnapshotEmitter emits budget snapshots to telemetry at a fixed call interval.
type SnapshotEmitter struct {
	mu               sync.Mutex
	advisor          *ContextBudgetAdvisor
	telemetry        snapshotTelemetry
	interval         int
	lastEmittedCalls int
}

// NewSnapshotEmitter creates a snapshot emitter with the provided interval.
func NewSnapshotEmitter(advisor *ContextBudgetAdvisor, telemetry snapshotTelemetry, interval int) *SnapshotEmitter {
	if interval <= 0 {
		interval = 10
	}
	return &SnapshotEmitter{
		advisor:   advisor,
		telemetry: telemetry,
		interval:  interval,
	}
}

// WithSnapshotEmitter stores the emitter in the context via the
// contracts.SnapshotObserver key so platform/llm can retrieve it without
// importing framework packages.
func WithSnapshotEmitter(ctx context.Context, emitter *SnapshotEmitter) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return contracts.WithSnapshotObserver(ctx, emitter)
}

// SnapshotEmitterFromContext extracts the emitter from the contracts.SnapshotObserver key.
func SnapshotEmitterFromContext(ctx context.Context) *SnapshotEmitter {
	if ctx == nil {
		return nil
	}
	obs := contracts.SnapshotObserverFromContext(ctx)
	emitter, _ := obs.(*SnapshotEmitter)
	return emitter
}

// Observe records the latest call count and emits a snapshot when the interval is reached.
func (e *SnapshotEmitter) Observe() {
	if e == nil || e.advisor == nil || e.telemetry == nil {
		return
	}
	snapshot := e.advisor.Snapshot()
	e.mu.Lock()
	defer e.mu.Unlock()
	if snapshot.CallCount-e.lastEmittedCalls < e.interval {
		return
	}
	e.lastEmittedCalls = snapshot.CallCount
	e.telemetry.Emit(contracts.Event{
		Type:      contracts.EventBudgetSnapshot,
		Timestamp: time.Now().UTC(),
		Message:   "budget snapshot",
		Metadata: map[string]any{
			"snapshot": snapshot,
		},
	})
}

// Reset clears the emitter's emission cursor.
func (e *SnapshotEmitter) Reset() {
	if e == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.lastEmittedCalls = 0
}

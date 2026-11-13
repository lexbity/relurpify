// Package model provides the single physical model request lane enforcement.
package model

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// gateActiveKey marks a context that already holds an active gate ticket.
// A nested acquisition on such a context is a programming error (D-04).
type gateActiveKey struct{}

// CallID uniquely identifies a logical model call across attempts.
type CallID string

// AttemptID uniquely identifies a physical attempt within a call.
type AttemptID string

// GateState is an immutable snapshot of the gate for observability.
type GateState struct {
	// Queued is the number of calls waiting for the lane.
	Queued int
	// Active is true if a physical request is currently admitted.
	Active bool
	// Poisoned is true if the lane has been poisoned and requires recovery.
	Poisoned bool
	// ActiveCallID is the call ID of the currently active request, if any.
	ActiveCallID CallID
	// ActiveAttemptID is the attempt ID of the currently active request, if any.
	ActiveAttemptID AttemptID
	// QueuePosition is the position of the next call in the queue (0 if active).
	QueuePosition int
}

// Ticket is a granted lease on the model lane. It MUST be released exactly
// once after the physical request and its response body/stream are reclaimed.
type Ticket struct {
	// CallID is the logical call identifier.
	CallID CallID
	// AttemptID is the physical attempt identifier.
	AttemptID AttemptID
	// Released signals that the physical request is complete.
	Released func() GateState
	// Poison signals that the request could not be canceled and the lane is poisoned.
	Poison func(reason string) GateState
	// Context is the ticket-scoped context. It carries the active-call marker
	// and the active timeout, and MUST be used for the physical request.
	Context context.Context
}

// GateConfig configures the model gate behavior.
type GateConfig struct {
	// QueueCapacity is the maximum number of queued calls (default 128).
	QueueCapacity int
	// QueueTimeout is the maximum time a call can wait in the queue (default 300s).
	QueueTimeout time.Duration
	// ActiveTimeout is the maximum time a physical request can be active (default 900s).
	ActiveTimeout time.Duration
	// CancellationGracePeriod is the time to wait for graceful cancellation (default 250ms).
	CancellationGracePeriod time.Duration
}

// DefaultGateConfig returns the default gate configuration per D-04.
func DefaultGateConfig() GateConfig {
	return GateConfig{
		QueueCapacity:           128,
		QueueTimeout:            300 * time.Second,
		ActiveTimeout:           900 * time.Second,
		CancellationGracePeriod: 250 * time.Millisecond,
	}
}

// CallGate enforces the one physical model request constraint per D-04.
// It uses a lock-protected FIFO queue with ticket-based admission. Granted
// tickets are never handed to a second waiter until physical reclamation is
// acknowledged by Released.
type CallGate struct {
	config GateConfig

	mu sync.Mutex

	active *activeCall
	queue  []*queuedCall

	poisoned     bool
	poisonReason string
	closed       bool

	statechange chan struct{}

	queueDepth     atomic.Int64
	activeCount    atomic.Int64
	poisonCount    atomic.Int64
	totalGranted   atomic.Int64
	totalReclaimed atomic.Int64
}

type activeCall struct {
	callID    CallID
	attemptID AttemptID
	cancel    context.CancelFunc
	timer     *time.Timer
}

type queuedCall struct {
	callID    CallID
	attemptID AttemptID
	ctx       context.Context
	result    chan acquireResult
}

type acquireResult struct {
	ticket *Ticket
	err    error
}

// NewCallGate creates a new call gate with the given configuration.
func NewCallGate(config GateConfig) *CallGate {
	defaults := DefaultGateConfig()
	if config.QueueCapacity <= 0 {
		config.QueueCapacity = defaults.QueueCapacity
	}
	if config.QueueTimeout <= 0 {
		config.QueueTimeout = defaults.QueueTimeout
	}
	if config.ActiveTimeout <= 0 {
		config.ActiveTimeout = defaults.ActiveTimeout
	}
	if config.CancellationGracePeriod <= 0 {
		config.CancellationGracePeriod = defaults.CancellationGracePeriod
	}

	return &CallGate{
		config:      config,
		statechange: make(chan struct{}, 1),
	}
}

// NewDefaultCallGate creates a new call gate with default configuration.
func NewDefaultCallGate() *CallGate {
	return NewCallGate(DefaultGateConfig())
}

// Acquire attempts to acquire a ticket for a physical model request.
// It returns ErrNestedModelCall for a reentrant call, ErrGatePoisoned for a
// poisoned lane, ErrQueueSaturated when the queue is full, ErrQueueTimeout when
// the queue deadline expires, or the caller's context error when canceled.
//
// The returned ticket MUST be released exactly once, even if the request fails.
func (g *CallGate) Acquire(ctx context.Context, callID CallID, attemptID AttemptID) (*Ticket, error) {
	if ctx.Value(gateActiveKey{}) != nil {
		return nil, ErrNestedModelCall
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if callID == "" {
		callID = GenerateCallID()
	}
	if attemptID == "" {
		attemptID = GenerateAttemptID()
	}

	g.mu.Lock()
	if g.closed {
		g.mu.Unlock()
		return nil, ErrGateClosed
	}
	if g.poisoned {
		g.mu.Unlock()
		return nil, ErrGatePoisoned
	}
	if g.active == nil {
		ticket := g.grantLocked(ctx, callID, attemptID)
		g.mu.Unlock()
		g.notify()
		return ticket, nil
	}
	if len(g.queue) >= g.config.QueueCapacity {
		g.mu.Unlock()
		return nil, ErrQueueSaturated
	}
	queued := &queuedCall{
		callID:    callID,
		attemptID: attemptID,
		ctx:       ctx,
		result:    make(chan acquireResult, 1),
	}
	g.queue = append(g.queue, queued)
	g.queueDepth.Store(int64(len(g.queue)))
	g.mu.Unlock()
	g.notify()

	queueTimer := time.NewTimer(g.config.QueueTimeout)
	defer queueTimer.Stop()

	select {
	case result := <-queued.result:
		if result.err != nil {
			return nil, result.err
		}
		return result.ticket, nil
	case <-ctx.Done():
		return g.abandonQueued(queued, ctx.Err())
	case <-queueTimer.C:
		return g.abandonQueued(queued, ErrQueueTimeout)
	}
}

// abandonQueued removes a waiting call from the FIFO. If the gate granted it
// concurrently, the granted ticket is released instead of leaked.
func (g *CallGate) abandonQueued(queued *queuedCall, cause error) (*Ticket, error) {
	if g.removeQueued(queued) {
		return nil, cause
	}
	result := <-queued.result
	if result.ticket != nil {
		result.ticket.Released()
	}
	if result.err != nil {
		return nil, result.err
	}
	return nil, cause
}

// removeQueued removes queued from the FIFO. It reports whether the entry was
// still queued; false means the gate already granted it.
func (g *CallGate) removeQueued(queued *queuedCall) bool {
	g.mu.Lock()
	for i, item := range g.queue {
		if item != queued {
			continue
		}
		g.queue = append(g.queue[:i], g.queue[i+1:]...)
		g.queueDepth.Store(int64(len(g.queue)))
		g.mu.Unlock()
		g.notify()
		return true
	}
	g.mu.Unlock()
	return false
}

// grantLocked admits a physical request. The caller must hold g.mu.
func (g *CallGate) grantLocked(parent context.Context, callID CallID, attemptID AttemptID) *Ticket {
	marked := context.WithValue(parent, gateActiveKey{}, struct{}{})
	activeCtx, cancel := context.WithTimeout(marked, g.config.ActiveTimeout)
	active := &activeCall{callID: callID, attemptID: attemptID, cancel: cancel}
	active.timer = time.AfterFunc(g.config.ActiveTimeout, func() {
		g.activeTimeout(callID, attemptID)
	})
	g.active = active
	g.activeCount.Store(1)
	g.totalGranted.Add(1)

	return &Ticket{
		CallID:    callID,
		AttemptID: attemptID,
		Context:   activeCtx,
		Released: func() GateState {
			return g.release(callID, attemptID)
		},
		Poison: func(reason string) GateState {
			return g.poisonTicket(callID, attemptID, reason)
		},
	}
}

// activeTimeout poisons the lane when an active request is not reclaimed
// within the cancellation grace period after its deadline.
func (g *CallGate) activeTimeout(callID CallID, attemptID AttemptID) {
	time.AfterFunc(g.config.CancellationGracePeriod, func() {
		g.mu.Lock()
		defer g.mu.Unlock()
		if g.active == nil || g.active.callID != callID || g.active.attemptID != attemptID {
			return
		}
		g.poisonLocked("active request not reclaimed after deadline")
	})
}

// release reclaims the active ticket and grants the next queued call.
func (g *CallGate) release(callID CallID, attemptID AttemptID) GateState {
	g.mu.Lock()
	if g.active == nil || g.active.callID != callID || g.active.attemptID != attemptID {
		state := g.stateLocked()
		g.mu.Unlock()
		return state
	}
	g.active.timer.Stop()
	g.active.cancel()
	g.active = nil
	g.activeCount.Store(0)
	g.totalReclaimed.Add(1)

	for len(g.queue) > 0 {
		next := g.queue[0]
		g.queue = g.queue[1:]
		if err := next.ctx.Err(); err != nil {
			next.result <- acquireResult{err: err}
			continue
		}
		next.result <- acquireResult{ticket: g.grantLocked(next.ctx, next.callID, next.attemptID)}
		break
	}
	g.queueDepth.Store(int64(len(g.queue)))
	state := g.stateLocked()
	g.mu.Unlock()
	g.notify()
	return state
}

// poisonTicket poisons the lane if the ticket is still the active one.
func (g *CallGate) poisonTicket(callID CallID, attemptID AttemptID, reason string) GateState {
	g.mu.Lock()
	if g.active == nil || g.active.callID != callID || g.active.attemptID != attemptID {
		state := g.stateLocked()
		g.mu.Unlock()
		return state
	}
	state := g.poisonLocked(reason)
	g.mu.Unlock()
	g.notify()
	return state
}

// poisonLocked poisons the lane and rejects every queued call. The caller must
// hold g.mu.
func (g *CallGate) poisonLocked(reason string) GateState {
	g.poisoned = true
	g.poisonReason = reason
	g.poisonCount.Add(1)

	if g.active != nil {
		g.active.timer.Stop()
		g.active.cancel()
		g.active = nil
	}
	g.activeCount.Store(0)

	for _, queued := range g.queue {
		queued.result <- acquireResult{err: ErrGatePoisoned}
	}
	g.queue = nil
	g.queueDepth.Store(0)

	return g.stateLocked()
}

// Poison poisons the gate with the given reason. It is called when a provider
// cannot be canceled within the grace period.
func (g *CallGate) Poison(reason string) GateState {
	g.mu.Lock()
	state := g.poisonLocked(reason)
	g.mu.Unlock()
	g.notify()
	return state
}

// Recover clears the poisoned state and allows new requests. It must only be
// called after the problematic backend has been recovered.
func (g *CallGate) Recover() GateState {
	g.mu.Lock()
	if g.poisoned {
		g.poisoned = false
		g.poisonReason = ""
	}
	state := g.stateLocked()
	g.mu.Unlock()
	g.notify()
	return state
}

// State returns an immutable snapshot of the gate.
func (g *CallGate) State() GateState {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.stateLocked()
}

func (g *CallGate) stateLocked() GateState {
	state := GateState{
		Queued:   len(g.queue),
		Active:   g.active != nil,
		Poisoned: g.poisoned,
	}
	if g.active != nil {
		state.ActiveCallID = g.active.callID
		state.ActiveAttemptID = g.active.attemptID
	}
	return state
}

// Close closes the gate and rejects all future requests.
func (g *CallGate) Close() {
	g.mu.Lock()
	g.closed = true

	if g.active != nil {
		g.active.timer.Stop()
		g.active.cancel()
		g.active = nil
	}
	g.activeCount.Store(0)

	for _, queued := range g.queue {
		queued.result <- acquireResult{err: ErrGateClosed}
	}
	g.queue = nil
	g.queueDepth.Store(0)
	g.mu.Unlock()
	g.notify()
}

// notify wakes state observers without blocking the caller.
func (g *CallGate) notify() {
	select {
	case g.statechange <- struct{}{}:
	default:
	}
}

// SubscribeState returns a channel that receives coalesced notifications when
// the gate state changes. Use State for the current value.
func (g *CallGate) SubscribeState() <-chan struct{} {
	return g.statechange
}

// QueueDepth returns the current queue depth.
func (g *CallGate) QueueDepth() int64 {
	return g.queueDepth.Load()
}

// ActiveCount returns 1 while a physical request is active, 0 otherwise.
func (g *CallGate) ActiveCount() int64 {
	return g.activeCount.Load()
}

// PoisonCount returns the number of poison events.
func (g *CallGate) PoisonCount() int64 {
	return g.poisonCount.Load()
}

// TotalGranted returns the total number of tickets granted.
func (g *CallGate) TotalGranted() int64 {
	return g.totalGranted.Load()
}

// TotalReclaimed returns the total number of tickets reclaimed.
func (g *CallGate) TotalReclaimed() int64 {
	return g.totalReclaimed.Load()
}

// String returns a human-readable representation of the gate state.
func (g *CallGate) String() string {
	state := g.State()
	return fmt.Sprintf("CallGate{queued=%d, active=%v, poisoned=%v, activeCall=%s}",
		state.Queued, state.Active, state.Poisoned, state.ActiveCallID)
}

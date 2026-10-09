package runtime

import (
	"context"
	"errors"
	"sync"
	"time"

	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// ErrRuntimeShuttingDown is returned when a run is submitted after Close has
// begun: the coordinator's intake has closed and a new run may not start.
var ErrRuntimeShuttingDown = errors.New("runtime is shutting down")

const (
	// ShutdownDrainDefault bounds how long Close waits for registered runs to
	// finish on their own before cancelling them (D10).
	ShutdownDrainDefault = 10 * time.Second
	// ShutdownReapDefault bounds how long Close waits after cancellation for
	// runs that ignore it before abandoning them (D10).
	ShutdownReapDefault = 2 * time.Second
)

// RunDrainReport is the bounded quiesce outcome of one Runtime.Close: every
// run that was live at the barrier is classified completed, cancelled, or
// abandoned, and WaitedMS records the total drain wait.
type RunDrainReport struct {
	Completed []string
	Cancelled []string
	Abandoned []string
	WaitedMS  int64
}

func (r RunDrainReport) total() int {
	return len(r.Completed) + len(r.Cancelled) + len(r.Abandoned)
}

// runCoordinatorEntry is one registered in-flight run.
type runCoordinatorEntry struct {
	ctx    context.Context
	cancel context.CancelFunc
}

// RunCoordinator is the unit of shutdown for a Runtime. It owns a cancellable
// parent context from which every live run derives its run-lifetime context
// (so Close can cancel them all at once), tracks runs by task ID, and provides
// the bounded drain → cancel → reap sequence Close performs. It is deliberately
// free of Runtime specifics so it can be tested in isolation.
//
// A run registered after Close has begun is refused: Close closes the intake
// before draining, so a new run can never begin tearing down stores underneath.
type RunCoordinator struct {
	mu           sync.Mutex
	closed       bool
	parent       context.Context
	cancelParent context.CancelFunc
	runs         map[string]*runCoordinatorEntry
	wake         chan struct{}

	drain time.Duration
	reap  time.Duration
	tel   telemetry.Telemetry
}

// newRunCoordinator builds a live coordinator. tel receives the shutdown
// accounting events; nil keeps the coordinator silent.
func newRunCoordinator(tel telemetry.Telemetry) *RunCoordinator {
	parent, cancel := context.WithCancel(context.Background())
	return &RunCoordinator{
		parent:       parent,
		cancelParent: cancel,
		runs:         make(map[string]*runCoordinatorEntry),
		wake:         make(chan struct{}, 1),
		drain:        ShutdownDrainDefault,
		reap:         ShutdownReapDefault,
		tel:          tel,
	}
}

// SetDurations overrides the drain (grace) and reap (post-cancel) deadlines.
// Zero or negative keep the current default. Tests parameterize these to
// millisecond scale so shutdown behavior is asserted without real tens-of-
// seconds waits.
func (c *RunCoordinator) SetDurations(drain, reap time.Duration) *RunCoordinator {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	if drain > 0 {
		c.drain = drain
	}
	if reap >= 0 {
		c.reap = reap
	}
	c.mu.Unlock()
	return c
}

// Register a run under the coordinator-owned parent, keyed by task ID. It
// returns the run context, a once-safe done function, and whether
// registration succeeded. When the intake is closed (Close has begun), the
// returned context is already cancelled and ok is false — the caller must not
// start the run.
func (c *RunCoordinator) Register(taskID string) (context.Context, func(), bool) {
	if c == nil {
		ctx, cancel := context.WithCancel(context.Background())
		return ctx, cancel, true
	}
	c.mu.Lock()
	if c.closed {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		c.mu.Unlock()
		return ctx, func() {}, false
	}
	ctx, cancel := context.WithCancel(c.parent)
	entry := &runCoordinatorEntry{ctx: ctx, cancel: cancel}
	c.runs[taskID] = entry
	c.mu.Unlock()

	var once sync.Once
	done := func() {
		once.Do(func() {
			c.mu.Lock()
			if e, ok := c.runs[taskID]; ok {
				e.cancel()
				delete(c.runs, taskID)
			}
			c.signalLocked()
			c.mu.Unlock()
		})
	}
	return ctx, done, true
}

// Count reports the number of live registered runs.
func (c *RunCoordinator) Count() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.runs)
}

// DrainAndStop is the Close sequence: close the intake (no new registrations),
// wait up to the drain deadline for live runs to finish, cancel the shared
// parent, wait up to the reap deadline for runs to observe cancellation, and
// abandon the survivors with an observable report. It is idempotent: only the
// first call owns the teardown; later calls return the empty report.
func (c *RunCoordinator) DrainAndStop() RunDrainReport {
	if c == nil {
		return RunDrainReport{}
	}
	started := time.Now()
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return RunDrainReport{}
	}
	c.closed = true
	c.mu.Unlock()

	report := RunDrainReport{}
	initial := c.ids()

	// Phase 1 — bounded drain: healthy runs finish on their own.
	remaining := c.waitForFinished(c.drain)
	switch remaining {
	case nil:
		report.Completed = initial
		report.WaitedMS = time.Since(started).Milliseconds()
		c.emitDrain(report)
		return report
	default:
		report.Completed = diffIDs(initial, remaining)
	}

	// Phase 2 — cancel: runs still live are told to stop.
	c.mu.Lock()
	cancelParent := c.cancelParent
	c.mu.Unlock()
	cancelParent()

	// Phase 3 — bounded reap: observe cancellation, then abandon survivors.
	left := c.waitForFinished(c.reap)
	report.Cancelled = diffIDs(remaining, left)
	report.Abandoned = append([]string(nil), left...)
	report.WaitedMS = time.Since(started).Milliseconds()

	for _, taskID := range report.Abandoned {
		c.emitAbandoned(taskID, report.WaitedMS)
	}
	c.emitDrain(report)
	return report
}

// waitForFinished waits up to d for the live run set to empty. It returns the
// ids still registered when the deadline elapses, or nil when everything
// finished.
func (c *RunCoordinator) waitForFinished(d time.Duration) []string {
	deadline := time.Now().Add(d)
	for {
		ids := c.ids()
		if len(ids) == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return ids
		}
		select {
		case <-c.wake:
		case <-time.After(time.Until(deadline)):
		}
	}
}

func (c *RunCoordinator) ids() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	ids := make([]string, 0, len(c.runs))
	for id := range c.runs {
		ids = append(ids, id)
	}
	return ids
}

// signalLocked wakes a DrainAndStop waiter that is currently pending. Callers
// MUST hold c.mu.
func (c *RunCoordinator) signalLocked() {
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

func (c *RunCoordinator) emitDrain(report RunDrainReport) {
	if c == nil || c.tel == nil {
		return
	}
	c.tel.Emit(telemetry.Event{
		Type:      telemetry.EventShutdownDrain,
		Message:   "runtime shutdown drain",
		Timestamp: time.Now().UTC(),
		Metadata: map[string]any{
			"waited_ms": report.WaitedMS,
			"completed": len(report.Completed),
			"cancelled": len(report.Cancelled),
			"abandoned": len(report.Abandoned),
		},
	})
}

func (c *RunCoordinator) emitAbandoned(taskID string, waitedMS int64) {
	if c == nil || c.tel == nil {
		return
	}
	c.tel.Emit(telemetry.Event{
		Type:      telemetry.EventShutdownAbandoned,
		Message:   "run abandoned at shutdown",
		Timestamp: time.Now().UTC(),
		Metadata: map[string]any{
			"task_id":   taskID,
			"waited_ms": waitedMS,
		},
	})
}

// diffIDs returns the members of a missing from b, preserving a's order.
func diffIDs(a, b []string) []string {
	setB := make(map[string]struct{}, len(b))
	for _, id := range b {
		setB[id] = struct{}{}
	}
	out := make([]string, 0, len(a))
	for _, id := range a {
		if _, ok := setB[id]; !ok {
			out = append(out, id)
		}
	}
	return out
}

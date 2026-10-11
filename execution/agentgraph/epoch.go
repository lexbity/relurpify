package agentgraph

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/contextstream"
	"codeburg.org/lexbit/relurpify/context/knowledge"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

const (
	// defaultStreamJobDeadline bounds how long a barrier awaits background
	// stream jobs before abandoning them (D6).
	defaultStreamJobDeadline = 2 * time.Second
	// defaultEpochDrain bounds the subscriber-side knowledge drain at a barrier.
	defaultEpochDrain = 50 * time.Millisecond
	// groundingRetryBackoff pauses between the first grounding attempt and its
	// single retry (D1).
	groundingRetryBackoff = 250 * time.Millisecond
)

// Grounder is the durable write boundary the epoch barrier flushes through.
// *knowledge.GroundingService satisfies it.
type Grounder interface {
	Ground(ctx context.Context, items []knowledge.GroundingItem) (knowledge.GroundingReport, error)
}

type epochCoordinatorContextKey struct{}

// WithEpochCoordinator attaches an epoch coordinator to a node context. The
// coordinator is also the run's capture sink, so it is stored under both keys.
func WithEpochCoordinator(ctx context.Context, coord *EpochCoordinator) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx = context.WithValue(ctx, epochCoordinatorContextKey{}, coord)
	return context.WithValue(ctx, captureSinkContextKey{}, coord)
}

// EpochCoordinatorFromContext extracts the epoch coordinator, if any, from a
// node context. Parallel branch executions share the parent's coordinator.
func EpochCoordinatorFromContext(ctx context.Context) *EpochCoordinator {
	if ctx == nil {
		return nil
	}
	coord, _ := ctx.Value(epochCoordinatorContextKey{}).(*EpochCoordinator)
	return coord
}

// EpochCoordinator owns the run's epoch lifecycle: capture grounding and
// background stream jobs land at epoch barriers, and nothing mutates a closed
// epoch. It implements CaptureSink so recipe capture execution enqueues into it.
//
// State machine (D2, normative). The coordinator uses the opening-epoch
// convention — the tree's established numbering, origin 1 — which is
// behaviorally identical to a last-closed counter initialized at 0:
//
//	c.epoch  = the epoch the next barrier will CLOSE (the opening epoch);
//	           the last closed epoch is c.epoch-1
//	env.AssemblyMetadata.EpochID mirrors c.epoch after each close
//
//	EnqueueCapture(item):   under c.mu:   item.Epoch = c.epoch + (1 if a flush
//	                                      is in flight else 0)
//	                          Items enqueued during an in-flight flush belong to
//	                          the NEXT barrier, never the one mid-flight.
//	TrackStreamJob(job):    under c.mu (unchanged registration)
//	CloseEpochIfPending:    barrierMu serializes whole barriers; under c.mu:
//	                        take items+jobs (atomic swap), epoch := c.epoch,
//	                        inFlush = true
//	                        outside all locks: flush grounding (one retry,
//	                        250 ms) → await jobs (2 s) → drain (50 ms)
//	                        under c.mu: inFlush = false; on success
//	                        c.epoch = epoch + 1 (a grounding failure aborts
//	                        before the close; an await failure closes and
//	                        then propagates — failure semantics unchanged)
//	FinalEpoch:             the barrier taken unconditionally, idempotent
//
// Concurrent barriers serialize on barrierMu: the second takes nothing
// already taken and cannot double-close. c.epoch is read and written only
// under c.mu — there is no unlocked read anywhere.
type EpochCoordinator struct {
	mu      sync.Mutex
	epoch   uint64
	inFlush bool
	items   []knowledge.GroundingItem
	jobs    []*contextstream.Job

	// barrierMu serializes whole close operations so two concurrent barriers
	// (parallel branches share the parent coordinator) cannot interleave
	// take/flush/close. c.mu is never held across flush work.
	barrierMu sync.Mutex

	runCtx    context.Context
	ground    Grounder
	tel       telemetry.Telemetry
	drain     func(time.Duration)
	finalized bool
	deadline  time.Duration
}

// NewEpochCoordinator creates the run-scoped coordinator. runCtx must carry
// the run envelope (contextdata.WithEnvelope) and outlive every node.
func NewEpochCoordinator(runCtx context.Context, ground Grounder, tel telemetry.Telemetry) *EpochCoordinator {
	return &EpochCoordinator{
		epoch:    1,
		runCtx:   runCtx,
		ground:   ground,
		tel:      tel,
		deadline: defaultStreamJobDeadline,
	}
}

// SetDrain installs the invalidation subscription's subscriber-side drain.
func (c *EpochCoordinator) SetDrain(drain func(time.Duration)) *EpochCoordinator {
	if c != nil {
		c.drain = drain
	}
	return c
}

// SetStreamJobDeadline overrides the background stream job deadline (tests).
func (c *EpochCoordinator) SetStreamJobDeadline(deadline time.Duration) *EpochCoordinator {
	if c != nil && deadline > 0 {
		c.deadline = deadline
	}
	return c
}

// RunContext returns the run-lifetime context background stream jobs run on.
func (c *EpochCoordinator) RunContext() context.Context {
	if c == nil {
		return nil
	}
	return c.runCtx
}

// EpochID returns the current (opening) epoch number: the epoch the next
// barrier will close. Guarded by c.mu like every other epoch read.
func (c *EpochCoordinator) EpochID() uint64 {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.epoch
}

// EnqueueCapture is the phase-1 capture hand-off: stamp the epoch that will
// carry the item and queue it for the next barrier. While a flush is in
// flight the stamp targets the FOLLOWING epoch (D2): items enqueued mid-flush
// belong to the next barrier, never the one being closed.
func (c *EpochCoordinator) EnqueueCapture(item knowledge.GroundingItem) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	item.Epoch = c.epoch
	if c.inFlush {
		item.Epoch = c.epoch + 1
	}
	c.items = append(c.items, item)
}

// TrackStreamJob registers a background stream job the next barrier awaits.
func (c *EpochCoordinator) TrackStreamJob(job *contextstream.Job) {
	if c == nil || job == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.jobs = append(c.jobs, job)
}

// Pending reports the queued capture count and in-flight job count.
func (c *EpochCoordinator) Pending() (int, int) {
	if c == nil {
		return 0, 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items), len(c.jobs)
}

// CloseEpochIfPending is the epoch barrier: a cheap no-op when the epoch is
// clean, otherwise flush → await → drain → close the epoch and open the next.
// Whole barriers serialize on barrierMu, so two concurrent barriers cannot
// double-flush or double-close (D2); each takes only what is pending when it
// holds the barrier lock.
func (c *EpochCoordinator) CloseEpochIfPending(nodeID string) error {
	if c == nil {
		return nil
	}
	c.barrierMu.Lock()
	defer c.barrierMu.Unlock()
	return c.barrier(nodeID, false)
}

// FinalEpoch always closes the run: it flushes whatever exists and stamps the
// last epoch. It is idempotent so the deferred safety net and the explicit call
// do not double-close.
func (c *EpochCoordinator) FinalEpoch(nodeID string) error {
	if c == nil {
		return nil
	}
	c.barrierMu.Lock()
	defer c.barrierMu.Unlock()
	return c.barrier(nodeID, true)
}

// barrier is the one close path, run under barrierMu. unconditional closes
// the run's final epoch even when the queues are empty; otherwise an empty
// epoch is a cheap no-op.
func (c *EpochCoordinator) barrier(nodeID string, unconditional bool) error {
	c.mu.Lock()
	if !unconditional && len(c.items) == 0 && len(c.jobs) == 0 {
		c.mu.Unlock()
		return nil
	}
	if c.finalized {
		c.mu.Unlock()
		return nil
	}
	if unconditional {
		c.finalized = true
	}
	items := c.items
	jobs := c.jobs
	c.items = nil
	c.jobs = nil
	epoch := c.epoch
	c.inFlush = true
	c.mu.Unlock()

	err := c.flushEpoch(nodeID, epoch, items, jobs)

	c.mu.Lock()
	c.inFlush = false
	if err == nil {
		c.epoch = epoch + 1
	}
	c.mu.Unlock()
	return err
}

func (c *EpochCoordinator) flushEpoch(nodeID string, epoch uint64, items []knowledge.GroundingItem, jobs []*contextstream.Job) error {
	started := time.Now()

	var groundErr error
	if len(items) > 0 {
		report, err := c.groundBatch(items)
		if err != nil {
			select {
			case <-time.After(groundingRetryBackoff):
			case <-c.runCtx.Done():
				return c.wrapGroundingError(nodeID, epoch, err)
			}
			if report, err = c.groundBatch(items); err != nil {
				return c.wrapGroundingError(nodeID, epoch, err)
			}
		}
		c.emitGrounded(nodeID, epoch, report)
	}
	// awaitJobs errors are barrier failures exactly like grounding errors;
	// the first one wins. Unlike a grounding failure (which aborts before
	// the close), an await failure still closes the epoch: the jobs' fate is
	// recorded, the memory order advances, and the error propagates.
	if err := c.awaitJobs(jobs, nodeID, epoch); err != nil {
		groundErr = err
	}
	if c.drain != nil {
		c.drain(defaultEpochDrain)
	}

	next := epoch + 1
	c.mu.Lock()
	c.epoch = next
	c.mu.Unlock()
	if env, ok := contextdata.EnvelopeFrom(c.runCtx); ok {
		env.UpdateAssemblyMetadata(func(meta contextdata.AssemblyMeta) contextdata.AssemblyMeta {
			meta.EpochID = next
			return meta
		})
	}
	c.emit(c.runCtx, telemetry.EventEpochClosed, "epoch closed", map[string]any{
		"node":       nodeID,
		"epoch":      epoch,
		"captures":   len(items),
		"jobs":       len(jobs),
		"flushed_ms": time.Since(started).Milliseconds(),
	})
	return groundErr
}

func (c *EpochCoordinator) groundBatch(items []knowledge.GroundingItem) (knowledge.GroundingReport, error) {
	if c.ground == nil {
		return knowledge.GroundingReport{}, knowledge.ErrGroundingFailed
	}
	return c.ground.Ground(c.runCtx, items)
}

func (c *EpochCoordinator) wrapGroundingError(nodeID string, epoch uint64, err error) error {
	c.emit(c.runCtx, telemetry.EventCaptureGroundFailed, "capture grounding failed", map[string]any{
		"node":  nodeID,
		"epoch": epoch,
		"err":   err.Error(),
	})
	return fmt.Errorf("epoch %d grounding: %w: %w", epoch, knowledge.ErrGroundingFailed, err)
}

// awaitJobs lands completed background stream jobs before the epoch closes.
// A job still running at the deadline is abandoned and its partial result is
// not applied. A stale-dropped apply (ErrStaleApplyDropped) is the designed
// drop, not a failure: the coordinator emits contextstream.stale_apply_dropped
// through its own sink — the barrier path's ctx carries no sink of its own.
func (c *EpochCoordinator) awaitJobs(jobs []*contextstream.Job, nodeID string, epoch uint64) error {
	if len(jobs) == 0 {
		return nil
	}
	deadline := time.Now().Add(c.deadline)
	var firstErr error
	for _, job := range jobs {
		select {
		case <-job.Done():
			result, err := job.Wait(context.Background())
			if err != nil && !errors.Is(err, context.Canceled) && firstErr == nil {
				firstErr = err
			}
			if env, ok := contextdata.EnvelopeFrom(c.runCtx); ok {
				if result != nil {
					if applyErr := contextstream.ApplyResult(env, result, epoch); applyErr != nil {
						var dropped *contextstream.ErrStaleApplyDropped
						if errors.As(applyErr, &dropped) {
							c.emitStaleApplyDropped(nodeID, dropped)
						} else if firstErr == nil {
							firstErr = applyErr
						}
					}
				}
				if err != nil {
					env.SetWorkingValueWithClass("contextstream.background_error", err.Error(), contextdata.MemoryClassTask)
				}
			}
		case <-time.After(time.Until(deadline)):
			c.emit(c.runCtx, telemetry.EventStreamAbandoned, "background stream abandoned", map[string]any{
				"node_id": nodeID,
				"job_id":  job.ID,
				"epoch":   epoch,
			})
		}
	}
	return firstErr
}

// emitStaleApplyDropped reports a guard rejection observed at the barrier
// through the coordinator's constructor-injected sink (nil-safe).
func (c *EpochCoordinator) emitStaleApplyDropped(nodeID string, dropped *contextstream.ErrStaleApplyDropped) {
	c.emit(c.runCtx, telemetry.EventContextStreamStaleApplyDropped, "stale stream result dropped by epoch guard", map[string]any{
		"node_id":       nodeID,
		"current_epoch": dropped.CurrentEpoch,
		"result_epoch":  dropped.GotEpoch,
		"request_id":    dropped.RequestID,
	})
}

func (c *EpochCoordinator) emitGrounded(nodeID string, epoch uint64, report knowledge.GroundingReport) {
	histogram := make(map[string]int, len(report.Grounded))
	for _, entry := range report.Grounded {
		histogram[string(entry.TrustClass)]++
	}
	c.emit(c.runCtx, telemetry.EventCaptureGrounded, "captures grounded", map[string]any{
		"node":            nodeID,
		"epoch":           epoch,
		"count":           len(report.Grounded),
		"trust_histogram": histogram,
	})
}

func (c *EpochCoordinator) emit(ctx context.Context, eventType telemetry.EventType, message string, metadata map[string]any) {
	if c == nil || c.tel == nil {
		return
	}
	event := telemetry.Event{
		Type:      eventType,
		Message:   message,
		Timestamp: time.Now().UTC(),
		Metadata:  metadata,
	}
	telemetry.StampCorrelation(ctx, &event)
	c.tel.Emit(event)
}

// executor.go is the runner's worker pool: claim from the store, resolve the
// handler by Spec.Kind, resume from checkpoint, execute within the attempt's
// timeout, and apply the Q15 retry/cancel semantics. Every state transition
// appends a store Event and emits a telemetry event — no silent transitions
// (NFR-9).
package ayenitd

import (
	"context"
	"fmt"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/jobs"
	"codeburg.org/lexbit/relurpify/telemetry"
)

// handlerRegistry resolves Spec.Kind → Handler. Unknown kinds are a terminal
// failure (never retried): storing an executor-ignorable kind would be a stub.
type handlerRegistry map[string]jobs.Handler

type executor struct {
	store    jobs.Store
	handlers handlerRegistry
	tel      telemetry.Telemetry
	workers  int
	queues   []string

	// now is the clock seam (test injection); defaults to time.Now.
	now func() time.Time

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func newExecutor(store jobs.Store, handlers handlerRegistry, tel telemetry.Telemetry, workers int, queues []string) *executor {
	if workers <= 0 {
		workers = 2
	}
	if len(queues) == 0 {
		queues = []string{"knowledge"}
	}
	return &executor{store: store, handlers: handlers, tel: tel, workers: workers, queues: queues, now: time.Now}
}

func (e *executor) Start(ctx context.Context) error {
	runCtx, cancel := context.WithCancel(ctx)
	e.cancel = cancel
	for i := 0; i < e.workers; i++ {
		e.wg.Add(1)
		go e.loop(runCtx, i)
	}
	return nil
}

func (e *executor) Stop() error {
	if e.cancel != nil {
		e.cancel()
	}
	e.wg.Wait()
	return nil
}

func (e *executor) loop(ctx context.Context, worker int) {
	defer e.wg.Done()
	workerID := fmt.Sprintf("worker-%d", worker)
	for {
		if ctx.Err() != nil {
			return
		}
		claimed, err := e.store.Claim(ctx, workerID, e.queues, 1)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			e.emit(ctx, telemetry.EventJobFailed, "claim failed", map[string]any{"error": err.Error()})
			sleepCtx(ctx, 500*time.Millisecond)
			continue
		}
		if len(claimed) == 0 {
			sleepCtx(ctx, 200*time.Millisecond)
			continue
		}
		e.runAttempt(ctx, claimed[0], workerID)
	}
}

// runAttempt executes one claimed job per Q14/Q15: handler resolve, checkpoint
// resume, timeout, and the retry-vs-terminal decision.
func (e *executor) runAttempt(ctx context.Context, job jobs.Job, workerID string) {
	e.emit(ctx, telemetry.EventJobClaimed, "job claimed", map[string]any{
		"job_id": job.ID, "kind": job.Spec.Kind, "attempt": job.Attempt, "worker": workerID,
	})

	handler, ok := e.handlers[job.Spec.Kind]
	if !ok {
		// Unknown kinds never become retryable: a kind the runner cannot run
		// is a submission bug, and retrying would only repeat it.
		e.fail(ctx, job, fmt.Errorf("unknown job kind %q", job.Spec.Kind), false)
		return
	}

	// Attempt ctx: Spec.Timeout bounds each attempt (0 = unbounded, the
	// stream-budget precedent); shutdown grace bounds everything regardless.
	attemptCtx := ctx
	if job.Spec.Timeout > 0 {
		var cancel context.CancelFunc
		attemptCtx, cancel = context.WithTimeout(ctx, job.Spec.Timeout)
		defer cancel()
	}

	// Checkpoint resume (Q15): the handler receives the prior checkpoint via
	// Job.ResumeToken → LoadCheckpoint.
	if job.ResumeToken != "" {
		if cp, err := e.store.LoadCheckpoint(ctx, job.ID); err == nil && cp != nil {
			job.Metadata = map[string]any{"resume_checkpoint": cp.ID}
		}
	}

	start := e.now()
	cp, handleErr := handler.Handle(attemptCtx, job)
	if handleErr == nil {
		if cp.State != nil {
			if err := e.store.SaveCheckpoint(ctx, cp); err != nil {
				e.fail(ctx, job, fmt.Errorf("save checkpoint: %w", err), false)
				return
			}
			job.ResumeToken = cp.Token
			if job.ResumeToken == "" {
				job.ResumeToken = cp.ID
			}
		}
		now := e.now()
		job.State = jobs.StateCompleted
		job.CompletedAt = now
		job.UpdatedAt = now
		if err := e.store.Update(ctx, job); err != nil {
			e.emit(ctx, telemetry.EventJobFailed, "complete transition failed", map[string]any{"job_id": job.ID, "error": err.Error()})
			return
		}
		_ = e.store.AppendEvent(ctx, jobs.Event{
			ID: fmt.Sprintf("%s-c-%d", job.ID, now.UnixNano()), JobID: job.ID,
			Type: jobs.EventCompleted, State: jobs.StateCompleted, Occurred: now,
		})
		e.emit(ctx, telemetry.EventJobCompleted, "job completed", map[string]any{
			"job_id": job.ID, "kind": job.Spec.Kind, "attempt": job.Attempt,
			"duration_ms": e.now().Sub(start).Milliseconds(),
		})
		return
	}

	// Shutdown cancellation is lifecycle, not failure: leave the job running
	// so the next Open's recovery pass handles it (Q14).
	if ctx.Err() != nil || (job.Spec.Timeout > 0 && handleErr == context.Canceled) {
		e.emit(ctx, telemetry.EventJobInterrupted, "attempt interrupted by shutdown", map[string]any{
			"job_id": job.ID, "attempt": job.Attempt,
		})
		return
	}

	retry := job.Attempt < maxAttempts(job)
	e.fail(ctx, job, handleErr, retry)
}

// fail applies the Q15 failure semantics: append failed event, set LastError;
// when attempts remain: back to queued with NextAttemptAt = now + NextBackoff
// and a retried event; otherwise terminal failed with CompletedAt.
func (e *executor) fail(ctx context.Context, job jobs.Job, cause error, retry bool) {
	now := e.now()
	job.LastError = cause.Error()
	job.UpdatedAt = now
	if retry {
		job.State = jobs.StateQueued
		job.NextAttemptAt = now.Add(jobs.NextBackoff(job.Spec, job.Attempt))
	} else {
		job.State = jobs.StateFailed
		job.CompletedAt = now
	}
	if err := e.store.Update(ctx, job); err != nil {
		fmt.Println("DBG fail Update error:", err)
		e.emit(ctx, telemetry.EventJobFailed, "failure transition failed", map[string]any{"job_id": job.ID, "error": err.Error()})
		return
	}
	evType := jobs.EventFailed
	message := "job failed"
	if retry {
		evType = jobs.EventRetried
		message = "job failed; retry scheduled"
	}
	_ = e.store.AppendEvent(ctx, jobs.Event{
		ID: fmt.Sprintf("%s-f-%d", job.ID, now.UnixNano()), JobID: job.ID,
		Type: evType, State: job.State, Occurred: now, Message: cause.Error(),
	})
	telemetryType := telemetry.EventJobFailed
	if retry {
		telemetryType = telemetry.EventJobRetried
	}
	e.emit(ctx, telemetryType, message, map[string]any{
		"job_id": job.ID, "kind": job.Spec.Kind, "attempt": job.Attempt,
		"error": cause.Error(), "retry": retry,
		"next_attempt_at": job.NextAttemptAt.Format(time.RFC3339),
	})
}

func maxAttempts(j jobs.Job) int {
	if j.Spec.MaxAttempts <= 0 {
		return 1
	}
	return j.Spec.MaxAttempts
}

func (e *executor) emit(ctx context.Context, t telemetry.EventType, message string, metadata map[string]any) {
	if e.tel == nil {
		return
	}
	ev := telemetry.Event{Type: t, Message: message, Timestamp: e.now().UTC(), Metadata: metadata}
	telemetry.StampCorrelation(ctx, &ev)
	e.tel.Emit(ev)
}

func sleepCtx(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

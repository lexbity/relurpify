package jobs

import (
	"context"
	"errors"
	"time"
)

// ErrConflict reports a store transaction conflict (optimistic concurrency).
// Callers retry the operation; the store implementations bound the retries
// internally.
var ErrConflict = errors.New("jobs: store transaction conflict")

// Handler executes one job kind. The runner resolves handlers by Spec.Kind
// from a registry built at process start; an unknown kind is a permanent
// failure (terminal, never retried).
//
// A Handler MUST:
//
//	H1. Be idempotent per Spec.CorrelateID: delivery is at-least-once — a
//	    crash mid-attempt re-runs the attempt after recovery. CorrelateID is
//	    the stable idempotency key.
//	H2. Honor ctx cancellation and return within the shutdown grace budget.
//	H3. Return a Checkpoint only when resuming from it is sound; the
//	    checkpoint is handed back to the next attempt via Job.ResumeToken.
//	H4. Do its own authorization/limits through the normal capability path —
//	    the runner grants no implicit scopes and its config carries no secrets.
//	H5. Emit progress via the injected telemetry sink, stamped with the job's
//	    CorrelateID — never via log output alone.
type Handler interface {
	// Handle executes one attempt. A returned Checkpoint with State != nil
	// is persisted and offered to the next attempt via Job.ResumeToken.
	Handle(ctx context.Context, j Job) (Checkpoint, error)
}

// NextBackoff computes the retry delay after an attempt failed, per Q15:
// base 30s when Spec.Backoff is unspecified, exponential ×2 per attempt,
// capped at 32× the base and at 5 minutes. attempt is the number of the
// attempt that just failed (1-based).
func NextBackoff(spec Spec, attempt int) time.Duration {
	base := spec.Backoff
	if base <= 0 {
		base = 30 * time.Second
	}
	if attempt < 1 {
		attempt = 1
	}
	// 32× base is 2^5, so shift by min(attempt-1, 5).
	shift := uint(attempt - 1)
	if shift > 5 {
		shift = 5
	}
	d := base << shift
	const cap = 5 * time.Minute
	if d > cap {
		d = cap
	}
	return d
}

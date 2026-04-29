package jobs

import "context"

// Submitter is the minimal interface a capability or agent needs to enqueue
// background work. It hides the full JobStore lifecycle from callers that only
// need to submit — lease management, retries, and state transitions remain the
// scheduler's concern.
type Submitter interface {
	Submit(ctx context.Context, spec JobSpec) (*Job, error)
}

package orchestrate

import (
	"context"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
)

// BackgroundJobNode manages background jobs (e.g., long-running tests, builds).
type BackgroundJobNode struct {
	id string
}

// NewBackgroundJobNode creates a new background job node.
func NewBackgroundJobNode(id string) *BackgroundJobNode {
	return &BackgroundJobNode{
		id: id,
	}
}

// ID returns the node ID.
func (n *BackgroundJobNode) ID() string {
	return n.id
}

// Type returns the node type.
func (n *BackgroundJobNode) Type() string {
	return "background_job"
}

// Execute manages a background job.
// Phase 12: Stub implementation - will integrate with job management.
func (n *BackgroundJobNode) Execute(ctx context.Context, env *contextdata.Envelope) (map[string]any, error) {
	// Phase 12: Stub - in production, this would launch a background job
	// Write job record to envelope
	env.SetWorkingValue("euclo.background.job_started", true, contextdata.MemoryClassTask)

	return map[string]any{
		"job_started": true,
		"stub":        true,
	}, nil
}

// JobRecord tracks a background job.
type JobRecord struct {
	JobID      string
	JobType    string
	StartedAt  int64
	Status     string
	Output     string
	CompletedAt *int64
}

package orchestrate

import (
	"context"

	"codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
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
func (n *BackgroundJobNode) Type() agentgraph.NodeType {
	return agentgraph.NodeTypeSystem
}

// Execute manages a background job.
func (n *BackgroundJobNode) Execute(ctx context.Context, env *contextdata.Envelope) (*core.Result, error) {
	_ = ctx
	result := &core.Result{
		NodeID:  n.id,
		Success: true,
		Data: map[string]any{
			"job_started": true,
		},
	}
	env.SetWorkingValue("euclo.background.job_started", true, contextdata.MemoryClassTask)
	return result, nil
}

// JobRecord tracks a background job.
type JobRecord struct {
	JobID       string
	JobType     string
	StartedAt   int64
	Status      string
	Output      string
	CompletedAt *int64
}

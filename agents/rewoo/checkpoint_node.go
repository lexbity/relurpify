package rewoo

import (
	"context"

	graph "codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
)

// CheckpointNode is a graph node that requests a checkpoint from the shared
// persistence boundary. It does not materialize the checkpoint itself.
type CheckpointNode struct {
	id     string
	phase  string
	Debugf func(string, ...interface{})
}

// NewCheckpointNode creates a new checkpoint node.
func NewCheckpointNode(id string, phase string, _ *RewooCheckpointStore) *CheckpointNode {
	return &CheckpointNode{
		id:    id,
		phase: phase,
	}
}

// ID returns the node's unique identifier.
func (n *CheckpointNode) ID() string {
	return n.id
}

// Type returns the node type.
func (n *CheckpointNode) Type() graph.NodeType {
	return graph.NodeTypeObservation
}

// Execute requests a checkpoint and records the request metadata in the envelope.
func (n *CheckpointNode) Execute(ctx context.Context, env *contextdata.Envelope) (*core.Result, error) {
	attempt := 0
	if attemptVal, ok := env.GetWorkingValue("rewoo.attempt"); ok {
		if a, ok := attemptVal.(int); ok {
			attempt = a
		}
	}
	env.RequestCheckpoint("rewoo:"+n.phase, 50, false)
	env.SetWorkingValue("rewoo.checkpoint_phase", n.phase, contextdata.MemoryClassTask)
	env.SetWorkingValue("rewoo.checkpoint_attempt", attempt, contextdata.MemoryClassTask)

	if n.Debugf != nil {
		n.Debugf("checkpoint requested at phase %s attempt %d", n.phase, attempt)
	}

	return &core.Result{
		Success: true,
		Data: map[string]interface{}{
			"checkpoint_requested": true,
			"phase":                n.phase,
		},
	}, nil
}

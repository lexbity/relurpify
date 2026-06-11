package rewoo

import (
	"context"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	graph "codeburg.org/lexbit/relurpify/execution/agentgraph"
)

// CheckpointNode is a graph node that requests a checkpoint from the shared
// persistence boundary. It does not materialize the checkpoint itself.
type CheckpointNode struct {
	id     string
	phase  string
	Debugf func(string, ...any)
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
func (n *CheckpointNode) Execute(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error) {
	attempt := 0
	if a, ok := contextdata.GetTyped[int](env, "rewoo.attempt"); ok {
		attempt = a
	}
	env.RequestCheckpoint("rewoo:"+n.phase, 50, false)
	env.SetWorkingValueWithClass("rewoo.checkpoint_phase", n.phase, contextdata.MemoryClassTask)
	env.SetWorkingValueWithClass("rewoo.checkpoint_attempt", attempt, contextdata.MemoryClassTask)

	if n.Debugf != nil {
		n.Debugf("checkpoint requested at phase %s attempt %d", n.phase, attempt)
	}

	return &execution.Result{
		Success: true,
		Data: execution.NewToolResultPayload(map[string]any{
			"checkpoint_requested": true,
			"phase":                n.phase,
		}),
	}, nil
}

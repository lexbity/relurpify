package rewoo

import (
	"context"
	"fmt"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
)

// CheckpointNode is a graph node that saves and restores execution state.
// It serves as a recovery point within the workflow.
type CheckpointNode struct {
	id       string
	phase    string
	store    *RewooCheckpointStore
	Debugf   func(string, ...interface{})
}

// NewCheckpointNode creates a new checkpoint node.
func NewCheckpointNode(id string, phase string, store *RewooCheckpointStore) *CheckpointNode {
	return &CheckpointNode{
		id:    id,
		phase: phase,
		store: store,
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

// Execute saves the current execution state as a checkpoint.
func (n *CheckpointNode) Execute(ctx context.Context, state *core.Context) (*core.Result, error) {
	if n.store == nil {
		// No checkpoint store: this is a no-op
		return &core.Result{
			Success: true,
			Data: map[string]interface{}{
				"checkpoint_skipped": true,
			},
		}, nil
	}

	// Get attempt counter from state
	attempt := 0
	if attemptVal, ok := state.Get("rewoo.attempt"); ok {
		if a, ok := attemptVal.(int); ok {
			attempt = a
		}
	}

	// Create checkpoint ID: rewoo.<phase>.<attempt>.<timestamp>
	checkpointID := fmt.Sprintf("rewoo.%s.%d.%d", n.phase, attempt, time.Now().UnixNano())

	// Save checkpoint
	if err := n.store.SaveCheckpoint(ctx, checkpointID, n.phase, attempt, state); err != nil {
		if n.Debugf != nil {
			n.Debugf("checkpoint node: save failed: %v", err)
		}
		// Non-fatal: continue execution even if checkpoint fails
	}

	state.Set("rewoo.checkpoint_id", checkpointID)
	state.Set("rewoo.checkpoint_phase", n.phase)
	state.Set("rewoo.checkpoint_timestamp", time.Now().UTC())

	if n.Debugf != nil {
		n.Debugf("checkpoint %s: saved at phase %s attempt %d", checkpointID, n.phase, attempt)
	}

	return &core.Result{
		Success: true,
		Data: map[string]interface{}{
			"checkpoint_id": checkpointID,
			"phase":         n.phase,
		},
	}, nil
}

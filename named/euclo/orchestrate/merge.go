package orchestrate

import (
	"context"

	"codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
)

// MergeNode merges results from parallel execution paths.
type MergeNode struct {
	id string
}

// NewMergeNode creates a new merge node.
func NewMergeNode(id string) *MergeNode {
	return &MergeNode{
		id: id,
	}
}

// ID returns the node ID.
func (n *MergeNode) ID() string {
	return n.id
}

// Type returns the node type.
func (n *MergeNode) Type() agentgraph.NodeType {
	return agentgraph.NodeTypeSystem
}

// Execute merges results from parallel branches.
func (n *MergeNode) Execute(ctx context.Context, env *contextdata.Envelope) (*core.Result, error) {
	_ = ctx
	env.SetWorkingValue("euclo.execution.merged", true, contextdata.MemoryClassTask)
	return &core.Result{
		NodeID:  n.id,
		Success: true,
		Data: map[string]any{
			"merged": true,
		},
	}, nil
}

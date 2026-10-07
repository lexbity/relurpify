package agentgraph

import (
	"context"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/execution"
)

// SystemAction is the body of a SystemNode execution: a system-level
// message injection or state transformation with no external side effects.
type SystemAction func(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error)

// SystemNode executes a system-level action within the graph. It is the
// escape hatch for control-flow that is neither model-backed (LLM node),
// tool-backed (tool node), nor external interaction (human node) — e.g.
// spawning a nested sub-graph, routing data between working-memory keys,
// or injecting a synthetic message.
type SystemNode struct {
	id     string
	action SystemAction
}

// NewSystemNode creates a system node that runs the given action. A nil
// action executes as a no-op pass-through, mirroring TerminalNode.
func NewSystemNode(id string, action SystemAction) *SystemNode {
	return &SystemNode{id: id, action: action}
}

// ID implements Node.
func (n *SystemNode) ID() string { return n.id }

// Type implements Node.
func (n *SystemNode) Type() NodeType { return NodeTypeSystem }

// Contract describes system nodes: they transform working context only and
// are replay-safe by construction — the referenced state lives in the
// envelope's working data, which the graph runtime re-materializes on replay.
func (n *SystemNode) Contract() NodeContract {
	return NodeContract{
		SideEffectClass: SideEffectNone,
		Idempotency:     IdempotencyReplaySafe,
		ContextPolicy:   systemContextPolicy(),
	}
}

// Execute runs the system action.
func (n *SystemNode) Execute(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error) {
	if n.action == nil {
		return &execution.Result{NodeID: n.id, Success: true}, nil
	}
	result, err := n.action(ctx, env)
	if result != nil && result.NodeID == "" {
		result.NodeID = n.id
	}
	return result, err
}

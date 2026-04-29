package orchestrate

import (
	"context"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
)

// RouteForkNode branches execution based on route kind.
type RouteForkNode struct {
	id string
}

// NewRouteForkNode creates a new route fork node.
func NewRouteForkNode(id string) *RouteForkNode {
	return &RouteForkNode{
		id: id,
	}
}

// ID returns the node ID.
func (f *RouteForkNode) ID() string {
	return f.id
}

// Type returns the node type.
func (f *RouteForkNode) Type() string {
	return "route_fork"
}

// Execute performs branching based on route kind.
// Phase 12: Stub implementation - will integrate with orchestration nodes.
func (f *RouteForkNode) Execute(ctx context.Context, env *contextdata.Envelope) (map[string]any, error) {
	// Get route kind from envelope
	routeKindVal, ok := env.GetWorkingValue("euclo.route.kind")
	if !ok {
		// Default to capability if no route kind is set
		routeKindVal = "capability"
	}

	routeKind, ok := routeKindVal.(string)
	if !ok {
		routeKind = "capability"
	}

	// Determine which branch to take
	var branch string
	switch routeKind {
	case "recipe":
		branch = "recipe_execution"
	case "capability":
		branch = "capability_execution"
	default:
		branch = "capability_execution"
	}

	// Write branch decision to envelope
	env.SetWorkingValue("euclo.fork.branch", branch, contextdata.MemoryClassTask)

	return map[string]any{
		"branch":     branch,
		"route_kind": routeKind,
	}, nil
}

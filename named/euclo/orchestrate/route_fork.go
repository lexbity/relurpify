package orchestrate

import (
	"context"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
)

// RouteForkNode branches execution based on the route selected by the dispatcher.
type RouteForkNode struct {
	id string
}

// NewRouteForkNode creates a new route fork node.
func NewRouteForkNode(id string) *RouteForkNode {
	return &RouteForkNode{id: id}
}

// ID implements agentgraph.Node.
func (f *RouteForkNode) ID() string { return f.id }

// Type implements agentgraph.Node.
func (f *RouteForkNode) Type() agentgraph.NodeType { return agentgraph.NodeTypeConditional }

// Execute resolves the branch name and returns the next node identifier.
func (f *RouteForkNode) Execute(ctx context.Context, env *contextdata.Envelope) (*agentgraph.Result, error) {
	_ = ctx
	routeKind := "capability"
	if env != nil {
		if v, ok := env.GetWorkingValue("euclo.dispatch.route_kind"); ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				routeKind = strings.TrimSpace(s)
			}
		} else if v, ok := env.GetWorkingValue("euclo.route.kind"); ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				routeKind = strings.TrimSpace(s)
			}
		}
	}

	branch := "capability_execution"
	next := "euclo.execute_capability"
	if routeKind == "recipe" {
		branch = "recipe_execution"
		next = "euclo.execute_recipe"
	}
	if env != nil {
		env.SetWorkingValue("euclo.fork.branch", branch, contextdata.MemoryClassTask)
	}
	return &agentgraph.Result{
		NodeID:  f.id,
		Success: true,
		Data: map[string]any{
			"branch":     branch,
			"route_kind": routeKind,
			"next":       next,
		},
	}, nil
}

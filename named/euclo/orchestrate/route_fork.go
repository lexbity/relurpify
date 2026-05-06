package orchestrate

import (
	"context"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
)

const dispatchRouteKindKey = "euclo.dispatch.route_kind"

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
func (f *RouteForkNode) Execute(ctx context.Context, env *contextdata.Envelope) (*core.Result, error) {
	_ = ctx
	if env == nil {
		return nil, fmt.Errorf("route fork %q requires an envelope", f.id)
	}

	routeKind := ""
	if v, ok := env.GetWorkingValue(dispatchRouteKindKey); ok {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			routeKind = strings.TrimSpace(s)
		}
	}
	routeKind = strings.TrimSpace(routeKind)
	if routeKind == "" {
		return nil, fmt.Errorf("route fork %q missing dispatch route kind", f.id)
	}

	branch := "capability_execution"
	next := "euclo.execute_capability"
	if routeKind == "recipe" {
		branch = "recipe_execution"
		next = "euclo.execute_recipe"
	}
	env.SetWorkingValue(dispatchRouteKindKey, routeKind, contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.fork.branch", branch, contextdata.MemoryClassTask)
	return &core.Result{
		NodeID:  f.id,
		Success: true,
		Data: map[string]any{
			"branch":     branch,
			"route_kind": routeKind,
			"next":       next,
			"next_node":  next,
		},
	}, nil
}

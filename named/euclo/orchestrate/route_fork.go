package orchestrate

import (
	"context"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
)

// RouteForkNode branches execution based on the resolved root route selection.
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

	routeKind := routeKindFromEnvelope(env)
	routeKind = strings.TrimSpace(routeKind)
	if routeKind == "" {
		return nil, fmt.Errorf("route fork %q missing resolved route kind", f.id)
	}

	branch := "capability_execution"
	next := "euclo.execute_capability"
	if IsThoughtRecipeRouteKind(routeKind) || IsIntentRouteKind(routeKind) {
		branch = "thoughtrecipe_execution"
		next = "euclo.execute_thoughtrecipe"
	}
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

func routeKindFromEnvelope(env *contextdata.Envelope) string {
	if env == nil {
		return ""
	}
	if selection := routeSelectionFromEnvelope(env); selection != nil {
		if routeKind := strings.TrimSpace(selection.RouteKind); routeKind != "" {
			return routeKind
		}
	}
	if resolution := routeResolutionFromEnvelope(env); resolution != nil {
		if routeKind := strings.TrimSpace(resolution.RouteKind); routeKind != "" {
			return routeKind
		}
	}
	return ""
}

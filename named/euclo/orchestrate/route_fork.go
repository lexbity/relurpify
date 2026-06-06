package orchestrate

import (
	"context"
	"fmt"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	"codeburg.org/lexbit/relurpify/named/euclo/reporting"
	euclostate "codeburg.org/lexbit/relurpify/named/euclo/state"
	execution "codeburg.org/lexbit/relurpify/execution"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
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
func (f *RouteForkNode) Execute(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error) {
	routeKind := routeKindFromEnvelope(env)
	routeKind = strings.TrimSpace(routeKind)
	if routeKind == "" {
		return nil, fmt.Errorf("route fork %q missing resolved route kind", f.id)
	}

	branch := "capability_execution"
	next := "euclo.execute_capability"
	if euclotypes.IsThoughtRecipeRouteKind(routeKind) || euclotypes.IsIntentRouteKind(routeKind) {
		branch = "thoughtrecipe_execution"
		next = "euclo.execute_thoughtrecipe"
	}
	euclostate.SetForkBranch(env, branch)

	emitBranchResolved(ctx, env, f.id, branch, routeKind)

	return &execution.Result{
		NodeID:  f.id,
		Success: true,
		Data: execution.NewToolResultPayload(map[string]any{
			"branch":     branch,
			"route_kind": routeKind,
			"next":       next,
			"next_node":  next,
		}),
	}, nil
}

func routeKindFromEnvelope(env *contextdata.Envelope) string {
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

func emitBranchResolved(ctx context.Context, env *contextdata.Envelope, groupID, chosenBranch, routeKind string) {
	tel := reporting.NewEucloTelemetry(telemetry.TelemetryFromContext(ctx))
	tel.EmitBranchResolved(ctx, reporting.EventBranchResolved{
		EventHeader: reporting.EventHeader{
			TaskID:     env.TaskID,
			SessionID:  env.SessionID,
			OccurredAt: time.Now().UTC(),
		},
		GroupID:      groupID,
		ChosenBranch: chosenBranch,
		RouteKind:    routeKind,
	})
}

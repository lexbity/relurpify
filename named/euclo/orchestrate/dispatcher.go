package orchestrate

import (
	"context"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
)

// Dispatcher routes execution to the appropriate orchestration path based on RouteSelection.
type Dispatcher struct {
	id string
}

// NewDispatcher creates a new dispatcher.
func NewDispatcher(id string) *Dispatcher {
	return &Dispatcher{
		id: id,
	}
}

// ID returns the node ID.
func (d *Dispatcher) ID() string {
	return d.id
}

// Type returns the node type.
func (d *Dispatcher) Type() string {
	return "dispatcher"
}

// Execute performs route selection and dispatches to the appropriate execution path.
// Phase 12: Stub implementation - will integrate with route fork and orchestration nodes.
func (d *Dispatcher) Execute(ctx context.Context, env *contextdata.Envelope) (map[string]any, error) {
	// Get route selection from envelope
	// Phase 12: Stub - in production, this would extract RouteSelection from envelope
	routeSelection := &RouteSelection{
		RouteKind:    "capability", // stub default
		RecipeID:     "",
		CapabilityID: "debug",
	}

	// Write route selection to envelope
	env.SetWorkingValue("euclo.route.kind", routeSelection.RouteKind, contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.route.recipe_id", routeSelection.RecipeID, contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.route.capability_id", routeSelection.CapabilityID, contextdata.MemoryClassTask)

	return map[string]any{
		"route_kind":     routeSelection.RouteKind,
		"recipe_id":      routeSelection.RecipeID,
		"capability_id":  routeSelection.CapabilityID,
	}, nil
}

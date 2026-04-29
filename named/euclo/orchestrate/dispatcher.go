package orchestrate

import (
	"context"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/named/euclo/intake"
)

// Dispatcher resolves the execution route from the envelope and persists the
// selected route for downstream nodes.
type Dispatcher struct {
	id string
}

// NewDispatcher creates a new dispatcher.
func NewDispatcher(id string) *Dispatcher {
	return &Dispatcher{id: id}
}

// ID implements agentgraph.Node.
func (d *Dispatcher) ID() string { return d.id }

// Type implements agentgraph.Node.
func (d *Dispatcher) Type() agentgraph.NodeType { return agentgraph.NodeTypeSystem }

// Execute selects recipe or capability execution and writes the route to the envelope.
func (d *Dispatcher) Execute(ctx context.Context, env *contextdata.Envelope) (*agentgraph.Result, error) {
	_ = ctx
	selection := routeSelectionFromEnvelope(env)
	if selection == nil {
		selection = &RouteSelection{}
	}

	if strings.TrimSpace(selection.RouteKind) == "" {
		selection.RouteKind = classifyRoute(env)
	}
	if strings.TrimSpace(selection.RouteKind) == "" {
		selection.RouteKind = "capability"
	}

	if selection.RouteKind == "recipe" && strings.TrimSpace(selection.RecipeID) == "" {
		selection.RecipeID = defaultRecipeID(env)
	}
	if selection.RouteKind != "recipe" && strings.TrimSpace(selection.CapabilityID) == "" {
		selection.CapabilityID = defaultCapabilityID(env)
	}

	if env != nil {
		env.SetWorkingValue("euclo.route_selection", selection, contextdata.MemoryClassTask)
		env.SetWorkingValue("euclo.dispatch.route_kind", selection.RouteKind, contextdata.MemoryClassTask)
		env.SetWorkingValue("euclo.route.kind", selection.RouteKind, contextdata.MemoryClassTask)
		env.SetWorkingValue("euclo.route.recipe_id", selection.RecipeID, contextdata.MemoryClassTask)
		env.SetWorkingValue("euclo.route.capability_id", selection.CapabilityID, contextdata.MemoryClassTask)
	}

	return &agentgraph.Result{
		NodeID:  d.id,
		Success: true,
		Data: map[string]any{
			"route_kind":    selection.RouteKind,
			"recipe_id":     selection.RecipeID,
			"capability_id": selection.CapabilityID,
		},
	}, nil
}

func routeSelectionFromEnvelope(env *contextdata.Envelope) *RouteSelection {
	if env == nil {
		return nil
	}
	if v, ok := env.GetWorkingValue("euclo.route_selection"); ok {
		if rs, ok := v.(*RouteSelection); ok && rs != nil {
			return rs
		}
	}
	return nil
}

func classifyRoute(env *contextdata.Envelope) string {
	if env == nil {
		return ""
	}
	if v, ok := env.GetWorkingValue("euclo.family_selection"); ok {
		if family, ok := v.(string); ok {
			switch strings.TrimSpace(family) {
			case "review", "investigation", "architecture":
				return "recipe"
			case "repair", "migration", "implementation":
				return "capability"
			}
		}
	}
	if v, ok := env.GetWorkingValue("euclo.intent_classification"); ok {
		if cls, ok := v.(*intake.IntentClassification); ok && cls != nil {
			if strings.TrimSpace(cls.WinningFamily) == "review" || strings.TrimSpace(cls.WinningFamily) == "investigation" {
				return "recipe"
			}
		}
	}
	return ""
}

func defaultRecipeID(env *contextdata.Envelope) string {
	if env == nil {
		return "euclo.recipe.default"
	}
	if v, ok := env.GetWorkingValue("euclo.recipe_id"); ok {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			return s
		}
	}
	return "euclo.recipe.default"
}

func defaultCapabilityID(env *contextdata.Envelope) string {
	if env == nil {
		return "euclo:cap.ast_query"
	}
	if v, ok := env.GetWorkingValue("euclo.capability_sequence"); ok {
		if seq, ok := v.([]string); ok && len(seq) > 0 && strings.TrimSpace(seq[0]) != "" {
			return strings.TrimSpace(seq[0])
		}
	}
	return "euclo:cap.ast_query"
}

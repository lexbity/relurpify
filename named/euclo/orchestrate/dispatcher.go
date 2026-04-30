package orchestrate

import (
	"context"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/named/euclo/intake"
	recipepkg "codeburg.org/lexbit/relurpify/named/euclo/recipes"
	"codeburg.org/lexbit/relurpify/named/euclo/reporting"
)

// Dispatcher resolves the execution route from the envelope and persists the
// selected route for downstream nodes.
type Dispatcher struct {
	id                 string
	capabilityRegistry *capability.CapabilityRegistry
	recipeRegistry     *recipepkg.RecipeRegistry
	workspace          string
}

// NewDispatcher creates a new dispatcher.
func NewDispatcher(id string) *Dispatcher {
	return &Dispatcher{id: id}
}

// WithCapabilityRegistry wires the capability registry used for route selection.
func (d *Dispatcher) WithCapabilityRegistry(reg *capability.CapabilityRegistry) *Dispatcher {
	if d != nil && reg != nil {
		d.capabilityRegistry = reg
	}
	return d
}

// WithRecipeRegistry wires the recipe registry used for route selection.
func (d *Dispatcher) WithRecipeRegistry(reg *recipepkg.RecipeRegistry) *Dispatcher {
	if d != nil && reg != nil {
		d.recipeRegistry = reg
	}
	return d
}

// WithWorkspace wires the workspace root used for skill resolution.
func (d *Dispatcher) WithWorkspace(workspace string) *Dispatcher {
	if d != nil {
		d.workspace = strings.TrimSpace(workspace)
	}
	return d
}

// ID implements agentgraph.Node.
func (d *Dispatcher) ID() string { return d.id }

// Type implements agentgraph.Node.
func (d *Dispatcher) Type() agentgraph.NodeType { return agentgraph.NodeTypeSystem }

// Execute selects recipe or capability execution and writes the route to the envelope.
func (d *Dispatcher) Execute(ctx context.Context, env *contextdata.Envelope) (*agentgraph.Result, error) {
	req := routeRequestFromEnvelope(env)
	if env != nil {
		if v, ok := env.GetWorkingValue("euclo.route.telemetry_off"); ok {
			if off, ok := v.(bool); ok {
				req.TelemetryOff = off
			}
		}
		if v, ok := env.GetWorkingValue("euclo.dry_run_mode"); ok {
			if dryRun, ok := v.(bool); ok {
				req.DryRun = dryRun
			}
		}
	}

	caps := d.capabilityRegistry
	skillFilterName := strings.TrimSpace(req.SkillFilter)
	if skillFilterName != "" && caps != nil {
		scopedCaps, err := applySkillFilterToRegistry(d.workspace, skillFilterName, caps)
		if err != nil {
			return &agentgraph.Result{NodeID: d.id, Success: false, Data: map[string]any{"error": err.Error()}}, err
		}
		caps = scopedCaps
	}

	var (
		result *RouteResult
		err    error
	)
	if req.DryRun {
		report, dryRunErr := DryRun(ctx, env, req, caps, d.recipeRegistry)
		err = dryRunErr
		if err != nil {
			return &agentgraph.Result{NodeID: d.id, Success: false, Data: map[string]any{"error": err.Error()}}, err
		}
		result = &RouteResult{
			RouteKind:           report.SelectedKind,
			RouteID:             string(report.SelectedRoute),
			SkillFilterName:     report.SkillFilterName,
			CandidateCount:      len(report.Candidates),
			FallbackID:          fallbackIDString(report.FallbackPath),
			ApprovalRequired:    report.HITLRequired,
			ArtifactKinds:       append([]string(nil), report.ExpectedArtifactKinds...),
			Outcome:             string(reporting.RouteOutcomeDryRun),
			TelemetrySuppressed: req.TelemetryOff,
		}
	} else {
		result, err = Dispatch(ctx, env, req, caps, d.recipeRegistry)
		if err != nil {
			return &agentgraph.Result{NodeID: d.id, Success: false, Data: map[string]any{"error": err.Error()}}, err
		}
		if result != nil && skillFilterName != "" {
			result.SkillFilterName = skillFilterName
		}
	}

	applyRouteResultToEnvelope(env, result)

	return &agentgraph.Result{
		NodeID:  d.id,
		Success: true,
		Data: map[string]any{
			"route_kind":      result.RouteKind,
			"route_id":        result.RouteID,
			"skill_filter":    result.SkillFilterName,
			"candidate_count": result.CandidateCount,
			"fallback_taken":  result.FallbackTaken,
			"fallback_id":     result.FallbackID,
			"outcome":         result.Outcome,
		},
	}, nil
}

func routeRequestFromEnvelope(env *contextdata.Envelope) RouteRequest {
	req := RouteRequest{}
	if env == nil {
		return req
	}
	if v, ok := env.GetWorkingValue("euclo.family_selection"); ok {
		if s, ok := v.(string); ok {
			req.FamilyID = strings.TrimSpace(s)
		}
	}
	if selection := routeSelectionFromEnvelope(env); selection != nil {
		req.RecipeID = strings.TrimSpace(selection.RecipeID)
		req.CapabilityID = strings.TrimSpace(selection.CapabilityID)
	}
	kind := ""
	if selection := routeSelectionFromEnvelope(env); selection != nil {
		kind = strings.TrimSpace(selection.RouteKind)
	}
	if req.RecipeID != "" {
		kind = "recipe"
	} else if req.CapabilityID != "" {
		kind = "capability"
	} else if v, ok := env.GetWorkingValue("euclo.recipe_id"); ok {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			req.RecipeID = strings.TrimSpace(s)
			kind = "recipe"
		}
	}
	if kind == "" {
		if v, ok := env.GetWorkingValue("euclo.capability_sequence"); ok {
			if seq, ok := v.([]string); ok && len(seq) > 0 && strings.TrimSpace(seq[0]) != "" {
				req.CapabilityID = strings.TrimSpace(seq[0])
				kind = "capability"
			}
		}
	}
	if kind == "" {
		kind = classifyRoute(env)
	}
	switch kind {
	case "recipe":
		if req.RecipeID == "" && req.FamilyID == "" {
			req.RecipeID = defaultRecipeID(env)
		}
	case "capability":
		if req.CapabilityID == "" && req.FamilyID == "" {
			req.CapabilityID = defaultCapabilityID(env)
		}
	default:
		if req.FamilyID == "" {
			req.CapabilityID = defaultCapabilityID(env)
			kind = "capability"
		}
	}
	if v, ok := env.GetWorkingValue("euclo.route.fallback_id"); ok {
		if s, ok := v.(string); ok {
			req.FallbackID = strings.TrimSpace(s)
		}
	}
	if v, ok := env.GetWorkingValue("euclo.skill_filter"); ok {
		if s, ok := v.(string); ok {
			req.SkillFilter = strings.TrimSpace(s)
		}
	}
	return req
}

func applyRouteResultToEnvelope(env *contextdata.Envelope, result *RouteResult) {
	if env == nil || result == nil {
		return
	}
	selection := &RouteSelection{RouteKind: result.RouteKind}
	switch result.RouteKind {
	case "recipe":
		selection.RecipeID = result.RouteID
	default:
		selection.CapabilityID = result.RouteID
	}
	env.SetWorkingValue("euclo.route_selection", selection, contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.dispatch.route_kind", result.RouteKind, contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.route.kind", result.RouteKind, contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.route.recipe_id", selection.RecipeID, contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.route.capability_id", selection.CapabilityID, contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.route.candidate_count", result.CandidateCount, contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.route.fallback_taken", result.FallbackTaken, contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.route.fallback_id", result.FallbackID, contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.route.skill_filter", result.SkillFilterName, contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.route.outcome", result.Outcome, contextdata.MemoryClassTask)
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

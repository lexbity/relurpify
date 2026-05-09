package orchestrate

import (
	"context"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/euclo/intake"
	intentcontext "codeburg.org/lexbit/relurpify/named/euclo/intentcontext"
	"codeburg.org/lexbit/relurpify/named/euclo/reporting"
	thoughtrecipepkg "codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
)

// Dispatcher resolves the execution route from the envelope and persists the
// selected route for downstream nodes.
type Dispatcher struct {
	id                    string
	capabilityRegistry    *capability.CapabilityRegistry
	thoughtrecipeRegistry *thoughtrecipepkg.ThoughtRecipeRegistry
	workspace             string
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

// WithThoughtRecipeRegistry wires the thoughtrecipe registry used for route selection.
func (d *Dispatcher) WithThoughtRecipeRegistry(reg *thoughtrecipepkg.ThoughtRecipeRegistry) *Dispatcher {
	if d != nil && reg != nil {
		d.thoughtrecipeRegistry = reg
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

// Execute selects thoughtrecipe or capability execution and writes the route to the envelope.
func (d *Dispatcher) Execute(ctx context.Context, env *contextdata.Envelope) (*core.Result, error) {
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
	if needsClarificationRoute(env) && strings.TrimSpace(req.ThoughtRecipeID) == "" && strings.TrimSpace(req.CapabilityID) == "" {
		emitClarificationGateResult(ctx, env, nil, false, "clarify", "clarification lifecycle required")
	}

	caps := d.capabilityRegistry
	skillFilterName := strings.TrimSpace(req.SkillFilter)
	if skillFilterName != "" && caps != nil {
		scopedCaps, err := applySkillFilterToRegistry(d.workspace, skillFilterName, caps)
		if err != nil {
			return &core.Result{NodeID: d.id, Success: false, Data: map[string]any{"error": err.Error()}}, err
		}
		caps = scopedCaps
	}

	var (
		result *RouteResult
		err    error
	)
	if req.DryRun {
		report, dryRunErr := DryRun(ctx, env, req, caps, d.thoughtrecipeRegistry)
		err = dryRunErr
		if err != nil {
			return &core.Result{NodeID: d.id, Success: false, Data: map[string]any{"error": err.Error()}}, err
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
		result, err = Dispatch(ctx, env, req, caps, d.thoughtrecipeRegistry)
		if err != nil {
			return &core.Result{NodeID: d.id, Success: false, Data: map[string]any{"error": err.Error()}}, err
		}
		if result != nil && skillFilterName != "" {
			result.SkillFilterName = skillFilterName
		}
	}

	applyRouteResultToEnvelope(env, result)

	return &core.Result{
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
		req.ThoughtRecipeID = strings.TrimSpace(selection.ThoughtRecipeID)
		req.CapabilityID = strings.TrimSpace(selection.CapabilityID)
	}
	if resolution := routeResolutionFromEnvelope(env); resolution != nil && req.ThoughtRecipeID == "" && req.CapabilityID == "" {
		switch strings.ToLower(strings.TrimSpace(resolution.RouteKind)) {
		case RouteKindThoughtRecipe, RouteKindIntent:
			req.ThoughtRecipeID = strings.TrimSpace(resolution.ThoughtRecipeID)
		case RouteKindCapability:
			req.CapabilityID = strings.TrimSpace(resolution.CapabilityID)
		}
	}
	if req.ThoughtRecipeID == "" && req.CapabilityID == "" {
		if v, ok := env.GetWorkingValue("euclo.thoughtrecipe_id"); ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				req.ThoughtRecipeID = strings.TrimSpace(s)
			}
		}
	}
	if req.ThoughtRecipeID == "" && req.CapabilityID == "" && (classificationRequiresClarification(env) || !hasIntentGrounding(env)) {
		req.ThoughtRecipeID = clarificationThoughtRecipeID
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
	case RouteKindThoughtRecipe, RouteKindIntent:
		selection.ThoughtRecipeID = result.RouteID
	default:
		selection.CapabilityID = result.RouteID
	}
	applyRouteSelectionToEnvelope(env, selection, result)
}

func applyRouteSelectionToEnvelope(env *contextdata.Envelope, selection *RouteSelection, result *RouteResult) {
	if env == nil {
		return
	}
	if selection != nil {
		env.SetWorkingValue("euclo.route_selection", selection, contextdata.MemoryClassTask)
		routeID := selection.ThoughtRecipeID
		if routeID == "" {
			routeID = selection.CapabilityID
		}
		env.SetWorkingValue("euclo.route.continuation", &RouteContinuation{
			SharedContext:         true,
			SourceRouteKind:       selection.RouteKind,
			SourceRouteID:         routeID,
			TargetRouteKind:       selection.RouteKind,
			TargetRouteID:         routeID,
			ActiveThoughtRecipeID: selection.ThoughtRecipeID,
		}, contextdata.MemoryClassTask)
	} else {
		env.SetWorkingValue("euclo.route_selection", nil, contextdata.MemoryClassTask)
		env.SetWorkingValue("euclo.route.continuation", nil, contextdata.MemoryClassTask)
	}
	if result != nil {
		env.SetWorkingValue("euclo.dispatch.route_kind", result.RouteKind, contextdata.MemoryClassTask)
		env.SetWorkingValue("euclo.route.candidate_count", result.CandidateCount, contextdata.MemoryClassTask)
		env.SetWorkingValue("euclo.route.fallback_taken", result.FallbackTaken, contextdata.MemoryClassTask)
		env.SetWorkingValue("euclo.route.fallback_id", result.FallbackID, contextdata.MemoryClassTask)
		env.SetWorkingValue("euclo.route.skill_filter", result.SkillFilterName, contextdata.MemoryClassTask)
		env.SetWorkingValue("euclo.route.outcome", result.Outcome, contextdata.MemoryClassTask)
	}
}

func applyRouteResolutionToEnvelope(env *contextdata.Envelope, resolution *RouteResolution) {
	if env == nil {
		return
	}
	if resolution == nil {
		env.SetWorkingValue(intentcontext.RouteResolutionKey, nil, contextdata.MemoryClassTask)
		env.SetWorkingValue("euclo.route_resolution", nil, contextdata.MemoryClassTask)
		return
	}
	resolution.Normalize()
	env.SetWorkingValue(intentcontext.RouteResolutionKey, resolution, contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.route_resolution", resolution, contextdata.MemoryClassTask)
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

func routeResolutionFromEnvelope(env *contextdata.Envelope) *RouteResolution {
	if env == nil {
		return nil
	}
	if v, ok := env.GetWorkingValue(intentcontext.RouteResolutionKey); ok {
		if res, ok := v.(*RouteResolution); ok && res != nil {
			return res
		}
	}
	if v, ok := env.GetWorkingValue("euclo.route_resolution"); ok {
		if res, ok := v.(*RouteResolution); ok && res != nil {
			return res
		}
	}
	return nil
}

func hasIntentGrounding(env *contextdata.Envelope) bool {
	if env == nil {
		return false
	}
	if v, ok := env.GetWorkingValue(intentcontext.IntentEvidenceKey); ok {
		if evidence, ok := v.(*intentcontext.IntentEvidence); ok && evidence != nil {
			return true
		}
	}
	if v, ok := env.GetWorkingValue(intentcontext.IntentInterpretationKey); ok {
		if interpretation, ok := v.(*intentcontext.IntentInterpretation); ok && interpretation != nil {
			return true
		}
	}
	if v, ok := env.GetWorkingValue(intentcontext.ClarificationStateKey); ok {
		if state, ok := v.(*intentcontext.ClarificationState); ok && state != nil {
			return true
		}
	}
	if v, ok := env.GetWorkingValue("euclo.family_selection"); ok {
		switch value := v.(type) {
		case string:
			if strings.TrimSpace(value) != "" {
				return true
			}
		case map[string]any:
			if _, ok := value["winning_family"]; ok {
				return true
			}
		}
	}
	if v, ok := env.GetWorkingValue("euclo.skill_filter"); ok {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			return true
		}
	}
	if routeSelectionFromEnvelope(env) != nil {
		return true
	}
	if resolution := routeResolutionFromEnvelope(env); resolution != nil {
		return strings.TrimSpace(resolution.RouteID()) != ""
	}
	return false
}

func classificationRequiresClarification(env *contextdata.Envelope) bool {
	if env == nil {
		return false
	}
	if v, ok := env.GetWorkingValue("euclo.intent_classification"); ok {
		if cls, ok := v.(*intake.IntentClassification); ok && cls != nil {
			return cls.Ambiguous
		}
	}
	return false
}

func classifyRoute(env *contextdata.Envelope) string {
	if env == nil {
		return ""
	}
	if needsClarificationRoute(env) {
		return RouteKindIntent
	}
	if v, ok := env.GetWorkingValue("euclo.family_selection"); ok {
		if family, ok := v.(string); ok {
			switch strings.TrimSpace(family) {
			case "review", "investigation", "architecture":
				return RouteKindThoughtRecipe
			case "repair", "migration", "implementation":
				return RouteKindCapability
			}
		}
	}
	if v, ok := env.GetWorkingValue("euclo.intent_classification"); ok {
		if cls, ok := v.(*intake.IntentClassification); ok && cls != nil {
			if strings.TrimSpace(cls.WinningFamily) == "review" || strings.TrimSpace(cls.WinningFamily) == "investigation" {
				return RouteKindThoughtRecipe
			}
		}
	}
	return ""
}

func defaultThoughtRecipeID(env *contextdata.Envelope) string {
	if env == nil {
		return clarificationThoughtRecipeID
	}
	if needsClarificationRoute(env) {
		return clarificationThoughtRecipeID
	}
	if v, ok := env.GetWorkingValue("euclo.thoughtrecipe_id"); ok {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			return s
		}
	}
	return clarificationThoughtRecipeID
}

func defaultCapabilityID(env *contextdata.Envelope) string {
	if env != nil {
		if v, ok := env.GetWorkingValue("euclo.capability_id"); ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return "euclo:cap.ast_query"
}

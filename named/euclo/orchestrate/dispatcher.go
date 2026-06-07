package orchestrate

import (
	"context"
	"strings"

	"codeburg.org/lexbit/relurpify/capability"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentgraph"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	intentcontext "codeburg.org/lexbit/relurpify/named/euclo/intentcontext"
	"codeburg.org/lexbit/relurpify/named/euclo/reporting"
	euclostate "codeburg.org/lexbit/relurpify/named/euclo/state"
	thoughtrecipepkg "codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
)

// Dispatcher resolves the execution route from the envelope and persists the
// selected route for downstream nodes.
type Dispatcher struct {
	id                    string
	capabilityRegistry    *capability.Registry
	thoughtrecipeRegistry *thoughtrecipepkg.ThoughtRecipeRegistry
	workspace             string
}

// NewDispatcher creates a new dispatcher.
func NewDispatcher(id string) *Dispatcher {
	return &Dispatcher{id: id}
}

// WithCapabilityRegistry wires the capability registry used for route selection.
func (d *Dispatcher) WithCapabilityRegistry(reg *capability.Registry) *Dispatcher {
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
func (d *Dispatcher) Execute(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error) {
	req := routeRequestFromEnvelope(env)
	if env != nil {
		req.TelemetryOff = euclostate.GetRouteTelemetryOff(env)
		dryRun, hasDryRun := euclostate.GetDryRunMode(env)
		if hasDryRun {
			req.DryRun = dryRun
		}
	}
	if needsClarificationRoute(env) && strings.TrimSpace(req.ThoughtRecipeID) == "" && strings.TrimSpace(req.CapabilityID) == "" {
		emitClarificationGateResult(ctx, env, nil, false, "clarify", "clarification lifecycle required")
	}

	caps := d.capabilityRegistry

	var (
		result *RouteResult
		err    error
	)
	if req.DryRun {
		report, dryRunErr := DryRun(ctx, env, req, caps, d.thoughtrecipeRegistry)
		err = dryRunErr
		if err != nil {
			return &execution.Result{NodeID: d.id, Success: false, Data: execution.NewErrorResultPayload(err.Error())}, err
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
			return &execution.Result{NodeID: d.id, Success: false, Data: execution.NewErrorResultPayload(err.Error())}, err
		}
	}

	applyRouteResultToEnvelope(env, result)

	return &execution.Result{
		NodeID:  d.id,
		Success: true,
		Data: execution.NewToolResultPayload(map[string]any{
			"route_kind":      result.RouteKind,
			"route_id":        result.RouteID,
			"skill_filter":    result.SkillFilterName,
			"candidate_count": result.CandidateCount,
			"fallback_taken":  result.FallbackTaken,
			"fallback_id":     result.FallbackID,
			"outcome":         result.Outcome,
		}),
	}, nil
}

func routeRequestFromEnvelope(env *contextdata.Envelope) RouteRequest {
	req := RouteRequest{}
	if s, ok := euclostate.GetFamilySelection(env); ok {
		req.FamilyID = strings.TrimSpace(s)
	}
	if selection := routeSelectionFromEnvelope(env); selection != nil {
		req.ThoughtRecipeID = strings.TrimSpace(selection.ThoughtRecipeID)
		req.CapabilityID = strings.TrimSpace(selection.CapabilityID)
	}
	if resolution := routeResolutionFromEnvelope(env); resolution != nil && req.ThoughtRecipeID == "" && req.CapabilityID == "" {
		switch strings.ToLower(strings.TrimSpace(resolution.RouteKind)) {
		case euclotypes.RouteKindThoughtRecipe, euclotypes.RouteKindIntent:
			req.ThoughtRecipeID = strings.TrimSpace(resolution.ThoughtRecipeID)
		case euclotypes.RouteKindCapability:
			req.CapabilityID = strings.TrimSpace(resolution.CapabilityID)
		}
	}
	if req.ThoughtRecipeID == "" && req.CapabilityID == "" {
		if s, ok := euclostate.GetThoughtRecipeID(env); ok && strings.TrimSpace(s) != "" {
			req.ThoughtRecipeID = strings.TrimSpace(s)
		}
	}
	if req.ThoughtRecipeID == "" && req.CapabilityID == "" && (classificationRequiresClarification(env) || !hasIntentGrounding(env)) {
		req.ThoughtRecipeID = clarificationThoughtRecipeID
	}
	req.FallbackID = strings.TrimSpace(euclostate.GetRouteFallbackID(env))
	req.SkillFilter = strings.TrimSpace(euclostate.GetSkillFilter(env))
	return req
}

func applyRouteResultToEnvelope(env *contextdata.Envelope, result *RouteResult) {
	if result == nil {
		return
	}
	selection := &euclotypes.RouteSelection{RouteKind: result.RouteKind}
	switch result.RouteKind {
	case euclotypes.RouteKindThoughtRecipe, euclotypes.RouteKindIntent:
		selection.ThoughtRecipeID = result.RouteID
	default:
		selection.CapabilityID = result.RouteID
	}
	applyRouteSelectionToEnvelope(env, selection, result)
}

func applyRouteSelectionToEnvelope(env *contextdata.Envelope, selection *euclotypes.RouteSelection, result *RouteResult) {
	if selection != nil {
		euclostate.SetRouteSelection(env, selection)
		routeID := selection.ThoughtRecipeID
		if routeID == "" {
			routeID = selection.CapabilityID
		}
		euclostate.SetRouteContinuation(env, &euclotypes.RouteContinuation{
			SharedContext:         true,
			SourceRouteKind:       selection.RouteKind,
			SourceRouteID:         routeID,
			TargetRouteKind:       selection.RouteKind,
			TargetRouteID:         routeID,
			ActiveThoughtRecipeID: selection.ThoughtRecipeID,
		})
	} else {
		euclostate.SetRouteSelection(env, nil)
		euclostate.SetRouteContinuation(env, nil)
	}
	if result != nil {
		euclostate.SetDispatchRouteKind(env, result.RouteKind)
		euclostate.SetRouteCandidateCount(env, result.CandidateCount)
		euclostate.SetRouteFallbackTaken(env, result.FallbackTaken)
		euclostate.SetRouteFallbackID(env, result.FallbackID)
		euclostate.SetRouteSkillFilter(env, result.SkillFilterName)
		euclostate.SetRouteOutcome(env, result.Outcome)
	}
}

func applyRouteResolutionToEnvelope(env *contextdata.Envelope, resolution *euclotypes.RouteResolution) {
	if resolution != nil {
		resolution.Normalize()
	}
	euclostate.SetRouteResolution(env, resolution)
}

func routeSelectionFromEnvelope(env *contextdata.Envelope) *euclotypes.RouteSelection {
	rs, _ := euclostate.GetRouteSelection(env)
	return rs
}

func routeResolutionFromEnvelope(env *contextdata.Envelope) *euclotypes.RouteResolution {
	res, _ := euclostate.GetRouteResolution(env)
	return res
}

func hasIntentGrounding(env *contextdata.Envelope) bool {
	if evidence, ok := euclostate.GetIntentEvidence(env); ok && evidence != nil {
		return true
	}
	if interpretation, ok := euclostate.GetIntentInterpretation(env); ok && interpretation != nil {
		return true
	}
	if v, ok := env.GetWorkingValue(intentcontext.ClarificationStateKey); ok {
		if state, ok := v.(*intentcontext.ClarificationState); ok && state != nil {
			return true
		}
	}
	if v, ok := euclostate.GetFamilySelection(env); ok && strings.TrimSpace(v) != "" {
		return true
	}
	if s := euclostate.GetSkillFilter(env); strings.TrimSpace(s) != "" {
		return true
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
	cls, ok := euclostate.GetIntentClassification(env)
	return ok && cls != nil && cls.Ambiguous
}

func classifyRoute(env *contextdata.Envelope) string {
	if needsClarificationRoute(env) {
		return euclotypes.RouteKindIntent
	}
	if family, ok := euclostate.GetFamilySelection(env); ok {
		switch strings.TrimSpace(family) {
		case "review", "investigation", "architecture":
			return euclotypes.RouteKindThoughtRecipe
		case "repair", "migration", "implementation":
			return euclotypes.RouteKindCapability
		}
	}
	if cls, ok := euclostate.GetIntentClassification(env); ok && cls != nil {
		if strings.TrimSpace(cls.WinningFamily) == "review" || strings.TrimSpace(cls.WinningFamily) == "investigation" {
			return euclotypes.RouteKindThoughtRecipe
		}
	}
	return ""
}

func defaultThoughtRecipeID(env *contextdata.Envelope) string {
	if needsClarificationRoute(env) {
		return clarificationThoughtRecipeID
	}
	if s, ok := euclostate.GetThoughtRecipeID(env); ok && strings.TrimSpace(s) != "" {
		return s
	}
	return clarificationThoughtRecipeID
}

func defaultCapabilityID(env *contextdata.Envelope) string {
	if env != nil {
		if v, ok := env.GetWorkingValue(euclostate.KeyCapabilityID); ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return "euclo:cap.ast_query"
}

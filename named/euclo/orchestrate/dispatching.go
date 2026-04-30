package orchestrate

import (
	"context"
	"sort"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	recipepkg "codeburg.org/lexbit/relurpify/named/euclo/recipes"
	"codeburg.org/lexbit/relurpify/named/euclo/reporting"
)

// Dispatch resolves a route request and records route telemetry.
func Dispatch(ctx context.Context, env *contextdata.Envelope, req RouteRequest, caps *capability.CapabilityRegistry, recipes *recipepkg.RecipeRegistry) (*RouteResult, error) {
	report, selected, fallbackTaken, ok := resolveRoute(req, caps, recipes)
	if !ok {
		if !req.TelemetryOff {
			for _, candidate := range report.Candidates {
				if candidate.Availability != RouteAvailable {
					reporting.EmitRouteUnavailable(ctx, taskID(env), sessionID(env), string(candidate.RouteID), string(candidate.Availability), candidate.SuppressReason)
				}
			}
		}
		return nil, &RouteResolutionError{PrimaryID: primaryRouteID(req), Reason: "no eligible route candidates"}
	}

	if fallbackTaken {
		fallback := selected.RouteID
		report.FallbackPath = &fallback
	}
	result := routeResultFromSelection(report, selected, fallbackTaken, false, req.TelemetryOff)
	if selected.Availability != RouteAvailable {
		reason := selected.SuppressReason
		if strings.TrimSpace(reason) == "" {
			reason = "route unavailable"
		}
		if !req.TelemetryOff {
			reporting.EmitRouteUnavailable(ctx, taskID(env), sessionID(env), string(selected.RouteID), string(selected.Availability), reason)
		}
		return nil, &RouteResolutionError{PrimaryID: string(selected.RouteID), Reason: reason}
	}
	if !req.TelemetryOff {
		reporting.EmitRouteSelected(ctx, taskID(env), sessionID(env), req.FamilyID, result.RouteKind, result.RouteID, result.CandidateCount, result.FallbackTaken)
		if result.FallbackTaken && result.FallbackID != "" {
			reporting.EmitRouteFallback(ctx, taskID(env), sessionID(env), primaryRouteID(req), result.FallbackID, "primary route unavailable")
		}
		reporting.EmitRouteCompleted(ctx, taskID(env), sessionID(env), result.RouteKind, result.RouteID, reporting.RouteOutcomeSuccess, result.ArtifactKinds, 0)
	}
	if env != nil {
		applyRouteResultToEnvelope(env, result)
	}
	return result, nil
}

func dryRun(ctx context.Context, env *contextdata.Envelope, req RouteRequest, caps *capability.CapabilityRegistry, recipes *recipepkg.RecipeRegistry) (*DryRunReport, error) {
	report, selected, fallbackTaken, ok := resolveRoute(req, caps, recipes)
	report.SkillFilterName = strings.TrimSpace(req.SkillFilter)
	if ok {
		report.SelectedRoute = selected.RouteID
		report.SelectedKind = selected.RouteKind
		report.ExecutionClass = executionClassForCandidate(selected)
		report.ExpectedArtifactKinds = expectedArtifactsForRoute(string(selected.RouteID), selected.RouteKind)
		if fallbackTaken {
			fallback := selected.RouteID
			report.FallbackPath = &fallback
		}
		if selected.Availability != RouteAvailable {
			report.PolicyBlockers = append(report.PolicyBlockers, selected.SuppressReason)
		}
	} else {
		report.ExecutionClass = "blocked"
		report.PreflightErrors = append(report.PreflightErrors, "no eligible route candidates")
	}

	if !req.TelemetryOff {
		for _, candidate := range report.Candidates {
			if candidate.Availability != RouteAvailable {
				reporting.EmitRouteUnavailable(ctx, taskID(env), sessionID(env), string(candidate.RouteID), string(candidate.Availability), candidate.SuppressReason)
			}
		}
		reporting.EmitRouteDryRun(ctx, taskID(env), sessionID(env), report)
	}

	if !ok {
		return report, &RouteResolutionError{PrimaryID: primaryRouteID(req), Reason: "no eligible route candidates"}
	}
	if env != nil {
		if result := routeResultFromSelection(report, selected, fallbackTaken, true, req.TelemetryOff); result != nil {
			applyRouteResultToEnvelope(env, result)
		}
	}
	return report, nil
}

func routeResultFromSelection(report *DryRunReport, selected CandidateRouteInfo, fallbackTaken, dryRun, telemetrySuppressed bool) *RouteResult {
	if report == nil {
		return nil
	}
	outcome := reporting.RouteOutcomeSuccess
	if dryRun {
		outcome = reporting.RouteOutcomeDryRun
	}
	artifactKinds := append([]string(nil), report.ExpectedArtifactKinds...)
	if len(artifactKinds) == 0 {
		artifactKinds = expectedArtifactsForRoute(string(selected.RouteID), selected.RouteKind)
	}
	result := &RouteResult{
		RouteKind:           selected.RouteKind,
		RouteID:             string(selected.RouteID),
		SkillFilterName:     report.SkillFilterName,
		CandidateCount:      len(report.Candidates),
		FallbackTaken:       fallbackTaken,
		FallbackID:          fallbackIDString(report.FallbackPath),
		ApprovalRequired:    report.HITLRequired,
		ArtifactKinds:       artifactKinds,
		Outcome:             string(outcome),
		TelemetrySuppressed: telemetrySuppressed,
	}
	return result
}

func rankCandidates(req RouteRequest, caps *capability.CapabilityRegistry, recipes *recipepkg.RecipeRegistry) []CandidateRouteInfo {
	candidates := make([]CandidateRouteInfo, 0)
	for _, cand := range rankCapabilityCandidates(req, caps) {
		candidates = append(candidates, CandidateRouteInfo{
			RouteID:        cand.RouteID,
			RouteKind:      cand.RouteKind,
			Availability:   cand.Availability,
			RankScore:      cand.RankScore,
			RankReasons:    append([]string(nil), cand.RankReasons...),
			Suppressed:     cand.Suppressed,
			SuppressReason: cand.SuppressReason,
		})
	}
	for _, cand := range rankRecipeCandidates(req, recipes) {
		candidates = append(candidates, CandidateRouteInfo{
			RouteID:        cand.RouteID,
			RouteKind:      cand.RouteKind,
			Availability:   cand.Availability,
			RankScore:      cand.RankScore,
			RankReasons:    append([]string(nil), cand.RankReasons...),
			Suppressed:     cand.Suppressed,
			SuppressReason: cand.SuppressReason,
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].RankScore == candidates[j].RankScore {
			return candidates[i].RouteID < candidates[j].RouteID
		}
		return candidates[i].RankScore > candidates[j].RankScore
	})
	return candidates
}

func rankCapabilityCandidates(req RouteRequest, caps *capability.CapabilityRegistry) []rankedRoute {
	if caps == nil {
		return nil
	}
	snapshots := caps.AllCapabilitySnapshots()
	candidates := make([]rankedRoute, 0, len(snapshots))
	for _, snap := range snapshots {
		if !routeMatchesFamily(snap.Descriptor, req.FamilyID, req.Instruction) {
			continue
		}
		availability, reason := routeAvailabilityFromSnapshot(snap)
		score := 0
		reasons := []string{}
		if strings.TrimSpace(req.CapabilityID) != "" && req.CapabilityID == snap.Descriptor.ID {
			score += 100
			reasons = append(reasons, "explicit capability")
		}
		if familyMatchBonus(snap.Descriptor, req.FamilyID) {
			score += 20
			reasons = append(reasons, "family match")
		}
		score += capabilityPriorityScore(snap.Descriptor)
		score += availabilityScore(availability)
		score += compatibilityScore(snap.Descriptor, req.Inputs)
		score -= riskPenalty(snap.Descriptor)
		if availability == RouteUnavailablePolicyDenied {
			reasons = append(reasons, "policy denied")
		}
		if reason != "" {
			reasons = append(reasons, reason)
		}
		candidates = append(candidates, rankedRoute{
			RouteID:        RouteID(snap.Descriptor.ID),
			RouteKind:      "capability",
			Availability:   availability,
			RankScore:      score,
			RankReasons:    reasons,
			Suppressed:     availability == RouteUnavailablePolicyDenied,
			SuppressReason: reason,
		})
	}
	return candidates
}

func rankRecipeCandidates(req RouteRequest, recipes *recipepkg.RecipeRegistry) []rankedRoute {
	if recipes == nil {
		return nil
	}
	ids := recipes.List()
	sort.Strings(ids)
	candidates := make([]rankedRoute, 0, len(ids))
	for _, id := range ids {
		score := 0
		reasons := []string{}
		if req.RecipeID != "" && req.RecipeID == id {
			score += 100
			reasons = append(reasons, "explicit recipe")
		}
		if strings.EqualFold(req.FamilyID, "review") || strings.EqualFold(req.FamilyID, "investigation") || strings.EqualFold(req.FamilyID, "architecture") {
			score += 10
			reasons = append(reasons, "family recipe")
		}
		candidates = append(candidates, rankedRoute{
			RouteID:      RouteID(id),
			RouteKind:    "recipe",
			Availability: RouteAvailable,
			RankScore:    score,
			RankReasons:  reasons,
		})
	}
	return candidates
}

type rankedRoute struct {
	RouteID        RouteID
	RouteKind      string
	Availability   RouteAvailability
	RankScore      int
	RankReasons    []string
	Suppressed     bool
	SuppressReason string
}

func selectCandidate(req RouteRequest, candidates []CandidateRouteInfo) (CandidateRouteInfo, bool, bool) {
	if requested, ok := explicitCandidate(req, candidates); ok {
		if requested.Availability == RouteAvailable {
			return requested, false, true
		}
		if req.FallbackID != "" {
			if fallback, ok := candidateByID(candidates, req.FallbackID); ok && fallback.Availability == RouteAvailable {
				return fallback, true, true
			}
		}
		return requested, false, true
	}

	for _, candidate := range candidates {
		if candidate.Availability == RouteAvailable {
			return candidate, false, true
		}
	}
	if req.FallbackID != "" {
		if fallback, ok := candidateByID(candidates, req.FallbackID); ok && fallback.Availability == RouteAvailable {
			return fallback, true, true
		}
	}
	return CandidateRouteInfo{}, false, false
}

func explicitCandidate(req RouteRequest, candidates []CandidateRouteInfo) (CandidateRouteInfo, bool) {
	if strings.TrimSpace(req.RecipeID) != "" {
		if candidate, ok := candidateByIDKind(candidates, "recipe", req.RecipeID); ok {
			return candidate, true
		}
	}
	if strings.TrimSpace(req.CapabilityID) != "" {
		if candidate, ok := candidateByIDKind(candidates, "capability", req.CapabilityID); ok {
			return candidate, true
		}
	}
	return CandidateRouteInfo{}, false
}

func candidateByID(candidates []CandidateRouteInfo, id string) (CandidateRouteInfo, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return CandidateRouteInfo{}, false
	}
	for _, candidate := range candidates {
		if string(candidate.RouteID) == id {
			return candidate, true
		}
	}
	return CandidateRouteInfo{}, false
}

func candidateByIDKind(candidates []CandidateRouteInfo, kind, id string) (CandidateRouteInfo, bool) {
	id = strings.TrimSpace(id)
	kind = strings.TrimSpace(kind)
	if id == "" || kind == "" {
		return CandidateRouteInfo{}, false
	}
	for _, candidate := range candidates {
		if candidate.RouteKind == kind && string(candidate.RouteID) == id {
			return candidate, true
		}
	}
	return CandidateRouteInfo{}, false
}

func resolveRoute(req RouteRequest, caps *capability.CapabilityRegistry, recipes *recipepkg.RecipeRegistry) (*DryRunReport, CandidateRouteInfo, bool, bool) {
	report := &DryRunReport{Request: req}
	report.Candidates = rankCandidates(req, caps, recipes)
	if len(report.Candidates) == 0 && caps == nil && recipes == nil {
		synth := syntheticCandidate(req)
		report.Candidates = []CandidateRouteInfo{synth}
		return report, synth, false, true
	}
	selected, fallbackTaken, ok := selectCandidate(req, report.Candidates)
	return report, selected, fallbackTaken, ok
}

func syntheticCandidate(req RouteRequest) CandidateRouteInfo {
	kind := routeKindFromRequest(req)
	id := strings.TrimSpace(req.CapabilityID)
	if kind == "recipe" {
		id = strings.TrimSpace(req.RecipeID)
		if id == "" {
			id = "euclo.recipe.default"
		}
	} else {
		if id == "" {
			id = "euclo:cap.ast_query"
		}
		kind = "capability"
	}
	return CandidateRouteInfo{
		RouteID:      RouteID(id),
		RouteKind:    kind,
		Availability: RouteAvailable,
		RankScore:    0,
	}
}

func routeKindFromRequest(req RouteRequest) string {
	if strings.TrimSpace(req.RecipeID) != "" {
		return "recipe"
	}
	if strings.TrimSpace(req.CapabilityID) != "" {
		return "capability"
	}
	switch strings.ToLower(strings.TrimSpace(req.FamilyID)) {
	case "review", "investigation", "architecture":
		return "recipe"
	default:
		return "capability"
	}
}

func routeAvailabilityFromSnapshot(snapshot capability.CapabilitySnapshot) (RouteAvailability, string) {
	if snapshot.Exposure == core.CapabilityExposureHidden {
		return RouteUnavailablePolicyDenied, "policy denied"
	}
	if snapshot.Descriptor.Availability.Available {
		return RouteAvailable, ""
	}
	reason := strings.ToLower(snapshot.Descriptor.Availability.Reason)
	switch {
	case strings.Contains(reason, "dependency"):
		return RouteUnavailableDependencyMissing, snapshot.Descriptor.Availability.Reason
	case strings.Contains(reason, "unsupported"):
		return RouteUnavailableUnsupported, snapshot.Descriptor.Availability.Reason
	default:
		return RouteUnavailableToolNotEnabled, snapshot.Descriptor.Availability.Reason
	}
}

func familyMatchBonus(desc core.CapabilityDescriptor, family string) bool {
	family = strings.ToLower(strings.TrimSpace(family))
	if family == "" {
		return false
	}
	switch family {
	case "query":
		return strings.Contains(strings.ToLower(desc.ID), "ast_query") || strings.Contains(strings.ToLower(desc.ID), "symbol_trace") || strings.Contains(strings.ToLower(desc.ID), "call_graph")
	case "review":
		return strings.Contains(strings.ToLower(desc.ID), "code_review") || strings.Contains(strings.ToLower(desc.ID), "diff_summary")
	case "repair":
		return strings.Contains(strings.ToLower(desc.ID), "targeted_refactor") || strings.Contains(strings.ToLower(desc.ID), "rename_symbol")
	case "test":
		return strings.Contains(strings.ToLower(desc.ID), "test_run") || strings.Contains(strings.ToLower(desc.ID), "coverage_check")
	case "architecture":
		return strings.Contains(strings.ToLower(desc.ID), "layer_check") || strings.Contains(strings.ToLower(desc.ID), "boundary_report")
	case "migration":
		return strings.Contains(strings.ToLower(desc.ID), "api_compat")
	case "debug":
		return strings.Contains(strings.ToLower(desc.ID), "bisect")
	default:
		return strings.Contains(strings.ToLower(desc.Name), family) || strings.Contains(strings.ToLower(desc.Category), family) || strings.Contains(strings.ToLower(desc.ID), family)
	}
}

func routeMatchesFamily(desc core.CapabilityDescriptor, family, instruction string) bool {
	family = strings.ToLower(strings.TrimSpace(family))
	if family == "" {
		return true
	}
	if familyMatchBonus(desc, family) {
		return true
	}
	instruction = strings.ToLower(instruction)
	return strings.Contains(strings.ToLower(desc.ID), family) || strings.Contains(strings.ToLower(desc.Name), family) || strings.Contains(strings.ToLower(desc.Category), family) || strings.Contains(instruction, family)
}

func capabilityPriorityScore(desc core.CapabilityDescriptor) int {
	if desc.Annotations == nil {
		return 0
	}
	if raw, ok := desc.Annotations["euclo.priority"]; ok {
		switch v := raw.(type) {
		case int:
			return v
		case int64:
			return int(v)
		case float64:
			return int(v)
		}
	}
	return 0
}

func compatibilityScore(desc core.CapabilityDescriptor, inputs map[string]any) int {
	if desc.InputSchema == nil || len(desc.InputSchema.Required) == 0 {
		return 0
	}
	score := 0
	for _, key := range desc.InputSchema.Required {
		if inputs == nil {
			continue
		}
		if _, ok := inputs[key]; ok {
			score++
		}
	}
	return score
}

func riskPenalty(desc core.CapabilityDescriptor) int {
	return len(desc.RiskClasses)
}

func availabilityScore(a RouteAvailability) int {
	switch a {
	case RouteAvailable:
		return 100
	case RouteUnavailableToolNotEnabled:
		return 10
	case RouteUnavailableDependencyMissing:
		return 5
	case RouteUnavailableUnsupported:
		return 1
	case RouteUnavailablePolicyDenied:
		return -100
	default:
		return 0
	}
}

func expectedArtifactsForRoute(routeID, routeKind string) []string {
	kind := strings.ToLower(strings.TrimSpace(routeID)) + " " + strings.ToLower(strings.TrimSpace(routeKind))
	switch {
	case strings.Contains(kind, "review"), strings.Contains(kind, "summary"):
		return []string{"report"}
	case strings.Contains(kind, "refactor"), strings.Contains(kind, "migration"):
		return []string{"patch"}
	case strings.Contains(kind, "verification"), strings.Contains(kind, "test"):
		return []string{"test_report"}
	default:
		return []string{"result"}
	}
}

func executionClassForCandidate(candidate CandidateRouteInfo) string {
	if candidate.RouteKind == "recipe" {
		return "graph"
	}
	if candidate.Availability != RouteAvailable {
		return "blocked"
	}
	return "fast"
}

func taskID(env *contextdata.Envelope) string {
	if env == nil {
		return ""
	}
	return env.TaskID
}

func sessionID(env *contextdata.Envelope) string {
	if env == nil {
		return ""
	}
	return env.SessionID
}

func fallbackIDString(id *RouteID) string {
	if id == nil {
		return ""
	}
	return string(*id)
}

func primaryRouteID(req RouteRequest) string {
	if strings.TrimSpace(req.RecipeID) != "" {
		return strings.TrimSpace(req.RecipeID)
	}
	if strings.TrimSpace(req.CapabilityID) != "" {
		return strings.TrimSpace(req.CapabilityID)
	}
	return strings.TrimSpace(req.FallbackID)
}

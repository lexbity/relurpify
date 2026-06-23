package orchestrate

import (
	"context"
	"sort"
	"strings"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/descriptor"
	registry "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/governance/risk"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	intentcontext "codeburg.org/lexbit/relurpify/named/euclo/intentcontext"
	"codeburg.org/lexbit/relurpify/named/euclo/reporting"
	thoughtrecipepkg "codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
)

// Dispatch resolves a route request and records route telemetry.
func Dispatch(ctx context.Context, env *contextdata.Envelope, req RouteRequest, caps *registry.CapabilityRegistry, thoughtrecipes *thoughtrecipepkg.ThoughtRecipeRegistry) (*RouteResult, error) {
	report, selected, fallbackTaken, ok := resolveRoute(env, req, caps, thoughtrecipes)
	resolution := buildRouteResolution(env, req, report, selected, ok, fallbackTaken)
	if env != nil {
		applyRouteResolutionToEnvelope(env, resolution)
		if ok {
			applyRouteSelectionToEnvelope(env, routeSelectionFromCandidate(selected), nil)
		} else {
			applyRouteSelectionToEnvelope(env, nil, nil)
		}
	}
	if !ok {
		if !req.TelemetryOff {
			for _, candidate := range report.Candidates {
				if candidate.Availability != RouteAvailable {
					reporting.EmitRouteUnavailable(ctx, taskID(env), sessionID(env), string(candidate.RouteID), string(candidate.Availability), candidate.SuppressReason)
				}
			}
		}
		return nil, &RouteResolutionError{PrimaryID: primaryRouteID(req), Reason: unresolvedRouteReason(report, selected, resolution)}
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

func dryRun(ctx context.Context, env *contextdata.Envelope, req RouteRequest, caps *registry.CapabilityRegistry, thoughtrecipes *thoughtrecipepkg.ThoughtRecipeRegistry) (*DryRunReport, error) {
	report, selected, fallbackTaken, ok := resolveRoute(env, req, caps, thoughtrecipes)
	resolution := buildRouteResolution(env, req, report, selected, ok, fallbackTaken)
	if env != nil {
		applyRouteResolutionToEnvelope(env, resolution)
		if ok {
			applyRouteSelectionToEnvelope(env, routeSelectionFromCandidate(selected), nil)
		} else {
			applyRouteSelectionToEnvelope(env, nil, nil)
		}
	}
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
		report.PreflightErrors = append(report.PreflightErrors, unresolvedRouteReason(report, selected, resolution))
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
		return report, &RouteResolutionError{PrimaryID: primaryRouteID(req), Reason: unresolvedRouteReason(report, selected, resolution)}
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

func resolveRoute(env *contextdata.Envelope, req RouteRequest, caps *registry.CapabilityRegistry, thoughtrecipes *thoughtrecipepkg.ThoughtRecipeRegistry) (*DryRunReport, CandidateRouteInfo, bool, bool) {
	report := &DryRunReport{Request: req}
	report.Candidates = deterministicRouteCandidates(env, req, caps, thoughtrecipes)
	selected, ok := selectDeterministicCandidate(req, report.Candidates)
	return report, selected, false, ok
}

func deterministicRouteCandidates(env *contextdata.Envelope, req RouteRequest, caps *registry.CapabilityRegistry, thoughtrecipes *thoughtrecipepkg.ThoughtRecipeRegistry) []CandidateRouteInfo {
	clarificationCandidate := clarificationRouteCandidate(env, req)
	if explicit := explicitRouteCandidate(req, caps, thoughtrecipes); explicit != nil {
		if clarificationCandidate != nil && candidateRouteID(*explicit) == candidateRouteID(*clarificationCandidate) {
			return []CandidateRouteInfo{*explicit}
		}
		return []CandidateRouteInfo{*explicit}
	}

	candidates := make([]CandidateRouteInfo, 0, 8)
	if clarificationCandidate != nil {
		candidates = append(candidates, *clarificationCandidate)
	}
	candidates = append(candidates, metadataThoughtRecipeCandidates(env, req, thoughtrecipes)...)
	candidates = append(candidates, metadataCapabilityCandidates(env, req, caps)...)
	return dedupeAndSortRouteCandidates(candidates)
}

func explicitRouteCandidate(req RouteRequest, caps *registry.CapabilityRegistry, thoughtrecipes *thoughtrecipepkg.ThoughtRecipeRegistry) *CandidateRouteInfo {
	switch {
	case strings.TrimSpace(req.ThoughtRecipeID) != "":
		id := strings.TrimSpace(req.ThoughtRecipeID)
		if id == clarificationThoughtRecipeID {
			return &CandidateRouteInfo{
				RouteID:      RouteID(id),
				RouteKind:    euclotypes.RouteKindIntent,
				Availability: RouteAvailable,
				RankScore:    1000,
				RankReasons:  []string{"explicit clarification route"},
			}
		}
		if thoughtrecipes != nil {
			if recipe, ok := thoughtrecipes.Get(id); ok && recipe != nil {
				return &CandidateRouteInfo{
					RouteID:      RouteID(recipe.ID),
					RouteKind:    euclotypes.RouteKindForThoughtRecipeID(recipe.ID),
					Availability: RouteAvailable,
					RankScore:    1000,
					RankReasons:  []string{"explicit thoughtrecipe"},
				}
			}
		}
		return &CandidateRouteInfo{
			RouteID:        RouteID(id),
			RouteKind:      euclotypes.RouteKindForThoughtRecipeID(id),
			Availability:   RouteUnavailableUnsupported,
			RankScore:      1000,
			RankReasons:    []string{"explicit thoughtrecipe not found"},
			SuppressReason: "explicit thoughtrecipe not found",
		}
	case strings.TrimSpace(req.CapabilityID) != "":
		id := strings.TrimSpace(req.CapabilityID)
		if caps != nil {
			if snapshot, ok := capabilitySnapshotByID(caps, id); ok {
				availability, reason := routeAvailabilityFromSnapshot(snapshot)
				return &CandidateRouteInfo{
					RouteID:        RouteID(snapshot.Descriptor.ID),
					RouteKind:      euclotypes.RouteKindCapability,
					Availability:   availability,
					RankScore:      1000,
					RankReasons:    []string{"explicit capability"},
					Suppressed:     availability == RouteUnavailablePolicyDenied,
					SuppressReason: reason,
				}
			}
		}
		return &CandidateRouteInfo{
			RouteID:        RouteID(id),
			RouteKind:      euclotypes.RouteKindCapability,
			Availability:   RouteUnavailableUnsupported,
			RankScore:      1000,
			RankReasons:    []string{"explicit capability not found"},
			SuppressReason: "explicit capability not found",
		}
	default:
		return nil
	}
}

func clarificationRouteCandidate(env *contextdata.Envelope, req RouteRequest) *CandidateRouteInfo {
	needsClarify := needsClarificationRoute(env)
	hasDirectGrounding := strings.TrimSpace(req.Instruction) != "" || strings.TrimSpace(req.FamilyID) != "" || strings.TrimSpace(req.SkillFilter) != ""
	if !needsClarify && !hasDirectGrounding && strings.TrimSpace(req.ThoughtRecipeID) != clarificationThoughtRecipeID {
		if !hasIntentGrounding(env) {
			needsClarify = true
		}
	}
	if !needsClarify && strings.TrimSpace(req.ThoughtRecipeID) != clarificationThoughtRecipeID {
		return nil
	}
	return &CandidateRouteInfo{
		RouteID:      RouteID(clarificationThoughtRecipeID),
		RouteKind:    euclotypes.RouteKindIntent,
		Availability: RouteAvailable,
		RankScore:    900,
		RankReasons:  []string{"clarification route"},
	}
}

func metadataThoughtRecipeCandidates(env *contextdata.Envelope, req RouteRequest, thoughtrecipes *thoughtrecipepkg.ThoughtRecipeRegistry) []CandidateRouteInfo {
	if thoughtrecipes == nil {
		return nil
	}
	tokens := routeSearchTokens(env, req)
	if len(tokens) == 0 {
		return nil
	}
	candidates := make([]CandidateRouteInfo, 0)
	for _, entry := range thoughtrecipes.Entries() {
		routeID := routeIDForThoughtRecipeEntry(entry)
		if routeID == "" {
			continue
		}
		score, reasons := scoreThoughtRecipeCandidate(entry, tokens)
		if score <= 0 {
			continue
		}
		candidates = append(candidates, CandidateRouteInfo{
			RouteID:      RouteID(routeID),
			RouteKind:    euclotypes.RouteKindForThoughtRecipeID(routeID),
			Availability: RouteAvailable,
			RankScore:    score,
			RankReasons:  reasons,
		})
	}
	return candidates
}

func metadataCapabilityCandidates(env *contextdata.Envelope, req RouteRequest, caps *registry.CapabilityRegistry) []CandidateRouteInfo {
	if caps == nil {
		return nil
	}
	tokens := routeSearchTokens(env, req)
	snapshots := caps.AllCapabilitySnapshots()
	if len(tokens) == 0 {
		if strings.TrimSpace(req.SkillFilter) == "" {
			return nil
		}
		candidates := make([]CandidateRouteInfo, 0, len(snapshots))
		for _, snapshot := range snapshots {
			availability, reason := routeAvailabilityFromSnapshot(snapshot)
			candidates = append(candidates, CandidateRouteInfo{
				RouteID:        RouteID(snapshot.Descriptor.ID),
				RouteKind:      euclotypes.RouteKindCapability,
				Availability:   availability,
				RankScore:      capabilityPriorityScore(snapshot.Descriptor) + availabilityScore(availability),
				Suppressed:     availability == RouteUnavailablePolicyDenied,
				SuppressReason: reason,
			})
		}
		return dedupeAndSortRouteCandidates(candidates)
	}
	candidates := make([]CandidateRouteInfo, 0, len(snapshots))
	for _, snapshot := range snapshots {
		score, reasons := scoreCapabilityCandidate(snapshot.Descriptor, tokens)
		if score <= 0 {
			continue
		}
		availability, reason := routeAvailabilityFromSnapshot(snapshot)
		candidates = append(candidates, CandidateRouteInfo{
			RouteID:        RouteID(snapshot.Descriptor.ID),
			RouteKind:      euclotypes.RouteKindCapability,
			Availability:   availability,
			RankScore:      score,
			RankReasons:    reasons,
			Suppressed:     availability == RouteUnavailablePolicyDenied,
			SuppressReason: reason,
		})
	}
	return candidates
}

func selectDeterministicCandidate(req RouteRequest, candidates []CandidateRouteInfo) (CandidateRouteInfo, bool) {
	if len(candidates) == 0 {
		return CandidateRouteInfo{}, false
	}
	if strings.TrimSpace(req.ThoughtRecipeID) != "" || strings.TrimSpace(req.CapabilityID) != "" {
		for _, candidate := range candidates {
			if candidateMatchesRequest(candidate, req) {
				return candidate, candidate.Availability == RouteAvailable
			}
		}
		return CandidateRouteInfo{}, false
	}
	bestScore := -1
	var best CandidateRouteInfo
	for _, candidate := range candidates {
		if candidate.Availability != RouteAvailable {
			continue
		}
		if candidate.RankScore > bestScore {
			bestScore = candidate.RankScore
			best = candidate
			continue
		}
		if candidate.RankScore == bestScore {
			if best.RouteID == "" || candidate.RouteID < best.RouteID {
				best = candidate
			}
		}
	}
	if bestScore < 0 {
		return CandidateRouteInfo{}, false
	}
	return best, true
}

func candidateMatchesRequest(candidate CandidateRouteInfo, req RouteRequest) bool {
	if candidate.RouteKind == euclotypes.RouteKindCapability && strings.TrimSpace(req.CapabilityID) != "" {
		return string(candidate.RouteID) == strings.TrimSpace(req.CapabilityID)
	}
	if strings.TrimSpace(req.ThoughtRecipeID) == "" {
		return false
	}
	return string(candidate.RouteID) == strings.TrimSpace(req.ThoughtRecipeID) || string(candidate.RouteID) == strings.TrimSpace(clarificationThoughtRecipeID)
}

func dedupeAndSortRouteCandidates(candidates []CandidateRouteInfo) []CandidateRouteInfo {
	if len(candidates) == 0 {
		return nil
	}
	byID := make(map[string]CandidateRouteInfo, len(candidates))
	for _, candidate := range candidates {
		id := candidateRouteID(candidate)
		if id == "" {
			continue
		}
		if existing, ok := byID[id]; ok {
			if candidate.RankScore > existing.RankScore {
				byID[id] = candidate
			}
			continue
		}
		byID[id] = candidate
	}
	out := make([]CandidateRouteInfo, 0, len(byID))
	for _, candidate := range byID {
		out = append(out, candidate)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].RankScore == out[j].RankScore {
			if out[i].RouteKind == out[j].RouteKind {
				return out[i].RouteID < out[j].RouteID
			}
			return out[i].RouteKind < out[j].RouteKind
		}
		return out[i].RankScore > out[j].RankScore
	})
	return out
}

func routeSearchTokens(env *contextdata.Envelope, req RouteRequest) []string {
	tokens := make([]string, 0, 16)
	add := func(value string) {
		trimmed := strings.ToLower(strings.TrimSpace(value))
		if trimmed != "" {
			tokens = append(tokens, trimmed)
		}
		for _, token := range splitRouteTokens(value) {
			if token != "" {
				tokens = append(tokens, token)
			}
		}
	}
	add(req.FamilyID)
	add(req.Instruction)
	if v, ok := envRouteEvidence(env); ok && v != nil {
		add(v.ActionType)
		add(v.Target)
		add(v.Scope)
		add(v.RiskLevel)
		add(v.ExpectedVerb)
		add(v.SessionContinuation)
		add(v.FollowUp)
		for _, item := range v.ContextHints {
			add(item)
		}
		for _, item := range v.MissingFields {
			add(item)
		}
		for _, item := range v.ReasonCodes {
			add(item)
		}
	}
	if v, ok := envRouteInterpretation(env); ok && v != nil {
		add(v.ActionType)
		add(v.Target)
		add(v.Scope)
		add(v.RiskLevel)
		add(v.Rationale)
		add(v.ConfidenceNote)
		for _, item := range v.MissingInfo {
			add(item)
		}
		for _, item := range v.ReasonCodes {
			add(item)
		}
	}
	if state := routeClarificationState(env); state != nil {
		add(state.ActiveThoughtRecipeID)
		if state.Ambiguity != nil {
			add(state.Ambiguity.Rationale)
			for _, family := range state.Ambiguity.CandidateFamilies {
				add(family)
			}
		}
		for _, question := range state.PendingQuestions {
			add(question.PromptFamily)
			add(question.Text)
		}
	}
	return uniqueStrings(tokens)
}

func splitRouteTokens(value string) []string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return nil
	}
	tokens := strings.FieldsFunc(value, func(r rune) bool {
		switch {
		case r == '_', r == '-', r == ':', r == '.', r == '/', r == '\\':
			return true
		case 'a' <= r && r <= 'z':
			return false
		case '0' <= r && r <= '9':
			return false
		default:
			return true
		}
	})
	out := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if trimmed := strings.TrimSpace(token); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(strings.ToLower(value))
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func envRouteEvidence(env *contextdata.Envelope) (*intentcontext.IntentEvidence, bool) {
	v, ok := contextdata.GetTyped[any](env, intentcontext.IntentEvidenceKey)
	if !ok {
		return nil, false
	}
	evidence, ok := v.(*intentcontext.IntentEvidence)
	return evidence, ok
}

func envRouteInterpretation(env *contextdata.Envelope) (*intentcontext.IntentInterpretation, bool) {
	v, ok := contextdata.GetTyped[any](env, intentcontext.IntentInterpretationKey)
	if !ok {
		return nil, false
	}
	interpretation, ok := v.(*intentcontext.IntentInterpretation)
	return interpretation, ok
}

func routeClarificationState(env *contextdata.Envelope) *intentcontext.ClarificationState {
	v, ok := contextdata.GetTyped[any](env, intentcontext.ClarificationStateKey)
	if !ok {
		return nil
	}
	state, _ := v.(*intentcontext.ClarificationState)
	return state
}

func routeSelectionFromCandidate(candidate CandidateRouteInfo) *euclotypes.RouteSelection {
	if candidate.RouteID == "" {
		return nil
	}
	selection := &euclotypes.RouteSelection{RouteKind: candidate.RouteKind}
	if euclotypes.IsCapabilityRouteKind(candidate.RouteKind) {
		selection.CapabilityID = string(candidate.RouteID)
	} else {
		selection.ThoughtRecipeID = string(candidate.RouteID)
	}
	return selection
}

func buildRouteResolution(env *contextdata.Envelope, req RouteRequest, report *DryRunReport, selected CandidateRouteInfo, ok bool, fallbackTaken bool) *euclotypes.RouteResolution {
	resolution := &euclotypes.RouteResolution{
		RouteKind:        selected.RouteKind,
		ThoughtRecipeID:  "",
		CapabilityID:     "",
		ResolutionSource: "registry",
		FallbackTaken:    fallbackTaken,
		ReasonCodes:      nil,
	}
	if state := routeClarificationState(env); state != nil {
		resolution.ClarificationStateVersion = state.StateVersion
	}
	if ok {
		if euclotypes.IsCapabilityRouteKind(selected.RouteKind) {
			resolution.CapabilityID = string(selected.RouteID)
		} else {
			resolution.ThoughtRecipeID = string(selected.RouteID)
		}
		if string(selected.RouteID) == clarificationThoughtRecipeID {
			resolution.ResolutionSource = "clarification"
		}
		if len(selected.RankReasons) > 0 {
			resolution.ReasonCodes = append(resolution.ReasonCodes, selected.RankReasons...)
		}
	} else {
		resolution.ResolutionSource = "unresolved"
		if strings.TrimSpace(req.ThoughtRecipeID) != "" {
			resolution.RouteKind = euclotypes.RouteKindForThoughtRecipeID(req.ThoughtRecipeID)
			resolution.ThoughtRecipeID = strings.TrimSpace(req.ThoughtRecipeID)
		} else if strings.TrimSpace(req.CapabilityID) != "" {
			resolution.RouteKind = euclotypes.RouteKindCapability
			resolution.CapabilityID = strings.TrimSpace(req.CapabilityID)
		}
		if selected.RouteID != "" {
			if euclotypes.IsCapabilityRouteKind(selected.RouteKind) {
				resolution.CapabilityID = string(selected.RouteID)
			} else {
				resolution.ThoughtRecipeID = string(selected.RouteID)
			}
		}
		if report != nil {
			resolution.ReasonCodes = append(resolution.ReasonCodes, report.PreflightErrors...)
		}
		if len(selected.RankReasons) > 0 {
			resolution.ReasonCodes = append(resolution.ReasonCodes, selected.RankReasons...)
		}
	}
	resolution.Normalize()
	return resolution
}

func routeKindFromRequest(req RouteRequest) string {
	if strings.TrimSpace(req.ThoughtRecipeID) != "" {
		return euclotypes.RouteKindForThoughtRecipeID(req.ThoughtRecipeID)
	}
	if strings.TrimSpace(req.CapabilityID) != "" {
		return euclotypes.RouteKindCapability
	}
	return ""
}

func unresolvedRouteReason(report *DryRunReport, selected CandidateRouteInfo, resolution *euclotypes.RouteResolution) string {
	reasons := make([]string, 0, 4)
	if resolution != nil {
		reasons = append(reasons, resolution.ReasonCodes...)
	}
	if report != nil {
		reasons = append(reasons, report.PreflightErrors...)
	}
	if selected.SuppressReason != "" {
		reasons = append(reasons, selected.SuppressReason)
	}
	if len(reasons) == 0 {
		return "no eligible route candidates"
	}
	return strings.Join(uniqueStrings(reasons), "; ")
}

func candidateRouteID(candidate CandidateRouteInfo) string {
	return strings.TrimSpace(string(candidate.RouteID))
}

func routeIDForThoughtRecipeEntry(entry thoughtrecipepkg.ThoughtRecipeEntry) string {
	if entry.ThoughtRecipe == nil {
		return strings.TrimSpace(entry.Name)
	}
	if id := strings.TrimSpace(entry.ThoughtRecipe.ID); id != "" {
		return id
	}
	return strings.TrimSpace(entry.Name)
}

func scoreThoughtRecipeCandidate(entry thoughtrecipepkg.ThoughtRecipeEntry, tokens []string) (int, []string) {
	if entry.ThoughtRecipe == nil {
		return 0, nil
	}
	score := 0
	reasons := make([]string, 0, 4)
	if exactTokenMatch(tokens, entry.ThoughtRecipe.ID, entry.Name) {
		score += 100
		reasons = append(reasons, "explicit thoughtrecipe")
	}
	if matched, ok := tokenMatchesAny(tokens, entry.ThoughtRecipe.Metadata.Families...); ok {
		score += 30
		reasons = append(reasons, "family")
		_ = matched
	}
	if matched, ok := tokenMatchesAny(tokens, entry.ThoughtRecipe.Metadata.Keywords...); ok {
		score += 20
		reasons = append(reasons, "keyword")
		_ = matched
	}
	if matched, ok := tokenMatchesAny(tokens, entry.ThoughtRecipe.Metadata.HandoffTargets...); ok {
		score += 40
		reasons = append(reasons, "handoff")
		_ = matched
	}
	if matched, ok := tokenMatchesAny(tokens, entry.ThoughtRecipe.Metadata.Tags...); ok {
		score += 10
		reasons = append(reasons, "tag")
		_ = matched
	}
	return score, reasons
}

func scoreCapabilityCandidate(desc descriptor.CapabilityDescriptor, tokens []string) (int, []string) {
	score := 0
	reasons := make([]string, 0, 4)
	if exactTokenMatch(tokens, desc.ID, desc.Name) {
		score += 100
		reasons = append(reasons, "explicit capability")
	}
	if _, ok := tokenMatchesAny(tokens, desc.Category); ok {
		score += 20
		reasons = append(reasons, "category")
	}
	if _, ok := tokenMatchesAny(tokens, desc.Tags...); ok {
		score += 30
		reasons = append(reasons, "tag")
	}
	for _, family := range []string{"query", "review", "repair", "test", "architecture", "migration", "debug"} {
		if _, ok := tokenMatchesAny(tokens, family); ok && familyMatchBonus(desc, family) {
			score += 20
			reasons = append(reasons, "family")
			break
		}
	}
	score += capabilityPriorityScore(desc)
	if _, ok := tokenMatchesAny(tokens, capabilityRiskTokens(desc)...); ok {
		score += 5
		reasons = append(reasons, "risk")
	}
	return score, reasons
}

func capabilityRiskTokens(desc descriptor.CapabilityDescriptor) []string {
	riskClasses := risk.Classify(desc.EffectClasses, desc.Source.Scope)
	if len(riskClasses) == 0 {
		return nil
	}
	out := make([]string, 0, len(riskClasses))
	for _, rc := range riskClasses {
		out = append(out, strings.TrimSpace(string(rc)))
	}
	return out
}

func capabilitySnapshotByID(caps *registry.CapabilityRegistry, id string) (registry.CapabilitySnapshot, bool) {
	if caps == nil {
		return registry.CapabilitySnapshot{}, false
	}
	for _, snapshot := range caps.AllCapabilitySnapshots() {
		if strings.TrimSpace(snapshot.Descriptor.ID) == strings.TrimSpace(id) {
			return snapshot, true
		}
	}
	return registry.CapabilitySnapshot{}, false
}

func tokenMatchesAny(tokens []string, values ...string) (string, bool) {
	normalized := make(map[string]struct{}, len(tokens))
	for _, token := range tokens {
		if trimmed := strings.TrimSpace(strings.ToLower(token)); trimmed != "" {
			normalized[trimmed] = struct{}{}
		}
		for _, split := range splitRouteTokens(token) {
			if split != "" {
				normalized[split] = struct{}{}
			}
		}
	}
	for _, value := range values {
		candidates := append([]string{value}, splitRouteTokens(value)...)
		for _, candidate := range candidates {
			trimmed := strings.TrimSpace(strings.ToLower(candidate))
			if trimmed == "" {
				continue
			}
			if _, ok := normalized[trimmed]; ok {
				return trimmed, true
			}
		}
	}
	return "", false
}

func exactTokenMatch(tokens []string, values ...string) bool {
	if len(tokens) == 0 || len(values) == 0 {
		return false
	}
	normalized := make(map[string]struct{}, len(tokens))
	for _, token := range tokens {
		if trimmed := strings.TrimSpace(strings.ToLower(token)); trimmed != "" {
			normalized[trimmed] = struct{}{}
		}
	}
	for _, value := range values {
		trimmed := strings.TrimSpace(strings.ToLower(value))
		if trimmed == "" {
			continue
		}
		if _, ok := normalized[trimmed]; ok {
			return true
		}
	}
	return false
}

func routeAvailabilityFromSnapshot(snapshot registry.CapabilitySnapshot) (RouteAvailability, string) {
	if snapshot.Exposure == agentspec.CapabilityExposureHidden {
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

func familyMatchBonus(desc descriptor.CapabilityDescriptor, family string) bool {
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
	case "migration":
		return strings.Contains(strings.ToLower(desc.ID), "api_compat")
	case "debug":
		return strings.Contains(strings.ToLower(desc.ID), "bisect")
	default:
		return strings.Contains(strings.ToLower(desc.Name), family) || strings.Contains(strings.ToLower(desc.Category), family) || strings.Contains(strings.ToLower(desc.ID), family)
	}
}

func routeMatchesFamily(desc descriptor.CapabilityDescriptor, family, instruction string) bool {
	family = strings.ToLower(strings.TrimSpace(family))
	if family == "" {
		return instructionMatchesRouteFamily(desc, instruction)
	}
	if familyMatchBonus(desc, family) {
		return true
	}
	instruction = strings.ToLower(instruction)
	return strings.Contains(strings.ToLower(desc.ID), family) || strings.Contains(strings.ToLower(desc.Name), family) || strings.Contains(strings.ToLower(desc.Category), family) || strings.Contains(instruction, family)
}

func instructionMatchesRouteFamily(desc descriptor.CapabilityDescriptor, instruction string) bool {
	instruction = strings.ToLower(strings.TrimSpace(instruction))
	if instruction == "" {
		return false
	}

	if instructionLooksAnalytical(instruction) {
		return familyMatchBonus(desc, "query")
	}
	if instructionLooksMutating(instruction) {
		return familyMatchBonus(desc, "repair")
	}

	return false
}

func instructionLooksAnalytical(instruction string) bool {
	analysisHints := []string{
		"analysis",
		"analyze",
		"analyse",
		"inspect",
		"investigate",
		"review",
		"lookup",
		"trace",
		"query",
		"debug",
		"diagnose",
	}
	for _, hint := range analysisHints {
		if strings.Contains(instruction, hint) {
			return true
		}
	}
	return false
}

func instructionLooksMutating(instruction string) bool {
	mutationHints := []string{
		"mutat",
		"modify",
		"edit",
		"change",
		"refactor",
		"rename",
		"implement",
		"patch",
		"fix",
		"update",
	}
	for _, hint := range mutationHints {
		if strings.Contains(instruction, hint) {
			return true
		}
	}
	return false
}

func capabilityPriorityScore(desc descriptor.CapabilityDescriptor) int {
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
	if euclotypes.IsThoughtRecipeRouteKind(candidate.RouteKind) || euclotypes.IsIntentRouteKind(candidate.RouteKind) {
		return "graph"
	}
	if candidate.Availability != RouteAvailable {
		return "blocked"
	}
	return "fast"
}

func taskID(env *contextdata.Envelope) string {
	return env.TaskID
}

func sessionID(env *contextdata.Envelope) string {
	return env.SessionID
}

func fallbackIDString(id *RouteID) string {
	if id == nil {
		return ""
	}
	return string(*id)
}

func primaryRouteID(req RouteRequest) string {
	if strings.TrimSpace(req.ThoughtRecipeID) != "" {
		return strings.TrimSpace(req.ThoughtRecipeID)
	}
	if strings.TrimSpace(req.CapabilityID) != "" {
		return strings.TrimSpace(req.CapabilityID)
	}
	return strings.TrimSpace(req.FallbackID)
}

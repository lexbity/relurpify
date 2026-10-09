package orchestrate

import (
	"context"
	"sort"
	"strings"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/descriptor"
	registry "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	"codeburg.org/lexbit/relurpify/named/euclo/families"
	intentcontext "codeburg.org/lexbit/relurpify/named/euclo/intentcontext"
	"codeburg.org/lexbit/relurpify/named/euclo/reporting"
	euclostate "codeburg.org/lexbit/relurpify/named/euclo/state"
	thoughtrecipepkg "codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
)

// SelectionDeps is the explicit dependency contract for deterministic route
// selection and bounded Tier-2 disambiguation (D8/D10).
type SelectionDeps struct {
	Capabilities *registry.CapabilityRegistry
	// ThoughtRecipes is the recipe registry whose entries form the recipe side
	// of the candidate pool.
	ThoughtRecipes *thoughtrecipepkg.ThoughtRecipeRegistry
	// Families is the keyword-family registry whose vocabulary feeds the
	// family-affinity and intent-keyword score components (D9).
	Families *families.KeywordFamilyRegistry
	// Tier2Model is the model consulted by the bounded disambiguator (D10).
	// Nil means Tier-2 is not configured: the gate records no attempt, and if
	// it would have fired the model is reported as unavailable.
	Tier2Model model.LanguageModel
}

// Dispatch resolves a route request and records route telemetry. The
// deterministic lattice selects first; the bounded Tier-2 gate may then
// disambiguate within that candidate set only (D8/D10).
func Dispatch(ctx context.Context, env *contextdata.Envelope, req RouteRequest, deps SelectionDeps) (*RouteResult, error) {
	report, selected, fallbackTaken, ok := resolveRoute(env, req, deps)
	if ok {
		selected, report.DecidedBy, report.Tier2 = applyTier2Gate(ctx, env, req, report, selected, report.DecidedBy, deps)
	}
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
		return nil, &RouteResolutionError{
			PrimaryID:       primaryRouteID(req),
			Reason:          unresolvedRouteReason(report, selected, resolution),
			MissingRecipeID: missingRecipeIDFromCandidates(report.Candidates),
		}
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
		return nil, &RouteResolutionError{
			PrimaryID:       string(selected.RouteID),
			Reason:          reason,
			MissingRecipeID: missingRecipeIDFromCandidates(report.Candidates),
		}
	}
	if !req.TelemetryOff {
		reporting.EmitRouteSelected(ctx, taskID(env), sessionID(env), req.FamilyID, result.RouteKind, result.RouteID, result.CandidateCount, result.FallbackTaken, result.DecidedBy)
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

// DryRun resolves a route request without executing it and returns the ranked
// candidate set. It shares the live dispatch's selection context and Tier-2
// gate, so preflight and execution select identically.
func DryRun(ctx context.Context, env *contextdata.Envelope, req RouteRequest, deps SelectionDeps) (*DryRunReport, error) {
	report, selected, fallbackTaken, ok := resolveRoute(env, req, deps)
	if ok {
		selected, report.DecidedBy, report.Tier2 = applyTier2Gate(ctx, env, req, report, selected, report.DecidedBy, deps)
	}
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
		return report, &RouteResolutionError{
			PrimaryID:       primaryRouteID(req),
			Reason:          unresolvedRouteReason(report, selected, resolution),
			MissingRecipeID: missingRecipeIDFromCandidates(report.Candidates),
		}
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
		DecidedBy:           report.DecidedBy,
		Tier2:               report.Tier2,
	}
	return result
}

// resolveRoute builds the deterministic candidate pool, applies the D8
// lattice, and applies the default-recipe degradation when no candidate
// scored above zero.
func resolveRoute(env *contextdata.Envelope, req RouteRequest, deps SelectionDeps) (*DryRunReport, CandidateRouteInfo, bool, bool) {
	report := &DryRunReport{Request: req}
	ctx := selectionContextFor(env, req, deps.Families)
	report.Candidates = deterministicRouteCandidates(env, req, deps, ctx)
	selected, ok, decidedBy := selectByLattice(req, report.Candidates)
	fallbackTaken := false
	if !ok && strings.TrimSpace(req.ThoughtRecipeID) == "" && strings.TrimSpace(req.CapabilityID) == "" && deps.ThoughtRecipes != nil {
		// No deterministic candidate is available (recipes absent, capability
		// matches suppressed). Fall back to the built-in default execution
		// recipe so general tasks still run through a paradigm instead of
		// failing route resolution.
		if candidate, okFallback := defaultExecutionRecipeCandidate(deps.ThoughtRecipes); okFallback {
			report.Candidates = append(report.Candidates, candidate)
			selected, ok, fallbackTaken, decidedBy = candidate, true, true, decidedByDefaultRecipe
		}
	}
	report.DecidedBy = decidedBy
	return report, selected, fallbackTaken, ok
}

// availableRecipeCandidate is the ONLY constructor of an
// Availability: RouteAvailable recipe candidate. Every code path that offers
// a thoughtrecipe route — explicit, clarification, metadata, and default
// fallback — must pass through it: an unregistered recipe is never offered,
// so selection can no longer succeed on a route that execution cannot run.
func availableRecipeCandidate(reg *thoughtrecipepkg.ThoughtRecipeRegistry, id string, kind string, score int, reasons []string) (CandidateRouteInfo, bool) {
	if reg == nil {
		return CandidateRouteInfo{}, false
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return CandidateRouteInfo{}, false
	}
	if _, ok := reg.Get(id); !ok {
		return CandidateRouteInfo{}, false
	}
	return CandidateRouteInfo{
		RouteID:      RouteID(id),
		RouteKind:    kind,
		Availability: RouteAvailable,
		RankScore:    score,
		RankReasons:  reasons,
	}, true
}

// defaultExecutionRecipeCandidate offers the built-in default execution
// thoughtrecipe when it is registered and no workspace recipe matched.
func defaultExecutionRecipeCandidate(reg *thoughtrecipepkg.ThoughtRecipeRegistry) (CandidateRouteInfo, bool) {
	id := defaultThoughtRecipeID
	candidate, ok := availableRecipeCandidate(reg, id,
		euclotypes.RouteKindForThoughtRecipeID(id), 0,
		[]string{"no deterministic route; falling back to default execution recipe"})
	if !ok {
		return CandidateRouteInfo{}, false
	}
	return candidate, true
}

func deterministicRouteCandidates(env *contextdata.Envelope, req RouteRequest, deps SelectionDeps, ctx selectionContext) []CandidateRouteInfo {
	clarificationCandidate := clarificationRouteCandidate(env, req, deps.ThoughtRecipes)
	if explicit := explicitRouteCandidate(req, deps); explicit != nil {
		if clarificationCandidate != nil && candidateRouteID(*explicit) == candidateRouteID(*clarificationCandidate) {
			return []CandidateRouteInfo{*explicit}
		}
		return []CandidateRouteInfo{*explicit}
	}

	candidates := make([]CandidateRouteInfo, 0, 8)
	if clarificationCandidate != nil {
		candidates = append(candidates, *clarificationCandidate)
	}
	candidates = append(candidates, metadataThoughtRecipeCandidates(deps.ThoughtRecipes, ctx)...)
	candidates = append(candidates, metadataCapabilityCandidates(req, deps.Capabilities, ctx)...)
	return dedupeAndSortRouteCandidates(candidates)
}

func explicitRouteCandidate(req RouteRequest, deps SelectionDeps) *CandidateRouteInfo {
	switch {
	case strings.TrimSpace(req.ThoughtRecipeID) != "":
		id := strings.TrimSpace(req.ThoughtRecipeID)
		if id == clarificationThoughtRecipeID {
			// The clarification interaction is built in: it is always
			// registered by graph construction (ensureClarificationThoughtRecipe).
			if candidate, ok := availableRecipeCandidate(deps.ThoughtRecipes, id,
				euclotypes.RouteKindIntent, scoreExplicit,
				[]string{"explicit clarification route"}); ok {
				return markExplicit(candidate)
			}
		}
		if deps.ThoughtRecipes != nil {
			if candidate, ok := availableRecipeCandidate(deps.ThoughtRecipes, id,
				euclotypes.RouteKindForThoughtRecipeID(id), scoreExplicit,
				[]string{"explicit thoughtrecipe"}); ok {
				return markExplicit(candidate)
			}
		}
		return markExplicit(CandidateRouteInfo{
			RouteID:        RouteID(id),
			RouteKind:      euclotypes.RouteKindForThoughtRecipeID(id),
			Availability:   RouteUnavailableUnsupported,
			RankReasons:    []string{"explicit thoughtrecipe not found"},
			SuppressReason: "explicit thoughtrecipe not found",
		})
	case strings.TrimSpace(req.CapabilityID) != "":
		id := strings.TrimSpace(req.CapabilityID)
		if deps.Capabilities != nil {
			if snapshot, ok := capabilitySnapshotByID(deps.Capabilities, id); ok {
				availability, reason := routeAvailabilityFromSnapshot(snapshot)
				return markExplicit(CandidateRouteInfo{
					RouteID:        RouteID(snapshot.Descriptor.ID),
					RouteKind:      euclotypes.RouteKindCapability,
					Availability:   availability,
					RankReasons:    []string{"explicit capability"},
					Suppressed:     availability == RouteUnavailablePolicyDenied,
					SuppressReason: reason,
				})
			}
		}
		return markExplicit(CandidateRouteInfo{
			RouteID:        RouteID(id),
			RouteKind:      euclotypes.RouteKindCapability,
			Availability:   RouteUnavailableUnsupported,
			RankReasons:    []string{"explicit capability not found"},
			SuppressReason: "explicit capability not found",
		})
	default:
		return nil
	}
}

// markExplicit stamps the rank-1 explicit component onto a candidate and
// returns it, so the selection record shows why the explicit route dominated.
func markExplicit(candidate CandidateRouteInfo) *CandidateRouteInfo {
	candidate.RankScore = scoreExplicit
	if candidate.Components == nil {
		candidate.Components = make(map[string]int, 1)
	}
	candidate.Components[compExplicit] = scoreExplicit
	return &candidate
}

func clarificationRouteCandidate(env *contextdata.Envelope, req RouteRequest, thoughtrecipes *thoughtrecipepkg.ThoughtRecipeRegistry) *CandidateRouteInfo {
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
	candidate, ok := availableRecipeCandidate(thoughtrecipes, clarificationThoughtRecipeID,
		euclotypes.RouteKindIntent, 900,
		[]string{"clarification route"})
	if !ok {
		return nil
	}
	return &candidate
}

func metadataThoughtRecipeCandidates(thoughtrecipes *thoughtrecipepkg.ThoughtRecipeRegistry, ctx selectionContext) []CandidateRouteInfo {
	if thoughtrecipes == nil {
		return nil
	}
	tokens := ctx.tokens
	if len(tokens) == 0 {
		return nil
	}
	candidates := make([]CandidateRouteInfo, 0)
	for _, entry := range thoughtrecipes.Entries() {
		routeID := routeIDForThoughtRecipeEntry(entry)
		if routeID == "" {
			continue
		}
		s := scoreThoughtRecipeCandidate(entry, ctx)
		if s.total <= 0 {
			continue
		}
		candidate, ok := availableRecipeCandidate(thoughtrecipes, routeID,
			euclotypes.RouteKindForThoughtRecipeID(routeID), s.total, s.reasons)
		if !ok {
			continue
		}
		candidate.Components = s.components
		candidate.MatchedKeywords = s.matchedKeywords
		candidate.Description = strings.TrimSpace(entry.ThoughtRecipe.Description)
		candidates = append(candidates, candidate)
	}
	return candidates
}

func metadataCapabilityCandidates(req RouteRequest, caps *registry.CapabilityRegistry, ctx selectionContext) []CandidateRouteInfo {
	if caps == nil {
		return nil
	}
	tokens := ctx.tokens
	snapshots := caps.AllCapabilitySnapshots()
	if len(tokens) == 0 {
		if strings.TrimSpace(req.SkillFilter) == "" {
			return nil
		}
		candidates := make([]CandidateRouteInfo, 0, len(snapshots))
		for _, snapshot := range snapshots {
			availability, reason := routeAvailabilityFromSnapshot(snapshot)
			priority := capabilityPriorityScore(snapshot.Descriptor)
			candidates = append(candidates, CandidateRouteInfo{
				RouteID:        RouteID(snapshot.Descriptor.ID),
				RouteKind:      euclotypes.RouteKindCapability,
				Availability:   availability,
				RankScore:      priority + availabilityScore(availability),
				Components:     map[string]int{compPriority: priority},
				Description:    capabilityDescription(snapshot.Descriptor),
				Suppressed:     availability == RouteUnavailablePolicyDenied,
				SuppressReason: reason,
			})
		}
		return dedupeAndSortRouteCandidates(candidates)
	}
	candidates := make([]CandidateRouteInfo, 0, len(snapshots))
	for _, snapshot := range snapshots {
		s := scoreCapabilityCandidate(snapshot.Descriptor, ctx)
		if s.total <= 0 {
			continue
		}
		availability, reason := routeAvailabilityFromSnapshot(snapshot)
		if availability == RouteAvailable && capabilityRequiresArgs(snapshot.Descriptor) {
			// Direct capability routes carry no argument source: a keyword
			// match on a capability with required inputs would fail schema
			// validation at invocation time. Suppress in favor of
			// paradigm-backed routes that synthesize arguments.
			availability = RouteUnavailableUnsupported
			reason = "capability requires arguments; no argument source for direct capability routes"
		}
		candidates = append(candidates, CandidateRouteInfo{
			RouteID:         RouteID(snapshot.Descriptor.ID),
			RouteKind:       euclotypes.RouteKindCapability,
			Availability:    availability,
			RankScore:       s.total,
			RankReasons:     s.reasons,
			Components:      s.components,
			MatchedKeywords: s.matchedKeywords,
			Description:     capabilityDescription(snapshot.Descriptor),
			Suppressed:      availability == RouteUnavailablePolicyDenied,
			SuppressReason:  reason,
		})
	}
	return candidates
}

// capabilityDescription renders the one-line candidate description used by the
// Tier-2 prompt and the selection record.
func capabilityDescription(desc descriptor.CapabilityDescriptor) string {
	parts := make([]string, 0, 3)
	if name := strings.TrimSpace(desc.Name); name != "" {
		parts = append(parts, name)
	}
	if category := strings.TrimSpace(desc.Category); category != "" {
		parts = append(parts, "category="+category)
	}
	if description := strings.TrimSpace(desc.Description); description != "" {
		parts = append(parts, description)
	}
	return strings.Join(parts, "; ")
}

// capabilityRequiresArgs reports whether the capability declares required
// input fields in its input schema.
func capabilityRequiresArgs(desc descriptor.CapabilityDescriptor) bool {
	if desc.InputSchema == nil {
		return false
	}
	return len(desc.InputSchema.Required) > 0
}

// selectByLattice is the D8 selection lattice: a pure, deterministic total
// order over the candidate pool, with the rule that decided recorded per rank:
//
//	1 explicit request (selected iff registered+available)
//	2 highest deterministic score
//	3 tie → higher family-affinity component
//	4 tie → non-builtin candidate (user recipes shadow builtins)
//	5 tie → thoughtrecipe before capability (governed decomposition over
//	       immediate host effect; deliberate reversal of the old
//	       lexicographic kind order)
//	6 tie → lexicographic RouteID (total order; determinism)
func selectByLattice(req RouteRequest, candidates []CandidateRouteInfo) (CandidateRouteInfo, bool, string) {
	if len(candidates) == 0 {
		return CandidateRouteInfo{}, false, ""
	}

	// Rank 1: an explicit request names the route; it is selected iff it is
	// available. An unavailable explicit route is a RouteResolutionError, never
	// a fallback (D8).
	if strings.TrimSpace(req.ThoughtRecipeID) != "" || strings.TrimSpace(req.CapabilityID) != "" {
		for _, candidate := range candidates {
			if candidateMatchesRequest(candidate, req) {
				return candidate, candidate.Availability == RouteAvailable, decidedByExplicit
			}
		}
		return CandidateRouteInfo{}, false, decidedByExplicit
	}

	var available []CandidateRouteInfo
	for _, candidate := range candidates {
		if candidate.Availability == RouteAvailable {
			available = append(available, candidate)
		}
	}
	if len(available) == 0 {
		return CandidateRouteInfo{}, false, ""
	}

	// Rank 2: the highest deterministic score.
	bestScore := -1
	for _, candidate := range available {
		if candidate.RankScore > bestScore {
			bestScore = candidate.RankScore
		}
	}
	top := scoreFilter(available, bestScore)
	if len(top) == 1 {
		return top[0], true, decidedByScore
	}

	// Rank 3: tie on score → higher family-affinity component.
	bestAffinity := -1
	for _, candidate := range top {
		if affinity := candidate.Components[compFamilyAffinity]; affinity > bestAffinity {
			bestAffinity = affinity
		}
	}
	affinity := affinityFilter(top, bestAffinity)
	if len(affinity) == 1 {
		return affinity[0], true, decidedByFamilyAffinity
	}

	// Rank 4: tie → user recipes shadow builtin recipes. Scoped to recipe-kind
	// candidates so a capability can never use this rank to outrank a recipe
	// (rank 5 owns kind precedence).
	userShadowed := dropShadowedBuiltins(affinity)
	if len(userShadowed) == 1 {
		return userShadowed[0], true, decidedByUserOverBuiltin
	}

	// Rank 5: tie → thoughtrecipe before capability.
	preferred := kindFilter(userShadowed)
	if len(preferred) == 1 {
		return preferred[0], true, decidedByRecipeOverCap
	}

	// Rank 6: tie → lexicographic RouteID (total order; determinism).
	sort.SliceStable(preferred, func(i, j int) bool {
		return preferred[i].RouteID < preferred[j].RouteID
	})
	return preferred[0], true, decidedByRouteID
}

// scoreFilter keeps the candidates whose RankScore equals best.
func scoreFilter(candidates []CandidateRouteInfo, best int) []CandidateRouteInfo {
	out := make([]CandidateRouteInfo, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.RankScore == best {
			out = append(out, candidate)
		}
	}
	return out
}

// affinityFilter keeps the candidates whose family-affinity component equals
// best.
func affinityFilter(candidates []CandidateRouteInfo, best int) []CandidateRouteInfo {
	if best <= 0 {
		return candidates
	}
	out := make([]CandidateRouteInfo, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Components[compFamilyAffinity] == best {
			out = append(out, candidate)
		}
	}
	return out
}

// dropShadowedBuiltins applies the D8 rank-4 rule: when the survivors include
// both a user thoughtrecipe and a builtin thoughtrecipe, the builtin recipes
// are shadowed and dropped. Capabilities never participate: only a genuine
// user recipe can shadow a builtin recipe.
func dropShadowedBuiltins(candidates []CandidateRouteInfo) []CandidateRouteInfo {
	userRecipeCount := 0
	builtinRecipeCount := 0
	for _, candidate := range candidates {
		if !isThoughtRecipeKind(candidate.RouteKind) {
			continue
		}
		if isBuiltinRoute(candidate.RouteID) {
			builtinRecipeCount++
		} else {
			userRecipeCount++
		}
	}
	if userRecipeCount == 0 || builtinRecipeCount == 0 {
		return candidates
	}
	out := make([]CandidateRouteInfo, 0, len(candidates))
	for _, candidate := range candidates {
		if isThoughtRecipeKind(candidate.RouteKind) && isBuiltinRoute(candidate.RouteID) {
			continue
		}
		out = append(out, candidate)
	}
	return out
}

// isThoughtRecipeKind reports whether a route kind is governed decomposition
// (thoughtrecipe or intent).
func isThoughtRecipeKind(kind string) bool {
	switch kind {
	case euclotypes.RouteKindThoughtRecipe, euclotypes.RouteKindIntent:
		return true
	default:
		return false
	}
}

// kindFilter prefers governed decomposition (thoughtrecipe/intent) over direct
// host effects (capability) at equal evidence (D8 rank 5).
func kindFilter(candidates []CandidateRouteInfo) []CandidateRouteInfo {
	out := make([]CandidateRouteInfo, 0, len(candidates))
	for _, candidate := range candidates {
		if isThoughtRecipeKind(candidate.RouteKind) {
			out = append(out, candidate)
		}
	}
	if len(out) > 0 {
		return out
	}
	return candidates
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
		existing, ok := byID[id]
		if !ok {
			byID[id] = candidate
			continue
		}
		// D8: duplicate IDs from multiple producer functions keep the
		// higher-scored variant and the union of reasons (and the per-component
		// evidence of both), so dedup never loses a reason or a matched
		// keyword.
		if candidate.RankScore > existing.RankScore {
			candidate.RankReasons = unionStrings(existing.RankReasons, candidate.RankReasons)
			candidate.Components = mergeComponents(existing.Components, candidate.Components)
			byID[id] = candidate
			continue
		}
		existing.RankReasons = unionStrings(existing.RankReasons, candidate.RankReasons)
		existing.Components = mergeComponents(existing.Components, candidate.Components)
		byID[id] = existing
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

// unionStrings merges two reason lists preserving order and de-duplicating by
// exact value.
func unionStrings(a, b []string) []string {
	if len(a) == 0 {
		return append([]string(nil), b...)
	}
	if len(b) == 0 {
		return append([]string(nil), a...)
	}
	seen := make(map[string]struct{}, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	for _, value := range a {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	for _, value := range b {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

// mergeComponents unions two component maps taking the maximum per component.
func mergeComponents(a, b map[string]int) map[string]int {
	if len(a) == 0 {
		return b
	}
	if len(b) == 0 {
		return a
	}
	out := make(map[string]int, len(a)+len(b))
	for key, value := range a {
		out[key] = value
	}
	for key, value := range b {
		if current, ok := out[key]; !ok || value > current {
			out[key] = value
		}
	}
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
		DecidedBy:        "",
	}
	if report != nil {
		resolution.DecidedBy = strings.TrimSpace(report.DecidedBy)
		resolution.Tier2 = report.Tier2
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

// missingRecipeIDFromCandidates names the thoughtrecipe the gate rejected
// when every unavailable candidate was rejected for not being registered.
// Empty when the failure is not a missing-recipe failure.
func missingRecipeIDFromCandidates(candidates []CandidateRouteInfo) string {
	missing := ""
	for _, candidate := range candidates {
		if candidate.Availability == RouteAvailable {
			return ""
		}
		if candidate.RouteKind == euclotypes.RouteKindCapability {
			continue
		}
		if candidate.SuppressReason != "explicit thoughtrecipe not found" &&
			candidate.SuppressReason != "thoughtrecipe not registered" {
			continue
		}
		if missing != "" && missing != string(candidate.RouteID) {
			return ""
		}
		missing = string(candidate.RouteID)
	}
	return missing
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

// selectionContext scopes deterministic scoring: the classified family, the
// utterance search tokens, and the family registry whose vocabulary feeds the
// family-affinity and intent-keyword score components (D9: the family system
// is a score component, not a parallel universe).
type selectionContext struct {
	family string
	tokens []string
	famReg *families.KeywordFamilyRegistry
}

// selectionContextFor builds the scoring context for a dispatch. The family
// is the classified family carried by intake; the accessor is formalized here
// (falling back from the request to the envelope's classification so resume
// and direct-call paths see the same family evidence).
func selectionContextFor(env *contextdata.Envelope, req RouteRequest, famReg *families.KeywordFamilyRegistry) selectionContext {
	return selectionContext{
		family: resolvedFamilyID(env, req),
		tokens: routeSearchTokens(env, req),
		famReg: famReg,
	}
}

// resolvedFamilyID returns the classified family ID: the request carries it
// when dispatch runs after intake; otherwise the envelope's classification
// records it (the same evidence on resume/direct paths).
func resolvedFamilyID(env *contextdata.Envelope, req RouteRequest) string {
	if family := strings.TrimSpace(req.FamilyID); family != "" {
		return family
	}
	if env == nil {
		return ""
	}
	if family, ok := euclostate.GetFamilySelection(env); ok {
		if trimmed := strings.TrimSpace(family); trimmed != "" {
			return trimmed
		}
	}
	if classification, ok := euclostate.GetIntentClassification(env); ok && classification != nil {
		return strings.TrimSpace(classification.WinningFamily)
	}
	return ""
}

// intentKeywordHits computes the §3.6.1 intent-keyword component: the
// classified family's IntentKeywords that are shared by the utterance and the
// candidate's own vocabulary. The candidate-side overlap keeps a family-intent
// term from promoting a candidate with no other evidence, and the pool-wide
// term cannot manufacture a candidate. +5 each, capped at 15.
func intentKeywordHits(ctx selectionContext, candidateVocabulary ...string) int {
	if ctx.famReg == nil || ctx.family == "" {
		return 0
	}
	family, ok := ctx.famReg.Lookup(ctx.family)
	if !ok {
		return 0
	}
	intentSet := tokenNormalizedSet(family.IntentKeywords)
	if len(intentSet) == 0 {
		return 0
	}
	candidateSet := tokenNormalizedSet(candidateVocabulary)
	if len(candidateSet) == 0 {
		return 0
	}
	utteranceSet := tokenNormalizedSet(ctx.tokens)
	hits := 0
	for token := range intentSet {
		if _, inCandidate := candidateSet[token]; !inCandidate {
			continue
		}
		if _, inUtterance := utteranceSet[token]; inUtterance {
			hits++
		}
	}
	return boundedComponent(hits, scorePerIntentKeyword, capIntentKeyword)
}

// candidateScore is the deterministic evidence a scorer produced for one
// candidate: the capped total, the per-component split, the reason codes, and
// the matched candidate-side vocabulary tokens (public registry data).
type candidateScore struct {
	total           int
	components      map[string]int
	reasons         []string
	matchedKeywords []string
}

func scoreThoughtRecipeCandidate(entry thoughtrecipepkg.ThoughtRecipeEntry, ctx selectionContext) candidateScore {
	score := candidateScore{components: make(map[string]int), reasons: make([]string, 0, 6)}
	if entry.ThoughtRecipe == nil {
		return score
	}
	addComponent := func(name string, value int, reason string) {
		if value <= 0 {
			return
		}
		score.components[name] = value
		score.total += value
		score.reasons = append(score.reasons, reason)
	}

	// Keyword hit: the utterance names the recipe (its declared keyword
	// vocabulary plus its name and ID) (+10 each, cap 30).
	keywordVocabulary := append([]string(nil), entry.ThoughtRecipe.Metadata.Keywords...)
	keywordVocabulary = append(keywordVocabulary, entry.ThoughtRecipe.Name, entry.ThoughtRecipe.ID, entry.Name)
	keywordMatches := tokenIntersectionTokens(ctx.tokens, keywordVocabulary)
	addComponent(compKeyword, boundedComponent(len(keywordMatches), scorePerKeyword, capKeyword), "keyword")
	score.matchedKeywords = append(score.matchedKeywords, keywordMatches...)

	// Handoff-context match (+100, cap 100): the utterance names a handoff
	// target of the recipe.
	handoff := tokenIntersectionCount(ctx.tokens, entry.ThoughtRecipe.Metadata.HandoffTargets)
	if handoff > 0 {
		addComponent(compHandoff, boundedComponent(1, scoreHandoff, capHandoff), "handoff")
	}

	// Family affinity (+50): the recipe declares the classified family.
	if ctx.family != "" && stringSliceContainsFold(entry.ThoughtRecipe.Metadata.Families, ctx.family) {
		addComponent(compFamilyAffinity, boundedComponent(1, scoreFamilyAffinity, capFamilyAffinity), "family_affinity")
		score.matchedKeywords = append(score.matchedKeywords, ctx.family)
	}

	// Intent-keyword evidence: family intent vocabulary shared by the utterance
	// and the recipe.
	addComponent(compIntentKeyword, intentKeywordHits(ctx, keywordVocabulary...), "intent_keyword")

	// Description-token overlap (+1 each, cap 10): shared vocabulary between the
	// utterance and the recipe description.
	descriptionMatches := tokenIntersectionTokens(ctx.tokens, []string{entry.ThoughtRecipe.Description})
	addComponent(compDescription, boundedComponent(len(descriptionMatches), scorePerDescription, capDescription), "description")
	score.matchedKeywords = append(score.matchedKeywords, descriptionMatches...)

	score.matchedKeywords = normalizedKeywords(score.matchedKeywords)
	return score
}

func scoreCapabilityCandidate(desc descriptor.CapabilityDescriptor, ctx selectionContext) candidateScore {
	score := candidateScore{components: make(map[string]int), reasons: make([]string, 0, 6)}
	addComponent := func(name string, value int, reason string) {
		if value <= 0 {
			return
		}
		score.components[name] = value
		score.total += value
		score.reasons = append(score.reasons, reason)
	}

	// Keyword hit: the utterance matches the capability vocabulary — its name,
	// ID, tags, and the family names it belongs to (+10 each, cap 30).
	keywordVocabulary := append([]string(nil), desc.Tags...)
	keywordVocabulary = append(keywordVocabulary, desc.Name, desc.ID)
	for _, family := range capabilityFamilyVocabulary {
		if familyMatchBonus(desc, family) {
			keywordVocabulary = append(keywordVocabulary, family)
		}
	}
	keywordMatches := tokenIntersectionTokens(ctx.tokens, keywordVocabulary)
	addComponent(compKeyword, boundedComponent(len(keywordMatches), scorePerKeyword, capKeyword), "keyword")
	score.matchedKeywords = append(score.matchedKeywords, keywordMatches...)

	// Family affinity (+50): the capability belongs to the classified family.
	if ctx.family != "" && familyMatchBonus(desc, ctx.family) {
		addComponent(compFamilyAffinity, boundedComponent(1, scoreFamilyAffinity, capFamilyAffinity), "family_affinity")
		score.matchedKeywords = append(score.matchedKeywords, ctx.family)
	}

	// Intent-keyword evidence: family intent vocabulary shared by the utterance
	// and the capability.
	addComponent(compIntentKeyword, intentKeywordHits(ctx, keywordVocabulary...), "intent_keyword")

	// Description-token overlap (+1 each, cap 10): shared vocabulary between
	// the utterance and the capability name/category/description.
	descriptionMatches := append([]string(nil),
		tokenIntersectionTokens(ctx.tokens, []string{desc.Name})...)
	descriptionMatches = append(descriptionMatches, tokenIntersectionTokens(ctx.tokens, []string{desc.Category})...)
	descriptionMatches = append(descriptionMatches, tokenIntersectionTokens(ctx.tokens, []string{desc.Description})...)
	addComponent(compDescription, boundedComponent(len(descriptionMatches), scorePerDescription, capDescription), "description")
	score.matchedKeywords = append(score.matchedKeywords, descriptionMatches...)

	// Priority: the shipped euclo.priority annotation (shipped data, not a
	// config surface) contributes directly to the deterministic score.
	priority := capabilityPriorityScore(desc)
	if priority > 0 {
		addComponent(compPriority, priority, "priority")
	}

	score.matchedKeywords = normalizedKeywords(score.matchedKeywords)
	return score
}

// capabilityFamilyVocabulary is the closed family vocabulary used to map
// capability IDs onto families (the capability-side equivalent of a recipe's
// `family [...]` declaration).
var capabilityFamilyVocabulary = []string{ //nolint:gochecknoglobals // immutable family vocabulary
	"query", "review", "repair", "test", "architecture", "migration", "debug",
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

// tokenNormalizedSet expands the utterance tokens into the normalized match
// vocabulary used by every scorer: the trimmed lowercased token plus its
// split sub-tokens.
func tokenNormalizedSet(tokens []string) map[string]struct{} {
	normalized := make(map[string]struct{}, len(tokens)*2)
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
	return normalized
}

// tokenIntersectionTokens returns the distinct normalized tokens shared by two
// token lists, sorted for determinism. Both lists are expanded into their split
// sub-tokens, so a candidate whose name is also its ID counts once.
func tokenIntersectionTokens(a, b []string) []string {
	if len(a) == 0 || len(b) == 0 {
		return nil
	}
	setA := tokenNormalizedSet(a)
	setB := tokenNormalizedSet(b)
	if len(setB) < len(setA) {
		setA, setB = setB, setA
	}
	shared := make([]string, 0, len(setA))
	for token := range setA {
		if _, ok := setB[token]; ok {
			shared = append(shared, token)
		}
	}
	sort.Strings(shared)
	return shared
}

// tokenIntersectionCount counts the distinct normalized tokens shared by two
// token lists.
func tokenIntersectionCount(a, b []string) int {
	return len(tokenIntersectionTokens(a, b))
}

// normalizedKeywords lowercases, trims, de-duplicates, and sorts a matched
// keyword list so the record is deterministic and whitespace-free.
func normalizedKeywords(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.ToLower(strings.TrimSpace(value))
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	sort.Strings(out)
	return out
}

// stringSliceContainsFold reports whether value matches any element of values
// case-insensitively.
func stringSliceContainsFold(values []string, value string) bool {
	want := strings.ToLower(strings.TrimSpace(value))
	if want == "" {
		return false
	}
	for _, existing := range values {
		if strings.ToLower(strings.TrimSpace(existing)) == want {
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
	return env.TaskIDSnapshot()
}

func sessionID(env *contextdata.Envelope) string {
	return env.SessionIDSnapshot()
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

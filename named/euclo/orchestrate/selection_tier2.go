package orchestrate

import (
	"context"
	"sort"
	"strings"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	"codeburg.org/lexbit/relurpify/named/euclo/intake"
	"codeburg.org/lexbit/relurpify/named/euclo/reporting"
)

// applyTier2Gate consults the bounded Tier-2 disambiguator only when the
// deterministic winner is weak or tied (D10). It returns the (possibly
// changed) selection, the deciding rule, and the recorded Tier2Info.
//
// The deterministic winner stands whenever the model is absent, errors, returns
// an out-of-set id, or answers below the confidence floor. An unavailable
// outcome emits route.tier2_unavailable; selection is never blocked on the
// model.
func applyTier2Gate(ctx context.Context, env *contextdata.Envelope, req RouteRequest, report *DryRunReport, selected CandidateRouteInfo, decidedBy string, deps SelectionDeps) (CandidateRouteInfo, string, euclotypes.Tier2Info) {
	if !needTier2(report) {
		return selected, decidedBy, euclotypes.Tier2Info{}
	}
	ranked := rankAvailableCandidates(report.Candidates)
	refs := topCandidateRefs(ranked, tier2TopK)
	requestedRouteID := string(selected.RouteID)

	outcome, err := intake.DisambiguateWithLLM(
		ctx,
		deps.Tier2Model,
		utteranceForGate(env, req),
		report.Request.FamilyID,
		streamedContextForGate(env),
		refs,
	)
	info := euclotypes.Tier2Info{
		Used:        true,
		Outcome:     outcome.Outcome,
		Model:       outcome.Model,
		CandidateID: outcome.CandidateID,
		Confidence:  outcome.Confidence,
		LatencyMs:   outcome.Latency.Milliseconds(),
	}
	if err != nil {
		info.Outcome = intake.Tier2OutcomeUnavailable
		if !req.TelemetryOff {
			task, session := "", ""
			if env != nil {
				task, session = taskID(env), sessionID(env)
			}
			reporting.EmitRouteTier2Unavailable(ctx, task, session, requestedRouteID, err.Error())
		}
		return selected, decidedBy, info
	}
	if outcome.Outcome != intake.Tier2OutcomeApplied {
		return selected, decidedBy, info
	}
	if outcome.Confidence < tier2ConfidenceFloor {
		info.Outcome = intake.Tier2OutcomeLowConfidence
		return selected, decidedBy, info
	}
	adopted, ok := findAvailableCandidate(report.Candidates, outcome.CandidateID)
	if !ok {
		// Defensive: the disambiguator already restricted the id to the ref
		// set, but a non-available candidate must never be adopted.
		info.Outcome = intake.Tier2OutcomeRejected
		return selected, decidedBy, info
	}
	info.Outcome = intake.Tier2OutcomeApplied
	return adopted, decidedByTier2, info
}

// needTier2 reports whether the deterministic winner is weak or tied enough to
// warrant bounded disambiguation (D10). Explicit requests and the default-recipe
// degradation never consult the model.
func needTier2(report *DryRunReport) bool {
	if report == nil {
		return false
	}
	if strings.TrimSpace(report.Request.ThoughtRecipeID) != "" || strings.TrimSpace(report.Request.CapabilityID) != "" {
		return false
	}
	if report.DecidedBy == decidedByDefaultRecipe {
		return false
	}
	ranked := rankAvailableCandidates(report.Candidates)
	if len(ranked) == 0 {
		return false
	}
	top1 := ranked[0].RankScore
	if top1 < strongMatchFloor {
		return true
	}
	return len(ranked) >= 2 && top1-ranked[1].RankScore <= tieBand
}

// rankAvailableCandidates returns the available candidates in the D8 total
// order (score desc, family-affinity desc, governed decomposition before direct
// capability, user recipe before builtin recipe, then route id). The order is a
// deterministic total order used to pick the Tier-2 top-K set.
func rankAvailableCandidates(candidates []CandidateRouteInfo) []CandidateRouteInfo {
	out := make([]CandidateRouteInfo, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Availability == RouteAvailable {
			out = append(out, candidate)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.RankScore != b.RankScore {
			return a.RankScore > b.RankScore
		}
		if left, right := a.Components[compFamilyAffinity], b.Components[compFamilyAffinity]; left != right {
			return left > right
		}
		if left, right := isThoughtRecipeKind(a.RouteKind), isThoughtRecipeKind(b.RouteKind); left != right {
			return left
		}
		if left, right := isBuiltinRoute(a.RouteID), isBuiltinRoute(b.RouteID); left != right {
			return !left
		}
		return a.RouteID < b.RouteID
	})
	return out
}

// topCandidateRefs renders the top-K available candidates as disambiguator refs.
// A candidate outside this set can never appear in the prompt (D10).
func topCandidateRefs(ranked []CandidateRouteInfo, k int) []intake.CandidateRef {
	if k <= 0 || len(ranked) == 0 {
		return nil
	}
	if len(ranked) > k {
		ranked = ranked[:k]
	}
	refs := make([]intake.CandidateRef, 0, len(ranked))
	for _, candidate := range ranked {
		refs = append(refs, intake.CandidateRef{
			ID:              string(candidate.RouteID),
			Kind:            candidate.RouteKind,
			Description:     candidate.Description,
			MatchedKeywords: append([]string(nil), candidate.MatchedKeywords...),
		})
	}
	return refs
}

// findAvailableCandidate returns the available candidate with the given id.
func findAvailableCandidate(candidates []CandidateRouteInfo, id string) (CandidateRouteInfo, bool) {
	id = strings.TrimSpace(id)
	for _, candidate := range candidates {
		if candidateRouteID(candidate) == id && candidate.Availability == RouteAvailable {
			return candidate, true
		}
	}
	return CandidateRouteInfo{}, false
}

// utteranceForGate reads the task instruction from the task envelope, falling
// back to the request's instruction.
func utteranceForGate(env *contextdata.Envelope, req RouteRequest) string {
	if instruction := strings.TrimSpace(instructionFromEnvelope(env)); instruction != "" {
		return instruction
	}
	return strings.TrimSpace(req.Instruction)
}

// streamedContextForGate summarizes the run's streamed context references as
// public chunk ids. It never carries raw user text.
func streamedContextForGate(env *contextdata.Envelope) string {
	if env == nil {
		return ""
	}
	ids := env.StreamedChunkIDs()
	if len(ids) == 0 {
		return ""
	}
	out := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		trimmed := strings.TrimSpace(string(id))
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
		if len(out) >= 16 {
			break
		}
	}
	return strings.Join(out, ",")
}

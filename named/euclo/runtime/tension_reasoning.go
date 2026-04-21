package runtime

import (
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/core"
)

func enrichCoherenceReasoning(bundle SemanticInputBundle, state *core.Context, work UnitOfWork, issues []DeferredExecutionIssue) SemanticInputBundle {
	bundle.TensionClusters = dedupeTensionClusters(buildTensionClusters(bundle))
	bundle.CoherenceSuggestions = dedupeCoherenceSuggestions(buildCoherenceSuggestions(bundle, state, work, issues))
	return bundle
}

func buildTensionClusters(bundle SemanticInputBundle) []TensionClusterSummary {
	out := make([]TensionClusterSummary, 0, len(bundle.TensionFindings))
	for idx, finding := range bundle.TensionFindings {
		tensionRefs := uniqueStrings(append([]string{strings.TrimSpace(finding.RefID)}, filterRefsByPrefix(finding.RelatedRefs, "tension")...))
		if len(tensionRefs) == 0 && len(bundle.TensionRefs) > 0 {
			tensionRefs = firstNNonEmpty(bundle.TensionRefs, 4)
		}
		if len(tensionRefs) == 0 {
			continue
		}
		clusterID := strings.TrimSpace(finding.RefID)
		if clusterID == "" {
			clusterID = fmt.Sprintf("tension-cluster-%d", idx+1)
		}
		patternRefs := uniqueStrings(filterPatternRefs(finding.RelatedRefs))
		if len(patternRefs) == 0 {
			patternRefs = firstNNonEmpty(bundle.PatternRefs, 4)
		}
		out = append(out, TensionClusterSummary{
			ClusterID:          clusterID,
			Title:              nonEmpty(strings.TrimSpace(finding.Title), "Tension cluster"),
			Summary:            nonEmpty(strings.TrimSpace(finding.Summary), "Tension evidence remains relevant to execution decisions."),
			Severity:           tensionSeverityForFinding(finding),
			TensionRefs:        tensionRefs,
			PatternRefs:        patternRefs,
			ProvenanceRefs:     firstNNonEmpty(bundle.ProvenanceRefs, 4),
			RelatedRequestRefs: firstNNonEmpty(bundle.RequestProvenanceRefs, 3),
		})
	}
	if len(out) == 0 && len(bundle.TensionRefs) > 0 {
		out = append(out, TensionClusterSummary{
			ClusterID:          "tension-cluster-active-context",
			Title:              "Active tension cluster",
			Summary:            "Known tensions remain attached to the current execution context.",
			Severity:           "medium",
			TensionRefs:        firstNNonEmpty(bundle.TensionRefs, 4),
			PatternRefs:        firstNNonEmpty(bundle.PatternRefs, 4),
			ProvenanceRefs:     firstNNonEmpty(bundle.ProvenanceRefs, 4),
			RelatedRequestRefs: firstNNonEmpty(bundle.RequestProvenanceRefs, 3),
		})
	}
	return out
}

func buildCoherenceSuggestions(bundle SemanticInputBundle, state *core.Context, work UnitOfWork, issues []DeferredExecutionIssue) []CoherenceSuggestion {
	touched := touchedSymbolsFromState(state)
	out := make([]CoherenceSuggestion, 0, 4)
	if len(touched) > 0 && (len(bundle.PatternRefs) > 0 || len(bundle.TensionRefs) > 0) {
		out = append(out, CoherenceSuggestion{
			SuggestionID:        "coherence-touched-symbols",
			Title:               "Recheck touched symbols against active semantics",
			Summary:             "Touched paths intersect a pattern- and tension-aware execution context.",
			SuggestedAction:     "re-verify the changed paths against active pattern and tension evidence before accepting the run",
			TouchedSymbols:      touched,
			PatternRefs:         firstNNonEmpty(bundle.PatternRefs, 4),
			TensionRefs:         firstNNonEmpty(bundle.TensionRefs, 4),
			RelevantRequestRefs: firstNNonEmpty(bundle.RequestProvenanceRefs, 3),
		})
	}
	if work.PlanBinding != nil && strings.TrimSpace(work.PlanBinding.ActiveStepID) != "" && (len(bundle.PendingRequests) > 0 || len(bundle.LearningFindings) > 0) {
		out = append(out, CoherenceSuggestion{
			SuggestionID:        "coherence-step-boundary",
			Title:               "Check stale assumptions at the active step boundary",
			Summary:             "Pending semantic work suggests the active step may rely on assumptions that need revalidation.",
			SuggestedAction:     "review pending semantic requests and learning before widening the current step",
			TouchedSymbols:      touched,
			PatternRefs:         firstNNonEmpty(bundle.PatternRefs, 3),
			TensionRefs:         firstNNonEmpty(bundle.TensionRefs, 3),
			RelevantRequestRefs: requestIDs(bundle.PendingRequests),
		})
	}
	if len(bundle.PatternProposals) > 0 && len(bundle.ProspectivePairings) > 0 {
		out = append(out, CoherenceSuggestion{
			SuggestionID:        "coherence-scope-expansion",
			Title:               "Review scope expansion before adopting prospective paths",
			Summary:             "Prospective analysis is available while pattern proposals remain active, which can signal scope growth.",
			SuggestedAction:     "use the prospective pairings to compare alternatives before expanding the current execution scope",
			PatternRefs:         firstNNonEmpty(bundle.PatternRefs, 4),
			TensionRefs:         firstNNonEmpty(bundle.TensionRefs, 3),
			RelevantRequestRefs: firstNNonEmpty(bundle.ProspectiveRefs, 3),
		})
	}
	if len(issues) > 0 {
		out = append(out, CoherenceSuggestion{
			SuggestionID:        "coherence-deferred-knowledge",
			Title:               "Deferred knowledge should feed the next archaeology pass",
			Summary:             "Execution surfaced unresolved concerns that should be preserved as semantic follow-up.",
			SuggestedAction:     "review deferred issues before starting the next plan-backed run",
			TouchedSymbols:      touched,
			PatternRefs:         firstNNonEmpty(bundle.PatternRefs, 3),
			TensionRefs:         firstNNonEmpty(bundle.TensionRefs, 3),
			RelevantRequestRefs: firstNNonEmpty(bundle.RequestProvenanceRefs, 3),
		})
	}
	return out
}

func ApplySemanticReasoningToDeferredIssues(issues []DeferredExecutionIssue, bundle SemanticInputBundle, state *core.Context) []DeferredExecutionIssue {
	if len(issues) == 0 {
		return nil
	}
	touched := touchedSymbolsFromState(state)
	out := make([]DeferredExecutionIssue, 0, len(issues))
	for _, issue := range issues {
		issue.Evidence.TouchedSymbols = uniqueStrings(append(issue.Evidence.TouchedSymbols, touched...))
		switch issue.Kind {
		case DeferredIssuePatternTension:
			issue.Evidence.RelevantPatternRefs = uniqueStrings(append(issue.Evidence.RelevantPatternRefs, flattenPatternProposalRefs(bundle.PatternProposals)...))
			issue.Evidence.RelevantTensionRefs = uniqueStrings(append(issue.Evidence.RelevantTensionRefs, flattenTensionClusterRefs(bundle.TensionClusters)...))
			issue.Evidence.ShortReasoningSummary = enrichReasoningSummary(issue.Evidence.ShortReasoningSummary, "Pattern and tension reasoning indicates the current scope should be revisited before the next run.")
		case DeferredIssueStaleAssumption:
			issue.Evidence.RelevantRequestRefs = uniqueStrings(append(issue.Evidence.RelevantRequestRefs, requestIDs(bundle.PendingRequests)...))
			issue.Evidence.RelevantProvenanceRefs = uniqueStrings(append(issue.Evidence.RelevantProvenanceRefs, bundle.LearningInteractionRefs...))
			issue.Evidence.ShortReasoningSummary = enrichReasoningSummary(issue.Evidence.ShortReasoningSummary, "Pending semantic work suggests the active step may depend on stale assumptions.")
		case DeferredIssueAmbiguity:
			issue.Evidence.RelevantRequestRefs = uniqueStrings(append(issue.Evidence.RelevantRequestRefs, prospectivePairingRefs(bundle.ProspectivePairings)...))
			issue.Evidence.ShortReasoningSummary = enrichReasoningSummary(issue.Evidence.ShortReasoningSummary, "Prospective pairings expose alternatives that were left for user-directed archaeology.")
		}
		if len(bundle.CoherenceSuggestions) > 0 {
			issue.Evidence.RelevantPatternRefs = uniqueStrings(append(issue.Evidence.RelevantPatternRefs, flattenCoherencePatternRefs(bundle.CoherenceSuggestions)...))
			issue.Evidence.RelevantTensionRefs = uniqueStrings(append(issue.Evidence.RelevantTensionRefs, flattenCoherenceTensionRefs(bundle.CoherenceSuggestions)...))
		}
		out = append(out, issue)
	}
	return out
}

func requestIDs(input []SemanticRequestRef) []string {
	out := make([]string, 0, len(input))
	for _, item := range input {
		if id := strings.TrimSpace(item.RequestID); id != "" {
			out = append(out, id)
		}
	}
	return uniqueStrings(out)
}

func filterPatternRefs(input []string) []string {
	out := make([]string, 0, len(input))
	for _, ref := range input {
		if strings.Contains(strings.ToLower(ref), "pattern") {
			out = append(out, ref)
		}
	}
	return uniqueStrings(out)
}

func filterRefsByPrefix(input []string, prefix string) []string {
	out := make([]string, 0, len(input))
	prefix = strings.ToLower(strings.TrimSpace(prefix))
	for _, ref := range input {
		lowered := strings.ToLower(strings.TrimSpace(ref))
		if strings.Contains(lowered, prefix) {
			out = append(out, ref)
		}
	}
	return uniqueStrings(out)
}

func tensionSeverityForFinding(finding SemanticFindingSummary) string {
	status := strings.ToLower(strings.TrimSpace(finding.Status))
	switch status {
	case "blocked", "rejected", "failed":
		return "high"
	case "pending":
		return "medium"
	default:
		if strings.Contains(strings.ToLower(finding.Kind), "active_plan") {
			return "medium"
		}
		return "low"
	}
}

func flattenPatternProposalRefs(input []PatternProposalSummary) []string {
	var out []string
	for _, item := range input {
		out = append(out, item.PatternRefs...)
	}
	return uniqueStrings(out)
}

func flattenTensionClusterRefs(input []TensionClusterSummary) []string {
	var out []string
	for _, item := range input {
		out = append(out, item.TensionRefs...)
	}
	return uniqueStrings(out)
}

func flattenCoherencePatternRefs(input []CoherenceSuggestion) []string {
	var out []string
	for _, item := range input {
		out = append(out, item.PatternRefs...)
	}
	return uniqueStrings(out)
}

func flattenCoherenceTensionRefs(input []CoherenceSuggestion) []string {
	var out []string
	for _, item := range input {
		out = append(out, item.TensionRefs...)
	}
	return uniqueStrings(out)
}

func prospectivePairingRefs(input []ProspectivePairingSummary) []string {
	var out []string
	for _, item := range input {
		if ref := strings.TrimSpace(item.ProspectiveRef); ref != "" {
			out = append(out, ref)
		}
	}
	return uniqueStrings(out)
}

func enrichReasoningSummary(existing, extra string) string {
	existing = strings.TrimSpace(existing)
	extra = strings.TrimSpace(extra)
	switch {
	case existing == "":
		return extra
	case extra == "":
		return existing
	case strings.Contains(existing, extra):
		return existing
	default:
		return existing + " " + extra
	}
}

func dedupeTensionClusters(input []TensionClusterSummary) []TensionClusterSummary {
	if len(input) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]TensionClusterSummary, 0, len(input))
	for _, item := range input {
		key := strings.Join([]string{
			strings.TrimSpace(item.ClusterID),
			strings.TrimSpace(item.Title),
			strings.Join(uniqueStrings(item.TensionRefs), ","),
		}, "|")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		item.TensionRefs = uniqueStrings(item.TensionRefs)
		item.PatternRefs = uniqueStrings(item.PatternRefs)
		item.ProvenanceRefs = uniqueStrings(item.ProvenanceRefs)
		item.RelatedRequestRefs = uniqueStrings(item.RelatedRequestRefs)
		out = append(out, item)
	}
	return out
}

func dedupeCoherenceSuggestions(input []CoherenceSuggestion) []CoherenceSuggestion {
	if len(input) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]CoherenceSuggestion, 0, len(input))
	for _, item := range input {
		key := strings.Join([]string{
			strings.TrimSpace(item.SuggestionID),
			strings.TrimSpace(item.Title),
			strings.Join(uniqueStrings(item.TouchedSymbols), ","),
		}, "|")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		item.TouchedSymbols = uniqueStrings(item.TouchedSymbols)
		item.PatternRefs = uniqueStrings(item.PatternRefs)
		item.TensionRefs = uniqueStrings(item.TensionRefs)
		item.RelevantRequestRefs = uniqueStrings(item.RelevantRequestRefs)
		out = append(out, item)
	}
	return out
}

package runtime

import (
	"fmt"
	"strings"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/runtime/statebus")

// EnrichSemanticInputBundle derives Euclo-owned reasoning summaries over the
// existing semantic inputs without requiring new Archaeo schema.
func EnrichSemanticInputBundle(bundle SemanticInputBundle, state *core.Context, work UnitOfWork, issues []DeferredExecutionIssue) SemanticInputBundle {
	bundle.PatternProposals = dedupePatternProposals(buildPatternProposals(bundle))
	bundle.ProspectivePairings = dedupeProspectivePairings(buildProspectivePairings(bundle))
	return enrichCoherenceReasoning(bundle, state, work, issues)
}

func buildPatternProposals(bundle SemanticInputBundle) []PatternProposalSummary {
	out := make([]PatternProposalSummary, 0, len(bundle.PatternFindings))
	for idx, finding := range bundle.PatternFindings {
		supportingRefs := uniqueStrings(append([]string{strings.TrimSpace(finding.RefID)}, finding.RelatedRefs...))
		if len(supportingRefs) == 0 && len(bundle.PatternRefs) == 0 {
			continue
		}
		patternRefs := firstNNonEmpty(uniqueStrings(append([]string{}, supportingRefs...)), 4)
		if len(patternRefs) == 0 {
			patternRefs = firstNNonEmpty(bundle.PatternRefs, 4)
		}
		proposalID := strings.TrimSpace(finding.RefID)
		if proposalID == "" {
			proposalID = fmt.Sprintf("pattern-proposal-%d", idx+1)
		}
		out = append(out, PatternProposalSummary{
			ProposalID:          proposalID,
			Title:               nonEmpty(strings.TrimSpace(finding.Title), "Pattern-grounded execution proposal"),
			Summary:             nonEmpty(strings.TrimSpace(finding.Summary), "Use the available pattern evidence to keep the execution path grounded."),
			PatternRefs:         patternRefs,
			RelatedTensionRefs:  firstNNonEmpty(bundle.TensionRefs, 4),
			SupportingRefs:      supportingRefs,
			RecommendedFollowup: patternProposalFollowup(finding),
		})
	}
	if len(out) == 0 && len(bundle.PatternRefs) > 0 {
		out = append(out, PatternProposalSummary{
			ProposalID:          "pattern-proposal-active-context",
			Title:               "Pattern-grounded active context",
			Summary:             "Pattern references remain available and should anchor follow-on execution decisions.",
			PatternRefs:         firstNNonEmpty(bundle.PatternRefs, 4),
			RelatedTensionRefs:  firstNNonEmpty(bundle.TensionRefs, 4),
			SupportingRefs:      firstNNonEmpty(bundle.ProvenanceRefs, 4),
			RecommendedFollowup: "confirm edits and verification continue to align with the active pattern set",
		})
	}
	return out
}

func buildProspectivePairings(bundle SemanticInputBundle) []ProspectivePairingSummary {
	out := make([]ProspectivePairingSummary, 0, len(bundle.ProspectiveFindings))
	for idx, finding := range bundle.ProspectiveFindings {
		pairingID := strings.TrimSpace(finding.RefID)
		if pairingID == "" {
			pairingID = fmt.Sprintf("prospective-pairing-%d", idx+1)
		}
		candidateRefs := uniqueStrings(append([]string{}, finding.RelatedRefs...))
		candidateRefs = append(candidateRefs, firstNNonEmpty(bundle.RequestProvenanceRefs, 2)...)
		out = append(out, ProspectivePairingSummary{
			PairingID:       pairingID,
			Title:           nonEmpty(strings.TrimSpace(finding.Title), "Prospective execution pairing"),
			Summary:         nonEmpty(strings.TrimSpace(finding.Summary), "Prospective analysis remains available for execution tradeoffs."),
			ProspectiveRef:  strings.TrimSpace(finding.RefID),
			PatternRefs:     firstNNonEmpty(bundle.PatternRefs, 4),
			ConvergenceRefs: firstNNonEmpty(bundle.ConvergenceRefs, 3),
			CandidateRefs:   firstNNonEmpty(uniqueStrings(candidateRefs), 5),
		})
	}
	return out
}

func patternProposalFollowup(finding SemanticFindingSummary) string {
	switch {
	case strings.Contains(strings.ToLower(finding.Kind), "request"):
		return "review the request-derived pattern evidence before widening scope"
	case strings.Contains(strings.ToLower(finding.Kind), "active_plan"):
		return "keep the active plan aligned with the surfaced pattern evidence"
	default:
		return "confirm the next step preserves the available pattern evidence"
	}
}

func dedupePatternProposals(input []PatternProposalSummary) []PatternProposalSummary {
	if len(input) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]PatternProposalSummary, 0, len(input))
	for _, item := range input {
		key := strings.Join([]string{
			strings.TrimSpace(item.ProposalID),
			strings.TrimSpace(item.Title),
			strings.Join(uniqueStrings(item.PatternRefs), ","),
		}, "|")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		item.PatternRefs = uniqueStrings(item.PatternRefs)
		item.RelatedTensionRefs = uniqueStrings(item.RelatedTensionRefs)
		item.SupportingRefs = uniqueStrings(item.SupportingRefs)
		out = append(out, item)
	}
	return out
}

func dedupeProspectivePairings(input []ProspectivePairingSummary) []ProspectivePairingSummary {
	if len(input) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]ProspectivePairingSummary, 0, len(input))
	for _, item := range input {
		key := strings.Join([]string{
			strings.TrimSpace(item.PairingID),
			strings.TrimSpace(item.ProspectiveRef),
			strings.Join(uniqueStrings(item.PatternRefs), ","),
		}, "|")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		item.PatternRefs = uniqueStrings(item.PatternRefs)
		item.ConvergenceRefs = uniqueStrings(item.ConvergenceRefs)
		item.CandidateRefs = uniqueStrings(item.CandidateRefs)
		out = append(out, item)
	}
	return out
}

func touchedSymbolsFromState(state *core.Context) []string {
	if state == nil {
		return nil
	}
	var out []string
	if raw, ok := statebus.GetAny(state, "euclo.touched_symbols"); ok && raw != nil {
		out = append(out, stringSliceAny(raw)...)
	}
	if raw, ok := statebus.GetAny(state, "euclo.edit_execution"); ok && raw != nil {
		switch typed := raw.(type) {
		case EditExecutionRecord:
			out = append(out, touchedSymbolsFromEditExecution(typed)...)
		case *EditExecutionRecord:
			if typed != nil {
				out = append(out, touchedSymbolsFromEditExecution(*typed)...)
			}
		}
	}
	return uniqueStrings(out)
}

func touchedSymbolsFromEditExecution(record EditExecutionRecord) []string {
	var out []string
	appendOps := func(items []EditOperationRecord) {
		for _, item := range items {
			if path := strings.TrimSpace(item.Path); path != "" {
				out = append(out, path)
			}
		}
	}
	appendOps(record.Executed)
	appendOps(record.Approved)
	appendOps(record.Requested)
	return uniqueStrings(out)
}

func firstNNonEmpty(input []string, limit int) []string {
	values := uniqueStrings(input)
	if limit <= 0 || len(values) <= limit {
		return values
	}
	return append([]string(nil), values[:limit]...)
}

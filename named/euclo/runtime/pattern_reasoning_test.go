package runtime

import (
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
)

func TestEnrichSemanticInputBundleAddsPatternTensionAndCoherenceReasoning(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.touched_symbols", []string{"pkg/service.go", "pkg/handler.go"})
	state.Set("euclo.edit_execution", EditExecutionRecord{
		Executed: []EditOperationRecord{{Path: "pkg/service.go", Status: "executed"}},
	})

	bundle := EnrichSemanticInputBundle(SemanticInputBundle{
		WorkflowID:            "wf-1",
		PatternRefs:           []string{"pattern-a", "pattern-b"},
		TensionRefs:           []string{"tension-a"},
		ProspectiveRefs:       []string{"req-prospect"},
		RequestProvenanceRefs: []string{"req-pattern", "req-prospect"},
		PatternFindings: []SemanticFindingSummary{{
			RefID:       "pattern-a",
			Kind:        "pattern_request",
			Status:      "pending",
			Title:       "Surface service boundary pattern",
			Summary:     "A stable service-boundary pattern is available.",
			RelatedRefs: []string{"pattern-a", "pattern-b"},
		}},
		TensionFindings: []SemanticFindingSummary{{
			RefID:       "tension-a",
			Kind:        "tension_request",
			Status:      "pending",
			Title:       "Service boundary tension",
			Summary:     "The current change may widen the boundary.",
			RelatedRefs: []string{"pattern-a", "tension-a"},
		}},
		ProspectiveFindings: []SemanticFindingSummary{{
			RefID:   "req-prospect",
			Kind:    "prospective_request",
			Status:  "completed",
			Title:   "Prospective pairing available",
			Summary: "Two possible structures remain viable.",
		}},
		LearningFindings: []SemanticFindingSummary{{
			RefID:   "learn-1",
			Kind:    "pending_learning",
			Status:  "pending",
			Title:   "Pending learning",
			Summary: "A learning item still needs confirmation.",
		}},
		PendingRequests: []SemanticRequestRef{{RequestID: "req-pattern", Kind: "pattern", Status: "pending"}},
	}, state, UnitOfWork{
		PlanBinding: &UnitOfWorkPlanBinding{
			PlanID:       "plan-1",
			PlanVersion:  2,
			ActiveStepID: "step-2",
			IsPlanBacked: true,
		},
	}, nil)

	if len(bundle.PatternProposals) == 0 {
		t.Fatalf("expected pattern proposals: %#v", bundle)
	}
	if len(bundle.TensionClusters) == 0 {
		t.Fatalf("expected tension clusters: %#v", bundle)
	}
	if len(bundle.ProspectivePairings) == 0 {
		t.Fatalf("expected prospective pairings: %#v", bundle)
	}
	if len(bundle.CoherenceSuggestions) == 0 {
		t.Fatalf("expected coherence suggestions: %#v", bundle)
	}
	foundTouched := false
	for _, suggestion := range bundle.CoherenceSuggestions {
		if len(suggestion.TouchedSymbols) > 0 {
			foundTouched = true
			break
		}
	}
	if !foundTouched {
		t.Fatalf("expected touched symbols to flow into coherence suggestions: %#v", bundle.CoherenceSuggestions)
	}
}

func TestApplySemanticReasoningToDeferredIssuesAugmentsEvidence(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.touched_symbols", []string{"pkg/service.go"})

	bundle := SemanticInputBundle{
		PatternProposals: []PatternProposalSummary{{
			ProposalID:  "pattern-a",
			PatternRefs: []string{"pattern-a"},
		}},
		TensionClusters: []TensionClusterSummary{{
			ClusterID:   "tension-a",
			TensionRefs: []string{"tension-a"},
		}},
		ProspectivePairings: []ProspectivePairingSummary{{
			PairingID:      "pair-1",
			ProspectiveRef: "req-prospect",
		}},
		CoherenceSuggestions: []CoherenceSuggestion{{
			SuggestionID:   "coherence-a",
			PatternRefs:    []string{"pattern-a"},
			TensionRefs:    []string{"tension-a"},
			TouchedSymbols: []string{"pkg/service.go"},
		}},
		PendingRequests:         []SemanticRequestRef{{RequestID: "req-pattern"}},
		LearningInteractionRefs: []string{"learn-1"},
	}

	issues := ApplySemanticReasoningToDeferredIssues([]DeferredExecutionIssue{
		{IssueID: "tension-1", Kind: DeferredIssuePatternTension, Evidence: DeferredExecutionEvidence{}},
		{IssueID: "stale-1", Kind: DeferredIssueStaleAssumption, Evidence: DeferredExecutionEvidence{}},
		{IssueID: "amb-1", Kind: DeferredIssueAmbiguity, Evidence: DeferredExecutionEvidence{}},
	}, bundle, state)

	if len(issues) != 3 {
		t.Fatalf("unexpected issue count: %d", len(issues))
	}
	if len(issues[0].Evidence.RelevantPatternRefs) == 0 || len(issues[0].Evidence.RelevantTensionRefs) == 0 {
		t.Fatalf("expected pattern tension issue to be enriched: %#v", issues[0].Evidence)
	}
	if len(issues[1].Evidence.RelevantRequestRefs) == 0 || len(issues[1].Evidence.RelevantProvenanceRefs) == 0 {
		t.Fatalf("expected stale assumption issue to be enriched: %#v", issues[1].Evidence)
	}
	if len(issues[2].Evidence.RelevantRequestRefs) == 0 {
		t.Fatalf("expected ambiguity issue to include prospective refs: %#v", issues[2].Evidence)
	}
	if len(issues[0].Evidence.TouchedSymbols) == 0 {
		t.Fatalf("expected touched symbols to be copied into issue evidence: %#v", issues[0].Evidence)
	}
}

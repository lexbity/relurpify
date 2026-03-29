package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/guidance"
)

func TestBuildDeferredExecutionIssuesMapsTaxonomyAndEvidence(t *testing.T) {
	now := time.Unix(1700, 0).UTC()
	plan := &guidance.DeferralPlan{
		ID:         "def-plan-1",
		WorkflowID: "wf-1",
		Observations: []guidance.EngineeringObservation{
			{ID: "amb-1", GuidanceKind: guidance.GuidanceAmbiguity, Title: "Ambiguous requirement", Description: "Need user clarification", BlastRadius: 1},
			{ID: "stale-1", GuidanceKind: guidance.GuidanceConfidence, Title: "Stale assumption", Description: "Confidence is low", BlastRadius: 4},
			{ID: "tension-1", GuidanceKind: guidance.GuidanceScopeExpansion, Title: "Scope moved", Description: "Pattern tension found", BlastRadius: 8, Evidence: map[string]any{"tension_refs": []string{"ten-1"}}},
			{ID: "fail-1", GuidanceKind: guidance.GuidanceRecovery, Title: "Nonfatal failure", Description: "Execution continued", BlastRadius: 3},
			{ID: "verify-1", GuidanceKind: guidance.GuidanceContradiction, Title: "Verification concern", Description: "Checks disagree", BlastRadius: 6, Evidence: map[string]any{"verification_refs": []string{"verify-1"}}},
			{ID: "provider-1", Source: "provider.monitor", GuidanceKind: guidance.GuidanceRecovery, Title: "Provider degraded", Description: "Model degraded during run", BlastRadius: 2, Evidence: map[string]any{"provider_state_snapshot": map[string]any{"llm": "degraded"}}},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	uow := UnitOfWork{
		ID:          "uow-1",
		WorkflowID:  "wf-1",
		RunID:       "run-1",
		ExecutionID: "exec-1",
		PlanBinding: &UnitOfWorkPlanBinding{
			PlanID:        "plan-1",
			PlanVersion:   2,
			ActiveStepID:  "step-2",
			StepIDs:       []string{"step-1", "step-2"},
			IsPlanBacked:  true,
			IsLongRunning: true,
			ArchaeoRefs: map[string][]string{
				"provenance_refs": {"prov-1"},
			},
		},
	}
	state := core.NewContext()
	state.Set("euclo.provider_state_snapshot", map[string]any{"provider": "fallback"})

	issues := BuildDeferredExecutionIssues(plan, uow, state, now)
	if len(issues) != 6 {
		t.Fatalf("expected 6 issues, got %d", len(issues))
	}

	byID := map[string]DeferredExecutionIssue{}
	for _, issue := range issues {
		byID[issue.IssueID] = issue
		if issue.WorkflowID != "wf-1" || issue.RunID != "run-1" || issue.ExecutionID != "exec-1" {
			t.Fatalf("identity missing from issue: %#v", issue)
		}
		if issue.StepID != "step-2" {
			t.Fatalf("expected active step to propagate, got %#v", issue.StepID)
		}
		if len(issue.RelatedStepIDs) < 2 {
			t.Fatalf("expected related step ids, got %#v", issue.RelatedStepIDs)
		}
	}

	if byID["amb-1"].Kind != DeferredIssueAmbiguity {
		t.Fatalf("ambiguity mapped to %q", byID["amb-1"].Kind)
	}
	if byID["stale-1"].Kind != DeferredIssueStaleAssumption {
		t.Fatalf("stale assumption mapped to %q", byID["stale-1"].Kind)
	}
	if byID["tension-1"].Kind != DeferredIssuePatternTension {
		t.Fatalf("pattern tension mapped to %q", byID["tension-1"].Kind)
	}
	if byID["fail-1"].Kind != DeferredIssueNonfatalFailure {
		t.Fatalf("nonfatal failure mapped to %q", byID["fail-1"].Kind)
	}
	if byID["verify-1"].Kind != DeferredIssueVerificationConcern {
		t.Fatalf("verification concern mapped to %q", byID["verify-1"].Kind)
	}
	if byID["provider-1"].Kind != DeferredIssueProviderConstraint {
		t.Fatalf("provider constraint mapped to %q", byID["provider-1"].Kind)
	}
	if byID["verify-1"].Evidence.VerificationRefs[0] != "verify-1" {
		t.Fatalf("verification refs missing: %#v", byID["verify-1"].Evidence)
	}
	if byID["provider-1"].Evidence.ProviderStateSnapshot["llm"] != "degraded" {
		t.Fatalf("provider snapshot missing: %#v", byID["provider-1"].Evidence.ProviderStateSnapshot)
	}
	if byID["tension-1"].Severity != DeferredIssueSeverityHigh {
		t.Fatalf("expected high severity, got %q", byID["tension-1"].Severity)
	}
	if len(byID["amb-1"].ArchaeoRefs["provenance_refs"]) != 1 {
		t.Fatalf("expected plan archaeo refs to carry through: %#v", byID["amb-1"].ArchaeoRefs)
	}
}

func TestPersistDeferredExecutionIssuesToWorkspaceWritesMarkdownArtifact(t *testing.T) {
	workspace := t.TempDir()
	task := &core.Task{ID: "task-1", Context: map[string]any{"workspace": workspace}}
	state := core.NewContext()
	issue := DeferredExecutionIssue{
		IssueID:               "defer-1",
		WorkflowID:            "wf-1",
		RunID:                 "run-1",
		ExecutionID:           "exec-1",
		ActivePlanID:          "plan-1",
		ActivePlanVersion:     1,
		StepID:                "step-1",
		RelatedStepIDs:        []string{"step-1"},
		Kind:                  DeferredIssueVerificationConcern,
		Severity:              DeferredIssueSeverityMedium,
		Status:                DeferredIssueStatusOpen,
		Title:                 "Verification drift",
		Summary:               "A verification concern remained unresolved.",
		WhyNotResolvedInline:  "execution continued and preserved the concern",
		RecommendedReentry:    "archaeology",
		RecommendedNextAction: "review the evidence",
		Evidence: DeferredExecutionEvidence{
			ShortReasoningSummary:  "One check contradicted another.",
			TouchedSymbols:         []string{"pkg.Handler"},
			RelevantPatternRefs:    []string{"pattern-1"},
			RelevantTensionRefs:    []string{"tension-1"},
			RelevantProvenanceRefs: []string{"prov-1"},
			RelevantRequestRefs:    []string{"req-1"},
		},
		ArchaeoRefs: map[string][]string{"request_refs": {"req-1"}},
		CreatedAt:   time.Unix(1900, 0).UTC(),
		UpdatedAt:   time.Unix(1901, 0).UTC(),
	}

	persisted := PersistDeferredExecutionIssuesToWorkspace(task, state, []DeferredExecutionIssue{issue})
	if len(persisted) != 1 {
		t.Fatalf("expected 1 persisted issue, got %d", len(persisted))
	}
	if persisted[0].WorkspaceArtifactPath == "" {
		t.Fatal("expected workspace artifact path to be populated")
	}
	if !strings.HasPrefix(persisted[0].WorkspaceArtifactPath, filepath.Join(workspace, "relurpify_cfg", "artifacts", "euclo", "deferred")) {
		t.Fatalf("unexpected artifact path: %q", persisted[0].WorkspaceArtifactPath)
	}

	raw, err := os.ReadFile(persisted[0].WorkspaceArtifactPath)
	if err != nil {
		t.Fatalf("read artifact: %v", err)
	}
	content := string(raw)
	for _, needle := range []string{
		"---\n",
		"issue_id: \"defer-1\"",
		"kind: \"verification_concern\"",
		"# Verification drift",
		"## Why Execution Continued",
		"## Recommended Next Action",
		"## Evidence",
		"- pkg.Handler",
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("artifact missing %q:\n%s", needle, content)
		}
	}
}

func TestSemanticReasoningAugmentsArtifactedDeferrals(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.touched_symbols", []string{"pkg/service.go"})
	issues := []DeferredExecutionIssue{{
		IssueID: "tension-1",
		Kind:    DeferredIssuePatternTension,
		Title:   "Pattern tension remained open",
		Evidence: DeferredExecutionEvidence{
			ShortReasoningSummary: "Scope widened during execution.",
		},
	}}
	bundle := SemanticInputBundle{
		PatternProposals: []PatternProposalSummary{{
			ProposalID:  "pattern-a",
			PatternRefs: []string{"pattern-a"},
		}},
		TensionClusters: []TensionClusterSummary{{
			ClusterID:   "tension-a",
			TensionRefs: []string{"tension-a"},
		}},
		CoherenceSuggestions: []CoherenceSuggestion{{
			SuggestionID:   "coherence-a",
			PatternRefs:    []string{"pattern-a"},
			TensionRefs:    []string{"tension-a"},
			TouchedSymbols: []string{"pkg/service.go"},
		}},
	}

	issues = ApplySemanticReasoningToDeferredIssues(issues, bundle, state)
	if len(issues[0].Evidence.RelevantPatternRefs) == 0 {
		t.Fatalf("expected pattern reasoning in deferral evidence: %#v", issues[0].Evidence)
	}
	if len(issues[0].Evidence.RelevantTensionRefs) == 0 {
		t.Fatalf("expected tension reasoning in deferral evidence: %#v", issues[0].Evidence)
	}
	if len(issues[0].Evidence.TouchedSymbols) == 0 {
		t.Fatalf("expected touched symbols in deferral evidence: %#v", issues[0].Evidence)
	}
	if !strings.Contains(issues[0].Evidence.ShortReasoningSummary, "Pattern and tension reasoning") {
		t.Fatalf("expected enriched reasoning summary: %#v", issues[0].Evidence.ShortReasoningSummary)
	}
}

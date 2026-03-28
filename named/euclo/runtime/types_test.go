package runtime

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
)

func TestUnitOfWorkJSONRoundTrip(t *testing.T) {
	now := time.Unix(123, 0).UTC()
	original := UnitOfWork{
		ID:                   "uow-1",
		WorkflowID:           "wf-1",
		RunID:                "run-1",
		ExecutionID:          "exec-1",
		ModeID:               "plan",
		ObjectiveKind:        "plan_execution",
		BehaviorFamily:       "gap_analysis",
		ContextStrategyID:    "workflow_heavy",
		VerificationPolicyID: "verify-policy",
		DeferralPolicyID:     "defer-policy",
		CheckpointPolicyID:   "checkpoint-policy",
		PlanBinding: &UnitOfWorkPlanBinding{
			WorkflowID:    "wf-1",
			PlanID:        "plan-1",
			PlanVersion:   3,
			ActiveStepID:  "step-2",
			StepIDs:       []string{"step-1", "step-2"},
			IsPlanBacked:  true,
			IsLongRunning: true,
			ArchaeoRefs: map[string][]string{
				"provenance": {"prov-1"},
			},
		},
		ContextBundle: UnitOfWorkContextBundle{
			Sources: []UnitOfWorkContextSource{{
				Kind:    "workflow_retrieval",
				Ref:     "retr-1",
				Summary: "workflow context",
			}},
			WorkspacePaths:     []string{"app/main.go"},
			RetrievalRefs:      []string{"retr-1"},
			ArtifactKinds:      []string{"euclo.plan"},
			PatternRefs:        []string{"pattern-1"},
			TensionRefs:        []string{"tension-1"},
			ProvenanceRefs:     []string{"prov-1"},
			LearningRefs:       []string{"learn-1"},
			ContextBudgetClass: "heavy",
			CompactionEligible: true,
			RestoreRequired:    true,
		},
		RoutineBindings: []UnitOfWorkRoutineBinding{{
			RoutineID: "routine-1",
			Family:    "gap_analysis",
			Reason:    "plan step verification",
			Priority:  10,
			Required:  true,
		}},
		SkillBindings: []UnitOfWorkSkillBinding{{
			SkillID:  "skill-1",
			Reason:   "repo-specific workflow",
			Required: true,
		}},
		ToolBindings: []UnitOfWorkToolBinding{{
			ToolID:  "tool:write",
			Allowed: true,
			Reason:  "mutations required",
		}},
		CapabilityBindings: []UnitOfWorkCapabilityBinding{{
			CapabilityID: "euclo:edit_verify_repair",
			Family:       "pipeline",
			Required:     true,
		}},
		Status:           UnitOfWorkStatusExecuting,
		ResultClass:      ExecutionResultClassCompletedWithDeferrals,
		DeferredIssueIDs: []string{"defer-1"},
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	raw, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded UnitOfWork
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.ID != original.ID || decoded.WorkflowID != original.WorkflowID {
		t.Fatalf("identity lost in round-trip: %#v", decoded)
	}
	if decoded.PlanBinding == nil || decoded.PlanBinding.PlanID != "plan-1" {
		t.Fatalf("plan binding lost in round-trip: %#v", decoded.PlanBinding)
	}
	if decoded.ResultClass != ExecutionResultClassCompletedWithDeferrals {
		t.Fatalf("got result class %q", decoded.ResultClass)
	}
	if len(decoded.RoutineBindings) != 1 || decoded.RoutineBindings[0].Family != "gap_analysis" {
		t.Fatalf("routine bindings lost in round-trip: %#v", decoded.RoutineBindings)
	}
}

func TestDeferredExecutionIssueJSONRoundTrip(t *testing.T) {
	now := time.Unix(456, 0).UTC()
	original := DeferredExecutionIssue{
		IssueID:               "defer-1",
		WorkflowID:            "wf-1",
		RunID:                 "run-1",
		ExecutionID:           "exec-1",
		ActivePlanID:          "plan-1",
		ActivePlanVersion:     2,
		StepID:                "step-4",
		RelatedStepIDs:        []string{"step-3", "step-4"},
		Kind:                  DeferredIssuePatternTension,
		Severity:              DeferredIssueSeverityMedium,
		Status:                DeferredIssueStatusOpen,
		Title:                 "Pattern tension surfaced during execution",
		Summary:               "The implementation conflicts with the current policy pattern.",
		WhyNotResolvedInline:  "requires user-level architecture decision",
		RecommendedReentry:    "archaeology",
		RecommendedNextAction: "review tension and reopen planning later",
		WorkspaceArtifactPath: "relurpify_cfg/artifacts/euclo/deferred/defer-1.md",
		ArchaeoRefs:           map[string][]string{"tensions": {"tension-1"}, "provenance": {"prov-1"}},
		CreatedAt:             now,
		UpdatedAt:             now,
		Evidence: DeferredExecutionEvidence{
			TouchedSymbols:         []string{"app.Handler", "framework.Policy"},
			RelevantPatternRefs:    []string{"pattern-1"},
			RelevantTensionRefs:    []string{"tension-1"},
			RelevantAnchorRefs:     []string{"anchor-1"},
			RelevantProvenanceRefs: []string{"prov-1"},
			RelevantRequestRefs:    []string{"req-1"},
			VerificationRefs:       []string{"verify-1"},
			CheckpointRefs:         []string{"checkpoint-1"},
			ProviderStateSnapshot:  map[string]any{"llm": "degraded"},
			ShortReasoningSummary:  "The touched symbols intersect a known tension.",
		},
	}

	raw, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded DeferredExecutionIssue
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Kind != DeferredIssuePatternTension {
		t.Fatalf("got kind %q", decoded.Kind)
	}
	if decoded.Evidence.ShortReasoningSummary == "" || len(decoded.Evidence.TouchedSymbols) != 2 {
		t.Fatalf("evidence lost in round-trip: %#v", decoded.Evidence)
	}
	if decoded.WorkspaceArtifactPath == "" {
		t.Fatal("workspace artifact path lost in round-trip")
	}
}

func TestRuntimeAndCapabilityStatusesRemainDistinct(t *testing.T) {
	if ExecutionStatusCompleted != "completed" {
		t.Fatalf("unexpected runtime execution status: %q", ExecutionStatusCompleted)
	}
	if euclotypes.ExecutionStatusCompleted != "completed" {
		t.Fatalf("unexpected capability execution status: %q", euclotypes.ExecutionStatusCompleted)
	}
	if ExecutionResultClassCompletedWithDeferrals != "completed_with_deferrals" {
		t.Fatalf("unexpected result class: %q", ExecutionResultClassCompletedWithDeferrals)
	}
}

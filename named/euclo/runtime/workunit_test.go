package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// objectiveKindForWork
// ---------------------------------------------------------------------------

func TestObjectiveKindPlanExecution(t *testing.T) {
	mode := ModeResolution{ModeID: "planning"}
	profile := ExecutionProfileSelection{}
	class := TaskClassification{}
	require.Equal(t, "plan_execution", objectiveKindForWork(mode, profile, class))
}

func TestObjectiveKindPlanProfile(t *testing.T) {
	mode := ModeResolution{ModeID: "code"}
	profile := ExecutionProfileSelection{ProfileID: "plan_stage_execute"}
	class := TaskClassification{}
	require.Equal(t, "plan_execution", objectiveKindForWork(mode, profile, class))
}

func TestObjectiveKindDebugInvestigation(t *testing.T) {
	mode := ModeResolution{ModeID: "debug"}
	profile := ExecutionProfileSelection{}
	class := TaskClassification{}
	require.Equal(t, "investigation", objectiveKindForWork(mode, profile, class))
}

func TestObjectiveKindReview(t *testing.T) {
	mode := ModeResolution{ModeID: "review"}
	profile := ExecutionProfileSelection{}
	class := TaskClassification{}
	require.Equal(t, "review", objectiveKindForWork(mode, profile, class))
}

func TestObjectiveKindTDDExecution(t *testing.T) {
	mode := ModeResolution{ModeID: "code"}
	profile := ExecutionProfileSelection{ProfileID: "test_driven_generation"}
	class := TaskClassification{}
	require.Equal(t, "tdd_execution", objectiveKindForWork(mode, profile, class))
}

func TestObjectiveKindInvestigationViaClassification(t *testing.T) {
	mode := ModeResolution{ModeID: "code"}
	profile := ExecutionProfileSelection{}
	class := TaskClassification{RequiresEvidenceBeforeMutation: true}
	require.Equal(t, "investigation", objectiveKindForWork(mode, profile, class))
}

func TestObjectiveKindDirectExecution(t *testing.T) {
	mode := ModeResolution{ModeID: "code"}
	profile := ExecutionProfileSelection{}
	class := TaskClassification{}
	require.Equal(t, "direct_execution", objectiveKindForWork(mode, profile, class))
}

// ---------------------------------------------------------------------------
// behaviorFamilyForWork
// ---------------------------------------------------------------------------

func TestBehaviorFamilyTensionAssessmentDefaultReview(t *testing.T) {
	mode := ModeResolution{ModeID: "review"}
	profile := ExecutionProfileSelection{}
	class := TaskClassification{}
	policy := ResolvedExecutionPolicy{}
	require.Equal(t, "tension_assessment", behaviorFamilyForWork(mode, profile, class, policy))
}

func TestBehaviorFamilyApprovalAssessment(t *testing.T) {
	mode := ModeResolution{ModeID: "review"}
	profile := ExecutionProfileSelection{}
	class := TaskClassification{}
	policy := ResolvedExecutionPolicy{
		ReviewApprovalRules: core.AgentReviewApprovalRules{RequireVerificationEvidence: true},
	}
	require.Equal(t, "approval_assessment", behaviorFamilyForWork(mode, profile, class, policy))
}

func TestBehaviorFamilyCoherenceAssessment(t *testing.T) {
	mode := ModeResolution{ModeID: "review"}
	profile := ExecutionProfileSelection{}
	class := TaskClassification{}
	policy := ResolvedExecutionPolicy{ReviewCriteria: []string{"naming"}}
	require.Equal(t, "coherence_assessment", behaviorFamilyForWork(mode, profile, class, policy))
}

func TestBehaviorFamilyGapAnalysisPlanning(t *testing.T) {
	mode := ModeResolution{ModeID: "planning"}
	profile := ExecutionProfileSelection{}
	class := TaskClassification{}
	policy := ResolvedExecutionPolicy{PreferredPlanningCapabilities: []string{"cap-a"}}
	require.Equal(t, "gap_analysis", behaviorFamilyForWork(mode, profile, class, policy))
}

func TestBehaviorFamilyFailedVerificationRepairForVerifyPolicy(t *testing.T) {
	mode := ModeResolution{ModeID: "code"}
	profile := ExecutionProfileSelection{ProfileID: "edit_verify_repair", VerificationRequired: true}
	class := TaskClassification{}
	policy := ResolvedExecutionPolicy{PreferredVerifyCapabilities: []string{"go_test"}}
	require.Equal(t, "failed_verification_repair", behaviorFamilyForWork(mode, profile, class, policy))
}

func TestBehaviorFamilyDirectChangeExecution(t *testing.T) {
	mode := ModeResolution{ModeID: "code"}
	profile := ExecutionProfileSelection{ProfileID: "edit_verify_repair"}
	class := TaskClassification{}
	policy := ResolvedExecutionPolicy{}
	require.Equal(t, "direct_change_execution", behaviorFamilyForWork(mode, profile, class, policy))
}

func TestBehaviorFamilyStaleAssumptionDetection(t *testing.T) {
	mode := ModeResolution{ModeID: "code"}
	profile := ExecutionProfileSelection{ProfileID: "reproduce_localize_patch"}
	class := TaskClassification{}
	policy := ResolvedExecutionPolicy{}
	require.Equal(t, "stale_assumption_detection", behaviorFamilyForWork(mode, profile, class, policy))
}

func TestBehaviorFamilyTDDRedGreenRefactor(t *testing.T) {
	mode := ModeResolution{ModeID: "code"}
	profile := ExecutionProfileSelection{ProfileID: "test_driven_generation"}
	class := TaskClassification{}
	policy := ResolvedExecutionPolicy{}
	require.Equal(t, "tdd_red_green_refactor", behaviorFamilyForWork(mode, profile, class, policy))
}

func TestRoutineBindingsUseFailedVerificationRepairFamily(t *testing.T) {
	mode := ModeResolution{ModeID: "debug"}
	profile := ExecutionProfileSelection{ProfileID: "reproduce_localize_patch", VerificationRequired: true}
	class := TaskClassification{}
	policy := ResolvedExecutionPolicy{}
	bindings := routineBindingsForWork(mode, profile, class, policy)
	found := false
	for _, binding := range bindings {
		if binding.RoutineID == "verification_repair" {
			found = true
			require.Equal(t, "failed_verification_repair", binding.Family)
			break
		}
	}
	require.True(t, found)
}

func TestRoutineBindingsDoNotAddFailedVerificationRepairForReviewSuggestImplement(t *testing.T) {
	mode := ModeResolution{ModeID: "review"}
	profile := ExecutionProfileSelection{ProfileID: "review_suggest_implement", VerificationRequired: false}
	class := TaskClassification{}
	policy := ResolvedExecutionPolicy{}
	bindings := routineBindingsForWork(mode, profile, class, policy)
	for _, binding := range bindings {
		if binding.Family == "failed_verification_repair" {
			t.Fatalf("did not expect failed_verification_repair binding for review_suggest_implement, got %#v", binding)
		}
	}
}

func TestBehaviorFamilyGapAnalysisPlanProfile(t *testing.T) {
	mode := ModeResolution{ModeID: "code"}
	profile := ExecutionProfileSelection{ProfileID: "plan_stage_execute"}
	class := TaskClassification{}
	policy := ResolvedExecutionPolicy{}
	require.Equal(t, "gap_analysis", behaviorFamilyForWork(mode, profile, class, policy))
}

func TestBehaviorFamilyStaleAssumptionFromClassification(t *testing.T) {
	mode := ModeResolution{ModeID: "code"}
	profile := ExecutionProfileSelection{}
	class := TaskClassification{RequiresEvidenceBeforeMutation: true}
	policy := ResolvedExecutionPolicy{}
	require.Equal(t, "stale_assumption_detection", behaviorFamilyForWork(mode, profile, class, policy))
}

func TestPrimaryRelurpicCapabilityPrefersInspectOverAskForInspectPrompt(t *testing.T) {
	envelope := TaskEnvelope{
		Instruction:   "Inspect testsuite/fixtures/rapid_chat/inspect/store.go and compare MemoryStore with NullStore. Do not modify any files.",
		EditPermitted: false,
	}
	classification := TaskClassification{
		EditPermitted: false,
	}
	mode := ModeResolution{ModeID: "chat"}
	profile := ExecutionProfileSelection{}

	got := primaryRelurpicCapabilityForWork(envelope, classification, mode, profile)
	require.Equal(t, "euclo:chat.inspect", got)
}

func TestPrimaryRelurpicCapabilityKeepsAskForExplainPrompt(t *testing.T) {
	envelope := TaskEnvelope{
		Instruction:   "Explain what the User type represents and what NewUser does. Do not modify any files.",
		EditPermitted: false,
	}
	classification := TaskClassification{
		EditPermitted: false,
	}
	mode := ModeResolution{ModeID: "chat"}
	profile := ExecutionProfileSelection{}

	got := primaryRelurpicCapabilityForWork(envelope, classification, mode, profile)
	require.Equal(t, "euclo:chat.ask", got)
}

func TestPrimaryRelurpicCapabilityAskPromptOverridesReviewClassification(t *testing.T) {
	envelope := TaskEnvelope{
		Instruction:   "Explain what the User type represents and what NewUser does. Do not modify any files.",
		EditPermitted: false,
	}
	classification := TaskClassification{
		EditPermitted:  false,
		IntentFamilies: []string{"review"},
	}
	mode := ModeResolution{ModeID: "chat"}
	profile := ExecutionProfileSelection{}

	got := primaryRelurpicCapabilityForWork(envelope, classification, mode, profile)
	require.Equal(t, "euclo:chat.ask", got)
}

func TestPrimaryRelurpicCapabilityPlanningExplorePromptStaysExplore(t *testing.T) {
	envelope := TaskEnvelope{
		Instruction:   "Explore testsuite/fixtures/rapid_arch_pattern and identify the dominant normalization pattern plus any inconsistent implementation. Do not modify files.",
		EditPermitted: false,
	}
	classification := TaskClassification{EditPermitted: false}
	mode := ModeResolution{ModeID: "planning"}
	profile := ExecutionProfileSelection{ProfileID: "plan_stage_execute"}

	got := primaryRelurpicCapabilityForWork(envelope, classification, mode, profile)
	require.Equal(t, "euclo:archaeology.explore", got)
}

func TestPrimaryRelurpicCapabilityPlanningCompiledExecutionPromptUsesImplement(t *testing.T) {
	envelope := TaskEnvelope{
		Instruction:   "Execute the compiled plan for testsuite/fixtures/rapid_arch_exec/slug.go. Keep the change limited to that file.",
		EditPermitted: true,
	}
	classification := TaskClassification{EditPermitted: true}
	mode := ModeResolution{ModeID: "planning"}
	profile := ExecutionProfileSelection{ProfileID: "plan_stage_execute"}

	got := primaryRelurpicCapabilityForWork(envelope, classification, mode, profile)
	require.Equal(t, "euclo:archaeology.implement-plan", got)
}

func TestPlanBindingFromStateUsesSeededPipelinePlan(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.workflow_id", "wf-seeded")
	state.Set("pipeline.plan", map[string]any{
		"steps": []map[string]any{{
			"id":          "seeded-step-1",
			"description": "execute the compiled plan",
		}},
	})

	binding := planBindingFromState(nil, state)
	require.NotNil(t, binding)
	require.True(t, binding.IsPlanBacked)
	require.Equal(t, "wf-seeded", binding.WorkflowID)
	require.Equal(t, "seeded-step-1", binding.ActiveStepID)
}

// ---------------------------------------------------------------------------
// ResultClassForOutcome
// ---------------------------------------------------------------------------

func TestResultClassCompleted(t *testing.T) {
	rc := ResultClassForOutcome(ExecutionStatusCompleted, nil, nil)
	require.Equal(t, ExecutionResultClassCompleted, rc)
}

func TestResultClassCompletedWithDeferrals(t *testing.T) {
	rc := ResultClassForOutcome(ExecutionStatusCompleted, []string{"issue-1"}, nil)
	require.Equal(t, ExecutionResultClassCompletedWithDeferrals, rc)
}

func TestResultClassCompletedWithDeferralsStatus(t *testing.T) {
	rc := ResultClassForOutcome(ExecutionStatusCompletedWithDeferrals, nil, nil)
	require.Equal(t, ExecutionResultClassCompletedWithDeferrals, rc)
}

func TestResultClassBlocked(t *testing.T) {
	rc := ResultClassForOutcome(ExecutionStatusBlocked, nil, nil)
	require.Equal(t, ExecutionResultClassBlocked, rc)
}

func TestResultClassFailed(t *testing.T) {
	rc := ResultClassForOutcome(ExecutionStatusFailed, nil, nil)
	require.Equal(t, ExecutionResultClassFailed, rc)
}

func TestResultClassFailedFromError(t *testing.T) {
	rc := ResultClassForOutcome(ExecutionStatusCompleted, nil, errors.New("boom"))
	require.Equal(t, ExecutionResultClassFailed, rc)
}

func TestResultClassCanceled(t *testing.T) {
	rc := ResultClassForOutcome(ExecutionStatusCompleted, nil, context.Canceled)
	require.Equal(t, ExecutionResultClassCanceled, rc)
}

func TestResultClassRestoreFailed(t *testing.T) {
	rc := ResultClassForOutcome(ExecutionStatusRestoreFailed, nil, nil)
	require.Equal(t, ExecutionResultClassRestoreFailed, rc)
}

// ---------------------------------------------------------------------------
// StatusForResultClass
// ---------------------------------------------------------------------------

func TestStatusForResultClassCompleted(t *testing.T) {
	s := StatusForResultClass("", ExecutionResultClassCompleted)
	require.Equal(t, ExecutionStatusCompleted, s)
}

func TestStatusForResultClassPreservesStatusWhenCompleted(t *testing.T) {
	s := StatusForResultClass(ExecutionStatusExecuting, ExecutionResultClassCompleted)
	require.Equal(t, ExecutionStatusExecuting, s)
}

func TestStatusForResultClassFailed(t *testing.T) {
	s := StatusForResultClass("", ExecutionResultClassFailed)
	require.Equal(t, ExecutionStatusFailed, s)
}

func TestStatusForResultClassBlocked(t *testing.T) {
	s := StatusForResultClass("", ExecutionResultClassBlocked)
	require.Equal(t, ExecutionStatusBlocked, s)
}

func TestStatusForResultClassCanceled(t *testing.T) {
	s := StatusForResultClass("", ExecutionResultClassCanceled)
	require.Equal(t, ExecutionStatusCanceled, s)
}

// ---------------------------------------------------------------------------
// BuildCompiledExecution / ReconstructUnitOfWorkFromCompiledExecution round-trip
// ---------------------------------------------------------------------------

func buildTestUnitOfWork() UnitOfWork {
	now := time.Now().UTC().Truncate(time.Second)
	return UnitOfWork{
		ID:                          "uow-1",
		RootID:                      "uow-root",
		WorkflowID:                  "wf-1",
		RunID:                       "run-1",
		ExecutionID:                 "exec-1",
		ModeID:                      "planning",
		ObjectiveKind:               "plan_execution",
		BehaviorFamily:              "gap_analysis",
		ContextStrategyID:           "narrow_to_wide",
		VerificationPolicyID:        "strict",
		DeferralPolicyID:            "default",
		CheckpointPolicyID:          "default",
		PrimaryRelurpicCapabilityID: "euclo.archaeology.compile_plan",
		SupportingRelurpicCapabilityIDs: []string{
			"euclo.archaeology.explore",
		},
		PredecessorUnitOfWorkID: "uow-0",
		TransitionReason:        "step_advance",
		DeferredIssueIDs:        []string{"di-1"},
		Status:                  UnitOfWorkStatusReady,
		CreatedAt:               now,
		UpdatedAt:               now,
	}
}

func TestBuildCompiledExecutionCopiesFields(t *testing.T) {
	uow := buildTestUnitOfWork()
	status := RuntimeExecutionStatus{
		Status:           ExecutionStatusCompleted,
		ResultClass:      ExecutionResultClassCompleted,
		AssuranceClass:   AssuranceClassVerifiedSuccess,
		UpdatedAt:        uow.CreatedAt,
		DeferredIssueIDs: []string{"di-1"},
	}

	compiled := BuildCompiledExecution(uow, status, uow.CreatedAt)

	require.Equal(t, uow.ID, compiled.UnitOfWorkID)
	require.Equal(t, uow.RootID, compiled.RootUnitOfWorkID)
	require.Equal(t, uow.WorkflowID, compiled.WorkflowID)
	require.Equal(t, uow.RunID, compiled.RunID)
	require.Equal(t, uow.ExecutionID, compiled.ExecutionID)
	require.Equal(t, uow.ModeID, compiled.ModeID)
	require.Equal(t, uow.ObjectiveKind, compiled.ObjectiveKind)
	require.Equal(t, uow.BehaviorFamily, compiled.BehaviorFamily)
	require.Equal(t, uow.ContextStrategyID, compiled.ContextStrategyID)
	require.Equal(t, uow.VerificationPolicyID, compiled.VerificationPolicyID)
	require.Equal(t, uow.PrimaryRelurpicCapabilityID, compiled.PrimaryRelurpicCapabilityID)
	require.Equal(t, uow.SupportingRelurpicCapabilityIDs, compiled.SupportingRelurpicCapabilityIDs)
	require.Equal(t, uow.PredecessorUnitOfWorkID, compiled.PredecessorUnitOfWorkID)
	require.Equal(t, uow.TransitionReason, compiled.TransitionReason)
	require.Equal(t, uow.TransitionState, compiled.TransitionState)
	require.Equal(t, status.DeferredIssueIDs, compiled.DeferredIssueIDs)
	require.Equal(t, ExecutionStatusCompleted, compiled.Status)
	require.Equal(t, ExecutionResultClassCompleted, compiled.ResultClass)
	require.Equal(t, AssuranceClassVerifiedSuccess, compiled.AssuranceClass)
}

func TestBuildCompiledExecutionSetsCompiledAtNow(t *testing.T) {
	uow := buildTestUnitOfWork()
	status := RuntimeExecutionStatus{Status: ExecutionStatusCompleted}

	before := time.Now().UTC().Add(-time.Second)
	compiled := BuildCompiledExecution(uow, status, time.Time{})
	after := time.Now().UTC().Add(time.Second)

	require.True(t, compiled.CompiledAt.After(before))
	require.True(t, compiled.CompiledAt.Before(after))
}

func TestBuildCompiledExecutionSlicesAreIsolated(t *testing.T) {
	uow := buildTestUnitOfWork()
	status := RuntimeExecutionStatus{
		Status:           ExecutionStatusCompleted,
		DeferredIssueIDs: []string{"di-1"},
	}

	compiled := BuildCompiledExecution(uow, status, uow.CreatedAt)

	// Mutating the source UoW slice must not affect the compiled copy.
	uow.SupportingRelurpicCapabilityIDs[0] = "mutated"
	require.Equal(t, "euclo.archaeology.explore", compiled.SupportingRelurpicCapabilityIDs[0])

	// Mutating the source status slice must not affect the compiled copy.
	status.DeferredIssueIDs[0] = "mutated"
	require.Equal(t, "di-1", compiled.DeferredIssueIDs[0])
}

func TestReconstructUnitOfWorkRoundTrip(t *testing.T) {
	uow := buildTestUnitOfWork()
	status := RuntimeExecutionStatus{
		Status:           ExecutionStatusCompleted,
		ResultClass:      ExecutionResultClassCompleted,
		UpdatedAt:        uow.CreatedAt,
		DeferredIssueIDs: []string{"di-1"},
	}

	compiled := BuildCompiledExecution(uow, status, uow.CreatedAt)

	state := core.NewContext()
	state.Set("euclo.compiled_execution", compiled)

	reconstructed, ok := ReconstructUnitOfWorkFromCompiledExecution(state)
	require.True(t, ok)

	require.Equal(t, uow.ID, reconstructed.ID)
	require.Equal(t, uow.RootID, reconstructed.RootID)
	require.Equal(t, uow.WorkflowID, reconstructed.WorkflowID)
	require.Equal(t, uow.RunID, reconstructed.RunID)
	require.Equal(t, uow.ExecutionID, reconstructed.ExecutionID)
	require.Equal(t, uow.ModeID, reconstructed.ModeID)
	require.Equal(t, uow.ObjectiveKind, reconstructed.ObjectiveKind)
	require.Equal(t, uow.BehaviorFamily, reconstructed.BehaviorFamily)
	require.Equal(t, uow.PrimaryRelurpicCapabilityID, reconstructed.PrimaryRelurpicCapabilityID)
	require.Equal(t, uow.SupportingRelurpicCapabilityIDs, reconstructed.SupportingRelurpicCapabilityIDs)
	require.Equal(t, uow.PredecessorUnitOfWorkID, reconstructed.PredecessorUnitOfWorkID)
	require.Equal(t, uow.TransitionReason, reconstructed.TransitionReason)
	require.Equal(t, uow.TransitionState, reconstructed.TransitionState)
	require.Equal(t, status.DeferredIssueIDs, reconstructed.DeferredIssueIDs)
	require.Equal(t, UnitOfWorkStatusCompleted, reconstructed.Status)
}

func TestReconstructUnitOfWorkMissingState(t *testing.T) {
	state := core.NewContext()
	_, ok := ReconstructUnitOfWorkFromCompiledExecution(state)
	require.False(t, ok)
}

func TestReconstructUnitOfWorkNilState(t *testing.T) {
	_, ok := ReconstructUnitOfWorkFromCompiledExecution(nil)
	require.False(t, ok)
}

// ---------------------------------------------------------------------------
// BuildRuntimeExecutionStatus
// ---------------------------------------------------------------------------

func TestBuildRuntimeExecutionStatusCopiesIDs(t *testing.T) {
	uow := buildTestUnitOfWork()
	uow.AssuranceClass = AssuranceClassRepairExhausted
	now := time.Now().UTC()

	res := BuildRuntimeExecutionStatus(uow, ExecutionStatusExecuting, ExecutionResultClassCompleted, now)

	require.Equal(t, uow.WorkflowID, res.WorkflowID)
	require.Equal(t, uow.RunID, res.RunID)
	require.Equal(t, uow.ExecutionID, res.ExecutionID)
	require.Equal(t, uow.ID, res.UnitOfWorkID)
	require.Equal(t, ExecutionStatusExecuting, res.Status)
	require.Equal(t, ExecutionResultClassCompleted, res.ResultClass)
	require.Equal(t, AssuranceClassRepairExhausted, res.AssuranceClass)
	require.Equal(t, now, res.UpdatedAt)
}

func TestBuildRuntimeExecutionStatusWithPlanBinding(t *testing.T) {
	uow := buildTestUnitOfWork()
	uow.PlanBinding = &UnitOfWorkPlanBinding{
		PlanID:       "plan-abc",
		PlanVersion:  2,
		ActiveStepID: "step-3",
	}

	res := BuildRuntimeExecutionStatus(uow, ExecutionStatusExecuting, ExecutionResultClassCompleted, time.Now())

	require.Equal(t, "plan-abc", res.ActivePlanID)
	require.Equal(t, 2, res.ActivePlanVersion)
	require.Equal(t, "step-3", res.ActiveStepID)
}

func TestBuildRuntimeExecutionStatusSetsUpdatedAtNow(t *testing.T) {
	uow := buildTestUnitOfWork()
	before := time.Now().UTC().Add(-time.Second)
	res := BuildRuntimeExecutionStatus(uow, ExecutionStatusCompleted, ExecutionResultClassCompleted, time.Time{})
	after := time.Now().UTC().Add(time.Second)

	require.True(t, res.UpdatedAt.After(before))
	require.True(t, res.UpdatedAt.Before(after))
}

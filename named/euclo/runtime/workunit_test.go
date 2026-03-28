package runtime

import (
	"testing"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	"github.com/lexcodex/relurpify/framework/core"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
)

func TestBuildUnitOfWorkDirectExecution(t *testing.T) {
	task := &core.Task{
		ID:          "task-1",
		Instruction: "fix a local bug",
		Context: map[string]any{
			"workspace": "/workspace",
			"paths":     []string{"app/main.go"},
		},
	}
	state := core.NewContext()
	envelope := TaskEnvelope{
		TaskID:             task.ID,
		Instruction:        task.Instruction,
		Workspace:          "/workspace",
		EditPermitted:      true,
		CapabilitySnapshot: euclotypes.CapabilitySnapshot{HasWriteTools: true, HasVerificationTools: true},
		ExecutionProfile:   "edit_verify_repair",
	}
	classification := TaskClassification{
		RecommendedMode: "code",
		EditPermitted:   true,
		Scope:           "local",
	}
	mode := euclotypes.ModeResolution{ModeID: "code"}
	profile := euclotypes.ExecutionProfileSelection{
		ProfileID:            "edit_verify_repair",
		VerificationRequired: true,
		PhaseRoutes:          map[string]string{"edit": "pipeline", "verify": "react"},
	}
	policy := ResolvedExecutionPolicy{
		ModeID:                      "code",
		ProfileID:                   "edit_verify_repair",
		PreferredVerifyCapabilities: []string{"verify.go_test"},
		RequireVerificationStep:     true,
		ResolvedFromSkillPolicy:     true,
	}

	uow := BuildUnitOfWork(task, state, envelope, classification, mode, profile, euclotypes.DefaultModeRegistry(), SemanticInputBundle{}, policy, WorkUnitExecutorDescriptor{})
	if uow.ObjectiveKind != "direct_execution" {
		t.Fatalf("got objective kind %q", uow.ObjectiveKind)
	}
	if uow.BehaviorFamily != "verification_repair" {
		t.Fatalf("got behavior family %q", uow.BehaviorFamily)
	}
	if uow.PlanBinding != nil {
		t.Fatalf("expected no plan binding, got %#v", uow.PlanBinding)
	}
	if len(uow.ToolBindings) == 0 || !uow.ToolBindings[0].Allowed {
		t.Fatalf("expected tool bindings from capability snapshot: %#v", uow.ToolBindings)
	}
	if len(uow.CapabilityBindings) == 0 {
		t.Fatal("expected capability bindings from phase routes")
	}
	if uow.ExecutorDescriptor.Family != ExecutorFamilyHTN {
		t.Fatalf("expected HTN executor from skill policy, got %#v", uow.ExecutorDescriptor)
	}
	if uow.ContextBundle.ContextBudgetClass != "light" {
		t.Fatalf("got context budget class %q", uow.ContextBundle.ContextBudgetClass)
	}
}

func TestBuildUnitOfWorkPlanBackedExecution(t *testing.T) {
	task := &core.Task{
		ID:          "task-plan",
		Instruction: "summarize current status",
		Context: map[string]any{
			"workspace":   "/workspace",
			"workflow_id": "wf-1",
			"run_id":      "run-1",
		},
	}
	state := core.NewContext()
	state.Set("euclo.workflow_id", "wf-1")
	state.Set("euclo.run_id", "run-1")
	state.Set("euclo.current_plan_step_id", "step-2")
	state.Set("euclo.pending_learning_ids", []string{"learn-1"})
	state.Set("pipeline.workflow_retrieval", map[string]any{"summary": "prior context"})
	state.Set("euclo.active_plan_version", &archaeodomain.VersionedLivingPlan{
		WorkflowID: "wf-1",
		Version:    3,
		Plan:       planFixture(),
	})
	envelope := TaskEnvelope{
		TaskID:             task.ID,
		Instruction:        task.Instruction,
		Workspace:          "/workspace",
		EditPermitted:      false,
		CapabilitySnapshot: euclotypes.CapabilitySnapshot{HasVerificationTools: true},
		ExecutionProfile:   "plan_stage_execute",
	}
	classification := TaskClassification{
		RecommendedMode:                "planning",
		EditPermitted:                  false,
		RequiresDeterministicStages:    true,
		RequiresEvidenceBeforeMutation: false,
		Scope:                          "cross_cutting",
	}
	mode := euclotypes.ModeResolution{ModeID: "planning"}
	profile := euclotypes.ExecutionProfileSelection{
		ProfileID:            "plan_stage_execute",
		VerificationRequired: false,
		PhaseRoutes:          map[string]string{"plan": "planner", "stage": "pipeline", "summarize": "react"},
	}
	semanticInputs := SemanticInputBundle{
		WorkflowID:              "wf-1",
		PatternRefs:             []string{"pattern-1"},
		TensionRefs:             []string{"tension-1"},
		LearningInteractionRefs: []string{"learn-1"},
	}
	policy := ResolvedExecutionPolicy{
		ModeID:                        "planning",
		ProfileID:                     "plan_stage_execute",
		PreferredPlanningCapabilities: []string{"planner.capability"},
		ResolvedFromSkillPolicy:       true,
	}

	uow := BuildUnitOfWork(task, state, envelope, classification, mode, profile, euclotypes.DefaultModeRegistry(), semanticInputs, policy, WorkUnitExecutorDescriptor{})
	if uow.ObjectiveKind != "plan_execution" {
		t.Fatalf("got objective kind %q", uow.ObjectiveKind)
	}
	if uow.BehaviorFamily != "gap_analysis" {
		t.Fatalf("got behavior family %q", uow.BehaviorFamily)
	}
	if uow.PlanBinding == nil {
		t.Fatal("expected plan binding")
	}
	if !uow.PlanBinding.IsPlanBacked || !uow.PlanBinding.IsLongRunning {
		t.Fatalf("unexpected plan binding flags: %#v", uow.PlanBinding)
	}
	if uow.PlanBinding.PlanID != "plan-1" || uow.PlanBinding.PlanVersion != 3 {
		t.Fatalf("unexpected plan binding: %#v", uow.PlanBinding)
	}
	if uow.ContextBundle.ContextBudgetClass != "heavy" {
		t.Fatalf("got context budget class %q", uow.ContextBundle.ContextBudgetClass)
	}
	if !uow.ContextBundle.CompactionEligible || !uow.ContextBundle.RestoreRequired {
		t.Fatalf("expected compaction/restore flags: %#v", uow.ContextBundle)
	}
	if len(uow.ContextBundle.LearningRefs) != 1 || uow.ContextBundle.LearningRefs[0] != "learn-1" {
		t.Fatalf("unexpected learning refs: %#v", uow.ContextBundle.LearningRefs)
	}
	if len(uow.SemanticInputs.PatternRefs) != 1 || uow.SemanticInputs.PatternRefs[0] != "pattern-1" {
		t.Fatalf("unexpected semantic inputs: %#v", uow.SemanticInputs)
	}
	if uow.ExecutorDescriptor.Family != ExecutorFamilyRewoo {
		t.Fatalf("expected rewoo executor, got %#v", uow.ExecutorDescriptor)
	}
	if len(uow.RoutineBindings) == 0 || uow.RoutineBindings[0].Family != "gap_analysis" {
		t.Fatalf("unexpected routine bindings: %#v", uow.RoutineBindings)
	}
}

func TestUnitOfWorkContextPayload(t *testing.T) {
	payload := UnitOfWorkContextPayload(UnitOfWork{
		ID:                "uow-1",
		WorkflowID:        "wf-1",
		RunID:             "run-1",
		ExecutionID:       "exec-1",
		ModeID:            "planning",
		ObjectiveKind:     "plan_execution",
		BehaviorFamily:    "gap_analysis",
		ContextStrategyID: "narrow_to_wide",
		ResultClass:       ExecutionResultClassCompletedWithDeferrals,
		Status:            UnitOfWorkStatusCompletedWithDeferrals,
		DeferredIssueIDs:  []string{"defer-1"},
	})
	if payload["id"] != "uow-1" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
	if payload["behavior_family"] != "gap_analysis" {
		t.Fatalf("unexpected payload family: %#v", payload)
	}
}

func planFixture() frameworkplan.LivingPlan {
	return frameworkplan.LivingPlan{
		ID:         "plan-1",
		WorkflowID: "wf-1",
		StepOrder:  []string{"step-1", "step-2"},
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {ID: "step-1", Status: frameworkplan.PlanStepCompleted},
			"step-2": {ID: "step-2", Status: frameworkplan.PlanStepPending},
		},
	}
}

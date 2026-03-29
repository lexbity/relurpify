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
	if uow.PrimaryRelurpicCapabilityID != "euclo:chat.implement" {
		t.Fatalf("got primary relurpic capability %q", uow.PrimaryRelurpicCapabilityID)
	}
	if len(uow.SupportingRelurpicCapabilityIDs) == 0 {
		t.Fatal("expected supporting relurpic capability ids")
	}
	if !containsStringSlice(uow.SupportingRelurpicCapabilityIDs, "euclo:chat.inspect") {
		t.Fatalf("expected chat inspect support, got %#v", uow.SupportingRelurpicCapabilityIDs)
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
	if uow.PrimaryRelurpicCapabilityID != "euclo:archaeology.implement-plan" {
		t.Fatalf("got primary relurpic capability %q", uow.PrimaryRelurpicCapabilityID)
	}
	if !containsStringSlice(uow.SupportingRelurpicCapabilityIDs, "euclo:archaeology.compile-plan") {
		t.Fatalf("expected archaeology compile-plan support, got %#v", uow.SupportingRelurpicCapabilityIDs)
	}
	if !containsStringSlice(uow.SupportingRelurpicCapabilityIDs, "euclo:archaeology.explore") {
		t.Fatalf("expected archaeology explore support, got %#v", uow.SupportingRelurpicCapabilityIDs)
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
		ID:                              "uow-1",
		WorkflowID:                      "wf-1",
		RunID:                           "run-1",
		ExecutionID:                     "exec-1",
		ModeID:                          "planning",
		ObjectiveKind:                   "plan_execution",
		BehaviorFamily:                  "gap_analysis",
		ContextStrategyID:               "narrow_to_wide",
		PrimaryRelurpicCapabilityID:     "euclo:archaeology.implement-plan",
		SupportingRelurpicCapabilityIDs: []string{"euclo:archaeology.compile-plan", "euclo:archaeology.explore"},
		ResultClass:                     ExecutionResultClassCompletedWithDeferrals,
		Status:                          UnitOfWorkStatusCompletedWithDeferrals,
		DeferredIssueIDs:                []string{"defer-1"},
		SemanticInputs: SemanticInputBundle{
			PatternRefs: []string{"pattern-a"},
			TensionRefs: []string{"tension-a"},
			PatternProposals: []PatternProposalSummary{{
				ProposalID:         "proposal-a",
				Title:              "Pattern proposal",
				PatternRefs:        []string{"pattern-a"},
				RelatedTensionRefs: []string{"tension-a"},
			}},
			TensionClusters: []TensionClusterSummary{{
				ClusterID:   "cluster-a",
				Title:       "Tension cluster",
				Severity:    "medium",
				TensionRefs: []string{"tension-a"},
			}},
			CoherenceSuggestions: []CoherenceSuggestion{{
				SuggestionID:    "coherence-a",
				Title:           "Check touched symbols",
				SuggestedAction: "re-verify the changed files",
				TouchedSymbols:  []string{"pkg/service.go"},
			}},
			ProspectivePairings: []ProspectivePairingSummary{{
				PairingID:      "pair-a",
				Title:          "Prospective pairing",
				ProspectiveRef: "req-prospect",
			}},
		},
	})
	if payload["id"] != "uow-1" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
	if payload["behavior_family"] != "gap_analysis" {
		t.Fatalf("unexpected payload family: %#v", payload)
	}
	if payload["primary_relurpic_capability_id"] != "euclo:archaeology.implement-plan" {
		t.Fatalf("unexpected primary relurpic capability payload: %#v", payload)
	}
	if ids, ok := payload["supporting_relurpic_capability_ids"].([]string); !ok || len(ids) != 2 {
		t.Fatalf("unexpected supporting relurpic payload: %#v", payload["supporting_relurpic_capability_ids"])
	}
	semanticInputs, ok := payload["semantic_inputs"].(map[string]any)
	if !ok {
		t.Fatalf("expected semantic inputs payload: %#v", payload)
	}
	if _, ok := semanticInputs["pattern_proposals"]; !ok {
		t.Fatalf("expected pattern proposals in semantic payload: %#v", semanticInputs)
	}
	if _, ok := semanticInputs["tension_clusters"]; !ok {
		t.Fatalf("expected tension clusters in semantic payload: %#v", semanticInputs)
	}
	if _, ok := semanticInputs["coherence_suggestions"]; !ok {
		t.Fatalf("expected coherence suggestions in semantic payload: %#v", semanticInputs)
	}
	if _, ok := semanticInputs["prospective_pairings"]; !ok {
		t.Fatalf("expected prospective pairings in semantic payload: %#v", semanticInputs)
	}
}

func TestBuildUnitOfWorkDebugModeAddsDebugReasoningFamilies(t *testing.T) {
	task := &core.Task{
		ID:          "task-debug",
		Instruction: "reproduce and fix the failing test",
		Context: map[string]any{
			"workspace": "/workspace",
			"mode":      "debug",
		},
	}
	state := core.NewContext()
	envelope := TaskEnvelope{
		TaskID:           task.ID,
		Instruction:      task.Instruction,
		Workspace:        "/workspace",
		ExecutionProfile: "reproduce_localize_patch",
	}
	classification := TaskClassification{
		RecommendedMode:                "debug",
		RequiresEvidenceBeforeMutation: true,
	}
	mode := euclotypes.ModeResolution{ModeID: "debug"}
	profile := euclotypes.ExecutionProfileSelection{
		ProfileID:            "reproduce_localize_patch",
		VerificationRequired: true,
	}
	uow := BuildUnitOfWork(task, state, envelope, classification, mode, profile, euclotypes.DefaultModeRegistry(), SemanticInputBundle{}, ResolvedExecutionPolicy{}, WorkUnitExecutorDescriptor{})
	requireRoutineFamily(t, uow.RoutineBindings, "tension_assessment")
	requireRoutineFamily(t, uow.RoutineBindings, "stale_assumption_detection")
	requireRoutineFamily(t, uow.RoutineBindings, "verification_repair")
}

func TestBuildUnitOfWorkReviewModeAddsReviewReasoningFamilies(t *testing.T) {
	task := &core.Task{
		ID:          "task-review",
		Instruction: "review the proposed change",
		Context:     map[string]any{"workspace": "/workspace", "mode": "review"},
	}
	uow := BuildUnitOfWork(task, core.NewContext(), TaskEnvelope{
		TaskID:           task.ID,
		Instruction:      task.Instruction,
		Workspace:        "/workspace",
		ExecutionProfile: "review_suggest_implement",
	}, TaskClassification{RecommendedMode: "review"}, euclotypes.ModeResolution{ModeID: "review"}, euclotypes.ExecutionProfileSelection{
		ProfileID: "review_suggest_implement",
	}, euclotypes.DefaultModeRegistry(), SemanticInputBundle{}, ResolvedExecutionPolicy{
		ModeID:         "review",
		ProfileID:      "review_suggest_implement",
		ReviewCriteria: []string{"backward_compatibility"},
	}, WorkUnitExecutorDescriptor{})
	requireRoutineFamily(t, uow.RoutineBindings, "tension_assessment")
	requireRoutineFamily(t, uow.RoutineBindings, "coherence_assessment")
	requireRoutineFamily(t, uow.RoutineBindings, "compatibility_assessment")
	requireRoutineFamily(t, uow.RoutineBindings, "approval_assessment")
}

func TestBuildUnitOfWorkPlanningModeAddsPlanningReasoningFamilies(t *testing.T) {
	task := &core.Task{
		ID:          "task-planning-routines",
		Instruction: "plan the migration",
		Context:     map[string]any{"workspace": "/workspace", "mode": "planning"},
	}
	uow := BuildUnitOfWork(task, core.NewContext(), TaskEnvelope{
		TaskID:           task.ID,
		Instruction:      task.Instruction,
		Workspace:        "/workspace",
		ExecutionProfile: "plan_stage_execute",
	}, TaskClassification{RecommendedMode: "planning"}, euclotypes.ModeResolution{ModeID: "planning"}, euclotypes.ExecutionProfileSelection{
		ProfileID: "plan_stage_execute",
	}, euclotypes.DefaultModeRegistry(), SemanticInputBundle{}, ResolvedExecutionPolicy{
		ModeID:                        "planning",
		ProfileID:                     "plan_stage_execute",
		PreferredPlanningCapabilities: []string{"planner.capability"},
	}, WorkUnitExecutorDescriptor{})
	requireRoutineFamily(t, uow.RoutineBindings, "pattern_surface_and_confirm")
	requireRoutineFamily(t, uow.RoutineBindings, "prospective_structure_assessment")
	requireRoutineFamily(t, uow.RoutineBindings, "convergence_guard")
	requireRoutineFamily(t, uow.RoutineBindings, "coherence_assessment")
	requireRoutineFamily(t, uow.RoutineBindings, "scope_expansion_assessment")
}

func TestBuildUnitOfWorkSelectsChatAskCapability(t *testing.T) {
	task := &core.Task{
		ID:          "task-ask",
		Instruction: "how does the auth middleware work?",
		Context:     map[string]any{"workspace": "/workspace"},
	}
	uow := BuildUnitOfWork(task, core.NewContext(), TaskEnvelope{
		TaskID:           task.ID,
		Instruction:      task.Instruction,
		Workspace:        "/workspace",
		ExecutionProfile: "edit_verify_repair",
		EditPermitted:    true,
	}, TaskClassification{
		RecommendedMode: "code",
		IntentFamilies:  []string{"code"},
		EditPermitted:   true,
	}, euclotypes.ModeResolution{ModeID: "code"}, euclotypes.ExecutionProfileSelection{
		ProfileID: "edit_verify_repair",
	}, euclotypes.DefaultModeRegistry(), SemanticInputBundle{}, ResolvedExecutionPolicy{}, WorkUnitExecutorDescriptor{})
	if uow.PrimaryRelurpicCapabilityID != "euclo:chat.ask" {
		t.Fatalf("got primary relurpic capability %q", uow.PrimaryRelurpicCapabilityID)
	}
}

func TestBuildUnitOfWorkSelectsChatInspectCapability(t *testing.T) {
	task := &core.Task{
		ID:          "task-inspect",
		Instruction: "inspect the parser error handling",
		Context:     map[string]any{"workspace": "/workspace"},
	}
	uow := BuildUnitOfWork(task, core.NewContext(), TaskEnvelope{
		TaskID:           task.ID,
		Instruction:      task.Instruction,
		Workspace:        "/workspace",
		ExecutionProfile: "edit_verify_repair",
		EditPermitted:    true,
	}, TaskClassification{
		RecommendedMode: "code",
		IntentFamilies:  []string{"review", "code"},
		EditPermitted:   true,
	}, euclotypes.ModeResolution{ModeID: "code"}, euclotypes.ExecutionProfileSelection{
		ProfileID: "edit_verify_repair",
	}, euclotypes.DefaultModeRegistry(), SemanticInputBundle{}, ResolvedExecutionPolicy{}, WorkUnitExecutorDescriptor{})
	if uow.PrimaryRelurpicCapabilityID != "euclo:chat.inspect" {
		t.Fatalf("got primary relurpic capability %q", uow.PrimaryRelurpicCapabilityID)
	}
}

func TestBuildUnitOfWorkSelectsArchaeologyCompilePlanCapability(t *testing.T) {
	task := &core.Task{
		ID:          "task-compile-plan",
		Instruction: "finalize the plan for the migration",
		Context:     map[string]any{"workspace": "/workspace", "mode": "planning"},
	}
	uow := BuildUnitOfWork(task, core.NewContext(), TaskEnvelope{
		TaskID:           task.ID,
		Instruction:      task.Instruction,
		Workspace:        "/workspace",
		ExecutionProfile: "plan_stage_execute",
	}, TaskClassification{
		RecommendedMode: "planning",
		IntentFamilies:  []string{"planning"},
	}, euclotypes.ModeResolution{ModeID: "planning"}, euclotypes.ExecutionProfileSelection{
		ProfileID: "plan_stage_execute",
	}, euclotypes.DefaultModeRegistry(), SemanticInputBundle{}, ResolvedExecutionPolicy{}, WorkUnitExecutorDescriptor{})
	if uow.PrimaryRelurpicCapabilityID != "euclo:archaeology.compile-plan" {
		t.Fatalf("got primary relurpic capability %q", uow.PrimaryRelurpicCapabilityID)
	}
	if !containsStringSlice(uow.SupportingRelurpicCapabilityIDs, "euclo:archaeology.pattern-surface") {
		t.Fatalf("expected archaeology pattern surface support, got %#v", uow.SupportingRelurpicCapabilityIDs)
	}
	if !containsStringSlice(uow.SupportingRelurpicCapabilityIDs, "euclo:archaeology.convergence-guard") {
		t.Fatalf("expected archaeology convergence support, got %#v", uow.SupportingRelurpicCapabilityIDs)
	}
}

func TestBuildUnitOfWorkSelectsDebugSupportingCapabilities(t *testing.T) {
	task := &core.Task{
		ID:          "task-debug-support",
		Instruction: "debug the failing login flow and localize the bug",
		Context:     map[string]any{"workspace": "/workspace", "mode": "debug"},
	}
	uow := BuildUnitOfWork(task, core.NewContext(), TaskEnvelope{
		TaskID:           task.ID,
		Instruction:      task.Instruction,
		Workspace:        "/workspace",
		ExecutionProfile: "reproduce_localize_patch",
	}, TaskClassification{
		RecommendedMode:                "debug",
		IntentFamilies:                 []string{"debug", "code"},
		RequiresEvidenceBeforeMutation: true,
	}, euclotypes.ModeResolution{ModeID: "debug"}, euclotypes.ExecutionProfileSelection{
		ProfileID: "reproduce_localize_patch",
	}, euclotypes.DefaultModeRegistry(), SemanticInputBundle{}, ResolvedExecutionPolicy{}, WorkUnitExecutorDescriptor{})
	if uow.PrimaryRelurpicCapabilityID != "euclo:debug.investigate" {
		t.Fatalf("got primary relurpic capability %q", uow.PrimaryRelurpicCapabilityID)
	}
	for _, id := range []string{
		"euclo:debug.root-cause",
		"euclo:debug.hypothesis-refine",
		"euclo:debug.localization",
		"euclo:debug.flaw-surface",
		"euclo:debug.verification-repair",
	} {
		if !containsStringSlice(uow.SupportingRelurpicCapabilityIDs, id) {
			t.Fatalf("missing supporting relurpic capability %q in %#v", id, uow.SupportingRelurpicCapabilityIDs)
		}
	}
}

func TestBuildUnitOfWorkAddsLazyArchaeologySupportForCrossCuttingChatImplement(t *testing.T) {
	task := &core.Task{
		ID:          "task-cross-cutting-implement",
		Instruction: "implement the auth redesign across multiple handlers",
		Context:     map[string]any{"workspace": "/workspace"},
	}
	uow := BuildUnitOfWork(task, core.NewContext(), TaskEnvelope{
		TaskID:           task.ID,
		Instruction:      task.Instruction,
		Workspace:        "/workspace",
		ExecutionProfile: "edit_verify_repair",
		EditPermitted:    true,
	}, TaskClassification{
		RecommendedMode: "code",
		IntentFamilies:  []string{"code", "planning"},
		EditPermitted:   true,
		Scope:           "cross_cutting",
	}, euclotypes.ModeResolution{ModeID: "code"}, euclotypes.ExecutionProfileSelection{
		ProfileID: "edit_verify_repair",
	}, euclotypes.DefaultModeRegistry(), SemanticInputBundle{}, ResolvedExecutionPolicy{}, WorkUnitExecutorDescriptor{})
	if !containsStringSlice(uow.SupportingRelurpicCapabilityIDs, "euclo:archaeology.explore") {
		t.Fatalf("expected lazy archaeology support, got %#v", uow.SupportingRelurpicCapabilityIDs)
	}
}

func requireRoutineFamily(t *testing.T, bindings []UnitOfWorkRoutineBinding, family string) {
	t.Helper()
	for _, binding := range bindings {
		if binding.Family == family {
			return
		}
	}
	t.Fatalf("missing routine family %q in %#v", family, bindings)
}

func containsStringSlice(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
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

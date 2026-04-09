package euclo

import (
	"context"
	"testing"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeolearning "github.com/lexcodex/relurpify/archaeo/learning"
	archaeoprojections "github.com/lexcodex/relurpify/archaeo/projections"
	"github.com/lexcodex/relurpify/framework/core"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
)

func TestShouldUseSummaryStatusFastPath(t *testing.T) {
	agent := &Agent{}
	task := &core.Task{Instruction: "Current status update please"}
	classification := eucloruntime.TaskClassification{}
	profile := eucloruntime.ExecutionProfileSelection{ProfileID: "plan_stage_execute"}

	if !agent.shouldUseSummaryStatusFastPath(task, classification, profile) {
		t.Fatal("expected summary/status instructions to trigger fast path")
	}
	if agent.shouldUseSummaryStatusFastPath(nil, classification, profile) {
		t.Fatal("expected nil task to bypass fast path")
	}
	if agent.shouldUseSummaryStatusFastPath(task, eucloruntime.TaskClassification{RequiresEvidenceBeforeMutation: true}, profile) {
		t.Fatal("expected evidence requirement to disable fast path")
	}
	if agent.shouldUseSummaryStatusFastPath(task, classification, eucloruntime.ExecutionProfileSelection{ProfileID: "other"}) {
		t.Fatal("expected non plan_stage_execute profile to bypass fast path")
	}
}

func TestExecutionPreparationHelpers(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.archaeo_phase_state", archaeodomain.WorkflowPhaseState{CurrentPhase: archaeodomain.PhaseIntentElicitation})

	if !hasTerminalExecutionPreparation(executionPreparation{summaryFastPath: true}) {
		t.Fatal("expected summary fast path to be terminal")
	}
	if !hasTerminalExecutionPreparation(executionPreparation{skipReason: "summary/status request completed from cached execution state"}) {
		t.Fatal("expected skip reason to be terminal")
	}
	if hasTerminalExecutionPreparation(executionPreparation{}) {
		t.Fatal("expected empty preparation to be non-terminal")
	}

	if !shouldShortCircuitExecution(state) {
		t.Fatal("expected intent elicitation phase to short-circuit")
	}
	state.Set("euclo.archaeo_phase_state", archaeodomain.WorkflowPhaseState{CurrentPhase: archaeodomain.PhaseSurfacing})
	if !shouldShortCircuitExecution(state) {
		t.Fatal("expected surfacing phase to short-circuit")
	}
	state.Set("euclo.archaeo_phase_state", archaeodomain.WorkflowPhaseState{CurrentPhase: archaeodomain.PhaseExecution})
	if shouldShortCircuitExecution(state) {
		t.Fatal("expected execution phase not to short-circuit")
	}
	if shouldShortCircuitExecution(nil) {
		t.Fatal("expected nil state not to short-circuit")
	}
}

func TestSummaryFastPathWithoutWorkflow(t *testing.T) {
	agent := &Agent{}
	prep := agent.prepareExecution(
		context.Background(),
		&core.Task{Instruction: "Please provide current status"},
		nil,
		eucloruntime.TaskClassification{},
		eucloruntime.ExecutionProfileSelection{ProfileID: "plan_stage_execute"},
	)
	if !prep.summaryFastPath {
		t.Fatal("expected summary fast path to be enabled")
	}
	if prep.skipReason != "summary/status request completed without explicit workflow" {
		t.Fatalf("expected explicit-workflow skip reason, got %q", prep.skipReason)
	}
	if prep.readBundle != nil || prep.livingPlan != nil || prep.activeStep != nil || prep.preflightResult != nil || prep.err != nil {
		t.Fatalf("unexpected preparation state: %#v", prep)
	}
}

func TestSummaryFastPathCachedBundleBranches(t *testing.T) {
	agent := &Agent{}
	task := &core.Task{Context: map[string]any{"workflow_id": "wf-1"}, Instruction: "Current status update please"}

	t.Run("non-blocking", func(t *testing.T) {
		state := core.NewContext()
		bundle := &executionReadBundle{
			workflowID: "wf-1",
			learningQueue: &archaeoprojections.LearningQueueProjection{
				PendingLearning: []archaeolearning.Interaction{{ID: "learn-1"}},
			},
			activePlan: &archaeoprojections.ActivePlanProjection{
				PhaseState: &archaeodomain.WorkflowPhaseState{CurrentPhase: archaeodomain.PhaseExecution},
				ActivePlanVersion: &archaeodomain.VersionedLivingPlan{
					Plan: frameworkplan.LivingPlan{},
				},
			},
		}

		prep := agent.finalizeSummaryFastPathExecution(task, state, executionPreparation{workflowID: "wf-1"}, bundle)
		if !prep.summaryFastPath {
			t.Fatal("expected cached execution state to short-circuit")
		}
		if prep.skipReason != "summary/status request completed from cached execution state" {
			t.Fatalf("unexpected skip reason: %q", prep.skipReason)
		}
		if prep.readBundle != bundle {
			t.Fatal("expected cached bundle to be recorded")
		}
		if raw, ok := state.Get("euclo.execution_read_bundle"); !ok || raw != bundle {
			t.Fatalf("expected bundle to be seeded into state, got %#v", raw)
		}
		if _, ok := state.Get("euclo.pending_learning_ids"); !ok {
			t.Fatal("expected pending learning ids to be seeded")
		}
		if _, ok := state.Get("euclo.phase_state"); !ok {
			t.Fatal("expected phase state to be seeded")
		}
	})

	t.Run("blocking", func(t *testing.T) {
		state := core.NewContext()
		bundle := &executionReadBundle{
			workflowID: "wf-1",
			learningQueue: &archaeoprojections.LearningQueueProjection{
				PendingGuidanceIDs: []string{"guide-1"},
			},
		}

		prep := agent.finalizeSummaryFastPathExecution(task, state, executionPreparation{workflowID: "wf-1"}, bundle)
		if prep.summaryFastPath {
			t.Fatal("expected blocking cached state to fall back to living plan")
		}
		if prep.skipReason != "" {
			t.Fatalf("expected no skip reason for blocking cache, got %q", prep.skipReason)
		}
		if prep.readBundle != bundle {
			t.Fatal("expected cached bundle to remain available for fallback")
		}
		if _, ok := state.Get("euclo.pending_guidance_ids"); !ok {
			t.Fatal("expected pending guidance ids to be seeded")
		}
	})
}

func TestLivingPlanFallbackNotes(t *testing.T) {
	agent := &Agent{}

	if got := agent.executionPreparationNote(&core.Task{}, core.NewContext()); got != "execution preparation note: workflow id unavailable" {
		t.Fatalf("expected workflow note, got %q", got)
	}
	if got := agent.executionPreparationNote(&core.Task{Context: map[string]any{"workflow_id": "wf-1"}}, core.NewContext()); got != "execution preparation note: plan store unavailable" {
		t.Fatalf("expected plan store note, got %q", got)
	}
	if lp, step, result, err, reason, note := agent.prepareLivingPlan(context.Background(), &core.Task{Context: map[string]any{"workflow_id": "wf-1"}}, core.NewContext()); lp != nil || step != nil || result != nil || err != nil || reason != "" || note != "execution preparation note: plan store unavailable" {
		t.Fatalf("expected missing plan store note, got %#v %#v %#v %v %q %q", lp, step, result, err, reason, note)
	}
	if lp, step, result, err, reason, note := agent.prepareLivingPlan(context.Background(), &core.Task{}, core.NewContext()); lp != nil || step != nil || result != nil || err != nil || reason != "" || note != "execution preparation note: workflow id unavailable" {
		t.Fatalf("expected missing workflow note, got %#v %#v %#v %v %q %q", lp, step, result, err, reason, note)
	}
}

func TestPrepareExecutionAndLivingPlanFallbacks(t *testing.T) {
	agent := &Agent{}

	normalPrep := agent.prepareExecution(
		context.Background(),
		&core.Task{Context: map[string]any{"workflow_id": "wf-123"}},
		nil,
		eucloruntime.TaskClassification{},
		eucloruntime.ExecutionProfileSelection{},
	)
	if normalPrep.summaryFastPath {
		t.Fatal("expected normal preparation path")
	}
	if normalPrep.skipReason != "" {
		t.Fatalf("expected no skip reason for missing plan store, got %q", normalPrep.skipReason)
	}
	if normalPrep.preparationNote != "execution preparation note: plan store unavailable" {
		t.Fatalf("expected plan store preparation note, got %q", normalPrep.preparationNote)
	}
	if normalPrep.workflowID != "wf-123" {
		t.Fatalf("expected workflow id to be captured, got %q", normalPrep.workflowID)
	}
	if normalPrep.livingPlan != nil || normalPrep.activeStep != nil || normalPrep.preflightResult != nil || normalPrep.err != nil {
		t.Fatalf("expected nil living plan fallback, got %#v", normalPrep)
	}
	if shouldShortCircuitExecution(nil) {
		t.Fatal("expected missing plan store note not to short-circuit execution")
	}
}

func TestShortCircuitResultUsesSkipReason(t *testing.T) {
	agent := &Agent{}
	state := core.NewContext()
	state.Set("euclo.learning_queue", []string{"learn-1"})
	state.Set("euclo.pending_learning_ids", []string{"learn-1"})

	result := agent.shortCircuitResult(state, executionPreparation{
		summaryFastPath: true,
		skipReason:      "summary/status request completed from cached execution state",
	})
	if result == nil {
		t.Fatal("expected a result")
	}
	if got := result.Metadata["summary"]; got != "summary/status request completed from cached execution state" {
		t.Fatalf("unexpected summary metadata: %#v", got)
	}
	if got := result.Data["pending_learning_ids"]; got == nil {
		t.Fatal("expected pending learning ids in short-circuit result")
	}
}

func TestLoadExecutionReadBundleAndSeedStateFallbacks(t *testing.T) {
	agent := &Agent{}
	if bundle, ok := agent.loadExecutionReadBundle(context.Background(), ""); bundle != nil || ok {
		t.Fatalf("expected blank workflow id to bypass bundle loading, got %#v %v", bundle, ok)
	}
	if bundle, ok := (*Agent)(nil).loadExecutionReadBundle(context.Background(), "wf-1"); bundle != nil || ok {
		t.Fatalf("expected nil agent to bypass bundle loading, got %#v %v", bundle, ok)
	}

	state := core.NewContext()
	bundle := &executionReadBundle{
		workflowID: "wf-1",
		learningQueue: &archaeoprojections.LearningQueueProjection{
			PendingLearning:    []archaeolearning.Interaction{{ID: "learn-1"}},
			PendingGuidanceIDs: []string{"guide-1"},
			BlockingLearning:   []string{"block-1"},
		},
		activePlan: &archaeoprojections.ActivePlanProjection{
			PhaseState: &archaeodomain.WorkflowPhaseState{CurrentPhase: archaeodomain.PhaseExecution},
			ActivePlanVersion: &archaeodomain.VersionedLivingPlan{
				Plan: frameworkplan.LivingPlan{},
			},
		},
	}
	agent.seedExecutionReadBundleState(state, bundle)
	if raw, ok := state.Get("euclo.phase_state"); !ok || raw == nil {
		t.Fatal("expected phase state to be seeded")
	}
	if raw, ok := state.Get("euclo.execution_read_bundle"); !ok || raw == nil {
		t.Fatal("expected execution read bundle to be stored in state")
	}
	if _, ok := state.Get("euclo.pending_learning_ids"); !ok {
		t.Fatal("expected pending learning ids to be seeded")
	}
	if _, ok := state.Get("euclo.pending_guidance_ids"); !ok {
		t.Fatal("expected pending guidance ids to be seeded")
	}
	if _, ok := state.Get("euclo.active_plan_version"); !ok {
		t.Fatal("expected active plan version to be seeded")
	}
	if _, ok := state.Get("euclo.living_plan"); !ok {
		t.Fatal("expected living plan to be seeded")
	}
}

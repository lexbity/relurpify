//go:build integration

package archaeology_test

import (
	"context"
	"testing"
	"time"

	archaeoconvergence "github.com/lexcodex/relurpify/archaeo/convergence"
	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeoexec "github.com/lexcodex/relurpify/archaeo/execution"
	archaeolearning "github.com/lexcodex/relurpify/archaeo/learning"
	archaeoplans "github.com/lexcodex/relurpify/archaeo/plans"
	archaeotensions "github.com/lexcodex/relurpify/archaeo/tensions"
	"github.com/lexcodex/relurpify/archaeo/testscenario"
	"github.com/lexcodex/relurpify/framework/core"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
)

func TestArchaeoPhases_SurfaceToConverge_HappyPath(t *testing.T) {
	f := testscenario.NewFixture(t)
	f.SeedWorkflow("wf-phases-happy", "run archaeo surface to converge happy path")

	plan := integrationPlan("plan-phases-happy", "wf-phases-happy")
	f.SeedActivePlan(plan, archaeoplans.DraftVersionInput{
		WorkflowID:          "wf-phases-happy",
		BasedOnRevision:     "rev-1",
		SemanticSnapshotRef: "semantic-1",
	})

	svc := f.ArchaeologyService()
	svc.PersistPhase = f.PersistPhaseFunc()
	svc.EvaluateGate = allowIntegrationPreflight

	state := f.NewState()
	task := f.Task("wf-phases-happy", "run archaeo surface to converge happy path", map[string]any{
		"based_on_revision":     "rev-1",
		"semantic_snapshot_ref": "semantic-1",
	})
	out := svc.PrepareLivingPlan(context.Background(), task, state, "wf-phases-happy")
	if out.Err != nil {
		t.Fatalf("PrepareLivingPlan: %v", out.Err)
	}
	if out.Plan == nil {
		t.Fatalf("expected active plan and step, got %+v", out)
	}
	step := out.Step
	if step == nil {
		step = out.Plan.Steps["inspect"]
	}
	if step == nil {
		t.Fatalf("expected inspect step in plan, got %+v", out.Plan.Steps)
	}
	testscenario.RequirePhase(t, f.PhaseService(), "wf-phases-happy", archaeodomain.PhaseSurfacing)

	f.SeedMutation(archaeodomain.MutationEvent{
		ID:          "mutation-happy",
		WorkflowID:  "wf-phases-happy",
		PlanID:      out.Plan.ID,
		PlanVersion: intPtr(out.Plan.Version),
		Category:    archaeodomain.MutationObservation,
		SourceKind:  "comment",
		SourceRef:   "note-1",
		Description: "observation only",
		Impact:      archaeodomain.ImpactInformational,
		Disposition: archaeodomain.DispositionContinue,
		CreatedAt:   f.Now(),
	})
	coord := archaeoexec.LiveMutationCoordinator{
		Service: f.ExecutionService(),
		Plans:   f.PlansService(),
	}
	eval, err := coord.CheckpointExecutionAt(context.Background(), archaeodomain.MutationCheckpointPreVerification, task, state, out.Plan, step)
	if err != nil {
		t.Fatalf("CheckpointExecutionAt: %v", err)
	}
	if eval == nil || eval.Disposition != archaeodomain.DispositionContinue {
		t.Fatalf("unexpected mutation evaluation: %+v", eval)
	}

	interaction, err := f.LearningService().Create(context.Background(), archaeolearning.CreateInput{
		WorkflowID:      "wf-phases-happy",
		ExplorationID:   state.GetString("euclo.active_exploration_id"),
		SnapshotID:      state.GetString("euclo.active_exploration_snapshot_id"),
		Kind:            archaeolearning.InteractionIntentRefinement,
		SubjectType:     archaeolearning.SubjectExploration,
		SubjectID:       state.GetString("euclo.active_exploration_id"),
		Title:           "Confirm intended repair direction",
		Blocking:        false,
		BasedOnRevision: "rev-1",
	})
	if err != nil {
		t.Fatalf("Learning Create: %v", err)
	}
	testscenario.RequirePendingLearningIDs(t, f.LearningService(), "wf-phases-happy", interaction.ID)
	if _, err := f.LearningService().Resolve(context.Background(), archaeolearning.ResolveInput{
		WorkflowID:      "wf-phases-happy",
		InteractionID:   interaction.ID,
		Kind:            archaeolearning.ResolutionConfirm,
		ResolvedBy:      "integration-test",
		BasedOnRevision: "rev-1",
	}); err != nil {
		t.Fatalf("Learning Resolve: %v", err)
	}
	testscenario.RequirePendingLearningIDs(t, f.LearningService(), "wf-phases-happy")

	record, err := f.ConvergenceService().Create(context.Background(), archaeoconvergence.CreateInput{
		WorkspaceID:   f.Workspace,
		WorkflowID:    "wf-phases-happy",
		ExplorationID: state.GetString("euclo.active_exploration_id"),
		PlanID:        out.Plan.ID,
		PlanVersion:   intPtr(out.Plan.Version),
		Question:      "Can this plan converge cleanly?",
	})
	if err != nil {
		t.Fatalf("Convergence Create: %v", err)
	}
	if _, err := f.ConvergenceService().Resolve(context.Background(), archaeoconvergence.ResolveInput{
		WorkflowID: "wf-phases-happy",
		RecordID:   record.ID,
		Resolution: archaeodomain.ConvergenceResolution{
			Status:       archaeodomain.ConvergenceResolutionResolved,
			ChosenOption: "ship",
			Summary:      "all blocking issues are resolved",
		},
	}); err != nil {
		t.Fatalf("Convergence Resolve: %v", err)
	}
	testscenario.RequireConvergenceState(t, f.ConvergenceService(), f.Workspace, archaeodomain.ConvergenceResolutionResolved)
	testscenario.RequireActiveTensionIDs(t, f.TensionService(), "wf-phases-happy")
}

func TestArchaeoPhases_TensionBlocking_ConvergenceRemainsOpen(t *testing.T) {
	f := testscenario.NewFixture(t)
	f.SeedWorkflow("wf-phases-blocked", "run archaeo blocked convergence flow")

	plan := integrationPlan("plan-phases-blocked", "wf-phases-blocked")
	f.SeedActivePlan(plan, archaeoplans.DraftVersionInput{
		WorkflowID:      "wf-phases-blocked",
		BasedOnRevision: "rev-1",
	})

	svc := f.ArchaeologyService()
	svc.PersistPhase = f.PersistPhaseFunc()
	svc.EvaluateGate = allowIntegrationPreflight

	state := f.NewState()
	task := f.Task("wf-phases-blocked", "run archaeo blocked convergence flow", map[string]any{
		"based_on_revision": "rev-1",
	})
	out := svc.PrepareLivingPlan(context.Background(), task, state, "wf-phases-blocked")
	if out.Err != nil {
		t.Fatalf("PrepareLivingPlan: %v", out.Err)
	}

	tension := f.SeedTension(archaeotensions.CreateInput{
		WorkflowID:      "wf-phases-blocked",
		ExplorationID:   state.GetString("euclo.active_exploration_id"),
		SnapshotID:      state.GetString("euclo.active_exploration_snapshot_id"),
		SourceRef:       "gap:blocked",
		Kind:            "intent_gap",
		Description:     "blocking tension remains unresolved",
		Status:          archaeodomain.TensionUnresolved,
		BasedOnRevision: "rev-1",
	})
	testscenario.RequireActiveTensionIDs(t, f.TensionService(), "wf-phases-blocked", tension.ID)

	if _, err := f.ConvergenceService().Create(context.Background(), archaeoconvergence.CreateInput{
		WorkspaceID:        f.Workspace,
		WorkflowID:         "wf-phases-blocked",
		ExplorationID:      state.GetString("euclo.active_exploration_id"),
		PlanID:             out.Plan.ID,
		PlanVersion:        intPtr(out.Plan.Version),
		Question:           "Can we ignore the unresolved tension?",
		RelevantTensionIDs: []string{tension.ID},
	}); err != nil {
		t.Fatalf("Convergence Create: %v", err)
	}
	testscenario.RequireConvergenceState(t, f.ConvergenceService(), f.Workspace, archaeodomain.ConvergenceResolutionOpen)
	testscenario.RequirePhase(t, f.PhaseService(), "wf-phases-blocked", archaeodomain.PhaseSurfacing)
}

func integrationPlan(planID, workflowID string) *frameworkplan.LivingPlan {
	now := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	return &frameworkplan.LivingPlan{
		ID:         planID,
		WorkflowID: workflowID,
		Title:      "Integration plan",
		Steps: map[string]*frameworkplan.PlanStep{
			"inspect": {
				ID:              "inspect",
				Description:     "Inspect current implementation",
				ConfidenceScore: 0.9,
				Status:          frameworkplan.PlanStepPending,
				CreatedAt:       now,
				UpdatedAt:       now,
			},
		},
		StepOrder: []string{"inspect"},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func allowIntegrationPreflight(context.Context, *core.Task, *core.Context, *frameworkplan.LivingPlan, *frameworkplan.PlanStep) (archaeoexec.PreflightOutcome, error) {
	return archaeoexec.PreflightOutcome{}, nil
}

func intPtr(value int) *int {
	copy := value
	return &copy
}

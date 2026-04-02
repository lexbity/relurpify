package execution_test

import (
	"context"
	"testing"
	"time"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeoevents "github.com/lexcodex/relurpify/archaeo/events"
	"github.com/lexcodex/relurpify/archaeo/execution"
	archaeoplans "github.com/lexcodex/relurpify/archaeo/plans"
	"github.com/lexcodex/relurpify/archaeo/testscenario"
	"github.com/lexcodex/relurpify/framework/graphdb"
	"github.com/lexcodex/relurpify/framework/guidance"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
	frameworkretrieval "github.com/lexcodex/relurpify/framework/retrieval"
)

func TestScenarioPreflightCleanGate(t *testing.T) {
	f := testscenario.NewFixture(t)
	f.SeedWorkflow("wf-clean", "run a clean preflight gate")

	plan := executionPlan("plan-clean", "wf-clean")
	active := f.SeedActivePlan(plan, archaeoplans.DraftVersionInput{
		WorkflowID:      "wf-clean",
		BasedOnRevision: "rev-1",
	})
	mustUpsertNode(t, f.Graph, "symbol.present")

	coord := f.PreflightCoordinator()
	state := f.NewState()
	task := f.Task("wf-clean", "run a clean preflight gate", map[string]any{
		"based_on_revision": "rev-1",
	})
	outcome, err := coord.EvaluatePlanStepGate(context.Background(), task, state, &active.Plan, active.Plan.Steps["inspect"], f.Graph)

	if err != nil {
		t.Fatalf("evaluate preflight gate: %v", err)
	}
	if outcome.ShouldInvalidate {
		t.Fatalf("expected clean gate without invalidation")
	}
	if outcome.Result != nil {
		t.Fatalf("expected no short-circuit result")
	}
	if outcome.MutationCheckpoint == nil || outcome.MutationCheckpoint.Checkpoint != archaeodomain.MutationCheckpointPreExecution {
		t.Fatalf("expected pre-execution checkpoint, got %+v", outcome.MutationCheckpoint)
	}
	history := execution.MutationCheckpointSummaries(state)
	if len(history) != 1 {
		t.Fatalf("expected 1 checkpoint, got %d", len(history))
	}
	if history[0].Disposition != archaeodomain.DispositionContinue {
		t.Fatalf("expected continue disposition, got %s", history[0].Disposition)
	}
}

func TestScenarioPreflightBlockingMutation(t *testing.T) {
	f := testscenario.NewFixture(t)
	f.SeedWorkflow("wf-block", "block execution from mutation")

	plan := executionPlan("plan-block", "wf-block")
	active := f.SeedActivePlan(plan, archaeoplans.DraftVersionInput{
		WorkflowID:      "wf-block",
		BasedOnRevision: "rev-1",
	})
	if err := archaeoevents.AppendMutationEvent(context.Background(), f.WorkflowStore, archaeodomain.MutationEvent{
		ID:          "mutation-block",
		WorkflowID:  "wf-block",
		PlanID:      active.Plan.ID,
		PlanVersion: intPtr(active.Version),
		StepID:      "inspect",
		Category:    archaeodomain.MutationBlockingSemantic,
		SourceKind:  "tension",
		SourceRef:   "tension-1",
		Description: "critical contradiction blocks execution",
		BlastRadius: archaeodomain.BlastRadius{
			Scope:           archaeodomain.BlastRadiusStep,
			AffectedStepIDs: []string{"inspect"},
		},
		Impact:      archaeodomain.ImpactLocalBlocking,
		Disposition: archaeodomain.DispositionBlockExecution,
		Blocking:    true,
		CreatedAt:   f.Now(),
	}); err != nil {
		t.Fatalf("append mutation event: %v", err)
	}

	coord := f.PreflightCoordinator()
	state := f.NewState()
	task := f.Task("wf-block", "block execution from mutation", nil)
	outcome, err := coord.EvaluatePlanStepGate(context.Background(), task, state, &active.Plan, active.Plan.Steps["inspect"], f.Graph)

	if err == nil {
		t.Fatalf("expected blocking mutation error")
	}
	if outcome.MutationEvaluation == nil || outcome.MutationEvaluation.Disposition != archaeodomain.DispositionBlockExecution {
		t.Fatalf("expected block_execution mutation evaluation, got %+v", outcome.MutationEvaluation)
	}
	if outcome.ShouldInvalidate {
		t.Fatalf("expected blocking mutation without plan invalidation")
	}
	history := execution.MutationCheckpointSummaries(state)
	if len(history) != 1 || history[0].Disposition != archaeodomain.DispositionBlockExecution {
		t.Fatalf("expected single block_execution checkpoint, got %+v", history)
	}
}

func TestScenarioPreflightGuidanceOnLowConfidence(t *testing.T) {
	f := testscenario.NewFixture(t)
	f.SeedWorkflow("wf-guidance", "ask for guidance on low confidence")

	plan := executionPlan("plan-guidance", "wf-guidance")
	plan.Steps["inspect"].ConfidenceScore = 0.10
	active := f.SeedActivePlan(plan, archaeoplans.DraftVersionInput{
		WorkflowID:      "wf-guidance",
		BasedOnRevision: "rev-1",
	})
	mustUpsertNode(t, f.Graph, "symbol.present")

	var guidanceCalled bool
	coord := f.PreflightCoordinator()
	coord.RequestGuidance = func(context.Context, guidance.GuidanceRequest, string) guidance.GuidanceDecision {
		guidanceCalled = true
		return guidance.GuidanceDecision{ChoiceID: "skip", DecidedBy: "scenario"}
	}

	state := f.NewState()
	task := f.Task("wf-guidance", "ask for guidance on low confidence", nil)
	outcome, err := coord.EvaluatePlanStepGate(context.Background(), task, state, &active.Plan, active.Plan.Steps["inspect"], f.Graph)

	if err != nil {
		t.Fatalf("evaluate preflight gate: %v", err)
	}
	if !guidanceCalled {
		t.Fatalf("expected guidance request")
	}
	if outcome.Result == nil || !outcome.Result.Success {
		t.Fatalf("expected successful short-circuit result, got %+v", outcome.Result)
	}
	if active.Plan.Steps["inspect"].Status != frameworkplan.PlanStepSkipped {
		t.Fatalf("expected skipped step, got %s", active.Plan.Steps["inspect"].Status)
	}
}

func TestScenarioPreflightAnchorDriftInvalidates(t *testing.T) {
	f := testscenario.NewFixture(t)
	f.SeedWorkflow("wf-drift", "invalidate on anchor drift")

	anchor := f.SeedAnchor("workspace", frameworkretrieval.AnchorDeclaration{
		Term:       "compatibility",
		Definition: "preserve legacy compatibility",
		Class:      "policy",
	}, "workspace_trusted")
	if err := frameworkretrieval.RecordAnchorDrift(context.Background(), f.RetrievalDB, anchor.AnchorID, "high", "compatibility no longer holds"); err != nil {
		t.Fatalf("record anchor drift: %v", err)
	}

	plan := executionPlan("plan-drift", "wf-drift")
	plan.Steps["inspect"].AnchorDependencies = []string{anchor.AnchorID}
	plan.Steps["inspect"].InvalidatedBy = []frameworkplan.InvalidationRule{{
		Kind:   frameworkplan.InvalidationAnchorDrifted,
		Target: anchor.AnchorID,
	}}
	plan.Steps["advance"].InvalidatedBy = []frameworkplan.InvalidationRule{{
		Kind:   frameworkplan.InvalidationAnchorDrifted,
		Target: anchor.AnchorID,
	}}
	active := f.SeedActivePlan(plan, archaeoplans.DraftVersionInput{
		WorkflowID:      "wf-drift",
		BasedOnRevision: "rev-1",
		AnchorRefs:      []string{anchor.AnchorID},
	})
	mustUpsertNode(t, f.Graph, "symbol.present")
	mustUpsertNode(t, f.Graph, "symbol.advance")

	coord := f.PreflightCoordinator()
	state := f.NewState()
	task := f.Task("wf-drift", "invalidate on anchor drift", nil)
	outcome, err := coord.EvaluatePlanStepGate(context.Background(), task, state, &active.Plan, active.Plan.Steps["inspect"], f.Graph)

	if err != nil {
		t.Fatalf("evaluate preflight gate: %v", err)
	}
	if !outcome.ShouldInvalidate {
		t.Fatalf("expected drift to trigger invalidation")
	}
	if len(outcome.InvalidatedStepIDs) == 0 {
		t.Fatalf("expected invalidated step ids")
	}
	if active.Plan.Steps["advance"].Status != frameworkplan.PlanStepInvalidated {
		t.Fatalf("expected dependent step invalidated, got %s", active.Plan.Steps["advance"].Status)
	}
}

func TestScenarioPreflightContinueOnStalePlanPolicy(t *testing.T) {
	f := testscenario.NewFixture(t)
	f.SeedWorkflow("wf-stale", "continue on stale plan")

	plan := executionPlan("plan-stale", "wf-stale")
	active := f.SeedActivePlan(plan, archaeoplans.DraftVersionInput{
		WorkflowID:      "wf-stale",
		BasedOnRevision: "rev-1",
	})
	mustUpsertNode(t, f.Graph, "symbol.present")
	if err := archaeoevents.AppendMutationEvent(context.Background(), f.WorkflowStore, archaeodomain.MutationEvent{
		ID:          "mutation-stale",
		WorkflowID:  "wf-stale",
		PlanID:      active.Plan.ID,
		PlanVersion: intPtr(active.Version),
		Category:    archaeodomain.MutationPlanStaleness,
		SourceKind:  "plan_version",
		SourceRef:   "plan-stale:1",
		Description: "active plan version is stale",
		BlastRadius: archaeodomain.BlastRadius{Scope: archaeodomain.BlastRadiusPlan},
		Impact:      archaeodomain.ImpactPlanRecomputeRequired,
		Disposition: archaeodomain.DispositionRequireReplan,
		CreatedAt:   f.Now(),
	}); err != nil {
		t.Fatalf("append stale mutation: %v", err)
	}

	coord := f.PreflightCoordinator()
	coord.Service.MutationPolicy = execution.MutationPolicy{ContinueOnStalePlan: true}

	state := f.NewState()
	task := f.Task("wf-stale", "continue on stale plan", nil)
	outcome, err := coord.EvaluatePlanStepGate(context.Background(), task, state, &active.Plan, active.Plan.Steps["inspect"], f.Graph)

	if err != nil {
		t.Fatalf("expected stale-plan policy to continue, got %v", err)
	}
	if outcome.MutationEvaluation == nil || outcome.MutationEvaluation.Disposition != archaeodomain.DispositionContinueOnStalePlan {
		t.Fatalf("expected continue_on_stale_plan evaluation, got %+v", outcome.MutationEvaluation)
	}
	if outcome.MutationEvaluation.RequireReplan {
		t.Fatalf("expected no hard replan requirement after policy adjustment")
	}
	history := execution.MutationCheckpointSummaries(state)
	if len(history) != 1 || history[0].Disposition != archaeodomain.DispositionContinueOnStalePlan {
		t.Fatalf("expected continue_on_stale_plan checkpoint, got %+v", history)
	}
}

func TestScenarioMutationCheckpointsAccumulateAcrossStepSequence(t *testing.T) {
	f := testscenario.NewFixture(t)
	f.SeedWorkflow("wf-sequence", "accumulate mutation checkpoints")

	plan := executionPlan("plan-sequence", "wf-sequence")
	active := f.SeedActivePlan(plan, archaeoplans.DraftVersionInput{
		WorkflowID:      "wf-sequence",
		BasedOnRevision: "rev-1",
	})
	if err := archaeoevents.AppendMutationEvent(context.Background(), f.WorkflowStore, archaeodomain.MutationEvent{
		ID:          "mutation-observation",
		WorkflowID:  "wf-sequence",
		PlanID:      active.Plan.ID,
		PlanVersion: intPtr(active.Version),
		StepID:      "inspect",
		Category:    archaeodomain.MutationObservation,
		SourceKind:  "comment",
		SourceRef:   "comment-1",
		Description: "supplemental note",
		BlastRadius: archaeodomain.BlastRadius{Scope: archaeodomain.BlastRadiusStep, AffectedStepIDs: []string{"inspect"}},
		Impact:      archaeodomain.ImpactInformational,
		Disposition: archaeodomain.DispositionContinue,
		CreatedAt:   f.Now(),
	}); err != nil {
		t.Fatalf("append observation mutation: %v", err)
	}
	mustUpsertNode(t, f.Graph, "symbol.present")

	state := f.NewState()
	state.Set("euclo.execution_handoff", archaeodomain.ExecutionHandoff{
		WorkflowID:  "wf-sequence",
		PlanID:      active.Plan.ID,
		PlanVersion: active.Version,
		CreatedAt:   time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC).Add(-time.Minute),
	})
	task := f.Task("wf-sequence", "accumulate mutation checkpoints", nil)

	preflight := f.PreflightCoordinator()
	if _, err := preflight.EvaluatePlanStepGate(context.Background(), task, state, &active.Plan, active.Plan.Steps["inspect"], f.Graph); err != nil {
		t.Fatalf("pre-execution gate: %v", err)
	}

	live := execution.LiveMutationCoordinator{
		Service: f.ExecutionService(),
		Plans:   f.PlansService(),
	}
	for _, checkpoint := range []archaeodomain.MutationCheckpoint{
		archaeodomain.MutationCheckpointPreDispatch,
		archaeodomain.MutationCheckpointPostExecution,
		archaeodomain.MutationCheckpointPreVerification,
		archaeodomain.MutationCheckpointPreFinalization,
	} {
		if _, err := live.CheckpointExecutionAt(context.Background(), checkpoint, task, state, &active.Plan, active.Plan.Steps["inspect"]); err != nil {
			t.Fatalf("checkpoint %s: %v", checkpoint, err)
		}
	}

	history := execution.MutationCheckpointSummaries(state)
	if len(history) != 5 {
		t.Fatalf("expected 5 checkpoints, got %d", len(history))
	}
	want := []archaeodomain.MutationCheckpoint{
		archaeodomain.MutationCheckpointPreExecution,
		archaeodomain.MutationCheckpointPreDispatch,
		archaeodomain.MutationCheckpointPostExecution,
		archaeodomain.MutationCheckpointPreVerification,
		archaeodomain.MutationCheckpointPreFinalization,
	}
	for i, checkpoint := range want {
		if history[i].Checkpoint != checkpoint {
			t.Fatalf("checkpoint %d mismatch: got %s want %s", i, history[i].Checkpoint, checkpoint)
		}
		if history[i].Disposition != archaeodomain.DispositionContinue {
			t.Fatalf("checkpoint %d disposition mismatch: got %s", i, history[i].Disposition)
		}
	}
}

func executionPlan(planID, workflowID string) *frameworkplan.LivingPlan {
	now := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	return &frameworkplan.LivingPlan{
		ID:         planID,
		WorkflowID: workflowID,
		Title:      "Execution Scenario Plan",
		Version:    1,
		Steps: map[string]*frameworkplan.PlanStep{
			"inspect": {
				ID:              "inspect",
				Description:     "Inspect the target symbol",
				Scope:           []string{"symbol.present"},
				ConfidenceScore: 0.90,
				Status:          frameworkplan.PlanStepPending,
				CreatedAt:       now,
				UpdatedAt:       now,
			},
			"advance": {
				ID:              "advance",
				Description:     "Advance execution",
				Scope:           []string{"symbol.advance"},
				ConfidenceScore: 0.90,
				DependsOn:       []string{"inspect"},
				Status:          frameworkplan.PlanStepPending,
				CreatedAt:       now,
				UpdatedAt:       now,
			},
		},
		StepOrder: []string{"inspect", "advance"},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func mustUpsertNode(t *testing.T, engine *graphdb.Engine, id string) {
	t.Helper()
	if err := engine.UpsertNode(graphdb.NodeRecord{ID: id, Kind: graphdb.NodeKind("symbol")}); err != nil {
		t.Fatalf("upsert node %s: %v", id, err)
	}
}

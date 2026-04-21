package archaeology_test

import (
	"context"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/archaeo/archaeology"
	archaeoconvergence "codeburg.org/lexbit/relurpify/archaeo/convergence"
	archaeodeferred "codeburg.org/lexbit/relurpify/archaeo/deferred"
	archaeodomain "codeburg.org/lexbit/relurpify/archaeo/domain"
	archaeoevents "codeburg.org/lexbit/relurpify/archaeo/events"
	archaeoexec "codeburg.org/lexbit/relurpify/archaeo/execution"
	archaeolearning "codeburg.org/lexbit/relurpify/archaeo/learning"
	archaeoplans "codeburg.org/lexbit/relurpify/archaeo/plans"
	archaeotensions "codeburg.org/lexbit/relurpify/archaeo/tensions"
	"codeburg.org/lexbit/relurpify/archaeo/testscenario"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/patterns"
	frameworkplan "codeburg.org/lexbit/relurpify/framework/plan"
)

func TestScenarioHappyPathLifecycle(t *testing.T) {
	f := testscenario.NewFixture(t)
	f.SeedWorkflow("wf-happy", "prepare a stable plan")

	plan := buildPlan("plan-happy", "wf-happy")
	f.SeedActivePlan(plan, archaeoplans.DraftVersionInput{
		WorkflowID:          "wf-happy",
		BasedOnRevision:     "rev-1",
		PatternRefs:         []string{"pattern-stable"},
		SemanticSnapshotRef: "semantic-1",
	})

	svc := f.ArchaeologyService()
	svc.PersistPhase = f.PersistPhaseFunc()
	svc.EvaluateGate = allowPreflight

	state := f.NewState()
	task := f.Task("wf-happy", "prepare a stable plan", map[string]any{
		"based_on_revision":     "rev-1",
		"semantic_snapshot_ref": "semantic-1",
	})
	result := svc.PrepareLivingPlan(context.Background(), task, state, "wf-happy")

	if result.Err != nil {
		t.Fatalf("prepare living plan: %v", result.Err)
	}
	if result.Plan == nil {
		t.Fatalf("expected active plan")
	}
	testscenario.RequirePhase(t, f.PhaseService(), "wf-happy", archaeodomain.PhaseSurfacing)
	testscenario.RequireActivePlanVersion(t, f.PlansService(), "wf-happy", 1)
	testscenario.RequireLineageVersions(t, f.PlansService(), "wf-happy", 1)
	testscenario.RequireExplorationStatus(t, svc, state.GetString("euclo.active_exploration_id"), archaeodomain.ExplorationStatusActive)
	testscenario.RequireExplorationSnapshotRevision(t, svc, "wf-happy", state.GetString("euclo.active_exploration_snapshot_id"), "rev-1")
}

func TestScenarioRevisionStalenessCreatesDraftSuccessor(t *testing.T) {
	f := testscenario.NewFixture(t)
	f.SeedWorkflow("wf-revision", "recompute after revision drift")

	svc := f.ArchaeologyService()
	session, snapshot := f.SeedExploration("wf-revision", f.Workspace, "rev-1", archaeology.SnapshotInput{
		WorkflowID:      "wf-revision",
		WorkspaceID:     f.Workspace,
		BasedOnRevision: "rev-1",
		Summary:         "existing scan",
	})
	plan := buildPlan("plan-revision", "wf-revision")
	f.SeedActivePlan(plan, archaeoplans.DraftVersionInput{
		WorkflowID:             "wf-revision",
		DerivedFromExploration: session.ID,
		BasedOnRevision:        "rev-1",
		SemanticSnapshotRef:    snapshot.ID,
	})

	svc.PersistPhase = f.PersistPhaseFunc()
	svc.EvaluateGate = allowPreflight

	state := f.NewState()
	task := f.Task("wf-revision", "recompute after revision drift", map[string]any{
		"based_on_revision": "rev-2",
	})
	result := svc.PrepareLivingPlan(context.Background(), task, state, "wf-revision")

	if result.Err != nil {
		t.Fatalf("prepare living plan: %v", result.Err)
	}
	testscenario.RequirePhase(t, f.PhaseService(), "wf-revision", archaeodomain.PhaseSurfacing)
	testscenario.RequireExplorationStatus(t, svc, state.GetString("euclo.active_exploration_id"), archaeodomain.ExplorationStatusActive)
	testscenario.RequireExplorationSnapshotRevision(t, svc, "wf-revision", state.GetString("euclo.active_exploration_snapshot_id"), "rev-2")

	updatedSnapshot, err := svc.LoadExplorationSnapshotByWorkflow(context.Background(), "wf-revision", state.GetString("euclo.active_exploration_snapshot_id"))
	if err != nil {
		t.Fatalf("load updated snapshot: %v", err)
	}
	if updatedSnapshot == nil {
		t.Fatalf("expected updated snapshot")
	}
	draft, err := f.PlansService().SyncActiveVersionWithExploration(context.Background(), "wf-revision", updatedSnapshot)
	if err != nil {
		t.Fatalf("sync active version with exploration: %v", err)
	}
	if draft == nil || draft.Version != 2 {
		t.Fatalf("expected draft successor version 2, got %+v", draft)
	}
	testscenario.RequireLineageVersions(t, f.PlansService(), "wf-revision", 1, 2)

	active, err := f.PlansService().LoadActiveVersion(context.Background(), "wf-revision")
	if err != nil {
		t.Fatalf("load active version: %v", err)
	}
	if active == nil || !active.RecomputeRequired {
		t.Fatalf("expected active version to be marked stale")
	}
	if draft.ParentVersion == nil || *draft.ParentVersion != 1 {
		t.Fatalf("expected draft successor to point to version 1, got %+v", draft.ParentVersion)
	}
}

func TestScenarioTensionLifecycleInferredToResolved(t *testing.T) {
	f := testscenario.NewFixture(t)
	f.SeedWorkflow("wf-tension", "walk a tension lifecycle")

	svc := f.ArchaeologyService()
	session, snapshot := f.SeedExploration("wf-tension", f.Workspace, "rev-1", archaeology.SnapshotInput{
		WorkflowID:      "wf-tension",
		WorkspaceID:     f.Workspace,
		BasedOnRevision: "rev-1",
		Summary:         "inspect boundary",
	})
	record := f.SeedTension(archaeotensions.CreateInput{
		WorkflowID:      "wf-tension",
		ExplorationID:   session.ID,
		SnapshotID:      snapshot.ID,
		SourceRef:       "gap:api-boundary",
		Kind:            "intent_gap",
		Description:     "handler boundary is ambiguous",
		Status:          archaeodomain.TensionInferred,
		BasedOnRevision: "rev-1",
	})

	testscenario.RequireTensionStatus(t, f.TensionService(), "wf-tension", record.ID, archaeodomain.TensionInferred)
	if _, err := f.TensionService().UpdateStatus(context.Background(), "wf-tension", record.ID, archaeodomain.TensionUnresolved, nil); err != nil {
		t.Fatalf("mark unresolved: %v", err)
	}
	testscenario.RequireActiveTensionIDs(t, f.TensionService(), "wf-tension", record.ID)

	if _, err := f.TensionService().UpdateStatus(context.Background(), "wf-tension", record.ID, archaeodomain.TensionResolved, []string{"comment-resolution"}); err != nil {
		t.Fatalf("mark resolved: %v", err)
	}
	testscenario.RequireTensionStatus(t, f.TensionService(), "wf-tension", record.ID, archaeodomain.TensionResolved)
	testscenario.RequireActiveTensionIDs(t, f.TensionService(), "wf-tension")

	view, err := svc.LoadExplorationView(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("load exploration view: %v", err)
	}
	if view == nil || view.TensionSummary == nil || view.TensionSummary.Resolved != 1 {
		t.Fatalf("expected resolved tension summary, got %+v", view)
	}
}

func TestScenarioLearningResolutionUpdatesStateAndLineage(t *testing.T) {
	f := testscenario.NewFixture(t)
	f.SeedWorkflow("wf-learning", "confirm an archaeology pattern")

	session, _ := f.SeedExploration("wf-learning", f.Workspace, "rev-1", archaeology.SnapshotInput{
		WorkflowID:      "wf-learning",
		WorkspaceID:     f.Workspace,
		BasedOnRevision: "rev-1",
		Summary:         "seed learning",
	})
	savePattern(t, f, patterns.PatternRecord{
		ID:           "pattern-1",
		Kind:         patterns.PatternKindStructural,
		Title:        "Adapter",
		Description:  "wrap external behavior",
		Status:       patterns.PatternStatusProposed,
		CorpusScope:  "workspace",
		CorpusSource: "workspace",
		CreatedAt:    f.Now(),
		UpdatedAt:    f.Now(),
	})
	plan := buildPlan("plan-learning", "wf-learning")
	f.SeedActivePlan(plan, archaeoplans.DraftVersionInput{
		WorkflowID:             "wf-learning",
		DerivedFromExploration: session.ID,
		BasedOnRevision:        "rev-1",
		PatternRefs:            []string{"pattern-1"},
	})

	interaction := f.SeedLearningInteraction(archaeolearning.CreateInput{
		WorkflowID:      "wf-learning",
		ExplorationID:   session.ID,
		Kind:            archaeolearning.InteractionPatternProposal,
		SubjectType:     archaeolearning.SubjectPattern,
		SubjectID:       "pattern-1",
		Title:           "Confirm Adapter pattern",
		Blocking:        true,
		BasedOnRevision: "rev-1",
	})

	testscenario.RequirePendingLearningIDs(t, f.LearningService(), "wf-learning", interaction.ID)
	if _, err := f.LearningService().Resolve(context.Background(), archaeolearning.ResolveInput{
		WorkflowID:      "wf-learning",
		InteractionID:   interaction.ID,
		Kind:            archaeolearning.ResolutionConfirm,
		ResolvedBy:      "reviewer",
		BasedOnRevision: "rev-1",
	}); err != nil {
		t.Fatalf("resolve learning interaction: %v", err)
	}

	testscenario.RequirePendingLearningIDs(t, f.LearningService(), "wf-learning")
	testscenario.RequireLineageVersions(t, f.PlansService(), "wf-learning", 1, 2)

	active, err := f.PlansService().LoadActiveVersion(context.Background(), "wf-learning")
	if err != nil {
		t.Fatalf("load active version: %v", err)
	}
	if active == nil || !active.RecomputeRequired {
		t.Fatalf("expected active version to require recompute after pattern resolution")
	}
	pattern, err := f.PatternStore.Load(context.Background(), "pattern-1")
	if err != nil {
		t.Fatalf("load pattern: %v", err)
	}
	if pattern == nil || pattern.Status != patterns.PatternStatusConfirmed {
		t.Fatalf("expected confirmed pattern, got %+v", pattern)
	}
}

func TestScenarioDeferredAmbiguityAndConvergenceRecords(t *testing.T) {
	f := testscenario.NewFixture(t)
	f.SeedWorkflow("wf-converge", "record deferred ambiguity")

	session, _ := f.SeedExploration("wf-converge", f.Workspace, "rev-1", archaeology.SnapshotInput{
		WorkflowID:      "wf-converge",
		WorkspaceID:     f.Workspace,
		BasedOnRevision: "rev-1",
		Summary:         "explore ambiguity",
	})
	plan := buildPlan("plan-converge", "wf-converge")
	active := f.SeedActivePlan(plan, archaeoplans.DraftVersionInput{
		WorkflowID:             "wf-converge",
		DerivedFromExploration: session.ID,
		BasedOnRevision:        "rev-1",
	})
	tension := f.SeedTension(archaeotensions.CreateInput{
		WorkflowID:      "wf-converge",
		ExplorationID:   session.ID,
		SourceRef:       "gap:legacy-contract",
		Kind:            "legacy_gap",
		Description:     "legacy caller contract is unclear",
		Status:          archaeodomain.TensionUnresolved,
		BasedOnRevision: "rev-1",
	})

	deferred := f.SeedDeferredDraft(archaeodeferred.CreateInput{
		WorkspaceID:   f.Workspace,
		WorkflowID:    "wf-converge",
		ExplorationID: session.ID,
		PlanID:        active.Plan.ID,
		PlanVersion:   &active.Version,
		AmbiguityKey:  "legacy-contract",
		Title:         "Legacy contract ambiguity",
		Description:   "caller contract needs deferred treatment",
	})
	testscenario.RequireDeferredDraftStatus(t, f.DeferredService(), "wf-converge", deferred.ID, archaeodomain.DeferredDraftPending)

	record := f.SeedConvergenceRecord(archaeoconvergence.CreateInput{
		WorkspaceID:        f.Workspace,
		WorkflowID:         "wf-converge",
		ExplorationID:      session.ID,
		PlanID:             active.Plan.ID,
		PlanVersion:        &active.Version,
		Title:              "Resolve deferred ambiguity",
		Question:           "Can the legacy contract be deferred safely?",
		RelevantTensionIDs: []string{tension.ID},
		DeferredDraftIDs:   []string{deferred.ID},
	})
	testscenario.RequireConvergenceState(t, f.ConvergenceService(), f.Workspace, archaeodomain.ConvergenceResolutionOpen)

	if _, err := f.DeferredService().Finalize(context.Background(), archaeodeferred.FinalizeInput{
		WorkflowID: "wf-converge",
		RecordID:   deferred.ID,
	}); err != nil {
		t.Fatalf("finalize deferred draft: %v", err)
	}
	if _, err := f.ConvergenceService().Resolve(context.Background(), archaeoconvergence.ResolveInput{
		WorkflowID: "wf-converge",
		RecordID:   record.ID,
		Resolution: archaeodomain.ConvergenceResolution{Status: archaeodomain.ConvergenceResolutionResolved},
	}); err != nil {
		t.Fatalf("resolve convergence record: %v", err)
	}

	testscenario.RequireDeferredDraftStatus(t, f.DeferredService(), "wf-converge", deferred.ID, archaeodomain.DeferredDraftFinalized)
	testscenario.RequireConvergenceState(t, f.ConvergenceService(), f.Workspace, archaeodomain.ConvergenceResolutionResolved)
}

func TestScenarioMutationDispositionEscalatesAcrossEvents(t *testing.T) {
	f := testscenario.NewFixture(t)
	f.SeedWorkflow("wf-mutations", "escalate mutation disposition")

	plan := buildPlan("plan-mutations", "wf-mutations")
	active := f.SeedActivePlan(plan, archaeoplans.DraftVersionInput{
		WorkflowID:      "wf-mutations",
		BasedOnRevision: "rev-1",
	})

	now := f.Now()
	for _, mutation := range []archaeodomain.MutationEvent{
		{
			ID:          "mutation-info",
			WorkflowID:  "wf-mutations",
			PlanID:      active.Plan.ID,
			PlanVersion: &active.Version,
			StepID:      "inspect",
			Category:    archaeodomain.MutationObservation,
			SourceKind:  "learning_interaction",
			SourceRef:   "learn-1",
			Description: "supplemental context landed",
			BlastRadius: archaeodomain.BlastRadius{
				Scope:           archaeodomain.BlastRadiusStep,
				AffectedStepIDs: []string{"inspect"},
			},
			Impact:      archaeodomain.ImpactInformational,
			Disposition: archaeodomain.DispositionContinue,
			CreatedAt:   now,
		},
		{
			ID:          "mutation-step",
			WorkflowID:  "wf-mutations",
			PlanID:      active.Plan.ID,
			PlanVersion: &active.Version,
			StepID:      "inspect",
			Category:    archaeodomain.MutationStepInvalidation,
			SourceKind:  "tension",
			SourceRef:   "tension-1",
			Description: "active step assumptions no longer hold",
			BlastRadius: archaeodomain.BlastRadius{
				Scope:           archaeodomain.BlastRadiusStep,
				AffectedStepIDs: []string{"inspect"},
			},
			Impact:      archaeodomain.ImpactLocalBlocking,
			Disposition: archaeodomain.DispositionInvalidateStep,
			Blocking:    true,
			CreatedAt:   now.Add(time.Second),
		},
		{
			ID:          "mutation-block",
			WorkflowID:  "wf-mutations",
			PlanID:      active.Plan.ID,
			PlanVersion: &active.Version,
			StepID:      "inspect",
			Category:    archaeodomain.MutationBlockingSemantic,
			SourceKind:  "convergence",
			SourceRef:   "convergence-1",
			Description: "workspace has unresolved blocking ambiguity",
			BlastRadius: archaeodomain.BlastRadius{
				Scope:           archaeodomain.BlastRadiusWorkflow,
				AffectedStepIDs: []string{"inspect"},
			},
			Impact:      archaeodomain.ImpactHandoffInvalidating,
			Disposition: archaeodomain.DispositionBlockExecution,
			Blocking:    true,
			CreatedAt:   now.Add(2 * time.Second),
		},
	} {
		if err := archaeoevents.AppendMutationEvent(context.Background(), f.WorkflowStore, mutation); err != nil {
			t.Fatalf("append mutation event: %v", err)
		}
	}

	eval, err := f.ExecutionService().EvaluateMutations(context.Background(), "wf-mutations", nil, &active.Plan, active.Plan.Steps["inspect"])
	if err != nil {
		t.Fatalf("evaluate mutations: %v", err)
	}
	if eval == nil {
		t.Fatalf("expected mutation evaluation")
	}
	if eval.Disposition != archaeodomain.DispositionBlockExecution {
		t.Fatalf("expected block_execution, got %s", eval.Disposition)
	}
	if eval.HighestImpact != archaeodomain.ImpactHandoffInvalidating {
		t.Fatalf("expected highest impact %s, got %s", archaeodomain.ImpactHandoffInvalidating, eval.HighestImpact)
	}
	if !eval.Blocking {
		t.Fatalf("expected blocking mutation evaluation")
	}
	if len(eval.RelevantMutations) != 3 {
		t.Fatalf("expected 3 relevant mutations, got %d", len(eval.RelevantMutations))
	}

	mutations, err := archaeoevents.ReadMutationEvents(context.Background(), f.WorkflowStore, "wf-mutations")
	if err != nil {
		t.Fatalf("read mutation events: %v", err)
	}
	if len(mutations) != 3 {
		t.Fatalf("expected 3 persisted mutations, got %d", len(mutations))
	}
	if mutations[len(mutations)-1].Disposition != archaeodomain.DispositionBlockExecution {
		t.Fatalf("expected latest mutation to persist block_execution, got %s", mutations[len(mutations)-1].Disposition)
	}
}

func TestScenarioPlanVersioningTracksLineage(t *testing.T) {
	f := testscenario.NewFixture(t)
	f.SeedWorkflow("wf-lineage", "track plan lineage")

	session, snapshot := f.SeedExploration("wf-lineage", f.Workspace, "rev-1", archaeology.SnapshotInput{
		WorkflowID:      "wf-lineage",
		WorkspaceID:     f.Workspace,
		BasedOnRevision: "rev-1",
		Summary:         "seed lineage",
	})
	basePlan := buildPlan("plan-lineage", "wf-lineage")
	v1 := f.SeedActivePlan(basePlan, archaeoplans.DraftVersionInput{
		WorkflowID:             "wf-lineage",
		DerivedFromExploration: session.ID,
		BasedOnRevision:        "rev-1",
		SemanticSnapshotRef:    snapshot.ID,
	})

	v2, err := f.PlansService().EnsureDraftSuccessor(context.Background(), "wf-lineage", v1.Version, "expanded archaeology inputs")
	if err != nil {
		t.Fatalf("ensure draft successor: %v", err)
	}
	if v2 == nil {
		t.Fatalf("expected v2 draft successor")
	}
	if _, err := f.PlansService().ActivateVersion(context.Background(), "wf-lineage", v2.Version); err != nil {
		t.Fatalf("activate v2: %v", err)
	}
	if _, err := f.PlansService().MarkVersionStale(context.Background(), "wf-lineage", v2.Version, "fresh tensions require recompute"); err != nil {
		t.Fatalf("mark v2 stale: %v", err)
	}
	v3, err := f.PlansService().EnsureDraftSuccessor(context.Background(), "wf-lineage", v2.Version, "fresh tensions require recompute")
	if err != nil {
		t.Fatalf("ensure v3 draft successor: %v", err)
	}
	if v3 == nil {
		t.Fatalf("expected v3 draft successor")
	}

	testscenario.RequireActivePlanVersion(t, f.PlansService(), "wf-lineage", 2)
	testscenario.RequireLineageVersions(t, f.PlansService(), "wf-lineage", 1, 2, 3)

	versions, err := f.PlansService().ListVersions(context.Background(), "wf-lineage")
	if err != nil {
		t.Fatalf("list versions: %v", err)
	}
	if len(versions) != 3 {
		t.Fatalf("expected 3 versions, got %d", len(versions))
	}
	if versions[0].Status != archaeodomain.LivingPlanVersionSuperseded {
		t.Fatalf("expected v1 superseded, got %s", versions[0].Status)
	}
	if versions[1].Status != archaeodomain.LivingPlanVersionActive || !versions[1].RecomputeRequired {
		t.Fatalf("expected v2 active and stale, got %+v", versions[1])
	}
	if versions[2].Status != archaeodomain.LivingPlanVersionDraft {
		t.Fatalf("expected v3 draft, got %s", versions[2].Status)
	}
	if versions[1].ParentVersion == nil || *versions[1].ParentVersion != 1 {
		t.Fatalf("expected v2 parent version 1, got %+v", versions[1].ParentVersion)
	}
	if versions[2].ParentVersion == nil || *versions[2].ParentVersion != 2 {
		t.Fatalf("expected v3 parent version 2, got %+v", versions[2].ParentVersion)
	}
}

func allowPreflight(context.Context, *core.Task, *core.Context, *frameworkplan.LivingPlan, *frameworkplan.PlanStep) (archaeoexec.PreflightOutcome, error) {
	return archaeoexec.PreflightOutcome{}, nil
}

func buildPlan(planID, workflowID string) *frameworkplan.LivingPlan {
	now := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	return &frameworkplan.LivingPlan{
		ID:         planID,
		WorkflowID: workflowID,
		Title:      "Scenario plan",
		Steps: map[string]*frameworkplan.PlanStep{
			"inspect": {
				ID:          "inspect",
				Description: "Inspect",
				Status:      frameworkplan.PlanStepPending,
				CreatedAt:   now,
				UpdatedAt:   now,
			},
		},
		StepOrder: []string{"inspect"},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func savePattern(t *testing.T, f *testscenario.Fixture, record patterns.PatternRecord) {
	t.Helper()
	if err := f.PatternStore.Save(context.Background(), record); err != nil {
		t.Fatalf("save pattern: %v", err)
	}
}

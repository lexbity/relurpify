package projections

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	archaeoarch "github.com/lexcodex/relurpify/archaeo/archaeology"
	archaeoconvergence "github.com/lexcodex/relurpify/archaeo/convergence"
	archaeodecisions "github.com/lexcodex/relurpify/archaeo/decisions"
	archaeodeferred "github.com/lexcodex/relurpify/archaeo/deferred"
	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeoevents "github.com/lexcodex/relurpify/archaeo/events"
	archaeolearning "github.com/lexcodex/relurpify/archaeo/learning"
	archaeophases "github.com/lexcodex/relurpify/archaeo/phases"
	archaeoplans "github.com/lexcodex/relurpify/archaeo/plans"
	archaeorequests "github.com/lexcodex/relurpify/archaeo/requests"
	archaeotensions "github.com/lexcodex/relurpify/archaeo/tensions"
	archaeoverification "github.com/lexcodex/relurpify/archaeo/verification"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	memorydb "github.com/lexcodex/relurpify/framework/memory/db"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
	eucloplan "github.com/lexcodex/relurpify/named/euclo/plan"
	"github.com/stretchr/testify/require"
)

type projectionVerifier struct{}

func (projectionVerifier) Verify(context.Context, frameworkplan.ConvergenceTarget) (*frameworkplan.ConvergenceFailure, error) {
	return nil, nil
}

func TestWorkflowProjectionRebuildsFromDurableState(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	workflowStore, err := memorydb.NewSQLiteWorkflowStateStore(filepath.Join(dir, "workflow.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, workflowStore.Close()) })
	planDB, err := eucloplan.OpenSQLite(filepath.Join(dir, "plans.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, planDB.Close()) })
	planStore, err := eucloplan.NewSQLitePlanStore(planDB)
	require.NoError(t, err)

	require.NoError(t, workflowStore.CreateWorkflow(ctx, testWorkflowRecord("wf-projection")))

	phaseSvc := archaeophases.Service{Store: workflowStore}
	_, err = phaseSvc.Transition(ctx, "wf-projection", archaeodomain.PhaseArchaeology, archaeodomain.PhaseTransition{To: archaeodomain.PhasePlanFormation})
	require.NoError(t, err)
	_, err = phaseSvc.Transition(ctx, "wf-projection", archaeodomain.PhaseArchaeology, archaeodomain.PhaseTransition{To: archaeodomain.PhaseExecution})
	require.NoError(t, err)

	archSvc := archaeoarch.Service{Store: workflowStore}
	session, err := archSvc.EnsureExplorationSession(ctx, "wf-projection", "/workspace/proj", "rev-1")
	require.NoError(t, err)
	_, err = archSvc.CreateExplorationSnapshot(ctx, session, archaeoarch.SnapshotInput{
		WorkflowID:           "wf-projection",
		WorkspaceID:          "/workspace/proj",
		BasedOnRevision:      "rev-1",
		SemanticSnapshotRef:  "semantic-1",
		CandidatePatternRefs: []string{"pattern-1"},
		OpenLearningIDs:      []string{"learn-1"},
		Summary:              "initial exploration",
	})
	require.NoError(t, err)

	learningSvc := archaeolearning.Service{Store: workflowStore}
	_, err = learningSvc.Create(ctx, archaeolearning.CreateInput{
		WorkflowID:    "wf-projection",
		ExplorationID: session.ID,
		Kind:          archaeolearning.InteractionPatternProposal,
		SubjectType:   archaeolearning.SubjectPattern,
		SubjectID:     "pattern-1",
		Title:         "Confirm pattern",
		Blocking:      true,
	})
	require.NoError(t, err)

	now := time.Now().UTC()
	plan := &frameworkplan.LivingPlan{
		ID:         "plan-projection",
		WorkflowID: "wf-projection",
		Title:      "Projection plan",
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {ID: "step-1", Description: "execute", Status: frameworkplan.PlanStepPending, CreatedAt: now, UpdatedAt: now},
		},
		StepOrder: []string{"step-1"},
		ConvergenceTarget: &frameworkplan.ConvergenceTarget{
			PatternIDs: []string{"pattern-1"},
		},
		Version:   1,
		CreatedAt: now,
		UpdatedAt: now,
	}
	planSvc := archaeoplans.Service{Store: planStore, WorkflowStore: workflowStore}
	version, err := planSvc.DraftVersion(ctx, plan, archaeoplans.DraftVersionInput{
		WorkflowID:             "wf-projection",
		DerivedFromExploration: session.ID,
		BasedOnRevision:        "rev-1",
		SemanticSnapshotRef:    "semantic-1",
		PatternRefs:            []string{"pattern-1"},
	})
	require.NoError(t, err)
	_, err = planSvc.ActivateVersion(ctx, "wf-projection", version.Version)
	require.NoError(t, err)

	verifySvc := archaeoverification.Service{
		Store:    planStore,
		Workflow: workflowStore,
		Verifier: projectionVerifier{},
	}
	_, err = verifySvc.FinalizeConvergence(ctx, plan, &core.Result{Success: true, Data: map[string]any{}})
	require.NoError(t, err)

	svc := Service{Store: workflowStore}
	model, err := svc.Workflow(ctx, "wf-projection")
	require.NoError(t, err)
	require.NotNil(t, model)
	require.NotNil(t, model.PhaseState)
	require.Equal(t, archaeodomain.PhaseExecution, model.PhaseState.CurrentPhase)
	require.NotNil(t, model.ActiveExploration)
	require.Equal(t, session.ID, model.ActiveExploration.ID)
	require.Len(t, model.ExplorationSnapshots, 1)
	require.Len(t, model.PendingLearning, 1)
	require.Equal(t, "pattern-1", model.PendingLearning[0].SubjectID)
	require.NotNil(t, model.ActivePlanVersion)
	require.Equal(t, version.Version, model.ActivePlanVersion.Version)
	require.NotNil(t, model.ConvergenceState)
	require.Equal(t, archaeodomain.ConvergenceStatusVerified, model.ConvergenceState.Status)
	require.NotEmpty(t, model.Timeline)

	explorationProj, err := svc.Exploration(ctx, "wf-projection")
	require.NoError(t, err)
	require.NotNil(t, explorationProj)
	require.Equal(t, session.ID, explorationProj.ActiveExploration.ID)

	learningProj, err := svc.LearningQueue(ctx, "wf-projection")
	require.NoError(t, err)
	require.NotNil(t, learningProj)
	require.Len(t, learningProj.PendingLearning, 1)
	require.Equal(t, []string{learningProj.PendingLearning[0].ID}, learningProj.BlockingLearning)

	activePlanProj, err := svc.ActivePlan(ctx, "wf-projection")
	require.NoError(t, err)
	require.NotNil(t, activePlanProj)
	require.NotNil(t, activePlanProj.ActivePlanVersion)
	require.Equal(t, version.Version, activePlanProj.ActivePlanVersion.Version)

	timelineProj, err := svc.TimelineProjection(ctx, "wf-projection")
	require.NoError(t, err)
	require.NotNil(t, timelineProj)
	require.NotEmpty(t, timelineProj.Timeline)
}

func TestTimelineMaterializerReplayIsOrderedAndIdempotent(t *testing.T) {
	ctx := context.Background()
	store, err := memorydb.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	require.NoError(t, store.CreateWorkflow(ctx, testWorkflowRecord("wf-timeline")))

	require.NoError(t, archaeoevents.AppendWorkflowEvent(ctx, store, "wf-timeline", archaeoevents.EventWorkflowPhaseTransitioned, "archaeology", map[string]any{"phase": "archaeology"}, time.Now().UTC()))
	require.NoError(t, archaeoevents.AppendWorkflowEvent(ctx, store, "wf-timeline", archaeoevents.EventExplorationSessionUpserted, "workspace", map[string]any{"exploration_id": "exp-1"}, time.Now().UTC()))
	require.NoError(t, archaeoevents.AppendWorkflowEvent(ctx, store, "wf-timeline", archaeoevents.EventLearningInteractionRequested, "learn", map[string]any{"interaction_id": "learn-1"}, time.Now().UTC()))

	log := &archaeoevents.WorkflowLog{Store: store}
	events, err := log.Read(ctx, "wf-timeline", 0, 0, false)
	require.NoError(t, err)
	require.Len(t, events, 3)

	materializer := &TimelineMaterializer{Store: store, WorkflowID: "wf-timeline"}
	require.NoError(t, materializer.Apply(ctx, events))
	first := append([]archaeodomain.TimelineEvent(nil), materializer.Projection.Timeline...)
	require.Len(t, first, 3)
	require.EqualValues(t, 1, first[0].Seq)
	require.EqualValues(t, 2, first[1].Seq)
	require.EqualValues(t, 3, first[2].Seq)

	require.NoError(t, materializer.Apply(ctx, events))
	require.Equal(t, first, materializer.Projection.Timeline)
}

func TestSubscribeWorkflowProjectionEmitsOnNewEvents(t *testing.T) {
	ctx := context.Background()
	store, err := memorydb.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	require.NoError(t, store.CreateWorkflow(ctx, testWorkflowRecord("wf-sub")))

	svc := &Service{Store: store, PollInterval: 10 * time.Millisecond}
	ch, cancel := svc.SubscribeWorkflow("wf-sub", 4)
	defer cancel()

	require.NoError(t, archaeoevents.AppendWorkflowEvent(ctx, store, "wf-sub", archaeoevents.EventWorkflowPhaseTransitioned, "archaeology", map[string]any{"phase": "archaeology"}, time.Now().UTC()))
	require.NoError(t, archaeoevents.AppendWorkflowEvent(ctx, store, "wf-sub", archaeoevents.EventLearningInteractionRequested, "learn", map[string]any{"interaction_id": "learn-sub"}, time.Now().UTC()))

	select {
	case event := <-ch:
		require.Equal(t, ProjectionEventSnapshot, event.Type)
		require.NotNil(t, event.Workflow)
		require.Equal(t, "wf-sub", event.Workflow.WorkflowID)
		require.NotEmpty(t, event.Timeline.Timeline)
	case <-time.After(time.Second):
		t.Fatal("expected workflow projection event")
	}
}

func TestHistoryAndCoherenceReadModels(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	workflowStore, err := memorydb.NewSQLiteWorkflowStateStore(filepath.Join(dir, "workflow.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, workflowStore.Close()) })
	planDB, err := eucloplan.OpenSQLite(filepath.Join(dir, "plans.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, planDB.Close()) })
	planStore, err := eucloplan.NewSQLitePlanStore(planDB)
	require.NoError(t, err)

	require.NoError(t, workflowStore.CreateWorkflow(ctx, testWorkflowRecord("wf-history")))

	archSvc := archaeoarch.Service{Store: workflowStore, Learning: archaeolearning.Service{Store: workflowStore}}
	session, err := archSvc.EnsureExplorationSession(ctx, "wf-history", "/workspace/history", "rev-1")
	require.NoError(t, err)
	snapshot, err := archSvc.CreateExplorationSnapshot(ctx, session, archaeoarch.SnapshotInput{
		WorkflowID:           "wf-history",
		WorkspaceID:          "/workspace/history",
		BasedOnRevision:      "rev-1",
		SemanticSnapshotRef:  "semantic-1",
		CandidatePatternRefs: []string{"pattern-1"},
		TensionIDs:           []string{"tension-seeded"},
		Summary:              "history exploration",
	})
	require.NoError(t, err)

	learningSvc := archaeolearning.Service{Store: workflowStore}
	interaction, err := learningSvc.Create(ctx, archaeolearning.CreateInput{
		WorkflowID:    "wf-history",
		ExplorationID: session.ID,
		SnapshotID:    snapshot.ID,
		Kind:          archaeolearning.InteractionPatternProposal,
		SubjectType:   archaeolearning.SubjectPattern,
		SubjectID:     "pattern-1",
		Title:         "Confirm pattern",
		Blocking:      true,
		Evidence: []archaeolearning.EvidenceRef{
			{Kind: "pattern", RefID: "pattern-1", Summary: "candidate pattern"},
		},
	})
	require.NoError(t, err)

	tensionSvc := archaeotensions.Service{Store: workflowStore}
	tension, err := tensionSvc.CreateOrUpdate(ctx, archaeotensions.CreateInput{
		WorkflowID:         "wf-history",
		ExplorationID:      session.ID,
		SnapshotID:         snapshot.ID,
		SourceRef:          "gap-1",
		PatternIDs:         []string{"pattern-1"},
		AnchorRefs:         []string{"anchor-1"},
		Kind:               "intent_gap",
		Description:        "history tension",
		Status:             archaeodomain.TensionAccepted,
		RelatedPlanStepIDs: []string{"step-1"},
		CommentRefs:        []string{"comment-1"},
		BasedOnRevision:    "rev-1",
	})
	require.NoError(t, err)

	reqSvc := archaeorequests.Service{Store: workflowStore}
	request, err := reqSvc.Create(ctx, archaeorequests.CreateInput{
		WorkflowID:      "wf-history",
		ExplorationID:   session.ID,
		SnapshotID:      snapshot.ID,
		Kind:            archaeodomain.RequestPatternSurfacing,
		Title:           "Refresh patterns",
		Description:     "request surfacing",
		RequestedBy:     "test",
		SubjectRefs:     []string{"pkg/history.go"},
		BasedOnRevision: "rev-1",
	})
	require.NoError(t, err)
	_, err = reqSvc.Dispatch(ctx, "wf-history", request.ID, map[string]any{"mode": "test"})
	require.NoError(t, err)
	_, err = reqSvc.Start(ctx, "wf-history", request.ID, map[string]any{"mode": "test"})
	require.NoError(t, err)
	_, err = reqSvc.Complete(ctx, archaeorequests.CompleteInput{
		WorkflowID: "wf-history",
		RequestID:  request.ID,
		Result: archaeodomain.RequestResult{
			Kind:    "pattern_records",
			Summary: "completed",
		},
	})
	require.NoError(t, err)

	now := time.Now().UTC()
	basePlan := &frameworkplan.LivingPlan{
		ID:         "plan-history",
		WorkflowID: "wf-history",
		Title:      "History plan",
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {ID: "step-1", Description: "resolve", Status: frameworkplan.PlanStepPending, CreatedAt: now, UpdatedAt: now},
		},
		StepOrder: []string{"step-1"},
		ConvergenceTarget: &frameworkplan.ConvergenceTarget{
			PatternIDs: []string{"pattern-1"},
			TensionIDs: []string{tension.ID},
		},
		Version:   1,
		CreatedAt: now,
		UpdatedAt: now,
	}
	planSvc := archaeoplans.Service{Store: planStore, WorkflowStore: workflowStore}
	v1, err := planSvc.DraftVersion(ctx, basePlan, archaeoplans.DraftVersionInput{
		WorkflowID:             "wf-history",
		DerivedFromExploration: session.ID,
		BasedOnRevision:        "rev-1",
		SemanticSnapshotRef:    "semantic-1",
		PatternRefs:            []string{"pattern-1"},
		TensionRefs:            []string{tension.ID},
		CommentRefs:            []string{"comment-plan"},
	})
	require.NoError(t, err)
	_, err = planSvc.ActivateVersion(ctx, "wf-history", v1.Version)
	require.NoError(t, err)
	_, err = planSvc.EnsureDraftSuccessor(ctx, "wf-history", v1.Version, "history successor")
	require.NoError(t, err)

	svc := Service{Store: workflowStore}

	mutations, err := svc.MutationHistory(ctx, "wf-history")
	require.NoError(t, err)
	require.NotNil(t, mutations)
	require.NotEmpty(t, mutations.Mutations)

	requests, err := svc.RequestHistory(ctx, "wf-history")
	require.NoError(t, err)
	require.NotNil(t, requests)
	require.Len(t, requests.Requests, 1)
	require.Equal(t, 1, requests.Completed)

	lineage, err := svc.PlanLineage(ctx, "wf-history")
	require.NoError(t, err)
	require.NotNil(t, lineage)
	require.Len(t, lineage.Versions, 2)
	require.NotNil(t, lineage.ActiveVersion)
	require.NotNil(t, lineage.LatestDraft)

	activity, err := svc.ExplorationActivity(ctx, "wf-history")
	require.NoError(t, err)
	require.NotNil(t, activity)
	require.Equal(t, session.ID, activity.ExplorationID)
	require.NotEmpty(t, activity.ActivityTimeline)
	require.Positive(t, activity.RequestCount)

	provenance, err := svc.Provenance(ctx, "wf-history")
	require.NoError(t, err)
	require.NotNil(t, provenance)
	require.Len(t, provenance.Learning, 1)
	require.Equal(t, interaction.ID, provenance.Learning[0].InteractionID)
	require.NotEmpty(t, provenance.Learning[0].Evidence)
	require.Len(t, provenance.Tensions, 1)
	require.Equal(t, tension.ID, provenance.Tensions[0].TensionID)
	require.NotEmpty(t, provenance.Tensions[0].MutationIDs)
	require.Len(t, provenance.PlanVersions, 2)
	require.True(t, anyPlanVersionHasCommentRefs(provenance.PlanVersions))

	coherence, err := svc.Coherence(ctx, "wf-history")
	require.NoError(t, err)
	require.NotNil(t, coherence)
	require.NotNil(t, coherence.TensionSummary)
	require.Equal(t, 1, coherence.AcceptedDebt)
	require.NotNil(t, coherence.ActivePlanVersion)
	require.Len(t, coherence.DraftPlanVersions, 1)
	require.NotEmpty(t, coherence.ConfidenceAffectingMutations)
}

func TestWorkspaceDeferredDecisionAndConvergenceProjections(t *testing.T) {
	ctx := context.Background()
	store, err := memorydb.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	require.NoError(t, store.CreateWorkflow(ctx, testWorkflowRecord("wf-workspace-proj")))

	_, err = (archaeodeferred.Service{Store: store}).CreateOrUpdate(ctx, archaeodeferred.CreateInput{
		WorkspaceID:  "/workspace/proj2",
		WorkflowID:   "wf-workspace-proj",
		AmbiguityKey: "step-1:type",
		Title:        "Need type choice",
	})
	require.NoError(t, err)
	conv, err := (archaeoconvergence.Service{Store: store}).Create(ctx, archaeoconvergence.CreateInput{
		WorkspaceID: "/workspace/proj2",
		WorkflowID:  "wf-workspace-proj",
		Question:    "Can execution proceed?",
	})
	require.NoError(t, err)
	_, err = (archaeodecisions.Service{Store: store}).Create(ctx, archaeodecisions.CreateInput{
		WorkspaceID:          "/workspace/proj2",
		WorkflowID:           "wf-workspace-proj",
		Kind:                 archaeodomain.DecisionKindConvergence,
		RelatedConvergenceID: conv.ID,
		Title:                "Need convergence decision",
	})
	require.NoError(t, err)

	svc := Service{Store: store}
	deferredProj, err := svc.DeferredDrafts(ctx, "/workspace/proj2")
	require.NoError(t, err)
	require.Len(t, deferredProj.Records, 1)
	convergenceProj, err := svc.ConvergenceHistory(ctx, "/workspace/proj2")
	require.NoError(t, err)
	require.NotNil(t, convergenceProj.Current)
	decisionProj, err := svc.DecisionTrail(ctx, "/workspace/proj2")
	require.NoError(t, err)
	require.Len(t, decisionProj.Records, 1)
}

func anyPlanVersionHasCommentRefs(values []PlanVersionProvenance) bool {
	for _, value := range values {
		if len(value.CommentRefs) > 0 {
			return true
		}
	}
	return false
}

func testWorkflowRecord(workflowID string) memory.WorkflowRecord {
	return memory.WorkflowRecord{
		WorkflowID:  workflowID,
		TaskID:      "task-" + workflowID,
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "test workflow",
		Status:      memory.WorkflowRunStatusRunning,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
}

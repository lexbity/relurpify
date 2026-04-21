package projections_test

import (
	"testing"
	"time"

	archaeoarch "codeburg.org/lexbit/relurpify/archaeo/archaeology"
	archaeoconvergence "codeburg.org/lexbit/relurpify/archaeo/convergence"
	archaeodecisions "codeburg.org/lexbit/relurpify/archaeo/decisions"
	archaeodeferred "codeburg.org/lexbit/relurpify/archaeo/deferred"
	archaeodomain "codeburg.org/lexbit/relurpify/archaeo/domain"
	archaeoevents "codeburg.org/lexbit/relurpify/archaeo/events"
	archaeolearning "codeburg.org/lexbit/relurpify/archaeo/learning"
	archaeoplans "codeburg.org/lexbit/relurpify/archaeo/plans"
	archaeoproj "codeburg.org/lexbit/relurpify/archaeo/projections"
	"codeburg.org/lexbit/relurpify/archaeo/requests"
	archaeotensions "codeburg.org/lexbit/relurpify/archaeo/tensions"
	"codeburg.org/lexbit/relurpify/archaeo/testscenario"
	"codeburg.org/lexbit/relurpify/framework/core"
	frameworkplan "codeburg.org/lexbit/relurpify/framework/plan"
	"github.com/stretchr/testify/require"
)

func TestProjectionBuildersAndMaterializers(t *testing.T) {
	f := testscenario.NewFixture(t)
	workflowID := "wf-projections"
	f.SeedWorkflow(workflowID, "seed projections")

	session, snapshot := f.SeedExploration(workflowID, f.Workspace, "rev-1", snapshotInput(workflowID, f.Workspace))
	plan := &frameworkplan.LivingPlan{
		ID:         "plan-projections",
		WorkflowID: workflowID,
		Title:      "Projection Plan",
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {ID: "step-1", Description: "step", Status: frameworkplan.PlanStepPending, CreatedAt: f.Now(), UpdatedAt: f.Now()},
		},
		StepOrder: []string{"step-1"},
		CreatedAt: f.Now(),
		UpdatedAt: f.Now(),
	}
	active := f.SeedActivePlan(plan, archaeoplans.DraftVersionInput{
		WorkflowID:             workflowID,
		DerivedFromExploration: session.ID,
		BasedOnRevision:        "rev-1",
		SemanticSnapshotRef:    snapshot.ID,
	})
	f.SeedPlanVersion(plan, archaeoplans.DraftVersionInput{
		WorkflowID:             workflowID,
		DerivedFromExploration: session.ID,
		BasedOnRevision:        "rev-2",
		SemanticSnapshotRef:    snapshot.ID,
	})
	tension := f.SeedTension(archaeotensions.CreateInput{
		WorkflowID:      workflowID,
		ExplorationID:   session.ID,
		SnapshotID:      snapshot.ID,
		SourceRef:       "source-1",
		Kind:            "gap",
		Description:     "gap",
		Status:          archaeodomain.TensionUnresolved,
		BasedOnRevision: "rev-1",
	})
	learning := f.SeedLearningInteraction(archaeolearning.CreateInput{
		WorkflowID:      workflowID,
		ExplorationID:   session.ID,
		Kind:            archaeolearning.InteractionIntentRefinement,
		SubjectType:     archaeolearning.SubjectExploration,
		SubjectID:       session.ID,
		Title:           "learn",
		Blocking:        true,
		BasedOnRevision: "rev-1",
	})
	reqSvc := requests.Service{Store: f.WorkflowStore, Now: f.Now, NewID: f.NewID}
	pendingReq, err := reqSvc.Create(f.Context(), requests.CreateInput{
		WorkflowID:      workflowID,
		Kind:            archaeodomain.RequestPatternSurfacing,
		Title:           "pending",
		IdempotencyKey:  "req-pending",
		BasedOnRevision: "rev-pending",
	})
	require.NoError(t, err)
	runningReq, err := reqSvc.Create(f.Context(), requests.CreateInput{
		WorkflowID:      workflowID,
		Kind:            archaeodomain.RequestPatternSurfacing,
		Title:           "running",
		IdempotencyKey:  "req-running",
		BasedOnRevision: "rev-running",
	})
	require.NoError(t, err)
	_, err = reqSvc.Dispatch(f.Context(), workflowID, runningReq.ID, nil)
	require.NoError(t, err)
	_, err = reqSvc.Start(f.Context(), workflowID, runningReq.ID, nil)
	require.NoError(t, err)
	completedReq, err := reqSvc.Create(f.Context(), requests.CreateInput{
		WorkflowID:      workflowID,
		Kind:            archaeodomain.RequestPatternSurfacing,
		Title:           "completed",
		IdempotencyKey:  "req-completed",
		BasedOnRevision: "rev-completed",
	})
	require.NoError(t, err)
	_, err = reqSvc.Complete(f.Context(), requests.CompleteInput{
		WorkflowID: workflowID,
		RequestID:  completedReq.ID,
		Result:     archaeodomain.RequestResult{Kind: "ok", RefID: "r-1", Summary: "done"},
	})
	require.NoError(t, err)
	failedReq, err := reqSvc.Create(f.Context(), requests.CreateInput{
		WorkflowID:      workflowID,
		Kind:            archaeodomain.RequestPatternSurfacing,
		Title:           "failed",
		IdempotencyKey:  "req-failed",
		BasedOnRevision: "rev-failed",
	})
	require.NoError(t, err)
	_, err = reqSvc.Fail(f.Context(), workflowID, failedReq.ID, "boom", true)
	require.NoError(t, err)
	canceledReq, err := reqSvc.Create(f.Context(), requests.CreateInput{
		WorkflowID:      workflowID,
		Kind:            archaeodomain.RequestPatternSurfacing,
		Title:           "canceled",
		IdempotencyKey:  "req-canceled",
		BasedOnRevision: "rev-canceled",
	})
	require.NoError(t, err)
	_, err = reqSvc.Cancel(f.Context(), workflowID, canceledReq.ID, "stop")
	require.NoError(t, err)

	deferredOpen := f.SeedDeferredDraft(archaeodeferred.CreateInput{
		WorkspaceID:   f.Workspace,
		WorkflowID:    workflowID,
		ExplorationID: session.ID,
		AmbiguityKey:  "open-1",
		Title:         "open",
	})
	f.SeedDeferredDraft(archaeodeferred.CreateInput{
		WorkspaceID:        f.Workspace,
		WorkflowID:         workflowID,
		ExplorationID:      session.ID,
		AmbiguityKey:       "formed-1",
		Title:              "formed",
		LinkedDraftVersion: testIntPtr(2),
		LinkedDraftPlanID:  "plan-projections",
	})
	convergence := f.SeedConvergenceRecord(archaeoconvergence.CreateInput{
		WorkspaceID:        f.Workspace,
		WorkflowID:         workflowID,
		ExplorationID:      session.ID,
		PlanID:             active.Plan.ID,
		PlanVersion:        &active.Version,
		Question:           "converge?",
		RelevantTensionIDs: []string{tension.ID},
		PendingLearningIDs: []string{learning.ID},
	})
	_, err = f.ConvergenceService().Resolve(f.Context(), archaeoconvergence.ResolveInput{
		WorkflowID: workflowID,
		RecordID:   convergence.ID,
		Resolution: archaeodomain.ConvergenceResolution{Status: archaeodomain.ConvergenceResolutionResolved, ChosenOption: "ok"},
	})
	require.NoError(t, err)
	decisionOpen, err := f.DecisionService().Create(f.Context(), archaeodecisions.CreateInput{
		WorkspaceID: f.Workspace,
		WorkflowID:  workflowID,
		Kind:        archaeodomain.DecisionKindConvergence,
		Title:       "decision open",
		Summary:     "open",
	})
	require.NoError(t, err)
	decisionResolved, err := f.DecisionService().Create(f.Context(), archaeodecisions.CreateInput{
		WorkspaceID: f.Workspace,
		WorkflowID:  workflowID,
		Kind:        archaeodomain.DecisionKindDeferredDraft,
		Title:       "decision resolved",
		Summary:     "resolved",
	})
	require.NoError(t, err)
	_, err = f.DecisionService().Resolve(f.Context(), archaeodecisions.ResolveInput{
		WorkflowID: workflowID,
		RecordID:   decisionResolved.ID,
		Status:     archaeodomain.DecisionStatusResolved,
	})
	require.NoError(t, err)

	f.SeedMutation(archaeodomain.MutationEvent{
		WorkflowID:  workflowID,
		PlanID:      active.Plan.ID,
		PlanVersion: testIntPtr(active.Version),
		Category:    archaeodomain.MutationObservation,
		SourceKind:  "observer",
		SourceRef:   "obs-1",
		Description: "note",
		Impact:      archaeodomain.ImpactInformational,
		Disposition: archaeodomain.DispositionContinue,
		BlastRadius: archaeodomain.BlastRadius{Scope: archaeodomain.BlastRadiusStep, AffectedStepIDs: []string{"step-1"}},
	})
	f.SeedMutation(archaeodomain.MutationEvent{
		WorkflowID:  workflowID,
		PlanID:      active.Plan.ID,
		PlanVersion: testIntPtr(active.Version),
		Category:    archaeodomain.MutationConfidenceChange,
		SourceKind:  "analyzer",
		SourceRef:   "m-2",
		Description: "confidence changed",
		Impact:      archaeodomain.ImpactPlanRecomputeRequired,
		Disposition: archaeodomain.DispositionRequireReplan,
		Blocking:    true,
		BlastRadius: archaeodomain.BlastRadius{Scope: archaeodomain.BlastRadiusPlan, AffectedStepIDs: []string{"step-1"}},
	})
	_ = archaeoevents.AppendWorkflowEvent(f.Context(), f.WorkflowStore, workflowID, archaeoevents.EventConvergenceFailed, "failed", map[string]any{
		"plan_id":                active.Plan.ID,
		"plan_version":           active.Version,
		"description":            "failed",
		"unresolved_tension_ids": []string{tension.ID},
		"based_on_revision":      "rev-2",
		"semantic_snapshot_ref":  snapshot.ID,
	}, f.Now())

	svc := f.ProjectionService()
	workflowProj, err := svc.Workflow(f.Context(), workflowID)
	require.NoError(t, err)
	require.NotNil(t, workflowProj)
	require.Equal(t, workflowID, workflowProj.WorkflowID)
	require.NotNil(t, workflowProj.ActiveExploration)
	require.NotNil(t, workflowProj.ActivePlanVersion)
	require.NotNil(t, workflowProj.ConvergenceState)

	explorationProj, err := svc.Exploration(f.Context(), workflowID)
	require.NoError(t, err)
	require.NotNil(t, explorationProj)

	learningQueue, err := svc.LearningQueue(f.Context(), workflowID)
	require.NoError(t, err)
	require.NotNil(t, learningQueue)

	activePlanProj, err := svc.ActivePlan(f.Context(), workflowID)
	require.NoError(t, err)
	require.NotNil(t, activePlanProj)

	timelineProj, err := svc.TimelineProjection(f.Context(), workflowID)
	require.NoError(t, err)
	require.NotNil(t, timelineProj)
	timeline, err := svc.Timeline(f.Context(), workflowID)
	require.NoError(t, err)
	require.NotEmpty(t, timeline)

	mutationHistory, err := svc.MutationHistory(f.Context(), workflowID)
	require.NoError(t, err)
	require.NotNil(t, mutationHistory)

	requestHistory, err := svc.RequestHistory(f.Context(), workflowID)
	require.NoError(t, err)
	require.NotNil(t, requestHistory)
	require.Len(t, requestHistory.Requests, 5)
	require.Equal(t, 1, requestHistory.Pending)
	require.Equal(t, 1, requestHistory.Running)
	require.Equal(t, 1, requestHistory.Completed)
	require.Equal(t, 1, requestHistory.Failed)
	require.Equal(t, 1, requestHistory.Canceled)

	lineage, err := svc.PlanLineage(f.Context(), workflowID)
	require.NoError(t, err)
	require.NotNil(t, lineage)
	require.NotNil(t, lineage.ActiveVersion)
	require.Equal(t, active.Version, lineage.ActiveVersion.Version)
	require.Len(t, lineage.DraftVersions, 1)

	explorationActivity, err := svc.ExplorationActivity(f.Context(), workflowID)
	require.NoError(t, err)
	require.NotNil(t, explorationActivity)
	require.NotEmpty(t, explorationActivity.ActivityTimeline)

	provenance, err := svc.Provenance(f.Context(), workflowID)
	require.NoError(t, err)
	require.NotNil(t, provenance)
	require.NotEmpty(t, provenance.Mutations)

	convergenceProj, err := svc.ConvergenceHistory(f.Context(), f.Workspace)
	require.NoError(t, err)
	require.NotNil(t, convergenceProj)

	decisionTrail, err := svc.DecisionTrail(f.Context(), f.Workspace)
	require.NoError(t, err)
	require.NotNil(t, decisionTrail)
	require.Equal(t, 1, decisionTrail.OpenCount)
	require.Equal(t, 1, decisionTrail.Resolved)

	deferredProj, err := svc.DeferredDrafts(f.Context(), f.Workspace)
	require.NoError(t, err)
	require.NotNil(t, deferredProj)
	require.Equal(t, 1, deferredProj.OpenCount)
	require.Equal(t, 1, deferredProj.FormedCount)

	coherenceProj, err := svc.Coherence(f.Context(), workflowID)
	require.NoError(t, err)
	require.NotNil(t, coherenceProj)
	require.NotNil(t, coherenceProj.ActivePlanVersion)
	require.NotNil(t, coherenceProj.ConvergenceState)
	require.Greater(t, coherenceProj.BlockingMutationCount, 0)

	events := []core.FrameworkEvent{{Type: archaeoevents.EventRequestCreated, Seq: 1, Timestamp: f.Now()}}
	phaseMat := &archaeoproj.PhaseStateMaterializer{Store: f.WorkflowStore, WorkflowID: workflowID}
	require.Equal(t, "archaeo-phase-state-projection", phaseMat.Name())
	require.NoError(t, phaseMat.Apply(f.Context(), events))
	data, err := phaseMat.Snapshot(f.Context())
	require.NoError(t, err)
	require.NoError(t, (&archaeoproj.PhaseStateMaterializer{}).Restore(f.Context(), data))

	explorationMat := &archaeoproj.ExplorationMaterializer{Store: f.WorkflowStore, WorkflowID: workflowID}
	require.Equal(t, "archaeo-exploration-projection", explorationMat.Name())
	require.NoError(t, explorationMat.Apply(f.Context(), events))
	data, err = explorationMat.Snapshot(f.Context())
	require.NoError(t, err)
	require.NoError(t, (&archaeoproj.ExplorationMaterializer{}).Restore(f.Context(), data))

	learningMat := &archaeoproj.LearningQueueMaterializer{Store: f.WorkflowStore, WorkflowID: workflowID}
	require.Equal(t, "archaeo-learning-queue-projection", learningMat.Name())
	require.NoError(t, learningMat.Apply(f.Context(), events))
	data, err = learningMat.Snapshot(f.Context())
	require.NoError(t, err)
	require.NoError(t, (&archaeoproj.LearningQueueMaterializer{}).Restore(f.Context(), data))

	activePlanMat := &archaeoproj.ActivePlanMaterializer{Store: f.WorkflowStore, WorkflowID: workflowID}
	require.Equal(t, "archaeo-active-plan-projection", activePlanMat.Name())
	require.NoError(t, activePlanMat.Apply(f.Context(), events))
	data, err = activePlanMat.Snapshot(f.Context())
	require.NoError(t, err)
	require.NoError(t, (&archaeoproj.ActivePlanMaterializer{}).Restore(f.Context(), data))

	timelineMat := &archaeoproj.TimelineMaterializer{Store: f.WorkflowStore, WorkflowID: workflowID}
	require.Equal(t, "archaeo-workflow-timeline-projection", timelineMat.Name())
	require.NoError(t, timelineMat.Apply(f.Context(), events))
	data, err = timelineMat.Snapshot(f.Context())
	require.NoError(t, err)
	require.NoError(t, (&archaeoproj.TimelineMaterializer{}).Restore(f.Context(), data))

	mutationMat := &archaeoproj.MutationHistoryMaterializer{Store: f.WorkflowStore, WorkflowID: workflowID}
	require.Equal(t, "archaeo-mutation-history-projection", mutationMat.Name())
	require.NoError(t, mutationMat.Apply(f.Context(), events))
	data, err = mutationMat.Snapshot(f.Context())
	require.NoError(t, err)
	require.NoError(t, (&archaeoproj.MutationHistoryMaterializer{}).Restore(f.Context(), data))

	requestMat := &archaeoproj.RequestHistoryMaterializer{Store: f.WorkflowStore, WorkflowID: workflowID}
	require.Equal(t, "archaeo-request-history-projection", requestMat.Name())
	require.NoError(t, requestMat.Apply(f.Context(), events))
	data, err = requestMat.Snapshot(f.Context())
	require.NoError(t, err)
	require.NoError(t, (&archaeoproj.RequestHistoryMaterializer{}).Restore(f.Context(), data))

	lineageMat := &archaeoproj.PlanLineageMaterializer{Store: f.WorkflowStore, WorkflowID: workflowID}
	require.Equal(t, "archaeo-plan-lineage-projection", lineageMat.Name())
	require.NoError(t, lineageMat.Apply(f.Context(), events))
	data, err = lineageMat.Snapshot(f.Context())
	require.NoError(t, err)
	require.NoError(t, (&archaeoproj.PlanLineageMaterializer{}).Restore(f.Context(), data))

	explorationActivityMat := &archaeoproj.ExplorationActivityMaterializer{Store: f.WorkflowStore, WorkflowID: workflowID}
	require.Equal(t, "archaeo-exploration-activity-projection", explorationActivityMat.Name())
	require.NoError(t, explorationActivityMat.Apply(f.Context(), events))
	data, err = explorationActivityMat.Snapshot(f.Context())
	require.NoError(t, err)
	require.NoError(t, (&archaeoproj.ExplorationActivityMaterializer{}).Restore(f.Context(), data))

	provenanceMat := &archaeoproj.ProvenanceMaterializer{Store: f.WorkflowStore, WorkflowID: workflowID}
	require.Equal(t, "archaeo-provenance-projection", provenanceMat.Name())
	require.NoError(t, provenanceMat.Apply(f.Context(), events))
	data, err = provenanceMat.Snapshot(f.Context())
	require.NoError(t, err)
	require.NoError(t, (&archaeoproj.ProvenanceMaterializer{}).Restore(f.Context(), data))

	coherenceMat := &archaeoproj.CoherenceMaterializer{Store: f.WorkflowStore, WorkflowID: workflowID}
	require.Equal(t, "archaeo-coherence-projection", coherenceMat.Name())
	require.NoError(t, coherenceMat.Apply(f.Context(), events))
	data, err = coherenceMat.Snapshot(f.Context())
	require.NoError(t, err)
	require.NoError(t, (&archaeoproj.CoherenceMaterializer{}).Restore(f.Context(), data))

	phaseMat.Projection = workflowProj.PhaseState
	explorationMat.Projection = explorationProj
	learningMat.Projection = learningQueue
	activePlanMat.Projection = activePlanProj
	mutationMat.Projection = mutationHistory
	requestMat.Projection = requestHistory
	lineageMat.Projection = lineage
	explorationActivityMat.Projection = explorationActivity
	provenanceMat.Projection = provenance
	coherenceMat.Projection = coherenceProj
	irrel := []core.FrameworkEvent{{Type: "irrelevant", Seq: 2, Timestamp: f.Now()}}
	require.NoError(t, phaseMat.Apply(f.Context(), irrel))
	require.NoError(t, explorationMat.Apply(f.Context(), irrel))
	require.NoError(t, learningMat.Apply(f.Context(), irrel))
	require.NoError(t, activePlanMat.Apply(f.Context(), irrel))
	require.NoError(t, mutationMat.Apply(f.Context(), irrel))
	require.NoError(t, requestMat.Apply(f.Context(), irrel))
	require.NoError(t, lineageMat.Apply(f.Context(), irrel))
	require.NoError(t, explorationActivityMat.Apply(f.Context(), irrel))
	require.NoError(t, provenanceMat.Apply(f.Context(), irrel))
	require.NoError(t, coherenceMat.Apply(f.Context(), irrel))

	composite := &archaeoproj.CompositeMaterializer{WorkflowID: workflowID, Service: svc}
	require.Equal(t, "archaeo-workflow-projection", composite.Name())
	require.NoError(t, composite.Apply(f.Context(), events))
	data, err = composite.Snapshot(f.Context())
	require.NoError(t, err)
	require.NoError(t, (&archaeoproj.CompositeMaterializer{}).Restore(f.Context(), data))
	require.NoError(t, composite.Apply(f.Context(), nil))

	subCh, unsub := svc.SubscribeWorkflow(workflowID, 1)
	defer unsub()
	select {
	case evt := <-subCh:
		require.Equal(t, archaeoproj.ProjectionEventSnapshot, evt.Type)
		require.Equal(t, workflowID, evt.WorkflowID)
	case <-time.After(2 * time.Second):
		t.Fatal("expected projection subscription event")
	}

	require.NotNil(t, pendingReq)
	require.NotNil(t, decisionOpen)
	require.NotNil(t, deferredOpen)
}

func snapshotInput(workflowID, workspaceID string) archaeoarch.SnapshotInput {
	return archaeoarch.SnapshotInput{
		WorkflowID:      workflowID,
		WorkspaceID:     workspaceID,
		BasedOnRevision: "rev-1",
		Summary:         "snapshot",
	}
}

func testIntPtr(v int) *int { return &v }

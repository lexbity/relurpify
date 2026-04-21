package relurpishbindings

import (
	"context"
	"testing"
	"time"

	archaeoarch "codeburg.org/lexbit/relurpify/archaeo/archaeology"
	archaeoconvergence "codeburg.org/lexbit/relurpify/archaeo/convergence"
	archaeodecisions "codeburg.org/lexbit/relurpify/archaeo/decisions"
	archaeodomain "codeburg.org/lexbit/relurpify/archaeo/domain"
	archaeolearning "codeburg.org/lexbit/relurpify/archaeo/learning"
	archaeoplans "codeburg.org/lexbit/relurpify/archaeo/plans"
	archaeotensions "codeburg.org/lexbit/relurpify/archaeo/tensions"
	archaeotestscenario "codeburg.org/lexbit/relurpify/archaeo/testscenario"
	frameworkplan "codeburg.org/lexbit/relurpify/framework/plan"
	"github.com/stretchr/testify/require"
)

func TestRuntimeServiceFactories(t *testing.T) {
	var zero Runtime
	require.Nil(t, zero.LearningService().Phases)

	f := archaeotestscenario.New(t)
	rt := Runtime{
		WorkflowStore:  f.WorkflowStore,
		PlanStore:      f.PlanStore,
		PatternStore:   f.PatternStore,
		CommentStore:   f.CommentStore,
		Retrieval:      f.Retrieval,
		LearningBroker: nil,
	}

	require.Equal(t, f.WorkflowStore, rt.ArchaeologyService().Store)
	require.Equal(t, f.WorkflowStore, rt.PhaseService().Store)
	require.Equal(t, f.PlanStore, rt.PlanService().Store)

	learningSvc := rt.LearningService()
	require.NotNil(t, learningSvc.Phases)
	require.Equal(t, f.WorkflowStore, learningSvc.Store)
	require.Equal(t, f.PatternStore, learningSvc.PatternStore)
	require.Equal(t, f.CommentStore, learningSvc.CommentStore)
	require.Equal(t, f.PlanStore, learningSvc.PlanStore)
	require.Equal(t, f.Retrieval, learningSvc.Retrieval)

	require.Equal(t, f.WorkflowStore, rt.TensionService().Store)
	require.Equal(t, f.WorkflowStore, rt.ProjectionService().Store)
	require.Equal(t, f.WorkflowStore, rt.DeferredDraftService().Store)
	require.Equal(t, f.WorkflowStore, rt.ConvergenceService().Store)
	require.Equal(t, f.WorkflowStore, rt.DecisionService().Store)
}

func TestRuntimeDelegatesAcrossBoundaries(t *testing.T) {
	ctx := context.Background()
	f := archaeotestscenario.New(t)
	workflowID := "wf-relurpish-binding"
	f.SeedWorkflow(workflowID, "exercise relurpish binding runtime")

	now := f.Now()
	plan := &frameworkplan.LivingPlan{
		ID:         "plan-relurpish-binding",
		WorkflowID: workflowID,
		Title:      "Binding Plan",
		Steps: map[string]*frameworkplan.PlanStep{
			"inspect": {
				ID:          "inspect",
				Description: "Inspect binding runtime",
				Status:      frameworkplan.PlanStepPending,
				CreatedAt:   now,
				UpdatedAt:   now,
			},
		},
		StepOrder: []string{"inspect"},
		CreatedAt: now,
		UpdatedAt: now,
	}
	activePlan := f.SeedActivePlan(plan, archaeoplans.DraftVersionInput{WorkflowID: workflowID, BasedOnRevision: "rev-1"})
	session, snapshot := f.SeedExploration(workflowID, f.Workspace, "rev-1", archaeoarch.SnapshotInput{
		WorkflowID:      workflowID,
		WorkspaceID:     f.Workspace,
		BasedOnRevision: "rev-1",
		Summary:         "prepared snapshot",
	})
	tension := f.SeedTension(archaeotensions.CreateInput{
		WorkflowID:      workflowID,
		ExplorationID:   session.ID,
		SnapshotID:      snapshot.ID,
		SourceRef:       "tension-binding",
		Kind:            "scope_gap",
		Description:     "Binding tension",
		Status:          archaeodomain.TensionUnresolved,
		BasedOnRevision: "rev-1",
	})
	interaction := f.SeedLearningInteraction(archaeolearning.CreateInput{
		WorkflowID:      workflowID,
		ExplorationID:   session.ID,
		Kind:            archaeolearning.InteractionIntentRefinement,
		SubjectType:     archaeolearning.SubjectExploration,
		SubjectID:       session.ID,
		Title:           "Refine exploration intent",
		BasedOnRevision: "rev-1",
	})

	rt := Runtime{
		WorkflowStore: f.WorkflowStore,
		PlanStore:     f.PlanStore,
		PatternStore:  f.PatternStore,
		CommentStore:  f.CommentStore,
		Retrieval:     f.Retrieval,
	}

	zeroCh, zeroUnsub := Runtime{}.SubscribeWorkflowProjection("", 1)
	defer zeroUnsub()
	select {
	case _, ok := <-zeroCh:
		require.False(t, ok)
	default:
		t.Fatal("expected closed subscription channel for empty runtime")
	}

	service := rt.ArchaeologyService()
	activeExploration, err := rt.ActiveExploration(ctx, f.Workspace)
	require.NoError(t, err)
	require.NotNil(t, activeExploration)
	require.Equal(t, session.ID, activeExploration.Session.ID)

	view, err := rt.ExplorationView(ctx, session.ID)
	require.NoError(t, err)
	require.NotNil(t, view)

	pendingLearning, err := rt.PendingLearning(ctx, workflowID)
	require.NoError(t, err)
	require.Len(t, pendingLearning, 1)

	resolvedLearning, err := rt.ResolveLearning(ctx, archaeolearning.ResolveInput{
		WorkflowID:     workflowID,
		InteractionID:  interaction.ID,
		Kind:           archaeolearning.ResolutionConfirm,
		ExpectedStatus: archaeolearning.StatusPending,
		ResolvedBy:     "binding-test",
	})
	require.NoError(t, err)
	require.NotNil(t, resolvedLearning)
	require.Equal(t, archaeolearning.StatusResolved, resolvedLearning.Status)

	tensionsByWorkflow, err := rt.TensionsByWorkflow(ctx, workflowID)
	require.NoError(t, err)
	require.Len(t, tensionsByWorkflow, 1)

	tensionsByExploration, err := rt.TensionsByExploration(ctx, session.ID)
	require.NoError(t, err)
	require.Len(t, tensionsByExploration, 1)

	updatedTension, err := rt.UpdateTensionStatus(ctx, workflowID, tension.ID, archaeodomain.TensionResolved, []string{"comment-1"})
	require.NoError(t, err)
	require.NotNil(t, updatedTension)
	require.Equal(t, archaeodomain.TensionResolved, updatedTension.Status)

	summaryByWorkflow, err := rt.TensionSummaryByWorkflow(ctx, workflowID)
	require.NoError(t, err)
	require.NotNil(t, summaryByWorkflow)

	summaryByExploration, err := rt.TensionSummaryByExploration(ctx, session.ID)
	require.NoError(t, err)
	require.NotNil(t, summaryByExploration)

	workflowProjection, err := rt.WorkflowProjection(ctx, workflowID)
	require.NoError(t, err)
	require.NotNil(t, workflowProjection)

	explorationProjection, err := rt.ExplorationProjection(ctx, workflowID)
	require.NoError(t, err)
	require.NotNil(t, explorationProjection)

	learningQueue, err := rt.LearningQueueProjection(ctx, workflowID)
	require.NoError(t, err)
	require.NotNil(t, learningQueue)

	activePlanProjection, err := rt.ActivePlanProjection(ctx, workflowID)
	require.NoError(t, err)
	require.NotNil(t, activePlanProjection)

	timeline, err := rt.WorkflowTimeline(ctx, workflowID)
	require.NoError(t, err)
	require.NotNil(t, timeline)

	versions, err := rt.PlanVersions(ctx, workflowID)
	require.NoError(t, err)
	require.Len(t, versions, 1)

	activeVersion, err := rt.ActivePlanVersion(ctx, workflowID)
	require.NoError(t, err)
	require.NotNil(t, activeVersion)
	require.Equal(t, activePlan.Version, activeVersion.Version)

	comparison, err := rt.ComparePlanVersions(ctx, workflowID, activeVersion.Version, activeVersion.Version)
	require.NoError(t, err)
	require.NotNil(t, comparison)

	deferredDrafts, err := rt.DeferredDrafts(ctx, f.Workspace)
	require.NoError(t, err)
	require.NotNil(t, deferredDrafts)

	convergenceHistory, err := rt.ConvergenceHistory(ctx, f.Workspace)
	require.NoError(t, err)
	require.NotNil(t, convergenceHistory)

	decisionTrail, err := rt.DecisionTrail(ctx, f.Workspace)
	require.NoError(t, err)
	require.NotNil(t, decisionTrail)

	convergenceRecord, err := rt.CreateConvergenceRecord(ctx, archaeoconvergence.CreateInput{
		WorkspaceID:        f.Workspace,
		WorkflowID:         workflowID,
		ExplorationID:      session.ID,
		PlanID:             activePlan.ID,
		PlanVersion:        &activePlan.Version,
		Question:           "Should this binding remain thin?",
		Title:              "Binding convergence",
		RelevantTensionIDs: []string{tension.ID},
		Metadata:           map[string]any{"source": "test"},
	})
	require.NoError(t, err)
	require.NotNil(t, convergenceRecord)

	resolvedConvergence, err := rt.ResolveConvergenceRecord(ctx, archaeoconvergence.ResolveInput{
		WorkflowID: workflowID,
		RecordID:   convergenceRecord.ID,
		Resolution: archaeodomain.ConvergenceResolution{
			Status:       archaeodomain.ConvergenceResolutionResolved,
			ChosenOption: "yes",
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resolvedConvergence)
	require.Equal(t, archaeodomain.ConvergenceResolutionResolved, resolvedConvergence.Status)

	decisionRecord, err := rt.CreateDecisionRecord(ctx, archaeodecisions.CreateInput{
		WorkspaceID:   f.Workspace,
		WorkflowID:    workflowID,
		Kind:          archaeodomain.DecisionKindConvergence,
		Title:         "Binding decision",
		Summary:       "Keep the binding small",
		RelatedPlanID: activePlan.ID,
	})
	require.NoError(t, err)
	require.NotNil(t, decisionRecord)

	resolvedDecision, err := rt.ResolveDecisionRecord(ctx, archaeodecisions.ResolveInput{
		WorkflowID: workflowID,
		RecordID:   decisionRecord.ID,
		Status:     archaeodomain.DecisionStatusResolved,
	})
	require.NoError(t, err)
	require.NotNil(t, resolvedDecision)
	require.Equal(t, archaeodomain.DecisionStatusResolved, resolvedDecision.Status)

	_ = service

	_ = rt.ArchaeologyService()
	_ = rt.PhaseService()
	_ = rt.PlanService()
	_ = rt.LearningService()
	_ = rt.TensionService()
	_ = rt.ProjectionService()
	_ = rt.DeferredDraftService()
	_ = rt.ConvergenceService()
	_ = rt.DecisionService()
	_ = rt.SubscribeWorkflowProjection

	select {
	case _, ok := <-zeroCh:
		require.False(t, ok)
	case <-time.After(time.Second):
		t.Fatal("subscription channel was not closed")
	}
}

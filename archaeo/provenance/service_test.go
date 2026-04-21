package provenance_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	archaeoarch "codeburg.org/lexbit/relurpify/archaeo/archaeology"
	archaeoconvergence "codeburg.org/lexbit/relurpify/archaeo/convergence"
	archaeodecisions "codeburg.org/lexbit/relurpify/archaeo/decisions"
	archaeodeferred "codeburg.org/lexbit/relurpify/archaeo/deferred"
	archaeodomain "codeburg.org/lexbit/relurpify/archaeo/domain"
	archaeoevents "codeburg.org/lexbit/relurpify/archaeo/events"
	archaeolearning "codeburg.org/lexbit/relurpify/archaeo/learning"
	archaeoprovenance "codeburg.org/lexbit/relurpify/archaeo/provenance"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
	memorydb "codeburg.org/lexbit/relurpify/framework/memory/db"
	"github.com/stretchr/testify/require"
)

func TestBuildUsesSnapshotAndRebuildsWhenWorkspaceRecordsChange(t *testing.T) {
	ctx := context.Background()
	store, err := memorydb.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	now := time.Date(2026, 3, 28, 0, 0, 0, 0, time.UTC)
	require.NoError(t, store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-prov",
		TaskID:      "task-wf-prov",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "provenance workflow",
		Status:      memory.WorkflowRunStatusRunning,
		CreatedAt:   now,
		UpdatedAt:   now,
	}))

	archSvc := archaeoarch.Service{Store: store, Learning: archaeolearning.Service{Store: store}}
	session, err := archSvc.EnsureExplorationSession(ctx, "wf-prov", "/workspace/prov", "rev-1")
	require.NoError(t, err)
	require.NotNil(t, session)
	snapshot, err := archSvc.CreateExplorationSnapshot(ctx, session, archaeoarch.SnapshotInput{
		WorkflowID:      "wf-prov",
		WorkspaceID:     "/workspace/prov",
		BasedOnRevision: "rev-1",
		Summary:         "snapshot",
	})
	require.NoError(t, err)
	require.NotNil(t, snapshot)

	learningSvc := archaeolearning.Service{Store: store}
	interaction, err := learningSvc.Create(ctx, archaeolearning.CreateInput{
		WorkflowID:    "wf-prov",
		ExplorationID: session.ID,
		Kind:          archaeolearning.InteractionIntentRefinement,
		SubjectType:   archaeolearning.SubjectExploration,
		SubjectID:     session.ID,
		Title:         "Clarify intent",
		Blocking:      true,
	})
	require.NoError(t, err)
	_, err = learningSvc.Resolve(ctx, archaeolearning.ResolveInput{
		WorkflowID:    "wf-prov",
		InteractionID: interaction.ID,
		Kind:          archaeolearning.ResolutionConfirm,
		ChoiceID:      "confirm",
		ResolvedBy:    "tester",
	})
	require.NoError(t, err)

	require.NoError(t, archaeoevents.AppendMutationEvent(ctx, store, archaeodomain.MutationEvent{
		WorkflowID:  "wf-prov",
		Category:    archaeodomain.MutationObservation,
		Impact:      archaeodomain.ImpactInformational,
		Disposition: archaeodomain.DispositionContinue,
		Description: "seed mutation",
		CreatedAt:   now,
	}))

	svc := archaeoprovenance.Service{Store: store}
	record, err := svc.Build(ctx, "wf-prov")
	require.NoError(t, err)
	require.NotNil(t, record)

	snapshotArtifact, ok, err := store.WorkflowArtifactByID(ctx, "provenance-snapshot:wf-prov")
	require.NoError(t, err)
	require.True(t, ok)
	require.NotNil(t, snapshotArtifact)
	firstSnapshotTime := snapshotArtifact.CreatedAt
	firstSnapshotRaw := snapshotArtifact.InlineRawText

	recordAgain, err := svc.Build(ctx, "wf-prov")
	require.NoError(t, err)
	require.Equal(t, record, recordAgain)
	snapshotArtifact, ok, err = store.WorkflowArtifactByID(ctx, "provenance-snapshot:wf-prov")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, firstSnapshotTime, snapshotArtifact.CreatedAt)

	deferredSvc := archaeodeferred.Service{Store: store}
	deferredRecord, err := deferredSvc.CreateOrUpdate(ctx, archaeodeferred.CreateInput{
		WorkspaceID:  "/workspace/prov",
		WorkflowID:   "wf-prov",
		PlanID:       "plan-1",
		RequestID:    "request-1",
		AmbiguityKey: "step-1:ambiguity",
		Title:        "Need clarification",
	})
	require.NoError(t, err)
	require.NotNil(t, deferredRecord)

	convergenceSvc := archaeoconvergence.Service{Store: store}
	convergenceRecord, err := convergenceSvc.Create(ctx, archaeoconvergence.CreateInput{
		WorkspaceID:      "/workspace/prov",
		WorkflowID:       "wf-prov",
		Question:         "Proceed?",
		DeferredDraftIDs: []string{deferredRecord.ID},
	})
	require.NoError(t, err)
	require.NotNil(t, convergenceRecord)

	decisionSvc := archaeodecisions.Service{Store: store}
	_, err = decisionSvc.Create(ctx, archaeodecisions.CreateInput{
		WorkspaceID:          "/workspace/prov",
		WorkflowID:           "wf-prov",
		Kind:                 archaeodomain.DecisionKindConvergence,
		RelatedConvergenceID: convergenceRecord.ID,
		Title:                "Need user decision",
	})
	require.NoError(t, err)

	updated, err := svc.Build(ctx, "wf-prov")
	require.NoError(t, err)
	require.NotEmpty(t, updated.DeferredDraftRefs)
	require.NotEmpty(t, updated.ConvergenceRefs)
	require.NotEmpty(t, updated.DecisionRefs)

	snapshotArtifact, ok, err = store.WorkflowArtifactByID(ctx, "provenance-snapshot:wf-prov")
	require.NoError(t, err)
	require.True(t, ok)
	require.NotEqual(t, firstSnapshotRaw, snapshotArtifact.InlineRawText)
	require.True(t, snapshotArtifact.CreatedAt.After(firstSnapshotTime) || snapshotArtifact.CreatedAt.Equal(firstSnapshotTime))
}

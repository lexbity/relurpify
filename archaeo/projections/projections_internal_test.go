package projections

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	archaeodomain "codeburg.org/lexbit/relurpify/archaeo/domain"
	archaeoevents "codeburg.org/lexbit/relurpify/archaeo/events"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
	memorydb "codeburg.org/lexbit/relurpify/framework/memory/db"
	"github.com/stretchr/testify/require"
)

func TestProjectionHelpers(t *testing.T) {
	svc := &Service{}
	require.Equal(t, 100*time.Millisecond, svc.pollInterval())
	require.False(t, hasRelevantEvent(nil))
	require.True(t, hasRelevantEvent([]core.FrameworkEvent{{Type: "anything"}}))
	require.True(t, hasRelevantEvent([]core.FrameworkEvent{{Type: archaeoevents.EventRequestCreated}}, archaeoevents.EventRequestCreated))
	require.False(t, hasRelevantEvent([]core.FrameworkEvent{{Type: archaeoevents.EventRequestCreated}}, archaeoevents.EventLearningInteractionResolved))

	require.Equal(t, "projection-workflow", projectionSnapshotKey("workflow"))
	require.Equal(t, "wf", projectionPartition("wf", ""))
	require.Equal(t, "wf::snapshot:snap-1", projectionPartition(" wf ", " snap-1 "))

	require.Equal(t, 100*time.Millisecond, (&Service{}).pollInterval())
	require.Equal(t, 25*time.Millisecond, (&Service{PollInterval: 25 * time.Millisecond}).pollInterval())

	now := time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC)
	require.WithinDuration(t, now, (&Service{Now: func() time.Time { return now }}).now(), time.Second)

	var addSvc Service
	ch := make(chan ProjectionEvent, 1)
	cancelCalled := false
	id := addSvc.addSub("wf-1", ch, func() { cancelCalled = true })
	require.Equal(t, 0, id)
	require.NotNil(t, addSvc.subs)
	addSvc.removeSub(id)
	require.Len(t, addSvc.subs, 0)
	addSvc.removeSub(42)
	require.False(t, cancelCalled)

	req := core.FrameworkEvent{Seq: 7, Timestamp: now, Type: archaeoevents.EventRequestCreated, Payload: mustJSON(t, workflowEventPayload{
		EventID:    "evt-1",
		WorkflowID: "wf-1",
		RunID:      "run-1",
		StepID:     "step-1",
		Message:    "created",
		Metadata:   map[string]any{"exploration_id": "explore-1"},
	})}
	timeline := timelineFromFrameworkEvents([]core.FrameworkEvent{req})
	require.Len(t, timeline, 1)
	require.Equal(t, uint64(7), timeline[0].Seq)
	require.Equal(t, "evt-1", timeline[0].EventID)
	require.Equal(t, "wf-1", timeline[0].WorkflowID)
	require.Equal(t, "run-1", timeline[0].RunID)
	require.Equal(t, "step-1", timeline[0].StepID)
	require.Equal(t, "created", timeline[0].Message)
	require.Equal(t, "explore-1", timeline[0].Metadata["exploration_id"])

	records := []memory.WorkflowEventRecord{
		{
			EventID:    "evt-2",
			WorkflowID: "wf-1",
			RunID:      "run-1",
			StepID:     "step-2",
			EventType:  archaeoevents.EventConvergenceFailed,
			Message:    "failed",
			Metadata:   map[string]any{"archaeo_seq": 9, "plan_version": 2},
			CreatedAt:  now,
		},
	}
	timeline = timelineFromWorkflowRecords(records)
	require.Len(t, timeline, 1)
	require.Equal(t, uint64(9), timeline[0].Seq)
	require.Equal(t, "evt-2", timeline[0].EventID)
	require.Equal(t, 2, intValue(timeline[0].Metadata["plan_version"]))

	failed := convergenceFromTimeline([]archaeodomain.TimelineEvent{
		{Seq: 1, EventType: archaeoevents.EventConvergenceVerified, Metadata: map[string]any{"plan_version": 3, "plan_id": "plan-1", "based_on_revision": "rev-1", "semantic_snapshot_ref": "snap-1"}, CreatedAt: now},
		{Seq: 2, EventType: archaeoevents.EventConvergenceFailed, Metadata: map[string]any{"plan_version": 4, "plan_id": "plan-2", "description": "failed", "unresolved_tension_ids": []any{"t-1"}, "based_on_revision": "rev-2", "semantic_snapshot_ref": "snap-2"}, CreatedAt: now.Add(time.Minute)},
	})
	require.NotNil(t, failed)
	require.Equal(t, archaeodomain.ConvergenceStatusFailed, failed.Status)
	require.Equal(t, "plan-2", failed.PlanID)
	require.Equal(t, 4, *failed.PlanVersion)
	require.Equal(t, []string{"t-1"}, failed.UnresolvedTensionIDs)

	verified := convergenceFromTimeline([]archaeodomain.TimelineEvent{
		{Seq: 1, EventType: archaeoevents.EventConvergenceVerified, Metadata: map[string]any{"plan_version": 3, "plan_id": "plan-1", "based_on_revision": "rev-1", "semantic_snapshot_ref": "snap-1"}, CreatedAt: now},
	})
	require.NotNil(t, verified)
	require.Equal(t, archaeodomain.ConvergenceStatusVerified, verified.Status)

	require.Equal(t, "value", stringValue("value"))
	require.Equal(t, 0, intValue("bad"))
	require.Equal(t, 10, intValue("10"))
	require.Equal(t, 2, intValue(float64(2)))
	require.Equal(t, 4, intValue(json.Number("4")))
	require.Equal(t, 6, intValue(int64(6)))
	require.Equal(t, 7, intValue(int(7)))

	v := 11
	cloned := intPointerClone(&v)
	require.NotNil(t, cloned)
	require.Equal(t, 11, *cloned)
	require.NotSame(t, &v, cloned)
	require.Nil(t, intPointerClone(nil))

	require.Equal(t, 12, *intPointer(int64(12)))
	require.Equal(t, 13, *intPointer(float64(13)))
	require.Nil(t, intPointer("nope"))

	require.Equal(t, []string{"a", "b"}, stringSlice([]any{"a", " ", "b", 3}))
	require.Nil(t, stringSlice("nope"))

	src := map[string]any{"a": 1}
	clone := cloneMap(src)
	require.Equal(t, src, clone)
	clone["a"] = 2
	require.Equal(t, 1, src["a"])
	require.Equal(t, uint64(5), uint64Value("5"))
	require.Equal(t, uint64(6), uint64Value(float64(6)))
	require.Equal(t, uint64(7), uint64Value(json.Number("7")))
	require.Equal(t, uint64(8), uint64Value(int64(8)))
	require.Equal(t, uint64(9), uint64Value(int(9)))
	require.Equal(t, uint64(0), uint64Value(-1))

	require.True(t, explorationActivityRelevant(archaeodomain.TimelineEvent{EventType: archaeoevents.EventExplorationSnapshotUpserted}, ""))
	require.True(t, explorationActivityRelevant(archaeodomain.TimelineEvent{EventType: archaeoevents.EventMutationRecorded, Metadata: map[string]any{"exploration_id": "explore-1"}}, "explore-1"))
	require.False(t, explorationActivityRelevant(archaeodomain.TimelineEvent{EventType: "other"}, "explore-1"))
}

func TestProjectionSubscriptionAndMaterializerGuards(t *testing.T) {
	svc := &Service{}
	ch, cancel := svc.SubscribeWorkflow("", 1)
	defer cancel()
	select {
	case _, ok := <-ch:
		require.False(t, ok)
	default:
		t.Fatal("expected closed channel for blank subscription")
	}

	workflowNil, err := (*Service)(nil).Workflow(context.Background(), "wf-none")
	require.NoError(t, err)
	require.Nil(t, workflowNil)
	builtWorkflowNil, err := (*Service)(nil).buildWorkflow(context.Background(), "wf-none")
	require.NoError(t, err)
	require.Nil(t, builtWorkflowNil)
	mutationHistoryNil, err := (*Service)(nil).MutationHistory(context.Background(), "wf-none")
	require.NoError(t, err)
	require.Nil(t, mutationHistoryNil)
	requestHistoryNil, err := (*Service)(nil).RequestHistory(context.Background(), "wf-none")
	require.NoError(t, err)
	require.Nil(t, requestHistoryNil)
	lineageNil, err := (*Service)(nil).PlanLineage(context.Background(), "wf-none")
	require.NoError(t, err)
	require.Nil(t, lineageNil)
	explorationActivityNil, err := (*Service)(nil).ExplorationActivity(context.Background(), "wf-none")
	require.NoError(t, err)
	require.Nil(t, explorationActivityNil)
	explorationNil, err := (*Service)(nil).Exploration(context.Background(), "wf-none")
	require.NoError(t, err)
	require.Nil(t, explorationNil)
	learningQueueNil, err := (*Service)(nil).LearningQueue(context.Background(), "wf-none")
	require.NoError(t, err)
	require.Nil(t, learningQueueNil)
	activePlanNil, err := (*Service)(nil).ActivePlan(context.Background(), "wf-none")
	require.NoError(t, err)
	require.Nil(t, activePlanNil)
	timelineProjectionNil, err := (*Service)(nil).TimelineProjection(context.Background(), "wf-none")
	require.NoError(t, err)
	require.Nil(t, timelineProjectionNil)
	timelineNil, err := (*Service)(nil).Timeline(context.Background(), "wf-none")
	require.NoError(t, err)
	require.Nil(t, timelineNil)
	provenanceNil, err := (*Service)(nil).Provenance(context.Background(), "wf-none")
	require.NoError(t, err)
	require.Nil(t, provenanceNil)
	deferredDraftsNil, err := (*Service)(nil).DeferredDrafts(context.Background(), "ws-none")
	require.NoError(t, err)
	require.Nil(t, deferredDraftsNil)
	convergenceHistoryNil, err := (*Service)(nil).ConvergenceHistory(context.Background(), "ws-none")
	require.NoError(t, err)
	require.Nil(t, convergenceHistoryNil)
	decisionTrailNil, err := (*Service)(nil).DecisionTrail(context.Background(), "ws-none")
	require.NoError(t, err)
	require.Nil(t, decisionTrailNil)
	coherenceNil, err := (*Service)(nil).Coherence(context.Background(), "wf-none")
	require.NoError(t, err)
	require.Nil(t, coherenceNil)

	workflow, err := svc.Workflow(context.Background(), "wf-none")
	require.NoError(t, err)
	require.Nil(t, workflow)
	activePlan, err := svc.ActivePlan(context.Background(), "wf-none")
	require.NoError(t, err)
	require.Nil(t, activePlan)
	timeline, err := svc.Timeline(context.Background(), "wf-none")
	require.NoError(t, err)
	require.Nil(t, timeline)
	mutationHistory, err := svc.MutationHistory(context.Background(), "wf-none")
	require.NoError(t, err)
	require.Nil(t, mutationHistory)
	requestHistory, err := svc.RequestHistory(context.Background(), "wf-none")
	require.NoError(t, err)
	require.Nil(t, requestHistory)
	lineage, err := svc.PlanLineage(context.Background(), "wf-none")
	require.NoError(t, err)
	require.Nil(t, lineage)
	explorationActivity, err := svc.ExplorationActivity(context.Background(), "wf-none")
	require.NoError(t, err)
	require.Nil(t, explorationActivity)
	provenance, err := svc.Provenance(context.Background(), "wf-none")
	require.NoError(t, err)
	require.Nil(t, provenance)
	deferredDrafts, err := svc.DeferredDrafts(context.Background(), "ws-none")
	require.NoError(t, err)
	require.Nil(t, deferredDrafts)
	convergenceHistory, err := svc.ConvergenceHistory(context.Background(), "ws-none")
	require.NoError(t, err)
	require.Nil(t, convergenceHistory)
	decisionTrail, err := svc.DecisionTrail(context.Background(), "ws-none")
	require.NoError(t, err)
	require.Nil(t, decisionTrail)
	coherence, err := svc.Coherence(context.Background(), "wf-none")
	require.NoError(t, err)
	require.Nil(t, coherence)

	var runnerSvc Service
	require.NoError(t, runnerSvc.runMaterializer(context.Background(), "", "snapshot", &MutationHistoryMaterializer{}))
}

func TestProjectionCoverageEdges(t *testing.T) {
	ctx := context.Background()
	store, err := memorydb.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	require.NoError(t, store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-edges",
		TaskID:      "task-wf-edges",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "projection edges",
		Status:      memory.WorkflowRunStatusRunning,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}))

	blankSvc := &Service{Store: store}
	builtWorkflow, err := blankSvc.buildWorkflow(ctx, "wf-edges")
	require.NoError(t, err)
	require.NotNil(t, builtWorkflow)
	require.Equal(t, "wf-edges", builtWorkflow.WorkflowID)
	require.Nil(t, builtWorkflow.ActiveExploration)
	require.Nil(t, builtWorkflow.ActivePlanVersion)
	require.Nil(t, builtWorkflow.ConvergenceState)
	require.Empty(t, builtWorkflow.Timeline)
	workflow, err := blankSvc.Workflow(ctx, "")
	require.NoError(t, err)
	require.Nil(t, workflow)
	exploration, err := blankSvc.Exploration(ctx, "")
	require.NoError(t, err)
	require.Nil(t, exploration)
	learningQueue, err := blankSvc.LearningQueue(ctx, "")
	require.NoError(t, err)
	require.Nil(t, learningQueue)
	activePlan, err := blankSvc.ActivePlan(ctx, "")
	require.NoError(t, err)
	require.Nil(t, activePlan)
	timelineProjection, err := blankSvc.TimelineProjection(ctx, "")
	require.NoError(t, err)
	require.Nil(t, timelineProjection)

	timelineMat := &TimelineMaterializer{Store: store, WorkflowID: "wf-edges"}
	timeline, seq, err := timelineMat.build(ctx)
	require.NoError(t, err)
	require.Empty(t, timeline)
	require.Zero(t, seq)
	timelineOnly, err := timelineMat.buildTimeline(ctx)
	require.NoError(t, err)
	require.Empty(t, timelineOnly)
	require.NoError(t, timelineMat.Apply(ctx, nil))
	require.NotNil(t, timelineMat.Projection)
	timelineMat.Projection = &TimelineProjection{WorkflowID: "wf-edges", Timeline: []archaeodomain.TimelineEvent{{Seq: 1}}, LastEventSeq: 1}
	require.NoError(t, timelineMat.Apply(ctx, []core.FrameworkEvent{}))
	require.NoError(t, timelineMat.Apply(ctx, []core.FrameworkEvent{{Type: "irrelevant", Seq: 2}}))
	timelineMat.Projection.LastEventSeq = 10
	require.NoError(t, timelineMat.Apply(ctx, []core.FrameworkEvent{{Type: "irrelevant", Seq: 2}}))
	require.Equal(t, uint64(10), timelineMat.Projection.LastEventSeq)

	require.NoError(t, (&TimelineMaterializer{}).Restore(ctx, nil))
	require.NoError(t, (&MutationHistoryMaterializer{}).Restore(ctx, nil))
	require.NoError(t, (&RequestHistoryMaterializer{}).Restore(ctx, nil))
	require.NoError(t, (&PlanLineageMaterializer{}).Restore(ctx, nil))
	require.NoError(t, (&ExplorationActivityMaterializer{}).Restore(ctx, nil))
	require.NoError(t, (&ProvenanceMaterializer{}).Restore(ctx, nil))
	require.NoError(t, (&CoherenceMaterializer{}).Restore(ctx, nil))
	require.NoError(t, (&CompositeMaterializer{}).Restore(ctx, nil))

	require.NoError(t, (&MutationHistoryMaterializer{WorkflowID: "wf-edges"}).Apply(ctx, []core.FrameworkEvent{{Type: "irrelevant"}}))
	require.NoError(t, (&RequestHistoryMaterializer{WorkflowID: "wf-edges"}).Apply(ctx, []core.FrameworkEvent{{Type: "irrelevant"}}))
	require.NoError(t, (&PlanLineageMaterializer{WorkflowID: "wf-edges"}).Apply(ctx, []core.FrameworkEvent{{Type: "irrelevant"}}))
	require.NoError(t, (&ExplorationActivityMaterializer{WorkflowID: "wf-edges"}).Apply(ctx, []core.FrameworkEvent{{Type: "irrelevant"}}))
	require.NoError(t, (&ProvenanceMaterializer{WorkflowID: "wf-edges"}).Apply(ctx, []core.FrameworkEvent{{Type: "irrelevant"}}))
	require.NoError(t, (&CoherenceMaterializer{WorkflowID: "wf-edges"}).Apply(ctx, []core.FrameworkEvent{{Type: "irrelevant"}}))
	require.NoError(t, (&TimelineMaterializer{WorkflowID: "wf-edges"}).Apply(ctx, []core.FrameworkEvent{{Type: "irrelevant"}}))

	mutationProj, err := buildMutationHistoryProjection(ctx, nil, "wf-edges")
	require.NoError(t, err)
	require.Nil(t, mutationProj)
	requestProj, err := buildRequestHistoryProjection(ctx, nil, "wf-edges")
	require.NoError(t, err)
	require.Nil(t, requestProj)
	planLineageProj, err := buildPlanLineageProjection(ctx, nil, "wf-edges")
	require.NoError(t, err)
	require.Nil(t, planLineageProj)
	explorationActivityProj, err := buildExplorationActivityProjection(ctx, nil, "wf-edges")
	require.NoError(t, err)
	require.Nil(t, explorationActivityProj)
	provenanceProj, err := buildProvenanceProjection(ctx, nil, "wf-edges")
	require.NoError(t, err)
	require.Nil(t, provenanceProj)
	deferredProj, err := buildDeferredDraftProjection(ctx, nil, "ws-edges")
	require.NoError(t, err)
	require.NotNil(t, deferredProj)
	require.Empty(t, deferredProj.Records)
	decisionProj, err := buildDecisionTrailProjection(ctx, nil, "ws-edges")
	require.NoError(t, err)
	require.NotNil(t, decisionProj)
	require.Empty(t, decisionProj.Records)
	coherenceProj, err := buildCoherenceProjection(ctx, nil, "wf-edges")
	require.NoError(t, err)
	require.Nil(t, coherenceProj)

	coherenceEmpty, err := buildCoherenceProjection(ctx, store, "wf-edges")
	require.NoError(t, err)
	require.NotNil(t, coherenceEmpty)
	require.Equal(t, "wf-edges", coherenceEmpty.WorkflowID)
	require.Equal(t, 0, coherenceEmpty.BlockingLearningCount)
	require.Equal(t, 0, coherenceEmpty.BlockingMutationCount)

	planLineageEmpty, err := buildPlanLineageProjection(ctx, store, "wf-edges")
	require.NoError(t, err)
	require.NotNil(t, planLineageEmpty)
	require.Equal(t, "wf-edges", planLineageEmpty.WorkflowID)
	require.Empty(t, planLineageEmpty.Versions)
	require.Empty(t, planLineageEmpty.DraftVersions)

	requestHistoryEmpty, err := buildRequestHistoryProjection(ctx, store, "wf-edges")
	require.NoError(t, err)
	require.NotNil(t, requestHistoryEmpty)
	require.Equal(t, 0, requestHistoryEmpty.Pending)
	require.Equal(t, 0, requestHistoryEmpty.Running)
	require.Equal(t, 0, requestHistoryEmpty.Completed)
	require.Equal(t, 0, requestHistoryEmpty.Failed)
	require.Equal(t, 0, requestHistoryEmpty.Canceled)

	explorationActivityEmpty, err := buildExplorationActivityProjection(ctx, store, "wf-edges")
	require.NoError(t, err)
	require.NotNil(t, explorationActivityEmpty)
	require.Equal(t, "wf-edges", explorationActivityEmpty.WorkflowID)
	require.Empty(t, explorationActivityEmpty.ActivityTimeline)

	provenanceEmpty, err := buildProvenanceProjection(ctx, store, "wf-edges")
	require.NoError(t, err)
	require.NotNil(t, provenanceEmpty)
	require.Equal(t, "wf-edges", provenanceEmpty.WorkflowID)
	require.Empty(t, provenanceEmpty.Mutations)

	invalidatedRequest := archaeodomain.RequestRecord{
		ID:          "req-invalidated",
		WorkflowID:  "wf-edges",
		Kind:        archaeodomain.RequestPatternSurfacing,
		Status:      archaeodomain.RequestStatusInvalidated,
		Title:       "invalidated",
		RequestedAt: time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	invalidatedRaw, err := json.Marshal(invalidatedRequest)
	require.NoError(t, err)
	require.NoError(t, store.UpsertWorkflowArtifact(ctx, memory.WorkflowArtifactRecord{
		ArtifactID:    invalidatedRequest.ID,
		WorkflowID:    invalidatedRequest.WorkflowID,
		Kind:          "archaeo_request",
		ContentType:   "application/json",
		StorageKind:   memory.ArtifactStorageInline,
		InlineRawText: string(invalidatedRaw),
		CreatedAt:     invalidatedRequest.UpdatedAt,
	}))
	supersededRequest := archaeodomain.RequestRecord{
		ID:          "req-superseded",
		WorkflowID:  "wf-edges",
		Kind:        archaeodomain.RequestPatternSurfacing,
		Status:      archaeodomain.RequestStatusSuperseded,
		Title:       "superseded",
		RequestedAt: time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	supersededRaw, err := json.Marshal(supersededRequest)
	require.NoError(t, err)
	require.NoError(t, store.UpsertWorkflowArtifact(ctx, memory.WorkflowArtifactRecord{
		ArtifactID:    supersededRequest.ID,
		WorkflowID:    supersededRequest.WorkflowID,
		Kind:          "archaeo_request",
		ContentType:   "application/json",
		StorageKind:   memory.ArtifactStorageInline,
		InlineRawText: string(supersededRaw),
		CreatedAt:     supersededRequest.UpdatedAt,
	}))
	requestHistoryManual, err := buildRequestHistoryProjection(ctx, store, "wf-edges")
	require.NoError(t, err)
	require.NotNil(t, requestHistoryManual)
	require.Equal(t, 2, requestHistoryManual.Canceled)

	require.Equal(t, map[string]any{}, cloneMap(nil))
	require.Equal(t, 0, intValue(json.Number("bad")))
	require.Equal(t, uint64(10), uint64Value(uint32(10)))
	require.Equal(t, uint64(11), uint64Value(uint(11)))
	require.Equal(t, uint64(0), uint64Value("bad"))
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	return raw
}

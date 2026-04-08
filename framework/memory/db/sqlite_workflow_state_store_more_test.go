package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/stretchr/testify/require"
)

func TestSQLiteWorkflowStateStoreEndToEndProjection(t *testing.T) {
	ctx := context.Background()
	store, err := NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	now := time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC)
	require.Equal(t, store.DB(), store.DB())
	require.NotNil(t, store.RetrievalService())
	require.NoError(t, store.EnsureRetrievalSchema(ctx))
	schemaVersion, err := store.SchemaVersion(ctx)
	require.NoError(t, err)
	require.Equal(t, WorkflowStateSchemaVersion, schemaVersion)

	require.Error(t, store.CreateWorkflow(ctx, memory.WorkflowRecord{}))
	require.NoError(t, store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-1",
		TaskID:      "task-1",
		TaskType:    core.TaskTypePlanning,
		Instruction: "plan it",
		Metadata:    map[string]any{"owner": "alice"},
		Status:      memory.WorkflowRunStatusPending,
		CreatedAt:   now,
		UpdatedAt:   now,
	}))
	_, err = store.UpdateWorkflowStatus(ctx, "wf-1", 0, memory.WorkflowRunStatusRunning, "")
	require.NoError(t, err)
	wf, ok, err := store.GetWorkflow(ctx, "wf-1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "wf-1", wf.WorkflowID)
	require.Equal(t, map[string]any{"owner": "alice"}, wf.Metadata)

	require.NoError(t, store.CreateRun(ctx, memory.WorkflowRunRecord{
		RunID:      "run-1",
		WorkflowID: "wf-1",
		Status:     memory.WorkflowRunStatusRunning,
		Metadata:   map[string]any{"attempt": 1},
		StartedAt:  now,
	}))
	run, ok, err := store.GetRun(ctx, "run-1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "run-1", run.RunID)
	require.Equal(t, map[string]any{"attempt": float64(1)}, run.Metadata)
	require.NoError(t, store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-2",
		TaskID:      "task-2",
		TaskType:    core.TaskTypePlanning,
		Instruction: "plan more",
		Status:      memory.WorkflowRunStatusPending,
		CreatedAt:   now,
		UpdatedAt:   now,
	}))
	_, err = store.UpdateWorkflowStatus(ctx, "wf-2", 0, memory.WorkflowRunStatusFailed, "")
	require.NoError(t, err)
	require.NoError(t, store.CreateRun(ctx, memory.WorkflowRunRecord{
		RunID:      "run-2",
		WorkflowID: "wf-2",
		Status:     memory.WorkflowRunStatusFailed,
		AgentMode:  "recover",
		StartedAt:  now,
	}))
	counts, err := store.AggregateWorkflowStatusCounts(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, counts[memory.WorkflowRunStatusRunning])
	require.Equal(t, 1, counts[memory.WorkflowRunStatusFailed])
	runsByStatus, err := store.ListRunsByStatus(ctx, []memory.WorkflowRunStatus{memory.WorkflowRunStatusRunning, memory.WorkflowRunStatusFailed}, 10)
	require.NoError(t, err)
	require.Len(t, runsByStatus, 2)

	plan := memory.WorkflowPlanRecord{
		PlanID:     "plan-1",
		WorkflowID: "wf-1",
		RunID:      "run-1",
		IsActive:   true,
		CreatedAt:  now,
		Plan: core.Plan{
			Goal: "ship",
			Steps: []core.PlanStep{
				{ID: "step-1", Description: "start"},
				{ID: "step-2", Description: "finish"},
			},
			Dependencies: map[string][]string{
				"step-2": {"step-1"},
			},
			Files: []string{"main.go"},
		},
	}
	require.NoError(t, store.SavePlan(ctx, plan))
	activePlan, ok, err := store.GetActivePlan(ctx, "wf-1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "plan-1", activePlan.PlanID)
	require.Len(t, activePlan.Plan.Steps, 2)

	steps, err := store.ListSteps(ctx, "wf-1")
	require.NoError(t, err)
	require.Len(t, steps, 2)
	require.Equal(t, []string{"step-1"}, steps[1].Dependencies)
	ready, err := store.ListReadySteps(ctx, "wf-1")
	require.NoError(t, err)
	require.Len(t, ready, 1)
	require.Equal(t, "step-1", ready[0].StepID)
	require.NoError(t, store.UpdateStepStatus(ctx, "wf-1", "step-1", memory.StepStatusCompleted, "done"))
	ready, err = store.ListReadySteps(ctx, "wf-1")
	require.NoError(t, err)
	require.Len(t, ready, 1)
	require.Equal(t, "step-2", ready[0].StepID)

	version, err := store.UpdateWorkflowStatus(ctx, "wf-1", 0, memory.WorkflowRunStatusRunning, "step-1")
	require.NoError(t, err)
	require.Equal(t, int64(3), version)
	_, err = store.UpdateWorkflowStatus(ctx, "wf-1", 1, memory.WorkflowRunStatusCompleted, "step-2")
	require.ErrorIs(t, err, sql.ErrNoRows)

	require.NoError(t, store.CreateStepRun(ctx, memory.StepRunRecord{
		StepRunID:      "sr-1",
		WorkflowID:     "wf-1",
		RunID:          "run-1",
		StepID:         "step-1",
		Attempt:        1,
		Status:         memory.StepStatusCompleted,
		Summary:        "ok",
		ResultData:     map[string]any{"score": 1},
		VerificationOK: true,
		StartedAt:      now,
		FinishedAt:     &now,
	}))
	stepRuns, err := store.ListStepRuns(ctx, "wf-1", "step-1")
	require.NoError(t, err)
	require.Len(t, stepRuns, 1)
	require.Equal(t, "sr-1", stepRuns[0].StepRunID)

	require.NoError(t, store.UpsertArtifact(ctx, memory.StepArtifactRecord{
		ArtifactID:      "art-1",
		WorkflowID:      "wf-1",
		StepRunID:       "sr-1",
		Kind:            "log",
		ContentType:     "text/plain",
		StorageKind:     memory.ArtifactStorageInline,
		SummaryText:     "step artifact",
		SummaryMetadata: map[string]any{"workspace_id": "ws-1"},
		InlineRawText:   "payload",
		CreatedAt:       now,
	}))
	artifacts, err := store.ListArtifacts(ctx, "wf-1", "sr-1")
	require.NoError(t, err)
	require.Len(t, artifacts, 1)
	require.Equal(t, "art-1", artifacts[0].ArtifactID)

	require.NoError(t, store.UpsertWorkflowArtifact(ctx, memory.WorkflowArtifactRecord{
		ArtifactID:      "wart-1",
		WorkflowID:      "wf-1",
		RunID:           "run-1",
		Kind:            "plan",
		ContentType:     "application/json",
		StorageKind:     memory.ArtifactStorageInline,
		SummaryText:     "workflow artifact",
		SummaryMetadata: map[string]any{"workspace_id": "ws-1"},
		InlineRawText:   "{}",
		CreatedAt:       now,
	}))
	require.NoError(t, store.UpsertWorkflowArtifact(ctx, memory.WorkflowArtifactRecord{
		ArtifactID:      "wart-2",
		WorkflowID:      "wf-1",
		RunID:           "run-1",
		Kind:            "plan",
		ContentType:     "application/json",
		StorageKind:     memory.ArtifactStorageInline,
		SummaryText:     "workflow artifact 2",
		SummaryMetadata: map[string]any{"workspace_id": "ws-1"},
		InlineRawText:   "{}",
		CreatedAt:       now.Add(time.Second),
	}))
	workflowArtifacts, err := store.ListWorkflowArtifacts(ctx, "wf-1", "run-1")
	require.NoError(t, err)
	require.Len(t, workflowArtifacts, 2)
	byKind, err := store.ListWorkflowArtifactsByKind(ctx, "wf-1", "run-1", "plan")
	require.NoError(t, err)
	require.Len(t, byKind, 2)
	byWS, err := store.ListWorkflowArtifactsByKindAndWorkspace(ctx, "wf-1", "run-1", "plan", "ws-1")
	require.NoError(t, err)
	require.Len(t, byWS, 2)
	latest, ok, err := store.LatestWorkflowArtifactByKind(ctx, "wf-1", "run-1", "plan")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "wart-2", latest.ArtifactID)
	latest, ok, err = store.LatestWorkflowArtifactByKindAndWorkspace(ctx, "wf-1", "run-1", "plan", "ws-1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "wart-2", latest.ArtifactID)
	byID, ok, err := store.WorkflowArtifactByID(ctx, "wart-1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "wart-1", byID.ArtifactID)

	require.NoError(t, store.SaveStageResult(ctx, memory.WorkflowStageResultRecord{
		ResultID:       "res-1",
		WorkflowID:     "wf-1",
		RunID:          "run-1",
		StageName:      "stage-a",
		StageIndex:     0,
		ResponseJSON:   `{"ok":true}`,
		DecodedOutput:  map[string]any{"ok": true},
		ValidationOK:   true,
		TransitionKind: "next",
		NextStage:      "stage-b",
		StartedAt:      now,
		FinishedAt:     now,
	}))
	stageResults, err := store.ListStageResults(ctx, "wf-1", "run-1")
	require.NoError(t, err)
	require.Len(t, stageResults, 1)
	latestStage, ok, err := store.GetLatestValidStageResult(ctx, "wf-1", "run-1", "stage-a")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "res-1", latestStage.ResultID)

	require.NoError(t, store.SavePipelineCheckpoint(ctx, memory.PipelineCheckpointRecord{
		CheckpointID: "chk-1",
		TaskID:       "task-1",
		WorkflowID:   "wf-1",
		RunID:        "run-1",
		StageName:    "stage-a",
		StageIndex:   0,
		ContextJSON:  `{"step":"a"}`,
		ResultJSON:   `{"ok":true}`,
		CreatedAt:    now,
	}))
	checkpoint, ok, err := store.LoadPipelineCheckpoint(ctx, "task-1", "chk-1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "chk-1", checkpoint.CheckpointID)
	ids, err := store.ListPipelineCheckpoints(ctx, "task-1")
	require.NoError(t, err)
	require.Equal(t, []string{"chk-1"}, ids)

	require.NoError(t, store.PutKnowledge(ctx, memory.KnowledgeRecord{
		RecordID:   "kn-1",
		WorkflowID: "wf-1",
		StepID:     "step-1",
		Kind:       memory.KnowledgeKindFact,
		Title:      "fact one",
		Content:    "content",
		Metadata:   map[string]any{"source": "test"},
		CreatedAt:  now,
	}))
	knowledge, err := store.ListKnowledge(ctx, "wf-1", memory.KnowledgeKindFact, false)
	require.NoError(t, err)
	require.Len(t, knowledge, 1)
	require.Equal(t, "kn-1", knowledge[0].RecordID)

	require.NoError(t, store.AppendEvent(ctx, memory.WorkflowEventRecord{
		EventID:    "ev-1",
		WorkflowID: "wf-1",
		RunID:      "run-1",
		StepID:     "step-1",
		EventType:  "note",
		Message:    "hello",
		Metadata:   map[string]any{"source": "test"},
		CreatedAt:  now,
	}))
	require.NoError(t, store.AppendEvent(ctx, memory.WorkflowEventRecord{
		EventID:    "ev-2",
		WorkflowID: "wf-1",
		RunID:      "run-1",
		StepID:     "step-2",
		EventType:  "state",
		Message:    "world",
		Metadata:   map[string]any{"source": "test"},
		CreatedAt:  now.Add(time.Second),
	}))
	events, err := store.ListEvents(ctx, "wf-1", 10)
	require.NoError(t, err)
	require.Len(t, events, 2)
	latestEvent, ok, err := store.LatestEvent(ctx, "wf-1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "ev-2", latestEvent.EventID)
	latestEvent, ok, err = store.LatestEventByTypes(ctx, "wf-1", "state", "note")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "ev-2", latestEvent.EventID)

	require.NoError(t, store.ReplaceProviderSnapshots(ctx, "wf-1", "run-1", []memory.WorkflowProviderSnapshotRecord{{
		SnapshotID:     "prov-1",
		WorkflowID:     "wf-1",
		RunID:          "run-1",
		ProviderID:     "provider-1",
		Recoverability: core.RecoverabilityInProcess,
		Descriptor:     core.ProviderDescriptor{},
		Health:         core.ProviderHealthSnapshot{Status: "ok"},
		CapabilityIDs:  []string{"cap-1"},
		TaskID:         "task-1",
		Metadata:       map[string]any{"source": "test"},
		State:          map[string]any{"running": true},
		CapturedAt:     now,
	}}))
	providers, err := store.ListProviderSnapshots(ctx, "wf-1", "run-1")
	require.NoError(t, err)
	require.Len(t, providers, 1)
	require.Equal(t, "provider-1", providers[0].ProviderID)
	require.Equal(t, core.RecoverabilityInProcess, providers[0].Descriptor.RecoverabilityMode)

	require.NoError(t, store.ReplaceProviderSessionSnapshots(ctx, "wf-1", "run-1", []memory.WorkflowProviderSessionSnapshotRecord{{
		SnapshotID: "sess-1",
		WorkflowID: "wf-1",
		RunID:      "run-1",
		Session: core.ProviderSession{
			ID:             "session-1",
			ProviderID:     "provider-1",
			Recoverability: core.RecoverabilityInProcess,
			Health:         "healthy",
		},
		Metadata:   map[string]any{"source": "test"},
		State:      map[string]any{"ready": true},
		CapturedAt: now,
	}}))
	sessions, err := store.ListProviderSessionSnapshots(ctx, "wf-1", "run-1")
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	require.Equal(t, "session-1", sessions[0].Session.ID)

	delegation := memory.WorkflowDelegationRecord{
		DelegationID: "del-1",
		WorkflowID:   "wf-1",
		RunID:        "run-1",
		Request: core.DelegationRequest{
			ID:                 "del-1",
			WorkflowID:         "wf-1",
			TaskID:             "task-1",
			TargetCapabilityID: "cap-1",
			TaskType:           "plan",
			Instruction:        "do work",
		},
		State:          core.DelegationStateRunning,
		TrustClass:     core.TrustClassBuiltinTrusted,
		Recoverability: core.RecoverabilityInProcess,
		StartedAt:      now,
		UpdatedAt:      now,
	}
	require.NoError(t, store.UpsertDelegation(ctx, delegation))
	require.NoError(t, store.AppendDelegationTransition(ctx, memory.WorkflowDelegationTransitionRecord{
		TransitionID: "del-1:running",
		DelegationID: "del-1",
		WorkflowID:   "wf-1",
		RunID:        "run-1",
		FromState:    core.DelegationStatePending,
		ToState:      core.DelegationStateRunning,
		CreatedAt:    now,
	}))
	delegations, err := store.ListDelegations(ctx, "wf-1", "run-1")
	require.NoError(t, err)
	require.Len(t, delegations, 1)
	transitions, err := store.ListDelegationTransitions(ctx, "del-1")
	require.NoError(t, err)
	require.Len(t, transitions, 1)

	slice, ok, err := store.LoadStepSlice(ctx, "wf-1", "step-2", 2)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "step-2", slice.Step.StepID)
	require.Len(t, slice.DependencySteps, 1)
	require.Len(t, slice.DependencyRuns, 1)
	require.Len(t, slice.RecentEvents, 2)

	invalidations, err := store.InvalidateDependents(ctx, "wf-1", "step-1", "replan")
	require.NoError(t, err)
	require.Len(t, invalidations, 1)
	invalidated, err := store.ListInvalidations(ctx, "wf-1")
	require.NoError(t, err)
	require.Len(t, invalidated, 1)
	require.Equal(t, "step-2", invalidated[0].InvalidatedStepID)
}

func TestWorkflowStateStoreHelperFunctions(t *testing.T) {
	require.Equal(t, "{}", mustJSON(nil))
	require.Equal(t, "null", mustJSONAny(nil))
	require.Equal(t, map[string]any{}, decodeJSONMap(""))
	require.Nil(t, decodeJSONAny(""))
	require.Nil(t, decodeJSONStringSlice(""))
	require.Equal(t, 1, boolInt(true))
	require.Equal(t, 0, boolInt(false))
	require.True(t, parseTime("bad").IsZero())
	require.Equal(t, "workflow:wf", strings.TrimSpace(workflowRetrievalScope("wf")))
	require.Contains(t, workflowArtifactRetrievalURI("wf", "run", "art"), "workflow://artifact/wf/run/art")
	require.Contains(t, stepArtifactRetrievalURI("wf", "step", "art"), "workflow://step-artifact/wf/step/art")
	require.Contains(t, workflowKnowledgeRetrievalURI("wf", "kn"), "workflow://knowledge/wf/kn")
	require.Equal(t, []string{"a", "b"}, compactNonEmpty("a", "", "{}", "null", "b"))
	require.Equal(t, []memory.KnowledgeRecord{{StepID: "step-1"}}, filterKnowledgeBySteps([]memory.KnowledgeRecord{{StepID: "step-1"}, {StepID: "step-2"}}, []string{"step-1"}))
	require.Empty(t, filterKnowledgeBySteps([]memory.KnowledgeRecord{{StepID: "step-1"}}, nil))
	require.NotEmpty(t, newRecordID("id"))
}

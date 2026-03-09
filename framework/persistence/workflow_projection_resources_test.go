package persistence

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

func TestWorkflowResourceURIRoundTrip(t *testing.T) {
	ref := WorkflowResourceRef{
		WorkflowID: "wf-1",
		RunID:      "run-1",
		StepID:     "step-2",
		Tier:       WorkflowProjectionTierWarm,
		Role:       core.CoordinationRoleArchitect,
	}

	uri := BuildWorkflowResourceURI(ref)
	parsed, err := ParseWorkflowResourceURI(uri)

	require.NoError(t, err)
	require.Equal(t, ref, parsed)
}

func TestWorkflowProjectionServiceProjectsHotAndColdViews(t *testing.T) {
	store := newProjectionFixtureStore(t)
	defer store.Close()

	service := WorkflowProjectionService{Store: store}

	hot, err := service.Project(context.Background(), WorkflowResourceRef{
		WorkflowID: "wf-proj",
		RunID:      "run-proj",
		StepID:     "step-b",
		Tier:       WorkflowProjectionTierHot,
		Role:       core.CoordinationRoleArchitect,
	})
	require.NoError(t, err)
	require.Len(t, hot.Contents, 1)
	hotPayload := hot.Contents[0].(core.StructuredContentBlock).Data.(map[string]any)
	require.Equal(t, "hot", hotPayload["tier"])
	step, ok := hotPayload["step"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "step-b", step["step_id"])
	require.NotEmpty(t, step["artifacts"])
	require.Nil(t, hotPayload["workflow_artifacts"])

	cold, err := service.Project(context.Background(), WorkflowResourceRef{
		WorkflowID: "wf-proj",
		RunID:      "run-proj",
		Tier:       WorkflowProjectionTierCold,
		Role:       core.CoordinationRolePlanner,
	})
	require.NoError(t, err)
	coldPayload := cold.Contents[0].(core.StructuredContentBlock).Data.(map[string]any)
	require.Equal(t, "cold", coldPayload["tier"])
	require.NotEmpty(t, coldPayload["workflow_artifacts"])
	require.NotEmpty(t, coldPayload["provider_snapshots"])
	require.NotEmpty(t, coldPayload["provider_sessions"])
	require.NotEmpty(t, coldPayload["events"])
}

func TestWorkflowResourceCapabilityReadsDirectly(t *testing.T) {
	store := newProjectionFixtureStore(t)
	defer store.Close()

	handler, err := NewWorkflowResourceCapability(store, WorkflowResourceRef{
		WorkflowID: "wf-proj",
		RunID:      "run-proj",
		StepID:     "step-b",
		Tier:       WorkflowProjectionTierWarm,
		Role:       core.CoordinationRoleReviewer,
	})
	require.NoError(t, err)

	desc := handler.Descriptor(context.Background(), core.NewContext())
	resource, err := handler.ReadResource(context.Background(), core.NewContext())
	require.NoError(t, err)
	require.Equal(t, BuildWorkflowResourceURI(handler.ref), resource.Metadata["workflow_uri"])
	require.Equal(t, desc.Annotations["workflow_uri"], resource.Metadata["workflow_uri"])
	payload := resource.Contents[0].(core.StructuredContentBlock).Data.(map[string]any)
	require.Equal(t, "warm", payload["tier"])
}

func newProjectionFixtureStore(t *testing.T) *SQLiteWorkflowStateStore {
	t.Helper()
	store, err := NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow_state.db"))
	require.NoError(t, err)

	ctx := context.Background()
	now := time.Now().UTC()
	finished := now.Add(-time.Minute)

	require.NoError(t, store.CreateWorkflow(ctx, WorkflowRecord{
		WorkflowID:  "wf-proj",
		TaskID:      "task-proj",
		TaskType:    core.TaskTypeCodeModification,
		Instruction: "Project workflow state",
		Status:      WorkflowRunStatusRunning,
	}))
	require.NoError(t, store.CreateRun(ctx, WorkflowRunRecord{
		RunID:      "run-proj",
		WorkflowID: "wf-proj",
		Status:     WorkflowRunStatusRunning,
		AgentName:  "architect",
		AgentMode:  "primary",
		StartedAt:  finished,
	}))
	require.NoError(t, store.SavePlan(ctx, WorkflowPlanRecord{
		PlanID:     "plan-proj",
		WorkflowID: "wf-proj",
		RunID:      "run-proj",
		Plan: core.Plan{
			Goal: "Ship projection resources",
			Steps: []core.PlanStep{
				{ID: "step-a", Description: "prepare context"},
				{ID: "step-b", Description: "consume handoff"},
			},
			Dependencies: map[string][]string{
				"step-b": {"step-a"},
			},
		},
		IsActive: true,
	}))
	require.NoError(t, store.CreateStepRun(ctx, StepRunRecord{
		StepRunID:      "step-run-a1",
		WorkflowID:     "wf-proj",
		RunID:          "run-proj",
		StepID:         "step-a",
		Attempt:        1,
		Status:         StepStatusCompleted,
		Summary:        "prepared handoff",
		ResultData:     map[string]any{"summary": "prepared handoff"},
		VerificationOK: true,
		StartedAt:      finished.Add(-time.Minute),
		FinishedAt:     &finished,
	}))
	require.NoError(t, store.UpdateStepStatus(ctx, "wf-proj", "step-a", StepStatusCompleted, "prepared handoff"))
	require.NoError(t, store.UpsertArtifact(ctx, StepArtifactRecord{
		ArtifactID:      "artifact-step-a1",
		WorkflowID:      "wf-proj",
		StepRunID:       "step-run-a1",
		Kind:            "handoff",
		ContentType:     "application/json",
		StorageKind:     ArtifactStorageInline,
		SummaryText:     "handoff summary",
		SummaryMetadata: map[string]any{"phase": "handoff"},
		InlineRawText:   `{"notes":"keep comments"}`,
		RawSizeBytes:    int64(len(`{"notes":"keep comments"}`)),
	}))
	require.NoError(t, store.UpsertWorkflowArtifact(ctx, WorkflowArtifactRecord{
		ArtifactID:      "workflow-artifact-1",
		WorkflowID:      "wf-proj",
		RunID:           "run-proj",
		Kind:            "planner_output",
		ContentType:     "application/json",
		StorageKind:     ArtifactStorageInline,
		SummaryText:     "planner summary",
		SummaryMetadata: map[string]any{"source": "planner"},
		InlineRawText:   `{"goal":"Ship projection resources"}`,
		RawSizeBytes:    int64(len(`{"goal":"Ship projection resources"}`)),
	}))
	require.NoError(t, store.PutKnowledge(ctx, KnowledgeRecord{
		RecordID:   "issue-1",
		WorkflowID: "wf-proj",
		StepRunID:  "step-run-a1",
		StepID:     "step-a",
		Kind:       KnowledgeKindIssue,
		Title:      "Preserve comments",
		Content:    "Generated edits must preserve comments.",
		Status:     "open",
	}))
	require.NoError(t, store.PutKnowledge(ctx, KnowledgeRecord{
		RecordID:   "decision-1",
		WorkflowID: "wf-proj",
		StepRunID:  "step-run-a1",
		StepID:     "step-a",
		Kind:       KnowledgeKindDecision,
		Title:      "Patch narrowly",
		Content:    "Use minimal patches for edits.",
	}))
	require.NoError(t, store.AppendEvent(ctx, WorkflowEventRecord{
		EventID:    "event-1",
		WorkflowID: "wf-proj",
		RunID:      "run-proj",
		StepID:     "step-a",
		EventType:  "step_completed",
		Message:    "step a completed",
		CreatedAt:  now,
	}))
	require.NoError(t, store.SaveStageResult(ctx, WorkflowStageResultRecord{
		ResultID:        "stage-1",
		WorkflowID:      "wf-proj",
		RunID:           "run-proj",
		StageName:       "review",
		StageIndex:      1,
		ContractName:    "review-report",
		ContractVersion: "v1",
		DecodedOutput:   map[string]any{"status": "ok"},
		ValidationOK:    true,
		StartedAt:       now,
		FinishedAt:      now,
	}))
	require.NoError(t, store.ReplaceProviderSnapshots(ctx, "wf-proj", "run-proj", []WorkflowProviderSnapshotRecord{{
		SnapshotID:     "provider-1",
		WorkflowID:     "wf-proj",
		RunID:          "run-proj",
		ProviderID:     "browser",
		Recoverability: core.RecoverabilityInProcess,
		Descriptor:     core.ProviderDescriptor{ID: "browser", Kind: core.ProviderKindAgentRuntime},
		Health:         core.ProviderHealthSnapshot{Status: "ok"},
		CapabilityIDs:  []string{"tool:browser.open"},
		Metadata:       map[string]any{"target": "docs"},
	}}))
	require.NoError(t, store.ReplaceProviderSessionSnapshots(ctx, "wf-proj", "run-proj", []WorkflowProviderSessionSnapshotRecord{{
		SnapshotID: "session-1",
		WorkflowID: "wf-proj",
		RunID:      "run-proj",
		Session: core.ProviderSession{
			ID:         "session-1",
			ProviderID: "browser",
			Health:     "initialized",
		},
		Metadata: map[string]any{"page": "README.md"},
		State:    map[string]any{"active_tab": 1},
	}}))

	return store
}

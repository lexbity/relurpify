package memory_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/memory/db"
	"github.com/stretchr/testify/require"
)

func TestWorkflowResourceURIRoundTrip(t *testing.T) {
	ref := memory.WorkflowResourceRef{
		WorkflowID: "wf-1",
		RunID:      "run-1",
		StepID:     "step-2",
		Tier:       memory.WorkflowProjectionTierWarm,
		Role:       core.CoordinationRoleArchitect,
	}

	uri := memory.BuildWorkflowResourceURI(ref)
	parsed, err := memory.ParseWorkflowResourceURI(uri)

	require.NoError(t, err)
	require.Equal(t, ref, parsed)
}

func TestWorkflowProjectionServiceProjectsHotAndColdViews(t *testing.T) {
	store := newProjectionFixtureStore(t)
	defer store.Close()

	service := memory.WorkflowProjectionService{Store: store}

	hot, err := service.Project(context.Background(), memory.WorkflowResourceRef{
		WorkflowID: "wf-proj",
		RunID:      "run-proj",
		StepID:     "step-b",
		Tier:       memory.WorkflowProjectionTierHot,
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

	cold, err := service.Project(context.Background(), memory.WorkflowResourceRef{
		WorkflowID: "wf-proj",
		RunID:      "run-proj",
		Tier:       memory.WorkflowProjectionTierCold,
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

	handler, err := memory.NewWorkflowResourceCapability(store, memory.WorkflowResourceRef{
		WorkflowID: "wf-proj",
		RunID:      "run-proj",
		StepID:     "step-b",
		Tier:       memory.WorkflowProjectionTierWarm,
		Role:       core.CoordinationRoleReviewer,
	})
	require.NoError(t, err)

	desc := handler.Descriptor(context.Background(), core.NewContext())
	resource, err := handler.ReadResource(context.Background(), core.NewContext())
	require.NoError(t, err)
	require.Equal(t, desc.Annotations["workflow_uri"], resource.Metadata["workflow_uri"])
	require.Equal(t, desc.Annotations["workflow_uri"], resource.Metadata["workflow_uri"])
	payload := resource.Contents[0].(core.StructuredContentBlock).Data.(map[string]any)
	require.Equal(t, "warm", payload["tier"])
}

func newProjectionFixtureStore(t *testing.T) *db.SQLiteWorkflowStateStore {
	t.Helper()
	store, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow_state.db"))
	require.NoError(t, err)

	ctx := context.Background()
	now := time.Now().UTC()
	finished := now.Add(-time.Minute)

	require.NoError(t, store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-proj",
		TaskID:      "task-proj",
		TaskType:    core.TaskTypeCodeModification,
		Instruction: "Project workflow state",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	require.NoError(t, store.CreateRun(ctx, memory.WorkflowRunRecord{
		RunID:      "run-proj",
		WorkflowID: "wf-proj",
		Status:     memory.WorkflowRunStatusRunning,
		AgentName:  "architect",
		AgentMode:  "primary",
		StartedAt:  finished,
	}))
	require.NoError(t, store.SavePlan(ctx, memory.WorkflowPlanRecord{
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
	require.NoError(t, store.CreateStepRun(ctx, memory.StepRunRecord{
		StepRunID:      "step-run-a1",
		WorkflowID:     "wf-proj",
		RunID:          "run-proj",
		StepID:         "step-a",
		Attempt:        1,
		Status:         memory.StepStatusCompleted,
		Summary:        "prepared handoff",
		ResultData:     map[string]any{"summary": "prepared handoff"},
		VerificationOK: true,
		StartedAt:      finished.Add(-time.Minute),
		FinishedAt:     &finished,
	}))
	require.NoError(t, store.UpdateStepStatus(ctx, "wf-proj", "step-a", memory.StepStatusCompleted, "prepared handoff"))
	require.NoError(t, store.UpsertArtifact(ctx, memory.StepArtifactRecord{
		ArtifactID:      "artifact-step-a1",
		WorkflowID:      "wf-proj",
		StepRunID:       "step-run-a1",
		Kind:            "handoff",
		ContentType:     "application/json",
		StorageKind:     memory.ArtifactStorageInline,
		SummaryText:     "handoff summary",
		SummaryMetadata: map[string]any{"phase": "handoff"},
		InlineRawText:   `{"notes":"keep comments"}`,
		RawSizeBytes:    int64(len(`{"notes":"keep comments"}`)),
	}))
	require.NoError(t, store.UpsertWorkflowArtifact(ctx, memory.WorkflowArtifactRecord{
		ArtifactID:      "workflow-artifact-1",
		WorkflowID:      "wf-proj",
		RunID:           "run-proj",
		Kind:            "planner_output",
		ContentType:     "application/json",
		StorageKind:     memory.ArtifactStorageInline,
		SummaryText:     "planner summary",
		SummaryMetadata: map[string]any{"source": "planner"},
		InlineRawText:   `{"goal":"Ship projection resources"}`,
		RawSizeBytes:    int64(len(`{"goal":"Ship projection resources"}`)),
	}))
	require.NoError(t, store.PutKnowledge(ctx, memory.KnowledgeRecord{
		RecordID:   "issue-1",
		WorkflowID: "wf-proj",
		StepRunID:  "step-run-a1",
		StepID:     "step-a",
		Kind:       memory.KnowledgeKindIssue,
		Title:      "Preserve comments",
		Content:    "Generated edits must preserve comments.",
		Status:     "open",
	}))
	require.NoError(t, store.PutKnowledge(ctx, memory.KnowledgeRecord{
		RecordID:   "decision-1",
		WorkflowID: "wf-proj",
		StepRunID:  "step-run-a1",
		StepID:     "step-a",
		Kind:       memory.KnowledgeKindDecision,
		Title:      "Patch narrowly",
		Content:    "Use minimal patches for edits.",
	}))
	require.NoError(t, store.AppendEvent(ctx, memory.WorkflowEventRecord{
		EventID:    "event-1",
		WorkflowID: "wf-proj",
		RunID:      "run-proj",
		StepID:     "step-a",
		EventType:  "step_completed",
		Message:    "step a completed",
		CreatedAt:  now,
	}))
	require.NoError(t, store.SaveStageResult(ctx, memory.WorkflowStageResultRecord{
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
	require.NoError(t, store.ReplaceProviderSnapshots(ctx, "wf-proj", "run-proj", []memory.WorkflowProviderSnapshotRecord{{
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
	require.NoError(t, store.ReplaceProviderSessionSnapshots(ctx, "wf-proj", "run-proj", []memory.WorkflowProviderSessionSnapshotRecord{{
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

package rewoo

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	frameworkmemory "codeburg.org/lexbit/relurpify/framework/memory"
	"codeburg.org/lexbit/relurpify/framework/memory/db"
)

func TestRewooCheckpointStoreSavePrefersArtifactRefs(t *testing.T) {
	store := NewRewooCheckpointStore(nil, nil)
	state := core.NewContext()
	state.Set("rewoo.plan", &RewooPlan{Goal: "g"})
	state.Set("rewoo.tool_results_ref", core.ArtifactReference{
		ArtifactID: "results-1",
		WorkflowID: "wf-1",
		RunID:      "run-1",
		Kind:       "rewoo_tool_results",
		Summary:    "a [ok]",
	})
	state.Set("rewoo.synthesis_ref", core.ArtifactReference{
		ArtifactID: "synth-1",
		WorkflowID: "wf-1",
		RunID:      "run-1",
		Kind:       "rewoo_synthesis",
		Summary:    "final answer",
	})
	state.Set("rewoo.tool_results", []RewooStepResult{{StepID: "a", Success: true}})
	state.Set("rewoo.synthesis", "final answer")

	if err := store.SaveCheckpoint(context.Background(), "cp-1", "execute", 0, state); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}
	checkpoint, err := store.LoadCheckpoint(context.Background(), "cp-1")
	if err != nil {
		t.Fatalf("LoadCheckpoint: %v", err)
	}
	snapshot, ok := checkpoint.Metadata["state_snapshot"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected state snapshot, got %#v", checkpoint.Metadata)
	}
	if _, ok := snapshot["rewoo.tool_results_ref"]; !ok {
		t.Fatal("expected tool_results_ref in checkpoint snapshot")
	}
	if _, ok := snapshot["rewoo.synthesis_ref"]; !ok {
		t.Fatal("expected synthesis_ref in checkpoint snapshot")
	}
	if _, ok := snapshot["rewoo.tool_results"]; ok {
		t.Fatal("did not expect inline tool_results when artifact ref exists")
	}
	if _, ok := snapshot["rewoo.synthesis"]; ok {
		t.Fatal("did not expect inline synthesis when artifact ref exists")
	}
}

func TestRewooCheckpointStoreListDeleteAndArtifactLoadingErrors(t *testing.T) {
	workflowStore, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	if err != nil {
		t.Fatalf("workflow store: %v", err)
	}
	t.Cleanup(func() { _ = workflowStore.Close() })
	requireNoErrCheckpoint(t, workflowStore.CreateWorkflow(context.Background(), frameworkmemory.WorkflowRecord{
		WorkflowID:  "wf-checkpoints",
		TaskID:      "task-checkpoints",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "checkpoints",
		Status:      frameworkmemory.WorkflowRunStatusRunning,
	}))
	requireNoErrCheckpoint(t, workflowStore.CreateRun(context.Background(), frameworkmemory.WorkflowRunRecord{
		RunID:      "run-checkpoints",
		WorkflowID: "wf-checkpoints",
		Status:     frameworkmemory.WorkflowRunStatusRunning,
		StartedAt:  time.Now().UTC(),
	}))

	store := NewRewooCheckpointStore(workflowStore, nil)
	listed, err := store.ListCheckpoints(context.Background())
	if err != nil {
		t.Fatalf("ListCheckpoints: %v", err)
	}
	if len(listed) != 0 {
		t.Fatalf("expected empty checkpoint list, got %d", len(listed))
	}

	state := core.NewContext()
	state.Set("rewoo.workflow_id", "wf-checkpoints")
	state.Set("rewoo.run_id", "run-checkpoints")
	state.Set("rewoo.plan", &RewooPlan{Goal: "goal", Steps: []RewooStep{{ID: "a", Tool: "tool"}}})
	state.Set("rewoo.tool_results", []RewooStepResult{{StepID: "a", Tool: "tool", Success: true}})
	state.Set("rewoo.synthesis", "final")
	if err := store.SaveCheckpoint(context.Background(), "cp-checkpoints", "synthesis", 2, state); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}

	listed, err = store.ListCheckpoints(context.Background())
	if err != nil {
		t.Fatalf("ListCheckpoints after save: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("expected one checkpoint, got %d", len(listed))
	}
	if err := store.DeleteCheckpoint(context.Background(), "cp-checkpoints"); err != nil {
		t.Fatalf("DeleteCheckpoint: %v", err)
	}
	listed, err = store.ListCheckpoints(context.Background())
	if err != nil {
		t.Fatalf("ListCheckpoints after delete: %v", err)
	}
	if len(listed) != 0 {
		t.Fatalf("expected empty checkpoint list after delete, got %d", len(listed))
	}

	var target map[string]any
	if err := (&RewooCheckpointStore{}).loadWorkflowArtifactJSON(context.Background(), core.ArtifactReference{}, &target); err == nil {
		t.Fatal("expected error when workflow store is unavailable")
	}

	requireNoErrCheckpoint(t, workflowStore.UpsertWorkflowArtifact(context.Background(), frameworkmemory.WorkflowArtifactRecord{
		ArtifactID:        "empty-artifact",
		WorkflowID:        "wf-checkpoints",
		RunID:             "run-checkpoints",
		Kind:              "rewoo_plan",
		ContentType:       "application/json",
		StorageKind:       frameworkmemory.ArtifactStorageInline,
		SummaryText:       "empty",
		InlineRawText:     "",
		RawSizeBytes:      0,
		CompressionMethod: "none",
		CreatedAt:         time.Now().UTC(),
	}))
	err = store.loadWorkflowArtifactJSON(context.Background(), core.ArtifactReference{
		ArtifactID: "empty-artifact",
		WorkflowID: "wf-checkpoints",
		RunID:      "run-checkpoints",
	}, &target)
	if err == nil {
		t.Fatal("expected error for missing inline payload")
	}

	requireNoErrCheckpoint(t, workflowStore.UpsertWorkflowArtifact(context.Background(), frameworkmemory.WorkflowArtifactRecord{
		ArtifactID:        "bad-artifact",
		WorkflowID:        "wf-checkpoints",
		RunID:             "run-checkpoints",
		Kind:              "rewoo_plan",
		ContentType:       "application/json",
		StorageKind:       frameworkmemory.ArtifactStorageInline,
		SummaryText:       "bad",
		InlineRawText:     "{bad json",
		RawSizeBytes:      9,
		CompressionMethod: "none",
		CreatedAt:         time.Now().UTC(),
	}))
	err = store.loadWorkflowArtifactJSON(context.Background(), core.ArtifactReference{
		ArtifactID: "bad-artifact",
		WorkflowID: "wf-checkpoints",
		RunID:      "run-checkpoints",
	}, &target)
	if err == nil {
		t.Fatal("expected decode error for malformed JSON")
	}

	err = store.loadWorkflowArtifactJSON(context.Background(), core.ArtifactReference{
		ArtifactID: "missing-artifact",
		WorkflowID: "wf-checkpoints",
		RunID:      "run-checkpoints",
	}, &target)
	if err == nil {
		t.Fatal("expected not-found error for missing artifact")
	}
}

func TestRewooCheckpointStoreRestoreHydratesArtifactBackedState(t *testing.T) {
	workflowStore, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	if err != nil {
		t.Fatalf("workflow store: %v", err)
	}
	t.Cleanup(func() { _ = workflowStore.Close() })
	requireNoErrCheckpoint(t, workflowStore.CreateWorkflow(context.Background(), frameworkmemory.WorkflowRecord{
		WorkflowID:  "wf-restore",
		TaskID:      "task-restore",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "restore",
		Status:      frameworkmemory.WorkflowRunStatusRunning,
	}))
	requireNoErrCheckpoint(t, workflowStore.CreateRun(context.Background(), frameworkmemory.WorkflowRunRecord{
		RunID:      "run-restore",
		WorkflowID: "wf-restore",
		Status:     frameworkmemory.WorkflowRunStatusRunning,
		StartedAt:  time.Now().UTC(),
	}))
	requireNoErrCheckpoint(t, workflowStore.UpsertWorkflowArtifact(context.Background(), frameworkmemory.WorkflowArtifactRecord{
		ArtifactID:        "results-1",
		WorkflowID:        "wf-restore",
		RunID:             "run-restore",
		Kind:              "rewoo_tool_results",
		ContentType:       "application/json",
		StorageKind:       frameworkmemory.ArtifactStorageInline,
		SummaryText:       "a [ok]",
		InlineRawText:     `[{"step_id":"a","tool":"tool","success":true,"output":{"ok":true}}]`,
		RawSizeBytes:      64,
		CompressionMethod: "none",
		CreatedAt:         time.Now().UTC(),
	}))
	requireNoErrCheckpoint(t, workflowStore.UpsertWorkflowArtifact(context.Background(), frameworkmemory.WorkflowArtifactRecord{
		ArtifactID:        "synth-1",
		WorkflowID:        "wf-restore",
		RunID:             "run-restore",
		Kind:              "rewoo_synthesis",
		ContentType:       "application/json",
		StorageKind:       frameworkmemory.ArtifactStorageInline,
		SummaryText:       "final answer",
		InlineRawText:     `{"synthesis":"final answer"}`,
		RawSizeBytes:      32,
		CompressionMethod: "none",
		CreatedAt:         time.Now().UTC(),
	}))

	store := NewRewooCheckpointStore(workflowStore, nil)
	checkpoint := &CheckpointMetadata{
		CheckpointID: "cp-restore",
		Phase:        "synthesis",
		Attempt:      1,
		Metadata: map[string]interface{}{
			"state_snapshot": map[string]interface{}{
				"rewoo.tool_results_ref": core.ArtifactReference{
					ArtifactID: "results-1",
					WorkflowID: "wf-restore",
					RunID:      "run-restore",
					Kind:       "rewoo_tool_results",
					Summary:    "a [ok]",
				},
				"rewoo.synthesis_ref": core.ArtifactReference{
					ArtifactID: "synth-1",
					WorkflowID: "wf-restore",
					RunID:      "run-restore",
					Kind:       "rewoo_synthesis",
					Summary:    "final answer",
				},
			},
		},
	}

	state := core.NewContext()
	if err := store.RestoreStateFromCheckpoint(context.Background(), state, checkpoint); err != nil {
		t.Fatalf("RestoreStateFromCheckpoint: %v", err)
	}
	rawResults, ok := state.Get("rewoo.tool_results")
	if !ok {
		t.Fatal("expected rehydrated tool_results")
	}
	results, ok := rawResults.([]RewooStepResult)
	if !ok || len(results) != 1 || results[0].StepID != "a" || !results[0].Success {
		t.Fatalf("unexpected rehydrated tool results: %#v", rawResults)
	}
	if got := state.GetString("rewoo.synthesis"); got != "final answer" {
		t.Fatalf("unexpected rehydrated synthesis: %q", got)
	}
}

func TestRewooCheckpointStoreRestorePlanAndSynthesisSummaryFallback(t *testing.T) {
	workflowStore, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	if err != nil {
		t.Fatalf("workflow store: %v", err)
	}
	t.Cleanup(func() { _ = workflowStore.Close() })
	requireNoErrCheckpoint(t, workflowStore.CreateWorkflow(context.Background(), frameworkmemory.WorkflowRecord{
		WorkflowID:  "wf-plan",
		TaskID:      "task-plan",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "restore plan",
		Status:      frameworkmemory.WorkflowRunStatusRunning,
	}))
	requireNoErrCheckpoint(t, workflowStore.CreateRun(context.Background(), frameworkmemory.WorkflowRunRecord{
		RunID:      "run-plan",
		WorkflowID: "wf-plan",
		Status:     frameworkmemory.WorkflowRunStatusRunning,
		StartedAt:  time.Now().UTC(),
	}))
	requireNoErrCheckpoint(t, workflowStore.UpsertWorkflowArtifact(context.Background(), frameworkmemory.WorkflowArtifactRecord{
		ArtifactID:        "plan-1",
		WorkflowID:        "wf-plan",
		RunID:             "run-plan",
		Kind:              "rewoo_plan",
		ContentType:       "application/json",
		StorageKind:       frameworkmemory.ArtifactStorageInline,
		SummaryText:       "goal",
		InlineRawText:     `{"goal":"goal","steps":[{"id":"a","tool":"tool"}]}`,
		RawSizeBytes:      56,
		CompressionMethod: "none",
		CreatedAt:         time.Now().UTC(),
	}))
	requireNoErrCheckpoint(t, workflowStore.UpsertWorkflowArtifact(context.Background(), frameworkmemory.WorkflowArtifactRecord{
		ArtifactID:        "synth-1",
		WorkflowID:        "wf-plan",
		RunID:             "run-plan",
		Kind:              "rewoo_synthesis",
		ContentType:       "application/json",
		StorageKind:       frameworkmemory.ArtifactStorageInline,
		SummaryText:       "summary fallback",
		InlineRawText:     `{"synthesis":""}`,
		RawSizeBytes:      16,
		CompressionMethod: "none",
		CreatedAt:         time.Now().UTC(),
	}))

	store := NewRewooCheckpointStore(workflowStore, nil)
	checkpoint := &CheckpointMetadata{
		CheckpointID: "cp-plan",
		Phase:        "synthesis",
		Attempt:      1,
		Metadata: map[string]interface{}{
			"state_snapshot": map[string]interface{}{
				"rewoo.plan_ref": core.ArtifactReference{
					ArtifactID: "plan-1",
					WorkflowID: "wf-plan",
					RunID:      "run-plan",
					Kind:       "rewoo_plan",
					Summary:    "goal",
				},
				"rewoo.synthesis_ref": core.ArtifactReference{
					ArtifactID: "synth-1",
					WorkflowID: "wf-plan",
					RunID:      "run-plan",
					Kind:       "rewoo_synthesis",
					Summary:    "summary fallback",
				},
			},
		},
	}
	state := core.NewContext()
	if err := store.RestoreStateFromCheckpoint(context.Background(), state, checkpoint); err != nil {
		t.Fatalf("RestoreStateFromCheckpoint: %v", err)
	}
	if _, ok := state.Get("rewoo.plan"); !ok {
		t.Fatal("expected hydrated plan")
	}
	if got := state.GetString("rewoo.synthesis"); got != "summary fallback" {
		t.Fatalf("expected summary fallback, got %q", got)
	}
}

func TestRewooCheckpointStoreSaveMaterializesArtifactRefsFromInlineState(t *testing.T) {
	workflowStore, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	if err != nil {
		t.Fatalf("workflow store: %v", err)
	}
	t.Cleanup(func() { _ = workflowStore.Close() })
	requireNoErrCheckpoint(t, workflowStore.CreateWorkflow(context.Background(), frameworkmemory.WorkflowRecord{
		WorkflowID:  "wf-inline",
		TaskID:      "task-inline",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "inline",
		Status:      frameworkmemory.WorkflowRunStatusRunning,
	}))
	requireNoErrCheckpoint(t, workflowStore.CreateRun(context.Background(), frameworkmemory.WorkflowRunRecord{
		RunID:      "run-inline",
		WorkflowID: "wf-inline",
		Status:     frameworkmemory.WorkflowRunStatusRunning,
		StartedAt:  time.Now().UTC(),
	}))

	store := NewRewooCheckpointStore(workflowStore, nil)
	state := core.NewContext()
	state.Set("rewoo.workflow_id", "wf-inline")
	state.Set("rewoo.run_id", "run-inline")
	state.Set("rewoo.plan", &RewooPlan{Goal: "g", Steps: []RewooStep{{ID: "a", Tool: "tool"}}})
	state.Set("rewoo.tool_results", []RewooStepResult{{StepID: "a", Tool: "tool", Success: true}})
	state.Set("rewoo.synthesis", "final answer")

	if err := store.SaveCheckpoint(context.Background(), "cp-inline", "synthesis", 1, state); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}
	checkpoint, err := store.LoadCheckpoint(context.Background(), "cp-inline")
	if err != nil {
		t.Fatalf("LoadCheckpoint: %v", err)
	}
	snapshot, ok := checkpoint.Metadata["state_snapshot"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected state snapshot, got %#v", checkpoint.Metadata)
	}
	if _, ok := snapshot["rewoo.plan_ref"]; !ok {
		t.Fatal("expected plan_ref in checkpoint snapshot")
	}
	if _, ok := snapshot["rewoo.tool_results_ref"]; !ok {
		t.Fatal("expected tool_results_ref in checkpoint snapshot")
	}
	if _, ok := snapshot["rewoo.synthesis_ref"]; !ok {
		t.Fatal("expected synthesis_ref in checkpoint snapshot")
	}
	artifacts, err := workflowStore.ListWorkflowArtifacts(context.Background(), "wf-inline", "run-inline")
	if err != nil {
		t.Fatalf("ListWorkflowArtifacts: %v", err)
	}
	if len(artifacts) < 3 {
		t.Fatalf("expected checkpoint artifacts, got %d", len(artifacts))
	}
}

func requireNoErrCheckpoint(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

package checkpoint_test

import (
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/agents/chainer/checkpoint"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/pipeline"
)

func TestRecoveryManager_FindLastCheckpoint(t *testing.T) {
	store := checkpoint.NewStore()
	manager := checkpoint.NewRecoveryManager(store)

	// Save checkpoints
	cp1 := &pipeline.Checkpoint{
		CheckpointID: "cp_1",
		TaskID:       "task_1",
		StageName:    "stage_1",
		StageIndex:   0,
		CreatedAt:    time.Now(),
		Context:      core.NewContext(),
	}
	cp2 := &pipeline.Checkpoint{
		CheckpointID: "cp_2",
		TaskID:       "task_1",
		StageName:    "stage_2",
		StageIndex:   1,
		CreatedAt:    time.Now().Add(1 * time.Second),
		Context:      core.NewContext(),
	}

	if err := store.Save(cp1); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if err := store.Save(cp2); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Find last checkpoint
	latest, err := manager.FindLastCheckpoint("task_1")
	if err != nil {
		t.Fatalf("FindLastCheckpoint failed: %v", err)
	}

	if latest == nil {
		t.Fatal("expected latest checkpoint, got nil")
	}
	if latest.CheckpointID != "cp_2" {
		t.Errorf("expected cp_2, got %q", latest.CheckpointID)
	}
}

func TestRecoveryManager_FindLastCheckpointNotFound(t *testing.T) {
	store := checkpoint.NewStore()
	manager := checkpoint.NewRecoveryManager(store)

	latest, err := manager.FindLastCheckpoint("nonexistent_task")
	if err != nil {
		t.Fatalf("FindLastCheckpoint failed: %v", err)
	}

	if latest != nil {
		t.Errorf("expected nil for nonexistent task, got %v", latest)
	}
}

func TestRecoveryManager_FindLastCheckpointNoStore(t *testing.T) {
	var manager *checkpoint.RecoveryManager = &checkpoint.RecoveryManager{Store: nil}

	_, err := manager.FindLastCheckpoint("task_1")
	if err == nil {
		t.Fatal("expected error with nil store")
	}
}

func TestRecoveryManager_HasCheckpoints(t *testing.T) {
	store := checkpoint.NewStore()
	manager := checkpoint.NewRecoveryManager(store)

	cp := &pipeline.Checkpoint{
		CheckpointID: "cp_1",
		TaskID:       "task_1",
		StageName:    "stage_1",
		StageIndex:   0,
		CreatedAt:    time.Now(),
		Context:      core.NewContext(),
	}
	if err := store.Save(cp); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Has checkpoints
	has, err := manager.HasCheckpoints("task_1")
	if err != nil {
		t.Fatalf("HasCheckpoints failed: %v", err)
	}
	if !has {
		t.Error("expected task_1 to have checkpoints")
	}

	// No checkpoints
	has, err = manager.HasCheckpoints("nonexistent_task")
	if err != nil {
		t.Fatalf("HasCheckpoints failed: %v", err)
	}
	if has {
		t.Error("expected nonexistent_task to have no checkpoints")
	}
}

func TestRecoveryManager_ClearCheckpoints(t *testing.T) {
	store := checkpoint.NewStore()
	manager := checkpoint.NewRecoveryManager(store)

	// Save checkpoints
	cp1 := &pipeline.Checkpoint{
		CheckpointID: "cp_1",
		TaskID:       "task_1",
		StageName:    "stage_1",
		StageIndex:   0,
		CreatedAt:    time.Now(),
		Context:      core.NewContext(),
	}
	cp2 := &pipeline.Checkpoint{
		CheckpointID: "cp_2",
		TaskID:       "task_1",
		StageName:    "stage_2",
		StageIndex:   1,
		CreatedAt:    time.Now(),
		Context:      core.NewContext(),
	}

	if err := store.Save(cp1); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if err := store.Save(cp2); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Should have checkpoints before clear
	has, err := manager.HasCheckpoints("task_1")
	if err != nil {
		t.Fatalf("HasCheckpoints failed: %v", err)
	}
	if !has {
		t.Fatal("expected checkpoints before clear")
	}

	// Clear checkpoints
	if err := manager.ClearCheckpoints("task_1"); err != nil {
		t.Fatalf("ClearCheckpoints failed: %v", err)
	}

	// Should have no checkpoints after clear
	has, err = manager.HasCheckpoints("task_1")
	if err != nil {
		t.Fatalf("HasCheckpoints failed: %v", err)
	}
	if has {
		t.Error("expected no checkpoints after clear")
	}
}

func TestRecoveryManager_ResumptionWorkflow(t *testing.T) {
	// Simulate a resume workflow:
	// 1. Save checkpoint after stage 1
	// 2. Interrupt
	// 3. Find checkpoint to resume
	// 4. Verify we resume from stage 2 (index 1)

	store := checkpoint.NewStore()
	manager := checkpoint.NewRecoveryManager(store)

	ctx := core.NewContext()
	ctx.Set("stage_1_output", "result from stage 1")

	// Stage 1 completed, save checkpoint for resumption
	cp := &pipeline.Checkpoint{
		CheckpointID: "cp_after_stage_1",
		TaskID:       "workflow_1",
		StageName:    "stage_1",
		StageIndex:   0, // completed stage 0
		CreatedAt:    time.Now(),
		Context:      ctx.Clone(),
		Result: pipeline.StageResult{
			StageName: "stage_1",
		},
	}

	if err := store.Save(cp); err != nil {
		t.Fatalf("Save checkpoint failed: %v", err)
	}

	// Simulate interruption and recovery...

	// Find checkpoint to resume from
	resumeCP, err := manager.FindLastCheckpoint("workflow_1")
	if err != nil {
		t.Fatalf("FindLastCheckpoint failed: %v", err)
	}

	if resumeCP == nil {
		t.Fatal("expected resumption checkpoint")
	}

	// Verify checkpoint content for resumption
	if resumeCP.StageName != "stage_1" {
		t.Errorf("expected stage_1, got %s", resumeCP.StageName)
	}
	if resumeCP.StageIndex != 0 {
		t.Errorf("expected stage index 0, got %d", resumeCP.StageIndex)
	}

	// Context should contain output from stage 1
	if val, ok := resumeCP.Context.Get("stage_1_output"); !ok || val != "result from stage 1" {
		t.Errorf("expected stage_1_output in context, got %v", val)
	}

	// Pipeline runner would resume from StageIndex+1, so next stage is index 1 (stage 2)
	nextStageIndex := resumeCP.StageIndex + 1
	if nextStageIndex != 1 {
		t.Errorf("expected next stage index 1, got %d", nextStageIndex)
	}
}

func TestRecoveryManager_MustFindCheckpoint(t *testing.T) {
	store := checkpoint.NewStore()
	manager := checkpoint.NewRecoveryManager(store)

	cp := &pipeline.Checkpoint{
		CheckpointID: "cp_1",
		TaskID:       "task_1",
		StageName:    "stage_1",
		StageIndex:   0,
		CreatedAt:    time.Now(),
		Context:      core.NewContext(),
	}
	if err := store.Save(cp); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Should return without panic
	found := manager.MustFindCheckpoint("task_1")
	if found == nil {
		t.Fatal("expected checkpoint")
	}

	// Should panic on missing store
	manager.Store = nil
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for nil store")
		}
	}()
	manager.MustFindCheckpoint("task_1")
}

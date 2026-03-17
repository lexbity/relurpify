package checkpoint_test

import (
	"testing"
	"time"

	"github.com/lexcodex/relurpify/agents/chainer/checkpoint"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/pipeline"
)

func TestStore_SaveAndLoad(t *testing.T) {
	store := checkpoint.NewStore()

	cp := &pipeline.Checkpoint{
		CheckpointID: "test_cp_1",
		TaskID:       "task_1",
		StageName:    "stage_1",
		StageIndex:   0,
		CreatedAt:    time.Now(),
		Context:      core.NewContext(),
		Result: pipeline.StageResult{
			StageName:    "stage_1",
			DecodedJSON:  "{}",
			ValidationOK: true,
		},
	}

	// Save checkpoint
	err := store.Save(cp)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Load checkpoint
	loaded, err := store.Load(cp.TaskID, cp.CheckpointID)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded == nil {
		t.Fatal("expected loaded checkpoint, got nil")
	}
	if loaded.TaskID != cp.TaskID {
		t.Errorf("expected task ID %q, got %q", cp.TaskID, loaded.TaskID)
	}
	if loaded.StageName != cp.StageName {
		t.Errorf("expected stage name %q, got %q", cp.StageName, loaded.StageName)
	}
}

func TestStore_SaveNilCheckpoint(t *testing.T) {
	store := checkpoint.NewStore()
	err := store.Save(nil)
	if err == nil {
		t.Fatal("expected error saving nil checkpoint")
	}
}

func TestStore_SaveMissingTaskID(t *testing.T) {
	store := checkpoint.NewStore()
	cp := &pipeline.Checkpoint{
		CheckpointID: "test_cp_1",
		// TaskID missing
		StageName: "stage_1",
		StageIndex: 0,
		CreatedAt: time.Now(),
		Context:   core.NewContext(),
	}
	err := store.Save(cp)
	if err == nil {
		t.Fatal("expected error saving checkpoint without task ID")
	}
}

func TestStore_LoadNotFound(t *testing.T) {
	store := checkpoint.NewStore()
	_, err := store.Load("nonexistent_task", "nonexistent_cp")
	if err == nil {
		t.Fatal("expected error loading nonexistent checkpoint")
	}
}

func TestStore_FindLastCheckpoint(t *testing.T) {
	store := checkpoint.NewStore()

	now := time.Now()

	// Save three checkpoints in order
	cp1 := &pipeline.Checkpoint{
		CheckpointID: "cp_1",
		TaskID:       "task_1",
		StageName:    "stage_1",
		StageIndex:   0,
		CreatedAt:    now,
		Context:      core.NewContext(),
	}
	cp2 := &pipeline.Checkpoint{
		CheckpointID: "cp_2",
		TaskID:       "task_1",
		StageName:    "stage_2",
		StageIndex:   1,
		CreatedAt:    now.Add(1 * time.Second),
		Context:      core.NewContext(),
	}
	cp3 := &pipeline.Checkpoint{
		CheckpointID: "cp_3",
		TaskID:       "task_1",
		StageName:    "stage_3",
		StageIndex:   2,
		CreatedAt:    now.Add(2 * time.Second),
		Context:      core.NewContext(),
	}

	for _, cp := range []*pipeline.Checkpoint{cp1, cp2, cp3} {
		if err := store.Save(cp); err != nil {
			t.Fatalf("Save failed: %v", err)
		}
	}

	// Find last should return cp3
	latest, err := store.FindLastCheckpoint("task_1")
	if err != nil {
		t.Fatalf("FindLastCheckpoint failed: %v", err)
	}
	if latest == nil {
		t.Fatal("expected latest checkpoint, got nil")
	}
	if latest.CheckpointID != "cp_3" {
		t.Errorf("expected latest checkpoint cp_3, got %q", latest.CheckpointID)
	}
}

func TestStore_FindLastCheckpointEmpty(t *testing.T) {
	store := checkpoint.NewStore()
	latest, err := store.FindLastCheckpoint("nonexistent_task")
	if err != nil {
		t.Fatalf("FindLastCheckpoint failed: %v", err)
	}
	if latest != nil {
		t.Errorf("expected nil for empty task, got %v", latest)
	}
}

func TestStore_Clear(t *testing.T) {
	store := checkpoint.NewStore()

	// Save checkpoints for two tasks
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
		TaskID:       "task_2",
		StageName:    "stage_1",
		StageIndex:   0,
		CreatedAt:    time.Now(),
		Context:      core.NewContext(),
	}

	if err := store.Save(cp1); err != nil {
		t.Fatalf("Save cp1 failed: %v", err)
	}
	if err := store.Save(cp2); err != nil {
		t.Fatalf("Save cp2 failed: %v", err)
	}

	// Clear task_1
	if err := store.Clear("task_1"); err != nil {
		t.Fatalf("Clear failed: %v", err)
	}

	// task_1 should be gone
	latest1, err := store.FindLastCheckpoint("task_1")
	if err != nil {
		t.Fatalf("FindLastCheckpoint failed: %v", err)
	}
	if latest1 != nil {
		t.Errorf("expected cleared task_1 to be nil, got %v", latest1)
	}

	// task_2 should remain
	latest2, err := store.FindLastCheckpoint("task_2")
	if err != nil {
		t.Fatalf("FindLastCheckpoint failed: %v", err)
	}
	if latest2 == nil || latest2.TaskID != "task_2" {
		t.Errorf("expected task_2 to remain, got %v", latest2)
	}
}

func TestStore_Count(t *testing.T) {
	store := checkpoint.NewStore()

	// Save three checkpoints for task_1
	for i := 0; i < 3; i++ {
		cp := &pipeline.Checkpoint{
			CheckpointID: "cp_" + string(rune(i)),
			TaskID:       "task_1",
			StageName:    "stage",
			StageIndex:   i,
			CreatedAt:    time.Now(),
			Context:      core.NewContext(),
		}
		if err := store.Save(cp); err != nil {
			t.Fatalf("Save failed: %v", err)
		}
	}

	count, err := store.Count("task_1")
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 checkpoints, got %d", count)
	}

	// Empty task should have 0
	count, err = store.Count("task_empty")
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 checkpoints for empty task, got %d", count)
	}
}

func TestStore_MultipleTaskIsolation(t *testing.T) {
	store := checkpoint.NewStore()

	// Save checkpoints for multiple tasks
	tasks := []string{"task_1", "task_2", "task_3"}
	for _, task := range tasks {
		for i := 0; i < 2; i++ {
			cp := &pipeline.Checkpoint{
				CheckpointID: task + "_cp_" + string(rune(i)),
				TaskID:       task,
				StageName:    "stage",
				StageIndex:   i,
				CreatedAt:    time.Now(),
				Context:      core.NewContext(),
			}
			if err := store.Save(cp); err != nil {
				t.Fatalf("Save failed: %v", err)
			}
		}
	}

	// Verify each task has 2 checkpoints
	for _, task := range tasks {
		count, err := store.Count(task)
		if err != nil {
			t.Fatalf("Count failed for %s: %v", task, err)
		}
		if count != 2 {
			t.Errorf("expected 2 checkpoints for %s, got %d", task, count)
		}
	}
}

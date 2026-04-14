package checkpoint

import (
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/pipeline"
)

// TestNewStore tests the store constructor
func TestNewStore(t *testing.T) {
	store := NewStore()
	if store == nil {
		t.Fatal("expected non-nil store")
	}

	// Verify store is empty
	count, err := store.Count("any-task")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("expected empty store, got count %d", count)
	}
}

// TestStoreMakeKey tests the internal makeKey helper
func TestStoreMakeKey(t *testing.T) {
	store := NewStore()

	tests := []struct {
		taskID       string
		checkpointID string
		expected     string
	}{
		{"task1", "cp1", "task1:cp1"},
		{"task", "checkpoint", "task:checkpoint"},
		{"", "cp", ":cp"},
		{"task", "", "task:"},
	}

	for _, tt := range tests {
		key := store.makeKey(tt.taskID, tt.checkpointID)
		if key != tt.expected {
			t.Errorf("makeKey(%q, %q) = %q, expected %q", tt.taskID, tt.checkpointID, key, tt.expected)
		}
	}
}

// TestStoreIsKeyForTask tests the internal isKeyForTask helper
func TestStoreIsKeyForTask(t *testing.T) {
	store := NewStore()

	tests := []struct {
		key    string
		taskID string
		want   bool
	}{
		{"task1:cp1", "task1", true},
		{"task1:cp2", "task1", true},
		{"task2:cp1", "task1", false},
		{"task1", "task1", false},      // no colon
		{"task1:", "task1", true},      // empty checkpoint ID
		{":cp1", "", true},             // empty task ID
		{"task10:cp1", "task1", false}, // prefix match but different task
		{"abc", "task1", false},        // too short
		{"", "task1", false},           // empty key
	}

	for _, tt := range tests {
		got := store.isKeyForTask(tt.key, tt.taskID)
		if got != tt.want {
			t.Errorf("isKeyForTask(%q, %q) = %v, expected %v", tt.key, tt.taskID, got, tt.want)
		}
	}
}

// TestStoreSaveMissingCheckpointID tests validation
func TestStoreSaveMissingCheckpointID(t *testing.T) {
	store := NewStore()

	cp := &pipeline.Checkpoint{
		TaskID:    "task1",
		StageName: "stage1",
		// CheckpointID is empty
		CreatedAt: time.Now(),
		Context:   core.NewContext(),
	}

	err := store.Save(cp)
	if err == nil {
		t.Error("expected error for missing checkpoint ID")
	}
}

// TestStoreLoadEmptyIDs tests validation
func TestStoreLoadEmptyIDs(t *testing.T) {
	store := NewStore()

	t.Run("empty task ID", func(t *testing.T) {
		_, err := store.Load("", "cp1")
		if err == nil {
			t.Error("expected error for empty task ID")
		}
	})

	t.Run("empty checkpoint ID", func(t *testing.T) {
		_, err := store.Load("task1", "")
		if err == nil {
			t.Error("expected error for empty checkpoint ID")
		}
	})
}

// TestStoreFindLastCheckpointEmptyTask tests edge case
func TestStoreFindLastCheckpointEmptyTask(t *testing.T) {
	store := NewStore()

	cp, err := store.FindLastCheckpoint("")
	if err == nil {
		t.Error("expected error for empty task ID")
	}
	if cp != nil {
		t.Error("expected nil checkpoint for error case")
	}
}

// TestStoreClearEmptyTask tests edge case
func TestStoreClearEmptyTask(t *testing.T) {
	store := NewStore()

	err := store.Clear("")
	if err == nil {
		t.Error("expected error for empty task ID")
	}
}

// TestStoreCountEmptyTask tests edge case
func TestStoreCountEmptyTask(t *testing.T) {
	store := NewStore()

	count, err := store.Count("")
	if err == nil {
		t.Error("expected error for empty task ID")
	}
	if count != 0 {
		t.Errorf("expected count 0 for error, got %d", count)
	}
}

// TestStoreFindLastCheckpointMultipleTasks tests finding latest across tasks
func TestStoreFindLastCheckpointMultipleTasks(t *testing.T) {
	store := NewStore()
	now := time.Now()

	// Create checkpoints for different tasks with different times
	cp1 := &pipeline.Checkpoint{
		CheckpointID: "cp1",
		TaskID:       "task1",
		StageName:    "stage1",
		StageIndex:   0,
		CreatedAt:    now,
		Context:      core.NewContext(),
	}
	cp2 := &pipeline.Checkpoint{
		CheckpointID: "cp2",
		TaskID:       "task2",
		StageName:    "stage1",
		StageIndex:   0,
		CreatedAt:    now.Add(time.Hour), // Later
		Context:      core.NewContext(),
	}
	cp3 := &pipeline.Checkpoint{
		CheckpointID: "cp3",
		TaskID:       "task1",
		StageName:    "stage2",
		StageIndex:   1,
		CreatedAt:    now.Add(30 * time.Minute), // Middle
		Context:      core.NewContext(),
	}

	// Save all
	for _, cp := range []*pipeline.Checkpoint{cp1, cp2, cp3} {
		if err := store.Save(cp); err != nil {
			t.Fatalf("Save failed: %v", err)
		}
	}

	// Find last for task1 - should be cp3 (latest of task1's checkpoints)
	latest, err := store.FindLastCheckpoint("task1")
	if err != nil {
		t.Fatalf("FindLastCheckpoint failed: %v", err)
	}
	if latest == nil || latest.CheckpointID != "cp3" {
		t.Errorf("expected cp3 for task1, got %v", latest)
	}

	// Find last for task2 - should be cp2
	latest, err = store.FindLastCheckpoint("task2")
	if err != nil {
		t.Fatalf("FindLastCheckpoint failed: %v", err)
	}
	if latest == nil || latest.CheckpointID != "cp2" {
		t.Errorf("expected cp2 for task2, got %v", latest)
	}
}

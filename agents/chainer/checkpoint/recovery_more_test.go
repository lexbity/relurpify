package checkpoint

import (
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/pipeline"
)

// TestNewRecoveryManager tests the constructor
func TestNewRecoveryManager(t *testing.T) {
	store := NewStore()
	rm := NewRecoveryManager(store)

	if rm == nil {
		t.Fatal("expected non-nil recovery manager")
	}

	if rm.Store != store {
		t.Error("expected store to be set")
	}
}

// TestRecoveryManagerFindLastCheckpointEmptyTask tests validation
func TestRecoveryManagerFindLastCheckpointEmptyTask(t *testing.T) {
	store := NewStore()
	rm := NewRecoveryManager(store)

	cp, err := rm.FindLastCheckpoint("")
	if err == nil {
		t.Error("expected error for empty task ID")
	}
	if cp != nil {
		t.Error("expected nil checkpoint for error case")
	}
}

// TestRecoveryManagerFindLastCheckpointNoStore tests nil store handling
func TestRecoveryManagerFindLastCheckpointNoStore(t *testing.T) {
	rm := &RecoveryManager{Store: nil}

	cp, err := rm.FindLastCheckpoint("task1")
	if err == nil {
		t.Error("expected error for nil store")
	}
	if cp != nil {
		t.Error("expected nil checkpoint for error case")
	}
}

// TestRecoveryManagerHasCheckpointsEmptyTask tests validation
func TestRecoveryManagerHasCheckpointsEmptyTask(t *testing.T) {
	store := NewStore()
	rm := NewRecoveryManager(store)

	has, err := rm.HasCheckpoints("")
	if err == nil {
		t.Error("expected error for empty task ID")
	}
	if has {
		t.Error("expected false for error case")
	}
}

// TestRecoveryManagerHasCheckpointsNoStore tests nil store handling
func TestRecoveryManagerHasCheckpointsNoStore(t *testing.T) {
	rm := &RecoveryManager{Store: nil}

	has, err := rm.HasCheckpoints("task1")
	if err == nil {
		t.Error("expected error for nil store")
	}
	if has {
		t.Error("expected false for error case")
	}
}

// TestRecoveryManagerClearCheckpointsEmptyTask tests validation
func TestRecoveryManagerClearCheckpointsEmptyTask(t *testing.T) {
	store := NewStore()
	rm := NewRecoveryManager(store)

	err := rm.ClearCheckpoints("")
	if err == nil {
		t.Error("expected error for empty task ID")
	}
}

// TestRecoveryManagerClearCheckpointsNoStore tests nil store handling
func TestRecoveryManagerClearCheckpointsNoStore(t *testing.T) {
	rm := &RecoveryManager{Store: nil}

	err := rm.ClearCheckpoints("task1")
	if err == nil {
		t.Error("expected error for nil store")
	}
}

// TestRecoveryManagerMustFindCheckpointSuccess tests successful case
func TestRecoveryManagerMustFindCheckpointSuccess(t *testing.T) {
	store := NewStore()
	rm := NewRecoveryManager(store)

	// Save a checkpoint
	cp := &pipeline.Checkpoint{
		CheckpointID: "cp1",
		TaskID:       "task1",
		StageName:    "stage1",
		StageIndex:   0,
		CreatedAt:    time.Now(),
		Context:      core.NewContext(),
	}
	if err := store.Save(cp); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Should not panic
	found := rm.MustFindCheckpoint("task1")
	if found == nil || found.CheckpointID != "cp1" {
		t.Error("expected to find checkpoint")
	}
}

// TestRecoveryManagerMustFindCheckpointPanic tests panic on error
func TestRecoveryManagerMustFindCheckpointPanic(t *testing.T) {
	rm := &RecoveryManager{Store: nil}

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for nil store")
		}
	}()

	rm.MustFindCheckpoint("task1")
}

// TestRecoveryManagerWithNonCheckpointStore tests fallback behavior
func TestRecoveryManagerWithNonCheckpointStore(t *testing.T) {
	// Create a recovery manager with a store that doesn't implement our internal interface
	customStore := &customCheckpointStore{}
	rm := NewRecoveryManager(customStore)

	// FindLastCheckpoint should return nil when using non-Store type
	cp, err := rm.FindLastCheckpoint("task1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cp != nil {
		t.Error("expected nil for non-Store type")
	}

	// HasCheckpoints should return false
	has, err := rm.HasCheckpoints("task1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if has {
		t.Error("expected false for non-Store type")
	}

	// ClearCheckpoints should return nil (no-op)
	err = rm.ClearCheckpoints("task1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// customCheckpointStore implements pipeline.CheckpointStore but not our internal *Store type
type customCheckpointStore struct{}

func (c *customCheckpointStore) Save(cp *pipeline.Checkpoint) error {
	return nil
}

func (c *customCheckpointStore) Load(taskID, checkpointID string) (*pipeline.Checkpoint, error) {
	return nil, nil
}

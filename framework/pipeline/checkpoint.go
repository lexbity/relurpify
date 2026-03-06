package pipeline

import (
	"errors"
	"fmt"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

// Checkpoint captures resumable pipeline state after one completed stage.
type Checkpoint struct {
	CheckpointID string
	TaskID       string
	StageName    string
	StageIndex   int
	CreatedAt    time.Time
	Context      *core.Context
	Result       StageResult
}

// CheckpointStore persists pipeline checkpoints. Phase 2 keeps the contract
// small so runtime and persistence can evolve independently in phase 3.
type CheckpointStore interface {
	Save(checkpoint *Checkpoint) error
	Load(taskID, checkpointID string) (*Checkpoint, error)
}

func validateCheckpoint(cp *Checkpoint) error {
	if cp == nil {
		return errors.New("pipeline checkpoint required")
	}
	if cp.Context == nil {
		return errors.New("pipeline checkpoint context required")
	}
	if cp.StageIndex < 0 {
		return errors.New("pipeline checkpoint stage index cannot be negative")
	}
	if cp.CheckpointID == "" {
		return fmt.Errorf("pipeline checkpoint id required")
	}
	return nil
}

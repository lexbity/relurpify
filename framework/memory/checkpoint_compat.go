package memory

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/graph"
)

var ErrCheckpointNotFound = errors.New("checkpoint not found")

func isCheckpointNotFound(err error) bool {
	return err != nil && errors.Is(err, ErrCheckpointNotFound)
}

// FallbackCheckpointStore chains a primary checkpoint store with a read-only fallback.
type FallbackCheckpointStore struct {
	Primary  CheckpointSnapshotStore
	Fallback CheckpointSnapshotStore
}

func (s *FallbackCheckpointStore) Save(checkpoint *graph.GraphCheckpoint) error {
	if s == nil || s.Primary == nil {
		return fmt.Errorf("primary checkpoint store unavailable")
	}
	return s.Primary.Save(checkpoint)
}

func (s *FallbackCheckpointStore) Load(taskID, checkpointID string) (*graph.GraphCheckpoint, error) {
	if s == nil {
		return nil, fmt.Errorf("checkpoint store unavailable")
	}
	if s.Primary != nil {
		checkpoint, err := s.Primary.Load(taskID, checkpointID)
		if err == nil {
			return checkpoint, nil
		}
		if !isCheckpointNotFound(err) {
			return nil, err
		}
	}
	if s.Fallback == nil {
		return nil, ErrCheckpointNotFound
	}
	return s.Fallback.Load(taskID, checkpointID)
}

func (s *FallbackCheckpointStore) List(taskID string) ([]string, error) {
	if s == nil {
		return nil, nil
	}
	seen := make(map[string]struct{})
	var out []string
	for _, store := range []CheckpointSnapshotStore{s.Primary, s.Fallback} {
		if store == nil {
			continue
		}
		ids, err := store.List(taskID)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out, nil
}

// WorkflowSnapshotCheckpointAdapter exposes legacy WorkflowStore snapshots as checkpoints.
type WorkflowSnapshotCheckpointAdapter struct {
	Store WorkflowStore
}

func (a *WorkflowSnapshotCheckpointAdapter) Save(checkpoint *graph.GraphCheckpoint) error {
	if a == nil || a.Store == nil {
		return fmt.Errorf("workflow snapshot store unavailable")
	}
	if checkpoint == nil {
		return fmt.Errorf("nil checkpoint")
	}
	return a.Store.Save(context.Background(), workflowSnapshotFromCheckpoint(checkpoint))
}

func (a *WorkflowSnapshotCheckpointAdapter) Load(taskID, checkpointID string) (*graph.GraphCheckpoint, error) {
	if a == nil || a.Store == nil {
		return nil, fmt.Errorf("workflow snapshot store unavailable")
	}
	snapshot, ok, err := a.Store.Load(context.Background(), checkpointID)
	if err != nil {
		return nil, err
	}
	if !ok || snapshot == nil {
		return nil, ErrCheckpointNotFound
	}
	checkpoint := checkpointFromWorkflowSnapshot(snapshot)
	if taskID != "" && checkpoint.TaskID != "" && checkpoint.TaskID != taskID {
		return nil, ErrCheckpointNotFound
	}
	return checkpoint, nil
}

func (a *WorkflowSnapshotCheckpointAdapter) List(taskID string) ([]string, error) {
	if a == nil || a.Store == nil {
		return nil, nil
	}
	snapshots, err := a.Store.List(context.Background())
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(snapshots))
	for _, snapshot := range snapshots {
		if taskID != "" && snapshot.Task != nil && strings.TrimSpace(snapshot.Task.ID) != taskID {
			continue
		}
		ids = append(ids, snapshot.ID)
	}
	sort.Strings(ids)
	return ids, nil
}

func workflowSnapshotFromCheckpoint(checkpoint *graph.GraphCheckpoint) *WorkflowSnapshot {
	if checkpoint == nil {
		return nil
	}
	status := WorkflowStatusRunning
	if checkpoint.NextNodeID == "" {
		status = WorkflowStatusCompleted
	}
	return &WorkflowSnapshot{
		ID: checkpoint.CheckpointID,
		Task: &core.Task{
			ID: checkpoint.TaskID,
		},
		Graph: &graph.GraphSnapshot{
			NextNodeID: checkpoint.NextNodeID,
			State:      checkpointContextSnapshot(checkpoint.Context),
		},
		Status:    status,
		Metadata:  cloneCheckpointMetadata(checkpoint.Metadata),
		UpdatedAt: checkpoint.CreatedAt,
	}
}

func checkpointFromWorkflowSnapshot(snapshot *WorkflowSnapshot) *graph.GraphCheckpoint {
	if snapshot == nil {
		return nil
	}
	taskID := ""
	if snapshot.Task != nil {
		taskID = snapshot.Task.ID
	}
	ctx := graph.NewContext()
	if snapshot.Graph != nil && snapshot.Graph.State != nil {
		_ = ctx.Restore(snapshot.Graph.State)
	}
	checkpoint := &graph.GraphCheckpoint{
		CheckpointID:    snapshot.ID,
		TaskID:          taskID,
		CreatedAt:       snapshot.UpdatedAt,
		CompletedNodeID: "",
		NextNodeID:      "",
		Context:         ctx,
		Metadata:        cloneCheckpointMetadata(snapshot.Metadata),
	}
	if snapshot.Graph != nil {
		checkpoint.NextNodeID = snapshot.Graph.NextNodeID
		if checkpoint.NextNodeID != "" {
			checkpoint.LastTransition = &graph.NodeTransitionRecord{
				NextNodeID:       checkpoint.NextNodeID,
				TransitionReason: "legacy-workflow-snapshot",
				CompletedAt:      snapshot.UpdatedAt,
			}
		}
	}
	if snapshot.Status == WorkflowStatusCompleted {
		checkpoint.LastResultSummary = &graph.CheckpointResultSummary{Success: true}
	}
	return checkpoint
}

func checkpointContextSnapshot(ctx *graph.Context) *graph.ContextSnapshot {
	if ctx == nil {
		return graph.NewContext().Snapshot()
	}
	return ctx.Snapshot()
}

func cloneCheckpointMetadata(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	out := make(map[string]any, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}

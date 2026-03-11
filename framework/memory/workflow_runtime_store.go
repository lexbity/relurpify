package memory

import (
	"context"

	"github.com/lexcodex/relurpify/framework/graph"
)

// CheckpointSnapshotStore captures resumable graph checkpoints.
type CheckpointSnapshotStore interface {
	Save(checkpoint *graph.GraphCheckpoint) error
	Load(taskID, checkpointID string) (*graph.GraphCheckpoint, error)
	List(taskID string) ([]string, error)
}

// WorkflowRuntimeStore is the unified runtime-facing storage surface used by
// phase 7 consolidation work.
type WorkflowRuntimeStore interface {
	WorkflowStateStore
	RuntimeMemoryStore
	CheckpointSnapshotStore
}

// CompositeRuntimeStore composes the authoritative runtime stores behind one surface.
type CompositeRuntimeStore struct {
	WorkflowStateStore
	RuntimeMemoryStore
	CheckpointSnapshotStore
}

func NewCompositeRuntimeStore(workflows WorkflowStateStore, runtime RuntimeMemoryStore, checkpoints CheckpointSnapshotStore) *CompositeRuntimeStore {
	return &CompositeRuntimeStore{
		WorkflowStateStore:      workflows,
		RuntimeMemoryStore:      runtime,
		CheckpointSnapshotStore: checkpoints,
	}
}

func (s *CompositeRuntimeStore) Remember(ctx context.Context, key string, value map[string]interface{}, scope MemoryScope) error {
	if generic, ok := s.RuntimeMemoryStore.(MemoryStore); ok {
		return generic.Remember(ctx, key, value, scope)
	}
	return nil
}

func (s *CompositeRuntimeStore) Recall(ctx context.Context, key string, scope MemoryScope) (*MemoryRecord, bool, error) {
	if generic, ok := s.RuntimeMemoryStore.(MemoryStore); ok {
		return generic.Recall(ctx, key, scope)
	}
	return nil, false, nil
}

func (s *CompositeRuntimeStore) Search(ctx context.Context, query string, scope MemoryScope) ([]MemoryRecord, error) {
	if generic, ok := s.RuntimeMemoryStore.(MemoryStore); ok {
		return generic.Search(ctx, query, scope)
	}
	return nil, nil
}

func (s *CompositeRuntimeStore) Forget(ctx context.Context, key string, scope MemoryScope) error {
	if generic, ok := s.RuntimeMemoryStore.(MemoryStore); ok {
		return generic.Forget(ctx, key, scope)
	}
	return nil
}

func (s *CompositeRuntimeStore) Summarize(ctx context.Context, scope MemoryScope) (string, error) {
	if generic, ok := s.RuntimeMemoryStore.(MemoryStore); ok {
		return generic.Summarize(ctx, scope)
	}
	return "", nil
}

// QueryWorkflowRuntime summarises the unified durable records visible for a workflow/run.
func (s *CompositeRuntimeStore) QueryWorkflowRuntime(ctx context.Context, workflowID, runID string) (map[string]any, error) {
	payload := map[string]any{
		"workflow_id": workflowID,
		"run_id":      runID,
	}
	if s == nil {
		return payload, nil
	}
	if s.WorkflowStateStore != nil {
		workflow, ok, err := s.GetWorkflow(ctx, workflowID)
		if err != nil {
			return nil, err
		}
		if ok {
			payload["workflow"] = workflow
		}
		if runID != "" {
			run, ok, err := s.GetRun(ctx, runID)
			if err != nil {
				return nil, err
			}
			if ok {
				payload["run"] = run
			}
			artifacts, err := s.ListWorkflowArtifacts(ctx, workflowID, runID)
			if err != nil {
				return nil, err
			}
			payload["workflow_artifacts"] = artifacts
			events, err := s.ListEvents(ctx, workflowID, 32)
			if err != nil {
				return nil, err
			}
			payload["events"] = events
		}
	}
	if s.RuntimeMemoryStore != nil {
		decl, err := s.SearchDeclarative(ctx, DeclarativeMemoryQuery{WorkflowID: workflowID, Limit: 32})
		if err != nil {
			return nil, err
		}
		payload["declarative_memory"] = decl
		proc, err := s.SearchProcedural(ctx, ProceduralMemoryQuery{WorkflowID: workflowID, Limit: 32})
		if err != nil {
			return nil, err
		}
		payload["procedural_memory"] = proc
	}
	return payload, nil
}

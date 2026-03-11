package memory

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// WorkflowStatus enumerates snapshot states.
type WorkflowStatus string

const (
	WorkflowStatusPending   WorkflowStatus = "pending"
	WorkflowStatusRunning   WorkflowStatus = "running"
	WorkflowStatusCompleted WorkflowStatus = "completed"
	WorkflowStatusFailed    WorkflowStatus = "failed"
)

// WorkflowSnapshot persists graph execution state on disk.
type WorkflowSnapshot struct {
	ID        string                 `json:"id"`
	Task      *core.Task             `json:"task"`
	Graph     *graph.GraphSnapshot   `json:"graph"`
	Status    WorkflowStatus         `json:"status"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	UpdatedAt time.Time              `json:"updated_at"`
}

// WorkflowStore persists snapshots between runs.
// Deprecated: use CheckpointSnapshotStore-backed runtime persistence instead.
type WorkflowStore interface {
	Save(ctx context.Context, snapshot *WorkflowSnapshot) error
	Load(ctx context.Context, id string) (*WorkflowSnapshot, bool, error)
	List(ctx context.Context) ([]WorkflowSnapshot, error)
	Delete(ctx context.Context, id string) error
}

// FileWorkflowStore stores snapshots as JSON on disk.
// Deprecated: use db.SQLiteCheckpointStore or another CheckpointSnapshotStore implementation.
type FileWorkflowStore struct {
	path  string
	mu    sync.RWMutex
	cache map[string]WorkflowSnapshot
}

// NewFileWorkflowStore creates a store under the provided directory.
// Deprecated: new code should use CheckpointSnapshotStore-backed runtime storage.
func NewFileWorkflowStore(root string) (*FileWorkflowStore, error) {
	if root == "" {
		return nil, errors.New("workflow store root required")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	store := &FileWorkflowStore{
		path:  filepath.Join(root, "workflows.json"),
		cache: make(map[string]WorkflowSnapshot),
	}
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

// load hydrates the in-memory cache from disk when the process starts so
// workflows survive restarts.
func (s *FileWorkflowStore) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	var snapshots []WorkflowSnapshot
	if err := json.Unmarshal(data, &snapshots); err != nil {
		return err
	}
	for _, snap := range snapshots {
		s.cache[snap.ID] = snap
	}
	return nil
}

// persist writes the cached snapshots back to disk after any mutation.
func (s *FileWorkflowStore) persist() error {
	var snapshots []WorkflowSnapshot
	for _, snap := range s.cache {
		snapshots = append(snapshots, snap)
	}
	data, err := json.MarshalIndent(snapshots, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o644)
}

// Save writes a snapshot to disk.
func (s *FileWorkflowStore) Save(ctx context.Context, snapshot *WorkflowSnapshot) error {
	if snapshot == nil {
		return errors.New("nil snapshot")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	snapshot.UpdatedAt = time.Now().UTC()
	clone := *snapshot
	clone.Metadata = core.RedactMetadataMap(clone.Metadata)
	if clone.Task != nil {
		taskClone := *clone.Task
		taskClone.Context = decodeRedactedMap(core.RedactAny(taskClone.Context))
		clone.Task = &taskClone
	}
	s.cache[snapshot.ID] = clone
	return s.persist()
}

// Load retrieves a snapshot by ID.
func (s *FileWorkflowStore) Load(ctx context.Context, id string) (*WorkflowSnapshot, bool, error) {
	select {
	case <-ctx.Done():
		return nil, false, ctx.Err()
	default:
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	snap, ok := s.cache[id]
	if !ok {
		return nil, false, nil
	}
	return &snap, true, nil
}

// List returns all snapshots.
func (s *FileWorkflowStore) List(ctx context.Context) ([]WorkflowSnapshot, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]WorkflowSnapshot, 0, len(s.cache))
	for _, snap := range s.cache {
		result = append(result, snap)
	}
	return result, nil
}

// Delete removes a snapshot.
func (s *FileWorkflowStore) Delete(ctx context.Context, id string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.cache, id)
	return s.persist()
}

func decodeRedactedMap(value any) map[string]any {
	if typed, ok := value.(map[string]interface{}); ok {
		return typed
	}
	return nil
}

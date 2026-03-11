package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/lexcodex/relurpify/framework/memory"
)

type checkpointEnvelope struct {
	Metadata          map[string]any                 `json:"metadata,omitempty"`
	VisitCounts       map[string]int                 `json:"visit_counts,omitempty"`
	ExecutionPath     []string                       `json:"execution_path,omitempty"`
	LastResultSummary *graph.CheckpointResultSummary `json:"last_result_summary,omitempty"`
	CompressedContext *graph.CompressedContext       `json:"compressed_context,omitempty"`
}

// SQLiteCheckpointStore persists resumable graph checkpoints into SQLite.
type SQLiteCheckpointStore struct {
	db         *sql.DB
	events     memory.WorkflowStateStore
	workflowID string
	runID      string
}

func NewSQLiteCheckpointStore(db *sql.DB) *SQLiteCheckpointStore {
	store := &SQLiteCheckpointStore{db: db}
	store.ensureSchema()
	return store
}

func NewSQLiteCheckpointStoreWithEvents(db *sql.DB, events memory.WorkflowStateStore, workflowID, runID string) *SQLiteCheckpointStore {
	store := &SQLiteCheckpointStore{
		db:         db,
		events:     events,
		workflowID: workflowID,
		runID:      runID,
	}
	store.ensureSchema()
	return store
}

func (s *SQLiteCheckpointStore) ensureSchema() {
	if s == nil || s.db == nil {
		return
	}
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS graph_checkpoints (
			checkpoint_id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			workflow_id TEXT NOT NULL DEFAULT '',
			run_id TEXT NOT NULL DEFAULT '',
			completed_node_id TEXT NOT NULL DEFAULT '',
			next_node_id TEXT NOT NULL DEFAULT '',
			graph_hash TEXT NOT NULL DEFAULT '',
			state_json TEXT NOT NULL DEFAULT 'null',
			transition_json TEXT NOT NULL DEFAULT 'null',
			metadata_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_graph_checkpoints_task_created ON graph_checkpoints(task_id, created_at DESC);`,
	}
	for _, stmt := range stmts {
		_, _ = s.db.Exec(stmt)
	}
}

func (s *SQLiteCheckpointStore) Save(checkpoint *graph.GraphCheckpoint) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("sqlite checkpoint store unavailable")
	}
	if checkpoint == nil {
		return fmt.Errorf("nil checkpoint")
	}
	checkpoint.CreatedAt = ensureTime(checkpoint.CreatedAt)
	stateJSON, err := json.Marshal(checkpoint.Context)
	if err != nil {
		return err
	}
	transitionJSON, err := json.Marshal(checkpoint.LastTransition)
	if err != nil {
		return err
	}
	metadataJSON, err := json.Marshal(checkpointEnvelope{
		Metadata:          checkpoint.Metadata,
		VisitCounts:       checkpoint.VisitCounts,
		ExecutionPath:     checkpoint.ExecutionPath,
		LastResultSummary: checkpoint.LastResultSummary,
		CompressedContext: checkpoint.CompressedContext,
	})
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		`INSERT OR REPLACE INTO graph_checkpoints (
			checkpoint_id, task_id, workflow_id, run_id, completed_node_id, next_node_id, graph_hash, state_json, transition_json, metadata_json, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		checkpoint.CheckpointID,
		checkpoint.TaskID,
		s.workflowID,
		s.runID,
		checkpoint.CompletedNodeID,
		checkpoint.NextNodeID,
		checkpoint.GraphHash,
		string(stateJSON),
		string(transitionJSON),
		string(metadataJSON),
		timeString(checkpoint.CreatedAt),
	)
	if err != nil {
		return err
	}
	s.emitCheckpointEvent(context.Background(), checkpoint)
	return nil
}

func (s *SQLiteCheckpointStore) Load(taskID, checkpointID string) (*graph.GraphCheckpoint, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("sqlite checkpoint store unavailable")
	}
	row := s.db.QueryRow(
		`SELECT checkpoint_id, task_id, workflow_id, run_id, completed_node_id, next_node_id, graph_hash, state_json, transition_json, metadata_json, created_at
		FROM graph_checkpoints WHERE task_id = ? AND checkpoint_id = ?`,
		taskID,
		checkpointID,
	)
	var checkpoint graph.GraphCheckpoint
	var workflowID string
	var runID string
	var stateJSON string
	var transitionJSON string
	var metadataJSON string
	var createdAt string
	if err := row.Scan(
		&checkpoint.CheckpointID,
		&checkpoint.TaskID,
		&workflowID,
		&runID,
		&checkpoint.CompletedNodeID,
		&checkpoint.NextNodeID,
		&checkpoint.GraphHash,
		&stateJSON,
		&transitionJSON,
		&metadataJSON,
		&createdAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, memory.ErrCheckpointNotFound
		}
		return nil, err
	}
	checkpoint.Context = graph.NewContext()
	if stateJSON != "" && stateJSON != "null" {
		if err := json.Unmarshal([]byte(stateJSON), checkpoint.Context); err != nil {
			return nil, err
		}
	}
	if transitionJSON != "" && transitionJSON != "null" {
		var transition graph.NodeTransitionRecord
		if err := json.Unmarshal([]byte(transitionJSON), &transition); err != nil {
			return nil, err
		}
		checkpoint.LastTransition = &transition
	}
	if metadataJSON != "" && metadataJSON != "null" {
		var env checkpointEnvelope
		if err := json.Unmarshal([]byte(metadataJSON), &env); err != nil {
			return nil, err
		}
		checkpoint.Metadata = env.Metadata
		checkpoint.VisitCounts = env.VisitCounts
		checkpoint.ExecutionPath = env.ExecutionPath
		checkpoint.LastResultSummary = env.LastResultSummary
		checkpoint.CompressedContext = env.CompressedContext
	}
	if parsed, err := time.Parse(time.RFC3339Nano, createdAt); err == nil {
		checkpoint.CreatedAt = parsed
	}
	if checkpoint.Metadata == nil {
		checkpoint.Metadata = map[string]any{}
	}
	checkpoint.CurrentNodeID = checkpoint.CompletedNodeID
	if workflowID != "" {
		checkpoint.Metadata["workflow_id"] = workflowID
	}
	if runID != "" {
		checkpoint.Metadata["run_id"] = runID
	}
	return &checkpoint, nil
}

func (s *SQLiteCheckpointStore) List(taskID string) ([]string, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	rows, err := s.db.Query(`SELECT checkpoint_id FROM graph_checkpoints WHERE task_id = ? ORDER BY created_at DESC`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var checkpointID string
		if err := rows.Scan(&checkpointID); err != nil {
			return nil, err
		}
		out = append(out, checkpointID)
	}
	return out, rows.Err()
}

func (s *SQLiteCheckpointStore) emitCheckpointEvent(ctx context.Context, checkpoint *graph.GraphCheckpoint) {
	if s == nil || s.events == nil || checkpoint == nil {
		return
	}
	_ = s.events.AppendEvent(ctx, memory.WorkflowEventRecord{
		EventID:    fmt.Sprintf("chk_%s_%d", checkpoint.CheckpointID, checkpoint.CreatedAt.UnixNano()),
		WorkflowID: s.workflowID,
		RunID:      s.runID,
		EventType:  "graph.checkpoint",
		Message:    "checkpoint saved",
		Metadata: map[string]any{
			"checkpoint_id":     checkpoint.CheckpointID,
			"task_id":           checkpoint.TaskID,
			"completed_node_id": checkpoint.CompletedNodeID,
			"next_node_id":      checkpoint.NextNodeID,
		},
		CreatedAt: checkpoint.CreatedAt,
	})
}

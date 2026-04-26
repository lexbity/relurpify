package memory

import (
	"context"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
)

type WorkflowRunStatus string

const (
	WorkflowRunStatusPending     WorkflowRunStatus = "pending"
	WorkflowRunStatusRunning     WorkflowRunStatus = "running"
	WorkflowRunStatusCompleted   WorkflowRunStatus = "completed"
	WorkflowRunStatusFailed      WorkflowRunStatus = "failed"
	WorkflowRunStatusCanceled    WorkflowRunStatus = "canceled"
	WorkflowRunStatusCancelled   WorkflowRunStatus = "cancelled"
	WorkflowRunStatusNeedsReplan WorkflowRunStatus = "needs_replan"
)

type WorkflowRecord struct {
	WorkflowID  string            `json:"workflow_id"`
	TaskID      string            `json:"task_id,omitempty"`
	TaskType    core.TaskType     `json:"task_type,omitempty"`
	Instruction string            `json:"instruction,omitempty"`
	Status      WorkflowRunStatus `json:"status,omitempty"`
	Metadata    map[string]any    `json:"metadata,omitempty"`
	CreatedAt   time.Time         `json:"created_at,omitempty"`
	UpdatedAt   time.Time         `json:"updated_at,omitempty"`
}

type WorkflowRunRecord struct {
	RunID          string            `json:"run_id"`
	WorkflowID     string            `json:"workflow_id,omitempty"`
	Status         WorkflowRunStatus `json:"status,omitempty"`
	AgentName      string            `json:"agent_name,omitempty"`
	AgentMode      string            `json:"agent_mode,omitempty"`
	RuntimeVersion string            `json:"runtime_version,omitempty"`
	StartedAt      time.Time         `json:"started_at,omitempty"`
	EndedAt        time.Time         `json:"ended_at,omitempty"`
	Metadata       map[string]any    `json:"metadata,omitempty"`
}

type WorkflowEventRecord struct {
	EventID    string         `json:"event_id,omitempty"`
	WorkflowID string         `json:"workflow_id,omitempty"`
	RunID      string         `json:"run_id,omitempty"`
	StepID     string         `json:"step_id,omitempty"`
	EventType  string         `json:"event_type,omitempty"`
	Message    string         `json:"message,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	CreatedAt  time.Time      `json:"created_at,omitempty"`
}

type MemoryRecord struct {
	Key       string         `json:"key,omitempty"`
	Value     map[string]any `json:"value,omitempty"`
	Scope     MemoryScope    `json:"scope,omitempty"`
	CreatedAt time.Time      `json:"created_at,omitempty"`
	UpdatedAt time.Time      `json:"updated_at,omitempty"`
}

type WorkflowStateStore interface {
	GetWorkflow(context.Context, string) (*WorkflowRecord, bool, error)
	CreateWorkflow(context.Context, WorkflowRecord) error
	GetRun(context.Context, string) (*WorkflowRunRecord, bool, error)
	CreateRun(context.Context, WorkflowRunRecord) error
	UpdateWorkflowMetadata(context.Context, string, map[string]any) error
	ListWorkflows(context.Context, int) ([]WorkflowRecord, error)
	UpsertWorkflowArtifact(context.Context, WorkflowArtifactRecord) error
	WorkflowArtifactByID(context.Context, string) (*WorkflowArtifactRecord, bool, error)
	ListWorkflowArtifacts(context.Context, string, string) ([]WorkflowArtifactRecord, error)
	AppendEvent(context.Context, WorkflowEventRecord) error
	ListEvents(context.Context, string, int) ([]WorkflowEventRecord, error)
	LatestEvent(context.Context, string) (*WorkflowEventRecord, bool, error)
	LatestEventByTypes(context.Context, string, ...string) (*WorkflowEventRecord, bool, error)
	Close() error
}

type MemoryStore interface {
	Remember(context.Context, string, map[string]any, MemoryScope) error
	Recall(context.Context, string, MemoryScope) (*MemoryRecord, bool, error)
	Search(context.Context, string, MemoryScope) ([]MemoryRecord, error)
	Forget(context.Context, string, MemoryScope) error
	Summarize(context.Context, MemoryScope) (string, error)
}

type CompositeRuntimeStore struct {
	WorkflowStateStore      WorkflowStateStore
	RuntimeMemoryStore      MemoryStore
	CheckpointSnapshotStore any
	mu                      sync.RWMutex
	vectorStore             VectorStore
	memoryRecords           map[string]map[MemoryScope]MemoryRecord
}

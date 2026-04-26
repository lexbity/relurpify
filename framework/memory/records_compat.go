package memory

import "time"

// KnowledgeEntry is the neutral name for a workflow-scoped knowledge record.
type KnowledgeEntry = KnowledgeRecord

// Deprecated: use KnowledgeEntry in new code.
type KnowledgeRecord struct {
	RecordID   string         `json:"record_id,omitempty" yaml:"record_id,omitempty"`
	WorkflowID string         `json:"workflow_id,omitempty" yaml:"workflow_id,omitempty"`
	Kind       KnowledgeKind  `json:"kind,omitempty" yaml:"kind,omitempty"`
	Title      string         `json:"title,omitempty" yaml:"title,omitempty"`
	Content    string         `json:"content,omitempty" yaml:"content,omitempty"`
	CreatedAt  time.Time      `json:"created_at,omitempty" yaml:"created_at,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

// DeclarativeMemoryEntry is the neutral name for a declarative memory record.
type DeclarativeMemoryEntry = DeclarativeMemoryRecord

// Deprecated: use DeclarativeMemoryEntry in new code.
type DeclarativeMemoryRecord struct {
	RecordID    string                `json:"record_id,omitempty" yaml:"record_id,omitempty"`
	Kind        DeclarativeMemoryKind `json:"kind,omitempty" yaml:"kind,omitempty"`
	Title       string                `json:"title,omitempty" yaml:"title,omitempty"`
	Content     string                `json:"content,omitempty" yaml:"content,omitempty"`
	Summary     string                `json:"summary,omitempty" yaml:"summary,omitempty"`
	ArtifactRef string                `json:"artifact_ref,omitempty" yaml:"artifact_ref,omitempty"`
	Scope       MemoryScope           `json:"scope,omitempty" yaml:"scope,omitempty"`
	Verified    bool                  `json:"verified,omitempty" yaml:"verified,omitempty"`
}

// ProceduralMemoryEntry is the neutral name for a procedural memory record.
type ProceduralMemoryEntry = ProceduralMemoryRecord

// Deprecated: use ProceduralMemoryEntry in new code.
type ProceduralMemoryRecord struct {
	RoutineID   string               `json:"routine_id,omitempty" yaml:"routine_id,omitempty"`
	Kind        ProceduralMemoryKind `json:"kind,omitempty" yaml:"kind,omitempty"`
	Title       string               `json:"title,omitempty" yaml:"title,omitempty"`
	Description string               `json:"description,omitempty" yaml:"description,omitempty"`
	Summary     string               `json:"summary,omitempty" yaml:"summary,omitempty"`
	BodyRef     string               `json:"body_ref,omitempty" yaml:"body_ref,omitempty"`
	Scope       MemoryScope          `json:"scope,omitempty" yaml:"scope,omitempty"`
	Verified    bool                 `json:"verified,omitempty" yaml:"verified,omitempty"`
	ReuseCount  int                  `json:"reuse_count,omitempty" yaml:"reuse_count,omitempty"`
}

// DelegationEntry is the neutral name for a workflow delegation record.
type DelegationEntry = WorkflowDelegationRecord

// Deprecated: use DelegationEntry in new code.
type WorkflowDelegationRecord struct {
	DelegationID   string         `json:"delegation_id,omitempty" yaml:"delegation_id,omitempty"`
	WorkflowID     string         `json:"workflow_id,omitempty" yaml:"workflow_id,omitempty"`
	RunID          string         `json:"run_id,omitempty" yaml:"run_id,omitempty"`
	TaskID         string         `json:"task_id,omitempty" yaml:"task_id,omitempty"`
	State          string         `json:"state,omitempty" yaml:"state,omitempty"`
	TrustClass     string         `json:"trust_class,omitempty" yaml:"trust_class,omitempty"`
	Recoverability string         `json:"recoverability,omitempty" yaml:"recoverability,omitempty"`
	Background     bool           `json:"background,omitempty" yaml:"background,omitempty"`
	Request        any            `json:"request,omitempty" yaml:"request,omitempty"`
	Result         any            `json:"result,omitempty" yaml:"result,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	StartedAt      time.Time      `json:"started_at,omitempty" yaml:"started_at,omitempty"`
	UpdatedAt      time.Time      `json:"updated_at,omitempty" yaml:"updated_at,omitempty"`
}

// DelegationTransitionEntry is the neutral name for a delegation transition record.
type DelegationTransitionEntry = WorkflowDelegationTransitionRecord

// Deprecated: use DelegationTransitionEntry in new code.
type WorkflowDelegationTransitionRecord struct {
	TransitionID string         `json:"transition_id,omitempty" yaml:"transition_id,omitempty"`
	DelegationID string         `json:"delegation_id,omitempty" yaml:"delegation_id,omitempty"`
	WorkflowID   string         `json:"workflow_id,omitempty" yaml:"workflow_id,omitempty"`
	RunID        string         `json:"run_id,omitempty" yaml:"run_id,omitempty"`
	ToState      string         `json:"to_state,omitempty" yaml:"to_state,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	CreatedAt    time.Time      `json:"created_at,omitempty" yaml:"created_at,omitempty"`
}

package ports

import "time"

// WorkflowRecord is the context-owned view of a workflow lifecycle record.
type WorkflowRecord struct {
	WorkflowID  string
	AgentID     string
	SessionID   string
	TaskID      string
	Status      string
	StartedAt   time.Time
	CompletedAt *time.Time
	Error       string
	Metadata    map[string]any
}

// WorkflowRunRecord is the context-owned view of a workflow run.
type WorkflowRunRecord struct {
	RunID        string
	WorkflowID   string
	AgentID      string
	SessionID    string
	Status       string
	Phase        string
	Depth        int
	ParentRunID  string
	StartedAt    time.Time
	CompletedAt  *time.Time
	Error        string
	InputTask    map[string]any
	OutputResult map[string]any
	Metadata     map[string]any
}

// DelegationEntry is the context-owned view of a delegation record.
type DelegationEntry struct {
	DelegationID       string
	WorkflowID         string
	RunID              string
	AgentID            string
	TargetCapabilityID string
	TargetProviderID   string
	State              string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	Error              string
	Metadata           map[string]any
}

// DelegationTransitionEntry records a state transition in a delegation.
type DelegationTransitionEntry struct {
	DelegationID string
	FromState    string
	ToState      string
	Reason       string
	Timestamp    time.Time
	Metadata     map[string]any
}

// WorkflowEventRecord is the context-owned view of a workflow event.
type WorkflowEventRecord struct {
	EventID    string
	WorkflowID string
	RunID      string
	AgentID    string
	EventType  string
	Payload    map[string]any
	Timestamp  time.Time
	Sequence   int64
	Metadata   map[string]any
}

// WorkflowArtifactRecord is the context-owned view of a workflow artifact.
type WorkflowArtifactRecord struct {
	ArtifactID  string
	WorkflowID  string
	RunID       string
	AgentID     string
	StorageRef  string
	StorageKind string
	ContentType string
	Summary     string
	SizeBytes   int64
	CreatedAt   time.Time
	TTL         *time.Duration
	Metadata    map[string]any
}

// LineageBindingRecord is the context-owned view of a lineage binding.
type LineageBindingRecord struct {
	BindingID    string
	WorkflowID   string
	FromEntityID string
	FromRunID    string
	ToEntityID   string
	ToRunID      string
	Relationship string
	CreatedAt    time.Time
	Metadata     map[string]any
}

// LifecycleRepository is the context-owned interface for workflow/run lifecycle storage.
// execution/agentlifecycle implements it; context/persistence provides adapters.
type LifecycleRepository interface {
	CreateWorkflow(record WorkflowRecord) error
	GetWorkflow(workflowID string) (*WorkflowRecord, error)
	ListWorkflows(agentID string) ([]WorkflowRecord, error)

	CreateRun(record WorkflowRunRecord) error
	GetRun(runID string) (*WorkflowRunRecord, error)
	ListRuns(workflowID string) ([]WorkflowRunRecord, error)

	UpsertDelegation(entry DelegationEntry) error
	GetDelegation(delegationID string) (*DelegationEntry, error)
	ListDelegations(workflowID string) ([]DelegationEntry, error)
	ListDelegationsByRun(runID string) ([]DelegationEntry, error)
	AppendDelegationTransition(transition DelegationTransitionEntry) error
	ListDelegationTransitions(delegationID string) ([]DelegationTransitionEntry, error)

	AppendEvent(record WorkflowEventRecord) error
	ListEvents(workflowID string, limit int) ([]WorkflowEventRecord, error)
	ListEventsByRun(runID string, limit int) ([]WorkflowEventRecord, error)

	UpsertArtifact(record WorkflowArtifactRecord) error
	GetArtifact(artifactID string) (*WorkflowArtifactRecord, error)
	ListArtifacts(workflowID string) ([]WorkflowArtifactRecord, error)
	ListArtifactsByRun(runID string) ([]WorkflowArtifactRecord, error)

	UpsertLineageBinding(record LineageBindingRecord) error
	GetLineageBinding(bindingID string) (*LineageBindingRecord, error)
	FindLineageBinding(fromEntityID, toEntityID string) (*LineageBindingRecord, error)
	FindLineageBindingsByFrom(fromEntityID string) ([]LineageBindingRecord, error)
	FindLineageBindingsByTo(toEntityID string) ([]LineageBindingRecord, error)
}

package ports

import (
	"context"
	"time"
)

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

// SelectionDecisionCandidate is one candidate route in a SelectionDecisionRecord's
// candidate set. It carries the deterministic score split and the matched
// candidate-side vocabulary (public registry data — never raw user text).
type SelectionDecisionCandidate struct {
	RouteID         string         `json:"route_id"`
	Kind            string         `json:"kind"`
	Availability    string         `json:"availability"`
	Score           int            `json:"score"`
	Components      map[string]int `json:"components,omitempty"`
	MatchedKeywords []string       `json:"matched_keywords,omitempty"`
}

// SelectionDecisionSelected is the chosen route of a SelectionDecisionRecord.
type SelectionDecisionSelected struct {
	RouteID string `json:"route_id"`
	Kind    string `json:"kind"`
}

// SelectionDecisionTier2 records the bounded Tier-2 disambiguation attempt for
// one selection decision (D10). Used=false means the gate was not consulted.
type SelectionDecisionTier2 struct {
	Used        bool    `json:"used"`
	Outcome     string  `json:"outcome,omitempty"`
	Model       string  `json:"model,omitempty"`
	LatencyMs   int64   `json:"latency_ms,omitempty"`
	Confidence  float64 `json:"confidence,omitempty"`
	CandidateID string  `json:"candidate_id,omitempty"`
}

// SelectionDecisionRecord is the durable provenance record of one route
// selection (D11, §3.6.4). The raw utterance is never persisted: the record
// carries a SHA-256 digest of the whitespace-normalized lowercase text and the
// candidate-side matched keywords only.
type SelectionDecisionRecord struct {
	Schema          string                       `json:"schema"`
	DecisionID      string                       `json:"decision_id"`
	WorkflowID      string                       `json:"workflow_id"`
	RunID           string                       `json:"run_id,omitempty"`
	UtteranceDigest string                       `json:"utterance_digest"`
	TokenCount      int                          `json:"token_count"`
	Family          string                       `json:"family,omitempty"`
	Candidates      []SelectionDecisionCandidate `json:"candidates"`
	Selected        SelectionDecisionSelected    `json:"selected"`
	DecidedBy       string                       `json:"decided_by"`
	FallbackTaken   bool                         `json:"fallback_taken"`
	Degradation     string                       `json:"degradation,omitempty"`
	Tier2           SelectionDecisionTier2       `json:"tier2"`
	ExecutionState  string                       `json:"execution_state"`
	CreatedAt       time.Time                    `json:"created_at"`
}

// Selection decision execution states (D11). A record is written as
// "dispatched" before route execution begins; the executor transitions it to
// "completed" or "failed" when the route finishes. A crash between leaves
// "dispatched" — truthful.
const (
	SelectionExecutionStateDispatched = "dispatched"
	SelectionExecutionStateCompleted  = "completed"
	SelectionExecutionStateFailed     = "failed"
)

// SelectionDecisionSchema is the canonical schema identifier of the record.
const SelectionDecisionSchema = "relurpify/selection_decision/v1"

// LifecycleRepository is the context-owned interface for workflow/run lifecycle storage.
// execution/agentlifecycle implements it; context/persistence provides adapters.
// Write methods accept a caller context so cancellation and deadlines are honored
// by the underlying durable store.
type LifecycleRepository interface {
	CreateWorkflow(ctx context.Context, record WorkflowRecord) error
	GetWorkflow(workflowID string) (*WorkflowRecord, error)
	ListWorkflows(agentID string) ([]WorkflowRecord, error)

	CreateRun(ctx context.Context, record WorkflowRunRecord) error
	GetRun(runID string) (*WorkflowRunRecord, error)
	ListRuns(workflowID string) ([]WorkflowRunRecord, error)

	UpsertDelegation(ctx context.Context, entry DelegationEntry) error
	GetDelegation(delegationID string) (*DelegationEntry, error)
	ListDelegations(workflowID string) ([]DelegationEntry, error)
	ListDelegationsByRun(runID string) ([]DelegationEntry, error)
	AppendDelegationTransition(ctx context.Context, transition DelegationTransitionEntry) error
	ListDelegationTransitions(delegationID string) ([]DelegationTransitionEntry, error)

	AppendEvent(ctx context.Context, record WorkflowEventRecord) error
	ListEvents(workflowID string, limit int) ([]WorkflowEventRecord, error)
	ListEventsByRun(runID string, limit int) ([]WorkflowEventRecord, error)

	UpsertArtifact(ctx context.Context, record WorkflowArtifactRecord) error
	GetArtifact(artifactID string) (*WorkflowArtifactRecord, error)
	ListArtifacts(workflowID string) ([]WorkflowArtifactRecord, error)
	ListArtifactsByRun(runID string) ([]WorkflowArtifactRecord, error)

	UpsertLineageBinding(ctx context.Context, record LineageBindingRecord) error
	GetLineageBinding(bindingID string) (*LineageBindingRecord, error)
	FindLineageBinding(fromEntityID, toEntityID string) (*LineageBindingRecord, error)
	FindLineageBindingsByFrom(fromEntityID string) ([]LineageBindingRecord, error)
	FindLineageBindingsByTo(toEntityID string) ([]LineageBindingRecord, error)

	// RecordSelectionDecision persists one selection decision record (D11).
	// The record's DecisionID is generated as sel_<workflowID>_<seq> where the
	// sequence is scoped to the workflow (no cross-workflow collision). The
	// record node is linked to its workflow and, when run_id is set, to its run.
	RecordSelectionDecision(ctx context.Context, record SelectionDecisionRecord) (string, error)
	// UpdateSelectionExecutionState transitions a selection record between
	// "dispatched", "completed", and "failed" (D11 crash-window truthfulness).
	UpdateSelectionExecutionState(ctx context.Context, decisionID, executionState string) error
	// ListSelectionDecisions returns the selection decision records of a
	// workflow in creation order (empty workflowID lists every record).
	ListSelectionDecisions(workflowID string, limit int) ([]SelectionDecisionRecord, error)
}

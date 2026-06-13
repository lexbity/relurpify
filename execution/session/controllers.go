package session

import (
	"context"
	"time"
)

// SecurityController provides policy and approval views. Implementation
// is backed by governance-owned types; execution does not define policy
// semantics.
type SecurityController interface {
	PolicySummary(context.Context) (PolicySummary, error)
	RequestApproval(context.Context, ApprovalRequest) (ApprovalDecision, error)
}

// PolicySummary describes the effective security policy for a workspace session.
type PolicySummary struct {
	DefaultPolicy string
	AgentID       string
}

// ApprovalRequest captures a policy decision request.
type ApprovalRequest struct {
	Action  string
	Reason  string
	Timeout time.Duration
}

// ApprovalDecision reports the result of an approval request.
type ApprovalDecision struct {
	Approved bool
	Reason   string
}

// KnowledgeController provides ingest and query access to workspace
// knowledge. Implementation is backed by context/knowledge-owned types.
type KnowledgeController interface {
	Ingest(context.Context, IngestRequest) (IngestResult, error)
	Query(context.Context, QueryRequest) (QueryResult, error)
}

// IngestRequest describes a chunk ingestion operation.
type IngestRequest struct {
	Content  string
	Source   string
	Metadata map[string]string
}

// IngestResult reports the outcome of an ingest operation.
type IngestResult struct {
	ChunksIngested int
}

// QueryRequest describes a knowledge lookup operation.
type QueryRequest struct {
	Query string
	Limit int
}

// QueryResultItem is one result row returned from a knowledge query.
type QueryResultItem struct {
	Content string
	Score   float64
	Source  string
}

// QueryResult contains the result set returned from a knowledge query.
type QueryResult struct {
	Results []QueryResultItem
}

// CapabilityController provides capability catalog listing and invocation.
// Implementation is backed by capability-owned types.
type CapabilityController interface {
	List(context.Context) ([]CapabilitySummary, error)
	Invoke(context.Context, CapabilityInvokeRequest) (CapabilityInvokeResult, error)
}

// CapabilitySummary describes a single capability exposed by the workspace.
type CapabilitySummary struct {
	ID          string
	Name        string
	Description string
	Enabled     bool
}

// CapabilityInvokeRequest captures a capability invocation.
type CapabilityInvokeRequest struct {
	CapabilityID string
	Input        map[string]any
}

// CapabilityInvokeResult reports the outcome of a capability invocation.
type CapabilityInvokeResult struct {
	Output map[string]any
}

// NamedAgentController provides catalog and session opening for named
// agent strategies. Implementation is backed by cognitionzoo and named
// domain packages.
type NamedAgentController interface {
	Catalog(context.Context) ([]NamedAgentSummary, error)
	Open(context.Context, NamedAgentOpenRequest) (NamedAgentSession, error)
}

// NamedAgentSummary describes a named-agent entry in the catalog.
type NamedAgentSummary struct {
	Name        string
	Description string
}

// NamedAgentOpenRequest captures a request to open a named agent session.
type NamedAgentOpenRequest struct {
	Name string
}

// NamedAgentSession identifies an opened named-agent session.
type NamedAgentSession struct {
	ID string
}

// TelemetryView provides read-only access to session telemetry.
type TelemetryView any

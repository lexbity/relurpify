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

type PolicySummary struct {
	DefaultPolicy string
	AgentID       string
}

type ApprovalRequest struct {
	Action  string
	Reason  string
	Timeout time.Duration
}

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

type IngestRequest struct {
	Content  string
	Source   string
	Metadata map[string]string
}

type IngestResult struct {
	ChunksIngested int
}

type QueryRequest struct {
	Query string
	Limit int
}

type QueryResultItem struct {
	Content string
	Score   float64
	Source  string
}

type QueryResult struct {
	Results []QueryResultItem
}

// CapabilityController provides capability catalog listing and invocation.
// Implementation is backed by capability-owned types.
type CapabilityController interface {
	List(context.Context) ([]CapabilitySummary, error)
	Invoke(context.Context, CapabilityInvokeRequest) (CapabilityInvokeResult, error)
}

type CapabilitySummary struct {
	ID          string
	Name        string
	Description string
	Enabled     bool
}

type CapabilityInvokeRequest struct {
	CapabilityID string
	Input        map[string]interface{}
}

type CapabilityInvokeResult struct {
	Output map[string]interface{}
}

// NamedAgentController provides catalog and session opening for named
// agent strategies. Implementation is backed by cognitionzoo and named
// domain packages.
type NamedAgentController interface {
	Catalog(context.Context) ([]NamedAgentSummary, error)
	Open(context.Context, NamedAgentOpenRequest) (NamedAgentSession, error)
}

type NamedAgentSummary struct {
	Name        string
	Description string
}

type NamedAgentOpenRequest struct {
	Name string
}

type NamedAgentSession struct {
	ID string
}

// TelemetryView provides read-only access to session telemetry.
type TelemetryView interface {
	// Observability methods added as domain extraction progresses.
}

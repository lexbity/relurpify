// Package ports defines the consumer ports for the relurpish application.
// This file contains the evidence port for P06.
package ports

import (
	"context"
)

// EvidenceKind identifies the type of evidence.
type EvidenceKind string

const (
	// EvidenceKindPreview indicates deterministic preview resolution.
	EvidenceKindPreview EvidenceKind = "preview"

	// EvidenceKindRuntime indicates actual runtime resolution.
	EvidenceKindRuntime EvidenceKind = "runtime"

	// EvidenceKindActualCall indicates final transport-level evidence.
	EvidenceKindActualCall EvidenceKind = "actual_call"
)

// PromptEvidence contains evidence about prompt resolution and usage.
// This is a TUI-facing version of the internal evidence type.
type PromptEvidence struct {
	// ID is the prompt identifier.
	ID string

	// Kind distinguishes preview, runtime resolution, or actual call.
	Kind EvidenceKind

	// SourceReference identifies the source prompt file or registry entry.
	SourceReference string

	// SourceRevision is the revision/digest of the source at resolution time.
	SourceRevision string

	// ResolvedAt is when resolution occurred.
	ResolvedAt string // ISO 8601 timestamp

	// IncludedBlockCount is the number of blocks included.
	IncludedBlockCount int

	// ExcludedBlockCount is the number of blocks excluded.
	ExcludedBlockCount int

	// OutputLength is the length of the resolved prompt output.
	OutputLength int

	// DurationMS is the resolution duration in milliseconds.
	DurationMS int64

	// Error contains any resolution error.
	Error string

	// CallID links to the logical call that used this prompt, if any.
	CallID string

	// AttemptID links to the physical attempt, if any.
	AttemptID string

	// ProvidersUsed lists the providers used in resolution.
	ProvidersUsed []string

	// CacheHit indicates whether the resolution used a cache.
	CacheHit bool
}

// ModelAttemptEvidence contains evidence about a model call attempt.
type ModelAttemptEvidence struct {
	// CallID is the logical call identifier.
	CallID string

	// AttemptID is the physical attempt identifier.
	AttemptID string

	// Kind is always EvidenceKindActualCall for this type.
	Kind EvidenceKind

	// Model identifies the model used.
	Model string

	// Provider identifies the provider.
	Provider string

	// ProfileRevision is the profile revision used.
	ProfileRevision string

	// PromptID links to the resolved prompt evidence.
	PromptID string

	// InputTokens is the estimated input token count.
	InputTokens int

	// OutputTokens is the estimated output token count.
	OutputTokens int

	// FinishReason is the model's finish reason.
	FinishReason string

	// ToolCallingMode indicates how tools were configured.
	ToolCallingMode string

	// DurationMS is the attempt duration in milliseconds.
	DurationMS int64

	// StartedAt is when the attempt started.
	StartedAt string // ISO 8601 timestamp

	// CompletedAt is when the attempt completed.
	CompletedAt string // ISO 8601 timestamp

	// Error contains any attempt error.
	Error string

	// TransportReclaimed indicates whether the transport was properly reclaimed.
	TransportReclaimed bool

	// PromptPreview is a truncated preview of the prompt (if available and allowed).
	PromptPreview string

	// MessageCount is the number of messages in the chat (for chat-based calls).
	MessageCount int
}

// KnowledgeFlowEvidence contains evidence about knowledge operations.
type KnowledgeFlowEvidence struct {
	// ID identifies this knowledge flow.
	ID string

	// Kind identifies the flow type (retrieve, compile, apply, capture, ground, persist).
	Kind string

	// Direction is "input" or "output".
	Direction string

	// Operation identifies the specific operation (e.g., "retrieve", "ground").
	Operation string

	// ItemsSelected is the number of items selected/retrieved.
	ItemsSelected int

	// BudgetUsed is the knowledge budget consumed.
	BudgetUsed int

	// DurationMS is the operation duration.
	DurationMS int64

	// StartedAt is when the operation started.
	StartedAt string // ISO 8601 timestamp

	// CompletedAt is when the operation completed.
	CompletedAt string // ISO 8601 timestamp

	// Error contains any operation error.
	Error string

	// ChunkID references the chunk if this is a chunk-related operation.
	ChunkID string

	// ContentHash is the content hash for the operation.
	ContentHash string

	// TokenEstimate is the token estimate for the operation.
	TokenEstimate int
}

// CallEvidenceBundle contains all evidence associated with a single call.
type CallEvidenceBundle struct {
	CallID string

	// ModelAttempt is the model attempt evidence.
	ModelAttempt *ModelAttemptEvidence

	// PromptPreview is the preview prompt evidence (if available).
	PromptPreview *PromptEvidence

	// PromptRuntime is the runtime prompt evidence (if available).
	PromptRuntime *PromptEvidence

	// KnowledgeFlows are the knowledge flow evidence associated with this call.
	KnowledgeFlows []*KnowledgeFlowEvidence
}

// EvidenceQuery represents a query for evidence.
type EvidenceQuery struct {
	// CallID filters evidence by call ID.
	CallID string

	// PromptID filters evidence by prompt ID.
	PromptID string

	// OperationID filters evidence by knowledge operation ID.
	OperationID string

	// Kind filters by evidence kind.
	Kind EvidenceKind

	// Limit limits the number of results.
	Limit int

	// IncludeRedacted includes redacted fields (for debugging).
	IncludeRedacted bool
}

// EvidenceResult contains the result of an evidence query.
type EvidenceResult struct {
	// Prompts contains matching prompt evidence.
	Prompts []*PromptEvidence

	// Calls contains matching model attempt evidence.
	Calls []*ModelAttemptEvidence

	// Knowledge contains matching knowledge flow evidence.
	Knowledge []*KnowledgeFlowEvidence

	// Bundles contains complete call bundles.
	Bundles []*CallEvidenceBundle

	// TotalCount is the total number of matching items.
	TotalCount int

	// HasMore indicates if there are more results.
	HasMore bool
}

// EvidencePort is the consumer port for evidence operations.
// It provides read-only access to evidence collected from telemetry.
type EvidencePort interface {
	// GetPromptEvidence retrieves prompt evidence for a given ID and kind.
	GetPromptEvidence(ctx context.Context, promptID string, kind EvidenceKind) (*PromptEvidence, error)

	// GetModelAttemptEvidence retrieves model attempt evidence for a given call ID.
	GetModelAttemptEvidence(ctx context.Context, callID string) (*ModelAttemptEvidence, error)

	// GetKnowledgeFlowEvidence retrieves knowledge flow evidence for a given operation ID.
	GetKnowledgeFlowEvidence(ctx context.Context, operationID string) (*KnowledgeFlowEvidence, error)

	// GetEvidenceByCallID retrieves all evidence associated with a specific call ID.
	GetEvidenceByCallID(ctx context.Context, callID string) (*CallEvidenceBundle, error)

	// QueryEvidence queries evidence based on the provided criteria.
	QueryEvidence(ctx context.Context, query EvidenceQuery) (*EvidenceResult, error)

	// ListPromptEvidence lists all prompt evidence IDs.
	ListPromptEvidence(ctx context.Context) ([]string, error)

	// ListModelAttempts lists all model attempt IDs.
	ListModelAttempts(ctx context.Context) ([]string, error)

	// ListKnowledgeFlows lists all knowledge flow IDs.
	ListKnowledgeFlows(ctx context.Context) ([]string, error)

	// Subscribe registers a channel to receive new evidence events.
	// The channel receives simplified evidence notifications.
	Subscribe(bufferSize int) <-chan EvidenceNotification
}

// EvidenceNotification represents a notification about new evidence.
type EvidenceNotification struct {
	// Kind identifies the evidence type.
	Kind string

	// CallID is the call ID for model attempt evidence.
	CallID string

	// PromptID is the prompt ID for prompt evidence.
	PromptID string

	// OperationID is the operation ID for knowledge flow evidence.
	OperationID string

	// Timestamp is when the evidence was recorded.
	Timestamp string
}

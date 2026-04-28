// Package persistence implements the runtime artifact write path.
// It receives promoted working memory, compiler-produced artifacts, and agent-initiated durability requests.
package persistence

import (
	"time"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/contextpolicy"
	"codeburg.org/lexbit/relurpify/framework/knowledge"
	"codeburg.org/lexbit/relurpify/relurpnet/identity"
)

// PersistenceRequest is the caller-facing write contract.
type PersistenceRequest struct {
	Content            []byte
	ContentType        string              // mime type or structural type
	SourcePrincipal    identity.SubjectRef
	SourceOrigin       knowledge.SourceOrigin
	Reason             string
	Tags               []string
	DerivedFrom        []knowledge.ChunkID // for compiler-produced artifacts
	DerivationMethod   string
	DerivationGeneration int
}

// PersistenceAction indicates the outcome of a persistence operation.
type PersistenceAction string

const (
	ActionCreated       PersistenceAction = "created"
	ActionUpdated       PersistenceAction = "updated"
	ActionDeduplicated  PersistenceAction = "deduplicated"
	ActionQuarantined   PersistenceAction = "quarantined"
	ActionRejected      PersistenceAction = "rejected"
)

// PersistenceResult is the outcome of a persistence operation.
type PersistenceResult struct {
	ChunkID knowledge.ChunkID
	Action  PersistenceAction
	AuditID string
	Error   error // non-nil if operation failed
}

// PersistenceAuditRecord tracks persistence operations for audit purposes.
type PersistenceAuditRecord struct {
	AuditID         string
	Action          PersistenceAction
	ChunkID         knowledge.ChunkID
	SourcePrincipal identity.SubjectRef
	SourceOrigin    knowledge.SourceOrigin
	TrustClass      agentspec.TrustClass
	Reason          string
	PolicyName      string
	CreatedAt       time.Time
}

// Writer is the main entry point for runtime persistence.
type Writer struct {
	Store         *knowledge.ChunkStore
	Events        EventLog
	Policy        *contextpolicy.ContextPolicyBundle
	Evaluator     *contextpolicy.Evaluator
	AuditLog      []PersistenceAuditRecord
}

// EventLog is a minimal event logging interface.
type EventLog interface {
	Emit(eventType string, payload map[string]any)
}

// WorkingMemoryStore interface for promotion operations.
type WorkingMemoryStore interface {
	Get(key string) ([]byte, bool)
	List(prefix string) []string
}

// PromotionRequest represents a request to promote content from working memory.
type PromotionRequest struct {
	Key                string
	ContentType        string
	SourcePrincipal    identity.SubjectRef
	SourceOrigin       knowledge.SourceOrigin
	Reason             string
	Tags               []string
	DerivedFrom        []knowledge.ChunkID
	DerivationMethod   string
	DerivationGeneration int
}

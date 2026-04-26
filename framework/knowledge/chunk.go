package knowledge

import "time"

// ChunkID is a stable artifact identifier.
type ChunkID string

// EdgeID is a stable edge identifier.
type EdgeID string

// FreshnessState tracks semantic validity.
type FreshnessState string

const (
	FreshnessValid      FreshnessState = "valid"
	FreshnessStale      FreshnessState = "stale"
	FreshnessInvalid    FreshnessState = "invalid"
	FreshnessUnverified FreshnessState = "unverified"
)

// CompilerPath distinguishes how a chunk was produced.
type CompilerPath string

const (
	CompilerDeterministic CompilerPath = "deterministic"
	CompilerLLMAssisted   CompilerPath = "llm_assisted"
	CompilerUserDirect    CompilerPath = "user_direct"
)

// ViewKind is a typed chunk projection.
type ViewKind string

const (
	ViewKindPattern    ViewKind = "pattern"
	ViewKindDecision   ViewKind = "decision"
	ViewKindConstraint ViewKind = "constraint"
	ViewKindPlanStep   ViewKind = "plan_step"
	ViewKindAnchor     ViewKind = "anchor"
	ViewKindTension    ViewKind = "tension"
	ViewKindIntent     ViewKind = "intent"
)

// EdgeKind is a typed relation between chunks.
type EdgeKind string

const (
	EdgeKindGrounds            EdgeKind = "grounds"
	EdgeKindContradicts        EdgeKind = "contradicts"
	EdgeKindRefines            EdgeKind = "refines"
	EdgeKindGeneralizes        EdgeKind = "generalizes"
	EdgeKindExemplifies        EdgeKind = "exemplifies"
	EdgeKindDerivesFrom        EdgeKind = "derives_from"
	EdgeKindComposedOf         EdgeKind = "composed_of"
	EdgeKindSupersedes         EdgeKind = "supersedes"
	EdgeKindRequiresContext    EdgeKind = "requires_context"
	EdgeKindAmplifies          EdgeKind = "amplifies"
	EdgeKindInvalidates        EdgeKind = "invalidates"
	EdgeKindDependsOnCodeState EdgeKind = "depends_on_code_state"
	EdgeKindConfirmed          EdgeKind = "confirmed"
	EdgeKindRejected           EdgeKind = "rejected"
	EdgeKindRefinedBy          EdgeKind = "refined_by"
	EdgeKindDeferred           EdgeKind = "deferred"
)

// ProvenanceSource identifies the source artifact behind a chunk or edge.
type ProvenanceSource struct {
	Kind string `json:"kind"`
	Ref  string `json:"ref"`
}

// ChunkProvenance records how semantic state was produced.
type ChunkProvenance struct {
	Sources      []ProvenanceSource `json:"sources"`
	SessionID    string             `json:"session_id,omitempty"`
	WorkflowID   string             `json:"workflow_id,omitempty"`
	CodeStateRef string             `json:"code_state_ref,omitempty"`
	CompiledBy   CompilerPath       `json:"compiled_by"`
	Timestamp    time.Time          `json:"timestamp"`
}

// ChunkBody stores raw and optionally structured knowledge.
type ChunkBody struct {
	Raw    string         `json:"raw"`
	Fields map[string]any `json:"fields,omitempty"`
}

// ChunkView is a typed projection over a chunk body.
type ChunkView struct {
	Kind ViewKind `json:"kind"`
	Data any      `json:"data"`
}

// KnowledgeChunk is the atomic artifact unit.
type KnowledgeChunk struct {
	ID            ChunkID         `json:"id"`
	Version       int             `json:"version"`
	WorkspaceID   string          `json:"workspace_id"`
	ContentHash   string          `json:"content_hash"`
	TokenEstimate int             `json:"token_estimate"`
	Provenance    ChunkProvenance `json:"provenance"`
	Freshness     FreshnessState  `json:"freshness"`
	Body          ChunkBody       `json:"body"`
	Views         []ChunkView     `json:"views,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

// ChunkEdge stores a relationship between chunks.
type ChunkEdge struct {
	ID         EdgeID          `json:"id"`
	FromChunk  ChunkID         `json:"from_chunk"`
	ToChunk    ChunkID         `json:"to_chunk,omitempty"`
	Kind       EdgeKind        `json:"kind"`
	Weight     float64         `json:"weight,omitempty"`
	Meta       map[string]any  `json:"meta,omitempty"`
	Provenance ChunkProvenance `json:"provenance"`
	CreatedAt  time.Time       `json:"created_at"`
}

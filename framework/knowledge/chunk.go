package knowledge

import (
	"time"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/relurpnet/identity"
)

// MemoryClass identifies the lifecycle and storage tier for knowledge chunks.
// This type mirrors core.MemoryClass to avoid an import cycle.
type MemoryClass string

const (
	// MemoryClassWorking indicates the chunk is in working memory (mutable).
	MemoryClassWorking MemoryClass = "working"
	// MemoryClassStreamed indicates the chunk is in streamed context (read-only).
	MemoryClassStreamed MemoryClass = "streamed"
)

// StorageMode identifies how a chunk is stored.
type StorageMode string

const (
	StorageModeInline     StorageMode = "inline"
	StorageModeExternal   StorageMode = "external"
	StorageModeSummarized StorageMode = "summarized"
)

// SourceOrigin identifies the origin of a chunk.
type SourceOrigin string

const (
	SourceOriginUser       SourceOrigin = "user"
	SourceOriginFile       SourceOrigin = "file"
	SourceOriginLLM        SourceOrigin = "llm"
	SourceOriginTool       SourceOrigin = "tool"
	SourceOriginDerivation SourceOrigin = "derivation"
)

// AcquisitionMethod identifies how a chunk was acquired.
type AcquisitionMethod string

const (
	AcquisitionMethodDirect        AcquisitionMethod = "direct"
	AcquisitionMethodIngestion     AcquisitionMethod = "ingestion"
	AcquisitionMethodSummarization AcquisitionMethod = "summarization"
	AcquisitionMethodDerivation    AcquisitionMethod = "derivation"
	AcquisitionMethodRuntimeWrite  AcquisitionMethod = "runtime_write"
)

// DerivationMethod identifies how a derived chunk was created.
type DerivationMethod string

const (
	DerivationMethodSummary        DerivationMethod = "summary"
	DerivationMethodAggregation    DerivationMethod = "aggregation"
	DerivationMethodTransformation DerivationMethod = "transformation"
)

// SuspicionFlags identifies why a chunk is suspected.
type SuspicionFlags string

const (
	SuspicionFlagContentMismatch    SuspicionFlags = "content_mismatch"
	SuspicionFlagProvenanceGap      SuspicionFlags = "provenance_gap"
	SuspicionFlagFreshnessViolation SuspicionFlags = "freshness_violation"
	SuspicionFlagTrustViolation     SuspicionFlags = "trust_violation"
)

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
	// Identity fields
	ID            ChunkID `json:"id"`
	Version       int     `json:"version"`
	WorkspaceID   string  `json:"workspace_id"`
	ContentHash   string  `json:"content_hash"`
	TokenEstimate int     `json:"token_estimate"`

	// Three-axis model fields
	MemoryClass  MemoryClass  `json:"memory_class,omitempty"`
	StorageMode  StorageMode  `json:"storage_mode,omitempty"`
	SourceOrigin SourceOrigin `json:"source_origin,omitempty"`

	// Full provenance fields
	SourcePrincipal      identity.SubjectRef  `json:"source_principal,omitempty"`
	AcquisitionMethod    AcquisitionMethod    `json:"acquisition_method,omitempty"`
	AcquiredAt           time.Time            `json:"acquired_at,omitempty"`
	TrustClass           agentspec.TrustClass `json:"trust_class,omitempty"`
	ContentSchemaVersion string               `json:"content_schema_version,omitempty"`
	DerivedFrom          []ChunkID            `json:"derived_from,omitempty"`
	DerivationMethod     DerivationMethod     `json:"derivation_method,omitempty"`
	DerivationGeneration int                  `json:"derivation_generation,omitempty"`
	CoverageHash         string               `json:"coverage_hash,omitempty"`

	// Tombstoning fields
	Tombstoned   bool    `json:"tombstoned,omitempty"`
	SupersededBy ChunkID `json:"superseded_by,omitempty"`

	// Suspicion fields
	SuspicionScore float64          `json:"suspicion_score,omitempty"`
	SuspicionFlags []SuspicionFlags `json:"suspicion_flags,omitempty"`

	// Legacy fields (kept for backward compatibility)
	Provenance ChunkProvenance `json:"provenance"`
	Freshness  FreshnessState  `json:"freshness"`
	Body       ChunkBody       `json:"body"`
	Views      []ChunkView     `json:"views,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
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

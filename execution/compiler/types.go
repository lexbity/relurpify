// Package compiler provides live context assembly with caching and event-driven invalidation.
//
// Ownership boundaries:
// - CompilationRequest, CompilationResult, CompilationRecord are compiler-owned
// - CacheKey, CacheEntry are compiler-owned
// - SummarySubstitution is compiler-owned
// - Artifact represents compiler-produced artifacts (candidates, replay metadata)
// - Compiler state and context-streaming behavior belong to this package
//
// The compiler depends on framework/persistence for its persistence adapter interface.
// It does NOT depend on agentlifecycle or generic workflow stores.
//
// Compiler persistence is separate from lifecycle persistence. Compiler records
// are owned by the compiler even if persisted through another package's adapter.
package compiler

import (
	"time"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/context/knowledge/retrieval"
)

// ReplayMode determines how compilation replay is performed.
type ReplayMode string

const (
	// StrictReplay re-runs compilation against the same query.
	StrictReplay ReplayMode = "strict"
	// CurrentReplay re-runs compilation against current state.
	CurrentReplay ReplayMode = "current"
)

const (
	// BudgetShortfallPolicyFail aborts the compilation when the token budget
	// cannot be met.
	BudgetShortfallPolicyFail = "fail"
	// BudgetShortfallPolicyEmitPartial emits a partial compilation when the
	// token budget cannot be met.
	BudgetShortfallPolicyEmitPartial = "emit_partial"
	// summaryDerivationSourceOrigin marks chunks whose content was produced by
	// summarization rather than retrieved.
	summaryDerivationSourceOrigin = "summary_derivation"
)

// CompilationRequest represents a request to compile context.
type CompilationRequest struct {
	Query          retrieval.RetrievalQuery
	ManifestID     string
	PolicyBundleID string
	EventLogSeq    uint64
	// EpochID is the memory-generation the compile reflects (owner ruling
	// R-1). It is part of cache identity: an entry can only be served for
	// the epoch it was compiled in, and a new epoch always misses. Epoch 0
	// is reserved for scope-less compiles (no run coordinator), which share
	// a namespace because nothing can ground without one.
	EpochID uint64
	// RunID scopes the compile identity to its owning run (with EpochID):
	// run-local epoch counters are not comparable across runs — and a resumed
	// run restarts its counter — so the cache must never serve an entry
	// across runs regardless of epoch equality.
	RunID                 string
	MaxTokens             int
	BudgetShortfallPolicy string
	Metadata              map[string]any
}

// CompilationResult contains the compiled context.
type CompilationResult struct {
	Chunks             []knowledge.KnowledgeChunk
	RankedChunks       []retrieval.RankedChunk
	SkippedStaleChunks []knowledge.ChunkID
	StreamedRefs       []contextdata.ChunkReference // Populated into Envelope.References.StreamedContext
	Substitutions      []SummarySubstitution
	PinReferences      []PinReference // Reserved-budget reference floor for pinned files
	EvictedPinContent  []string       // Paths of pinned files whose content chunks were evicted by budget
	TotalTokens        int
	ShortfallTokens    int // If > 0, budget could not be met
	ReplayMode         ReplayMode
}

// CompilationRecord captures the full compilation operation for replay and audit.
type CompilationRecord struct {
	RequestID           string
	Timestamp           time.Time
	Request             CompilationRequest
	Result              CompilationResult
	CacheHit            bool
	EventLogSeq         uint64
	RankersUsed         []string
	Dependencies        []knowledge.ChunkID // Chunks used in this compilation
	Substitutions       []SummarySubstitution
	BudgetShortfall     int
	DeterministicDigest string                   // Hash for determinism verification
	AssemblyMetadata    contextdata.AssemblyMeta // Compiler assembly provenance
}

// PinReference captures a bounded reference floor for a pinned file.
// It holds path, content hash, and a short digest so the consumer
// always has a usable handle to the original file regardless of budget.
type PinReference struct {
	Path          string `json:"path"`
	ContentHash   string `json:"content_hash"`
	ShortDigest   string `json:"short_digest"`
	TokenEstimate int    `json:"token_estimate"`
}

// SummarySubstitution records when a chunk was replaced with its summary.
type SummarySubstitution struct {
	OriginalChunkID knowledge.ChunkID
	SummaryChunkID  knowledge.ChunkID
	Reason          string // "budget_pressure" or "policy_directive"
	TokenSavings    int
}

// CompilationDiff shows differences between two compilations.
type CompilationDiff struct {
	AddedChunks             []knowledge.ChunkID
	RemovedChunks           []knowledge.ChunkID
	Reordered               bool
	TokenChange             int
	FreshnessDelta          map[knowledge.ChunkID]knowledge.FreshnessState
	RankerDifferences       []RankerDifference
	FilterDifferences       []FilterDifference
	SubstitutionDifferences []SubstitutionDifference
	DeterminismMatch        bool // true if strict replay produces identical digest
}

// RankerDifference shows differences in ranker output.
type RankerDifference struct {
	RankerID string
	ChunkID  knowledge.ChunkID
	OldScore float64
	NewScore float64
}

// FilterDifference shows differences in filter decisions.
type FilterDifference struct {
	ChunkID     knowledge.ChunkID
	OldDecision string
	NewDecision string
	Reason      string
}

// SubstitutionDifference shows differences in substitutions.
type SubstitutionDifference struct {
	ChunkID       knowledge.ChunkID
	OldSubstitute knowledge.ChunkID
	NewSubstitute knowledge.ChunkID
}

// CacheKey uniquely identifies a compilable context. The key includes the
// envelope epoch (owner ruling R-1, 2026-10-10): cache correctness is
// structural (epoch-scoped), not TTL-based — a cached compilation reflects
// exactly the knowledge state of the epoch it was compiled in, and a new
// epoch always misses. TTL and capacity are memory bounds only. Event-log
// sequence remains record metadata (CompilationRecord.EventLogSeq) and
// digest input, not cache identity.
type CacheKey struct {
	QueryFingerprint        string
	ManifestFingerprint     string
	PolicyBundleFingerprint string
	EpochFingerprint        string
	// RunFingerprint scopes the key to the owning run: run-local epoch
	// counters are not comparable across runs (and a resumed run restarts
	// its counter), so a cached compilation must never be served across
	// runs regardless of epoch equality. Empty fingerprints share the
	// scope-less namespace (no run coordinator ⇒ no barriers, no grounding).
	RunFingerprint string
}

// String returns a string representation of the cache key.
func (k CacheKey) String() string {
	return k.QueryFingerprint + ":" + k.ManifestFingerprint + ":" + k.PolicyBundleFingerprint + ":" + k.EpochFingerprint + ":" + k.RunFingerprint
}

// CacheEntry stores a compiled result with its dependencies.
// This is deliberately specialized (dependency-eviction) and not unified with
// the LRU caches in governance/authorization or the knowledge layer.
type CacheEntry struct {
	Key          CacheKey
	Record       CompilationRecord
	Dependencies map[knowledge.ChunkID]struct{} // Set of chunk IDs used
	CreatedAt    time.Time
	AccessedAt   time.Time
	AccessCount  int
}

// IsValid checks if the cache entry is still valid given invalidated chunks.
func (e *CacheEntry) IsValid(invalidatedChunks map[knowledge.ChunkID]struct{}) bool {
	if e == nil {
		return false
	}
	for chunkID := range e.Dependencies {
		if _, invalidated := invalidatedChunks[chunkID]; invalidated {
			return false
		}
	}
	return true
}

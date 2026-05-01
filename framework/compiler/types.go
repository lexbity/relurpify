// Package compiler provides live context assembly with caching and event-driven invalidation.
//
// Ownership boundaries:
// - CompilationRequest, CompilationResult, CompilationRecord are compiler-owned
// - CacheKey, CacheEntry are compiler-owned
// - SummarySubstitution is compiler-owned
// - CompilerArtifact represents compiler-produced artifacts (candidates, replay metadata)
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

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/knowledge"
	"codeburg.org/lexbit/relurpify/framework/retrieval"
)

// ReplayMode determines how compilation replay is performed.
type ReplayMode string

const (
	// StrictReplay reconstructs knowledge graph state at original EventLogSeq.
	StrictReplay ReplayMode = "strict"
	// CurrentReplay re-runs compilation against current state.
	CurrentReplay ReplayMode = "current"
)

// CompilationRequest represents a request to compile context.
type CompilationRequest struct {
	Query                 retrieval.RetrievalQuery
	ManifestID            string
	PolicyBundleID        string
	EventLogSeq           uint64
	MaxTokens             int
	BudgetShortfallPolicy string // "fail" or "emit_partial"
	Metadata              map[string]any
}

// CompilationResult contains the compiled context.
type CompilationResult struct {
	Chunks             []knowledge.KnowledgeChunk
	RankedChunks       []retrieval.RankedChunk
	SkippedStaleChunks []knowledge.ChunkID
	StreamedRefs       []contextdata.ChunkReference // Populated into Envelope.References.StreamedContext
	Substitutions      []SummarySubstitution
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

// CacheKey uniquely identifies a compilable context.
type CacheKey struct {
	QueryFingerprint        string
	ManifestFingerprint     string
	PolicyBundleFingerprint string
	EventLogSeq             uint64
}

// String returns a string representation of the cache key.
func (k CacheKey) String() string {
	return k.QueryFingerprint + ":" + k.ManifestFingerprint + ":" + k.PolicyBundleFingerprint
}

// CacheEntry stores a compiled result with its dependencies.
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

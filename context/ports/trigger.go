// Package ports defines consumer-owned interfaces for cross-domain
// communication. Context owns these ports so context never needs
// to import execution directly for compiler trigger or lifecycle.
package ports

import "context"

// CompilationRequest is the context-owned view of a compilation request.
type CompilationRequest struct {
	SessionID    string
	BaseContext  string
	BudgetTokens int
	Mode         string
	Priority     int
	// EventLogSeq and Metadata round-trip the caller's request attributes so
	// compilation records stay attributable to their creating run.
	EventLogSeq uint64
	Metadata    map[string]any
}

// CompilationResult is the context-owned view of a compilation result.
type CompilationResult struct {
	// StreamedChunks is the ordered, body-carrying slice the backward pass
	// delivers: one entry per streamed chunk in compiler rank order, bodies
	// verbatim, summary-substituted entries flagged and carrying the summary
	// body (D-1). It is the only content channel; there is no flat string.
	StreamedChunks     []StreamedChunkView
	ShortfallTokens    int
	StreamedRefs       []string
	SkippedStaleChunks []string
	Substitutions      []SummarySubstitution
	Record             CompilationRecord
}

// StreamedChunkView is one body-carrying entry of a compilation result.
type StreamedChunkView struct {
	ChunkID       string
	ContentHash   string
	Body          string // verbatim chunk content, as ranked by the compiler
	TokenEstimate int
	TrustClass    string // agentspec trust class label; "" when unset
	IsSummary     bool   // summary-substituted chunk
}

// CompilationRecord captures metadata about a compilation.
type CompilationRecord struct {
	ID              string
	OriginalBudget  int
	FinalTokens     int
	CompressionRate float64
	Error           string
	// CacheHit reports whether the result was served from the compiler's
	// in-memory cache. It is the signal the contextstream layer uses to emit
	// compiler.cache_hit / compiler.cache_miss telemetry (FR-12).
	CacheHit bool
}

// SummarySubstitution records a text replacement made during compilation.
type SummarySubstitution struct {
	Original string
	Replaced string
	ChunkID  string
}

// CompilerTrigger is the context-owned interface for triggering
// compilations. Execution/compiler implements it.
type CompilerTrigger interface {
	Compile(ctx context.Context, req CompilationRequest) (*CompilationResult, error)
}

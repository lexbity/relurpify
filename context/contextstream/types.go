package contextstream

import (
	"context"
	"time"

	"codeburg.org/lexbit/relurpify/context/knowledge/retrieval"
	contextports "codeburg.org/lexbit/relurpify/context/ports"
)

// Mode determines whether a streaming request blocks or runs in the background.
type Mode string

const (
	ModeBlocking   Mode = "blocking"
	ModeBackground Mode = "background"
)

// Request describes a compiler-triggered context streaming request.
type Request struct {
	ID          string
	Query       retrieval.RetrievalQuery
	MaxTokens   int
	EventLogSeq uint64
	// EpochID is the envelope's memory-generation at request-build time
	// (owner ruling R-1). It rides the request into the compiler's cache
	// identity; builders stamp it from AssemblyMetadataSnapshot().EpochID.
	EpochID uint64
	// RunID is the owning run's identity (with EpochID, part of the compile
	// identity): run-local epoch counters are not comparable across runs, so
	// the cache must never serve an entry across runs. Builders stamp it from
	// AssemblyMetadataSnapshot().RunID (seeded by the epoch coordinator);
	// empty only for scope-less compiles with no run coordinator.
	RunID                 string
	BudgetShortfallPolicy string
	Mode                  Mode
	Metadata              map[string]any
	RequestedAt           time.Time
}

// TrimMetadata captures how the compiler trimmed a streamed result.
type TrimMetadata struct {
	BudgetTokens    int
	ShortfallTokens int
	Substitutions   []contextports.SummarySubstitution
}

// Result is the orchestrator-facing outcome of a context streaming request.
type Result struct {
	Request     Request
	Compilation *contextports.CompilationResult
	Record      *contextports.CompilationRecord
	Trim        TrimMetadata
	StartedAt   time.Time
	CompletedAt time.Time
	Err         error
	Applied     bool
}

// CompilerInvoker is the narrow compiler contract used by the streaming layer.
type CompilerInvoker interface {
	Compile(ctx context.Context, request contextports.CompilationRequest) (*contextports.CompilationResult, error)
}

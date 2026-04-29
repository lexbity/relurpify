package contextstream

import (
	"context"
	"time"

	"codeburg.org/lexbit/relurpify/framework/compiler"
	"codeburg.org/lexbit/relurpify/framework/retrieval"
)

// Mode determines whether a streaming request blocks or runs in the background.
type Mode string

const (
	ModeBlocking   Mode = "blocking"
	ModeBackground Mode = "background"
)

// Request describes a compiler-triggered context streaming request.
type Request struct {
	ID                    string
	Query                 retrieval.RetrievalQuery
	MaxTokens             int
	EventLogSeq           uint64
	BudgetShortfallPolicy string
	Mode                  Mode
	Metadata              map[string]any
	RequestedAt           time.Time
}

// TrimMetadata captures how the compiler trimmed a streamed result.
type TrimMetadata struct {
	BudgetTokens    int
	ShortfallTokens int
	Substitutions   []compiler.SummarySubstitution
	Truncated       bool
}

// Result is the orchestrator-facing outcome of a context streaming request.
type Result struct {
	Request     Request
	Compilation *compiler.CompilationResult
	Record      *compiler.CompilationRecord
	Trim        TrimMetadata
	StartedAt   time.Time
	CompletedAt time.Time
	Err         error
	Applied     bool
}

// CompilerInvoker is the narrow compiler contract used by the streaming layer.
type CompilerInvoker interface {
	Compile(ctx context.Context, request compiler.CompilationRequest) (*compiler.CompilationResult, *compiler.CompilationRecord, error)
}

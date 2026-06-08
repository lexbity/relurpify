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
}

// CompilationResult is the context-owned view of a compilation result.
type CompilationResult struct {
	Context            string
	ShortfallTokens    int
	StreamedRefs       []string
	SkippedStaleChunks []string
	Substitutions      []SummarySubstitution
	Record             CompilationRecord
}

// CompilationRecord captures metadata about a compilation.
type CompilationRecord struct {
	ID              string
	OriginalBudget  int
	FinalTokens     int
	CompressionRate float64
	Error           string
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

package agentgraph

import (
	"context"

	"codeburg.org/lexbit/relurpify/framework/compiler"
)

// CompilationTrigger is the interface used by graph nodes to trigger context compilation.
// The compiler implements this interface to provide on-demand context assembly.
type CompilationTrigger interface {
	// Compile assembles context for the given request.
	Compile(ctx context.Context, request compiler.CompilationRequest) (*compiler.CompilationResult, *compiler.CompilationRecord, error)

	// Replay re-runs a compilation for verification.
	Replay(ctx context.Context, originalRecord compiler.CompilationRecord, mode compiler.ReplayMode) (*compiler.CompilationResult, *compiler.CompilationDiff, error)
}

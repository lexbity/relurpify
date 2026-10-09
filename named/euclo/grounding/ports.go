// Package grounding declares the ports Euclo consumes from the knowledge
// layer's capture-bridge (Wave 1, "Capture-as-Bridge and Context Epochs").
// Consumer-owned per the P-PORT pattern (devdocs/ref-old/domain-dag.md):
// named/euclo declares the narrow interface; context/knowledge implements
// the behavior; app/envcomposition adapts and injects.
//
// This file is a shared contract between the Wave 1 and Wave 2
// specifications (Wave 1 Appendix C §C.2; Wave 2 Appendix A): both carry it
// verbatim. The first branch to need it creates it byte-identically; the
// merge keeps exactly one copy. No other file is shared between the
// branches.
package grounding

import (
	"context"
	"time"
)

// RegroundEntry is one durable state value restored at recipe restart.
// Vocabulary is Wave 1's (epistemics claimed|given; origin user|tool|llm);
// restore re-enters through the capture-application path so capture and
// restore share one typed code path.
type RegroundEntry struct {
	StateKey    string // target key, e.g. "state.findings"
	Value       any    // in-memory envelope value (decoded from the grounded chunk)
	Epistemics  string // "claimed" | "given"
	Origin      string // "user" | "tool" | "llm"
	ChunkID     string // "chunk:<kind>:<hash16>" provenance pointer
	SourceRunID string // run that grounded it
	GroundedAt  time.Time
}

// RegroundRequest scopes a restart query. SessionID empty means workspace
// scope. MaxEntries 0 means the implementation default (64).
type RegroundRequest struct {
	WorkspaceID string
	SessionID   string
	RecipeID    string
	MaxEntries  int
}

// RegroundResult is the ordered restore payload. Grounded=false with a nil
// error is a normal cold start, never an error.
type RegroundResult struct {
	Entries     []RegroundEntry // oldest → newest; final capture per state key
	SourceRunID string
	Grounded    bool
}

// StateRegroundSource is the read-only restart query over the grounded
// capture corpus (implemented by the knowledge layer, Wave 1; consumed at
// recipe dispatch, Wave 2).
type StateRegroundSource interface {
	// Reground returns the durable state.* captures of the most recent
	// prior grounded run of (WorkspaceID, RecipeID) within SessionID (when
	// non-empty). Implementations MUST be read-only, MUST honor context
	// cancellation, and MUST bound work by MaxEntries.
	Reground(ctx context.Context, req RegroundRequest) (RegroundResult, error)
}

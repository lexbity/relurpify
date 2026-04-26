package agentspec

// ASTSymbolSummary is a compact representation of an AST symbol entry
// suitable for injection as a blackboard fact or artifact chunk.
type ASTSymbolSummary struct {
	Name       string
	Kind       string // "function" | "type" | "method" | "field" | "const" | etc.
	File       string
	Line       int
	Signature  string
	DocSummary string
}

// AgentSemanticContext is the pre-resolved semantic artifact bundle
// passed to executors before execution begins. All fields are optional;
// an empty bundle is valid and represents a cold-start with no semantic
// preloading.
//
// This type carries resolved content, not references. It is assembled
// by the session trigger and ExecutorFactory upstream of executor
// construction. Executors must not perform their own assembly of this
// bundle at runtime.
type AgentSemanticContext struct {
	// Chunks is the ordered sequence of BKC knowledge chunks for this
	// session, as emitted by the BKC backward pass. Order is
	// dependency-first.
	Chunks []AgentContextChunk

	// ASTSymbols is a pre-resolved set of AST symbol summaries for the
	// task scope. Populated from IndexManager before executor
	// construction.
	ASTSymbols []ASTSymbolSummary

	// TokenBudgetUsed is the token total consumed by Chunks, for budget
	// accounting by the streaming policy.
	TokenBudgetUsed int

	// WorkspaceID and WorkflowID are carried for provenance and for
	// downstream BKC operations (e.g. invalidation after mutation).
	WorkspaceID string
	WorkflowID  string

	// CodeRevision is the git SHA at which this bundle was assembled.
	// Used for chunk staleness validation.
	CodeRevision string
}

// AgentContextChunk is a framework-generic semantic chunk payload
// suitable for injection into agent runtime state.
type AgentContextChunk struct {
	ID            string
	Content       string
	TokenEstimate int
	Metadata      map[string]string
}

// IsEmpty returns true when the bundle contains no pre-resolved content.
func (e AgentSemanticContext) IsEmpty() bool {
	return len(e.Chunks) == 0 &&
		len(e.ASTSymbols) == 0 &&
		e.TokenBudgetUsed == 0 &&
		e.WorkspaceID == "" &&
		e.WorkflowID == "" &&
		e.CodeRevision == ""
}

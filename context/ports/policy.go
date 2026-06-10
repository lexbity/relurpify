package ports

// PolicyBundle is the context-owned view of a context policy bundle.
// Execution/context provides the implementation that wraps this type.
type PolicyBundle struct {
	DefaultTrustClass      string
	MaxTokensPerWindow     int
	MaxTokensPerChunk      int
	LowWatermarkTokens     int
	DegradedChunkPolicy    string
	MaxTraversalCandidates int // 0 ⇒ retriever uses its hardcoded default
}

// PolicyEvaluator is the context-owned interface for evaluating
// context policies. execution/context implements it.
type PolicyEvaluator interface {
	AdmitTrustClass(trustClass string) bool
	QuotaRemaining(principal string) int
}

package agentgraph

// ContextSnapshot captures a point-in-time view of context state for graph snapshots.
// This is a simple wrapper around the working memory snapshot from an envelope.
type ContextSnapshot struct {
	State map[string]any
}

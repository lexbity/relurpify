package agentgraph

import (
	"context"

	"codeburg.org/lexbit/relurpify/context/knowledge"
)

// CaptureSink receives grounding items produced by recipe capture execution.
// The run's EpochCoordinator implements it (Phase 5); paradigm-internal unit
// tests use a recording implementation. When no sink is present in the node
// context the item is dropped with an explicit event — never silently.
type CaptureSink interface {
	EnqueueCapture(item knowledge.GroundingItem)
}

type captureSinkContextKey struct{}

// WithCaptureSink attaches a capture sink to a context for the node execution.
func WithCaptureSink(ctx context.Context, sink CaptureSink) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, captureSinkContextKey{}, sink)
}

// CaptureSinkFromContext extracts the capture sink, if any, from a context.
func CaptureSinkFromContext(ctx context.Context) CaptureSink {
	if ctx == nil {
		return nil
	}
	sink, _ := ctx.Value(captureSinkContextKey{}).(CaptureSink)
	return sink
}

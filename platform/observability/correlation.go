package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

type traceContextKey struct{}
type runContextKey struct{}
type nodeContextKey struct{}

// TraceContext carries the active trace and span identifiers through context.
type TraceContext struct {
	TraceID string
	SpanID  string
}

// RunContext carries the turn-scoped correlation identifiers through context.
type RunContext struct {
	SessionID string
	RunID     string
	TraceID   string
	AgentID   string
}

// Correlation is the resolved correlation field set for an emission site.
type Correlation struct {
	SessionID string
	RunID     string
	TraceID   string
	AgentID   string
	NodeID    string
	SpanID    string
	// TraceIDTurnScoped reports whether TraceID came from the turn-scoped
	// RunContext (authoritative, overwrites) rather than the node-scoped
	// TraceContext (fallback, never downgrades an event's TraceID).
	TraceIDTurnScoped bool
}

// WithTraceContext stores trace context in the given context.
func WithTraceContext(ctx context.Context, tc TraceContext) context.Context {
	return context.WithValue(ctx, traceContextKey{}, tc)
}

// TraceContextFromContext extracts trace context, returning zero value when absent.
func TraceContextFromContext(ctx context.Context) (TraceContext, bool) {
	if ctx == nil {
		return TraceContext{}, false
	}
	tc, ok := ctx.Value(traceContextKey{}).(TraceContext)
	return tc, ok
}

// WithRunContext stores run context in the given context.
func WithRunContext(ctx context.Context, rc RunContext) context.Context {
	return context.WithValue(ctx, runContextKey{}, rc)
}

// RunContextFromContext extracts run context, returning zero value when absent.
func RunContextFromContext(ctx context.Context) (RunContext, bool) {
	if ctx == nil {
		return RunContext{}, false
	}
	rc, ok := ctx.Value(runContextKey{}).(RunContext)
	return rc, ok
}

// WithNodeContext stores the ID of the node currently executing in the
// given context. The graph runtime stamps the active node before invoking
// it, so every downstream emitter — including the LLM instrumentation,
// which cannot import the execution packages — can attribute its events
// to the node that caused them.
func WithNodeContext(ctx context.Context, nodeID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, nodeContextKey{}, nodeID)
}

// NodeIDFromContext extracts the executing node's ID, returning false
// when no node context is present.
func NodeIDFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	nodeID, ok := ctx.Value(nodeContextKey{}).(string)
	return nodeID, ok
}

// HasTraceID reports whether a TraceID was resolved from any context carrier.
func (c Correlation) HasTraceID() bool { return c.TraceID != "" }

// CorrelationFromContext resolves all correlation identifiers carried by ctx.
// It is the single source of truth for merge semantics; every stamper
// (telemetry.StampCorrelation and layer-specific stamps downstream) consumes
// it so merge rules cannot drift between event types.
func CorrelationFromContext(ctx context.Context) Correlation {
	var c Correlation
	if rc, ok := RunContextFromContext(ctx); ok {
		c.SessionID = rc.SessionID
		c.RunID = rc.RunID
		c.AgentID = rc.AgentID
		if rc.TraceID != "" {
			c.TraceID = rc.TraceID
			c.TraceIDTurnScoped = true
		}
	}
	if tc, ok := TraceContextFromContext(ctx); ok {
		c.SpanID = tc.SpanID
		if c.TraceID == "" {
			c.TraceID = tc.TraceID
		}
	}
	if nodeID, ok := NodeIDFromContext(ctx); ok {
		c.NodeID = nodeID
	}
	return c
}

// StampCorrelation populates the correlation fields on ev from the
// correlation identifiers carried by ctx. Merge semantics come from
// CorrelationFromContext (single source of truth): a non-empty RunContext
// value overwrites the corresponding field, an empty one never clears a
// field the caller already set, and TraceID is turn-scoped — a RunContext
// TraceID wins over any node-local TraceContext, whose TraceID is only a
// fallback. Emitters must never construct correlation fields by hand.
func StampCorrelation(ctx context.Context, ev *Event) {
	if ev == nil {
		return
	}
	c := CorrelationFromContext(ctx)
	if c.SessionID != "" {
		ev.SessionID = c.SessionID
	}
	if c.RunID != "" {
		ev.RunID = c.RunID
	}
	if c.TraceIDTurnScoped {
		ev.TraceID = c.TraceID
	} else if c.TraceID != "" && ev.TraceID == "" {
		ev.TraceID = c.TraceID
	}
	if c.AgentID != "" {
		ev.AgentID = c.AgentID
	}
	if c.SpanID != "" {
		ev.SpanID = c.SpanID
	}
	if c.NodeID != "" && ev.NodeID == "" {
		ev.NodeID = c.NodeID
	}
}

// NewTraceID generates a random trace ID.
func NewTraceID() string {
	return generateID(16) // 16 bytes = 32 hex chars
}

// NewSpanID generates a random span ID.
func NewSpanID() string {
	return generateID(8) // 8 bytes = 16 hex chars
}

// NewRunID generates a random run (turn) identifier.
func NewRunID() string {
	return generateID(12) // 12 bytes = 24 hex chars
}

// NewSessionID generates a random session identifier.
func NewSessionID() string {
	return generateID(12) // 12 bytes = 24 hex chars
}

func generateID(bytes int) string {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		// Fallback: use a timestamp-based ID if crypto/rand fails
		return fmt.Sprintf("gen_%x", b)
	}
	return hex.EncodeToString(b)
}

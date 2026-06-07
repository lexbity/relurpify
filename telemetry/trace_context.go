package telemetry

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

type traceContextKey struct{}

// TraceContext carries the active trace and span identifiers through context.
type TraceContext struct {
	TraceID string
	SpanID  string
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

// NewTraceID generates a random trace ID.
func NewTraceID() string {
	return generateID(16) // 16 bytes = 32 hex chars
}

// NewSpanID generates a random span ID.
func NewSpanID() string {
	return generateID(8) // 8 bytes = 16 hex chars
}

func generateID(bytes int) string {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		// Fallback: use a timestamp-based ID if crypto/rand fails
		return fmt.Sprintf("gen_%x", b)
	}
	return hex.EncodeToString(b)
}

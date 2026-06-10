package telemetry

import (
	"fmt"
	"strconv"
	"time"
)

// ToolSpanExporter converts tool call/result telemetry events into
// structured spans. When attached to a Telemetry sink, it intercepts
// EventToolCall and EventToolResult events and produces SpanEvents that
// can be consumed by a SpanExporter backend (OTel adapter, JSONL, etc.).
type ToolSpanExporter struct {
	next    Telemetry           // chain to next sink
	spans   SpanExporter        // span backend (nil = no-op)
	attrs   map[string]struct{} // allowlisted extra attribute keys
	agentID string
}

// ToolSpanExporterOption configures the exporter.
type ToolSpanExporterOption func(*ToolSpanExporter)

// WithSpanExporter sets the span backend for the exporter.
func WithSpanExporter(exporter SpanExporter) ToolSpanExporterOption {
	return func(e *ToolSpanExporter) { e.spans = exporter }
}

// WithExtraAttributes sets the allowlist of param names whose values
// are safe to emit as span attributes.
func WithExtraAttributes(attrs []string) ToolSpanExporterOption {
	return func(e *ToolSpanExporter) {
		e.attrs = make(map[string]struct{}, len(attrs))
		for _, a := range attrs {
			e.attrs[a] = struct{}{}
		}
	}
}

// NewToolSpanExporter wraps a Telemetry sink with tool span export.
func NewToolSpanExporter(next Telemetry, opts ...ToolSpanExporterOption) *ToolSpanExporter {
	e := &ToolSpanExporter{next: next, spans: NopExporter{}}
	for _, o := range opts {
		o(e)
	}
	return e
}

// Emit implements Telemetry. It forwards the event to the next sink
// and, for tool events, also generates a span via the configured exporter.
func (e *ToolSpanExporter) Emit(event Event) {
	if e.next != nil {
		e.next.Emit(event)
	}
	if e.spans == nil {
		return
	}

	switch event.Type {
	case EventToolResult:
		e.emitToolSpan(event)
	case EventToolCall:
		e.emitToolCallSpan(event)
	}
}

func (e *ToolSpanExporter) emitToolSpan(event Event) {
	attrs := e.spanAttrsFromEvent(event)
	spanCtx := spanContextFromEvent(event)
	parentCtx := parentSpanContextFromEvent(event)
	duration := durationFromEvent(event)
	status := spanStatus(event)

	e.spans.ExportSpan(
		nameFromEvent(event, "tool"),
		attrs,
		spanCtx,
		parentCtx,
		duration,
		status,
	)
}

func (e *ToolSpanExporter) emitToolCallSpan(event Event) {
	// ToolCall events are the span start — the span is completed on ToolResult.
	// For now, we emit a brief span for the call itself.
	attrs := e.spanAttrsFromEvent(event)
	spanCtx := spanContextFromEvent(event)
	parentCtx := parentSpanContextFromEvent(event)

	e.spans.ExportSpan(
		nameFromEvent(event, "tool.call"),
		attrs,
		spanCtx,
		parentCtx,
		0,
		"started",
	)
}

// spanAttrsFromEvent extracts span attributes from the event metadata's
// "span_attrs" key, merging in runtime values from the event top level.
func (e *ToolSpanExporter) spanAttrsFromEvent(event Event) SpanAttributes {
	attrs := SpanAttributes{}

	if raw, ok := event.Metadata["span_attrs"]; ok {
		if m, ok := raw.(map[string]any); ok {
			for k, v := range m {
				attrs[k] = fmt.Sprint(v)
			}
		}
	}

	// Merge runtime attributes from event metadata
	if exitCode, ok := event.Metadata["exit_code"]; ok {
		attrs["exit_code"] = fmt.Sprint(exitCode)
	}
	if stdoutBytes, ok := event.Metadata["stdout_bytes"]; ok {
		attrs["stdout_bytes"] = fmt.Sprint(stdoutBytes)
	}
	if artifactRef, ok := event.Metadata["artifact_ref"]; ok {
		attrs["artifact_ref"] = fmt.Sprint(artifactRef)
	}
	if durationMs, ok := event.Metadata["duration_ms"]; ok {
		attrs["elapsed_ms"] = fmt.Sprint(durationMs)
	}
	if success, ok := event.Metadata["success"]; ok {
		attrs["success"] = fmt.Sprint(success)
	}
	if toolErr, ok := event.Metadata["tool_error"]; ok {
		attrs["error"] = fmt.Sprint(toolErr)
	}

	// Extra attributes from allowlist
	if len(e.attrs) > 0 {
		if argsRaw, ok := event.Metadata["args"]; ok {
			if argsMap, ok := argsRaw.(map[string]any); ok {
				for k, v := range argsMap {
					if _, allowed := e.attrs[k]; allowed {
						attrs["param."+k] = fmt.Sprint(v)
					}
				}
			}
		}
	}

	return attrs
}

func nameFromEvent(event Event, fallback string) string {
	if event.Message != "" {
		return event.Message
	}
	if tool, ok := event.Metadata["tool"]; ok {
		return fmt.Sprintf("tool %s", tool)
	}
	return fallback
}

func spanContextFromEvent(event Event) SpanContext {
	return SpanContext{
		TraceID: stringField(event.Metadata, "trace_id"),
		SpanID:  stringField(event.Metadata, "span_id"),
	}
}

func parentSpanContextFromEvent(event Event) SpanContext {
	return SpanContext{
		TraceID: stringField(event.Metadata, "parent_trace_id"),
		SpanID:  stringField(event.Metadata, "parent_span_id"),
	}
}

func durationFromEvent(event Event) time.Duration {
	if raw, ok := event.Metadata["duration_ms"]; ok {
		switch v := raw.(type) {
		case float64:
			return time.Duration(v) * time.Millisecond
		case int64:
			return time.Duration(v) * time.Millisecond
		case int:
			return time.Duration(v) * time.Millisecond
		case string:
			if ms, err := strconv.ParseInt(v, 10, 64); err == nil {
				return time.Duration(ms) * time.Millisecond
			}
		}
	}
	return 0
}

func spanStatus(event Event) string {
	if success, ok := event.Metadata["success"]; ok {
		if b, ok := success.(bool); ok && b {
			return "ok"
		}
		return "error"
	}
	if _, ok := event.Metadata["error"]; ok {
		return "error"
	}
	if _, ok := event.Metadata["tool_error"]; ok {
		return "error"
	}
	return "ok"
}

func stringField(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if raw, ok := m[key]; ok {
		if s, ok := raw.(string); ok {
			return s
		}
		return fmt.Sprint(raw)
	}
	return ""
}

// InMemoryExporter records all exported spans in memory for testing.
type InMemoryExporter struct {
	Spans []SpanEvent
}

func (e *InMemoryExporter) ExportSpan(name string, attrs SpanAttributes, spanCtx SpanContext, parentCtx SpanContext, duration time.Duration, status string) {
	e.Spans = append(e.Spans, SpanEvent{
		Name:       name,
		Attributes: attrs,
		SpanCtx:    spanCtx,
		ParentCtx:  parentCtx,
		Duration:   duration,
		Status:     status,
	})
}

// Ensure compile-time interface satisfaction.
var _ Telemetry = (*ToolSpanExporter)(nil)

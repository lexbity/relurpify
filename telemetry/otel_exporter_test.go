package telemetry

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestToolSpanExporterEmitsSpanOnToolResult(t *testing.T) {
	mem := &InMemoryExporter{}
	exporter := NewToolSpanExporter(nil, WithSpanExporter(mem))

	exporter.Emit(Event{
		Type:    EventToolResult,
		Message: "tool cli_jq completed",
		Metadata: map[string]any{
			"tool":         "cli_jq",
			"span_attrs":   map[string]any{"tool.name": "cli_jq", "tool.family": "text"},
			"exit_code":    0,
			"stdout_bytes": int64(42),
			"duration_ms":  int64(150),
			"success":      true,
		},
	})

	require.Len(t, mem.Spans, 1)
	span := mem.Spans[0]
	require.Equal(t, "tool cli_jq completed", span.Name)
	require.Equal(t, "ok", span.Status)
	require.Equal(t, "cli_jq", span.Attributes["tool.name"])
	require.Equal(t, "text", span.Attributes["tool.family"])
	require.Equal(t, "42", span.Attributes["stdout_bytes"])
	require.Equal(t, "150", span.Attributes["elapsed_ms"])
}

func TestToolSpanExporterEmitsSpanOnToolCall(t *testing.T) {
	mem := &InMemoryExporter{}
	exporter := NewToolSpanExporter(nil, WithSpanExporter(mem))

	exporter.Emit(Event{
		Type:    EventToolCall,
		Message: "tool cli_rg invoked",
		Metadata: map[string]any{
			"tool":       "cli_rg",
			"span_attrs": map[string]any{"tool.name": "cli_rg", "tool.family": "fileops"},
		},
	})

	require.Len(t, mem.Spans, 1)
	require.Equal(t, "tool cli_rg invoked", mem.Spans[0].Name)
	require.Equal(t, "started", mem.Spans[0].Status)
}

func TestToolSpanExporterSkipsNonToolEvents(t *testing.T) {
	mem := &InMemoryExporter{}
	exporter := NewToolSpanExporter(nil, WithSpanExporter(mem))

	exporter.Emit(Event{Type: EventGraphStart})
	exporter.Emit(Event{Type: EventLLMPrompt})
	exporter.Emit(Event{Type: EventStateChange})

	require.Empty(t, mem.Spans, "non-tool events must not generate spans")
}

func TestToolSpanExporterNopWhenNoSpanBackend(t *testing.T) {
	// When no WithSpanExporter is provided, the NopExporter is used — no crash
	exporter := NewToolSpanExporter(nil)
	exporter.Emit(Event{
		Type: EventToolResult,
		Metadata: map[string]any{
			"tool":       "cli_echo",
			"span_attrs": map[string]any{},
		},
	})
	// No assertion needed — just verify no panic
}

func TestToolSpanExporterErrorStatus(t *testing.T) {
	mem := &InMemoryExporter{}
	exporter := NewToolSpanExporter(nil, WithSpanExporter(mem))

	exporter.Emit(Event{
		Type: EventToolResult,
		Metadata: map[string]any{
			"tool":       "cli_fail",
			"success":    false,
			"tool_error": "exit code 1",
			"span_attrs": map[string]any{"tool.name": "cli_fail"},
		},
	})

	require.Len(t, mem.Spans, 1)
	require.Equal(t, "error", mem.Spans[0].Status)
	require.Contains(t, mem.Spans[0].Attributes["error"], "exit code 1")
}

func TestToolSpanExporterExtraAttributes(t *testing.T) {
	mem := &InMemoryExporter{}
	exporter := NewToolSpanExporter(nil,
		WithSpanExporter(mem),
		WithExtraAttributes([]string{"pattern"}),
	)

	exporter.Emit(Event{
		Type: EventToolResult,
		Metadata: map[string]any{
			"tool":       "cli_rg",
			"span_attrs": map[string]any{"tool.name": "cli_rg"},
			"args": map[string]any{
				"pattern":  "func",
				"secret":   "should-not-leak",
				"password": "hunter2",
			},
		},
	})

	require.Len(t, mem.Spans, 1)
	require.Equal(t, "func", mem.Spans[0].Attributes["param.pattern"], "allowlisted param must appear")
	require.NotContains(t, mem.Spans[0].Attributes, "param.secret", "non-allowlisted param must not leak")
	require.NotContains(t, mem.Spans[0].Attributes, "param.password", "non-allowlisted param must not leak")
}

func TestToolSpanExporterDuration(t *testing.T) {
	mem := &InMemoryExporter{}
	exporter := NewToolSpanExporter(nil, WithSpanExporter(mem))

	exporter.Emit(Event{
		Type: EventToolResult,
		Metadata: map[string]any{
			"tool":        "cli_echo",
			"duration_ms": float64(2500),
			"span_attrs":  map[string]any{"tool.name": "cli_echo"},
		},
	})

	require.Len(t, mem.Spans, 1)
	require.Equal(t, 2500*time.Millisecond, mem.Spans[0].Duration)
}

func TestInMemoryExporterRecordsMultipleSpans(t *testing.T) {
	mem := &InMemoryExporter{}
	exporter := NewToolSpanExporter(nil, WithSpanExporter(mem))

	for i := 0; i < 3; i++ {
		exporter.Emit(Event{
			Type: EventToolResult,
			Metadata: map[string]any{
				"tool":       "tool",
				"span_attrs": map[string]any{},
			},
		})
	}

	require.Len(t, mem.Spans, 3)
}

func TestSpanContextPassedToExporter(t *testing.T) {
	mem := &InMemoryExporter{}
	exporter := NewToolSpanExporter(nil, WithSpanExporter(mem))

	exporter.Emit(Event{
		Type: EventToolResult,
		Metadata: map[string]any{
			"tool":            "cli_curl",
			"trace_id":        "abc123",
			"span_id":         "def456",
			"parent_trace_id": "parent_abc",
			"parent_span_id":  "parent_def",
			"span_attrs":      map[string]any{},
		},
	})

	require.Len(t, mem.Spans, 1)
	require.Equal(t, "abc123", mem.Spans[0].SpanCtx.TraceID)
	require.Equal(t, "def456", mem.Spans[0].SpanCtx.SpanID)
	require.Equal(t, "parent_abc", mem.Spans[0].ParentCtx.TraceID)
	require.Equal(t, "parent_def", mem.Spans[0].ParentCtx.SpanID)
}

type recordingTelemetry struct {
	events []Event
}

func (r *recordingTelemetry) Emit(event Event) {
	r.events = append(r.events, event)
}

func TestToolSpanExporterForwardsToNextSink(t *testing.T) {
	next := &recordingTelemetry{}

	mem := &InMemoryExporter{}
	exporter := NewToolSpanExporter(next, WithSpanExporter(mem))

	exporter.Emit(Event{
		Type: EventGraphStart,
	})

	require.Empty(t, mem.Spans, "non-tool events don't create spans")
	require.Len(t, next.events, 1, "all events are forwarded to next sink")
	require.Equal(t, EventGraphStart, next.events[0].Type)
}

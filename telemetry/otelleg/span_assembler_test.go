package otelleg

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

const (
	testTraceID   = "0123456789abcdef0123456789abcdef"
	testSpanID    = "0123456789abcdef"
	testToolName  = "shell"
	testModelName = "gemma4"
)

type assemblerHarness struct {
	recorder  *tracetest.SpanRecorder
	logs      *recordingLogExporter
	assembler *spanAssembler
}

func newAssemblerHarness(t *testing.T, orphanTimeout time.Duration, maxOpen int) *assemblerHarness {
	t.Helper()
	return newAssemblerHarnessWithClock(t, orphanTimeout, maxOpen, time.Now)
}

func newAssemblerHarnessWithClock(t *testing.T, orphanTimeout time.Duration, maxOpen int, clock func() time.Time) *assemblerHarness {
	t.Helper()
	recorder := tracetest.NewSpanRecorder()
	tracerProvider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	logs := &recordingLogExporter{}
	loggerProvider := sdklog.NewLoggerProvider(sdklog.WithProcessor(sdklog.NewSimpleProcessor(logs)))
	assembler := newSpanAssembler(tracerProvider.Tracer("test"), loggerProvider.Logger("test"), orphanTimeout, maxOpen, clock, nil)
	t.Cleanup(func() {
		assembler.shutdown()
		_ = tracerProvider.Shutdown(context.Background())
		_ = loggerProvider.Shutdown(context.Background())
	})
	return &assemblerHarness{recorder: recorder, logs: logs, assembler: assembler}
}

func spanAttrValue(span sdktrace.ReadOnlySpan, key string) (attribute.Value, bool) {
	for _, a := range span.Attributes() {
		if string(a.Key) == key {
			return a.Value, true
		}
	}
	return attribute.Value{}, false
}

func TestToolPairAssemblesSpan(t *testing.T) {
	h := newAssemblerHarness(t, time.Hour, 64)
	startTS := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	endTS := startTS.Add(1500 * time.Millisecond)
	h.assembler.Emit(telemetry.Event{
		Type:      telemetry.EventToolCall,
		TraceID:   testTraceID,
		SpanID:    testSpanID,
		AgentID:   "a1",
		NodeID:    "n1",
		Timestamp: startTS,
		Metadata: map[string]any{
			metaTool: testToolName,
			"span_attrs": map[string]any{
				"tool.name":    testToolName,
				"tool.family":  "exec",
				"trust_class":  "trusted",
				"effect_class": "process-spawn",
			},
		},
	})
	h.assembler.Emit(telemetry.Event{
		Type:      telemetry.EventToolResult,
		TraceID:   testTraceID,
		SpanID:    testSpanID,
		Timestamp: endTS,
		Metadata: map[string]any{
			metaTool:       testToolName,
			metaSuccess:    true,
			"exit_code":    0,
			"stdout_bytes": 12,
			"artifact_ref": "art-1",
			"duration_ms":  1450,
		},
	})

	spans := h.recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("ended span count = %d, want 1", len(spans))
	}
	span := spans[0]
	if span.Name() != "tool shell" {
		t.Errorf("span name = %q, want %q", span.Name(), "tool shell")
	}
	if span.Status().Code != codes.Ok {
		t.Errorf("status = %v, want Ok", span.Status().Code)
	}
	if !span.StartTime().Equal(startTS) || !span.EndTime().Equal(endTS) {
		t.Errorf("bounds = [%s,%s], want [%s,%s]", span.StartTime(), span.EndTime(), startTS, endTS)
	}
	for key, want := range map[string]string{
		"tool.name":               testToolName,
		"tool.family":             "exec",
		"capability.trust_class":  "trusted",
		"capability.effect_class": "process-spawn",
		"artifact_ref":            "art-1",
		"relurpify.span.event_id": testSpanID,
		"agent.id":                "a1",
		"node.id":                 "n1",
	} {
		value, ok := spanAttrValue(span, key)
		if !ok || value.AsString() != want {
			t.Errorf("attribute %s = %v (present=%v), want %q", key, value, ok, want)
		}
	}
	if value, ok := spanAttrValue(span, "exit_code"); !ok || value.AsInt64() != 0 {
		t.Errorf("exit_code = %v (present=%v), want 0", value, ok)
	}
	if value, ok := spanAttrValue(span, "stdout_bytes"); !ok || value.AsInt64() != 12 {
		t.Errorf("stdout_bytes = %v (present=%v), want 12", value, ok)
	}
	if value, ok := spanAttrValue(span, "relurpify.duration_ms"); !ok || value.AsInt64() != 1450 {
		t.Errorf("relurpify.duration_ms = %v (present=%v), want 1450", value, ok)
	}
}

func TestTraceIDFidelity(t *testing.T) {
	h := newAssemblerHarness(t, time.Hour, 64)
	startTS := time.Now()
	h.assembler.Emit(telemetry.Event{Type: telemetry.EventGraphStart, TraceID: testTraceID, SpanID: testSpanID, RunID: "r1", Timestamp: startTS})
	h.assembler.Emit(telemetry.Event{Type: telemetry.EventGraphFinish, TraceID: testTraceID, SpanID: testSpanID, RunID: "r1", Timestamp: startTS.Add(time.Second), Metadata: map[string]any{"status": "ok"}})

	spans := h.recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("ended span count = %d, want 1", len(spans))
	}
	if got := spans[0].SpanContext().TraceID().String(); got != testTraceID {
		t.Fatalf("trace id = %s, want %s", got, testTraceID)
	}
}

func TestAgentPairAndLLMPair(t *testing.T) {
	t.Run("agent", func(t *testing.T) {
		h := newAssemblerHarness(t, time.Hour, 64)
		startTS := time.Now()
		h.assembler.Emit(telemetry.Event{Type: telemetry.EventAgentStart, TraceID: testTraceID, SpanID: testSpanID, RunID: "r1", AgentID: "a1", Timestamp: startTS})
		h.assembler.Emit(telemetry.Event{Type: telemetry.EventAgentFinish, TraceID: testTraceID, SpanID: testSpanID, RunID: "r1", AgentID: "a1", Timestamp: startTS.Add(time.Second), Metadata: map[string]any{metaStatus: statusOK}})
		spans := h.recorder.Ended()
		if len(spans) != 1 || spans[0].Name() != "agent run a1" {
			t.Fatalf("agent span = %v", spans)
		}
		if spans[0].Status().Code != codes.Ok {
			t.Fatalf("agent status = %v, want Ok", spans[0].Status().Code)
		}
	})
	t.Run("llm", func(t *testing.T) {
		h := newAssemblerHarness(t, time.Hour, 64)
		startTS := time.Now()
		h.assembler.Emit(telemetry.Event{Type: telemetry.EventLLMPrompt, TraceID: testTraceID, SpanID: testSpanID, Timestamp: startTS, Metadata: map[string]any{metaModel: testModelName}})
		h.assembler.Emit(telemetry.Event{Type: telemetry.EventLLMResponse, TraceID: testTraceID, SpanID: testSpanID, Timestamp: startTS.Add(time.Second), Metadata: map[string]any{metaModel: testModelName, metaFinishReason: "stop", metaToolCallingMode: "native"}})
		spans := h.recorder.Ended()
		if len(spans) != 1 || spans[0].Name() != "llm gemma4" {
			t.Fatalf("llm span = %v", spans)
		}
		if value, ok := spanAttrValue(spans[0], "gen_ai.request.model"); !ok || value.AsString() != testModelName {
			t.Fatalf("gen_ai.request.model = %v (present=%v)", value, ok)
		}
		if value, ok := spanAttrValue(spans[0], "finish_reason"); !ok || value.AsString() != "stop" {
			t.Fatalf("finish_reason = %v (present=%v)", value, ok)
		}
	})
}

func TestNodeErrorSetsErrorStatus(t *testing.T) {
	h := newAssemblerHarness(t, time.Hour, 64)
	startTS := time.Now()
	h.assembler.Emit(telemetry.Event{Type: telemetry.EventNodeStart, TraceID: testTraceID, NodeID: "n1", Timestamp: startTS})
	h.assembler.Emit(telemetry.Event{Type: telemetry.EventNodeError, TraceID: testTraceID, NodeID: "n1", Timestamp: startTS.Add(time.Second), Message: "boom"})

	spans := h.recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("ended span count = %d, want 1", len(spans))
	}
	if spans[0].Name() != "node n1" {
		t.Errorf("span name = %q, want node n1", spans[0].Name())
	}
	if spans[0].Status().Code != codes.Error {
		t.Fatalf("status = %v, want Error", spans[0].Status().Code)
	}
}

func TestDelegationCancelEndsWithCancelled(t *testing.T) {
	h := newAssemblerHarness(t, time.Hour, 64)
	startTS := time.Now()
	meta := map[string]any{"delegation_id": "d1"}
	h.assembler.Emit(telemetry.Event{Type: telemetry.EventDelegationStart, Timestamp: startTS, Metadata: meta})
	h.assembler.Emit(telemetry.Event{Type: telemetry.EventDelegationCancel, Timestamp: startTS.Add(time.Second), Metadata: meta})

	spans := h.recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("ended span count = %d, want 1", len(spans))
	}
	if spans[0].Name() != "delegation d1" {
		t.Errorf("span name = %q, want delegation d1", spans[0].Name())
	}
	if spans[0].Status().Code != codes.Error || spans[0].Status().Description != "cancelled" {
		t.Fatalf("status = %v %q, want Error/cancelled", spans[0].Status().Code, spans[0].Status().Description)
	}
}

func TestUnpairedFinishIsLogRecordOnly(t *testing.T) {
	h := newAssemblerHarness(t, time.Hour, 64)
	h.assembler.Emit(telemetry.Event{
		Type:      telemetry.EventToolResult,
		TraceID:   testTraceID,
		SpanID:    testSpanID,
		Timestamp: time.Now(),
		Metadata:  map[string]any{metaTool: testToolName},
	})
	if got := len(h.recorder.Ended()); got != 0 {
		t.Fatalf("unpaired finish fabricated %d span(s)", got)
	}
	records := h.logs.snapshot()
	if len(records) != 1 {
		t.Fatalf("log record count = %d, want 1", len(records))
	}
	if records[0].EventName() != "tool_result" {
		t.Errorf("event name = %q, want tool_result", records[0].EventName())
	}
	if value := recordAttrMap(records[0])["relurpify.span.unpaired"]; !value.AsBool() {
		t.Fatalf("relurpify.span.unpaired = %v, want true", value)
	}
}

func TestOrphanSweep(t *testing.T) {
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	h := newAssemblerHarnessWithClock(t, 15*time.Minute, 64, func() time.Time { return now })
	h.assembler.Emit(telemetry.Event{Type: telemetry.EventToolCall, TraceID: testTraceID, SpanID: testSpanID, Timestamp: now, Metadata: map[string]any{metaTool: testToolName}})

	// A fresh entry is untouched.
	h.assembler.sweepOrphans(now.Add(14 * time.Minute))
	if got := len(h.recorder.Ended()); got != 0 {
		t.Fatalf("fresh span was swept (%d ended)", got)
	}

	now = now.Add(16 * time.Minute)
	h.assembler.sweepOrphans(now)
	spans := h.recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("orphan sweep ended %d spans, want 1", len(spans))
	}
	if spans[0].Status().Code != codes.Error || spans[0].Status().Description != "orphaned: no matching end event" {
		t.Errorf("orphan status = %v %q", spans[0].Status().Code, spans[0].Status().Description)
	}
	if !spans[0].EndTime().Equal(now.Add(-time.Minute)) { // openedAt + orphanTimeout
		t.Errorf("orphan end time = %s, want %s", spans[0].EndTime(), now.Add(-time.Minute))
	}
	if records := h.logs.snapshot(); len(records) != 1 || records[0].EventName() != "telemetry.span.orphaned" {
		t.Fatalf("orphan self-record missing: %v", records)
	}
	if got := h.assembler.openSpans.Load(); got != 0 {
		t.Fatalf("open spans = %d, want 0", got)
	}
}

func TestRegistryCapEvictsOldest(t *testing.T) {
	h := newAssemblerHarness(t, time.Hour, 4)
	base := time.Now()
	for i := 0; i < 5; i++ {
		h.assembler.Emit(telemetry.Event{
			Type:      telemetry.EventToolCall,
			TraceID:   testTraceID,
			SpanID:    fmt.Sprintf("%016x", i+1),
			Timestamp: base.Add(time.Duration(i) * time.Millisecond),
			Metadata:  map[string]any{metaTool: fmt.Sprintf("t%d", i)},
		})
	}
	spans := h.recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("eviction ended %d spans, want 1", len(spans))
	}
	if spans[0].Status().Code != codes.Error || spans[0].Status().Description != "registry_overflow" {
		t.Fatalf("evicted status = %v %q", spans[0].Status().Code, spans[0].Status().Description)
	}
	if got := h.assembler.openSpans.Load(); got != 4 {
		t.Fatalf("open spans = %d, want 4", got)
	}
}

func TestOutOfOrderFinishDegradesToLog(t *testing.T) {
	h := newAssemblerHarness(t, time.Hour, 64)
	startTS := time.Now()
	h.assembler.Emit(telemetry.Event{Type: telemetry.EventToolCall, TraceID: testTraceID, SpanID: testSpanID, Timestamp: startTS, Metadata: map[string]any{metaTool: testToolName}})
	h.assembler.Emit(telemetry.Event{Type: telemetry.EventToolResult, TraceID: testTraceID, SpanID: testSpanID, Timestamp: startTS.Add(-time.Second), Metadata: map[string]any{metaTool: testToolName}})

	if got := len(h.recorder.Ended()); got != 0 {
		t.Fatalf("out-of-order finish closed the span (%d ended)", got)
	}
	records := h.logs.snapshot()
	if len(records) != 1 {
		t.Fatalf("log record count = %d, want 1", len(records))
	}
	if value := recordAttrMap(records[0])["relurpify.span.out_of_order"]; !value.AsBool() {
		t.Fatalf("relurpify.span.out_of_order = %v, want true", value)
	}
	if got := h.assembler.openSpans.Load(); got != 1 {
		t.Fatalf("open spans = %d, want 1 (span left open for the orphan sweep)", got)
	}
}

func TestConcurrentPairing(t *testing.T) {
	h := newAssemblerHarness(t, time.Hour, 4096)
	const goroutines = 32
	const pairs = 500
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < pairs; i++ {
				spanID := fmt.Sprintf("%016x", g*pairs+i+1)
				base := time.Now()
				h.assembler.Emit(telemetry.Event{Type: telemetry.EventCapabilityCall, TraceID: testTraceID, SpanID: spanID, Timestamp: base, Metadata: map[string]any{metaCapability: "cap"}})
				h.assembler.Emit(telemetry.Event{Type: telemetry.EventCapabilityResult, TraceID: testTraceID, SpanID: spanID, Timestamp: base.Add(time.Millisecond), Metadata: map[string]any{metaCapability: "cap", metaSuccess: true}})
			}
		}(g)
	}
	wg.Wait()

	if got := len(h.recorder.Ended()); got != goroutines*pairs {
		t.Fatalf("assembled %d spans, want %d", got, goroutines*pairs)
	}
	if got := h.assembler.openSpans.Load(); got != 0 {
		t.Fatalf("open spans = %d, want 0", got)
	}
}

func TestAssemblerRecoversFromPanic(t *testing.T) {
	panics := 0
	assembler := newSpanAssembler(nil, nil, time.Hour, 64, time.Now, func() { panics++ })
	defer assembler.shutdown()
	// A nil tracer makes assembly panic; the boundary must contain it.
	assembler.Emit(telemetry.Event{Type: telemetry.EventToolCall, TraceID: testTraceID, SpanID: testSpanID, Timestamp: time.Now()})
	if panics != 1 {
		t.Fatalf("panic counter = %d, want 1", panics)
	}
}

func TestAssemblerDedupRule(t *testing.T) {
	h := newAssemblerHarness(t, time.Hour, 64)
	h.assembler.Emit(telemetry.Event{
		Type:      telemetry.EventToolCall,
		TraceID:   testTraceID,
		SpanID:    testSpanID,
		Timestamp: time.Now(),
		Metadata:  map[string]any{"otel_span": true},
	})
	if got := h.assembler.openSpans.Load(); got != 0 {
		t.Fatalf("otel_span=true event opened %d span(s)", got)
	}
}

func TestUnassemblableLifecycleDegradesToLog(t *testing.T) {
	h := newAssemblerHarness(t, time.Hour, 64)
	// No trace id and no derivable identity.
	h.assembler.Emit(telemetry.Event{Type: telemetry.EventToolCall, Timestamp: time.Now()})
	if got := h.assembler.openSpans.Load(); got != 0 {
		t.Fatalf("uncorrelated lifecycle event opened %d span(s)", got)
	}
	records := h.logs.snapshot()
	if len(records) != 1 || records[0].EventName() != "tool_call" {
		t.Fatalf("degraded log record missing: %v", records)
	}
}

func TestAssemblerSurvivesHostileMetadata(t *testing.T) {
	h := newAssemblerHarness(t, time.Hour, 64)
	recursive := map[string]any{}
	recursive["self"] = recursive
	base := time.Now()
	h.assembler.Emit(telemetry.Event{
		Type:      telemetry.EventToolCall,
		TraceID:   testTraceID,
		SpanID:    testSpanID,
		Timestamp: base,
		Metadata:  map[string]any{metaTool: testToolName, "span_attrs": recursive, "nested": map[int]any{1: recursive}},
	})
	h.assembler.Emit(telemetry.Event{Type: telemetry.EventToolResult, TraceID: testTraceID, SpanID: testSpanID, Timestamp: base.Add(time.Millisecond), Metadata: map[string]any{metaSuccess: true}})
	if got := len(h.recorder.Ended()); got != 1 {
		t.Fatalf("assembled %d spans, want 1", got)
	}
}

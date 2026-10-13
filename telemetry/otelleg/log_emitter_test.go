package otelleg

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/telemetry"
	otellog "go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
)

type recordingLogExporter struct {
	mu      sync.Mutex
	records []sdklog.Record
}

// testLogMessage is reused across the emitter and routing tests.
const testLogMessage = "job done"

func (e *recordingLogExporter) Export(_ context.Context, records []sdklog.Record) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := range records {
		e.records = append(e.records, records[i].Clone())
	}
	return nil
}

func (e *recordingLogExporter) Shutdown(context.Context) error   { return nil }
func (e *recordingLogExporter) ForceFlush(context.Context) error { return nil }

func (e *recordingLogExporter) snapshot() []sdklog.Record {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]sdklog.Record(nil), e.records...)
}

func newRecordingLogger(t *testing.T) (otellog.Logger, *recordingLogExporter) {
	t.Helper()
	exporter := &recordingLogExporter{}
	provider := sdklog.NewLoggerProvider(sdklog.WithProcessor(sdklog.NewSimpleProcessor(exporter)))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	return provider.Logger(instrumentationScopeName), exporter
}

func recordAttrMap(rec sdklog.Record) map[string]otellog.Value {
	m := map[string]otellog.Value{}
	rec.WalkAttributes(func(kv otellog.KeyValue) bool {
		m[kv.Key] = kv.Value
		return true
	})
	return m
}

func TestEmitLogRecordShape(t *testing.T) {
	logger, exporter := newRecordingLogger(t)
	timestamp := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	traceID := strings.Repeat("ab", 16)
	spanID := strings.Repeat("cd", 8)
	ev := telemetry.Event{
		Type:      telemetry.EventSchedulerJobCompleted,
		SessionID: "s1",
		RunID:     "r1",
		AgentID:   "a1",
		NodeID:    "n1",
		TaskID:    "t1",
		TraceID:   traceID,
		SpanID:    spanID,
		Message:   testLogMessage,
		Timestamp: timestamp,
		Seq:       7,
		Metadata:  map[string]any{"duration_ms": 42, "success": true, "note": "x"},
	}
	emitLog(logger, ev)

	records := exporter.snapshot()
	if len(records) != 1 {
		t.Fatalf("record count = %d, want 1", len(records))
	}
	rec := records[0]
	if rec.EventName() != "scheduler.job_completed" {
		t.Errorf("EventName = %q", rec.EventName())
	}
	if rec.Severity() != otellog.SeverityInfo1 {
		t.Errorf("Severity = %v, want Info", rec.Severity())
	}
	if rec.SeverityText() != "INFO" {
		t.Errorf("SeverityText = %q, want INFO", rec.SeverityText())
	}
	if !rec.Timestamp().Equal(timestamp) {
		t.Errorf("Timestamp = %s, want %s", rec.Timestamp(), timestamp)
	}
	if rec.Body().AsString() != testLogMessage {
		t.Errorf("Body = %q", rec.Body().AsString())
	}
	if rec.TraceID().String() != traceID {
		t.Errorf("TraceID = %s, want %s", rec.TraceID(), traceID)
	}
	if rec.SpanID().String() != spanID {
		t.Errorf("SpanID = %s, want %s", rec.SpanID(), spanID)
	}

	attrs := recordAttrMap(rec)
	for key, want := range map[string]string{
		"session.id": "s1",
		"run.id":     "r1",
		"agent.id":   "a1",
		"node.id":    "n1",
		"task.id":    "t1",
		"note":       "x",
	} {
		if attrs[key].AsString() != want {
			t.Errorf("attribute %s = %q, want %q", key, attrs[key].AsString(), want)
		}
	}
	if attrs["relurpify.seq"].AsInt64() != 7 {
		t.Errorf("relurpify.seq = %v, want 7", attrs["relurpify.seq"])
	}
	if attrs["duration_ms"].AsInt64() != 42 {
		t.Errorf("duration_ms = %v, want 42", attrs["duration_ms"])
	}
	if !attrs["success"].AsBool() {
		t.Errorf("success = %v, want true", attrs["success"])
	}
}

func TestEmitLogZeroTimestampUsesNow(t *testing.T) {
	logger, exporter := newRecordingLogger(t)
	before := time.Now()
	emitLog(logger, telemetry.Event{Type: telemetry.EventBootstrapComplete})
	after := time.Now()

	records := exporter.snapshot()
	if len(records) != 1 {
		t.Fatalf("record count = %d, want 1", len(records))
	}
	got := records[0].Timestamp()
	if got.Before(before) || got.After(after) {
		t.Fatalf("zero timestamp did not resolve to now: %s not within [%s,%s]", got, before, after)
	}
}

func TestEmitLogTraceLessWhenIDsInvalid(t *testing.T) {
	logger, exporter := newRecordingLogger(t)
	emitLog(logger, telemetry.Event{Type: telemetry.EventStateChange, TraceID: "zz", SpanID: "bad"})

	records := exporter.snapshot()
	if len(records) != 1 {
		t.Fatalf("record count = %d, want 1", len(records))
	}
	rec := records[0]
	if rec.TraceID().IsValid() {
		t.Fatal("invalid trace id produced a valid trace context")
	}
	attrs := recordAttrMap(rec)
	if attrs["relurpify.trace_id"].AsString() != "zz" || attrs["relurpify.span_id"].AsString() != "bad" {
		t.Fatalf("unusable ids not preserved as attributes: %v", attrs)
	}
}

func TestEmitLogSeverityText(t *testing.T) {
	logger, exporter := newRecordingLogger(t)
	emitLog(logger, telemetry.Event{Type: telemetry.EventNodeError})
	records := exporter.snapshot()
	if len(records) != 1 || records[0].SeverityText() != "ERROR" {
		t.Fatalf("severity text = %q, want ERROR", records[0].SeverityText())
	}
}

func TestEmitLogNilLoggerIsNoop(_ *testing.T) {
	emitLog(nil, telemetry.Event{Type: telemetry.EventStateChange})
}

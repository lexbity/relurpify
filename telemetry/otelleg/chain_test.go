package otelleg

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/telemetry"
	"go.opentelemetry.io/otel/log"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

func TestChainFileLegWritesParsableOTLPJSON(t *testing.T) {
	dir := t.TempDir()
	chain := newFileOnlyChain(t, dir)
	emitSpan(t, chain)
	emitProviderLog(t, chain)
	emitMetric(t, chain)
	if err := chain.Close(context.Background()); err != nil {
		t.Fatalf("close: %v", err)
	}

	seen := map[string]bool{}
	for _, line := range segmentJSONObjects(t, dir) {
		for key := range line {
			seen[key] = true
		}
	}
	for _, want := range []string{"resourceSpans", "resourceLogs", "resourceMetrics"} {
		if !seen[want] {
			t.Fatalf("file leg missing %q line; saw %v", want, seen)
		}
	}
	if names := nestedNames(t, dir, "resourceSpans", "scopeSpans", "spans", "name"); !names["test-span"] {
		t.Fatalf("span missing: %v", names)
	}
	if names := nestedNames(t, dir, "resourceLogs", "scopeLogs", "logRecords", "eventName"); !names["test.event"] {
		t.Fatalf("log record missing: %v", names)
	}
	if names := nestedNames(t, dir, "resourceMetrics", "scopeMetrics", "metrics", "name"); !names["test.counter"] {
		t.Fatalf("metric missing: %v", names)
	}
}

// nestedNames collects line[top][middle][list][field] across every segment line,
// proving the OTLP-JSON shape the downstream selection report parses.
func nestedNames(t *testing.T, dir, top, middle, list, field string) map[string]bool {
	t.Helper()
	names := map[string]bool{}
	for _, line := range segmentJSONObjects(t, dir) {
		raw, ok := line[top]
		if !ok {
			continue
		}
		var topList []map[string]json.RawMessage
		if err := json.Unmarshal(raw, &topList); err != nil {
			t.Fatalf("unmarshal %s: %v", top, err)
		}
		for _, item := range topList {
			var middleList []map[string]json.RawMessage
			if err := json.Unmarshal(item[middle], &middleList); err != nil {
				t.Fatalf("unmarshal %s.%s: %v", top, middle, err)
			}
			for _, entry := range middleList {
				var items []map[string]json.RawMessage
				if err := json.Unmarshal(entry[list], &items); err != nil {
					t.Fatalf("unmarshal %s.%s.%s: %v", top, middle, list, err)
				}
				for _, item := range items {
					var name string
					if err := json.Unmarshal(item[field], &name); err != nil {
						continue
					}
					names[name] = true
				}
			}
		}
	}
	return names
}

func TestChainOTLPHTTPRoundTrip(t *testing.T) {
	var mu sync.Mutex
	bodies := map[string][][]byte{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		mu.Lock()
		bodies[r.URL.Path] = append(bodies[r.URL.Path], body)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx := context.Background()
	chain, err := New(ctx, Settings{Enabled: true, Endpoint: srv.URL, Timeout: time.Second})
	if err != nil {
		t.Fatalf("new chain: %v", err)
	}
	emitSpan(t, chain)
	emitProviderLog(t, chain)
	emitMetric(t, chain)
	if err := chain.Close(ctx); err != nil {
		t.Fatalf("close: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	for path, decode := range map[string]func([]byte) error{
		"/v1/traces": func(b []byte) error {
			var data tracepb.TracesData
			return proto.Unmarshal(b, &data)
		},
		"/v1/logs": func(b []byte) error {
			var data logspb.LogsData
			return proto.Unmarshal(b, &data)
		},
		"/v1/metrics": func(b []byte) error {
			var data metricspb.MetricsData
			return proto.Unmarshal(b, &data)
		},
	} {
		received := bodies[path]
		if len(received) == 0 {
			t.Fatalf("no OTLP/HTTP request received at %s", path)
		}
		for _, body := range received {
			if err := decode(body); err != nil {
				t.Fatalf("decode %s body: %v", path, err)
			}
		}
	}
}

func TestChainConfigValidationFailsClosed(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name     string
		settings Settings
	}{
		{name: "bad url", settings: Settings{Enabled: true, Endpoint: "http://[::1"}},
		{name: "bad scheme", settings: Settings{Enabled: true, Endpoint: "ftp://x"}},
		{name: "missing ca", settings: Settings{Enabled: true, Endpoint: testTLSEndpoint, CAFile: filepath.Join(dir, "missing.pem")}},
		{name: "ratio out of range", settings: Settings{Enabled: true, SamplingRatio: 1.5}},
		{name: "file without dir", settings: Settings{Enabled: true, FileEnabled: true}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := New(context.Background(), tc.settings); err == nil {
				t.Fatal("expected New to fail closed, got nil")
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatalf("read dir: %v", err)
			}
			if len(entries) != 0 {
				t.Fatalf("failed construction created files: %v", entries)
			}
		})
	}
}

func TestChainDisabledProducesNothing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "otlp")
	chain, err := New(context.Background(), Settings{Enabled: false, FileEnabled: true, FileDir: dir})
	if err != nil {
		t.Fatalf("new chain: %v", err)
	}
	if chain.tracerProvider != nil || chain.loggerProvider != nil || chain.meterProvider != nil || chain.segment != nil {
		t.Fatal("disabled chain built providers or a file leg")
	}
	chain.Emit(telemetry.Event{Type: telemetry.EventBootstrapComplete})
	if err := chain.Close(context.Background()); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("disabled chain created %s: %v", dir, err)
	}
}

func TestEmitNeverPanics(t *testing.T) {
	var nilChain *Chain
	nilChain.Emit(telemetry.Event{Type: telemetry.EventGraphStart})

	disabled, err := New(context.Background(), Settings{})
	if err != nil {
		t.Fatalf("new disabled chain: %v", err)
	}
	disabled.Emit(telemetry.Event{Type: telemetry.EventGraphStart})

	chain := newFileOnlyChain(t, t.TempDir())
	if err := chain.Close(context.Background()); err != nil {
		t.Fatalf("close: %v", err)
	}
	chain.Emit(telemetry.Event{Type: telemetry.EventGraphStart})
	if got := chain.SelfMetrics().Dropped; got != 1 {
		t.Fatalf("Dropped = %d, want 1 after emit on a closed chain", got)
	}
}

// TestEmitSurvivesHostileMetadata proves the emit boundary tolerates metadata
// that would break a naive stringifier: a self-referential map, a map with a
// non-string key, nested containers, and a value with a panicking String method.
func TestEmitSurvivesHostileMetadata(t *testing.T) {
	chain := newFileOnlyChain(t, t.TempDir())
	recursive := map[string]any{}
	recursive["self"] = recursive
	chain.Emit(telemetry.Event{Type: telemetry.EventStateChange, Metadata: recursive})
	chain.Emit(telemetry.Event{Type: telemetry.EventStateChange, Metadata: map[string]any{
		"nested": map[int]any{1: recursive, 2: []any{recursive}},
		"panic":  panickingStringer{},
		"nil":    nil,
	}})
	if err := chain.Close(context.Background()); err != nil {
		t.Fatalf("close: %v", err)
	}
	if got := chain.SelfMetrics().Received; got != 2 {
		t.Fatalf("Received = %d, want 2", got)
	}
}

func TestEmitDropCounting(t *testing.T) {
	chain := newFileOnlyChain(t, t.TempDir())
	chain.Emit(telemetry.Event{Type: telemetry.EventStateChange})
	if got := chain.SelfMetrics().Received; got != 1 {
		t.Fatalf("Received = %d, want 1", got)
	}
	if err := chain.Close(context.Background()); err != nil {
		t.Fatalf("close: %v", err)
	}
	chain.Emit(telemetry.Event{Type: telemetry.EventStateChange})
	if got := chain.SelfMetrics().Dropped; got != 1 {
		t.Fatalf("Dropped = %d, want 1 after emit on a closed chain", got)
	}
}

// TestChainRoutesPointEventsToLogs proves a point event becomes an OTLP-JSON log
// record while an assemblable lifecycle event is kept off the log signal (it
// becomes a span in the assembler).
func TestChainRoutesPointEventsToLogs(t *testing.T) {
	dir := t.TempDir()
	chain := newFileOnlyChain(t, dir)
	chain.Emit(telemetry.Event{
		Type:      telemetry.EventSchedulerJobCompleted,
		Timestamp: time.Now(),
		Message:   testLogMessage,
	})
	chain.Emit(telemetry.Event{
		Type:      telemetry.EventGraphStart,
		TraceID:   testTraceID,
		SpanID:    testSpanID,
		RunID:     "r1",
		Timestamp: time.Now(),
	})
	if err := chain.Close(context.Background()); err != nil {
		t.Fatalf("close: %v", err)
	}

	eventNames := logRecordEventNames(t, dir)
	if !eventNames["scheduler.job_completed"] {
		t.Fatalf("point event missing from log records: %v", eventNames)
	}
	if eventNames["graph_start"] {
		t.Fatal("assemblable lifecycle event was emitted as a log record")
	}
}

// panickingStringer panics when formatted, proving fmt-based rendering is
// contained.
type panickingStringer struct{}

func (panickingStringer) String() string { panic("boom") }

func TestCloseIsIdempotent(t *testing.T) {
	chain := newFileOnlyChain(t, t.TempDir())
	if err := chain.Close(context.Background()); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := chain.Close(context.Background()); err != nil {
		t.Fatalf("second close: %v", err)
	}
}

func newFileOnlyChain(t *testing.T, dir string) *Chain {
	t.Helper()
	chain, err := New(context.Background(), Settings{Enabled: true, FileEnabled: true, FileDir: dir})
	if err != nil {
		t.Fatalf("new chain: %v", err)
	}
	return chain
}

func emitSpan(t *testing.T, c *Chain) {
	t.Helper()
	_, span := c.tracerProvider.Tracer("test").Start(context.Background(), "test-span")
	span.End()
}

func emitProviderLog(t *testing.T, c *Chain) {
	t.Helper()
	var rec log.Record
	rec.SetTimestamp(time.Now())
	rec.SetEventName("test.event")
	rec.SetSeverity(log.SeverityInfo1)
	rec.SetBody(log.StringValue("hello"))
	c.loggerProvider.Logger("test").Emit(context.Background(), rec)
}

func emitMetric(t *testing.T, c *Chain) {
	t.Helper()
	ctr, err := c.meterProvider.Meter("test").Int64Counter("test.counter")
	if err != nil {
		t.Fatalf("counter: %v", err)
	}
	ctr.Add(context.Background(), 1)
}

// segmentJSONObjects reads every complete line from every segment under dir and
// unmarshals it into a generic object, failing on a partial or non-JSON line.
func segmentJSONObjects(t *testing.T, dir string) []map[string]json.RawMessage {
	t.Helper()
	var out []map[string]json.RawMessage
	for _, name := range segmentFiles(t, dir) {
		if !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		for _, line := range readLines(t, filepath.Join(dir, name)) {
			if strings.TrimSpace(line) == "" {
				continue
			}
			var obj map[string]json.RawMessage
			if err := json.Unmarshal([]byte(line), &obj); err != nil {
				t.Fatalf("segment %s line is not OTLP-JSON: %v (%q)", name, err, line)
			}
			out = append(out, obj)
		}
	}
	return out
}

// logRecordEventNames collects the eventName of every log record in the segment
// store, proving the OTLP-JSON shape the downstream parsers walk.
func logRecordEventNames(t *testing.T, dir string) map[string]bool {
	t.Helper()
	return nestedNames(t, dir, "resourceLogs", "scopeLogs", "logRecords", "eventName")
}

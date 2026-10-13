package otelleg

// Integration proofs for the full chain against a real OTLP/HTTP receiver and
// a hostile collector. Slice 8: AC-10 (a receiver accepts trace, log, and
// metric exports from a live chain, with content assertions on the decoded
// payloads), D9/NFR-11 (a collector outage produces counted drops and at most
// one warning — never a runtime error), and NFR-5 (close under a cancelled
// context still returns within its internal deadlines).

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	logspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	metricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	tracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb1 "go.opentelemetry.io/proto/otlp/logs/v1"

	"google.golang.org/protobuf/proto"

	"codeburg.org/lexbit/relurpify/telemetry"
)

// collector is an in-process OTLP/HTTP receiver decoding the three export
// services and keeping the requests for content assertions.
type collector struct {
	URL     string
	mu      sync.Mutex
	spans   []*tracepb.ExportTraceServiceRequest
	logs    []*logspb.ExportLogsServiceRequest
	metrics []*metricspb.ExportMetricsServiceRequest
	server  *httptest.Server
}

func newCollector(t *testing.T) *collector {
	t.Helper()
	c := &collector{}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/traces", func(w http.ResponseWriter, r *http.Request) {
		body := readAll(t, r)
		var req tracepb.ExportTraceServiceRequest
		if err := proto.Unmarshal(body, &req); err != nil {
			t.Errorf("decode trace export: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		c.mu.Lock()
		c.spans = append(c.spans, &req)
		c.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/v1/logs", func(w http.ResponseWriter, r *http.Request) {
		body := readAll(t, r)
		var req logspb.ExportLogsServiceRequest
		if err := proto.Unmarshal(body, &req); err != nil {
			t.Errorf("decode log export: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		c.mu.Lock()
		c.logs = append(c.logs, &req)
		c.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/v1/metrics", func(w http.ResponseWriter, r *http.Request) {
		body := readAll(t, r)
		var req metricspb.ExportMetricsServiceRequest
		if err := proto.Unmarshal(body, &req); err != nil {
			t.Errorf("decode metric export: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		c.mu.Lock()
		c.metrics = append(c.metrics, &req)
		c.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	})
	c.server = httptest.NewServer(mux)
	c.URL = c.server.URL
	t.Cleanup(c.server.Close)
	return c
}

func (c *collector) counts() (spans, logs, metrics int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.spans), len(c.logs), len(c.metrics)
}

// spanNames returns every span name the collector received.
func (c *collector) spanNames() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var names []string
	for _, req := range c.spans {
		for _, rs := range req.ResourceSpans {
			for _, ss := range rs.ScopeSpans {
				for _, span := range ss.Spans {
					names = append(names, span.Name)
				}
			}
		}
	}
	return names
}

// logEventNames returns every eventName the collector received.
func (c *collector) logEventNames() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var names []string
	for _, req := range c.logs {
		for _, rl := range req.ResourceLogs {
			for _, sl := range rl.ScopeLogs {
				for _, rec := range sl.LogRecords {
					names = append(names, eventNameOf(rec))
				}
			}
		}
	}
	return names
}

// eventNameOf extracts the event name from a record: the OTel log API sets
// the eventName field on the wire; the body/severity fallthrough is defensive
// only for records emitted through raw provider APIs.
func eventNameOf(rec *logspb1.LogRecord) string {
	if rec.EventName != "" {
		return rec.EventName
	}
	return rec.Body.GetStringValue()
}

func attrStringValue(attrs []*commonpb.KeyValue, key string) string {
	for _, attr := range attrs {
		if attr.Key == key {
			return attr.GetValue().GetStringValue()
		}
	}
	return ""
}

func readAll(t *testing.T, r *http.Request) []byte {
	t.Helper()
	body := make([]byte, 0, 4096)
	buf := make([]byte, 4096)
	for {
		n, err := r.Body.Read(buf)
		body = append(body, buf[:n]...)
		if err != nil {
			break
		}
	}
	return body
}

// newDeadCollector returns a URL that refuses connections.
func newDeadCollector() *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	srv.Close()
	return srv
}

// TestIntegrationCollectorReceivesAllThreeSignals drives a live chain — file
// leg AND OTLP/HTTP endpoint — through the real Emit path (lifecycle pair,
// point event) and proves the collector receives decoded payloads with the
// expected content while the local segment carries the same records (AC-10).
func TestIntegrationCollectorReceivesAllThreeSignals(t *testing.T) {
	srv := newCollector(t)
	segmentDir := t.TempDir()
	ctx := context.Background()
	chain, err := New(ctx, Settings{
		Enabled:     true,
		Endpoint:    srv.URL,
		Timeout:     2 * time.Second,
		FileEnabled: true,
		FileDir:     segmentDir,
	})
	if err != nil {
		t.Fatalf("new chain: %v", err)
	}

	// Real Emit path: a tool pair assembles a span, a point event becomes a
	// log record. The periodic metric reader exports on the metrics interval,
	// exercised by Close's final flush.
	chain.Emit(telemetry.Event{
		Type:      telemetry.EventToolCall,
		Timestamp: time.Now().UTC(),
		Metadata:  map[string]any{"span_id": "abcdef0123456789", "tool": "shell"},
	})
	chain.Emit(telemetry.Event{
		Type:      telemetry.EventToolResult,
		Timestamp: time.Now().UTC(),
		Metadata:  map[string]any{"span_id": "abcdef0123456789", "success": true},
	})
	chain.Emit(telemetry.Event{
		Type:      telemetry.EventStateChange,
		Timestamp: time.Now().UTC(),
		Message:   "integration",
	})
	if err := chain.Close(ctx); err != nil {
		t.Fatalf("close: %v", err)
	}

	spans, logs, metrics := srv.counts()
	if spans == 0 || logs == 0 || metrics == 0 {
		t.Fatalf("collector received span-batches=%d log-batches=%d metric-batches=%d, want all non-zero (AC-10)", spans, logs, metrics)
	}

	// Content: the assembled tool span and the point-event record reached the
	// wire with their identities intact.
	foundToolSpan, foundRecord := false, false
	for _, name := range srv.spanNames() {
		if name == "tool shell" {
			foundToolSpan = true
		}
	}
	for _, name := range srv.logEventNames() {
		if name == string(telemetry.EventStateChange) {
			foundRecord = true
		}
	}
	if !foundToolSpan {
		t.Fatalf("collector never received the assembled tool span; got %v", srv.spanNames())
	}
	if !foundRecord {
		t.Fatalf("collector never received the point-event log record; got %v", srv.logEventNames())
	}

	// The local leg carries the same traffic, line-by-line parsable.
	segmentSpans, err := ReadSegmentDirSpans(segmentDir)
	if err != nil {
		t.Fatalf("read segments: %v", err)
	}
	toolSpans := 0
	for _, span := range segmentSpans {
		if span.Name == "tool shell" {
			toolSpans++
		}
	}
	if toolSpans == 0 {
		t.Fatalf("no tool span in the local segment (%d spans)", len(segmentSpans))
	}
	segmentRecords, err := ReadSegmentDirLogRecords(segmentDir)
	if err != nil {
		t.Fatalf("read segments: %v", err)
	}
	foundLocal := false
	for _, rec := range segmentRecords {
		if rec.Name == string(telemetry.EventStateChange) {
			foundLocal = true
		}
	}
	if !foundLocal {
		t.Fatal("point event missing from the local segment")
	}
}

// TestIntegrationCollectorOutageIsBestEffort points the chain at a dead
// endpoint: emits keep working, the failure is counted, and Close returns
// within its deadline — a collector outage must never break or stall the
// runtime (D9/NFR-11).
func TestIntegrationCollectorOutageIsBestEffort(t *testing.T) {
	dead := newDeadCollector()
	ctx := context.Background()
	chain, err := New(ctx, Settings{Enabled: true, Endpoint: dead.URL, Timeout: time.Second})
	if err != nil {
		t.Fatalf("new chain: %v", err)
	}

	for i := 0; i < 100; i++ {
		chain.Emit(telemetry.Event{Type: telemetry.EventStateChange, Timestamp: time.Now().UTC()})
	}

	closeDone := make(chan error, 1)
	go func() { closeDone <- chain.Close(ctx) }()
	select {
	case <-closeDone:
	case <-time.After(35 * time.Second):
		t.Fatal("Close did not return within the NFR-5 budget against a dead collector")
	}

	snap := chain.SelfMetrics()
	if snap.ExportDropped == 0 {
		t.Fatal("export failures were not counted (D8 drop accounting missing)")
	}
}

// TestIntegrationShutdownRespectsCancelledContext proves Close with an
// already-cancelled context still returns promptly: the internal deadlines
// (D9) bound the flushes instead of propagating the dead context forever.
func TestIntegrationShutdownRespectsCancelledContext(t *testing.T) {
	srv := newCollector(t)
	ctx := context.Background()
	chain, err := New(ctx, Settings{Enabled: true, Endpoint: srv.URL, Timeout: time.Second})
	if err != nil {
		t.Fatalf("new chain: %v", err)
	}
	chain.Emit(telemetry.Event{Type: telemetry.EventStateChange, Timestamp: time.Now().UTC()})

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	done := make(chan error, 1)
	go func() { done <- chain.Close(cancelled) }()
	select {
	case <-done:
	case <-time.After(35 * time.Second):
		t.Fatal("Close ignored its internal deadlines under a cancelled context")
	}
}

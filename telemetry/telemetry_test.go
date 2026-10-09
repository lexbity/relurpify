package telemetry

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func newTestJSONFileTelemetry(t *testing.T) (*JSONFileTelemetry, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "telemetry.jsonl")
	sink, err := NewJSONFileTelemetry(path)
	if err != nil {
		t.Fatalf("NewJSONFileTelemetry: %v", err)
	}
	t.Cleanup(func() { _ = sink.Close() })
	return sink, path
}

// TestJSONFileTelemetryEncodeErrorDropsAndCounts proves a marshal failure drops
// the event, counts it, and does not poison the sink for later events.
func TestJSONFileTelemetryEncodeErrorDropsAndCounts(t *testing.T) {
	sink, path := newTestJSONFileTelemetry(t)

	sink.Emit(Event{Type: EventGraphStart, TaskID: "ok"})
	if got := sink.DroppedTotal(); got != 0 {
		t.Fatalf("DroppedTotal after good emit = %d, want 0", got)
	}

	// math.Inf is not JSON-encodable, so json.Encoder returns an error while the
	// file stays open — the exact branch that used to panic.
	sink.Emit(Event{Type: EventGraphStart, Metadata: map[string]any{"bad": math.Inf(1)}})
	if got := sink.DroppedTotal(); got != 1 {
		t.Fatalf("DroppedTotal after encode error = %d, want 1", got)
	}

	// The sink is not poisoned: a later good emit still lands.
	sink.Emit(Event{Type: EventGraphFinish, TaskID: "after"})
	if got := sink.DroppedTotal(); got != 1 {
		t.Fatalf("DroppedTotal after recovery = %d, want 1", got)
	}

	if err := sink.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatalf("read telemetry file: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 written lines, got %d: %q", len(lines), string(data))
	}
	var first Event
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("unmarshal first line: %v", err)
	}
	if first.TaskID != "ok" {
		t.Fatalf("first event TaskID = %q, want ok", first.TaskID)
	}
}

// TestJSONFileTelemetryCloseIdempotent proves Close is safe to repeat and that
// emits after Close drop and count rather than writing to a closed file.
func TestJSONFileTelemetryCloseIdempotent(t *testing.T) {
	sink, _ := newTestJSONFileTelemetry(t)

	if err := sink.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := sink.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if err := sink.Close(); err != nil {
		t.Fatalf("third Close: %v", err)
	}

	const drops = 5
	for i := 0; i < drops; i++ {
		sink.Emit(Event{Type: EventGraphStart})
	}
	if got := sink.DroppedTotal(); got != drops {
		t.Fatalf("DroppedTotal after post-close emits = %d, want %d", got, drops)
	}
}

// TestJSONFileTelemetryConcurrentCloseDuringEmit drives emitters against a
// closer. Run with -race; the contract is that this completes without a panic
// or data race.
func TestJSONFileTelemetryConcurrentCloseDuringEmit(t *testing.T) {
	sink, _ := newTestJSONFileTelemetry(t)

	const emitters = 8
	const iterations = 200
	start := make(chan struct{})

	var wg sync.WaitGroup
	for i := 0; i < emitters; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for j := 0; j < iterations; j++ {
				sink.Emit(Event{Type: EventNodeFinish, TaskID: "race"})
			}
		}()
	}
	closerDone := make(chan struct{})
	go func() {
		defer close(closerDone)
		<-start
		for j := 0; j < iterations; j++ {
			_ = sink.Close()
		}
	}()

	close(start)
	wg.Wait()
	<-closerDone
}

// TestJSONFileTelemetryOneShotWarning proves the degradation log fires exactly
// once no matter how many events are dropped.
func TestJSONFileTelemetryOneShotWarning(t *testing.T) {
	var buf bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(previous)

	sink, _ := newTestJSONFileTelemetry(t)
	if err := sink.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	const drops = 100
	for i := 0; i < drops; i++ {
		sink.Emit(Event{Type: EventGraphStart})
	}
	if got := sink.DroppedTotal(); got != drops {
		t.Fatalf("DroppedTotal = %d, want %d", got, drops)
	}
	if n := strings.Count(buf.String(), "degraded"); n != 1 {
		t.Fatalf("expected exactly one degradation log line, got %d: %q", n, buf.String())
	}
}

// TestNewJSONFileTelemetryOpenError covers the construction failure path: a
// missing parent directory must return an error, never a panicking sink.
func TestNewJSONFileTelemetryOpenError(t *testing.T) {
	if _, err := NewJSONFileTelemetry(filepath.Join(t.TempDir(), "missing", "telemetry.jsonl")); err == nil {
		t.Fatal("expected an error opening a path in a missing directory")
	}
}

// TestJSONFileTelemetryZeroValueClose covers the nil-file branch of Close.
func TestJSONFileTelemetryZeroValueClose(t *testing.T) {
	if err := (&JSONFileTelemetry{}).Close(); err != nil {
		t.Fatalf("zero-value Close: %v", err)
	}
}

// TestLoggerTelemetryEmitsAllEventKinds covers every LoggerTelemetry method,
// including the nil-logger fallback to the default logger.
func TestLoggerTelemetryEmitsAllEventKinds(t *testing.T) {
	var buf bytes.Buffer
	sink := LoggerTelemetry{Logger: log.New(&buf, "", 0)}

	sink.Emit(Event{Type: EventGraphStart, NodeID: "n", TaskID: "t", Message: "m"})
	sink.OnArtifactPruning("t", 1, 2)
	sink.OnBudgetExceeded("t", 3, 4)
	sink.OnCheckpointCreated("t", "cp", "n")
	sink.OnCheckpointRestored("t", "cp")
	sink.OnGraphResume("t", "cp", "n")

	if buf.Len() == 0 {
		t.Fatal("expected LoggerTelemetry to write records")
	}

	previous := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(previous)
	(LoggerTelemetry{}).OnBudgetExceeded("t", 0, 0)
}

// TestMultiplexTelemetryCloseClosesFileSink proves the multiplex releases the
// resources its sinks own while leaving error-less fan-out sinks alone.
func TestMultiplexTelemetryCloseClosesFileSink(t *testing.T) {
	sink, _ := newTestJSONFileTelemetry(t)
	mux := MultiplexTelemetry{Sinks: []Telemetry{LoggerTelemetry{}, sink}}

	if err := mux.Close(); err != nil {
		t.Fatalf("multiplex Close: %v", err)
	}
	sink.Emit(Event{Type: EventGraphStart})
	if got := sink.DroppedTotal(); got != 1 {
		t.Fatalf("file sink was not closed by multiplex Close (DroppedTotal = %d)", got)
	}
	// Idempotent: a second Close is still nil.
	if err := mux.Close(); err != nil {
		t.Fatalf("second multiplex Close: %v", err)
	}
}

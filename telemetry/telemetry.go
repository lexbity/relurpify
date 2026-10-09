// Package telemetry provides audit logging and execution tracing for agent runs.
// It collects structured records of node executions, tool calls, and LLM interactions,
// supporting retention-based audit policies defined in the agent manifest.
//
// BroadcastSink (broadcast.go) provides an in-memory subscribable sink for live
// event observers — used by the TUI to receive execution events in real time.
package telemetry

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"codeburg.org/lexbit/relurpify/platform/observability"
)

// MultiplexTelemetry broadcasts events to multiple sinks.
type MultiplexTelemetry struct {
	Sinks []Telemetry
}

// StampCorrelation populates the first-class correlation fields on ev from the
// correlation identifiers carried by ctx. It is the single sanctioned way to
// populate SessionID/RunID/TraceID/AgentID/SpanID (NFR-6): emitters must never
// construct those fields by hand, and must never smuggle them through Metadata.
//
// Merge semantics come from CorrelationFromContext (single source of truth):
//
//   - A non-empty RunContext value overwrites the corresponding field, so every
//     event emitted inside a turn agrees on the turn's identifiers.
//   - An empty RunContext value never clears a field the caller already set, so
//     an emitter that legitimately knows a SessionID is not downgraded.
//   - TraceID is turn-scoped (Decision 3): when a RunContext is present it wins
//     over any node-local TraceContext. TraceContext.SpanID is per-node and is
//     always stamped when present; a TraceContext TraceID is only used as a
//     fallback when no turn-scoped TraceID is known.
//   - NodeID is only filled when the event does not already carry one: the
//     graph runtime stamps the authoritative executing node onto the context,
//     so a caller-supplied NodeID is preserved unless context disagrees.
//
// Emit has no context parameter (the Telemetry interface is unchanged), so
// stamping happens at the call site immediately before Emit. That is the only
// place the caller's context is reachable.
func StampCorrelation(ctx context.Context, ev *Event) {
	if ev == nil {
		return
	}
	c := observability.CorrelationFromContext(ctx)
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

// Emit forwards the event to all registered sinks. Correlation fields are
// stamped by the caller via StampCorrelation before the event reaches the
// multiplex; see the StampCorrelation doc comment for why.
func (m MultiplexTelemetry) Emit(event Event) {
	for _, s := range m.Sinks {
		s.Emit(event)
	}
}

// Close releases every sink that owns an OS resource. Only sinks whose Close
// returns an error — today *JSONFileTelemetry and nested multiplexes — are
// closed; pure fan-out sinks (LoggerTelemetry, EventTelemetry, *BroadcastSink,
// whose Close returns nothing) are skipped so their owning lifecycles are not
// ended twice. Safe to call on a multiplex whose file sinks are already closed
// because JSONFileTelemetry.Close is idempotent.
func (m MultiplexTelemetry) Close() error {
	var errs []error
	for _, s := range m.Sinks {
		if closer, ok := s.(io.Closer); ok {
			if err := closer.Close(); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

// JSONFileTelemetry writes events as newline-delimited JSON to a file.
// This allows external tools to tail and process the stream in real-time.
//
// It never panics. Encode/write failures and emits after Close are dropped and
// counted (see DroppedTotal); the first drop logs exactly once. Telemetry must
// never break the runtime, so the write path fails open rather than fatal.
type JSONFileTelemetry struct {
	path    string
	file    *os.File
	enc     *json.Encoder
	mu      sync.Mutex
	closed  bool
	dropped atomic.Uint64
	warned  atomic.Bool
}

// NewJSONFileTelemetry opens (or creates) the log file.
func NewJSONFileTelemetry(path string) (*JSONFileTelemetry, error) {
	f, err := os.OpenFile(filepath.Clean(path), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	return &JSONFileTelemetry{
		path: path,
		file: f,
		enc:  json.NewEncoder(f),
	}, nil
}

// Emit writes the JSON record. It never panics: if the sink is closed or the
// write fails, the event is dropped and counted, and the first drop is logged
// once. The mutex keeps the write synchronous (no batching) while serializing
// concurrent emitters against Close.
func (j *JSONFileTelemetry) Emit(event Event) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed || j.enc == nil {
		j.drop()
		return
	}
	if err := j.enc.Encode(event); err != nil {
		j.drop()
	}
}

// drop counts a dropped event and logs the degradation exactly once. Callers
// must hold j.mu; the counters themselves are atomic so DroppedTotal is
// lock-free.
func (j *JSONFileTelemetry) drop() {
	j.dropped.Add(1)
	if j.warned.CompareAndSwap(false, true) {
		log.Printf("telemetry: sink %s degraded, dropping events (total will accumulate)", j.path)
	}
}

// Close releases the file handle. It is idempotent: once closed, subsequent
// calls return nil and later Emits are dropped and counted rather than writing
// to a closed file.
func (j *JSONFileTelemetry) Close() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return nil
	}
	j.closed = true
	j.enc = nil
	if j.file == nil {
		return nil
	}
	err := j.file.Close()
	j.file = nil
	return err
}

// DroppedTotal returns the number of events dropped since construction. It is
// safe to call concurrently with Emit.
func (j *JSONFileTelemetry) DroppedTotal() uint64 {
	return j.dropped.Load()
}

// LoggerTelemetry emits events via the standard logger. It is intentionally
// tiny yet immensely helpful while debugging workflows locally because every
// node transition becomes visible without extra tooling.
type LoggerTelemetry struct {
	Logger *log.Logger
}

// Emit logs the event.
func (t LoggerTelemetry) Emit(event Event) {
	logger := t.Logger
	if logger == nil {
		logger = log.Default()
	}
	logger.Printf("[%s] node=%s task=%s meta=%v msg=%s\n", event.Type, event.NodeID, event.TaskID, event.Metadata, event.Message)
}

func (t LoggerTelemetry) OnArtifactPruning(taskID string, itemsRemoved int, tokensFreed int) {
	logger := t.Logger
	if logger == nil {
		logger = log.Default()
	}
	logger.Printf("[context_pruning] task=%s removed=%d tokens=%d\n", taskID, itemsRemoved, tokensFreed)
}

func (t LoggerTelemetry) OnBudgetExceeded(taskID string, attempted int, available int) {
	logger := t.Logger
	if logger == nil {
		logger = log.Default()
	}
	logger.Printf("[budget_exceeded] task=%s attempted=%d available=%d\n", taskID, attempted, available)
}

func (t LoggerTelemetry) OnCheckpointCreated(taskID string, checkpointID string, nodeID string) {
	logger := t.Logger
	if logger == nil {
		logger = log.Default()
	}
	logger.Printf("[checkpoint_created] task=%s checkpoint=%s node=%s\n", taskID, checkpointID, nodeID)
}

func (t LoggerTelemetry) OnCheckpointRestored(taskID string, checkpointID string) {
	logger := t.Logger
	if logger == nil {
		logger = log.Default()
	}
	logger.Printf("[checkpoint_restored] task=%s checkpoint=%s\n", taskID, checkpointID)
}

func (t LoggerTelemetry) OnGraphResume(taskID string, checkpointID string, nodeID string) {
	logger := t.Logger
	if logger == nil {
		logger = log.Default()
	}
	logger.Printf("[graph_resume] task=%s checkpoint=%s node=%s\n", taskID, checkpointID, nodeID)
}

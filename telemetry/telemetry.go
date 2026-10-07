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
	"log"
	"os"
	"path/filepath"
	"sync"
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

// Emit forwards the event to all registered sinks. Correlation fields are
// stamped by the caller via StampCorrelation before the event reaches the
// multiplex; see the StampCorrelation doc comment for why.
func (m MultiplexTelemetry) Emit(event Event) {
	for _, s := range m.Sinks {
		s.Emit(event)
	}
}

// JSONFileTelemetry writes events as newline-delimited JSON to a file.
// This allows external tools to tail and process the stream in real-time.
type JSONFileTelemetry struct {
	path string
	file *os.File
	enc  *json.Encoder
	mu   sync.Mutex
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

// Emit writes the JSON record.
func (j *JSONFileTelemetry) Emit(event Event) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.enc != nil {
		if err := j.enc.Encode(event); err != nil {
			panic(err)
		}
	}
}

// Close releases the file handle.
func (j *JSONFileTelemetry) Close() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.file != nil {
		return j.file.Close()
	}
	return nil
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

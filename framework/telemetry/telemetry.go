// Package telemetry provides audit logging and execution tracing for agent runs.
// It collects structured records of node executions, tool calls, and LLM interactions,
// supporting retention-based audit policies defined in the agent manifest.
package telemetry

import (
	"encoding/json"
	"log"
	"os"
	"sync"

	"github.com/lexcodex/relurpify/framework/core"
)

// MultiplexTelemetry broadcasts events to multiple sinks.
type MultiplexTelemetry struct {
	Sinks []core.Telemetry
}

// Emit forwards the event to all registered sinks.
func (m MultiplexTelemetry) Emit(event core.Event) {
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
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
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
func (j *JSONFileTelemetry) Emit(event core.Event) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.enc != nil {
		_ = j.enc.Encode(event)
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
func (t LoggerTelemetry) Emit(event core.Event) {
	logger := t.Logger
	if logger == nil {
		logger = log.Default()
	}
	logger.Printf("[%s] node=%s task=%s meta=%v msg=%s\n", event.Type, event.NodeID, event.TaskID, event.Metadata, event.Message)
}

func (t LoggerTelemetry) OnContextCompression(taskID string, stats core.CompressionStats) {
	logger := t.Logger
	if logger == nil {
		logger = log.Default()
	}
	logger.Printf("[context_compression] task=%s stats=%+v\n", taskID, stats)
}

func (t LoggerTelemetry) OnContextPruning(taskID string, itemsRemoved int, tokensFreed int) {
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

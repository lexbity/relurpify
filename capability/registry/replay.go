package registry

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/capability/ports"
)

// ToolCallRecord captures one invocation for replay.
type ToolCallRecord struct {
	Name      string            `json:"name"`
	Args      map[string]any    `json:"args"`
	Result    *ports.ToolResult `json:"result"`
	StartedAt time.Time         `json:"started_at,omitempty"`
	ElapsedMs int64             `json:"elapsed_ms,omitempty"`
}

// ErrReplayExhausted is returned by ToolPlayer when no more recorded calls
// remain.
var ErrReplayExhausted = fmt.Errorf("replay exhausted: no more recorded tool calls")

// ErrUnexpectedToolCall is returned by ToolPlayer when the next recorded call
// does not match the requested tool name.
type ErrUnexpectedToolCall struct {
	Expected string
	Got      string
}

func (e *ErrUnexpectedToolCall) Error() string {
	return fmt.Sprintf("unexpected tool call: expected %q, got %q", e.Expected, e.Got)
}

// ToolRecorder wraps a CapabilityInvoker and records every invocation as a
// JSONL entry to the configured writer.
type ToolRecorder struct {
	inner interface {
		InvokeCapability(ctx context.Context, env ports.State, idOrName string, args map[string]any) (*ports.ToolResult, error)
	}
	w io.Writer
}

// NewToolRecorder creates a recorder that delegates to inner and writes
// records to w.
func NewToolRecorder(inner interface {
	InvokeCapability(ctx context.Context, env ports.State, idOrName string, args map[string]any) (*ports.ToolResult, error)
}, w io.Writer) *ToolRecorder {
	return &ToolRecorder{inner: inner, w: w}
}

// InvokeCapability records the call and result, then delegates to the
// inner invoker.
func (r *ToolRecorder) InvokeCapability(ctx context.Context, env ports.State, idOrName string, args map[string]any) (*ports.ToolResult, error) {
	start := time.Now()
	result, err := r.inner.InvokeCapability(ctx, env, idOrName, args)
	elapsed := time.Since(start)
	record := ToolCallRecord{
		Name:      idOrName,
		Args:      copyArgs(args),
		StartedAt: start,
		ElapsedMs: elapsed.Milliseconds(),
	}
	if err != nil {
		record.Result = &ports.ToolResult{Success: false, Error: err.Error()}
	} else if result != nil {
		record.Result = result
	}
	// Best-effort write; failures to record are non-fatal.
	if data, marshalErr := json.Marshal(record); marshalErr == nil {
		_, _ = r.w.Write(data)
		_, _ = r.w.Write([]byte("\n"))
	}
	return result, err
}

// ToolPlayer replays recorded tool calls in order. It is used for
// deterministic testing and debugging without an LLM.
type ToolPlayer struct {
	records []ToolCallRecord
	pos     int
}

// NewToolPlayerFromReader reads JSONL records from r and creates a player.
func NewToolPlayerFromReader(r io.Reader) (*ToolPlayer, error) {
	var records []ToolCallRecord
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var rec ToolCallRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			return nil, fmt.Errorf("parse replay record: %w", err)
		}
		records = append(records, rec)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read replay: %w", err)
	}
	return &ToolPlayer{records: records}, nil
}

// NewToolPlayer creates a player from a static slice of records.
func NewToolPlayer(records []ToolCallRecord) *ToolPlayer {
	cp := make([]ToolCallRecord, len(records))
	copy(cp, records)
	return &ToolPlayer{records: cp}
}

// InvokeCapability returns the next recorded result for the given
// tool name. If the name doesn't match the next record, an
// ErrUnexpectedToolCall is returned.
func (p *ToolPlayer) InvokeCapability(ctx context.Context, env ports.State, idOrName string, args map[string]any) (*ports.ToolResult, error) {
	if p.pos >= len(p.records) {
		return nil, ErrReplayExhausted
	}
	rec := p.records[p.pos]
	if rec.Name != idOrName {
		return nil, &ErrUnexpectedToolCall{Expected: rec.Name, Got: idOrName}
	}
	p.pos++
	return rec.Result, nil
}

// Remaining returns the number of unplayed records.
func (p *ToolPlayer) Remaining() int {
	if p == nil {
		return 0
	}
	return len(p.records) - p.pos
}

func copyArgs(args map[string]any) map[string]any {
	if args == nil {
		return nil
	}
	out := make(map[string]any, len(args))
	for k, v := range args {
		out[k] = v
	}
	return out
}

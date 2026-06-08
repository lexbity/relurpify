package capability

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/ports"
)

type mockInvoker2 struct {
	results map[string]*ports.ToolResult
	err     error
}

func (m *mockInvoker2) InvokeCapability(ctx context.Context, env ports.State, idOrName string, args map[string]any) (*ports.ToolResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.results != nil {
		if r, ok := m.results[idOrName]; ok {
			return r, nil
		}
	}
	return &ports.ToolResult{Success: true}, nil
}

func TestToolRecorderWritesJSONL(t *testing.T) {
	var buf bytes.Buffer
	inner := &mockInvoker2{
		results: map[string]*ports.ToolResult{
			"test_tool": {Success: true, Data: map[string]any{"output": "ok"}},
		},
	}
	rec := NewToolRecorder(inner, &buf)

	_, err := rec.InvokeCapability(context.Background(), nil, "test_tool", map[string]any{"input": "hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 JSONL line, got %d", len(lines))
	}
	if !strings.Contains(lines[0], `"test_tool"`) {
		t.Fatalf("expected record to contain tool name, got: %s", lines[0])
	}
	if !strings.Contains(lines[0], `"output":"ok"`) {
		t.Fatalf("expected record to contain result, got: %s", lines[0])
	}
}

func TestToolPlayerReplaysInOrder(t *testing.T) {
	records := []ToolCallRecord{
		{Name: "first", Args: nil, Result: &ports.ToolResult{Success: true, Data: map[string]any{"val": 1}}},
		{Name: "second", Args: nil, Result: &ports.ToolResult{Success: true, Data: map[string]any{"val": 2}}},
		{Name: "third", Args: nil, Result: &ports.ToolResult{Success: false, Error: "fail"}},
	}
	player := NewToolPlayer(records)

	for i, rec := range records {
		result, err := player.InvokeCapability(context.Background(), nil, rec.Name, nil)
		if err != nil {
			t.Fatalf("call %d: unexpected error: %v", i, err)
		}
		if result.Success != rec.Result.Success {
			t.Fatalf("call %d: expected Success=%v, got %v", i, rec.Result.Success, result.Success)
		}
	}
}

func TestToolPlayerWrongNameReturnsError(t *testing.T) {
	player := NewToolPlayer([]ToolCallRecord{
		{Name: "foo", Result: &ports.ToolResult{Success: true}},
	})

	_, err := player.InvokeCapability(context.Background(), nil, "bar", nil)
	if err == nil {
		t.Fatal("expected error for wrong tool name")
	}
	var unexpected *ErrUnexpectedToolCall
	if !errors.As(err, &unexpected) {
		t.Fatalf("expected ErrUnexpectedToolCall, got: %T(%v)", err, err)
	}
	if unexpected.Expected != "foo" || unexpected.Got != "bar" {
		t.Fatalf("expected foo/bar, got %s/%s", unexpected.Expected, unexpected.Got)
	}
}

func TestToolPlayerExhaustedReturnsError(t *testing.T) {
	player := NewToolPlayer([]ToolCallRecord{
		{Name: "only", Result: &ports.ToolResult{Success: true}},
	})

	_, err := player.InvokeCapability(context.Background(), nil, "only", nil)
	if err != nil {
		t.Fatalf("first call should succeed: %v", err)
	}

	_, err = player.InvokeCapability(context.Background(), nil, "only", nil)
	if err != ErrReplayExhausted {
		t.Fatalf("expected ErrReplayExhausted, got: %v", err)
	}
}

func TestToolRecorderPlayerRoundTrip(t *testing.T) {
	inner := &mockInvoker2{
		results: map[string]*ports.ToolResult{
			"tool_a": {Success: true, Data: map[string]any{"result": "a"}},
			"tool_b": {Success: false, Error: "b failed"},
		},
	}

	var buf bytes.Buffer
	rec := NewToolRecorder(inner, &buf)

	_, _ = rec.InvokeCapability(context.Background(), nil, "tool_a", map[string]any{"x": 1})
	_, _ = rec.InvokeCapability(context.Background(), nil, "tool_b", map[string]any{"y": 2})

	player, err := NewToolPlayerFromReader(strings.NewReader(buf.String()))
	if err != nil {
		t.Fatalf("NewToolPlayerFromReader: %v", err)
	}

	// Replay first call
	result, err := player.InvokeCapability(context.Background(), nil, "tool_a", nil)
	if err != nil {
		t.Fatalf("replay tool_a: %v", err)
	}
	if result.Data["result"] != "a" {
		t.Fatalf("expected result='a', got %v", result.Data["result"])
	}

	// Replay second call
	result, err = player.InvokeCapability(context.Background(), nil, "tool_b", nil)
	if err != nil {
		t.Fatalf("replay tool_b: %v", err)
	}
	if result.Error != "b failed" {
		t.Fatalf("expected error='b failed', got %q", result.Error)
	}
}

func TestToolRecorderRecordsErrors(t *testing.T) {
	inner := &mockInvoker2{err: innerError("something went wrong")}
	var buf bytes.Buffer
	rec := NewToolRecorder(inner, &buf)

	_, err := rec.InvokeCapability(context.Background(), nil, "failing", nil)
	if err == nil {
		t.Fatal("expected error")
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	if !strings.Contains(lines[0], "something went wrong") {
		t.Fatalf("expected error in record, got: %s", lines[0])
	}
}

type innerError string

func (e innerError) Error() string { return string(e) }

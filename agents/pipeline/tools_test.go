package pipeline

import (
	"context"
	"errors"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// mockInvoker implements CapabilityInvoker for testing.
type mockInvoker struct {
	results map[string]*contracts.ToolResult
	err     map[string]error
}

func (m *mockInvoker) InvokeCapability(ctx context.Context, env *contextdata.Envelope, idOrName string, args map[string]any) (*contracts.ToolResult, error) {
	if m.err != nil {
		if e, ok := m.err[idOrName]; ok && e != nil {
			return nil, e
		}
	}
	if m.results != nil {
		if r, ok := m.results[idOrName]; ok {
			return r, nil
		}
	}
	return &contracts.ToolResult{Success: true}, nil
}

func TestPipelineToolWithoutInvokerReturnsHardError(t *testing.T) {
	calls := []contracts.ToolCall{
		{Name: "test_tool", Args: map[string]any{"input": "hello"}},
	}
	tools := []contracts.Tool{
		&fakeTool{name: "test_tool"},
	}

	observations, err := executeToolCalls(context.Background(), nil, calls, tools, nil)
	if err == nil {
		t.Fatal("expected error when invoker is nil, got nil")
	}
	if !contains(err.Error(), "capability invoker required") {
		t.Fatalf("expected error about capability invoker, got: %v", err)
	}
	if len(observations) != 0 {
		t.Fatalf("expected no observations when invoker is nil, got %d", len(observations))
	}
}

func TestPipelineToolWithInvokerSucceeds(t *testing.T) {
	calls := []contracts.ToolCall{
		{Name: "test_tool", Args: map[string]any{"input": "world"}},
	}
	tools := []contracts.Tool{
		&fakeTool{name: "test_tool"},
	}
	invoker := &mockInvoker{
		results: map[string]*contracts.ToolResult{
			"test_tool": {Success: true, Data: map[string]any{"output": "ok"}},
		},
	}

	observations, err := executeToolCalls(context.Background(), nil, calls, tools, invoker)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(observations) != 1 {
		t.Fatalf("expected 1 observation, got %d", len(observations))
	}
	if !observations[0].Success {
		t.Fatal("expected success")
	}
	if observations[0].Data["output"] != "ok" {
		t.Fatalf("expected output 'ok', got %v", observations[0].Data["output"])
	}
}

func TestPipelineToolWithInvokerError(t *testing.T) {
	calls := []contracts.ToolCall{
		{Name: "fail_tool", Args: map[string]any{}},
	}
	tools := []contracts.Tool{
		&fakeTool{name: "fail_tool"},
	}
	invoker := &mockInvoker{
		err: map[string]error{
			"fail_tool": errors.New("invoker error"),
		},
	}

	observations, err := executeToolCalls(context.Background(), nil, calls, tools, invoker)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !contains(err.Error(), "invoker error") {
		t.Fatalf("expected 'invoker error', got: %v", err)
	}
	if len(observations) != 0 {
		t.Fatalf("expected no observations on error, got %d", len(observations))
	}
}

func TestPipelineToolUnknownNameReturnsError(t *testing.T) {
	calls := []contracts.ToolCall{
		{Name: "unknown_tool", Args: map[string]any{}},
	}
	tools := []contracts.Tool{
		&fakeTool{name: "allowed_tool"},
	}
	invoker := &mockInvoker{}

	observations, err := executeToolCalls(context.Background(), nil, calls, tools, invoker)
	if err == nil {
		t.Fatal("expected error for unknown tool, got nil")
	}
	if !contains(err.Error(), "not allowed") {
		t.Fatalf("expected 'not allowed' error, got: %v", err)
	}
	if len(observations) != 0 {
		t.Fatalf("expected no observations, got %d", len(observations))
	}
}

func TestPipelineToolEmptyCalls(t *testing.T) {
	observations, err := executeToolCalls(context.Background(), nil, nil, []contracts.Tool{}, nil)
	if err != nil {
		t.Fatalf("expected no error for empty calls, got: %v", err)
	}
	if len(observations) != 0 {
		t.Fatalf("expected no observations, got %d", len(observations))
	}
}

// fakeTool implements contracts.Tool for testing.
type fakeTool struct {
	name string
}

func (f *fakeTool) Name() string                           { return f.name }
func (f *fakeTool) Description() string                    { return "fake tool for testing" }
func (f *fakeTool) Category() string                       { return "test" }
func (f *fakeTool) Parameters() []contracts.ToolParameter  { return nil }
func (f *fakeTool) Execute(ctx context.Context, args map[string]interface{}) (*contracts.ToolResult, error) {
	return &contracts.ToolResult{Success: true}, nil
}
func (f *fakeTool) IsAvailable(ctx context.Context) bool    { return true }
func (f *fakeTool) Permissions() contracts.ToolPermissions {
	return contracts.ToolPermissions{Permissions: &contracts.PermissionSet{
		Executables: []contracts.ExecutablePermission{
			{Binary: "echo"},
		},
	}}
}
func (f *fakeTool) Tags() []string { return nil }

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

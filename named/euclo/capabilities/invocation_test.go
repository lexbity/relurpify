package capabilities

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

type mockRegistry struct {
	args map[string]interface{}
}

func (m *mockRegistry) InvokeCapability(ctx context.Context, state *contextdata.Envelope, idOrName string, args map[string]interface{}) (*contracts.ToolResult, error) {
	m.args = args
	return &contracts.ToolResult{Success: true, Data: map[string]interface{}{}}, nil
}

func TestInvokeCapability_ExtractsArgsFromTask(t *testing.T) {
	registry := &mockRegistry{}
	task := &core.Task{
		Data: map[string]any{
			"path":    "/tmp/test.txt",
			"content": "hello",
		},
	}

	result, err := InvokeCapability(context.Background(), "test_cap", task, nil, registry)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	if result.Success != true {
		t.Fatalf("expected success=true, got %v", result.Success)
	}

	// Verify args were extracted and passed to registry
	if len(registry.args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(registry.args))
	}
	if registry.args["path"] != "/tmp/test.txt" {
		t.Fatalf("expected path=/tmp/test.txt, got %v", registry.args["path"])
	}
	if registry.args["content"] != "hello" {
		t.Fatalf("expected content=hello, got %v", registry.args["content"])
	}
}

func TestInvokeCapability_NilTask(t *testing.T) {
	registry := &mockRegistry{}

	result, err := InvokeCapability(context.Background(), "test_cap", nil, nil, registry)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	if result.Success != true {
		t.Fatalf("expected success=true, got %v", result.Success)
	}

	// Verify empty args were passed (backward compatibility)
	if len(registry.args) != 0 {
		t.Fatalf("expected 0 args for nil task, got %d", len(registry.args))
	}
}

func TestInvokeCapability_TaskContext(t *testing.T) {
	registry := &mockRegistry{}
	task := &core.Task{
		Data:    map[string]any{},
		Context: map[string]any{
			"query": "test",
		},
	}

	result, err := InvokeCapability(context.Background(), "test_cap", task, nil, registry)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	if result.Success != true {
		t.Fatalf("expected success=true, got %v", result.Success)
	}

	// Verify args were extracted from Context when Data is empty
	if len(registry.args) != 1 {
		t.Fatalf("expected 1 arg from context, got %d", len(registry.args))
	}
	if registry.args["query"] != "test" {
		t.Fatalf("expected query=test, got %v", registry.args["query"])
	}
}

func TestInvokeCapability_TaskDataPriority(t *testing.T) {
	registry := &mockRegistry{}
	task := &core.Task{
		Data: map[string]any{
			"query": "from_data",
		},
		Context: map[string]any{
			"query": "from_context",
		},
	}

	result, err := InvokeCapability(context.Background(), "test_cap", task, nil, registry)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}

	// Verify Data takes priority over Context
	if registry.args["query"] != "from_data" {
		t.Fatalf("expected query=from_data (Data priority), got %v", registry.args["query"])
	}
}

func TestInvokeCapability_NilRegistry(t *testing.T) {
	task := &core.Task{
		Data: map[string]any{
			"path": "/tmp/test.txt",
		},
	}

	result, err := InvokeCapability(context.Background(), "test_cap", task, nil, nil)

	if result == nil {
		t.Fatal("expected result, got nil")
	}
	if err == nil {
		t.Fatal("expected error")
	}
	if result.Success != false {
		t.Fatalf("expected success=false, got %v", result.Success)
	}
	if got, ok := core.ResultField(result.Data, "error"); !ok || got != "capability registry unavailable" {
		t.Fatalf("expected registry error, got %v", got)
	}
}

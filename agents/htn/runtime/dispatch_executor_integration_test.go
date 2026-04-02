//go:build integration

package runtime_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	htnruntime "github.com/lexcodex/relurpify/agents/htn/runtime"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
)

type instructionTool struct {
	name string
}

func (t instructionTool) Name() string        { return t.name }
func (t instructionTool) Description() string { return "returns the instruction argument" }
func (t instructionTool) Category() string    { return "test" }
func (t instructionTool) Parameters() []core.ToolParameter {
	return []core.ToolParameter{{Name: "instruction", Type: "string", Required: false}}
}
func (t instructionTool) Execute(_ context.Context, _ *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
	return &core.ToolResult{
		Success: true,
		Data:    map[string]interface{}{"echo": strings.TrimSpace(stringValue(args["instruction"]))},
	}, nil
}
func (t instructionTool) IsAvailable(context.Context, *core.Context) bool { return true }
func (t instructionTool) Permissions() core.ToolPermissions               { return core.ToolPermissions{} }
func (t instructionTool) Tags() []string                                  { return nil }

func TestPrimitiveDispatcher_ExecutesRegisteredTool(t *testing.T) {
	registry := capability.NewRegistry()
	if err := registry.Register(instructionTool{name: "echo"}); err != nil {
		t.Fatalf("register tool: %v", err)
	}

	dispatcher := htnruntime.NewPrimitiveDispatcher(registry, nil)
	task := &core.Task{
		ID:          "task-dispatch",
		Instruction: "hello",
		Context: map[string]any{
			"current_step": core.PlanStep{
				ID:   "method.echo",
				Tool: "echo",
				Params: map[string]any{
					"operator_executor": "echo",
					"operator_name":     "echo",
				},
			},
		},
	}

	result, err := dispatcher.Execute(context.Background(), task, core.NewContext())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected success result, got %+v", result)
	}
	if got := stringValue(result.Data["echo"]); got != "hello" {
		t.Fatalf("result.Data[echo] = %q, want hello", got)
	}
}

func TestPrimitiveDispatcher_NilRegistryFallsBackOnNilFallback(t *testing.T) {
	dispatcher := htnruntime.NewPrimitiveDispatcher(nil, nil)
	task := &core.Task{
		ID:          "task-dispatch-missing",
		Instruction: "hello",
		Context: map[string]any{
			"current_step": core.PlanStep{
				ID:   "method.echo",
				Tool: "echo",
				Params: map[string]any{
					"operator_executor": "echo",
					"operator_name":     "echo",
				},
			},
		},
	}

	result, err := dispatcher.Execute(context.Background(), task, core.NewContext())
	if err == nil {
		t.Fatal("expected error")
	}
	if result != nil {
		t.Fatalf("expected nil result on error, got %+v", result)
	}
	if !strings.Contains(err.Error(), "no primitive dispatch target available") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func stringValue(raw any) string {
	if raw == nil {
		return ""
	}
	value := strings.TrimSpace(fmt.Sprint(raw))
	if value == "<nil>" {
		return ""
	}
	return value
}

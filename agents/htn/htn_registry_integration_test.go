//go:build integration

package htn_test

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/agents/htn"
	htnruntime "github.com/lexcodex/relurpify/agents/htn/runtime"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	testutil "github.com/lexcodex/relurpify/testutil/euclotestutil"
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
	return &core.ToolResult{Success: true, Data: map[string]interface{}{"echo": args["instruction"]}}, nil
}
func (t instructionTool) IsAvailable(context.Context, *core.Context) bool { return true }
func (t instructionTool) Permissions() core.ToolPermissions               { return core.ToolPermissions{} }
func (t instructionTool) Tags() []string                                  { return nil }

func TestHTNAgent_PrimitiveDispatch_CapabilityResolution(t *testing.T) {
	registry := capability.NewRegistry()
	if err := registry.Register(instructionTool{name: "echo"}); err != nil {
		t.Fatalf("register tool: %v", err)
	}

	methods := &htn.MethodLibrary{}
	methods.Register(htn.Method{
		Name:     "integration-echo",
		TaskType: core.TaskTypeCodeGeneration,
		Subtasks: []htn.SubtaskSpec{{
			Name:        "echo",
			Type:        core.TaskTypeCodeGeneration,
			Instruction: "hello",
			Executor:    "echo",
		}},
	})

	agent := &htn.HTNAgent{
		Config:        &core.Config{Name: "htn-integration"},
		Tools:         registry,
		Methods:       methods,
		PrimitiveExec: htnruntime.NewPrimitiveDispatcher(registry, &testutil.NoopExecutor{}),
	}
	result, err := agent.Execute(context.Background(), &core.Task{
		ID:          "htn-integration-echo",
		Type:        core.TaskTypeCodeGeneration,
		Instruction: "implement feature",
		Context:     map[string]any{"workflow_id": "wf-htn-capability"},
	}, core.NewContext())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected success result, got %+v", result)
	}
	state := core.NewContext()
	result, err = agent.Execute(context.Background(), &core.Task{
		ID:          "htn-integration-echo",
		Type:        core.TaskTypeCodeGeneration,
		Instruction: "implement feature",
		Context:     map[string]any{"workflow_id": "wf-htn-capability"},
	}, state)
	if err != nil {
		t.Fatalf("Execute with state: %v", err)
	}
	lastDispatch, ok := state.Get("htn.execution.last_dispatch")
	if !ok {
		t.Fatal("expected last dispatch state")
	}
	dispatch, ok := lastDispatch.(map[string]any)
	if !ok {
		t.Fatalf("expected dispatch map, got %T", lastDispatch)
	}
	if dispatch["mode"] != "capability" {
		t.Fatalf("mode = %v, want capability", dispatch["mode"])
	}
	if dispatch["resolved_target"] != "echo" {
		t.Fatalf("resolved_target = %v, want echo", dispatch["resolved_target"])
	}
}

func TestHTNAgent_PrimitiveDispatch_FallsBackOnMissingCapability(t *testing.T) {
	fallback := &testutil.NoopExecutor{}
	methods := &htn.MethodLibrary{}
	methods.Register(htn.Method{
		Name:     "integration-fallback",
		TaskType: core.TaskTypeCodeGeneration,
		Subtasks: []htn.SubtaskSpec{{
			Name:        "echo",
			Type:        core.TaskTypeCodeGeneration,
			Instruction: "hello",
			Executor:    "echo",
		}},
	})

	agent := &htn.HTNAgent{
		Config:        &core.Config{Name: "htn-integration"},
		Tools:         capability.NewRegistry(),
		Methods:       methods,
		PrimitiveExec: htnruntime.NewPrimitiveDispatcher(capability.NewRegistry(), fallback),
	}
	result, err := agent.Execute(context.Background(), &core.Task{
		ID:          "htn-integration-fallback",
		Type:        core.TaskTypeCodeGeneration,
		Instruction: "implement feature",
	}, core.NewContext())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected success result, got %+v", result)
	}
	if fallback.Calls != 1 {
		t.Fatalf("fallback calls = %d, want 1", fallback.Calls)
	}
}

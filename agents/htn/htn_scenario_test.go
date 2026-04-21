//go:build scenario

package htn_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/agents/htn"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	agenttestscenario "codeburg.org/lexbit/relurpify/testutil/agenttestscenario"
)

func TestHTNAgent_Scenario_CodeGenerationDecomposesToPlanAndCode(t *testing.T) {
	f := agenttestscenario.NewFixture(t)
	f.Env.Registry = capability.NewRegistry()

	methods := &htn.MethodLibrary{}
	methods.Register(htn.Method{
		Name:     "scenario-two-step",
		TaskType: core.TaskTypeCodeGeneration,
		Subtasks: []htn.SubtaskSpec{
			{Name: "plan", Type: core.TaskTypePlanning, Instruction: "Plan {{.Instruction}}"},
			{Name: "code", Type: core.TaskTypeCodeGeneration, Instruction: "Code {{.Instruction}}", DependsOn: []string{"plan"}},
		},
	})

	agent := htn.New(f.Env, methods, htn.WithPrimitiveExec(f.Exec))
	result, err := agent.Execute(context.Background(), &core.Task{
		ID:          "htn-scenario-plan-code",
		Type:        core.TaskTypeCodeGeneration,
		Instruction: "implement feature",
	}, core.NewContext())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	agenttestscenario.RequireResultSuccess(t, result)
	agenttestscenario.RequireExecutorCallCount(t, f, 2)
	agenttestscenario.Require(t, len(f.Exec.Tasks) == 2, "expected 2 delegated tasks, got %d", len(f.Exec.Tasks))
	agenttestscenario.Require(t, f.Exec.Tasks[0].Type == core.TaskTypePlanning, "expected first task to be planning, got %q", f.Exec.Tasks[0].Type)
	agenttestscenario.Require(t, f.Exec.Tasks[1].Type == core.TaskTypeCodeGeneration, "expected second task to be code_generation, got %q", f.Exec.Tasks[1].Type)
}

func TestHTNAgent_Scenario_CheckpointStoreIsPopulated(t *testing.T) {
	f := agenttestscenario.NewFixture(t)
	f.Env.Registry = capability.NewRegistry()

	methods := &htn.MethodLibrary{}
	methods.Register(htn.Method{
		Name:     "scenario-checkpoint",
		TaskType: core.TaskTypeCodeGeneration,
		Subtasks: []htn.SubtaskSpec{
			{Name: "plan", Type: core.TaskTypePlanning, Instruction: "Plan {{.Instruction}}"},
			{Name: "code", Type: core.TaskTypeCodeGeneration, Instruction: "Code {{.Instruction}}", DependsOn: []string{"plan"}},
		},
	})

	checkpointPath := filepath.Join(t.TempDir(), "htn-workflow.db")
	agent := htn.New(f.Env, methods, htn.WithPrimitiveExec(f.Exec))
	agent.CheckpointPath = checkpointPath

	result, err := agent.Execute(context.Background(), &core.Task{
		ID:          "htn-scenario-checkpoint",
		Type:        core.TaskTypeCodeGeneration,
		Instruction: "implement feature",
		Context:     map[string]any{"workflow_id": "wf-htn-scenario"},
	}, core.NewContext())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	agenttestscenario.RequireResultSuccess(t, result)
	info, err := os.Stat(checkpointPath)
	if err != nil {
		t.Fatalf("stat checkpoint path: %v", err)
	}
	agenttestscenario.Require(t, info.Size() > 0, "expected checkpoint db to be populated")
}

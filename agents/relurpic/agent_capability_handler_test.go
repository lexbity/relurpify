package relurpic

import (
	"errors"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
)

func TestTaskFromArgsHonorsExplicitTaskType(t *testing.T) {
	task := taskFromArgs("react", map[string]interface{}{
		"task_id":     "step-1",
		"instruction": "inspect the bug",
		"task_type":   string(core.TaskTypeAnalysis),
	})
	if task.Type != core.TaskTypeAnalysis {
		t.Fatalf("expected task type %q, got %q", core.TaskTypeAnalysis, task.Type)
	}
}

func TestToToolResultPreservesErrorAndMetadata(t *testing.T) {
	result := toToolResult(&core.Result{
		Success: false,
		Data: map[string]any{
			"summary": "incomplete",
		},
		Metadata: map[string]any{
			"source": "react",
		},
		Error: errors.New("missing final answer"),
	})
	if result.Success {
		t.Fatal("expected unsuccessful capability result")
	}
	if result.Error != "missing final answer" {
		t.Fatalf("expected error to propagate, got %q", result.Error)
	}
	if result.Metadata["source"] != "react" {
		t.Fatalf("expected metadata to propagate, got %#v", result.Metadata)
	}
	if result.Data["error"] != "missing final answer" {
		t.Fatalf("expected payload error to be retained, got %#v", result.Data["error"])
	}
}

func TestSeedTaskStateOverridesDelegatedTaskMetadata(t *testing.T) {
	state := core.NewContext()
	state.Set("task.type", string(core.TaskTypeCodeModification))
	state.Set("task.instruction", "parent task")

	seedTaskState(state, &core.Task{
		ID:          "child-step",
		Type:        core.TaskTypeAnalysis,
		Instruction: "child task",
	})

	if got := state.GetString("task.type"); got != string(core.TaskTypeAnalysis) {
		t.Fatalf("expected delegated task type to override parent, got %q", got)
	}
	if got := state.GetString("task.instruction"); got != "child task" {
		t.Fatalf("expected delegated instruction to override parent, got %q", got)
	}
	if got := state.GetString("task.id"); got != "child-step" {
		t.Fatalf("expected delegated task id to be seeded, got %q", got)
	}
}

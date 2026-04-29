package envelope

import (
	"testing"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
)

func TestNormalizeFromTaskAndState(t *testing.T) {
	env2 := contextdata.NewEnvelope("task-1", "")
	env2.SetWorkingValue("rex.workflow_id", "wf-1", contextdata.MemoryClassTask)
	task := &core.Task{
		ID:          "task-1",
		Instruction: "review this code",
		Context: map[string]any{
			"workspace":      "/tmp/work",
			"mode_hint":      "review",
			"source":         "nexus",
			"edit_permitted": false,
		},
	}
	env := Normalize(task, env2)
	if env.TaskID != "task-1" || env.WorkflowID != "wf-1" {
		t.Fatalf("unexpected envelope: %+v", env)
	}
	if env.Source != "nexus" || env.ModeHint != "review" {
		t.Fatalf("unexpected routing fields: %+v", env)
	}
}

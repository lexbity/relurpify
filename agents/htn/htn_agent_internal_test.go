package htn

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/agents/htn/runtime"
	"codeburg.org/lexbit/relurpify/framework/core"
)

func TestCompactHTNCheckpoint(t *testing.T) {
	value := compactHTNCheckpoint(runtime.CheckpointState{
		SchemaVersion:  1,
		CheckpointID:   "cp-1",
		StageName:      "dispatch",
		StageIndex:     2,
		WorkflowID:     "wf-1",
		RunID:          "run-1",
		CompletedSteps: []string{"a", "b"},
		Snapshot:       &runtime.HTNState{},
	})

	if got := value["checkpoint_id"]; got != "cp-1" {
		t.Fatalf("unexpected checkpoint_id: %#v", got)
	}
	if got := value["completed_steps"]; got != 2 {
		t.Fatalf("unexpected completed_steps count: %#v", got)
	}
	if got := value["has_snapshot"]; got != true {
		t.Fatalf("unexpected has_snapshot: %#v", got)
	}
}

func TestCompactHTNCheckpointStateWhenArtifactRefExists(t *testing.T) {
	state := core.NewContext()
	state.Set(runtime.ContextKeyCheckpoint, runtime.CheckpointState{
		SchemaVersion:  1,
		CheckpointID:   "cp-1",
		StageName:      "dispatch",
		StageIndex:     2,
		WorkflowID:     "wf-1",
		RunID:          "run-1",
		CompletedSteps: []string{"a", "b"},
		Snapshot:       &runtime.HTNState{},
	})
	state.Set(runtime.ContextKeyCheckpointRef, core.ArtifactReference{
		ArtifactID: "artifact-1",
		Kind:       "htn_checkpoint",
	})

	compactHTNCheckpointState(state)

	raw, ok := state.Get(runtime.ContextKeyCheckpoint)
	if !ok {
		t.Fatal("expected compacted htn.checkpoint")
	}
	payload, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("expected compacted checkpoint map, got %#v", raw)
	}
	if got := payload["checkpoint_id"]; got != "cp-1" {
		t.Fatalf("unexpected checkpoint_id: %#v", got)
	}
	if got := payload["completed_steps"]; got != 2 {
		t.Fatalf("unexpected completed_steps count: %#v", got)
	}
	if got := payload["has_snapshot"]; got != true {
		t.Fatalf("unexpected has_snapshot: %#v", got)
	}
}

// --- afterStep tests --------------------------------------------------------

func TestAfterStep_TracksCompletedStepsInState(t *testing.T) {
	a := &HTNAgent{}
	state := core.NewContext()
	step := core.PlanStep{ID: "step-a", Tool: "react"}
	a.afterStep(context.Background(), step, state, &core.Result{Success: true}, nil, map[string]int{"step-a": 0}, nil, "", "", nil)

	raw, ok := state.Get("plan.completed_steps")
	if !ok {
		t.Fatal("expected plan.completed_steps in state")
	}
	steps, ok := raw.([]string)
	if !ok || len(steps) != 1 || steps[0] != "step-a" {
		t.Fatalf("unexpected completed steps: %v", raw)
	}
}

func TestAfterStep_DeduplicatesStepID(t *testing.T) {
	a := &HTNAgent{}
	state := core.NewContext()
	state.Set("plan.completed_steps", []string{"step-a"})
	step := core.PlanStep{ID: "step-a"}
	a.afterStep(context.Background(), step, state, &core.Result{Success: true}, nil, map[string]int{}, nil, "", "", nil)

	raw, _ := state.Get("plan.completed_steps")
	steps, _ := raw.([]string)
	if len(steps) != 1 {
		t.Fatalf("expected 1 completed step (no dup), got %d: %v", len(steps), steps)
	}
}

func TestAfterStep_PublishesExecutionState(t *testing.T) {
	a := &HTNAgent{}
	state := core.NewContext()
	step := core.PlanStep{ID: "step-b"}
	a.afterStep(context.Background(), step, state, &core.Result{Success: true}, nil, map[string]int{}, nil, "", "", nil)

	raw, ok := state.Get("htn.execution")
	if !ok {
		t.Fatal("expected execution state published after afterStep")
	}
	exec, ok := raw.(runtime.ExecutionState)
	if !ok {
		t.Fatalf("expected ExecutionState, got %T", raw)
	}
	if len(exec.CompletedSteps) != 1 || exec.CompletedSteps[0] != "step-b" {
		t.Fatalf("unexpected execution completed steps: %v", exec.CompletedSteps)
	}
}

func TestAfterStep_AccumulatesMultipleSteps(t *testing.T) {
	a := &HTNAgent{}
	state := core.NewContext()
	for _, id := range []string{"s1", "s2", "s3"} {
		a.afterStep(context.Background(), core.PlanStep{ID: id}, state, &core.Result{Success: true}, nil, map[string]int{}, nil, "", "", nil)
	}
	raw, _ := state.Get("plan.completed_steps")
	steps, _ := raw.([]string)
	if len(steps) != 3 {
		t.Fatalf("expected 3 completed steps, got %d: %v", len(steps), steps)
	}
}

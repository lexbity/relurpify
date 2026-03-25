package htn

import (
	"testing"

	"github.com/lexcodex/relurpify/agents/htn/runtime"
	"github.com/lexcodex/relurpify/framework/core"
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

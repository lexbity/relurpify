package core

import (
	"encoding/json"
	"testing"
)

func TestContextSnapshotHandlesAndBranchDeltas(t *testing.T) {
	ctx := NewContext()
	ctx.Set("shared", map[string]any{"nested": "value"})
	ctx.SetVariable("temp", "scratch")
	ctx.SetKnowledge("fact", "known")
	ctx.SetExecutionPhase("running")
	ctx.AddInteraction("user", "hello", map[string]any{"x": 1})
	ctx.AppendCompressedContext(CompressedContext{Summary: "summary"})

	type payload struct{ Value string }
	handle := payload{Value: "kept"}
	handleID := ctx.SetHandle("handle", handle)
	if handleID == "" {
		t.Fatal("expected handle id")
	}
	scopedID := ctx.SetHandleScoped("scoped", payload{Value: "scoped"}, "scope-1")
	if scopedID == "" {
		t.Fatal("expected scoped handle id")
	}

	got, ok := ctx.GetHandle("handle")
	if !ok {
		t.Fatal("expected handle lookup")
	}
	if got.(payload).Value != "kept" {
		t.Fatalf("unexpected handle payload: %#v", got)
	}
	ctx.ClearHandleScope("scope-1")
	if _, ok := ctx.GetHandle("scoped"); ok {
		t.Fatal("expected scoped handle to be cleared")
	}

	snapshot := ctx.Snapshot()
	if snapshot.Phase != "running" {
		t.Fatalf("unexpected snapshot phase: %q", snapshot.Phase)
	}
	snapshotJSON, err := json.Marshal(ctx)
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}

	restored := NewContext()
	if err := restored.UnmarshalJSON(snapshotJSON); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if restored.ExecutionPhase() != "running" {
		t.Fatalf("unexpected restored phase: %q", restored.ExecutionPhase())
	}
	if restored.GetString("shared") == "" {
		t.Fatal("expected restored state")
	}

	fromSnapshot := NewContextFromSnapshot(snapshot, ctx.Registry())
	if fromSnapshot.GetString("shared") == "" {
		t.Fatal("expected snapshot restored state")
	}
	if _, ok := fromSnapshot.GetHandle("handle"); !ok {
		t.Fatal("expected registry handle to survive snapshot rebuild")
	}

	dirty := ctx.DirtyDelta()
	if len(dirty.StateWrites) == 0 || len(dirty.SideEffects.VariableWrites) == 0 || len(dirty.SideEffects.KnowledgeWrites) == 0 {
		t.Fatalf("expected dirty delta to capture writes: %+v", dirty)
	}
	if !dirty.SideEffects.HistoryChanged || !dirty.SideEffects.CompressedChanged || !dirty.SideEffects.PhaseChanged {
		t.Fatalf("expected dirty flags to be set: %+v", dirty.SideEffects)
	}
	branch := ctx.BranchDelta()
	if len(branch.StateWrites) == 0 {
		t.Fatal("expected branch delta to include writes")
	}

	okSet := NewBranchDeltaSet(2)
	okSet.Add("branch-a", BranchContextDelta{StateWrites: map[string]any{"branch": "value"}})
	okSet.Add("branch-b", BranchContextDelta{StateWrites: map[string]any{"branch": "value"}})
	if err := ctx.ApplyBranchDeltaSet(okSet); err != nil {
		t.Fatalf("ApplyBranchDeltaSet: %v", err)
	}
	if got := ctx.GetString("branch"); got != "value" {
		t.Fatalf("unexpected merged branch value: %q", got)
	}

	conflictSet := NewBranchDeltaSet(2)
	conflictSet.Add("branch-a", BranchContextDelta{StateWrites: map[string]any{"conflict": "a"}})
	conflictSet.Add("branch-b", BranchContextDelta{StateWrites: map[string]any{"conflict": "b"}})
	if err := ctx.ApplyBranchDeltaSet(conflictSet); err == nil {
		t.Fatal("expected branch delta conflict")
	}
	if err := ctx.ApplyBranchDeltas(map[string]BranchContextDelta{}); err != nil {
		t.Fatalf("ApplyBranchDeltas(empty): %v", err)
	}
}

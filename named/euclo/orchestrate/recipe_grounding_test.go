package orchestrate

import (
	"context"
	"errors"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	"codeburg.org/lexbit/relurpify/named/euclo/grounding"
	euclostate "codeburg.org/lexbit/relurpify/named/euclo/state"
)

// fakeRegroundSource is a deterministic StateRegroundSource for the dispatch
// restore tests.
type fakeRegroundSource struct {
	result grounding.RegroundResult
	err    error
	req    grounding.RegroundRequest
}

func (f *fakeRegroundSource) Reground(ctx context.Context, req grounding.RegroundRequest) (grounding.RegroundResult, error) {
	f.req = req
	return f.result, f.err
}

// TestStateRegroundDispatchRestoresEntries is AC-20's happy path: two entries
// from a composed StateRegroundSource are restored through the capture path
// with origin preserved and ChunkIDs recorded in run metadata.
func TestStateRegroundDispatchRestoresEntries(t *testing.T) {
	now := time.Now().UTC()
	src := &fakeRegroundSource{result: grounding.RegroundResult{
		SourceRunID: "run-prior-1",
		Grounded:    true,
		Entries: []grounding.RegroundEntry{
			{StateKey: "state.alpha", Value: "value-a", Origin: "user", Epistemics: "given", ChunkID: "chunk:capture:aabb", SourceRunID: "run-prior-1", GroundedAt: now},
			{StateKey: "state.beta", Value: map[string]any{"n": 2}, Origin: "tool", Epistemics: "claimed", ChunkID: "chunk:capture:ccdd", SourceRunID: "run-prior-1", GroundedAt: now},
		},
	}}

	node := NewThoughtRecipeExecutorNode("euclo.execute_thoughtrecipe").
		WithWorkspace("ws-1").
		WithStateReground(src)
	env := contextdata.NewEnvelope("task-run", "session-1")
	env.SetWorkingValueWithClass("input.workspace", "ws-1", contextdata.MemoryClassTask)

	restoreResult, stop := node.restoreState(context.Background(), env, "euclo.thoughtrecipe.debug")
	if stop {
		t.Fatalf("restore unexpectedly stopped the recipe: %+v", restoreResult)
	}
	if restoreResult != nil {
		t.Fatalf("restore at cold path must not return a result, got %+v", restoreResult)
	}

	if got, ok := contextdata.GetTyped[string](env, "state.alpha"); !ok || got != "value-a" {
		t.Fatalf("state.alpha = %#v (ok=%v), want value-a", got, ok)
	}
	if got := env.OriginOf("state.alpha"); got != contextdata.OriginUser {
		t.Fatalf("state.alpha origin = %q, want user (origin preserved)", got)
	}
	beta, ok := contextdata.GetTyped[map[string]any](env, "state.beta")
	if !ok || beta["n"] != 2 {
		t.Fatalf("state.beta = %#v (ok=%v), want {n:2}", beta, ok)
	}
	if got := env.OriginOf("state.beta"); got != contextdata.OriginTool {
		t.Fatalf("state.beta origin = %q, want tool", got)
	}

	chunkIDs, ok := contextdata.GetTyped[[]string](env, regroundChunkIDsKey)
	if !ok || len(chunkIDs) != 2 {
		t.Fatalf("regrounded chunk_ids = %#v (ok=%v), want 2 chunk IDs", chunkIDs, ok)
	}
	if src.req.RecipeID != "euclo.thoughtrecipe.debug" || src.req.WorkspaceID != "ws-1" || src.req.SessionID != "session-1" {
		t.Fatalf("reground request = %+v, want scoped recipe/workspace/session", src.req)
	}
}

// TestStateRegroundColdStartNil is AC-20's nil-mode: without a composed source
// the dispatch starts cold and writes nothing.
func TestStateRegroundColdStartNil(t *testing.T) {
	node := NewThoughtRecipeExecutorNode("euclo.execute_thoughtrecipe").WithWorkspace("ws-1")
	env := contextdata.NewEnvelope("task-cold", "session-cold")

	restoreResult, stop := node.restoreState(context.Background(), env, "euclo.thoughtrecipe.default")
	if stop || restoreResult != nil {
		t.Fatalf("nil source must be a no-op cold start, got result=%+v stop=%v", restoreResult, stop)
	}
	if _, ok := contextdata.GetTyped[any](env, "state.alpha"); ok {
		t.Fatal("cold start must not restore any state")
	}
	if _, ok := contextdata.GetTyped[any](env, regroundChunkIDsKey); ok {
		t.Fatal("cold start must not write reground provenance")
	}
}

// TestStateRegroundGroundedFalseColdStart covers Grounded=false: a normal cold
// start, never an error and never a restore.
func TestStateRegroundGroundedFalseColdStart(t *testing.T) {
	src := &fakeRegroundSource{result: grounding.RegroundResult{Grounded: false}}
	node := NewThoughtRecipeExecutorNode("euclo.execute_thoughtrecipe").
		WithWorkspace("ws-1").
		WithStateReground(src)
	env := contextdata.NewEnvelope("task-cold2", "session-cold2")

	restoreResult, stop := node.restoreState(context.Background(), env, "euclo.thoughtrecipe.default")
	if stop || restoreResult != nil {
		t.Fatalf("Grounded=false must be a no-op cold start, got result=%+v stop=%v", restoreResult, stop)
	}
	if _, ok := contextdata.GetTyped[any](env, regroundChunkIDsKey); ok {
		t.Fatal("Grounded=false must not write reground provenance")
	}
}

// TestStateRegroundQueryErrorContinuesCold reports the query failure loudly
// (grounding.reground_failed event via deps telemetry) and continues cold.
func TestStateRegroundQueryErrorContinuesCold(t *testing.T) {
	src := &fakeRegroundSource{err: errors.New("corpus unavailable")}
	node := NewThoughtRecipeExecutorNode("euclo.execute_thoughtrecipe").
		WithWorkspace("ws-1").
		WithStateReground(src)
	env := contextdata.NewEnvelope("task-qerr", "session-qerr")

	restoreResult, stop := node.restoreState(context.Background(), env, "euclo.thoughtrecipe.default")
	if stop || restoreResult != nil {
		t.Fatalf("query error must continue cold, got result=%+v stop=%v", restoreResult, stop)
	}
}

// TestStateRegroundCaptureMismatchAborts covers AC-20's capture-mismatch: an
// entry whose destination is not a namespace reference is a typed capture
// failure, not a silent coercion, and stops the recipe structured.
func TestStateRegroundCaptureMismatchAborts(t *testing.T) {
	src := &fakeRegroundSource{result: grounding.RegroundResult{
		Grounded: true,
		Entries: []grounding.RegroundEntry{
			{StateKey: "not_a_namespace_path", Value: "x", Origin: "user", ChunkID: "chunk:capture:bb"},
		},
	}}
	node := NewThoughtRecipeExecutorNode("euclo.execute_thoughtrecipe").
		WithWorkspace("ws-1").
		WithStateReground(src)
	env := contextdata.NewEnvelope("task-mismatch", "session-mismatch")

	restoreResult, stop := node.restoreState(context.Background(), env, "euclo.thoughtrecipe.default")
	if !stop {
		t.Fatal("capture mismatch must stop the recipe dispatch")
	}
	if restoreResult == nil || restoreResult.Success {
		t.Fatalf("capture mismatch must yield a failed structured result, got %+v", restoreResult)
	}
	if failure, ok := euclostate.GetStepFailure(env); !ok || failure.Kind != euclotypes.FailureUnknown {
		t.Fatalf("envelope step failure = %+v (ok=%v), want unknown-kind typed failure", failure, ok)
	}
}

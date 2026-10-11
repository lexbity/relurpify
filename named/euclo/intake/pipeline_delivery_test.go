package intake

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/contextstream"
	contextports "codeburg.org/lexbit/relurpify/context/ports"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/named/euclo/euclokeys"
	"codeburg.org/lexbit/relurpify/named/euclo/families"
)

// deliveringTrigger returns a canned compilation and records the request.
type deliveringTrigger struct {
	request contextstream.Request
	result  *contextstream.Result
	err     error
}

func (t *deliveringTrigger) RequestBlocking(_ context.Context, req contextstream.Request) (*contextstream.Result, error) {
	t.request = req
	return t.result, t.err
}

func deliveryResult(body string, epoch uint64) *contextstream.Result {
	return &contextstream.Result{
		Request: contextstream.Request{ID: "intake-delivery", EpochID: epoch},
		Compilation: &contextports.CompilationResult{
			StreamedChunks: []contextports.StreamedChunkView{
				{ChunkID: "chunk-seeded", Body: body, TokenEstimate: 16},
			},
			StreamedRefs: []string{"chunk-seeded"},
			Record:       contextports.CompilationRecord{ID: "rec-intake", FinalTokens: 16, OriginalBudget: 4096},
		},
		Record: &contextports.CompilationRecord{ID: "rec-intake", FinalTokens: 16, OriginalBudget: 4096},
	}
}

func intakeEnvelope(t *testing.T) *contextdata.Envelope {
	t.Helper()
	env := contextdata.NewEnvelope("task-intake", "session-intake")
	contextdata.SetTyped(env, euclokeys.KeyTaskInput, &execution.Task{
		ID:          "task-intake",
		Instruction: "review the seeded evidence",
	})
	return env
}

func intakeNode(t *testing.T, trigger StreamTrigger) *IntakePipelineNode {
	t.Helper()
	registry := families.NewRegistry()
	_ = families.RegisterBuiltins(registry)
	return NewIntakePipelineNode("intake", registry, 1024, trigger)
}

// TestIntakeDeliversSlice is the D5 proof: the intake compile lands its slice
// on the envelope through ApplyResult, so the FIRST model call renders the
// seeded body — the compile stops being a side effect.
func TestIntakeDeliversSlice(t *testing.T) {
	const body = "wombat-intake ALPHA seeded fixture evidence"
	env := intakeEnvelope(t)
	epoch := env.AssemblyMetadataSnapshot().EpochID
	trigger := &deliveringTrigger{result: deliveryResult(body, epoch)}
	node := intakeNode(t, trigger)

	if _, err := node.Execute(context.Background(), env); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	slice := env.StreamedSliceSnapshot()
	if slice == nil {
		t.Fatal("intake compile did not land a slice on the envelope")
	}
	if slice.RequestID != "rec-intake" || slice.Epoch != epoch {
		t.Fatalf("slice header = %+v, want request rec-intake at epoch %d", slice, epoch)
	}
	found := false
	for _, chunk := range slice.Chunks {
		if chunk.Body == body {
			found = true
		}
	}
	if !found {
		t.Fatalf("seeded body missing from the envelope slice: %+v", slice.Chunks)
	}
	if ids := env.StreamedChunkIDs(); len(ids) != 1 || ids[0] != "chunk-seeded" {
		t.Fatalf("streamed refs = %v, want [chunk-seeded]", ids)
	}
	// The request carried the envelope epoch (cache identity).
	if trigger.request.EpochID != epoch {
		t.Fatalf("intake request epoch = %d, want the envelope epoch %d", trigger.request.EpochID, epoch)
	}
}

// TestIntakeWorkingStateBounded proves FR-15's bound: the working entry is
// the shared summary map — no Body key anywhere — and it is far under the
// node contract's 4096-byte cap.
func TestIntakeWorkingStateBounded(t *testing.T) {
	env := intakeEnvelope(t)
	trigger := &deliveringTrigger{result: deliveryResult(strings.Repeat("body-bytes ", 4096), env.AssemblyMetadataSnapshot().EpochID)}
	node := intakeNode(t, trigger)

	if _, err := node.Execute(context.Background(), env); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	summary, ok := contextdata.GetTyped[map[string]any](env, euclokeys.KeyStreamResult)
	if !ok || summary == nil {
		t.Fatal("expected the stream summary in working state")
	}
	encoded, err := json.Marshal(summary)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) > 4096 {
		t.Fatalf("summary = %d bytes, must be <= 4096", len(encoded))
	}
	for key := range summary {
		if strings.EqualFold(key, "body") {
			t.Fatal("the summary must never carry a body key")
		}
	}
	if summary["chunks"] != 1 || summary["tokens"] != 16 {
		t.Fatalf("summary = %+v, want chunks=1 tokens=16", summary)
	}
	// The result payload carries the summary too, never the full result.
	raw, _ := json.Marshal(env.WorkingDataSnapshot())
	if strings.Contains(string(raw), strings.Repeat("body-bytes ", 32)) {
		t.Fatal("chunk bodies leaked into working state")
	}
}

// TestIntakeCompileBlocking proves the compile is synchronous: the trigger
// has completed and its result is already applied when Execute returns —
// no goroutine, no background job.
func TestIntakeCompileBlocking(t *testing.T) {
	env := intakeEnvelope(t)
	trigger := &deliveringTrigger{result: deliveryResult("sync body", env.AssemblyMetadataSnapshot().EpochID)}
	node := intakeNode(t, trigger)

	if _, err := node.Execute(context.Background(), env); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if trigger.request.ID == "" {
		t.Fatal("the trigger was never invoked")
	}
	if env.StreamedSliceSnapshot() == nil {
		t.Fatal("the compile result was not applied by the time Execute returned")
	}
}

// TestIntakeCompileErrorFailsNode proves the trigger-node failure rule: a
// compile error at intake fails the node instead of being swallowed into
// result data.
func TestIntakeCompileErrorFailsNode(t *testing.T) {
	trigger := &deliveringTrigger{err: errors.New("compiler offline")}
	node := intakeNode(t, trigger)
	env := intakeEnvelope(t)

	if _, err := node.Execute(context.Background(), env); err == nil {
		t.Fatal("a compile error must fail the intake node")
	}
}

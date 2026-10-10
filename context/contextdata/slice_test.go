package contextdata

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestSliceSnapshotRedaction is FR-13/D-11: envelope snapshot surfaces carry
// chunk IDs, hashes, and token counts — never bodies.
func TestSliceSnapshotRedaction(t *testing.T) {
	const body = "SEEDED-SECRET-BODY-do-not-serialize"
	env := NewEnvelope("task-slice", "session-slice")
	env.SetStreamedSlice(&StreamedSlice{
		RequestID:   "req-redact",
		Epoch:       9,
		FinalTokens: 42,
		Chunks: []StreamedChunk{
			{ChunkID: "chunk-redact-1", ContentHash: "hash-redact-1", Body: body, TokenEstimate: 42, TrustClass: "workspace"},
		},
	})

	// The durable reference snapshot (checkpoint path) serializes IDs only.
	refs, err := json.Marshal(env.ReferencesSnapshot())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(refs), body) {
		t.Fatal("chunk body leaked into the reference snapshot")
	}

	// Stamps carry the redacted accounting: IDs, hashes, token counts, epochs.
	stamps := env.StreamedSliceStamps()
	if len(stamps) != 1 {
		t.Fatalf("stamps = %d, want 1", len(stamps))
	}
	stampBytes, err := json.Marshal(stamps)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"chunk-redact-1", "hash-redact-1", "42", "9"} {
		if !strings.Contains(string(stampBytes), want) {
			t.Fatalf("stamp JSON missing %q: %s", want, stampBytes)
		}
	}
	if strings.Contains(string(stampBytes), body) {
		t.Fatal("chunk body leaked into the slice stamps")
	}

	// A Clone carries the in-memory slice (branches see it) but the clone's
	// serialized reference view still excludes the body.
	clone := env.Clone()
	if got := clone.StreamedSliceSnapshot(); got == nil || len(got.Chunks) != 1 || got.Chunks[0].Body != body {
		t.Fatalf("clone lost the slice: %+v", got)
	}
	cloneRefs, err := json.Marshal(clone.ReferencesSnapshot())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(cloneRefs), body) {
		t.Fatal("chunk body leaked into the clone's reference snapshot")
	}
}

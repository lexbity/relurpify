package knowledge

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/context/contextdata"
)

// The tests in this file are the grounding-side heirs of the absorbed output
// ingester's retraction pins (ingester_tombstone_test.go, deleted with the
// ingester plumbing): the runtime write boundary that owns chunk identity
// must also own the retraction semantics, and they must be pinned by tests.

func newTombstoneService(t *testing.T, store *ChunkStore, bus *EventBus) *GroundingService {
	t.Helper()
	service := NewGroundingService(store, bus, nil, nil)
	service.SetClock(func() time.Time { return time.Unix(1700000000, 0).UTC() })
	return service
}

// TestCaptureKindForOrigin pins the absorbed input taxonomy: tool-floor
// captures ground as tool facts, every other origin stays a capture.
func TestCaptureKindForOrigin(t *testing.T) {
	require.Equal(t, ChunkKindTool, CaptureKindForOrigin(contextdata.OriginTool))
	require.Equal(t, ChunkKindCapture, CaptureKindForOrigin(contextdata.OriginLLM))
	require.Equal(t, ChunkKindCapture, CaptureKindForOrigin(contextdata.OriginUser))
	require.Equal(t, ChunkKindCapture, CaptureKindForOrigin(contextdata.OriginClass("")))
}

// TestGroundingPreservesTombstoneOnEqualGeneration proves re-grounding content
// whose chunk was tombstoned at equal derivation generation does not resurrect
// it and does not mutate the stored record, while reporting the preserved
// tombstone as a skipped entry and on the bus.
func TestGroundingPreservesTombstoneOnEqualGeneration(t *testing.T) {
	store := newTestStore(t)
	bus := &EventBus{}
	events, cancel := bus.Subscribe(4)
	t.Cleanup(cancel)
	service := newTombstoneService(t, store, bus)
	ctx := context.Background()
	item := groundItem(map[string]any{"text": "retracted observation"}, EpistemicClaimed, contextdata.OriginLLM)

	first, err := service.Ground(ctx, []GroundingItem{item})
	require.NoError(t, err)
	require.Len(t, first.Grounded, 1)
	id := first.Grounded[0].ChunkID

	require.NoError(t, store.Tombstone(ctx, id, ""))
	prior, ok, err := store.LoadIncludingTombstoned(id)
	require.NoError(t, err)
	require.True(t, ok)
	priorVersion := prior.Version

	second, err := service.Ground(ctx, []GroundingItem{item})
	require.NoError(t, err)
	require.Empty(t, second.Grounded, "preserved tombstone must not ground")
	require.Len(t, second.Skipped, 1)
	require.Equal(t, GroundingSkipped, second.Skipped[0].Action)
	require.Equal(t, id, second.Skipped[0].ChunkID)
	require.Equal(t, "tombstone preserved", second.Skipped[0].Reason)

	stored, ok, err := store.LoadIncludingTombstoned(id)
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, stored.Tombstoned, "store must remain tombstoned")
	require.Equal(t, priorVersion, stored.Version, "preserved tombstone must not bump version")

	live, err := store.FindByContentHash(stored.ContentHash)
	require.NoError(t, err)
	require.Empty(t, live, "tombstoned chunk must not appear as a live hash match")

	require.Eventually(t, func() bool {
		for {
			select {
			case event := <-events:
				if event.Kind != EventTombstonePreserved {
					continue // chunk_ingested from the first ground
				}
				payload, ok := event.Payload.(TombstonePreservedPayload)
				require.True(t, ok)
				require.Equal(t, string(id), payload.ChunkID)
				return true
			default:
				return false
			}
		}
	}, 2*time.Second, 10*time.Millisecond, "expected knowledge.tombstone_preserved event")
}

// TestGroundingResurrectsOnNewerDerivation proves a newer derivation is the
// only path that clears a tombstone, and that the resurrected chunk keeps its
// grounding history and reports the pre-existing identity.
func TestGroundingResurrectsOnNewerDerivation(t *testing.T) {
	store := newTestStore(t)
	service := newTombstoneService(t, store, nil)
	ctx := context.Background()
	item := groundItem(map[string]any{"text": "re-established finding"}, EpistemicClaimed, contextdata.OriginLLM)

	first, err := service.Ground(ctx, []GroundingItem{item})
	require.NoError(t, err)
	require.Len(t, first.Grounded, 1)
	id := first.Grounded[0].ChunkID
	require.NoError(t, store.Tombstone(ctx, id, ""))

	// Seed a generation-1 source chunk so the re-ground derives generation 2,
	// strictly newer than the tombstoned generation 1.
	seed := KnowledgeChunk{
		ID:                   CanonicalChunkID(ChunkKindCapture, []byte("seed-generation-1")),
		WorkspaceID:          "ws-1",
		DerivationGeneration: 1,
		Freshness:            FreshnessValid,
		Provenance:           ChunkProvenance{SessionID: "session-1", WorkflowID: "task-1", CompiledBy: CompilerDeterministic, Timestamp: time.Unix(1700000000, 0).UTC()},
		Body:                 ChunkBody{Raw: "seed"},
	}
	_, err = store.Save(ctx, seed)
	require.NoError(t, err)

	newer := item
	newer.SourceChunkIDs = []ChunkID{seed.ID}
	second, err := service.Ground(ctx, []GroundingItem{newer})
	require.NoError(t, err)
	require.Len(t, second.Grounded, 1)
	require.True(t, second.Grounded[0].AlreadyExisted, "resurrection re-establishes an existing identity")
	require.False(t, second.Grounded[0].Action != GroundingGrounded)

	stored, ok, err := store.LoadIncludingTombstoned(id)
	require.NoError(t, err)
	require.True(t, ok)
	require.False(t, stored.Tombstoned, "newer derivation must clear the tombstone")
	require.Equal(t, FreshnessValid, stored.Freshness)
	require.Equal(t, 2, stored.DerivationGeneration, "re-ground must derive from the seeded generation")
	require.Len(t, stored.GroundedBy, 2, "resurrected chunk must keep its grounding history")

	live, err := store.FindByContentHash(stored.ContentHash)
	require.NoError(t, err)
	require.Len(t, live, 1, "resurrected chunk is live again")
}

// TestGroundingToolOriginCaptureGroundsAsToolKind proves the absorbed
// taxonomy at the runtime write boundary: a capture whose dataflow floor is
// tool output grounds as ChunkKindTool with the tool-result trust floor, while
// agent-claim captures stay ChunkKindCapture. Items are built exactly as the
// production capture site does — Kind resolved via CaptureKindForOrigin.
func TestGroundingToolOriginCaptureGroundsAsToolKind(t *testing.T) {
	store := newTestStore(t)
	service := newTombstoneService(t, store, nil)
	ctx := context.Background()
	toolOrigin := groundItem(map[string]any{"match": "line"}, EpistemicClaimed, contextdata.OriginTool)
	toolOrigin.Kind = CaptureKindForOrigin(toolOrigin.Origin)
	llmOrigin := groundItem(map[string]any{"text": "claim"}, EpistemicClaimed, contextdata.OriginLLM)
	llmOrigin.Kind = CaptureKindForOrigin(llmOrigin.Origin)
	items := []GroundingItem{toolOrigin, llmOrigin}

	report, err := service.Ground(ctx, items)
	require.NoError(t, err)
	require.Len(t, report.Grounded, 2)

	toolChunk, ok, err := store.Load(report.Grounded[0].ChunkID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, string(ChunkKindTool), toolChunk.Body.Fields["kind"])
	require.Equal(t, agentspec.TrustClassToolResult, toolChunk.TrustClass)

	llmChunk, ok, err := store.Load(report.Grounded[1].ChunkID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, string(ChunkKindCapture), llmChunk.Body.Fields["kind"])

	// The kind participates in the canonical identity: re-grounding converges
	// on the same chunks rather than forking per-kind identities.
	again, err := service.Ground(ctx, items)
	require.NoError(t, err)
	for i := range items {
		require.Equal(t, report.Grounded[i].ChunkID, again.Grounded[i].ChunkID)
		require.True(t, again.Grounded[i].AlreadyExisted)
	}
}

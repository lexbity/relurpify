package knowledge

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
)

func newObservationInput(generation int) ingestTextInput {
	return ingestTextInput{
		kind:                 ChunkKindObservation,
		text:                 "retracted observation",
		sourceOrigin:         SourceOriginDerivation,
		trustClass:           agentspec.TrustClassLLMGenerated,
		memoryClass:          MemoryClassStreamed,
		storageMode:          StorageModeSummarized,
		derivationGeneration: generation,
	}
}

// TestIngestTextPreservesTombstoneOnEqualGeneration proves a content-hash match
// against a tombstoned chunk does not resurrect it and does not mutate the
// store, while reporting the preserved tombstone on the bus.
func TestIngestTextPreservesTombstoneOnEqualGeneration(t *testing.T) {
	store := newTestStore(t)
	ing := NewOutputIngester(store, nil)
	ctx := context.Background()

	original, err := ing.ingestText(ctx, newObservationInput(0))
	require.NoError(t, err)
	require.NoError(t, store.Tombstone(ctx, original.ID, ""))

	prior, ok, err := store.LoadIncludingTombstoned(original.ID)
	require.NoError(t, err)
	require.True(t, ok)
	priorVersion := prior.Version

	bus := &EventBus{}
	events, cancel := bus.Subscribe(4)
	defer cancel()
	ing.Events = bus

	returned, err := ing.ingestText(ctx, newObservationInput(0))
	require.NoError(t, err)
	require.True(t, returned.Tombstoned, "equal-generation ingest must return the tombstoned record")

	stored, ok, err := store.LoadIncludingTombstoned(original.ID)
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, stored.Tombstoned, "store must remain tombstoned")
	require.Equal(t, priorVersion, stored.Version, "preserved tombstone must not bump version")

	live, err := store.FindByContentHash(original.ContentHash)
	require.NoError(t, err)
	require.Empty(t, live, "tombstoned chunk must not appear as a live hash match")

	select {
	case event := <-events:
		require.Equal(t, EventTombstonePreserved, event.Kind)
		payload, ok := event.Payload.(TombstonePreservedPayload)
		require.True(t, ok)
		require.Equal(t, string(original.ID), payload.ChunkID)
	case <-time.After(2 * time.Second):
		t.Fatal("expected knowledge.tombstone_preserved event")
	}
}

// TestIngestTextResurrectsOnNewerDerivation proves newer derivation re-enters
// the store as a live, fresh chunk.
func TestIngestTextResurrectsOnNewerDerivation(t *testing.T) {
	store := newTestStore(t)
	ing := NewOutputIngester(store, nil)
	ctx := context.Background()

	original, err := ing.ingestText(ctx, newObservationInput(0))
	require.NoError(t, err)
	require.NoError(t, store.Tombstone(ctx, original.ID, ""))

	returned, err := ing.ingestText(ctx, newObservationInput(1))
	require.NoError(t, err)
	require.False(t, returned.Tombstoned)
	require.Equal(t, FreshnessValid, returned.Freshness)
	require.Equal(t, original.ID, returned.ID)

	stored, ok, err := store.LoadIncludingTombstoned(original.ID)
	require.NoError(t, err)
	require.True(t, ok)
	require.False(t, stored.Tombstoned)

	live, err := store.FindByContentHash(original.ContentHash)
	require.NoError(t, err)
	require.Len(t, live, 1)
}

package knowledge

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

func newGroundingService(t *testing.T, store *ChunkStore, tel *recordingBusTelemetry) *GroundingService {
	t.Helper()
	var sink telemetry.Telemetry
	if tel != nil {
		sink = tel
	}
	service := NewGroundingService(store, nil, nil, sink)
	service.SetClock(func() time.Time { return time.Unix(1700000000, 0).UTC() })
	return service
}

func groundItem(value any, epistemics Epistemics, origin contextdata.OriginClass) GroundingItem {
	return GroundingItem{
		Value:          value,
		TypeAnnotation: "ReviewFindings",
		Epistemics:     epistemics,
		Origin:         origin,
		StateKey:       "state.findings",
		NodeID:         "node-1",
		TaskID:         "task-1",
		SessionID:      "session-1",
		WorkspaceID:    "ws-1",
		RecipeID:       "recipe-1",
		Epoch:          3,
	}
}

// TestGroundingServiceHappyPath proves a batch grounds deterministically and
// idempotently: two service instances produce identical IDs for the same items.
func TestGroundingServiceHappyPath(t *testing.T) {
	store1 := newTestStore(t)
	service1 := newGroundingService(t, store1, nil)
	items := []GroundingItem{
		groundItem(map[string]any{"text": "finding a"}, EpistemicClaimed, contextdata.OriginLLM),
		groundItem(map[string]any{"text": "finding b"}, EpistemicClaimed, contextdata.OriginLLM),
		groundItem(map[string]any{"text": "finding c"}, EpistemicGiven, contextdata.OriginUser),
	}

	report, err := service1.Ground(context.Background(), items)
	require.NoError(t, err)
	require.Len(t, report.Grounded, 3)
	require.Empty(t, report.Skipped)
	require.Empty(t, report.Quarantined)

	store2 := newTestStore(t)
	service2 := newGroundingService(t, store2, nil)
	report2, err := service2.Ground(context.Background(), items)
	require.NoError(t, err)
	for i := range items {
		require.Equal(t, report.Grounded[i].ChunkID, report2.Grounded[i].ChunkID, "IDs must be deterministic")
	}

	all, err := store1.FindAll()
	require.NoError(t, err)
	require.Len(t, all, 3, "one transaction must commit exactly the batch")
}

// TestGroundingServiceTrustFloors proves the D4 trust floor: `given` is
// downgraded on llm origin, honored on user origin, and tool origin lowers the
// default claim.
func TestGroundingServiceTrustFloors(t *testing.T) {
	store := newTestStore(t)
	tel := newRecordingBusTelemetry()
	service := newGroundingService(t, store, tel)

	report, err := service.Ground(context.Background(), []GroundingItem{
		groundItem(map[string]any{"v": "downgraded"}, EpistemicGiven, contextdata.OriginLLM),
		groundItem(map[string]any{"v": "honored"}, EpistemicGiven, contextdata.OriginUser),
		groundItem(map[string]any{"v": "tool"}, EpistemicClaimed, contextdata.OriginTool),
	})
	require.NoError(t, err)
	require.Len(t, report.Grounded, 3)

	require.Equal(t, agentspec.TrustClassLLMGenerated, report.Grounded[0].TrustClass)
	require.Equal(t, EpistemicClaimed, report.Grounded[0].Epistemics, "given on llm origin must downgrade")
	require.Equal(t, agentspec.TrustClassWorkspaceTrusted, report.Grounded[1].TrustClass)
	require.Equal(t, EpistemicGiven, report.Grounded[1].Epistemics)
	require.Equal(t, agentspec.TrustClassToolResult, report.Grounded[2].TrustClass)

	require.Equal(t, 1, tel.count(telemetry.EventCaptureEpistemicsDowngraded))

	downgraded, ok, err := store.Load(report.Grounded[0].ChunkID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "claimed", downgraded.Epistemics)
	require.Equal(t, "llm", downgraded.OriginClass)
}

// TestGroundingServiceEdges proves grounds and derives_from edges are written
// for streamed context sources and forwarded values respectively.
func TestGroundingServiceEdges(t *testing.T) {
	store := newTestStore(t)
	service := newGroundingService(t, store, nil)
	ctx := context.Background()

	source, err := store.Save(ctx, KnowledgeChunk{
		ID: "chunk:capture:source-1", WorkspaceID: "ws-1",
		Provenance: ChunkProvenance{CompiledBy: CompilerDeterministic, Timestamp: time.Now().UTC()},
		Body:       ChunkBody{Raw: "source"},
		Freshness:  FreshnessValid,
	})
	require.NoError(t, err)
	forwarded, err := store.Save(ctx, KnowledgeChunk{
		ID: "chunk:capture:forwarded-1", WorkspaceID: "ws-1",
		Provenance: ChunkProvenance{CompiledBy: CompilerDeterministic, Timestamp: time.Now().UTC()},
		Body:       ChunkBody{Raw: "forwarded"},
		Freshness:  FreshnessValid,
	})
	require.NoError(t, err)

	item := groundItem(map[string]any{"text": "edge finding"}, EpistemicClaimed, contextdata.OriginLLM)
	item.SourceChunkIDs = []ChunkID{source.ID}
	item.ForwardedFrom = []ChunkID{forwarded.ID}

	report, err := service.Ground(ctx, []GroundingItem{item})
	require.NoError(t, err)
	chunkID := report.Grounded[0].ChunkID

	grounds, err := store.LoadEdgesFrom(chunkID, EdgeKindGrounds)
	require.NoError(t, err)
	require.Len(t, grounds, 1)
	require.Equal(t, source.ID, grounds[0].ToChunk)

	derives, err := store.LoadEdgesFrom(chunkID, EdgeKindDerivesFrom)
	require.NoError(t, err)
	require.Len(t, derives, 1)
	require.Equal(t, forwarded.ID, derives[0].ToChunk)

	stored, ok, err := store.Load(chunkID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, 1, stored.DerivationGeneration)
	require.Len(t, stored.GroundedBy, 1)
	require.Equal(t, "task-1", stored.GroundedBy[0].TaskID)
	require.Equal(t, uint64(3), stored.GroundedBy[0].Epoch)
}

// TestCanonicalCaptureEncodingDeterministic proves map-iteration order and
// nested key order cannot leak into the capture content hash.
func TestCanonicalCaptureEncodingDeterministic(t *testing.T) {
	first := groundItem(map[string]any{
		"b": 1,
		"a": 2,
		"nested": map[string]any{
			"z": "last",
			"x": "first",
		},
	}, EpistemicClaimed, contextdata.OriginLLM)
	second := groundItem(map[string]any{
		"a": 2,
		"b": 1,
		"nested": map[string]any{
			"x": "first",
			"z": "last",
		},
	}, EpistemicClaimed, contextdata.OriginLLM)

	encodedFirst, err := canonicalCaptureItem(first)
	require.NoError(t, err)
	encodedSecond, err := canonicalCaptureItem(second)
	require.NoError(t, err)
	require.Equal(t, encodedFirst, encodedSecond)
	require.Equal(t, CanonicalChunkID(groundingKind, encodedFirst), CanonicalChunkID(groundingKind, encodedSecond))
	require.NotContains(t, string(encodedFirst), "1700000000", "content must carry no clock")
}

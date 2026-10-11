package knowledge

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// TestConcurrentIdenticalGroundsSerialize is the FR-10 proof: 8 goroutines
// grounding the identical value must serialize on the OCC cycle — exactly one
// chunk, every landed merge accounted in the version and the provenance list,
// no lost update. Under -race it also proves the decision's reads and the
// apply share one critical section.
func TestConcurrentIdenticalGroundsSerialize(t *testing.T) {
	store := newTestStore(t)
	service := newGroundingService(t, store, nil)
	item := groundItem(map[string]any{"text": "contended finding"}, EpistemicClaimed, contextdata.OriginLLM)

	const goroutines = 8
	errs := make([]error, goroutines)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, errs[i] = service.Ground(context.Background(), []GroundingItem{item})
		}(i)
	}
	close(start)
	wg.Wait()
	for i, err := range errs {
		require.NoError(t, err, "goroutine %d", i)
	}

	chunk, ok, err := store.Load(ChunkID(CanonicalChunkID(ChunkKindCapture, mustEncode(t, item))))
	require.NoError(t, err)
	require.True(t, ok, "the contended chunk must exist exactly once")
	require.Len(t, chunk.GroundedBy, goroutines, "every landed ground merges its provenance record")
	require.Equal(t, goroutines, chunk.Version, "one version bump per landed merge")
	for _, record := range chunk.GroundedBy {
		require.Equal(t, "task-1", record.TaskID)
	}
}

func mustEncode(t *testing.T, item GroundingItem) []byte {
	t.Helper()
	encoded, err := canonicalCaptureItem(item)
	require.NoError(t, err)
	return encoded
}

// TestGroundedByCappedNewestFirst proves the FR-13 cap: re-grounding the same
// content 17 times keeps the newest 16 records, newest at the head; the tail
// (oldest) drops. It also proves emitGrounded attributes to the producing
// node passed by the caller, not a derived record.
func TestGroundedByCappedNewestFirst(t *testing.T) {
	store := newTestStore(t)
	tel := newRecordingBusTelemetry()
	service := newGroundingService(t, store, tel)
	item := groundItem(map[string]any{"text": "capped finding"}, EpistemicClaimed, contextdata.OriginLLM)
	item.NodeID = "node-final"

	for epoch := uint64(1); epoch <= MaxGroundingRecords+1; epoch++ {
		sweep := item
		sweep.Epoch = epoch
		_, err := service.Ground(context.Background(), []GroundingItem{sweep})
		require.NoError(t, err)
	}

	chunk, ok, err := store.Load(ChunkID(CanonicalChunkID(ChunkKindCapture, mustEncode(t, item))))
	require.NoError(t, err)
	require.True(t, ok)
	require.Len(t, chunk.GroundedBy, MaxGroundingRecords, "the provenance list is capped")
	require.Equal(t, uint64(MaxGroundingRecords+1), chunk.GroundedBy[0].Epoch, "newest record first")
	require.Equal(t, uint64(2), chunk.GroundedBy[len(chunk.GroundedBy)-1].Epoch, "oldest surviving record is epoch 2 (epoch 1 dropped)")
	require.Equal(t, "node-final", chunk.GroundedBy[0].NodeID)
}

// TestTombstonedCaptureSourceResolvesAbsent proves D4.2: a tombstoned chunk
// resolves absent from ChunkIDForCaptureValue REGARDLESS of freshness, so
// retracted content can never become derives_from provenance.
func TestTombstonedCaptureSourceResolvesAbsent(t *testing.T) {
	store := newTestStore(t)
	service := newGroundingService(t, store, nil)
	item := groundItem("retract me", EpistemicClaimed, contextdata.OriginLLM)
	report, err := service.Ground(context.Background(), []GroundingItem{item})
	require.NoError(t, err)
	require.Len(t, report.Grounded, 1)

	id, ok, err := service.ChunkIDForCaptureValue(item.Value, item.TypeAnnotation)
	require.NoError(t, err)
	require.True(t, ok)

	require.NoError(t, store.Tombstone(context.Background(), id, ""))

	// The tombstone must make the value absent even though the chunk's
	// freshness field was never touched by retraction.
	id2, ok2, err := service.ChunkIDForCaptureValue(item.Value, item.TypeAnnotation)
	require.NoError(t, err)
	require.False(t, ok2, "tombstoned content must resolve absent")
	require.Empty(t, id2)
}

// TestGivenMislabelDowngradesFully proves D4.3/FR-11: a `given` capture whose
// identity check fails grounds at the more restrictive of the claimed floor
// and the dataflow origin floor — even when the claimed origin is user — and
// the downgrade event fires.
func TestGivenMislabelDowngradesFully(t *testing.T) {
	store := newTestStore(t)
	tel := newRecordingBusTelemetry()
	service := newGroundingService(t, store, tel)
	// The recipe claims user-given, the dataflow origin claims user, but the
	// lookup resolves the value as llm-origin: the annotation is rejected and
	// the claimed floor (llm-generated) composes with the origin floor.
	item := groundItem("mislabeled given", EpistemicGiven, contextdata.OriginUser)
	item.Origin = contextdata.OriginUser
	lookup := func(any) (contextdata.OriginClass, bool) { return contextdata.OriginLLM, true }

	report, err := service.Ground(context.Background(), []GroundingItem{item}, WithGivenOriginLookup(lookup))
	require.NoError(t, err)
	require.Len(t, report.Grounded, 1)
	require.Equal(t, agentspec.TrustClassLLMGenerated, report.Grounded[0].TrustClass,
		"a failed given check must compose the claimed floor with the origin floor, never elevate")
	require.Equal(t, EpistemicClaimed, report.Grounded[0].Epistemics)
	require.Equal(t, 1, tel.count(telemetry.EventCaptureEpistemicsDowngraded), "the downgrade is observable")

	stored, ok, err := store.Load(report.Grounded[0].ChunkID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, string(agentspec.TrustClassLLMGenerated), string(stored.TrustClass))
}

// TestGivenLookupConfirmsUserOrigin proves the defense-in-depth layer is live
// per call (FR-12): a genuine user-origin given with a confirming lookup
// grounds workspace-trusted, no downgrade.
func TestGivenLookupConfirmsUserOrigin(t *testing.T) {
	store := newTestStore(t)
	tel := newRecordingBusTelemetry()
	service := newGroundingService(t, store, tel)
	item := groundItem("genuine user value", EpistemicGiven, contextdata.OriginUser)
	lookup := func(any) (contextdata.OriginClass, bool) { return contextdata.OriginUser, true }

	report, err := service.Ground(context.Background(), []GroundingItem{item}, WithGivenOriginLookup(lookup))
	require.NoError(t, err)
	require.Len(t, report.Grounded, 1)
	require.Equal(t, agentspec.TrustClassWorkspaceTrusted, report.Grounded[0].TrustClass)
	require.Equal(t, EpistemicGiven, report.Grounded[0].Epistemics)
	require.Equal(t, 0, tel.count(telemetry.EventCaptureEpistemicsDowngraded))
}

// TestRegroundSelectsByProvenance proves FR-14: restore selection follows the
// grounding-record provenance, not the chunk-kind string. A tool-kind chunk
// with grounding records restores; a chunk with no grounding record does not
// — regardless of the kind string its fields carry.
func TestRegroundSelectsByProvenance(t *testing.T) {
	store := newTestStore(t)
	service := newGroundingService(t, store, nil)
	ctx := context.Background()

	// A tool-origin capture grounds as ChunkKindTool (CaptureKindForOrigin).
	tool := groundItem(map[string]any{"text": "tool observation"}, EpistemicClaimed, contextdata.OriginTool)
	tool.Kind = ChunkKindTool
	tool.StateKey = "state.tool_finding"
	report, err := service.Ground(ctx, []GroundingItem{tool})
	require.NoError(t, err)
	require.Len(t, report.Grounded, 1)

	// A chunk with no grounding record — saved directly, kind string set to
	// capture — is not restorable state.
	direct := KnowledgeChunk{
		ID:          "chunk:ungrounded",
		WorkspaceID: "ws-1",
		TrustClass:  agentspec.TrustClassLLMGenerated,
		Body:        ChunkBody{Fields: map[string]any{"kind": string(ChunkKindCapture), "state_key": "state.orphan"}},
		Provenance:  ChunkProvenance{SessionID: "session-1", WorkflowID: "task-1"},
		Freshness:   FreshnessValid,
	}
	_, err = store.Save(ctx, direct)
	require.NoError(t, err)

	result, err := service.Reground(ctx, RegroundRequest{WorkspaceID: "ws-1", SessionID: "session-1"})
	require.NoError(t, err)
	require.True(t, result.Grounded)
	require.Len(t, result.Entries, 1, "the tool-kind grounded chunk restores; the ungrounded capture-kind chunk does not")
	require.Equal(t, "state.tool_finding", result.Entries[0].StateKey)
	require.Equal(t, tool.Kind, ChunkKindTool)
}

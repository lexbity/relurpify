package knowledge

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// newClockGroundingService returns a service whose clock is a mutable pointer,
// so test runs are ordered deterministically.
func newClockGroundingService(t *testing.T, store *ChunkStore, tel *recordingBusTelemetry) (*GroundingService, *time.Time) {
	t.Helper()
	var sink telemetry.Telemetry
	if tel != nil {
		sink = tel
	}
	clock := time.Unix(1700000000, 0).UTC()
	ptr := &clock
	service := NewGroundingService(store, nil, nil, sink)
	service.SetClock(func() time.Time { return *ptr })
	return service, ptr
}

// TestRegroundSelectsLatestRun proves the query returns only the most recent
// grounded run for the scope.
func TestRegroundSelectsLatestRun(t *testing.T) {
	store := newTestStore(t)
	service, now := newClockGroundingService(t, store, nil)
	ctx := context.Background()

	first := groundItem(map[string]any{"text": "run-one"}, EpistemicClaimed, contextdata.OriginLLM)
	first.TaskID = "run-1"
	_, err := service.Ground(ctx, []GroundingItem{first})
	require.NoError(t, err)
	*now = now.Add(time.Hour)

	second := groundItem(map[string]any{"text": "run-two"}, EpistemicClaimed, contextdata.OriginLLM)
	second.TaskID = "run-2"
	_, err = service.Ground(ctx, []GroundingItem{second})
	require.NoError(t, err)

	result, err := service.Reground(ctx, RegroundRequest{WorkspaceID: "ws-1", SessionID: "session-1", RecipeID: "recipe-1"})
	require.NoError(t, err)
	require.True(t, result.Grounded)
	require.Equal(t, "run-2", result.SourceRunID)
	require.Len(t, result.Entries, 1)
	value, ok := result.Entries[0].Value.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "run-two", value["text"])
}

// TestRegroundFinalCapturePerKey proves later captures of the same state key
// within a run supersede earlier ones.
func TestRegroundFinalCapturePerKey(t *testing.T) {
	store := newTestStore(t)
	service, now := newClockGroundingService(t, store, nil)
	ctx := context.Background()

	alpha := groundItem(map[string]any{"text": "alpha"}, EpistemicClaimed, contextdata.OriginLLM)
	alpha.TaskID = "run-1"
	_, err := service.Ground(ctx, []GroundingItem{alpha})
	require.NoError(t, err)
	*now = now.Add(time.Second)

	beta := groundItem(map[string]any{"text": "beta"}, EpistemicClaimed, contextdata.OriginLLM)
	beta.TaskID = "run-1"
	_, err = service.Ground(ctx, []GroundingItem{beta})
	require.NoError(t, err)

	result, err := service.Reground(ctx, RegroundRequest{WorkspaceID: "ws-1", SessionID: "session-1", RecipeID: "recipe-1"})
	require.NoError(t, err)
	require.Len(t, result.Entries, 1, "one state key, one final capture")
	require.Equal(t, "state.findings", result.Entries[0].StateKey)
	value, ok := result.Entries[0].Value.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "beta", value["text"], "later capture must supersede")
}

// TestRegroundExcludesTombstoned proves tombstoned chunks never restore.
func TestRegroundExcludesTombstoned(t *testing.T) {
	store := newTestStore(t)
	service, _ := newClockGroundingService(t, store, nil)
	ctx := context.Background()

	report, err := service.Ground(ctx, []GroundingItem{groundItem(map[string]any{"text": "gone"}, EpistemicClaimed, contextdata.OriginLLM)})
	require.NoError(t, err)
	require.NoError(t, store.Tombstone(ctx, report.Grounded[0].ChunkID, ""))

	result, err := service.Reground(ctx, RegroundRequest{WorkspaceID: "ws-1", RecipeID: "recipe-1"})
	require.NoError(t, err)
	require.False(t, result.Grounded)
	require.Empty(t, result.Entries)
}

// TestRegroundColdStartIsNotError proves an empty corpus is a normal, silent
// cold start.
func TestRegroundColdStartIsNotError(t *testing.T) {
	store := newTestStore(t)
	service, _ := newClockGroundingService(t, store, nil)
	result, err := service.Reground(context.Background(), RegroundRequest{WorkspaceID: "ws-1", RecipeID: "recipe-1"})
	require.NoError(t, err)
	require.False(t, result.Grounded)
	require.Empty(t, result.Entries)
}

// TestRegroundScopesBySessionAndRecipe proves the query filters on both axes.
func TestRegroundScopesBySessionAndRecipe(t *testing.T) {
	store := newTestStore(t)
	service, now := newClockGroundingService(t, store, nil)
	ctx := context.Background()

	match := groundItem(map[string]any{"text": "match"}, EpistemicClaimed, contextdata.OriginLLM)
	match.TaskID = "run-match"
	_, err := service.Ground(ctx, []GroundingItem{match})
	require.NoError(t, err)
	*now = now.Add(time.Minute)

	other := groundItem(map[string]any{"text": "other"}, EpistemicClaimed, contextdata.OriginLLM)
	other.TaskID = "run-other"
	other.SessionID = "session-2"
	other.RecipeID = "recipe-2"
	_, err = service.Ground(ctx, []GroundingItem{other})
	require.NoError(t, err)

	result, err := service.Reground(ctx, RegroundRequest{WorkspaceID: "ws-1", SessionID: "session-1", RecipeID: "recipe-1"})
	require.NoError(t, err)
	require.True(t, result.Grounded)
	require.Len(t, result.Entries, 1)

	// A session that never touched the recipe is a cold start.
	miss, err := service.Reground(ctx, RegroundRequest{WorkspaceID: "ws-1", SessionID: "session-2", RecipeID: "recipe-1"})
	require.NoError(t, err)
	require.False(t, miss.Grounded)
}

// TestRegroundHonorsCancellation proves a cancelled context aborts the query.
func TestRegroundHonorsCancellation(t *testing.T) {
	store := newTestStore(t)
	service, _ := newClockGroundingService(t, store, nil)
	_, err := service.Ground(context.Background(), []GroundingItem{groundItem(map[string]any{"text": "x"}, EpistemicClaimed, contextdata.OriginLLM)})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = service.Reground(ctx, RegroundRequest{WorkspaceID: "ws-1", RecipeID: "recipe-1"})
	require.Error(t, err)
}

// TestRegroundRestoreTrustFloor proves a stored `given` whose origin cannot
// satisfy the identity rule is restored as claimed with an observable
// downgrade (FR-16).
func TestRegroundRestoreTrustFloor(t *testing.T) {
	store := newTestStore(t)
	tel := newRecordingBusTelemetry()
	service, now := newClockGroundingService(t, store, tel)
	ctx := context.Background()

	content, err := canonicalCaptureItem(GroundingItem{Value: map[string]any{"answer": "yes"}, TypeAnnotation: "Text"})
	require.NoError(t, err)
	_, err = store.Save(ctx, KnowledgeChunk{
		ID:          "chunk:capture:given-tool",
		WorkspaceID: "ws-1",
		ContentHash: contentHashForText("given-tool"),
		Epistemics:  "given",
		OriginClass: "tool",
		AcquiredAt:  *now,
		Freshness:   FreshnessValid,
		Provenance:  ChunkProvenance{SessionID: "session-1", CompiledBy: CompilerDeterministic, Timestamp: *now},
		GroundedBy:  []GroundingRecord{{TaskID: "restore-run", NodeID: "n1", RecipeID: "recipe-1"}},
		Body:        ChunkBody{Raw: string(content), Fields: map[string]any{"kind": string(groundingKind), "state_key": "state.answer"}},
	})
	require.NoError(t, err)

	result, err := service.Reground(ctx, RegroundRequest{WorkspaceID: "ws-1", SessionID: "session-1", RecipeID: "recipe-1"})
	require.NoError(t, err)
	require.True(t, result.Grounded)
	require.Len(t, result.Entries, 1)
	require.Equal(t, "claimed", result.Entries[0].Epistemics, "restore must never elevate a given claim above its origin floor")
	require.Equal(t, "tool", result.Entries[0].Origin)
	require.Equal(t, 1, tel.count(telemetry.EventCaptureEpistemicsDowngraded))
}

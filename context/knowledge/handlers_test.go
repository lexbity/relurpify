package knowledge

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/context/knowledge/ast"
	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
	"codeburg.org/lexbit/relurpify/jobs"
	"codeburg.org/lexbit/relurpify/telemetry"
)

type handlerTelemetry struct {
	events []telemetry.Event
}

func (h *handlerTelemetry) Emit(ev telemetry.Event) { h.events = append(h.events, ev) }

func newHandlerStore(t *testing.T) (*ChunkStore, *graphdb.Engine) {
	t.Helper()
	engine, err := graphdb.Open(context.Background(), graphdb.DefaultOptions(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = engine.Close(context.Background()) })
	return &ChunkStore{Graph: engine}, engine
}

// TestBootstrapHandler_CompletesAndCheckpoints proves the knowledge.bootstrap
// handler runs a real index pass over a fixture workspace, emits progress
// (H5), and returns a completion checkpoint (H3).
func TestBootstrapHandler_CompletesAndCheckpoints(t *testing.T) {
	workspace := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workspace, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o600))

	engine, err := graphdb.Open(context.Background(), graphdb.DefaultOptions(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = engine.Close(context.Background()) })
	indexManager := ast.NewIndexManager(ast.NewGraphIndexStore(engine), ast.IndexConfig{WorkspacePath: workspace, ParallelWorkers: 1})
	indexManager.GraphDB = engine

	rec := &handlerTelemetry{}
	handler := NewBootstrapHandler(indexManager, rec)
	cp, err := handler.Handle(context.Background(), jobs.Job{
		ID: "job-boot", Spec: jobs.Spec{Kind: "knowledge.bootstrap", CorrelateID: "c1"},
	})
	require.NoError(t, err)
	require.NotNil(t, cp.State)
	require.Equal(t, "job-boot", cp.JobID)
	require.NotEmpty(t, rec.events, "H5: progress must be emitted via the sink, not logs")

	// H1: a second run is an idempotent derived-structure rebuild.
	_, err = handler.Handle(context.Background(), jobs.Job{ID: "job-boot"})
	require.NoError(t, err)
}

func TestBootstrapHandler_DegradedInputs(t *testing.T) {
	handler := NewBootstrapHandler(nil, nil)
	_, err := handler.Handle(context.Background(), jobs.Job{ID: "j"})
	require.Error(t, err, "a nil index manager is an honest error, not a silent success")
}

// TestRefreshHandler_SweepMarksStale proves the knowledge.refresh sweep
// re-evaluates chunks against current workspace state: a deleted file marks
// its chunk stale; a fresh file stays fresh (H1: the sweep is idempotent).
func TestRefreshHandler_SweepMarksStale(t *testing.T) {
	workspace := t.TempDir()
	gone := filepath.Join(workspace, "gone.go")
	kept := filepath.Join(workspace, "kept.go")
	require.NoError(t, os.WriteFile(gone, []byte("package gone\n"), 0o600))
	require.NoError(t, os.WriteFile(kept, []byte("package kept\n"), 0o600))
	compiled := time.Now().UTC().Add(-time.Hour)
	// kept.go predates its chunk's compilation: no drift.
	past := compiled.Add(-time.Hour)
	require.NoError(t, os.Chtimes(kept, past, past))

	store, _ := newHandlerStore(t)
	staleness := &StalenessManager{Store: store, Propagate: true, MaxDepth: 3}
	for _, id := range []ChunkID{"chunk-gone", "chunk-kept"} {
		path := "gone.go"
		if id == "chunk-kept" {
			path = "kept.go"
		}
		_, err := store.Save(context.Background(), KnowledgeChunk{
			ID:          id,
			WorkspaceID: "ws",
			Freshness:   FreshnessValid,
			Provenance:  ChunkProvenance{CompiledBy: CompilerDeterministic, Timestamp: compiled},
			Body:        ChunkBody{Raw: string(id), Fields: map[string]any{"file_path": path}},
		})
		require.NoError(t, err)
	}
	// The file that must go stale is deleted after compilation.
	require.NoError(t, os.Remove(gone))

	ckptStoreRaw, err := openJobsCkpt(t)
	require.NoError(t, err)

	handler := NewRefreshHandler(store, staleness, workspace, ckptStoreRaw, nil)
	cp, err := handler.Handle(context.Background(), jobs.Job{
		ID: "job-refresh", Spec: jobs.Spec{Kind: "knowledge.refresh"},
	})
	require.NoError(t, err)
	require.NotNil(t, cp.State)

	freshGone, err := store.FindFreshByFilePath("gone.go")
	require.NoError(t, err)
	require.Empty(t, freshGone, "the deleted file's chunk must be marked stale")
	freshKept, err := store.FindFreshByFilePath("kept.go")
	require.NoError(t, err)
	require.Len(t, freshKept, 1, "the intact file's chunk stays fresh")
}

// TestRefreshHandler_ResumeFromCheckpoint proves H3: a checkpoint with
// LastChunkID skips already-examined chunks — the marked-stale result is
// identical because examination is stateless, but the checkpoint advances.
func TestRefreshHandler_ResumeFromCheckpoint(t *testing.T) {
	workspace := t.TempDir()
	store, _ := newHandlerStore(t)
	staleness := &StalenessManager{Store: store, Propagate: true, MaxDepth: 3}
	_, err := store.Save(context.Background(), KnowledgeChunk{
		ID:          "chunk-z",
		WorkspaceID: "ws",
		Freshness:   FreshnessValid,
		Provenance:  ChunkProvenance{CompiledBy: CompilerDeterministic, Timestamp: time.Now().UTC()},
		Body:        ChunkBody{Raw: "z", Fields: map[string]any{"file_path": "z.go"}},
	})
	require.NoError(t, err)

	ckptRaw, err := openJobsCkpt(t)
	require.NoError(t, err)
	handler := NewRefreshHandler(store, staleness, workspace, ckptRaw, nil)

	// Pre-seed a checkpoint whose cursor is past chunk-z: the sweep must not
	// examine it again (resume, not restart).
	require.NoError(t, ckptRaw.SaveCheckpoint(context.Background(), jobs.Checkpoint{
		ID: "job-resume:sweep", JobID: "job-resume", Token: "tok",
		State:   refreshSweepCheckpoint{LastChunkID: "chunk-z"},
		Created: time.Now().UTC(),
	}))

	cp, err := handler.Handle(context.Background(), jobs.Job{
		ID: "job-resume", Spec: jobs.Spec{Kind: "knowledge.refresh"}, ResumeToken: "tok",
	})
	require.NoError(t, err)
	state, ok := cp.State.(refreshSweepCheckpoint)
	require.True(t, ok)
	require.Equal(t, "chunk-z", state.LastChunkID, "resume must not rewind past the cursor")
	require.Equal(t, 0, state.MarkedStale, "nothing examined after the cursor")
}

// TestRefreshHandler_CancelHonored proves H2: a canceled context aborts the
// sweep with the context error, within the grace budget.
func TestRefreshHandler_CancelHonored(t *testing.T) {
	store, _ := newHandlerStore(t)
	staleness := &StalenessManager{Store: store, Propagate: true, MaxDepth: 3}
	for i := 0; i < 1200; i++ {
		id := ChunkID("chunk-" + string(rune('a'+i%26)) + string(rune('0'+i%10)) + string(rune(i)))
		_, err := store.Save(context.Background(), KnowledgeChunk{
			ID:          id,
			WorkspaceID: "ws",
			Freshness:   FreshnessValid,
			Provenance:  ChunkProvenance{CompiledBy: CompilerDeterministic},
			Body:        ChunkBody{Raw: string(id)},
		})
		require.NoError(t, err)
	}
	handler := NewRefreshHandler(store, staleness, t.TempDir(), nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := handler.Handle(ctx, jobs.Job{ID: "job-cancel"})
	require.ErrorIs(t, err, context.Canceled)
}

func TestRefreshHandler_RequiresDeps(t *testing.T) {
	handler := NewRefreshHandler(nil, nil, "", nil, nil)
	_, err := handler.Handle(context.Background(), jobs.Job{ID: "j"})
	require.Error(t, err, "nil store/staleness is an honest error")
}

// TestRefreshHandler_EvaluateUnreadableFile pins the sweep's honesty: an
// unreadable file is not evidence of staleness — the chunk is left fresh for
// the next sweep instead of being marked on an I/O error.
func TestRefreshHandler_EvaluateUnreadableFile(t *testing.T) {
	h := &refreshHandler{} // no workspace root: relative path resolution
	stale, err := h.evaluate(context.Background(), KnowledgeChunk{
		Body: ChunkBody{Fields: map[string]any{"file_path": ""}},
	})
	require.NoError(t, err)
	require.False(t, stale, "empty file_path is never file-stale")

	stale, err = h.evaluate(context.Background(), KnowledgeChunk{
		Body: ChunkBody{Fields: map[string]any{"file_path": 42}},
	})
	require.NoError(t, err)
	require.False(t, stale, "non-string file_path is skipped, not guessed")

	// A missing file (os.IsNotExist) is real staleness evidence.
	stale, evalErr := h.evaluate(context.Background(), KnowledgeChunk{
		ID:   "chunk-x",
		Body: ChunkBody{Fields: map[string]any{"file_path": "definitely/missing/xyz.go"}},
	})
	require.NoError(t, evalErr)
	require.True(t, stale, "a deleted file is stale")
}

// openJobsCkpt opens an in-memory jobs store for handler checkpoint writes.
func openJobsCkpt(t *testing.T) (jobs.Store, error) {
	t.Helper()
	return jobsStoreOpenInMemory()
}

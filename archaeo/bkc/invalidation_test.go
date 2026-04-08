package bkc

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeotensions "github.com/lexcodex/relurpify/archaeo/tensions"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	memorydb "github.com/lexcodex/relurpify/framework/memory/db"
)

func TestInvalidationPassHandleRevisionChangedMarksAffectedChunksStale(t *testing.T) {
	store := newTestChunkStore(t)
	workflowStore := newTestWorkflowStore(t, "wf-invalidate")
	pass := &InvalidationPass{
		Store:     store,
		Staleness: &StalenessManager{Store: store, Propagate: true, MaxDepth: 3},
		Tensions:  archaeotensions.Service{Store: workflowStore},
	}
	chunk := withChunkFile(testChunk("chunk-a", "ws", "rev-1"), "main.go")
	chunk.Provenance.WorkflowID = "wf-invalidate"
	_, _ = store.Save(chunk)

	if err := pass.HandleRevisionChanged(context.Background(), CodeRevisionChangedPayload{
		NewRevision:   "rev-2",
		AffectedPaths: []string{"main.go"},
	}); err != nil {
		t.Fatalf("handle revision changed: %v", err)
	}
	got, _, _ := store.Load("chunk-a")
	if got.Freshness != FreshnessStale {
		t.Fatalf("expected chunk to become stale, got %s", got.Freshness)
	}
}

func TestInvalidationPassPropagatesAndSurfacesTensions(t *testing.T) {
	store := newTestChunkStore(t)
	workflowStore := newTestWorkflowStore(t, "wf-propagate")
	pass := &InvalidationPass{
		Store:     store,
		Staleness: &StalenessManager{Store: store, Propagate: true, MaxDepth: 3},
		Tensions:  archaeotensions.Service{Store: workflowStore},
	}
	root := withChunkFile(testChunk("root", "ws", "rev-1"), "pkg/service.go")
	root.Provenance.WorkflowID = "wf-propagate"
	dep := testChunk("dep", "ws", "rev-1")
	dep.Provenance.WorkflowID = "wf-propagate"
	_, _ = store.Save(root)
	_, _ = store.Save(dep)
	_, _ = store.SaveEdge(ChunkEdge{FromChunk: root.ID, ToChunk: dep.ID, Kind: EdgeKindInvalidates})

	if err := pass.HandleRevisionChanged(context.Background(), CodeRevisionChangedPayload{
		NewRevision:   "rev-2",
		AffectedPaths: []string{"pkg/service.go"},
	}); err != nil {
		t.Fatalf("handle revision changed: %v", err)
	}
	depChunk, _, _ := store.Load("dep")
	if depChunk.Freshness != FreshnessStale {
		t.Fatalf("expected invalidated dependency to be stale, got %s", depChunk.Freshness)
	}
	tensions, err := pass.Tensions.ListByWorkflow(context.Background(), "wf-propagate")
	if err != nil {
		t.Fatalf("list tensions: %v", err)
	}
	if len(tensions) != 2 {
		t.Fatalf("expected 2 tensions, got %+v", tensions)
	}
}

func TestInvalidationPassUnverifiedChunkCreatesConfirmedTension(t *testing.T) {
	store := newTestChunkStore(t)
	workflowStore := newTestWorkflowStore(t, "wf-unverified")
	pass := &InvalidationPass{
		Store:     store,
		Staleness: &StalenessManager{Store: store},
		Tensions:  archaeotensions.Service{Store: workflowStore},
	}
	chunk := withChunkFile(testChunk("unverified", "ws", "rev-1"), "main.go")
	chunk.Provenance.WorkflowID = "wf-unverified"
	chunk.Freshness = FreshnessUnverified
	_, _ = store.Save(chunk)

	if err := pass.SurfaceStaleChunks(context.Background(), []string{"unverified"}, []string{"main.go"}, "stale_during_stream"); err != nil {
		t.Fatalf("surface stale chunks: %v", err)
	}
	tensions, err := pass.Tensions.ListByWorkflow(context.Background(), "wf-unverified")
	if err != nil {
		t.Fatalf("list tensions: %v", err)
	}
	if len(tensions) != 1 || tensions[0].Status != archaeodomain.TensionConfirmed {
		t.Fatalf("expected confirmed tension for unverified chunk, got %+v", tensions)
	}
}

func TestInvalidationPassSurfaceStaleDuringStream(t *testing.T) {
	store := newTestChunkStore(t)
	workflowStore := newTestWorkflowStore(t, "wf-stream")
	pass := &InvalidationPass{
		Store:     store,
		Staleness: &StalenessManager{Store: store},
		Tensions:  archaeotensions.Service{Store: workflowStore},
	}
	chunk := testChunk("stream-stale", "ws", "rev-1")
	chunk.Provenance.WorkflowID = "wf-stream"
	_, _ = store.Save(chunk)

	if err := pass.SurfaceStaleDuringStream(context.Background(), &StreamResult{
		StaleDuringStream: []ChunkID{"stream-stale"},
	}); err != nil {
		t.Fatalf("surface stale during stream: %v", err)
	}
	tensions, err := pass.Tensions.ListByWorkflow(context.Background(), "wf-stream")
	if err != nil {
		t.Fatalf("list tensions: %v", err)
	}
	if len(tensions) != 1 || tensions[0].SourceRef != "stream-stale" {
		t.Fatalf("expected stream stale tension, got %+v", tensions)
	}
}

func TestInvalidationPassSurfaceStaleChunksIncludesReasonAndEdgeRefs(t *testing.T) {
	store := newTestChunkStore(t)
	workflowStore := newTestWorkflowStore(t, "wf-reason")
	pass := &InvalidationPass{
		Store:     store,
		Staleness: &StalenessManager{Store: store},
		Tensions:  archaeotensions.Service{Store: workflowStore},
	}
	chunk := testChunk("reason-chunk", "ws", "rev-1")
	chunk.Provenance.WorkflowID = "wf-reason"
	_, _ = store.Save(chunk)
	_, _ = store.Save(testChunk("dep-chunk", "ws", "rev-1"))
	_, _ = store.Save(testChunk("amp-chunk", "ws", "rev-1"))
	_, _ = store.SaveEdge(ChunkEdge{
		FromChunk:  chunk.ID,
		Kind:       EdgeKindDependsOnCodeState,
		Provenance: chunk.Provenance,
		Meta:       map[string]any{"code_state_ref": "abc123"},
	})
	_, _ = store.SaveEdge(ChunkEdge{
		FromChunk: chunk.ID,
		ToChunk:   "dep-chunk",
		Kind:      EdgeKindRequiresContext,
	})
	_, _ = store.SaveEdge(ChunkEdge{
		FromChunk: chunk.ID,
		ToChunk:   "amp-chunk",
		Kind:      EdgeKindAmplifies,
	})

	if err := pass.SurfaceStaleChunks(context.Background(), []string{"reason-chunk"}, []string{"pkg/service.go"}, "revision_drift"); err != nil {
		t.Fatalf("surface stale chunks: %v", err)
	}
	tensions, err := pass.Tensions.ListByWorkflow(context.Background(), "wf-reason")
	if err != nil {
		t.Fatalf("list tensions: %v", err)
	}
	if len(tensions) != 1 {
		t.Fatalf("expected one tension, got %+v", tensions)
	}
	got := tensions[0]
	if got.Description != "knowledge chunk reason-chunk became stale: revision_drift" {
		t.Fatalf("unexpected description: %q", got.Description)
	}
	if !containsString(got.CommentRefs, "depends_on_code_state:abc123") {
		t.Fatalf("expected code-state edge ref in comment refs, got %+v", got.CommentRefs)
	}
	if !containsString(got.CommentRefs, "requires_context:dep-chunk") {
		t.Fatalf("expected requires_context edge ref in comment refs, got %+v", got.CommentRefs)
	}
	if !containsString(got.CommentRefs, "amplifies:amp-chunk") {
		t.Fatalf("expected amplifies edge ref in comment refs, got %+v", got.CommentRefs)
	}
}

func TestInvalidationPassSurfaceStaleChunksNoEdgesCommentRefsEmpty(t *testing.T) {
	store := newTestChunkStore(t)
	workflowStore := newTestWorkflowStore(t, "wf-no-edges")
	pass := &InvalidationPass{
		Store:     store,
		Staleness: &StalenessManager{Store: store},
		Tensions:  archaeotensions.Service{Store: workflowStore},
	}
	chunk := testChunk("plain-chunk", "ws", "rev-1")
	chunk.Provenance.WorkflowID = "wf-no-edges"
	_, _ = store.Save(chunk)

	if err := pass.SurfaceStaleChunks(context.Background(), []string{"plain-chunk"}, nil, "manual_invalidation"); err != nil {
		t.Fatalf("surface stale chunks: %v", err)
	}
	tensions, err := pass.Tensions.ListByWorkflow(context.Background(), "wf-no-edges")
	if err != nil {
		t.Fatalf("list tensions: %v", err)
	}
	if len(tensions) != 1 {
		t.Fatalf("expected one tension, got %+v", tensions)
	}
	if len(tensions[0].CommentRefs) != 0 {
		t.Fatalf("expected empty comment refs, got %+v", tensions[0].CommentRefs)
	}
}

func TestInvalidationPassStartConsumesBusEvents(t *testing.T) {
	store := newTestChunkStore(t)
	workflowStore := newTestWorkflowStore(t, "wf-bus")
	bus := &EventBus{}
	pass := &InvalidationPass{
		Store:     store,
		Staleness: &StalenessManager{Store: store},
		Tensions:  archaeotensions.Service{Store: workflowStore},
		Events:    bus,
	}
	chunk := withChunkFile(testChunk("bus-chunk", "ws", "rev-1"), "main.go")
	chunk.Provenance.WorkflowID = "wf-bus"
	_, _ = store.Save(chunk)

	done := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { done <- pass.Start(ctx) }()
	time.Sleep(20 * time.Millisecond)
	bus.EmitCodeRevisionChanged(CodeRevisionChangedPayload{
		NewRevision:   "rev-2",
		AffectedPaths: []string{"main.go"},
	})
	time.Sleep(80 * time.Millisecond)
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("start loop: %v", err)
	}
	got, _, _ := store.Load("bus-chunk")
	if got.Freshness != FreshnessStale {
		t.Fatalf("expected bus-driven stale chunk, got %s", got.Freshness)
	}
}

func withChunkFile(chunk KnowledgeChunk, path string) KnowledgeChunk {
	if chunk.Body.Fields == nil {
		chunk.Body.Fields = map[string]any{}
	}
	chunk.Body.Fields["file_path"] = path
	return chunk
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func newTestWorkflowStore(t *testing.T, workflowID string) memory.WorkflowStateStore {
	t.Helper()
	store, err := memorydb.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	if err != nil {
		t.Fatalf("open workflow store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.CreateWorkflow(context.Background(), memory.WorkflowRecord{
		WorkflowID:  workflowID,
		TaskID:      "task-" + workflowID,
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "test",
		Status:      memory.WorkflowRunStatusRunning,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	return store
}

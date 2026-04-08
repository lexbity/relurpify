package bkc

import "testing"

func TestViewRendererRegistryRenderViews(t *testing.T) {
	var registry ViewRendererRegistry
	called := false
	registry.Register(ViewKindPattern, func(chunk KnowledgeChunk) (ChunkView, bool) {
		called = true
		return ChunkView{
			Kind: ViewKindPattern,
			Data: map[string]any{"id": chunk.ID},
		}, true
	})
	views := registry.RenderViews(testChunk("view-1", "ws", "rev"), ViewKindPattern)
	if !called {
		t.Fatal("expected registered renderer to be called")
	}
	if len(views) != 1 || views[0].Kind != ViewKindPattern {
		t.Fatalf("unexpected views: %+v", views)
	}
}

func TestViewRendererRegistryUnknownKindReturnsEmpty(t *testing.T) {
	var registry ViewRendererRegistry
	views := registry.RenderViews(testChunk("view-2", "ws", "rev"), ViewKindDecision)
	if len(views) != 0 {
		t.Fatalf("expected no views for unknown kind, got %+v", views)
	}
}

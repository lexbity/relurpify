package framework

import (
	"context"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
)

// TestChunkCreation validates that knowledge chunks can be created
// with stable identifiers and required fields.
func TestChunkCreation(t *testing.T) {
	t.Run("chunk with valid ID can be created", func(t *testing.T) {
		chunk := knowledge.KnowledgeChunk{
			ID:          knowledge.ChunkID("test-chunk-1"),
			WorkspaceID: "test-workspace",
			Body: knowledge.ChunkBody{
				Raw: "test content",
			},
			Provenance: knowledge.ChunkProvenance{
				Sources: []knowledge.ProvenanceSource{
					{Kind: "user", Ref: "test-ref"},
				},
				Timestamp: time.Now().UTC(),
			},
			Freshness: knowledge.FreshnessValid,
			CreatedAt: time.Now().UTC(),
		}

		if chunk.ID != knowledge.ChunkID("test-chunk-1") {
			t.Errorf("expected chunk ID 'test-chunk-1', got %s", chunk.ID)
		}
		if chunk.WorkspaceID != "test-workspace" {
			t.Errorf("expected workspace ID 'test-workspace', got %s", chunk.WorkspaceID)
		}
		if chunk.Body.Raw != "test content" {
			t.Errorf("expected raw content 'test content', got %s", chunk.Body.Raw)
		}
	})

	t.Run("chunk ID is stable", func(t *testing.T) {
		id := knowledge.ChunkID("stable-chunk-id")
		chunk1 := knowledge.KnowledgeChunk{
			ID:          id,
			WorkspaceID: "test-workspace",
			Body:        knowledge.ChunkBody{Raw: "content 1"},
			Provenance: knowledge.ChunkProvenance{
				Sources:   []knowledge.ProvenanceSource{{Kind: "user", Ref: "ref"}},
				Timestamp: time.Now().UTC(),
			},
			Freshness: knowledge.FreshnessValid,
			CreatedAt: time.Now().UTC(),
		}
		chunk2 := knowledge.KnowledgeChunk{
			ID:          id,
			WorkspaceID: "test-workspace",
			Body:        knowledge.ChunkBody{Raw: "content 2"},
			Provenance: knowledge.ChunkProvenance{
				Sources:   []knowledge.ProvenanceSource{{Kind: "user", Ref: "ref"}},
				Timestamp: time.Now().UTC(),
			},
			Freshness: knowledge.FreshnessValid,
			CreatedAt: time.Now().UTC(),
		}

		if chunk1.ID != chunk2.ID {
			t.Error("chunk ID should be stable across instances")
		}
	})

	t.Run("chunk with views can be created", func(t *testing.T) {
		chunk := knowledge.KnowledgeChunk{
			ID:          knowledge.ChunkID("chunk-with-views"),
			WorkspaceID: "test-workspace",
			Body: knowledge.ChunkBody{
				Raw: "test content",
				Fields: map[string]any{
					"field1": "value1",
					"field2": 42,
				},
			},
			Views: []knowledge.ChunkView{
				{Kind: knowledge.ViewKindPattern, Data: "pattern data"},
				{Kind: knowledge.ViewKindDecision, Data: "decision data"},
			},
			Provenance: knowledge.ChunkProvenance{
				Sources:   []knowledge.ProvenanceSource{{Kind: "user", Ref: "ref"}},
				Timestamp: time.Now().UTC(),
			},
			Freshness: knowledge.FreshnessValid,
			CreatedAt: time.Now().UTC(),
		}

		if len(chunk.Views) != 2 {
			t.Errorf("expected 2 views, got %d", len(chunk.Views))
		}
		if chunk.Views[0].Kind != knowledge.ViewKindPattern {
			t.Errorf("expected view kind %s, got %s", knowledge.ViewKindPattern, chunk.Views[0].Kind)
		}
	})
}

// TestChunkStorage validates that chunks can be stored and retrieved
// from the chunk store.
func TestChunkStorage(t *testing.T) {
	t.Run("chunk can be saved to store", func(t *testing.T) {
		env := NewTestEnvironment(t)

		// Create a graph engine for testing
		opts := graphdb.DefaultOptions(env.WorkspacePath)
		graph, err := graphdb.Open(context.Background(), opts)
		if err != nil {
			t.Fatalf("failed to open graph engine: %v", err)
		}
		defer func() { _ = graph.Close(context.Background()) }()
		store := &knowledge.ChunkStore{Graph: graph}

		chunk := knowledge.KnowledgeChunk{
			ID:          knowledge.ChunkID("saved-chunk"),
			WorkspaceID: env.WorkspacePath,
			Body: knowledge.ChunkBody{
				Raw: "saved content",
			},
			Provenance: knowledge.ChunkProvenance{
				Sources:   []knowledge.ProvenanceSource{{Kind: "user", Ref: "ref"}},
				Timestamp: time.Now().UTC(),
			},
			Freshness: knowledge.FreshnessValid,
			CreatedAt: time.Now().UTC(),
		}

		saved, err := store.Save(context.TODO(), chunk)
		if err != nil {
			t.Fatalf("failed to save chunk: %v", err)
		}

		if saved.ID != chunk.ID {
			t.Errorf("expected saved chunk ID %s, got %s", chunk.ID, saved.ID)
		}
		if saved.Version < 1 {
			t.Errorf("expected version >= 1, got %d", saved.Version)
		}
	})

	t.Run("chunk without ID cannot be saved", func(t *testing.T) {
		env := NewTestEnvironment(t)
		opts := graphdb.DefaultOptions(env.WorkspacePath)
		graph, err := graphdb.Open(context.Background(), opts)
		if err != nil {
			t.Fatalf("failed to open graph engine: %v", err)
		}
		defer func() { _ = graph.Close(context.Background()) }()
		store := &knowledge.ChunkStore{Graph: graph}

		chunk := knowledge.KnowledgeChunk{
			ID:          "",
			WorkspaceID: "test-workspace",
			Body:        knowledge.ChunkBody{Raw: "content"},
			Provenance: knowledge.ChunkProvenance{
				Sources:   []knowledge.ProvenanceSource{{Kind: "user", Ref: "ref"}},
				Timestamp: time.Now().UTC(),
			},
			Freshness: knowledge.FreshnessValid,
			CreatedAt: time.Now().UTC(),
		}

		_, err = store.Save(context.TODO(), chunk)
		if err == nil {
			t.Error("saving chunk without ID should fail")
		}
	})

	t.Run("chunk version increments on update", func(t *testing.T) {
		env := NewTestEnvironment(t)
		opts := graphdb.DefaultOptions(env.WorkspacePath)
		graph, err := graphdb.Open(context.Background(), opts)
		if err != nil {
			t.Fatalf("failed to open graph engine: %v", err)
		}
		defer func() { _ = graph.Close(context.Background()) }()
		store := &knowledge.ChunkStore{Graph: graph}

		chunk := knowledge.KnowledgeChunk{
			ID:          knowledge.ChunkID("version-chunk"),
			WorkspaceID: "test-workspace",
			Body:        knowledge.ChunkBody{Raw: "initial content"},
			Provenance: knowledge.ChunkProvenance{
				Sources:   []knowledge.ProvenanceSource{{Kind: "user", Ref: "ref"}},
				Timestamp: time.Now().UTC(),
			},
			Freshness: knowledge.FreshnessValid,
			CreatedAt: time.Now().UTC(),
		}

		// Save initial version
		saved1, err := store.Save(context.TODO(), chunk)
		if err != nil {
			t.Fatalf("failed to save initial chunk: %v", err)
		}

		// Update the chunk
		saved1.Body.Raw = "updated content"
		saved2, err := store.Save(context.TODO(), *saved1)
		if err != nil {
			t.Fatalf("failed to save updated chunk: %v", err)
		}

		if saved2.Version <= saved1.Version {
			t.Errorf("expected version increment, got %d -> %d", saved1.Version, saved2.Version)
		}
	})
}

// TestChunkRetrieval validates that chunks can be retrieved from
// the chunk store by ID.
func TestChunkRetrieval(t *testing.T) {
	t.Run("chunk can be retrieved by ID", func(t *testing.T) {
		env := NewTestEnvironment(t)
		opts := graphdb.DefaultOptions(env.WorkspacePath)
		graph, err := graphdb.Open(context.Background(), opts)
		if err != nil {
			t.Fatalf("failed to open graph engine: %v", err)
		}
		defer func() { _ = graph.Close(context.Background()) }()
		store := &knowledge.ChunkStore{Graph: graph}

		chunk := knowledge.KnowledgeChunk{
			ID:          knowledge.ChunkID("retrieve-chunk"),
			WorkspaceID: "test-workspace",
			Body: knowledge.ChunkBody{
				Raw: "retrievable content",
			},
			Provenance: knowledge.ChunkProvenance{
				Sources:   []knowledge.ProvenanceSource{{Kind: "user", Ref: "ref"}},
				Timestamp: time.Now().UTC(),
			},
			Freshness: knowledge.FreshnessValid,
			CreatedAt: time.Now().UTC(),
		}

		_, err = store.Save(context.TODO(), chunk)
		if err != nil {
			t.Fatalf("failed to save chunk: %v", err)
		}

		retrieved, ok, err := store.Load(chunk.ID)
		if err != nil {
			t.Fatalf("failed to load chunk: %v", err)
		}
		if !ok {
			t.Fatal("chunk should be found")
		}

		if retrieved.ID != chunk.ID {
			t.Errorf("expected chunk ID %s, got %s", chunk.ID, retrieved.ID)
		}
		if retrieved.Body.Raw != chunk.Body.Raw {
			t.Errorf("expected raw content %s, got %s", chunk.Body.Raw, retrieved.Body.Raw)
		}
	})

	t.Run("non-existent chunk returns not found", func(t *testing.T) {
		env := NewTestEnvironment(t)
		opts := graphdb.DefaultOptions(env.WorkspacePath)
		graph, err := graphdb.Open(context.Background(), opts)
		if err != nil {
			t.Fatalf("failed to open graph engine: %v", err)
		}
		defer func() { _ = graph.Close(context.Background()) }()
		store := &knowledge.ChunkStore{Graph: graph}

		_, ok, err := store.Load(knowledge.ChunkID("non-existent"))
		if err != nil {
			t.Fatalf("unexpected error loading non-existent chunk: %v", err)
		}
		if ok {
			t.Error("non-existent chunk should not be found")
		}
	})

	t.Run("multiple chunks can be retrieved", func(t *testing.T) {
		env := NewTestEnvironment(t)
		opts := graphdb.DefaultOptions(env.WorkspacePath)
		graph, err := graphdb.Open(context.Background(), opts)
		if err != nil {
			t.Fatalf("failed to open graph engine: %v", err)
		}
		defer func() { _ = graph.Close(context.Background()) }()
		store := &knowledge.ChunkStore{Graph: graph}

		ids := []knowledge.ChunkID{
			"multi-chunk-1",
			"multi-chunk-2",
			"multi-chunk-3",
		}

		for _, id := range ids {
			chunk := knowledge.KnowledgeChunk{
				ID:          id,
				WorkspaceID: "test-workspace",
				Body:        knowledge.ChunkBody{Raw: "content"},
				Provenance: knowledge.ChunkProvenance{
					Sources:   []knowledge.ProvenanceSource{{Kind: "user", Ref: "ref"}},
					Timestamp: time.Now().UTC(),
				},
				Freshness: knowledge.FreshnessValid,
				CreatedAt: time.Now().UTC(),
			}
			if _, err := store.Save(context.TODO(), chunk); err != nil {
				t.Fatalf("failed to save chunk %s: %v", id, err)
			}
		}

		retrieved, err := store.LoadMany(ids)
		if err != nil {
			t.Fatalf("failed to load multiple chunks: %v", err)
		}

		if len(retrieved) != len(ids) {
			t.Errorf("expected %d chunks, got %d", len(ids), len(retrieved))
		}
	})
}

// TestKnowledgeGraphQuery validates that knowledge graph traversal
// works correctly.
func TestKnowledgeGraphQuery(t *testing.T) {
	t.Run("extract requires context subgraph", func(t *testing.T) {
		env := NewTestEnvironment(t)
		opts := graphdb.DefaultOptions(env.WorkspacePath)
		graph, err := graphdb.Open(context.Background(), opts)
		if err != nil {
			t.Fatalf("failed to open graph engine: %v", err)
		}
		defer func() { _ = graph.Close(context.Background()) }()
		store := &knowledge.ChunkStore{Graph: graph}
		chunkGraph := &knowledge.ChunkGraph{Store: store}

		// Create seed chunk
		seed := knowledge.KnowledgeChunk{
			ID:          knowledge.ChunkID("seed-chunk"),
			WorkspaceID: "test-workspace",
			Body:        knowledge.ChunkBody{Raw: "seed content"},
			Provenance: knowledge.ChunkProvenance{
				Sources:   []knowledge.ProvenanceSource{{Kind: "user", Ref: "ref"}},
				Timestamp: time.Now().UTC(),
			},
			Freshness: knowledge.FreshnessValid,
			CreatedAt: time.Now().UTC(),
		}
		if _, err := store.Save(context.TODO(), seed); err != nil {
			t.Fatalf("failed to save seed: %v", err)
		}

		// Extract subgraph
		chunks, edges, err := chunkGraph.ExtractRequiresContextSubgraph([]knowledge.ChunkID{seed.ID})
		if err != nil {
			t.Fatalf("failed to extract subgraph: %v", err)
		}

		if len(chunks) != 1 {
			t.Errorf("expected 1 chunk, got %d", len(chunks))
		}
		if chunks[0].ID != seed.ID {
			t.Errorf("expected seed chunk ID %s, got %s", seed.ID, chunks[0].ID)
		}
		if len(edges) != 0 {
			t.Errorf("expected 0 edges, got %d", len(edges))
		}
	})

	t.Run("order requires context", func(t *testing.T) {
		env := NewTestEnvironment(t)
		opts := graphdb.DefaultOptions(env.WorkspacePath)
		graph, err := graphdb.Open(context.Background(), opts)
		if err != nil {
			t.Fatalf("failed to open graph engine: %v", err)
		}
		defer func() { _ = graph.Close(context.Background()) }()
		store := &knowledge.ChunkStore{Graph: graph}
		chunkGraph := &knowledge.ChunkGraph{Store: store}

		// Create independent chunks
		chunk1 := knowledge.KnowledgeChunk{
			ID:          knowledge.ChunkID("chunk-1"),
			WorkspaceID: "test-workspace",
			Body:        knowledge.ChunkBody{Raw: "content 1"},
			Provenance: knowledge.ChunkProvenance{
				Sources:   []knowledge.ProvenanceSource{{Kind: "user", Ref: "ref"}},
				Timestamp: time.Now().UTC(),
			},
			Freshness: knowledge.FreshnessValid,
			CreatedAt: time.Now().UTC(),
		}
		chunk2 := knowledge.KnowledgeChunk{
			ID:          knowledge.ChunkID("chunk-2"),
			WorkspaceID: "test-workspace",
			Body:        knowledge.ChunkBody{Raw: "content 2"},
			Provenance: knowledge.ChunkProvenance{
				Sources:   []knowledge.ProvenanceSource{{Kind: "user", Ref: "ref"}},
				Timestamp: time.Now().UTC(),
			},
			Freshness: knowledge.FreshnessValid,
			CreatedAt: time.Now().UTC(),
		}

		if _, err := store.Save(context.TODO(), chunk1); err != nil {
			t.Fatalf("failed to save chunk1: %v", err)
		}
		if _, err := store.Save(context.TODO(), chunk2); err != nil {
			t.Fatalf("failed to save chunk2: %v", err)
		}

		// Order chunks
		ordered, err := chunkGraph.OrderRequiresContext([]knowledge.ChunkID{chunk1.ID, chunk2.ID})
		if err != nil {
			t.Fatalf("failed to order chunks: %v", err)
		}

		if len(ordered) != 2 {
			t.Errorf("expected 2 chunks, got %d", len(ordered))
		}
	})

	t.Run("amplify from chunks", func(t *testing.T) {
		env := NewTestEnvironment(t)
		opts := graphdb.DefaultOptions(env.WorkspacePath)
		graph, err := graphdb.Open(context.Background(), opts)
		if err != nil {
			t.Fatalf("failed to open graph engine: %v", err)
		}
		defer func() { _ = graph.Close(context.Background()) }()
		store := &knowledge.ChunkStore{Graph: graph}
		chunkGraph := &knowledge.ChunkGraph{Store: store}

		// Create seed chunk
		seed := knowledge.KnowledgeChunk{
			ID:          knowledge.ChunkID("amplify-seed"),
			WorkspaceID: "test-workspace",
			Body:        knowledge.ChunkBody{Raw: "seed content"},
			Provenance: knowledge.ChunkProvenance{
				Sources:   []knowledge.ProvenanceSource{{Kind: "user", Ref: "ref"}},
				Timestamp: time.Now().UTC(),
			},
			Freshness: knowledge.FreshnessValid,
			CreatedAt: time.Now().UTC(),
		}
		if _, err := store.Save(context.TODO(), seed); err != nil {
			t.Fatalf("failed to save seed: %v", err)
		}

		// Amplify with max depth 0 (should return no chunks)
		amplified, err := chunkGraph.AmplifyFrom([]knowledge.ChunkID{seed.ID}, 0)
		if err != nil {
			t.Fatalf("failed to amplify: %v", err)
		}

		if len(amplified) != 0 {
			t.Errorf("expected 0 amplified chunks with depth 0, got %d", len(amplified))
		}
	})
}

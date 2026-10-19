package compiler

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
	"codeburg.org/lexbit/relurpify/context/knowledge/retrieval"
)

// newBenchStoreWithCorpus opens a real GraphDB engine in a temp directory and
// saves chunkCount chunks with deterministic IDs (chunk:000 … chunk:NNN),
// fixed content of contentLen bytes, TrustClass "builtin.trusted", and a
// linear chain of EdgeKindRequiresContext edges linking consecutive chunks.
// It returns a ChunkStore wrapping the engine. The engine is closed via
// b.Cleanup.
func newBenchStoreWithCorpus(b *testing.B, chunkCount, contentLen int) *knowledge.ChunkStore {
	b.Helper()
	engine, err := graphdb.Open(context.Background(), graphdb.DefaultOptions(b.TempDir()))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = engine.Close(context.Background()) })
	store := &knowledge.ChunkStore{Graph: engine}
	content := strings.Repeat("x", contentLen)
	for i := 0; i < chunkCount; i++ {
		id := knowledge.ChunkID(fmt.Sprintf("chunk:%03d", i))
		_, err := store.Save(context.Background(), knowledge.KnowledgeChunk{
			ID:         id,
			TrustClass: "builtin.trusted",
			Body:       knowledge.ChunkBody{Raw: content, Fields: map[string]any{"content": content}},
		})
		if err != nil {
			b.Fatal(err)
		}
		if i > 0 {
			prevID := knowledge.ChunkID(fmt.Sprintf("chunk:%03d", i-1))
			_, err := store.SaveEdge(context.Background(), knowledge.ChunkEdge{
				FromChunk: prevID,
				ToChunk:   id,
				Kind:      knowledge.EdgeKindRequiresContext,
				Weight:    1,
			})
			if err != nil {
				b.Fatal(err)
			}
		}
	}
	return store
}

// BenchmarkCompileCold measures the full cold-path compile pipeline over a
// 100-chunk synthetic corpus. Each iteration uses a fresh RunID to guarantee
// a cache miss, exercising the complete 20-function chain from scatter
// through persistCompilationRecord. The benchmark reports both ns/op (via
// -benchmem) and the token result (via b.ReportMetric).
func BenchmarkCompileCold(b *testing.B) {
	store := newBenchStoreWithCorpus(b, 100, 40)
	registry := retrieval.NewRankerRegistry()
	ids := make([]knowledge.ChunkID, 100)
	for i := range ids {
		ids[i] = knowledge.ChunkID(fmt.Sprintf("chunk:%03d", i))
	}
	registry.Register(&staticRanker{name: "bench", ids: ids})
	c := NewCompiler(retrieval.NewRetriever(registry, store), nil, store)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := CompilationRequest{
			Query: retrieval.RetrievalQuery{
				Text:    "bench",
				Anchors: []retrieval.AnchorRef{{AnchorID: "file:main.go", Term: "main.go", Class: "user_file", Active: true}},
			},
			ManifestID:     "manifest-bench",
			PolicyBundleID: "policy-bench",
			EventLogSeq:    100,
			EpochID:        1,
			RunID:          fmt.Sprintf("run-%d", i),
			MaxTokens:      4096,
		}
		result, record, err := c.Compile(ctx, req)
		if err != nil {
			b.Fatal(err)
		}
		if record.CacheHit {
			b.Fatalf("iteration %d: expected cache miss, got cache hit", i)
		}
		b.ReportMetric(float64(result.TotalTokens), "tokens")
	}
}

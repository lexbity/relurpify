package runtime

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/retrieval"
	"codeburg.org/lexbit/relurpify/platform/llm"
	"github.com/stretchr/testify/require"
)

type runtimeFakeEmbedder struct{}

func (runtimeFakeEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, 0, len(texts))
	for _, text := range texts {
		out = append(out, []float32{float32(len(text)), 1})
	}
	return out, nil
}

func (runtimeFakeEmbedder) ModelID() string { return "runtime-fake-v1" }
func (runtimeFakeEmbedder) Dims() int       { return 2 }

type runtimeFakeBackend struct {
	embedder llm.Embedder
}

func (b runtimeFakeBackend) Model() core.LanguageModel { return nil }
func (b runtimeFakeBackend) Embedder() llm.Embedder    { return b.embedder }
func (runtimeFakeBackend) Capabilities() core.BackendCapabilities {
	return core.BackendCapabilities{}
}
func (runtimeFakeBackend) Health(context.Context) (*llm.HealthReport, error) {
	return &llm.HealthReport{State: llm.BackendHealthReady}, nil
}
func (runtimeFakeBackend) ListModels(context.Context) ([]llm.ModelInfo, error) { return nil, nil }
func (runtimeFakeBackend) Warm(context.Context) error                          { return nil }
func (runtimeFakeBackend) Close() error                                        { return nil }
func (runtimeFakeBackend) SetDebugLogging(bool)                                {}

func TestRetrieverSemanticAdapterMapsCandidatesToVectorMatches(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "retrieval.db")
	db, err := sql.Open("sqlite3", dbPath)
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, retrieval.EnsureSchema(context.Background(), db))

	pipeline := retrieval.NewIngestionPipeline(db, runtimeFakeEmbedder{})
	ingested, err := pipeline.Ingest(context.Background(), retrieval.IngestRequest{
		CanonicalURI: filepath.Join(t.TempDir(), "match.go"),
		Content:      []byte("package sample\nfunc Hello() string { return \"needle\" }\n"),
		CorpusScope:  "workspace",
		PolicyTags:   []string{"code"},
	})
	require.NoError(t, err)

	adapter := &retrieverSemanticAdapter{retriever: retrieval.NewRetriever(db, runtimeFakeEmbedder{})}
	results, err := adapter.Query(context.Background(), "needle", 5)
	require.NoError(t, err)
	require.NotEmpty(t, results)
	require.Equal(t, ingested.Chunks[0].ChunkID, results[0].ID)
	require.Contains(t, results[0].Content, "needle")
	require.Equal(t, ingested.Document.CanonicalURI, results[0].Metadata["path"])
}

func TestRetrievalBootstrap_UsesBackendEmbedder(t *testing.T) {
	t.Parallel()

	stub := &runtimeFakeEmbedder{}
	embedder, err := resolveSemanticEmbedder(runtimeFakeBackend{embedder: stub}, Config{}, "ignored-model")
	require.NoError(t, err)
	require.Same(t, stub, embedder)
}

func TestRetrievalBootstrap_NilEmbedder_GracefulFallback(t *testing.T) {
	t.Parallel()

	embedder, err := resolveSemanticEmbedder(nil, Config{}, "")
	require.NoError(t, err)
	require.Nil(t, embedder)
}

package retrieval

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

type fakeEmbedder struct{}

func (fakeEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, 0, len(texts))
	for _, text := range texts {
		out = append(out, []float32{float32(len(text)), 1})
	}
	return out, nil
}

func (fakeEmbedder) ModelID() string { return "fake-v1" }
func (fakeEmbedder) Dims() int       { return 2 }

func TestIngestionPipelineIngestsGoDocument(t *testing.T) {
	db := openRetrievalTestDB(t)
	p := NewIngestionPipeline(db, fakeEmbedder{})
	p.now = func() time.Time { return time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC) }

	result, err := p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "framework/retrieval/sample.go",
		CorpusScope:  "workspace",
		PolicyTags:   []string{"code"},
		Content: []byte(`package sample

type Greeter struct{}

func (Greeter) Hello() string { return "hi" }

func Bye() string { return "bye" }
`),
	})
	require.NoError(t, err)
	require.Equal(t, "go", result.Document.SourceType)
	require.Len(t, result.Chunks, 3)
	require.Len(t, result.Embeddings, 3)
	require.Equal(t, "go/Greeter", result.Chunks[0].StructuralKey)
	require.Equal(t, "go/Greeter.Hello", result.Chunks[1].StructuralKey)

	requireCount(t, db, "retrieval_documents", 1)
	requireCount(t, db, "retrieval_document_versions", 1)
	requireCount(t, db, "retrieval_chunks", 3)
	requireCount(t, db, "retrieval_chunk_versions", 3)
	requireCount(t, db, "retrieval_embeddings", 3)

	var scope string
	err = db.QueryRow(`SELECT corpus_scope FROM retrieval_documents WHERE doc_id = ?`, result.Document.DocID).Scan(&scope)
	require.NoError(t, err)
	require.Equal(t, "workspace", scope)

	var chunkerVersion string
	err = db.QueryRow(`SELECT chunker_version FROM retrieval_documents WHERE doc_id = ?`, result.Document.DocID).Scan(&chunkerVersion)
	require.NoError(t, err)
	require.Equal(t, "chunk-go-0.1.0", chunkerVersion)

	err = db.QueryRow(`SELECT chunker_version FROM retrieval_chunk_versions WHERE chunk_id = ? AND version_id = ?`, result.Chunks[0].ChunkID, result.Version.VersionID).Scan(&chunkerVersion)
	require.NoError(t, err)
	require.Equal(t, "chunk-go-0.1.0", chunkerVersion)
}

func TestIngestionPipelineChunksMarkdownByHeading(t *testing.T) {
	db := openRetrievalTestDB(t)
	p := NewIngestionPipeline(db, nil)
	p.now = func() time.Time { return time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC) }

	result, err := p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "docs/spec.md",
		CorpusScope:  "workspace",
		Content: []byte(`# Intro
Hello

## Details
World
`),
	})
	require.NoError(t, err)
	require.Len(t, result.Chunks, 2)
	require.Equal(t, "md/intro", result.Chunks[0].StructuralKey)
	require.Equal(t, "md/details", result.Chunks[1].StructuralKey)
	require.Equal(t, "chunk-markdown-0.1.0", result.Document.ChunkerVersion)
	require.Equal(t, "chunk-markdown-0.1.0", result.Chunks[0].ChunkerVersion)
}

func TestIngestionPipelineSupersedesPriorVersion(t *testing.T) {
	db := openRetrievalTestDB(t)
	p := NewIngestionPipeline(db, nil)
	p.now = func() time.Time { return time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC) }

	first, err := p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "notes.txt",
		CorpusScope:  "session",
		Content:      []byte("one\n\ntwo"),
	})
	require.NoError(t, err)
	second, err := p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "notes.txt",
		CorpusScope:  "session",
		Content:      []byte("one\n\ntwo\n\nthree"),
	})
	require.NoError(t, err)
	require.Equal(t, first.Document.DocID, second.Document.DocID)
	require.NotEqual(t, first.Version.VersionID, second.Version.VersionID)

	var superseded int
	err = db.QueryRow(`SELECT superseded FROM retrieval_document_versions WHERE version_id = ?`, first.Version.VersionID).Scan(&superseded)
	require.NoError(t, err)
	require.Equal(t, 1, superseded)

	requireCount(t, db, "retrieval_document_versions", 2)
	requireCount(t, db, "retrieval_chunk_versions", 5)

	var activeChunks int
	err = db.QueryRow(`SELECT COUNT(*) FROM retrieval_chunks WHERE doc_id = ? AND tombstoned = 0`, first.Document.DocID).Scan(&activeChunks)
	require.NoError(t, err)
	require.Equal(t, 3, activeChunks)

	var tombstonedChunks int
	err = db.QueryRow(`SELECT COUNT(*) FROM retrieval_chunks WHERE doc_id = ? AND tombstoned = 1`, first.Document.DocID).Scan(&tombstonedChunks)
	require.NoError(t, err)
	require.Equal(t, 0, tombstonedChunks)
}

func TestIngestionPipelineSkipsRedundantVersionForUnchangedContent(t *testing.T) {
	db := openRetrievalTestDB(t)
	p := NewIngestionPipeline(db, fakeEmbedder{})
	p.now = func() time.Time { return time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC) }
	sourceUpdatedAt := time.Date(2026, 3, 10, 8, 0, 0, 0, time.UTC)

	first, err := p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "docs/guide.md",
		Content:      []byte("# Intro\nhello\n"),
		CorpusScope:  "workspace",
		PolicyTags:   []string{"docs"},
		SourceUpdatedAt: &sourceUpdatedAt,
	})
	require.NoError(t, err)
	p.now = func() time.Time { return time.Date(2026, 3, 11, 13, 0, 0, 0, time.UTC) }
	second, err := p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "docs/guide.md",
		Content:      []byte("# Intro\nhello\n"),
		CorpusScope:  "workspace",
		PolicyTags:   []string{"docs", "public"},
	})
	require.NoError(t, err)

	require.Equal(t, first.Version.VersionID, second.Version.VersionID)
	requireCount(t, db, "retrieval_document_versions", 1)
	requireCount(t, db, "retrieval_chunk_versions", 1)

	var policyTags string
	err = db.QueryRow(`SELECT policy_tags_json FROM retrieval_documents WHERE doc_id = ?`, first.Document.DocID).Scan(&policyTags)
	require.NoError(t, err)
	require.JSONEq(t, `["docs","public"]`, policyTags)

	var sourceUpdatedRaw string
	var lastIngestedRaw string
	err = db.QueryRow(`SELECT source_updated_at, last_ingested_at FROM retrieval_documents WHERE doc_id = ?`, first.Document.DocID).Scan(&sourceUpdatedRaw, &lastIngestedRaw)
	require.NoError(t, err)
	require.Equal(t, sourceUpdatedAt.Format(time.RFC3339Nano), sourceUpdatedRaw)
	require.Equal(t, time.Date(2026, 3, 11, 13, 0, 0, 0, time.UTC).Format(time.RFC3339Nano), lastIngestedRaw)

	var tagCount int
	err = db.QueryRow(`SELECT COUNT(*) FROM retrieval_document_policy_tags WHERE doc_id = ?`, first.Document.DocID).Scan(&tagCount)
	require.NoError(t, err)
	require.Equal(t, 2, tagCount)
}

func TestIngestionPipelineBackfillsEmbeddings(t *testing.T) {
	db := openRetrievalTestDB(t)
	p := NewIngestionPipeline(db, fakeEmbedder{})
	p.now = func() time.Time { return time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC) }

	result, err := p.Ingest(context.Background(), IngestRequest{
		CanonicalURI:   "notes.txt",
		Content:        []byte("one\n\ntwo"),
		CorpusScope:    "session",
		SkipEmbeddings: true,
	})
	require.NoError(t, err)
	require.Empty(t, result.Embeddings)
	requireCount(t, db, "retrieval_embeddings", 0)

	embeddings, err := p.BackfillEmbeddings(context.Background(), BackfillRequest{DocID: result.Document.DocID, VersionID: result.Version.VersionID})
	require.NoError(t, err)
	require.Len(t, embeddings, 2)
	requireCount(t, db, "retrieval_embeddings", 2)
}

func TestIngestionPipelineIngestFile(t *testing.T) {
	db := openRetrievalTestDB(t)
	p := NewIngestionPipeline(db, nil)
	p.now = func() time.Time { return time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC) }

	dir := t.TempDir()
	path := filepath.Join(dir, "sample.md")
	err := os.WriteFile(path, []byte("# Intro\nhello\n"), 0o644)
	require.NoError(t, err)

	result, err := p.IngestFile(context.Background(), path, "workspace", []string{"docs"})
	require.NoError(t, err)
	require.Equal(t, CanonicalizeURI(path), result.Document.CanonicalURI)
	require.Len(t, result.Chunks, 1)
}

func TestIngestionPipelineTombstonesDocument(t *testing.T) {
	db := openRetrievalTestDB(t)
	p := NewIngestionPipeline(db, fakeEmbedder{})
	p.now = func() time.Time { return time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC) }

	result, err := p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "notes.txt",
		Content:      []byte("one\n\ntwo"),
		CorpusScope:  "session",
	})
	require.NoError(t, err)

	require.NoError(t, p.TombstoneDocument(context.Background(), "notes.txt"))

	var activeChunks int
	err = db.QueryRow(`SELECT COUNT(*) FROM retrieval_chunks WHERE doc_id = ? AND tombstoned = 0`, result.Document.DocID).Scan(&activeChunks)
	require.NoError(t, err)
	require.Equal(t, 0, activeChunks)

	var tombstonedVersions int
	err = db.QueryRow(`SELECT COUNT(*) FROM retrieval_chunk_versions WHERE doc_id = ? AND tombstoned = 1`, result.Document.DocID).Scan(&tombstonedVersions)
	require.NoError(t, err)
	require.Equal(t, 2, tombstonedVersions)

	var ftsRows int
	err = db.QueryRow(`SELECT COUNT(*) FROM retrieval_chunks_fts WHERE doc_id = ?`, result.Document.DocID).Scan(&ftsRows)
	require.NoError(t, err)
	require.Equal(t, 0, ftsRows)
}

func openRetrievalTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", "file::memory:?cache=shared")
	require.NoError(t, err)
	require.NoError(t, EnsureSchema(context.Background(), db))
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func requireCount(t *testing.T, db *sql.DB, table string, want int) {
	t.Helper()
	var got int
	err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&got)
	require.NoError(t, err)
	require.Equal(t, want, got)
}

package retrieval

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMaintenanceCompactPreservesActiveRetrievalResults(t *testing.T) {
	db := openRetrievalTestDB(t)
	pipeline := NewIngestionPipeline(db, fakeEmbedder{})
	pipeline.now = func() time.Time { return time.Date(2026, 3, 11, 9, 0, 0, 0, time.UTC) }

	_, err := pipeline.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "notes.txt",
		CorpusScope:  "workspace",
		Content:      []byte("alpha\n\nbeta"),
	})
	require.NoError(t, err)

	pipeline.now = func() time.Time { return time.Date(2026, 3, 11, 10, 0, 0, 0, time.UTC) }
	latest, err := pipeline.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "notes.txt",
		CorpusScope:  "workspace",
		Content:      []byte("alpha\n\nbeta\n\ngamma"),
	})
	require.NoError(t, err)

	retriever := NewRetriever(db, fakeEmbedder{})
	before, err := retriever.RetrieveCandidates(context.Background(), RetrievalQuery{
		Text:  "gamma",
		Scope: "workspace",
		Limit: 5,
	})
	require.NoError(t, err)
	require.NotEmpty(t, before.Fused)

	requireCount(t, db, "retrieval_document_versions", 2)
	requireCount(t, db, "retrieval_chunk_versions", 5)

	maintenance := NewMaintenance(db, fakeEmbedder{})
	result, err := maintenance.Compact(context.Background(), CompactionOptions{})
	require.NoError(t, err)
	require.GreaterOrEqual(t, result.DeletedChunkVersions, 2)
	require.GreaterOrEqual(t, result.DeletedDocumentVersions, 1)

	after, err := retriever.RetrieveCandidates(context.Background(), RetrievalQuery{
		Text:  "gamma",
		Scope: "workspace",
		Limit: 5,
	})
	require.NoError(t, err)

	// Compare without Derivation field (timestamps differ on each run)
	require.Len(t, after.Fused, len(before.Fused))
	for i := range before.Fused {
		require.Equal(t, before.Fused[i].ChunkID, after.Fused[i].ChunkID)
		require.Equal(t, before.Fused[i].DocID, after.Fused[i].DocID)
		require.Equal(t, before.Fused[i].VersionID, after.Fused[i].VersionID)
		require.Equal(t, before.Fused[i].FusedScore, after.Fused[i].FusedScore)
		require.Equal(t, before.Fused[i].MatchedBySparse, after.Fused[i].MatchedBySparse)
		require.Equal(t, before.Fused[i].MatchedByDense, after.Fused[i].MatchedByDense)
	}
	requireCount(t, db, "retrieval_document_versions", 1)
	requireCount(t, db, "retrieval_chunk_versions", len(latest.Chunks))
}

func TestMaintenanceRebuildSearchIndexAndEmbeddings(t *testing.T) {
	db := openRetrievalTestDB(t)
	pipeline := NewIngestionPipeline(db, fakeEmbedder{})
	pipeline.now = func() time.Time { return time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC) }

	result, err := pipeline.Ingest(context.Background(), IngestRequest{
		CanonicalURI:   "docs/spec.md",
		CorpusScope:    "workspace",
		SkipEmbeddings: true,
		Content: []byte(`# Intro
alpha

## Details
beta
`),
	})
	require.NoError(t, err)
	requireCount(t, db, "retrieval_embeddings", 0)

	_, err = db.Exec(`DELETE FROM retrieval_chunks_fts`)
	require.NoError(t, err)
	requireCount(t, db, "retrieval_chunks_fts", 0)

	maintenance := NewMaintenance(db, fakeEmbedder{})
	reindexed, err := maintenance.RebuildSearchIndex(context.Background())
	require.NoError(t, err)
	require.Equal(t, len(result.Chunks), reindexed)
	requireCount(t, db, "retrieval_chunks_fts", len(result.Chunks))

	rebuilt, err := maintenance.RebuildEmbeddings(context.Background(), EmbeddingRebuildOptions{
		DocID: result.Document.DocID,
		Force: true,
	})
	require.NoError(t, err)
	require.Equal(t, len(result.Chunks), rebuilt)
	requireCount(t, db, "retrieval_embeddings", len(result.Chunks))
}

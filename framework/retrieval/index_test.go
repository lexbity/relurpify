package retrieval

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

func TestSparseIndexSearchRanksActiveMatches(t *testing.T) {
	db := openRetrievalTestDB(t)
	p := NewIngestionPipeline(db, fakeEmbedder{})
	p.now = func() time.Time { return time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC) }

	_, err := p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "doc1.txt",
		Content:      []byte("apple apple\n\nbanana"),
		CorpusScope:  "workspace",
	})
	require.NoError(t, err)
	second, err := p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "doc2.txt",
		Content:      []byte("apple\n\ncarrot"),
		CorpusScope:  "workspace",
	})
	require.NoError(t, err)

	index := NewSparseIndex(db)
	results, err := index.Search(context.Background(), IndexQuery{Text: "apple", Limit: 5})
	require.NoError(t, err)
	require.Len(t, results, 2)
	require.Equal(t, CandidateSourceSparse, results[0].Source)
	require.Equal(t, "doc:"+shortStableHash("doc1.txt"), results[0].DocID)
	require.Equal(t, second.Document.DocID, results[1].DocID)
	require.GreaterOrEqual(t, results[0].Score, results[1].Score)
}

func TestSparseIndexSearchRespectsAllowlistAndActiveRows(t *testing.T) {
	db := openRetrievalTestDB(t)
	p := NewIngestionPipeline(db, nil)
	p.now = func() time.Time { return time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC) }

	active, err := p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "active.txt",
		Content:      []byte("alpha\n\nbeta"),
	})
	require.NoError(t, err)
	_, err = p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "stale.txt",
		Content:      []byte("alpha\n\nbeta"),
	})
	require.NoError(t, err)
	require.NoError(t, p.TombstoneDocument(context.Background(), "stale.txt"))

	index := NewSparseIndex(db)
	results, err := index.Search(context.Background(), IndexQuery{
		Text:          "alpha",
		AllowChunkIDs: []string{active.Chunks[0].ChunkID},
		Limit:         5,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, active.Chunks[0].ChunkID, results[0].ChunkID)
}

func TestDenseIndexSearchRanksByCosineSimilarity(t *testing.T) {
	db := openRetrievalTestDB(t)
	p := NewIngestionPipeline(db, fakeEmbedder{})
	p.now = func() time.Time { return time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC) }

	first, err := p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "dense1.txt",
		Content:      []byte("apple apple"),
	})
	require.NoError(t, err)
	second, err := p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "dense2.txt",
		Content:      []byte("apple apple apple apple apple"),
	})
	require.NoError(t, err)

	index := NewDenseIndex(db, fakeEmbedder{})
	results, err := index.Search(context.Background(), IndexQuery{Text: "apple apple", Limit: 5})
	require.NoError(t, err)
	require.Len(t, results, 2)
	require.Equal(t, CandidateSourceDense, results[0].Source)
	require.Equal(t, first.Chunks[0].ChunkID, results[0].ChunkID)
	require.Equal(t, second.Chunks[0].ChunkID, results[1].ChunkID)
	require.GreaterOrEqual(t, results[0].Score, results[1].Score)
}

func TestDenseIndexSearchRespectsAllowlistAndTombstones(t *testing.T) {
	db := openRetrievalTestDB(t)
	p := NewIngestionPipeline(db, fakeEmbedder{})
	p.now = func() time.Time { return time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC) }

	active, err := p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "a.txt",
		Content:      []byte("one two"),
	})
	require.NoError(t, err)
	_, err = p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "b.txt",
		Content:      []byte("one two"),
	})
	require.NoError(t, err)
	require.NoError(t, p.TombstoneDocument(context.Background(), "b.txt"))

	index := NewDenseIndex(db, fakeEmbedder{})
	results, err := index.Search(context.Background(), IndexQuery{
		Text:          "one two",
		AllowChunkIDs: []string{active.Chunks[0].ChunkID},
		Limit:         10,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, active.Chunks[0].ChunkID, results[0].ChunkID)
}

func TestSparseFallbackSearchWorksWithoutFTS(t *testing.T) {
	db, err := sql.Open("sqlite3", "file::memory:?cache=shared")
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, EnsureSchema(context.Background(), db))

	_, err = db.Exec(`DROP TABLE retrieval_chunks_fts`)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TABLE retrieval_chunks_fts (
		chunk_id TEXT NOT NULL,
		doc_id TEXT NOT NULL,
		version_id TEXT NOT NULL,
		text TEXT NOT NULL,
		PRIMARY KEY(chunk_id, version_id)
	)`)
	require.NoError(t, err)

	p := NewIngestionPipeline(db, nil)
	p.now = func() time.Time { return time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC) }
	_, err = p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "fallback.txt",
		Content:      []byte("zebra alpha zebra"),
	})
	require.NoError(t, err)

	index := NewSparseIndex(db)
	results, err := index.Search(context.Background(), IndexQuery{Text: "zebra", Limit: 5})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, CandidateSourceSparse, results[0].Source)
}

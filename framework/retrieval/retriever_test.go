package retrieval

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMetadataPrefilterReturnsActiveAllowlist(t *testing.T) {
	db := openRetrievalTestDB(t)
	p := NewIngestionPipeline(db, nil)
	p.now = func() time.Time { return time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC) }

	first, err := p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "a.md",
		Content:      []byte("# Intro\nhello\n"),
		CorpusScope:  "workspace",
		PolicyTags:   []string{"docs"},
	})
	require.NoError(t, err)
	_, err = p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "b.txt",
		Content:      []byte("hello\n\nworld"),
		CorpusScope:  "workspace",
		PolicyTags:   []string{"notes"},
	})
	require.NoError(t, err)
	require.NoError(t, p.TombstoneDocument(context.Background(), "b.txt"))

	filter := NewMetadataPrefilter(db)
	result, err := filter.Prefilter(context.Background(), RetrievalQuery{Scope: "workspace"})
	require.NoError(t, err)
	require.Equal(t, 1, result.FilteredIn)
	require.Equal(t, 1, len(result.AllowChunkIDs))
	require.Equal(t, first.Chunks[0].ChunkID, result.AllowChunkIDs[0])
	require.Contains(t, result.FilterSummary, "active=")
	require.Contains(t, result.FilterSummary, "filtered=1")
}

func TestMetadataPrefilterUsesMetadataBackedActiveChunkCount(t *testing.T) {
	db := openRetrievalTestDB(t)
	p := NewIngestionPipeline(db, nil)
	p.now = func() time.Time { return time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC) }

	_, err := p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "count-a.txt",
		Content:      []byte("alpha"),
		CorpusScope:  "workspace",
	})
	require.NoError(t, err)
	_, err = p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "count-b.txt",
		Content:      []byte("beta"),
		CorpusScope:  "workspace",
	})
	require.NoError(t, err)

	count, err := currentActiveChunkCount(context.Background(), db)
	require.NoError(t, err)
	require.Equal(t, 2, count)

	require.NoError(t, p.TombstoneDocument(context.Background(), "count-b.txt"))
	count, err = currentActiveChunkCount(context.Background(), db)
	require.NoError(t, err)
	require.Equal(t, 1, count)

	filter := NewMetadataPrefilter(db)
	result, err := filter.Prefilter(context.Background(), RetrievalQuery{Scope: "workspace"})
	require.NoError(t, err)
	require.Contains(t, result.FilterSummary, "active=1")
}

func TestMetadataPrefilterFiltersByScopeAndSourceType(t *testing.T) {
	db := openRetrievalTestDB(t)
	p := NewIngestionPipeline(db, nil)
	p.now = func() time.Time { return time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC) }

	workspaceDoc, err := p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "guide.md",
		Content:      []byte("# Intro\nhello\n"),
		CorpusScope:  "workspace",
	})
	require.NoError(t, err)
	_, err = p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "session.txt",
		Content:      []byte("hello\n\nworld"),
		CorpusScope:  "session",
	})
	require.NoError(t, err)

	filter := NewMetadataPrefilter(db)
	result, err := filter.Prefilter(context.Background(), RetrievalQuery{
		Scope:       "workspace",
		SourceTypes: []string{"markdown"},
	})
	require.NoError(t, err)
	require.Len(t, result.AllowChunkIDs, 1)
	require.Equal(t, workspaceDoc.Chunks[0].ChunkID, result.AllowChunkIDs[0])
}

func TestMetadataPrefilterFiltersByUpdatedAfter(t *testing.T) {
	db := openRetrievalTestDB(t)
	p := NewIngestionPipeline(db, nil)

	p.now = func() time.Time { return time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC) }
	_, err := p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "old.txt",
		Content:      []byte("old\n\ncontent"),
	})
	require.NoError(t, err)

	p.now = func() time.Time { return time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC) }
	newer, err := p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "new.txt",
		Content:      []byte("new\n\ncontent"),
	})
	require.NoError(t, err)

	cutoff := time.Date(2026, 3, 11, 0, 0, 0, 0, time.UTC)
	filter := NewMetadataPrefilter(db)
	result, err := filter.Prefilter(context.Background(), RetrievalQuery{
		UpdatedAfter: &cutoff,
	})
	require.NoError(t, err)
	require.Len(t, result.AllowChunkIDs, 2)
	require.Equal(t, newer.Document.DocID, result.Chunks[0].DocID)
}

func TestMetadataPrefilterUsesSourceFreshnessNotLastIngestedAt(t *testing.T) {
	db := openRetrievalTestDB(t)
	p := NewIngestionPipeline(db, nil)

	sourceTime := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	p.now = func() time.Time { return sourceTime }
	_, err := p.Ingest(context.Background(), IngestRequest{
		CanonicalURI:    "stable.txt",
		Content:         []byte("unchanged content"),
		SourceUpdatedAt: &sourceTime,
	})
	require.NoError(t, err)

	reingestTime := time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC)
	p.now = func() time.Time { return reingestTime }
	_, err = p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "stable.txt",
		Content:      []byte("unchanged content"),
	})
	require.NoError(t, err)

	cutoff := time.Date(2026, 3, 11, 0, 0, 0, 0, time.UTC)
	filter := NewMetadataPrefilter(db)
	result, err := filter.Prefilter(context.Background(), RetrievalQuery{
		UpdatedAfter: &cutoff,
	})
	require.NoError(t, err)
	require.Empty(t, result.AllowChunkIDs)
}

func TestMetadataPrefilterFiltersByPolicyTags(t *testing.T) {
	db := openRetrievalTestDB(t)
	p := NewIngestionPipeline(db, nil)
	p.now = func() time.Time { return time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC) }

	tagged, err := p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "tagged.md",
		Content:      []byte("# Intro\nhello\n"),
		PolicyTags:   []string{"public", "docs"},
	})
	require.NoError(t, err)
	_, err = p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "private.md",
		Content:      []byte("# Intro\nsecret\n"),
		PolicyTags:   []string{"private"},
	})
	require.NoError(t, err)

	filter := NewMetadataPrefilter(db)
	result, err := filter.Prefilter(context.Background(), RetrievalQuery{
		PolicyTags: []string{"public", "docs"},
	})
	require.NoError(t, err)
	require.Len(t, result.AllowChunkIDs, 1)
	require.Equal(t, tagged.Chunks[0].ChunkID, result.AllowChunkIDs[0])
	require.Contains(t, result.FilterSummary, "policy_tags=public,docs")
}

func TestRetrieverFusesOverlapCandidates(t *testing.T) {
	db := openRetrievalTestDB(t)
	p := NewIngestionPipeline(db, fakeEmbedder{})
	p.now = func() time.Time { return time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC) }

	shared, err := p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "shared.txt",
		Content:      []byte("apple apple"),
	})
	require.NoError(t, err)
	_, err = p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "other.txt",
		Content:      []byte("apple"),
	})
	require.NoError(t, err)

	retriever := NewRetriever(db, fakeEmbedder{})
	result, err := retriever.RetrieveCandidates(context.Background(), RetrievalQuery{
		Text:  "apple apple",
		Scope: "workspace",
		Limit: 10,
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.Fused)
	require.Equal(t, shared.Chunks[0].ChunkID, result.Fused[0].ChunkID)
	require.True(t, result.Fused[0].MatchedBySparse)
	require.True(t, result.Fused[0].MatchedByDense)
}

func TestRetrieverReturnsSparseOnlyCandidatesWithoutDenseIndex(t *testing.T) {
	db := openRetrievalTestDB(t)
	p := NewIngestionPipeline(db, nil)
	p.now = func() time.Time { return time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC) }

	doc, err := p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "sparse.txt",
		Content:      []byte("banana banana"),
	})
	require.NoError(t, err)

	retriever := NewRetriever(db, nil)
	result, err := retriever.RetrieveCandidates(context.Background(), RetrievalQuery{
		Text:  "banana",
		Scope: "workspace",
		Limit: 10,
	})
	require.NoError(t, err)
	require.Len(t, result.Dense, 0)
	require.NotEmpty(t, result.Fused)
	require.Equal(t, doc.Chunks[0].ChunkID, result.Fused[0].ChunkID)
	require.True(t, result.Fused[0].MatchedBySparse)
	require.False(t, result.Fused[0].MatchedByDense)
}

func TestRetrieverReturnsDenseOnlyCandidatesWhenSparseMisses(t *testing.T) {
	db := openRetrievalTestDB(t)
	p := NewIngestionPipeline(db, fakeEmbedder{})
	p.now = func() time.Time { return time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC) }

	doc, err := p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "dense-only.txt",
		Content:      []byte("zz"),
	})
	require.NoError(t, err)
	_, err = p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "other.txt",
		Content:      []byte("yyyy"),
	})
	require.NoError(t, err)

	retriever := NewRetriever(db, fakeEmbedder{})
	result, err := retriever.RetrieveCandidates(context.Background(), RetrievalQuery{
		Text:  "qq",
		Scope: "workspace",
		Limit: 10,
	})
	require.NoError(t, err)
	require.Len(t, result.Sparse, 0)
	require.NotEmpty(t, result.Dense)
	require.Equal(t, doc.Chunks[0].ChunkID, result.Fused[0].ChunkID)
	require.False(t, result.Fused[0].MatchedBySparse)
	require.True(t, result.Fused[0].MatchedByDense)
}

func TestRetrieverFusedOrderingIsDeterministic(t *testing.T) {
	db := openRetrievalTestDB(t)
	p := NewIngestionPipeline(db, fakeEmbedder{})
	p.now = func() time.Time { return time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC) }

	_, err := p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "a.txt",
		Content:      []byte("alpha"),
	})
	require.NoError(t, err)
	_, err = p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "b.txt",
		Content:      []byte("alpha"),
	})
	require.NoError(t, err)

	retriever := NewRetriever(db, fakeEmbedder{})
	first, err := retriever.RetrieveCandidates(context.Background(), RetrievalQuery{
		Text:  "alpha",
		Scope: "workspace",
		Limit: 10,
	})
	require.NoError(t, err)
	second, err := retriever.RetrieveCandidates(context.Background(), RetrievalQuery{
		Text:  "alpha",
		Scope: "workspace",
		Limit: 10,
	})
	require.NoError(t, err)

	// Compare without Derivation field (timestamps differ on each run)
	require.Len(t, second.Fused, len(first.Fused))
	for i := range first.Fused {
		require.Equal(t, first.Fused[i].ChunkID, second.Fused[i].ChunkID)
		require.Equal(t, first.Fused[i].DocID, second.Fused[i].DocID)
		require.Equal(t, first.Fused[i].VersionID, second.Fused[i].VersionID)
		require.Equal(t, first.Fused[i].FusedScore, second.Fused[i].FusedScore)
		require.Equal(t, first.Fused[i].MatchedBySparse, second.Fused[i].MatchedBySparse)
		require.Equal(t, first.Fused[i].MatchedByDense, second.Fused[i].MatchedByDense)
	}
}

func TestRetrieverDoesNotUseFinalLimitToCapMetadataPrefilter(t *testing.T) {
	db := openRetrievalTestDB(t)
	p := NewIngestionPipeline(db, nil)

	p.now = func() time.Time { return time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC) }
	matching, err := p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "matching.txt",
		Content:      []byte("needle needle"),
		CorpusScope:  "workspace",
	})
	require.NoError(t, err)

	p.now = func() time.Time { return time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC) }
	_, err = p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "recent.txt",
		Content:      []byte("completely unrelated"),
		CorpusScope:  "workspace",
	})
	require.NoError(t, err)

	retriever := NewRetriever(db, nil)
	result, err := retriever.RetrieveCandidates(context.Background(), RetrievalQuery{
		Text:  "needle",
		Scope: "workspace",
		Limit: 1,
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.Fused)
	require.Equal(t, matching.Chunks[0].ChunkID, result.Fused[0].ChunkID)
}

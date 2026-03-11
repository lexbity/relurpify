package retrieval

import (
	"context"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

func TestContextPackerPacksStructuredBlocksWithCitations(t *testing.T) {
	db := openRetrievalTestDB(t)
	p := NewIngestionPipeline(db, nil)
	p.now = func() time.Time { return time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC) }

	doc, err := p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "guide.md",
		Content: []byte(`# Intro
hello

## Details
world
`),
	})
	require.NoError(t, err)

	packer := NewContextPacker(db)
	result, err := packer.Pack(context.Background(), []RankedCandidate{
		{ChunkID: doc.Chunks[0].ChunkID, VersionID: doc.Version.VersionID, DocID: doc.Document.DocID, FusedScore: 1},
	}, PackingOptions{MaxTokens: 500})
	require.NoError(t, err)
	require.Len(t, result.Blocks, 1)
	block, ok := result.Blocks[0].(core.StructuredContentBlock)
	require.True(t, ok)
	payload := block.Data.(map[string]any)
	require.Equal(t, "retrieval_evidence", payload["type"])
	require.Contains(t, payload["text"].(string), "Intro")
	citations := payload["citations"].([]PackedCitation)
	require.Len(t, citations, 1)
	require.Equal(t, doc.Chunks[0].ChunkID, citations[0].ChunkID)
}

func TestContextPackerDedupesRepeatedChunkIDs(t *testing.T) {
	db := openRetrievalTestDB(t)
	p := NewIngestionPipeline(db, nil)
	p.now = func() time.Time { return time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC) }

	doc, err := p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "notes.txt",
		Content:      []byte("one\n\ntwo"),
	})
	require.NoError(t, err)

	packer := NewContextPacker(db)
	result, err := packer.Pack(context.Background(), []RankedCandidate{
		{ChunkID: doc.Chunks[0].ChunkID, VersionID: doc.Version.VersionID, DocID: doc.Document.DocID, FusedScore: 1},
		{ChunkID: doc.Chunks[0].ChunkID, VersionID: doc.Version.VersionID, DocID: doc.Document.DocID, FusedScore: 0.9},
	}, PackingOptions{MaxTokens: 500})
	require.NoError(t, err)
	require.Len(t, result.Blocks, 1)
	require.Equal(t, "deduped", result.DroppedChunks[doc.Chunks[0].ChunkID])
}

func TestContextPackerStitchesAdjacentChunks(t *testing.T) {
	db := openRetrievalTestDB(t)
	p := NewIngestionPipeline(db, nil)
	p.now = func() time.Time { return time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC) }

	doc, err := p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "notes.txt",
		Content:      []byte("first\n\nsecond"),
	})
	require.NoError(t, err)
	require.Len(t, doc.Chunks, 2)

	packer := NewContextPacker(db)
	result, err := packer.Pack(context.Background(), []RankedCandidate{
		{ChunkID: doc.Chunks[0].ChunkID, VersionID: doc.Version.VersionID, DocID: doc.Document.DocID, FusedScore: 1},
		{ChunkID: doc.Chunks[1].ChunkID, VersionID: doc.Version.VersionID, DocID: doc.Document.DocID, FusedScore: 0.9},
	}, PackingOptions{MaxTokens: 500})
	require.NoError(t, err)
	require.Len(t, result.Blocks, 1)
	block := result.Blocks[0].(core.StructuredContentBlock)
	payload := block.Data.(map[string]any)
	require.Contains(t, payload["text"].(string), "first")
	require.Contains(t, payload["text"].(string), "second")
	require.Len(t, payload["citations"].([]PackedCitation), 2)
}

func TestContextPackerRespectsBudgetAndPerDocumentCap(t *testing.T) {
	db := openRetrievalTestDB(t)
	p := NewIngestionPipeline(db, nil)
	p.now = func() time.Time { return time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC) }

	doc, err := p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "caps.txt",
		Content:      []byte("one\n\ntwo\n\nthree\n\nfour"),
	})
	require.NoError(t, err)

	packer := NewContextPacker(db)
	result, err := packer.Pack(context.Background(), []RankedCandidate{
		{ChunkID: doc.Chunks[0].ChunkID, VersionID: doc.Version.VersionID, DocID: doc.Document.DocID, FusedScore: 1},
		{ChunkID: doc.Chunks[1].ChunkID, VersionID: doc.Version.VersionID, DocID: doc.Document.DocID, FusedScore: 0.9},
		{ChunkID: doc.Chunks[2].ChunkID, VersionID: doc.Version.VersionID, DocID: doc.Document.DocID, FusedScore: 0.8},
		{ChunkID: doc.Chunks[3].ChunkID, VersionID: doc.Version.VersionID, DocID: doc.Document.DocID, FusedScore: 0.7},
	}, PackingOptions{MaxTokens: 3, MaxPerDocument: 2})
	require.NoError(t, err)
	require.LessOrEqual(t, result.TokensUsed, 3)
	require.LessOrEqual(t, len(result.InjectedChunks), 2)
}

func TestContextPackerPreservesFusedCandidateOrder(t *testing.T) {
	db := openRetrievalTestDB(t)
	p := NewIngestionPipeline(db, nil)
	p.now = func() time.Time { return time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC) }

	first, err := p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "zeta.txt",
		Content:      []byte("zeta"),
	})
	require.NoError(t, err)
	second, err := p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "alpha.txt",
		Content:      []byte("alpha"),
	})
	require.NoError(t, err)

	packer := NewContextPacker(db)
	result, err := packer.Pack(context.Background(), []RankedCandidate{
		{ChunkID: first.Chunks[0].ChunkID, VersionID: first.Version.VersionID, DocID: first.Document.DocID, FusedScore: 2},
		{ChunkID: second.Chunks[0].ChunkID, VersionID: second.Version.VersionID, DocID: second.Document.DocID, FusedScore: 1},
	}, PackingOptions{MaxTokens: 500})
	require.NoError(t, err)
	require.Len(t, result.Blocks, 2)

	firstBlock := result.Blocks[0].(core.StructuredContentBlock)
	firstPayload := firstBlock.Data.(map[string]any)
	firstCitations := firstPayload["citations"].([]PackedCitation)
	require.Equal(t, first.Chunks[0].ChunkID, firstCitations[0].ChunkID)

	secondBlock := result.Blocks[1].(core.StructuredContentBlock)
	secondPayload := secondBlock.Data.(map[string]any)
	secondCitations := secondPayload["citations"].([]PackedCitation)
	require.Equal(t, second.Chunks[0].ChunkID, secondCitations[0].ChunkID)
}

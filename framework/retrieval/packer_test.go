package retrieval

import (
	"context"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
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
	require.NotEmpty(t, payload["summary"])
	reference, ok := payload["reference"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, string(core.ContextReferenceRetrievalEvidence), reference["kind"])
	citations := payload["citations"].([]PackedCitation)
	require.Len(t, citations, 1)
	require.Equal(t, doc.Chunks[0].ChunkID, citations[0].ChunkID)

	items := result.ContextItems()
	require.Len(t, items, 1)
	retrievalItem, ok := items[0].(*core.RetrievalContextItem)
	require.True(t, ok)
	require.Equal(t, doc.Document.DocID, retrievalItem.Reference.URI)
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

// Phase 5 tests: Semantic Provenance - Surfacing & Consumption

func TestPackedBlocksContainAnchorMetadataWhenAnchorsExist(t *testing.T) {
	db := openRetrievalTestDB(t)
	p := NewIngestionPipeline(db, fakeEmbedder{})
	p.now = func() time.Time { return time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC) }

	// Ingest with anchor declarations
	doc, err := p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "spec.md",
		CorpusScope:  "workspace",
		Content:      []byte("The API key is secret123"),
		Anchors: []AnchorDeclaration{
			{
				Term:       "API key",
				Definition: "A unique identifier used for authentication",
				Class:      "technical",
				Context:    map[string]string{"scope": "API authentication mechanism"},
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, doc.Chunks, 1)

	// Pack the chunk
	packer := NewContextPacker(db)
	result, err := packer.Pack(context.Background(), []RankedCandidate{
		{ChunkID: doc.Chunks[0].ChunkID, VersionID: doc.Version.VersionID, DocID: doc.Document.DocID, FusedScore: 1},
	}, PackingOptions{MaxTokens: 500})
	require.NoError(t, err)
	require.Len(t, result.Blocks, 1)

	block, ok := result.Blocks[0].(core.StructuredContentBlock)
	require.True(t, ok)
	payload := block.Data.(map[string]any)

	// Verify anchor metadata is present
	anchors, ok := payload["anchors"].([]PackedAnchorMetadata)
	require.True(t, ok, "anchors field should be present in payload")
	require.Len(t, anchors, 1)
	require.Equal(t, "API key", anchors[0].Term)
	require.Equal(t, "A unique identifier used for authentication", anchors[0].Definition)
	require.Equal(t, "technical", anchors[0].Class)
	require.Equal(t, "fresh", anchors[0].Status)
}

func TestPackedBlocksWithNoAnchorsOmitsAnchorsField(t *testing.T) {
	db := openRetrievalTestDB(t)
	p := NewIngestionPipeline(db, fakeEmbedder{})
	p.now = func() time.Time { return time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC) }

	// Ingest without anchors
	doc, err := p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "plain.txt",
		CorpusScope:  "workspace",
		Content:      []byte("Some plain text without anchors"),
	})
	require.NoError(t, err)

	// Pack the chunk
	packer := NewContextPacker(db)
	result, err := packer.Pack(context.Background(), []RankedCandidate{
		{ChunkID: doc.Chunks[0].ChunkID, VersionID: doc.Version.VersionID, DocID: doc.Document.DocID, FusedScore: 1},
	}, PackingOptions{MaxTokens: 500})
	require.NoError(t, err)
	require.Len(t, result.Blocks, 1)

	block, ok := result.Blocks[0].(core.StructuredContentBlock)
	require.True(t, ok)
	payload := block.Data.(map[string]any)

	// Verify anchors field is not present or empty
	_, anchorsExists := payload["anchors"]
	require.False(t, anchorsExists, "anchors field should not be present when no anchors exist")
}

func TestPackedBlocksIncludeDerivationSummary(t *testing.T) {
	db := openRetrievalTestDB(t)
	p := NewIngestionPipeline(db, fakeEmbedder{})
	p.now = func() time.Time { return time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC) }

	doc, err := p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "doc.txt",
		CorpusScope:  "workspace",
		Content:      []byte("Content"),
	})
	require.NoError(t, err)

	// Pack the chunk
	packer := NewContextPacker(db)
	result, err := packer.Pack(context.Background(), []RankedCandidate{
		{ChunkID: doc.Chunks[0].ChunkID, VersionID: doc.Version.VersionID, DocID: doc.Document.DocID, FusedScore: 1},
	}, PackingOptions{MaxTokens: 500})
	require.NoError(t, err)

	block, ok := result.Blocks[0].(core.StructuredContentBlock)
	require.True(t, ok)
	payload := block.Data.(map[string]any)

	// Verify derivation summary is present
	derivation, ok := payload["derivation"]
	require.True(t, ok, "derivation field should be present")
	require.NotNil(t, derivation)
}

func TestFreshnessStatusReflectsDriftEvents(t *testing.T) {
	db := openRetrievalTestDB(t)
	p := NewIngestionPipeline(db, fakeEmbedder{})
	p.now = func() time.Time { return time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC) }

	// Ingest with anchor
	doc, err := p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "drift.md",
		CorpusScope:  "workspace",
		Content:      []byte("The term means one thing"),
		Anchors: []AnchorDeclaration{
			{
				Term:       "term",
				Definition: "Original meaning",
				Class:      "technical",
				Context:    map[string]string{"scope": "Example context"},
			},
		},
	})
	require.NoError(t, err)

	// Get the anchor ID to simulate drift event
	var anchorID string
	err = db.QueryRow(`SELECT anchor_id FROM retrieval_semantic_anchors LIMIT 1`).Scan(&anchorID)
	require.NoError(t, err)

	// Insert a drift event for this anchor
	eventID := "drift-evt-1"
	_, err = db.ExecContext(context.Background(), `
		INSERT INTO retrieval_anchor_events
		  (event_id, anchor_id, event_type, detail, similarity_score, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, eventID, anchorID, "drift_detected", "meaning changed", 0.4, time.Now().UTC().Format(time.RFC3339Nano))
	require.NoError(t, err)

	// Pack the chunk
	packer := NewContextPacker(db)
	result, err := packer.Pack(context.Background(), []RankedCandidate{
		{ChunkID: doc.Chunks[0].ChunkID, VersionID: doc.Version.VersionID, DocID: doc.Document.DocID, FusedScore: 1},
	}, PackingOptions{MaxTokens: 500})
	require.NoError(t, err)

	block, ok := result.Blocks[0].(core.StructuredContentBlock)
	require.True(t, ok)
	payload := block.Data.(map[string]any)

	// Verify drift status is reflected
	anchors, ok := payload["anchors"].([]PackedAnchorMetadata)
	require.True(t, ok)
	require.Len(t, anchors, 1)
	require.Equal(t, "drifted", anchors[0].Status)
}

func TestMultipleAnchorsPerChunkAreAllIncluded(t *testing.T) {
	db := openRetrievalTestDB(t)
	p := NewIngestionPipeline(db, fakeEmbedder{})
	p.now = func() time.Time { return time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC) }

	// Ingest with multiple anchor declarations
	doc, err := p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "multi.md",
		CorpusScope:  "workspace",
		Content:      []byte("JWT tokens and API keys are used for authentication"),
		Anchors: []AnchorDeclaration{
			{
				Term:       "JWT token",
				Definition: "JSON Web Token for stateless authentication",
				Class:      "technical",
				Context:    map[string]string{"scope": "Authentication"},
			},
			{
				Term:       "API key",
				Definition: "Secret string for API access",
				Class:      "technical",
				Context:    map[string]string{"scope": "API authentication"},
			},
			{
				Term:       "authentication",
				Definition: "Process of verifying identity",
				Class:      "technical",
				Context:    map[string]string{"scope": "Security concept"},
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, doc.Chunks, 1)

	// Pack the chunk
	packer := NewContextPacker(db)
	result, err := packer.Pack(context.Background(), []RankedCandidate{
		{ChunkID: doc.Chunks[0].ChunkID, VersionID: doc.Version.VersionID, DocID: doc.Document.DocID, FusedScore: 1},
	}, PackingOptions{MaxTokens: 500})
	require.NoError(t, err)

	block, ok := result.Blocks[0].(core.StructuredContentBlock)
	require.True(t, ok)
	payload := block.Data.(map[string]any)

	// Verify all anchors are included
	anchors, ok := payload["anchors"].([]PackedAnchorMetadata)
	require.True(t, ok)
	require.Len(t, anchors, 3)

	// Verify all terms are present
	terms := make(map[string]bool)
	for _, anchor := range anchors {
		terms[anchor.Term] = true
		require.NotEmpty(t, anchor.Definition)
		require.NotEmpty(t, anchor.AnchorID)
		require.Equal(t, "fresh", anchor.Status)
	}
	require.True(t, terms["JWT token"])
	require.True(t, terms["API key"])
	require.True(t, terms["authentication"])
}

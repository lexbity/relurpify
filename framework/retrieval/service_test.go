package retrieval

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/perfstats"
	"github.com/stretchr/testify/require"
)

type telemetryStub struct {
	events []core.Event
}

func (t *telemetryStub) Emit(event core.Event) {
	t.events = append(t.events, event)
}

func TestServiceRetrieveReturnsPackedBlocksAndEvents(t *testing.T) {
	db := openRetrievalTestDB(t)
	p := NewIngestionPipeline(db, fakeEmbedder{})
	p.now = func() time.Time { return time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC) }

	_, err := p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "guide.md",
		Content: []byte(`# Intro
hello

## Details
hello world
`),
	})
	require.NoError(t, err)

	telemetry := &telemetryStub{}
	service := NewService(db, fakeEmbedder{}, telemetry)
	service.now = func() time.Time { return time.Date(2026, 3, 11, 13, 0, 0, 0, time.UTC) }

	blocks, retrievalEvent, err := service.Retrieve(context.Background(), RetrievalQuery{
		Text:      "hello world",
		Scope:     "workspace",
		MaxTokens: 200,
		Limit:     5,
	})
	require.NoError(t, err)
	require.NotEmpty(t, blocks)
	require.NotEmpty(t, retrievalEvent.QueryID)
	require.Equal(t, "hello world", retrievalEvent.Query)
	require.Len(t, telemetry.events, 1)
}

func TestServiceRetrievePersistsAuditRecords(t *testing.T) {
	db := openRetrievalTestDB(t)
	p := NewIngestionPipeline(db, fakeEmbedder{})
	p.now = func() time.Time { return time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC) }

	_, err := p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "notes.txt",
		Content:      []byte("one\n\ntwo\n\nthree"),
	})
	require.NoError(t, err)

	service := NewService(db, fakeEmbedder{}, nil)
	service.now = func() time.Time { return time.Date(2026, 3, 11, 13, 0, 0, 0, time.UTC) }

	blocks, retrievalEvent, err := service.Retrieve(context.Background(), RetrievalQuery{
		Text:      "one two",
		Scope:     "workspace",
		MaxTokens: 2,
		Limit:     5,
	})
	require.NoError(t, err)
	require.NotEmpty(t, blocks)

	var queryText string
	var excludedJSON string
	err = db.QueryRow(`SELECT query_text, excluded_reasons_json FROM retrieval_events WHERE query_id = ?`, retrievalEvent.QueryID).Scan(&queryText, &excludedJSON)
	require.NoError(t, err)
	require.Equal(t, "one two", queryText)

	var excluded map[string]string
	require.NoError(t, json.Unmarshal([]byte(excludedJSON), &excluded))
	require.NotNil(t, excluded)

	var injectedJSON string
	var tokenBudget int
	var tokensUsed int
	err = db.QueryRow(`SELECT injected_chunks_json, token_budget, tokens_used FROM retrieval_packing_events WHERE query_id = ?`, retrievalEvent.QueryID).Scan(&injectedJSON, &tokenBudget, &tokensUsed)
	require.NoError(t, err)
	require.Equal(t, 2, tokenBudget)
	require.LessOrEqual(t, tokensUsed, tokenBudget)

	var injected []string
	require.NoError(t, json.Unmarshal([]byte(injectedJSON), &injected))
	require.NotEmpty(t, injected)
}

func TestServiceRetrieveUsesExactCacheWithoutPersistingSecondAuditRow(t *testing.T) {
	db := openRetrievalTestDB(t)
	p := NewIngestionPipeline(db, fakeEmbedder{})
	p.now = func() time.Time { return time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC) }

	_, err := p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "cache.txt",
		CorpusScope:  "workspace",
		Content:      []byte("cache me if you can"),
	})
	require.NoError(t, err)

	service := NewServiceWithOptions(db, fakeEmbedder{}, nil, ServiceOptions{
		Cache: CacheConfig{MaxEntries: 8, TTL: time.Hour},
	})
	service.now = func() time.Time { return time.Date(2026, 3, 11, 13, 0, 0, 0, time.UTC) }

	firstBlocks, firstEvent, err := service.Retrieve(context.Background(), RetrievalQuery{
		Text:      "cache me",
		Scope:     "workspace",
		MaxTokens: 100,
		Limit:     5,
	})
	require.NoError(t, err)
	require.Equal(t, "l3_main", firstEvent.CacheTier)

	service.now = func() time.Time { return time.Date(2026, 3, 11, 13, 5, 0, 0, time.UTC) }
	secondBlocks, secondEvent, err := service.Retrieve(context.Background(), RetrievalQuery{
		Text:      "cache me",
		Scope:     "workspace",
		MaxTokens: 100,
		Limit:     5,
	})
	require.NoError(t, err)
	require.Equal(t, firstBlocks, secondBlocks)
	require.Equal(t, "l1_exact", secondEvent.CacheTier)
	require.NotEqual(t, firstEvent.QueryID, secondEvent.QueryID)

	var eventRows int
	err = db.QueryRow(`SELECT COUNT(*) FROM retrieval_events`).Scan(&eventRows)
	require.NoError(t, err)
	require.Equal(t, 1, eventRows)
}

func TestServiceRetrieveAvoidsRepeatedSchemaChecksOnHotPath(t *testing.T) {
	db := openRetrievalTestDB(t)
	p := NewIngestionPipeline(db, fakeEmbedder{})
	_, err := p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "schema-once.txt",
		CorpusScope:  "workspace",
		Content:      []byte("alpha beta gamma"),
	})
	require.NoError(t, err)

	service := NewServiceWithOptions(db, fakeEmbedder{}, nil, ServiceOptions{
		Cache: CacheConfig{MaxEntries: 8, TTL: time.Hour},
	})
	perfstats.Reset()
	_, _, err = service.Retrieve(context.Background(), RetrievalQuery{
		Text:      "alpha",
		Scope:     "workspace",
		MaxTokens: 100,
		Limit:     5,
	})
	require.NoError(t, err)
	_, _, err = service.Retrieve(context.Background(), RetrievalQuery{
		Text:      "beta",
		Scope:     "workspace",
		MaxTokens: 100,
		Limit:     5,
	})
	require.NoError(t, err)

	stats := perfstats.Get()
	require.Equal(t, int64(0), stats.RetrievalSchemaCheckCount)
}

func TestServiceRetrieveInvalidatesExactCacheWhenCorpusRevisionChanges(t *testing.T) {
	db := openRetrievalTestDB(t)
	p := NewIngestionPipeline(db, fakeEmbedder{})
	p.now = func() time.Time { return time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC) }

	_, err := p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "cache-revision-a.txt",
		CorpusScope:  "workspace",
		Content:      []byte("alpha first"),
	})
	require.NoError(t, err)

	service := NewServiceWithOptions(db, fakeEmbedder{}, nil, ServiceOptions{
		Cache: CacheConfig{MaxEntries: 8, TTL: time.Hour},
	})
	service.now = func() time.Time { return time.Date(2026, 3, 11, 13, 0, 0, 0, time.UTC) }

	_, firstEvent, err := service.Retrieve(context.Background(), RetrievalQuery{
		Text:      "alpha",
		Scope:     "workspace",
		MaxTokens: 100,
		Limit:     5,
	})
	require.NoError(t, err)
	require.Equal(t, "l3_main", firstEvent.CacheTier)

	p.now = func() time.Time { return time.Date(2026, 3, 11, 13, 1, 0, 0, time.UTC) }
	_, err = p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "cache-revision-b.txt",
		CorpusScope:  "workspace",
		Content:      []byte("alpha second"),
	})
	require.NoError(t, err)

	service.now = func() time.Time { return time.Date(2026, 3, 11, 13, 2, 0, 0, time.UTC) }
	_, secondEvent, err := service.Retrieve(context.Background(), RetrievalQuery{
		Text:      "alpha",
		Scope:     "workspace",
		MaxTokens: 100,
		Limit:     5,
	})
	require.NoError(t, err)
	require.Equal(t, "l3_main", secondEvent.CacheTier)
	require.NotEqual(t, firstEvent.QueryID, secondEvent.QueryID)

	var eventRows int
	err = db.QueryRow(`SELECT COUNT(*) FROM retrieval_events`).Scan(&eventRows)
	require.NoError(t, err)
	require.Equal(t, 2, eventRows)
}

func TestServiceRetrieveCanUseHotStoreNarrowing(t *testing.T) {
	db := openRetrievalTestDB(t)
	p := NewIngestionPipeline(db, fakeEmbedder{})
	p.now = func() time.Time { return time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC) }

	_, err := p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "hot-a.txt",
		CorpusScope:  "workspace",
		Content:      []byte("alpha signal"),
	})
	require.NoError(t, err)
	_, err = p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "hot-b.txt",
		CorpusScope:  "workspace",
		Content:      []byte("beta unrelated"),
	})
	require.NoError(t, err)

	service := NewServiceWithOptions(db, fakeEmbedder{}, nil, ServiceOptions{
		Cache:     CacheConfig{MaxEntries: 8, TTL: time.Hour},
		HotWindow: 24 * time.Hour,
		HotLimit:  8,
	})
	service.now = func() time.Time { return time.Date(2026, 3, 11, 13, 0, 0, 0, time.UTC) }

	_, firstEvent, err := service.Retrieve(context.Background(), RetrievalQuery{
		Text:      "alpha",
		Scope:     "workspace",
		MaxTokens: 100,
		Limit:     5,
	})
	require.NoError(t, err)
	require.Equal(t, "l3_main", firstEvent.CacheTier)

	service.now = func() time.Time { return time.Date(2026, 3, 11, 13, 10, 0, 0, time.UTC) }
	blocks, secondEvent, err := service.Retrieve(context.Background(), RetrievalQuery{
		Text:      "signal",
		Scope:     "workspace",
		MaxTokens: 100,
		Limit:     1,
	})
	require.NoError(t, err)
	require.NotEmpty(t, blocks)
	require.Equal(t, "l2_hot", secondEvent.CacheTier)
}

func TestServiceRetrieveFallsBackToMainCorpusWhenHotSubsetWouldHideBetterResult(t *testing.T) {
	db := openRetrievalTestDB(t)
	p := NewIngestionPipeline(db, nil)
	p.now = func() time.Time { return time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC) }

	hotDoc, err := p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "hot.txt",
		CorpusScope:  "workspace",
		Content:      []byte("alpha"),
	})
	require.NoError(t, err)
	coldDoc, err := p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "cold.txt",
		CorpusScope:  "workspace",
		Content:      []byte("alpha alpha alpha"),
	})
	require.NoError(t, err)

	_, err = db.Exec(`INSERT INTO retrieval_packing_events
		(query_id, injected_chunks_json, dropped_chunks_json, token_budget, tokens_used, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		"seed-hot",
		`["`+hotDoc.Chunks[0].ChunkID+`"]`,
		`{}`,
		100,
		1,
		time.Date(2026, 3, 11, 12, 30, 0, 0, time.UTC).Format(time.RFC3339Nano),
	)
	require.NoError(t, err)

	service := NewServiceWithOptions(db, nil, nil, ServiceOptions{
		Cache:     CacheConfig{MaxEntries: 8, TTL: time.Hour},
		HotWindow: 24 * time.Hour,
		HotLimit:  8,
	})
	service.now = func() time.Time { return time.Date(2026, 3, 11, 13, 0, 0, 0, time.UTC) }

	blocks, event, err := service.Retrieve(context.Background(), RetrievalQuery{
		Text:      "alpha",
		Scope:     "workspace",
		MaxTokens: 100,
		Limit:     1,
	})
	require.NoError(t, err)
	require.NotEmpty(t, blocks)
	require.Equal(t, "l3_main", event.CacheTier)

	block := blocks[0].(core.StructuredContentBlock)
	payload := block.Data.(map[string]any)
	citations := payload["citations"].([]PackedCitation)
	require.Len(t, citations, 1)
	require.Equal(t, coldDoc.Chunks[0].ChunkID, citations[0].ChunkID)
}

func TestServiceRetrievePersistsRetrievalStageExclusions(t *testing.T) {
	db := openRetrievalTestDB(t)
	p := NewIngestionPipeline(db, nil)
	p.now = func() time.Time { return time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC) }

	first, err := p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "first.txt",
		Content:      []byte("alpha alpha alpha"),
	})
	require.NoError(t, err)
	second, err := p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "second.txt",
		Content:      []byte("alpha alpha"),
	})
	require.NoError(t, err)
	third, err := p.Ingest(context.Background(), IngestRequest{
		CanonicalURI: "third.txt",
		Content:      []byte("irrelevant"),
	})
	require.NoError(t, err)

	service := NewService(db, nil, nil)
	service.now = func() time.Time { return time.Date(2026, 3, 11, 13, 0, 0, 0, time.UTC) }

	_, retrievalEvent, err := service.Retrieve(context.Background(), RetrievalQuery{
		Text:      "alpha",
		Scope:     "workspace",
		MaxTokens: 100,
		Limit:     1,
	})
	require.NoError(t, err)

	var excludedJSON string
	err = db.QueryRow(`SELECT excluded_reasons_json FROM retrieval_events WHERE query_id = ?`, retrievalEvent.QueryID).Scan(&excludedJSON)
	require.NoError(t, err)

	var excluded map[string]string
	require.NoError(t, json.Unmarshal([]byte(excludedJSON), &excluded))
	require.Contains(t, excluded[second.Chunks[0].ChunkID], "fusion:rank_cutoff")
	require.NotContains(t, excluded, first.Chunks[0].ChunkID)
	require.Contains(t, excluded[third.Chunks[0].ChunkID], "retrieval:no_index_match")
}

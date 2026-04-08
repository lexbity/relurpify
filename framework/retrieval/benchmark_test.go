package retrieval

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	_ "github.com/mattn/go-sqlite3"
)

func openRetrievalBenchmarkDB(b *testing.B) *sql.DB {
	b.Helper()
	db, err := sql.Open("sqlite3", "file::memory:?cache=shared")
	if err != nil {
		b.Fatal(err)
	}
	if err := EnsureSchema(context.Background(), db); err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = db.Close() })
	return db
}

func seedRetrievalBenchmarkCorpus(b *testing.B, db *sql.DB, docs int) {
	b.Helper()
	p := NewIngestionPipeline(db, fakeEmbedder{})
	p.now = func() time.Time { return time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC) }
	for i := 0; i < docs; i++ {
		if _, err := p.Ingest(context.Background(), IngestRequest{
			CanonicalURI: fmt.Sprintf("bench-%03d.txt", i),
			CorpusScope:  "workspace",
			Content:      []byte(fmt.Sprintf("alpha topic %03d\n\nbeta topic %03d\n\ngamma topic %03d", i, i, i)),
			PolicyTags:   []string{"bench"},
		}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRetrievalServiceRetrieveWarmCache(b *testing.B) {
	db := openRetrievalBenchmarkDB(b)
	seedRetrievalBenchmarkCorpus(b, db, 64)

	service := NewServiceWithOptions(db, fakeEmbedder{}, nil, ServiceOptions{
		Cache: CacheConfig{MaxEntries: 32, TTL: time.Hour},
	})
	service.now = func() time.Time { return time.Date(2026, 3, 11, 13, 0, 0, 0, time.UTC) }
	query := RetrievalQuery{Text: "alpha topic", Scope: "workspace", MaxTokens: 512, Limit: 8}
	if _, _, err := service.Retrieve(context.Background(), query); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := service.Retrieve(context.Background(), query); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRetrievalServiceRetrieveColdCache(b *testing.B) {
	db := openRetrievalBenchmarkDB(b)
	seedRetrievalBenchmarkCorpus(b, db, 64)

	service := NewService(db, fakeEmbedder{}, nil)
	service.cache = nil
	service.now = func() time.Time { return time.Date(2026, 3, 11, 13, 0, 0, 0, time.UTC) }

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := service.Retrieve(context.Background(), RetrievalQuery{
			Text:      fmt.Sprintf("alpha topic %03d", i%64),
			Scope:     "workspace",
			MaxTokens: 512,
			Limit:     8,
		}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMetadataPrefilter(b *testing.B) {
	db := openRetrievalBenchmarkDB(b)
	seedRetrievalBenchmarkCorpus(b, db, 128)
	filter := NewMetadataPrefilter(db)
	query := RetrievalQuery{Scope: "workspace", PolicyTags: []string{"bench"}}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := filter.Prefilter(context.Background(), query); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkContextPackerPack(b *testing.B) {
	db := openRetrievalBenchmarkDB(b)
	seedRetrievalBenchmarkCorpus(b, db, 64)
	retriever := NewRetriever(db, fakeEmbedder{})
	result, err := retriever.RetrieveCandidates(context.Background(), RetrievalQuery{
		Text:  "alpha topic",
		Scope: "workspace",
		Limit: 16,
	})
	if err != nil {
		b.Fatal(err)
	}
	packer := NewContextPacker(db)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := packer.Pack(context.Background(), result.Fused, PackingOptions{
			MaxTokens: 512,
			MaxChunks: 8,
		}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRetrievalIngestionRevisionUpdate(b *testing.B) {
	db := openRetrievalBenchmarkDB(b)
	p := NewIngestionPipeline(db, fakeEmbedder{})
	p.now = func() time.Time { return time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC) }

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := p.Ingest(context.Background(), IngestRequest{
			CanonicalURI: "bench-revision.txt",
			CorpusScope:  "workspace",
			Content:      []byte(fmt.Sprintf("alpha revision %d", i)),
		}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMixedEvidencePayloadFromEnvelopes(b *testing.B) {
	results := make([]core.MemoryRecordEnvelope, 0, 24)
	for i := 0; i < 24; i++ {
		results = append(results, core.MemoryRecordEnvelope{
			Key:         fmt.Sprintf("doc:%03d", i),
			RecordID:    fmt.Sprintf("doc:%03d", i),
			MemoryClass: core.MemoryClassDeclarative,
			Scope:       "project",
			Summary:     fmt.Sprintf("retrieved memory summary %03d", i),
			Text:        fmt.Sprintf("retrieved memory text %03d", i),
			Source:      "retrieval",
			Kind:        "document",
			Score:       1.0 - float64(i)/1000.0,
			Reference: map[string]any{
				"kind": string(core.ContextReferenceRetrievalEvidence),
				"uri":  fmt.Sprintf("memory://runtime/doc:%03d", i),
			},
			Citations: []map[string]any{{
				"doc_id":        fmt.Sprintf("doc-%03d", i),
				"chunk_id":      fmt.Sprintf("chunk-%03d", i),
				"canonical_uri": fmt.Sprintf("file:///tmp/doc-%03d.txt", i),
			}},
		})
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		payload := MixedEvidencePayloadFromEnvelopes("find memory", "project", results)
		if payload == nil {
			b.Fatal("expected payload")
		}
	}
}

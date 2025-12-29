package framework_test

import (
	"context"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/search"
	"strings"
	"testing"
)

type stubCodeIndex struct {
	chunks []*core.CodeChunk
}

func (s *stubCodeIndex) GetFileMetadata(string) (*core.FileMetadata, bool)      { return nil, false }
func (s *stubCodeIndex) ListFiles() []string                                    { return nil }
func (s *stubCodeIndex) GetSymbolsByName(string) ([]core.SymbolLocation, error) { return nil, nil }
func (s *stubCodeIndex) GetSymbolDefinition(string) (*core.SymbolLocation, error) {
	return nil, nil
}
func (s *stubCodeIndex) GetSymbolReferences(string) ([]core.SymbolLocation, error) {
	return nil, nil
}
func (s *stubCodeIndex) GetFileDependencies(string) []string         { return nil }
func (s *stubCodeIndex) GetDependents(string) []string               { return nil }
func (s *stubCodeIndex) GetChunksForFile(string) []*core.CodeChunk   { return nil }
func (s *stubCodeIndex) GetChunkByID(string) (*core.CodeChunk, bool) { return nil, false }
func (s *stubCodeIndex) FindChunksByName(string) []*core.CodeChunk   { return nil }
func (s *stubCodeIndex) FindChunksByFileAndRange(string, int, int) []*core.CodeChunk {
	return nil
}
func (s *stubCodeIndex) SearchChunks(string, int) []*core.CodeChunk { return s.chunks }
func (s *stubCodeIndex) BuildIndex(context.Context) error           { return nil }
func (s *stubCodeIndex) UpdateIncremental([]string) error           { return nil }
func (s *stubCodeIndex) Save() error                                { return nil }
func (s *stubCodeIndex) Version() string                            { return "" }

type stubSemanticStore struct{}

func (*stubSemanticStore) Query(context.Context, string, int) ([]search.VectorMatch, error) {
	return nil, nil
}

func TestSearchWithBudgetUsesSummaryFallback(t *testing.T) {
	index := &stubCodeIndex{
		chunks: []*core.CodeChunk{
			{
				ID:         "chunk-1",
				File:       "service.go",
				Preview:    strings.Repeat("very long snippet ", 50),
				Summary:    "short summary of code",
				TokenCount: 1000,
				StartLine:  1,
				EndLine:    10,
			},
		},
	}
	engine := search.NewSearchEngine(&stubSemanticStore{}, index)

	results, err := engine.SearchWithBudget(search.SearchQuery{
		Text:       "service",
		Mode:       search.SearchKeyword,
		MaxResults: 1,
	}, 50)
	if err != nil {
		t.Fatalf("SearchWithBudget returned error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Snippet != "" {
		t.Fatalf("expected snippet to be trimmed when budget exceeded")
	}
	if results[0].Summary == "" {
		t.Fatalf("expected summary to remain")
	}
}

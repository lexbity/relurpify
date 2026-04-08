package search

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

type stubSemanticStore struct {
	results []VectorMatch
	err     error
}

func (s stubSemanticStore) Query(_ context.Context, _ string, limit int) ([]VectorMatch, error) {
	if s.err != nil {
		return nil, s.err
	}
	if limit > 0 && len(s.results) > limit {
		return append([]VectorMatch(nil), s.results[:limit]...), nil
	}
	return append([]VectorMatch(nil), s.results...), nil
}

type stubCodeIndex struct {
	searchChunks   []*core.CodeChunk
	symbols        []core.SymbolLocation
	incrementalErr error
	saveErr        error
	searchErr      error
	symbolErr      error
	updateFiles    []string
	saved          bool
}

func (s *stubCodeIndex) GetFileMetadata(path string) (*core.FileMetadata, bool) { return nil, false }
func (s *stubCodeIndex) ListFiles() []string                                    { return nil }
func (s *stubCodeIndex) GetSymbolsByName(name string) ([]core.SymbolLocation, error) {
	return s.symbols, s.symbolErr
}
func (s *stubCodeIndex) GetSymbolDefinition(name string) (*core.SymbolLocation, error) {
	return nil, nil
}
func (s *stubCodeIndex) GetSymbolReferences(name string) ([]core.SymbolLocation, error) {
	return nil, nil
}
func (s *stubCodeIndex) GetFileDependencies(path string) []string { return nil }
func (s *stubCodeIndex) GetDependents(path string) []string       { return nil }
func (s *stubCodeIndex) GetChunksForFile(path string) []*core.CodeChunk {
	return nil
}
func (s *stubCodeIndex) GetChunkByID(id string) (*core.CodeChunk, bool) { return nil, false }
func (s *stubCodeIndex) FindChunksByName(name string) []*core.CodeChunk { return nil }
func (s *stubCodeIndex) FindChunksByFileAndRange(path string, start, end int) []*core.CodeChunk {
	return nil
}
func (s *stubCodeIndex) SearchChunks(query string, limit int) []*core.CodeChunk {
	if s.searchErr != nil {
		return nil
	}
	if limit > 0 && len(s.searchChunks) > limit {
		return s.searchChunks[:limit]
	}
	return s.searchChunks
}

func (s *stubCodeIndex) UpdateIncremental(files []string) error {
	s.updateFiles = append([]string(nil), files...)
	return s.incrementalErr
}

func (s *stubCodeIndex) Save() error {
	s.saved = true
	return s.saveErr
}

func TestMatchGlobAndRegexHelpers(t *testing.T) {
	require.False(t, MatchGlob("", "file.go"))
	require.True(t, MatchGlob(permissionMatchAll, "anything/else"))
	require.True(t, MatchGlob("**/*.go", "mathutil.go"))
	require.True(t, MatchGlob("**/*.go", "nested/mathutil.go"))
	require.False(t, MatchGlob("*.go", "nested/mathutil.go"))
	require.Equal(t, "^foo/(?:.*/)?bar$", globToRegexPublic("foo/**/bar"))
}

func TestSearchEngineModesAndBudget(t *testing.T) {
	codeIndex := &stubCodeIndex{
		searchChunks: []*core.CodeChunk{
			{ID: "chunk-1", File: "a.go", Preview: "alpha", Summary: "sum-a", TokenCount: 4, StartLine: 1, EndLine: 3},
			{ID: "chunk-2", File: "b.go", Preview: "beta", Summary: "sum-b", TokenCount: 5, StartLine: 5, EndLine: 8},
		},
		symbols: []core.SymbolLocation{
			{File: "symbol.go", Line: 9, Symbol: &core.Symbol{Name: "Example", Signature: "func Example()", DocString: "docs"}},
		},
	}
	engine := NewSearchEngine(stubSemanticStore{
		results: []VectorMatch{
			{Content: "semantic-one", Metadata: map[string]any{"path": "semantic.go"}, Score: 0.8},
			{Content: "semantic-two", Metadata: map[string]any{"path": "semantic2.go"}, Score: 0.5},
		},
	}, codeIndex)

	semantic, err := engine.Search(SearchQuery{Mode: SearchSemantic, Text: "alpha", MaxResults: 1})
	require.NoError(t, err)
	require.Len(t, semantic, 1)
	require.Equal(t, "semantic.go", semantic[0].File)

	keyword, err := engine.Search(SearchQuery{Mode: SearchKeyword, Text: "Alpha", MaxResults: 1})
	require.NoError(t, err)
	require.Len(t, keyword, 1)
	require.Equal(t, "keyword", keyword[0].RelevanceType)

	symbols, err := engine.Search(SearchQuery{Mode: SearchSymbolOnly, Text: "Example"})
	require.NoError(t, err)
	require.Len(t, symbols, 1)
	require.Equal(t, "symbol", symbols[0].RelevanceType)

	hybrid, err := engine.Search(SearchQuery{Mode: SearchHybrid, Text: "alpha", MaxResults: 1})
	require.NoError(t, err)
	require.Len(t, hybrid, 1)

	pruned, err := engine.SearchWithBudget(SearchQuery{Mode: SearchKeyword, Text: "alpha", MaxResults: 2}, 5)
	require.NoError(t, err)
	require.Len(t, pruned, 1)

	empty, err := (&SearchEngine{}).SearchWithBudget(SearchQuery{Mode: SearchKeyword, Text: "alpha"}, 0)
	require.NoError(t, err)
	require.Nil(t, empty)
}

func TestSearchEngineRefreshFiles(t *testing.T) {
	index := &stubCodeIndex{}
	engine := NewSearchEngine(nil, index)
	require.NoError(t, engine.RefreshFiles([]string{"a.go"}))
	require.Equal(t, []string{"a.go"}, index.updateFiles)
	require.True(t, index.saved)

	index.incrementalErr = errors.New("boom")
	require.Error(t, engine.RefreshFiles([]string{"b.go"}))

	require.NoError(t, (&SearchEngine{}).RefreshFiles(nil))
}

func TestMergeResults(t *testing.T) {
	a := []SearchResult{{File: "a"}}
	b := []SearchResult{{File: "b"}, {File: "c"}}
	require.True(t, reflect.DeepEqual(mergeResults(a, b, 0), append(a, b...)))
	require.Len(t, mergeResults(a, b, 2), 2)
}

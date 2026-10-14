package search

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeSemanticStore is a hermetic SemanticStore: it records the query it
// received and returns canned matches or an error.
type fakeSemanticStore struct {
	gotQuery  string
	gotLimit  int
	matches   []VectorMatch
	err       error
	queryErr  error
	queryText string
}

func (f *fakeSemanticStore) Query(_ context.Context, query string, limit int) ([]VectorMatch, error) {
	f.gotQuery = query
	f.gotLimit = limit
	if f.queryErr != nil {
		return nil, f.queryErr
	}
	return f.matches, f.err
}

// fakeCodeIndex is a hermetic CodeIndex returning canned chunks and symbols.
type fakeCodeIndex struct {
	chunks      []*CodeChunk
	symbols     []SymbolLocation
	symbolsErr  error
	searchQuery string
	searchLimit int
}

func (f *fakeCodeIndex) GetFileMetadata(string) (any, bool) { return nil, false }
func (f *fakeCodeIndex) ListFiles() []string                { return nil }
func (f *fakeCodeIndex) GetSymbolDefinition(string) (*SymbolLocation, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeCodeIndex) GetSymbolReferences(string) ([]SymbolLocation, error) { return nil, nil }
func (f *fakeCodeIndex) GetFileDependencies(string) []string                  { return nil }
func (f *fakeCodeIndex) GetDependents(string) []string                        { return nil }
func (f *fakeCodeIndex) GetChunksForFile(string) []*CodeChunk                 { return nil }
func (f *fakeCodeIndex) GetChunkByID(string) (*CodeChunk, bool)               { return nil, false }
func (f *fakeCodeIndex) FindChunksByName(string) []*CodeChunk                 { return nil }
func (f *fakeCodeIndex) FindChunksByFileAndRange(string, int, int) []*CodeChunk {
	return nil
}
func (f *fakeCodeIndex) SearchChunks(query string, limit int) []*CodeChunk {
	f.searchQuery = query
	f.searchLimit = limit
	return f.chunks
}
func (f *fakeCodeIndex) GetSymbolsByName(string) ([]SymbolLocation, error) {
	return f.symbols, f.symbolsErr
}

// countingIndex additionally implements the optional incremental and
// persistence capability interfaces probed by RefreshFiles.
type countingIndex struct {
	fakeCodeIndex
	updatedFiles [][]string
	updateErr    error
	saves        int
	saveErr      error
}

func (c *countingIndex) UpdateIncremental(files []string) error {
	c.updatedFiles = append(c.updatedFiles, files)
	return c.updateErr
}

func (c *countingIndex) Save() error {
	c.saves++
	return c.saveErr
}

func semanticMatches() []VectorMatch {
	return []VectorMatch{
		{ID: "m1", Content: "first snippet", Score: 0.9, Metadata: map[string]any{"path": "a/b.go"}},
		{ID: "m2", Content: "second snippet", Score: 0.5, Metadata: map[string]any{"path": 42}},
		{ID: "m3", Content: "third snippet", Score: 0.1, Metadata: nil},
	}
}

func TestSearchSemanticModeMapsVectorMatches(t *testing.T) {
	store := &fakeSemanticStore{matches: semanticMatches()}
	engine := NewSearchEngine(store, nil)

	results, err := engine.Search(context.Background(), SearchQuery{
		Text:       "find snippets",
		Mode:       SearchSemantic,
		MaxResults: 3,
	})
	require.NoError(t, err)
	require.Len(t, results, 3)
	require.Equal(t, "find snippets", store.gotQuery)
	require.Equal(t, 3, store.gotLimit)

	first := results[0]
	require.Equal(t, "a/b.go", first.File)
	require.Equal(t, "semantic-1", first.ChunkID)
	require.Equal(t, "first snippet", first.Snippet)
	require.Equal(t, 0.9, first.Score)
	require.Equal(t, "semantic", first.RelevanceType)

	// Non-string "path" metadata yields an empty file, but the match is kept.
	require.Empty(t, results[1].File)
	require.Equal(t, "semantic-2", results[1].ChunkID)
	// Missing metadata yields an empty file too.
	require.Empty(t, results[2].File)
	require.Equal(t, "semantic-3", results[2].ChunkID)
}

func TestSearchSemanticDefaultsMaxResultsToFive(t *testing.T) {
	store := &fakeSemanticStore{matches: semanticMatches()}
	engine := NewSearchEngine(store, nil)

	_, err := engine.Search(context.Background(), SearchQuery{Text: "q", Mode: SearchSemantic})
	require.NoError(t, err)
	require.Equal(t, 5, store.gotLimit)
}

func TestSearchSemanticWithoutVectorStoreReturnsNoResults(t *testing.T) {
	engine := NewSearchEngine(nil, nil)
	results, err := engine.Search(context.Background(), SearchQuery{Text: "q", Mode: SearchSemantic})
	require.NoError(t, err)
	require.Empty(t, results)
}

func TestSearchSemanticPropagatesStoreError(t *testing.T) {
	store := &fakeSemanticStore{queryErr: errors.New("vector store down")}
	engine := NewSearchEngine(store, nil)

	results, err := engine.Search(context.Background(), SearchQuery{Text: "q", Mode: SearchSemantic})
	require.Nil(t, results)
	require.EqualError(t, err, "vector store down")
}

func keywordChunks() []*CodeChunk {
	return []*CodeChunk{
		{ID: "c1", File: "x.go", Kind: ChunkFunction, Name: "Alpha", StartLine: 1, EndLine: 10,
			Summary: "alpha summary", TokenCount: 7, Preview: "func Alpha()"},
		{ID: "c2", File: "y.go", Kind: ChunkClass, Name: "Beta", StartLine: 20, EndLine: 30,
			Summary: "beta summary", TokenCount: 3, Preview: "class Beta"},
	}
}

func TestSearchKeywordModeMapsChunks(t *testing.T) {
	index := &fakeCodeIndex{chunks: keywordChunks()}
	engine := NewSearchEngine(nil, index)

	results, err := engine.Search(context.Background(), SearchQuery{
		Text:       "ALPHA",
		Mode:       SearchKeyword,
		MaxResults: 2,
	})
	require.NoError(t, err)
	require.Len(t, results, 2)
	// The query is lowercased before hitting the index.
	require.Equal(t, "alpha", index.searchQuery)
	require.Equal(t, 2, index.searchLimit)

	first := results[0]
	require.Equal(t, "x.go", first.File)
	require.Equal(t, "c1", first.ChunkID)
	require.Equal(t, "func Alpha()", first.Snippet)
	require.Equal(t, "alpha summary", first.Summary)
	require.Equal(t, float64(7), first.Score)
	require.Equal(t, "keyword", first.RelevanceType)
	require.Equal(t, 1, first.StartLine)
	require.Equal(t, 10, first.EndLine)
}

func TestSearchKeywordDefaultsMaxResultsToFive(t *testing.T) {
	index := &fakeCodeIndex{chunks: keywordChunks()}
	engine := NewSearchEngine(nil, index)

	_, err := engine.Search(context.Background(), SearchQuery{Text: "q", Mode: SearchKeyword})
	require.NoError(t, err)
	require.Equal(t, 5, index.searchLimit)
}

func TestSearchKeywordSkipsWhenIndexMissingOrQueryEmpty(t *testing.T) {
	results, err := NewSearchEngine(nil, nil).Search(context.Background(),
		SearchQuery{Text: "q", Mode: SearchKeyword})
	require.NoError(t, err)
	require.Empty(t, results)

	results, err = NewSearchEngine(nil, &fakeCodeIndex{}).Search(context.Background(),
		SearchQuery{Mode: SearchKeyword})
	require.NoError(t, err)
	require.Empty(t, results)
}

func TestSearchSymbolOnlyMapsSymbolLocations(t *testing.T) {
	index := &fakeCodeIndex{symbols: []SymbolLocation{
		{File: "s.go", Line: 12, Column: 3, Symbol: &Symbol{
			Name: "Widget", Kind: SymbolClass, Signature: "type Widget struct", DocString: "A widget.",
		}},
	}}
	engine := NewSearchEngine(nil, index)

	results, err := engine.Search(context.Background(), SearchQuery{Text: "Widget", Mode: SearchSymbolOnly})
	require.NoError(t, err)
	require.Len(t, results, 1)
	result := results[0]
	require.Equal(t, "s.go", result.File)
	require.Equal(t, "Widget", result.ChunkID)
	require.Equal(t, "type Widget struct", result.Snippet)
	require.Equal(t, "A widget.", result.Summary)
	require.Equal(t, 1.0, result.Score)
	require.Equal(t, "symbol", result.RelevanceType)
	require.Equal(t, 12, result.StartLine)
	require.Equal(t, 12, result.EndLine)
}

func TestSearchSymbolOnlyPropagatesIndexError(t *testing.T) {
	index := &fakeCodeIndex{symbolsErr: errors.New("symbol index corrupt")}
	engine := NewSearchEngine(nil, index)

	results, err := engine.Search(context.Background(), SearchQuery{Text: "Widget", Mode: SearchSymbolOnly})
	require.Nil(t, results)
	require.EqualError(t, err, "symbol index corrupt")
}

func TestSearchSymbolOnlySkipsWhenIndexMissingOrQueryEmpty(t *testing.T) {
	results, err := NewSearchEngine(nil, nil).Search(context.Background(),
		SearchQuery{Text: "q", Mode: SearchSymbolOnly})
	require.NoError(t, err)
	require.Empty(t, results)

	results, err = NewSearchEngine(nil, &fakeCodeIndex{}).Search(context.Background(),
		SearchQuery{Mode: SearchSymbolOnly})
	require.NoError(t, err)
	require.Empty(t, results)
}

func TestSearchHybridMergesSemanticAndKeywordResults(t *testing.T) {
	store := &fakeSemanticStore{matches: []VectorMatch{
		{ID: "m1", Content: "semantic hit", Score: 0.8, Metadata: map[string]any{"path": "a.go"}},
	}}
	index := &fakeCodeIndex{chunks: keywordChunks()[:1]}
	engine := NewSearchEngine(store, index)

	results, err := engine.Search(context.Background(), SearchQuery{
		Text:       "query",
		Mode:       SearchHybrid,
		MaxResults: 5,
	})
	require.NoError(t, err)
	require.Len(t, results, 2)
	require.Equal(t, "semantic", results[0].RelevanceType)
	require.Equal(t, "keyword", results[1].RelevanceType)
}

func TestSearchHybridTruncatesToMaxResults(t *testing.T) {
	store := &fakeSemanticStore{matches: []VectorMatch{
		{ID: "m1", Content: "s1", Score: 1, Metadata: map[string]any{"path": "a.go"}},
		{ID: "m2", Content: "s2", Score: 0.9},
	}}
	index := &fakeCodeIndex{chunks: keywordChunks()}
	engine := NewSearchEngine(store, index)

	results, err := engine.Search(context.Background(), SearchQuery{
		Text:       "query",
		Mode:       SearchHybrid,
		MaxResults: 2,
	})
	require.NoError(t, err)
	require.Len(t, results, 2)
}

func TestSearchHybridPropagatesSemanticErrorBeforeKeyword(t *testing.T) {
	store := &fakeSemanticStore{queryErr: errors.New("semantic failed")}
	index := &fakeCodeIndex{chunks: keywordChunks()}
	engine := NewSearchEngine(store, index)

	results, err := engine.Search(context.Background(), SearchQuery{Text: "q", Mode: SearchHybrid})
	require.Nil(t, results)
	require.EqualError(t, err, "semantic failed")
}

func TestRefreshFilesNoOpsWithoutWork(t *testing.T) {
	var nilEngine *SearchEngine
	require.NoError(t, nilEngine.RefreshFiles([]string{"a.go"}))

	engine := NewSearchEngine(nil, nil)
	require.NoError(t, engine.RefreshFiles(nil))
	require.NoError(t, engine.RefreshFiles([]string{}))

	// An index without incremental support is silently skipped.
	plain := &fakeCodeIndex{}
	engine = NewSearchEngine(nil, plain)
	require.NoError(t, engine.RefreshFiles([]string{"a.go"}))
	require.Empty(t, plain.searchQuery)
}

func TestRefreshFilesUpdatesAndPersistsIncrementalIndex(t *testing.T) {
	index := &countingIndex{}
	engine := NewSearchEngine(nil, index)

	require.NoError(t, engine.RefreshFiles([]string{"a.go", "b.go"}))
	require.Equal(t, [][]string{{"a.go", "b.go"}}, index.updatedFiles)
	require.Equal(t, 1, index.saves)
}

func TestRefreshFilesPropagatesUpdateError(t *testing.T) {
	index := &countingIndex{updateErr: errors.New("update failed")}
	engine := NewSearchEngine(nil, index)

	err := engine.RefreshFiles([]string{"a.go"})
	require.EqualError(t, err, "update failed")
	require.Equal(t, 0, index.saves, "Save must not run when the incremental update fails")
}

func TestRefreshFilesPropagatesSaveError(t *testing.T) {
	index := &countingIndex{saveErr: errors.New("save failed")}
	engine := NewSearchEngine(nil, index)

	err := engine.RefreshFiles([]string{"a.go"})
	require.EqualError(t, err, "save failed")
	require.Len(t, index.updatedFiles, 1, "incremental update ran before the save failed")
}

func TestSearchWithBudgetReturnsAllWhenBudgetMissingOrNegative(t *testing.T) {
	store := &fakeSemanticStore{matches: []VectorMatch{
		{ID: "m1", Content: "short", Score: 1},
	}}
	engine := NewSearchEngine(store, nil)

	for _, budget := range []int{0, -1} {
		results, err := engine.SearchWithBudget(context.Background(),
			SearchQuery{Text: "q", Mode: SearchSemantic}, budget)
		require.NoError(t, err)
		require.Len(t, results, 1)
	}
}

func TestSearchWithBudgetPrunesByEstimatedTokens(t *testing.T) {
	// estimateTokens counts ~4 chars per token: 40-char snippet = 10 tokens,
	// 20-char summary = 5 tokens.
	mk := func(content string) []VectorMatch {
		return []VectorMatch{{ID: "m1", Content: content, Score: 1, Metadata: map[string]any{"path": "a.go"}}}
	}

	t.Run("fits entirely under budget", func(t *testing.T) {
		engine := NewSearchEngine(&fakeSemanticStore{matches: mk(makeLongString(40, 'x'))}, nil)
		results, err := engine.SearchWithBudget(context.Background(),
			SearchQuery{Text: "q", Mode: SearchSemantic}, 10)
		require.NoError(t, err)
		require.Len(t, results, 1)
		require.Equal(t, makeLongString(40, 'x'), results[0].Snippet)
	})

	t.Run("drops snippet but keeps summary within remaining budget", func(t *testing.T) {
		// Semantic results carry no summary, so exercise the summary branch
		// through keyword results which do.
		index := &fakeCodeIndex{chunks: []*CodeChunk{
			{ID: "c1", File: "x.go", TokenCount: 1, Preview: makeLongString(40, 'p'), Summary: makeLongString(12, 's')},
		}}
		engine := NewSearchEngine(nil, index)
		// snippet cost 10, summary cost 3: budget 3 keeps only the summary.
		results, err := engine.SearchWithBudget(context.Background(),
			SearchQuery{Text: "q", Mode: SearchKeyword}, 3)
		require.NoError(t, err)
		require.Len(t, results, 1)
		require.Empty(t, results[0].Snippet)
		require.Equal(t, makeLongString(12, 's'), results[0].Summary)
	})

	t.Run("skips results that cannot fit at all", func(t *testing.T) {
		index := &fakeCodeIndex{chunks: []*CodeChunk{
			// 10-token preview, summary too short to cost a token: the
			// summary-only branch does not apply (cost 0), so the whole
			// result is skipped.
			{ID: "c1", File: "x.go", TokenCount: 1, Preview: makeLongString(40, 'p'), Summary: "sum"},
			{ID: "c2", File: "y.go", TokenCount: 1, Preview: "tiny", Summary: ""},
		}}
		engine := NewSearchEngine(nil, index)
		results, err := engine.SearchWithBudget(context.Background(),
			SearchQuery{Text: "q", Mode: SearchKeyword}, 3)
		require.NoError(t, err)
		require.Len(t, results, 1)
		require.Equal(t, "c2", results[0].ChunkID)
	})
}

func makeLongString(n int, c byte) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = c
	}
	return string(b)
}

func TestEstimateTokens(t *testing.T) {
	require.Zero(t, estimateTokens(""))
	require.Equal(t, 2, estimateTokens("abcdabcd"))
	require.Equal(t, 0, estimateTokens("abc"), "fewer than 4 chars estimates to zero tokens")
}

package search

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseSearchGrepParams(t *testing.T) {
	params, err := ParseSearchGrepParams(map[string]any{
		"pattern":   "TODO",
		"directory": "src",
	})
	require.NoError(t, err)
	require.Equal(t, "TODO", params.Pattern)
	require.Equal(t, "src", params.Directory)

	_, err = ParseSearchGrepParams(map[string]any{})
	require.Error(t, err)
	require.Contains(t, err.Error(), `missing required parameter "pattern"`)

	// nil counts as absent for required parameters
	_, err = ParseSearchGrepParams(map[string]any{"pattern": nil})
	require.Error(t, err)
	require.Contains(t, err.Error(), `missing required parameter "pattern"`)

	params, err = ParseSearchGrepParams(map[string]any{"pattern": 42})
	require.NoError(t, err)
	require.Equal(t, "42", params.Pattern, "convString stringifies non-strings")
}

func TestParseSearchFindSimilarParams(t *testing.T) {
	params, err := ParseSearchFindSimilarParams(map[string]any{"snippet": "foo bar"})
	require.NoError(t, err)
	require.Equal(t, "foo bar", params.Snippet)

	_, err = ParseSearchFindSimilarParams(map[string]any{})
	require.Error(t, err)
	require.Contains(t, err.Error(), `missing required parameter "snippet"`)
}

func TestParseSearchSemanticParams(t *testing.T) {
	params, err := ParseSearchSemanticParams(map[string]any{"query": "payment"})
	require.NoError(t, err)
	require.Equal(t, "payment", params.Query)

	_, err = ParseSearchSemanticParams(map[string]any{})
	require.Error(t, err)
	require.Contains(t, err.Error(), `missing required parameter "query"`)
}

func TestParamKeys(t *testing.T) {
	require.Equal(t, []string{"pattern", "directory"}, SearchGrepParamKeys())
	require.Equal(t, []string{"snippet", "directory"}, SearchFindSimilarParamKeys())
	require.Equal(t, []string{"query"}, SearchSemanticParamKeys())
}

func TestConvInt64(t *testing.T) {
	for _, tc := range []struct {
		in   any
		want int64
	}{
		{int64(5), 5},
		{float64(5.9), 5},
		{int(7), 7},
		{"42", 42},
		{" 42 ", 42},
	} {
		got, err := convInt64("p", tc.in)
		require.NoError(t, err, "input %v", tc.in)
		require.Equal(t, tc.want, got, "input %v", tc.in)
	}

	_, err := convInt64("p", "not-a-number")
	require.Error(t, err)
	_, err = convInt64("p", true)
	require.Error(t, err)
	require.Contains(t, err.Error(), "expected integer")
}

func TestConvFloat64(t *testing.T) {
	for _, tc := range []struct {
		in   any
		want float64
	}{
		{float64(1.5), 1.5},
		{int64(2), 2},
		{int(3), 3},
		{"3.5", 3.5},
		{"  2.25  ", 2.25},
	} {
		got, err := convFloat64("p", tc.in)
		require.NoError(t, err, "input %v", tc.in)
		require.Equal(t, tc.want, got, "input %v", tc.in)
	}

	_, err := convFloat64("p", "not-a-number")
	require.Error(t, err)
	_, err = convFloat64("p", []any{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "expected number")
}

func TestConvBool(t *testing.T) {
	got, err := convBool("p", true)
	require.NoError(t, err)
	require.True(t, got)

	got, err = convBool("p", "false")
	require.NoError(t, err)
	require.False(t, got)

	_, err = convBool("p", "not-a-bool")
	require.Error(t, err)
	_, err = convBool("p", 1)
	require.Error(t, err)
	require.Contains(t, err.Error(), "expected boolean")
}

func TestConvSliceAndMap(t *testing.T) {
	slice, err := convSliceInterface("p", []any{1, 2})
	require.NoError(t, err)
	require.Len(t, slice, 2)

	_, err = convSliceInterface("p", "nope")
	require.Error(t, err)
	require.Contains(t, err.Error(), "expected array")

	m, err := convMapStringInterface("p", map[string]any{"k": "v"})
	require.NoError(t, err)
	require.Equal(t, "v", m["k"])

	_, err = convMapStringInterface("p", []any{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "expected object")
}

func TestSemanticTermHelpers(t *testing.T) {
	require.Empty(t, semanticTerms("!! ??"))
	require.Equal(t, []string{"alpha", "beta"}, semanticTerms("Alpha, BETA; alpha!"))
	require.Empty(t, semanticTerms("ab 12 x"))

	require.Equal(t, 0.0, semanticScore(nil, "anything"))
	require.Equal(t, 0.5, semanticScore([]string{"hit", "miss"}, "contains hit"))

	require.Equal(t, "kitten", sanitizeSnippet("Kit ten"))
}

func TestJaccardAndSummarize(t *testing.T) {
	require.Equal(t, 0.0, jaccard("", ""))
	require.InDelta(t, 1.0, jaccard("abc", "cba"), 1e-9)
	require.InDelta(t, 1.0/3.0, jaccard("abcd", "abxy"), 1e-9)

	short := "one\ntwo\nthree"
	require.Equal(t, short, summarize(short))

	long := "1\n2\n3\n4\n5\n6\n7"
	require.Equal(t, "1\n2\n3\n4\n5", summarize(long))
}

func TestCandidateFilters(t *testing.T) {
	require.True(t, isSimilarityCandidate("a/b.go"))
	require.True(t, isSimilarityCandidate("a/b.TS"))
	require.False(t, isSimilarityCandidate("a/b.md"))
	require.False(t, isSimilarityCandidate(".x.go"))

	require.True(t, isSemanticCandidate("a/b.md"))
	require.True(t, isSemanticCandidate("a/b.txt"))
	require.False(t, isSemanticCandidate("a/b.png"))

	require.True(t, shouldSkipSearchPath("x/testsuite/agenttests/y.go"))
	require.True(t, shouldSkipSearchPath("dir/.hidden.go"))
	require.False(t, shouldSkipSearchPath("dir/visible.go"))
}

package retrieval

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestJaccardSimilarityEmptyStrings(t *testing.T) {
	require.Equal(t, 1.0, JaccardSimilarity("", ""))
	require.Equal(t, 0.0, JaccardSimilarity("", "hello"))
	require.Equal(t, 0.0, JaccardSimilarity("hello", ""))
}

func TestJaccardSimilarityIdenticalStrings(t *testing.T) {
	require.Equal(t, 1.0, JaccardSimilarity("hello world", "hello world"))
	require.Equal(t, 1.0, JaccardSimilarity("a b c", "a b c"))
}

func TestJaccardSimilarityPartialOverlap(t *testing.T) {
	// "hello world" and "hello there" share "hello"
	// union: {hello, world, there} = 3, intersection: {hello} = 1
	// similarity = 1/3 ≈ 0.333
	sim := JaccardSimilarity("hello world", "hello there")
	require.InDelta(t, 0.333, sim, 0.01)
}

func TestJaccardSimilarityCompletelyDisjoint(t *testing.T) {
	require.Equal(t, 0.0, JaccardSimilarity("abc def", "xyz uvw"))
}

func TestJaccardSimilaritySubset(t *testing.T) {
	// "hello" is subset of "hello world"
	// union: {hello, world} = 2, intersection: {hello} = 1
	// similarity = 1/2 = 0.5
	require.Equal(t, 0.5, JaccardSimilarity("hello", "hello world"))
}

func TestExtractSentenceFindsSimpleSentence(t *testing.T) {
	text := "Hello world. This is great. Welcome back."
	sentence, start, end, found := ExtractSentence(text, "great")
	require.True(t, found)
	require.Equal(t, "This is great.", sentence)
	require.Greater(t, end, start)
}

func TestExtractSentenceFirstSentence(t *testing.T) {
	text := "First sentence. Second sentence."
	sentence, _, _, found := ExtractSentence(text, "First")
	require.True(t, found)
	require.Equal(t, "First sentence.", sentence)
}

func TestExtractSentenceLastSentence(t *testing.T) {
	text := "First sentence. Last one"
	sentence, _, _, found := ExtractSentence(text, "Last")
	require.True(t, found)
	require.Equal(t, "Last one", sentence)
}

func TestExtractSentenceTermNotFound(t *testing.T) {
	text := "First sentence. Second sentence."
	_, _, _, found := ExtractSentence(text, "missing")
	require.False(t, found)
}

func TestExtractSentenceEmptyText(t *testing.T) {
	_, _, _, found := ExtractSentence("", "term")
	require.False(t, found)
}

func TestExtractSentenceEmptyTerm(t *testing.T) {
	_, _, _, found := ExtractSentence("some text", "")
	require.False(t, found)
}

func TestExtractSentenceCaseInsensitive(t *testing.T) {
	text := "The KEYWORD is important."
	sentence, _, _, found := ExtractSentence(text, "keyword")
	require.True(t, found)
	require.Equal(t, "The KEYWORD is important.", sentence)
}

func TestTermPresentBasic(t *testing.T) {
	require.True(t, TermPresent("hello world", "hello"))
	require.True(t, TermPresent("hello world", "world"))
	require.False(t, TermPresent("hello world", "xyz"))
}

func TestTermPresentCaseInsensitive(t *testing.T) {
	require.True(t, TermPresent("Hello World", "hello"))
	require.True(t, TermPresent("hello world", "WORLD"))
}

func TestTermPresentEmptyStrings(t *testing.T) {
	require.False(t, TermPresent("", "term"))
	require.False(t, TermPresent("text", ""))
	require.False(t, TermPresent("", ""))
}

func TestJaccardSimilarityWithPunctuation(t *testing.T) {
	// Both should tokenize the same way
	sim := JaccardSimilarity("hello, world!", "hello world")
	require.Equal(t, 1.0, sim)
}

func TestJaccardSimilarityWithNumbers(t *testing.T) {
	sim := JaccardSimilarity("version 2.5 is released", "version 2.5 released")
	require.GreaterOrEqual(t, sim, 0.5)
	require.LessOrEqual(t, sim, 1.0)
}

func TestExtractSentenceWithMultiplePunctuation(t *testing.T) {
	text := "What is this? This is a test! It works."
	sentence, _, _, found := ExtractSentence(text, "test")
	require.True(t, found)
	require.Equal(t, "This is a test!", sentence)
}

func TestJaccardSimilarityBoundaryValues(t *testing.T) {
	tests := []struct {
		a, b string
		min  float64
		max  float64
	}{
		{"identical", "identical", 1.0, 1.0},
		{"a", "b", 0.0, 0.0},
		{"abc", "abc def", 0.49, 0.51},
	}

	for _, tt := range tests {
		sim := JaccardSimilarity(tt.a, tt.b)
		require.True(t, sim >= tt.min && sim <= tt.max,
			"expected %v to be between %v and %v", sim, tt.min, tt.max)
	}
}

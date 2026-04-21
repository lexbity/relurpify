package search

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

func TestSearchHelpersClassifyPaths(t *testing.T) {
	require.True(t, shouldSkipGeneratedDir(".git"))
	require.True(t, shouldSkipGeneratedDir("build"))
	require.False(t, shouldSkipGeneratedDir("src"))

	require.True(t, shouldSkipSearchPath(".hidden.go"))
	require.True(t, shouldSkipSearchPath("workspace/testsuite/agenttests/demo/file.go"))
	require.False(t, shouldSkipSearchPath("workspace/src/file.go"))

	require.True(t, isSimilarityCandidate("main.go"))
	require.False(t, isSimilarityCandidate("README.md"))
	require.True(t, isSemanticCandidate("README.md"))
	require.False(t, isSemanticCandidate("image.png"))
}

func TestSearchHelpersScoringAndSummaries(t *testing.T) {
	require.Equal(t, "abc", sanitizeSnippet("A B C"))
	require.Equal(t, []string{"alpha", "beta", "1234"}, semanticTerms("alpha beta alpha 12 1234"))
	require.Equal(t, 0.5, semanticScore([]string{"alpha", "beta"}, "alpha only"))
	require.Equal(t, 1.0, jaccard("ab", "ab"))
	require.Equal(t, "line1\nline2\nline3\nline4\nline5", summarize("line1\nline2\nline3\nline4\nline5\nline6"))
}

func TestTraversalPermissionCacheCachesResults(t *testing.T) {
	calls := 0
	cache := &traversalPermissionCache{
		check: func(context.Context, core.FileSystemAction, string) error {
			calls++
			return nil
		},
		cached: make(map[permissionCacheKey]permissionCacheEntry),
	}
	require.NoError(t, cache.Check(context.Background(), core.FileSystemRead, "/tmp/file"))
	require.NoError(t, cache.Check(context.Background(), core.FileSystemRead, "/tmp/file"))
	require.Equal(t, 1, calls)
}

func TestGrepAndSemanticSearchToolsExecute(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "main.go"), []byte("package main\n// alpha beta\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "notes.md"), []byte("alpha beta notes\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "image.png"), []byte("not searchable\n"), 0o644))

	grep := &GrepTool{BasePath: dir}
	res, err := grep.Execute(context.Background(), core.NewContext(), map[string]interface{}{
		"directory": "src",
		"pattern":   "alpha",
	})
	require.NoError(t, err)
	require.True(t, res.Success)
	matchesBytes, err := json.Marshal(res.Data["matches"])
	require.NoError(t, err)
	var matches []map[string]interface{}
	require.NoError(t, json.Unmarshal(matchesBytes, &matches))
	require.Len(t, matches, 2)

	semantic := &SemanticSearchTool{BasePath: dir}
	res, err = semantic.Execute(context.Background(), core.NewContext(), map[string]interface{}{
		"query": "alpha beta",
	})
	require.NoError(t, err)
	require.True(t, res.Success)
	hitsBytes, err := json.Marshal(res.Data["results"])
	require.NoError(t, err)
	var hits []map[string]interface{}
	require.NoError(t, json.Unmarshal(hitsBytes, &hits))
	require.NotEmpty(t, hits)
}

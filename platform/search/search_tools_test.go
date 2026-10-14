package search

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	path := filepath.Join(dir, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

func encodeResult(t *testing.T, res any) string {
	t.Helper()
	data, err := json.Marshal(res)
	require.NoError(t, err)
	return string(data)
}

func TestGrepTool_FindsMatchesCaseInsensitiveAndSkipsGeneratedDirs(t *testing.T) {
	base := t.TempDir()
	writeFile(t, base, "main.go", "package main\n\n// TODO fix this\n")
	writeFile(t, base, "docs/note.md", "# Notes\n\ntodo: write docs\n")
	writeFile(t, base, ".git/config", "[core]\ntodo = hidden\n")
	writeFile(t, base, "node_modules/dep.js", "// TODO hidden\n")
	writeFile(t, base, "dist/out.txt", "TODO hidden\n")

	tool := &GrepTool{BasePath: base}
	res, err := tool.Execute(context.Background(), map[string]any{"pattern": "todo"})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.True(t, res.Success)

	encoded := encodeResult(t, res.Data)
	require.Contains(t, encoded, "TODO fix this")
	require.Contains(t, encoded, "todo: write docs")
	require.NotContains(t, encoded, "hidden", "generated dirs must be skipped")
}

func TestGrepTool_StripsCarriageReturnsFromMatches(t *testing.T) {
	base := t.TempDir()
	writeFile(t, base, "crlf.txt", "first\r\ntodo crlf\r\nlast\r\n")

	tool := &GrepTool{BasePath: base}
	res, err := tool.Execute(context.Background(), map[string]any{"pattern": "todo"})
	require.NoError(t, err)
	require.True(t, res.Success)

	type grepMatch struct {
		File    string `json:"file"`
		Line    int    `json:"line"`
		Content string `json:"content"`
	}
	var payload struct {
		Matches []grepMatch `json:"matches"`
	}
	require.NoError(t, json.Unmarshal([]byte(encodeResult(t, res.Data)), &payload))
	require.Len(t, payload.Matches, 1)
	require.Equal(t, 2, payload.Matches[0].Line)
	require.Equal(t, "todo crlf", payload.Matches[0].Content, "CR must be stripped")
}

func TestGrepTool_ScansOverlongLinesInChunks(t *testing.T) {
	base := t.TempDir()
	writeFile(t, base, "huge.log", "TODO this line is longer than the chunk size")

	tool := &GrepTool{BasePath: base}
	res, err := tool.Execute(context.Background(), map[string]any{"pattern": "chunk size"})
	require.NoError(t, err)
	require.True(t, res.Success)

	encoded := encodeResult(t, res.Data)
	require.Contains(t, encoded, "huge.log", "match inside an overlong line must be found")
}

func TestGrepTool_SkipsUnreadableFiles(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("permission bits are not enforceable for this user")
	}
	base := t.TempDir()
	locked := filepath.Join(base, "locked.txt")
	require.NoError(t, os.WriteFile(locked, []byte("TODO secret\n"), 0o000))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o600) })

	writeFile(t, base, "open.txt", "todo visible\n")

	tool := &GrepTool{BasePath: base}
	res, err := tool.Execute(context.Background(), map[string]any{"pattern": "todo"})
	require.NoError(t, err, "unreadable files must be skipped, not fail the walk")
	require.True(t, res.Success)

	encoded := encodeResult(t, res.Data)
	require.Contains(t, encoded, "todo visible")
	require.NotContains(t, encoded, "locked.txt")
}

func TestGrepTool_WalkAbortsOnSymlinkEscape(t *testing.T) {
	base := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.Symlink(outside, filepath.Join(base, "escape.go")))

	tool := &GrepTool{BasePath: base}
	_, err := tool.Execute(context.Background(), map[string]any{"pattern": "x", "directory": "."})
	require.Error(t, err, "a symlinked source file pointing outside the base must abort the walk")
}

func TestGrepTool_MissingPatternFails(t *testing.T) {
	tool := &GrepTool{BasePath: t.TempDir()}
	_, err := tool.Execute(context.Background(), map[string]any{})
	require.Error(t, err)
	require.Contains(t, err.Error(), `missing required parameter "pattern"`)
}

func TestGrepTool_Metadata(t *testing.T) {
	tool := &GrepTool{BasePath: t.TempDir()}
	require.Equal(t, "search_grep", tool.Name())
	require.Equal(t, "search", tool.Category())
	require.NotEmpty(t, tool.Description())
	require.NotEmpty(t, tool.Tags())
	require.True(t, tool.IsAvailable(context.Background()))
	require.NotNil(t, tool.Permissions().Permissions)
	require.Equal(t, []string{"pattern", "directory"}, tool.ParamKeys())

	params := tool.Parameters()
	require.Len(t, params, 2)
	require.Equal(t, "pattern", params[0].Name)
	require.True(t, params[0].Required)
	require.Equal(t, "directory", params[1].Name)
	require.False(t, params[1].Required)
}

func TestSimilarityTool_RanksSimilarFilesBySemanticTerms(t *testing.T) {
	base := t.TempDir()
	writeFile(t, base, "payment.go", "func processPayment(invoice) { charge(invoice) }\n")
	writeFile(t, base, "unrelated.go", "func download(url) { fetch(url) }\n")
	writeFile(t, base, "payment.txt", "processPayment invoice charge\n") // not a candidate extension

	tool := &SimilarityTool{BasePath: base}
	res, err := tool.Execute(context.Background(), map[string]any{
		"snippet":   "func processPayment(invoice) { charge(invoice) }",
		"directory": ".",
	})
	require.NoError(t, err)
	require.True(t, res.Success)

	type simMatch struct {
		File  string  `json:"file"`
		Score float64 `json:"score"`
	}
	var payload struct {
		Matches []simMatch `json:"matches"`
	}
	require.NoError(t, json.Unmarshal([]byte(encodeResult(t, res.Data)), &payload))
	require.NotEmpty(t, payload.Matches)
	require.Contains(t, payload.Matches[0].File, "payment.go")
	require.Greater(t, payload.Matches[0].Score, 0.3)
	for i := 1; i < len(payload.Matches); i++ {
		require.GreaterOrEqual(t, payload.Matches[i-1].Score, payload.Matches[i].Score,
			"matches must be sorted by descending score")
	}
}

func TestSimilarityTool_FallsBackToJaccardWhenNoTerms(t *testing.T) {
	base := t.TempDir()
	// Snippet has no terms of length >= 3, so semanticScore is 0 and the
	// character-jaccard fallback decides.
	writeFile(t, base, "close.go", "ab cd xy\n")
	writeFile(t, base, "far.go", "zz zz zz\n")

	tool := &SimilarityTool{BasePath: base}
	res, err := tool.Execute(context.Background(), map[string]any{"snippet": "ab cd", "directory": "."})
	require.NoError(t, err)
	require.True(t, res.Success)

	encoded := encodeResult(t, res.Data)
	require.Contains(t, encoded, "close.go", "character-overlapping file must match via jaccard")
	require.NotContains(t, encoded, "far.go")
}

func TestSimilarityTool_SkipsHiddenFilesAndTestsuite(t *testing.T) {
	base := t.TempDir()
	writeFile(t, base, ".hidden.go", "alpha beta gamma\n")
	writeFile(t, base, "testsuite/agenttests/x.go", "alpha beta gamma\n")
	writeFile(t, base, "visible.go", "alpha beta gamma\n")

	tool := &SimilarityTool{BasePath: base}
	res, err := tool.Execute(context.Background(), map[string]any{"snippet": "alpha beta gamma", "directory": "."})
	require.NoError(t, err)
	require.True(t, res.Success)

	encoded := encodeResult(t, res.Data)
	require.Contains(t, encoded, "visible.go")
	require.NotContains(t, encoded, ".hidden.go")
	require.NotContains(t, encoded, "agenttests")
}

func TestSimilarityTool_SymlinkEscapeFails(t *testing.T) {
	base := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.Symlink(outside, filepath.Join(base, "escape.go")))

	tool := &SimilarityTool{BasePath: base}
	_, err := tool.Execute(context.Background(), map[string]any{"snippet": "anything", "directory": "."})
	require.Error(t, err, "symlinked candidate outside the base must abort")
}

func TestSimilarityTool_MissingSnippetFails(t *testing.T) {
	tool := &SimilarityTool{BasePath: t.TempDir()}
	_, err := tool.Execute(context.Background(), map[string]any{})
	require.Error(t, err)
	require.Contains(t, err.Error(), `missing required parameter "snippet"`)
}

func TestSimilarityTool_Metadata(t *testing.T) {
	tool := &SimilarityTool{BasePath: t.TempDir()}
	require.Equal(t, "search_find_similar", tool.Name())
	require.Equal(t, "search", tool.Category())
	require.NotEmpty(t, tool.Description())
	require.NotEmpty(t, tool.Tags())
	require.True(t, tool.IsAvailable(context.Background()))
	require.NotNil(t, tool.Permissions().Permissions)
	require.Equal(t, []string{"snippet", "directory"}, tool.ParamKeys())
	require.Len(t, tool.Parameters(), 2)
}

func TestSemanticSearchTool_FindsAndRanksMatches(t *testing.T) {
	base := t.TempDir()
	writeFile(t, base, "billing.md", "# Billing\n\nPayment processing happens nightly.\n")
	writeFile(t, base, "notes/payment.txt", "payment pending\n")
	writeFile(t, base, "unrelated.md", "nothing relevant here\n")
	writeFile(t, base, ".git/hidden.md", "payment\n")
	writeFile(t, base, "logo.png", "payment\n") // extension not a candidate

	tool := &SemanticSearchTool{BasePath: base}
	res, err := tool.Execute(context.Background(), map[string]any{"query": "Payment Processing"})
	require.NoError(t, err)
	require.True(t, res.Success)

	type hit struct {
		File  string  `json:"file"`
		Score float64 `json:"score"`
	}
	var payload struct {
		Results []hit `json:"results"`
	}
	require.NoError(t, json.Unmarshal([]byte(encodeResult(t, res.Data)), &payload))
	require.Len(t, payload.Results, 2, "only candidate files with term hits are returned")
	require.Contains(t, payload.Results[0].File, "billing.md", "full match must rank first")
	require.Equal(t, 1.0, payload.Results[0].Score)
	require.Contains(t, payload.Results[1].File, "payment.txt")
	for _, h := range payload.Results {
		require.NotContains(t, h.File, ".git")
	}
}

func TestSemanticSearchTool_NoMatches(t *testing.T) {
	base := t.TempDir()
	writeFile(t, base, "a.md", "completely different\n")

	tool := &SemanticSearchTool{BasePath: base}
	res, err := tool.Execute(context.Background(), map[string]any{"query": "zeppelin"})
	require.NoError(t, err)
	require.True(t, res.Success)
	require.Empty(t, res.Data["results"])
}

func TestSemanticSearchTool_MissingQueryFails(t *testing.T) {
	tool := &SemanticSearchTool{BasePath: t.TempDir()}
	_, err := tool.Execute(context.Background(), map[string]any{})
	require.Error(t, err)
	require.Contains(t, err.Error(), `missing required parameter "query"`)
}

func TestSemanticSearchTool_Metadata(t *testing.T) {
	tool := &SemanticSearchTool{BasePath: t.TempDir()}
	require.Equal(t, "search_semantic", tool.Name())
	require.Equal(t, "search", tool.Category())
	require.NotEmpty(t, tool.Description())
	require.NotEmpty(t, tool.Tags())
	require.True(t, tool.IsAvailable(context.Background()))
	require.NotNil(t, tool.Permissions().Permissions)
	require.Equal(t, []string{"query"}, tool.ParamKeys())
	require.Len(t, tool.Parameters(), 1)
}

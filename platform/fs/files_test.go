package fs

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	files     = "files"
	hello_txt = "hello.txt"
	matches   = "matches"
	src       = "src"
)

func TestReadWriteListFileTools(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	writeTool := &WriteFileTool{BasePath: dir, Backup: true}
	_, err := writeTool.Execute(ctx, map[string]any{
		path:    hello_txt,
		content: "hi relurpify",
	})
	require.NoError(t, err)

	readTool := &ReadFileTool{BasePath: dir}
	readRes, err := readTool.Execute(ctx, map[string]any{path: hello_txt})
	require.NoError(t, err)
	assert.Equal(t, "hi relurpify", readRes.Data[content])

	listTool := &ListFilesTool{BasePath: dir}
	listRes, err := listTool.Execute(ctx, map[string]any{
		directory: ".",
		pattern:   txt,
	})
	require.NoError(t, err)
	files := listRes.Data[files].([]string)
	assert.Len(t, files, 1)
	assert.Equal(t, filepath.Join(dir, hello_txt), files[0])
}

func TestFileToolsHonorSandboxProtectedPaths(t *testing.T) {
	dir := t.TempDir()
	protected := filepath.Join(dir, "relurpify_cfg", "agent.yaml")
	require.NoError(t, MkdirAllSecure(filepath.Dir(protected)))
	require.NoError(t, WriteFileSecure(protected, []byte(secret)))

	scope := NewFileScopePolicy(dir, []string{protected})

	readTool := &ReadFileTool{BasePath: dir}
	readTool.SetSandboxScope(scope)
	_, err := readTool.Execute(context.Background(), map[string]any{path: protected})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrFileScopeProtectedPath)

	writeTool := &WriteFileTool{BasePath: dir}
	writeTool.SetSandboxScope(scope)
	_, err = writeTool.Execute(context.Background(), map[string]any{
		path:    protected,
		content: "mutate",
	})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrFileScopeProtectedPath)
}

func TestSearchInFilesTool(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "code.go")
	require.NoError(t, WriteFileSecure(file, []byte("package main\n// TODO: fix bug\n")))

	tool := &SearchInFilesTool{BasePath: dir}
	res, err := tool.Execute(context.Background(), map[string]any{
		directory: ".",
		pattern:   "TODO",
	})
	require.NoError(t, err)
	bytes, err := json.Marshal(res.Data[matches])
	require.NoError(t, err)
	var decoded []map[string]any
	require.NoError(t, json.Unmarshal(bytes, &decoded))
	assert.NotEmpty(t, decoded)
}

func TestSearchInFilesToolDefaultsDirectory(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.c")
	require.NoError(t, WriteFileSecure(file, []byte("#include <stdio.h>\n")))

	tool := &SearchInFilesTool{BasePath: dir}
	res, err := tool.Execute(context.Background(), map[string]any{
		pattern: "#include",
	})
	require.NoError(t, err)
	bytes, err := json.Marshal(res.Data[matches])
	require.NoError(t, err)
	var decoded []map[string]any
	require.NoError(t, json.Unmarshal(bytes, &decoded))
	assert.NotEmpty(t, decoded)
}

func TestListFilesToolMatchesRecursiveRelativePatterns(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, src, "nested", "lib.rs")
	require.NoError(t, MkdirAllSecure(filepath.Dir(target)))
	require.NoError(t, WriteFileSecure(target, []byte("pub fn demo() {}\n")))

	tool := &ListFilesTool{BasePath: dir}
	res, err := tool.Execute(context.Background(), map[string]any{
		directory: ".",
		pattern:   "**/*.rs",
	})
	require.NoError(t, err)
	files := res.Data[files].([]string)
	assert.Contains(t, files, target)
}

func TestListFilesToolDefaultsDirectory(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "README.md")
	require.NoError(t, WriteFileSecure(target, []byte("# docs\n")))

	tool := &ListFilesTool{BasePath: dir}
	res, err := tool.Execute(context.Background(), map[string]any{
		pattern: "*.md",
	})
	require.NoError(t, err)
	files := res.Data[files].([]string)
	assert.Contains(t, files, target)
}

func TestListFilesToolSkipsGeneratedDirectories(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, src, "main.rs")
	generated := filepath.Join(dir, "target", "debug", "build.rs")
	require.NoError(t, MkdirAllSecure(filepath.Dir(source)))
	require.NoError(t, MkdirAllSecure(filepath.Dir(generated)))
	require.NoError(t, WriteFileSecure(source, []byte("fn main() {}\n")))
	require.NoError(t, WriteFileSecure(generated, []byte("fn generated() {}\n")))

	tool := &ListFilesTool{BasePath: dir}
	res, err := tool.Execute(context.Background(), map[string]any{
		directory: ".",
		pattern:   "**/*.rs",
	})
	require.NoError(t, err)
	files := res.Data[files].([]string)
	assert.Contains(t, files, source)
	assert.NotContains(t, files, generated)
}

func TestSearchInFilesToolSkipsGeneratedDirectories(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, src, "main.rs")
	generated := filepath.Join(dir, "target", "debug", "build.rs")
	require.NoError(t, MkdirAllSecure(filepath.Dir(source)))
	require.NoError(t, MkdirAllSecure(filepath.Dir(generated)))
	require.NoError(t, WriteFileSecure(source, []byte("// TODO: source\n")))
	require.NoError(t, WriteFileSecure(generated, []byte("// TODO: generated\n")))

	tool := &SearchInFilesTool{BasePath: dir}
	res, err := tool.Execute(context.Background(), map[string]any{
		directory: ".",
		pattern:   "TODO",
	})
	require.NoError(t, err)
	bytes, err := json.Marshal(res.Data[matches])
	require.NoError(t, err)
	var decoded []map[string]any
	require.NoError(t, json.Unmarshal(bytes, &decoded))
	assert.Len(t, decoded, 1)
	assert.Equal(t, source, decoded[0][file])
}

func TestSearchInFilesToolDefaultsToCaseInsensitiveMatching(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	require.NoError(t, WriteFileSecure(file, []byte("TODO: fix bug\n")))

	tool := &SearchInFilesTool{BasePath: dir}
	res, err := tool.Execute(context.Background(), map[string]any{
		directory: ".",
		pattern:   "todo",
	})
	require.NoError(t, err)
	bytes, err := json.Marshal(res.Data[matches])
	require.NoError(t, err)
	var decoded []map[string]any
	require.NoError(t, json.Unmarshal(bytes, &decoded))
	assert.Len(t, decoded, 1)
	assert.Equal(t, file, decoded[0][file])
}

func TestSearchInFilesToolSupportsCaseSensitiveMatching(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	require.NoError(t, WriteFileSecure(file, []byte("TODO: fix bug\n")))

	tool := &SearchInFilesTool{BasePath: dir}
	res, err := tool.Execute(context.Background(), map[string]any{
		directory:        ".",
		pattern:          "todo",
		"case_sensitive": true,
	})
	require.NoError(t, err)
	bytes, err := json.Marshal(res.Data[matches])
	require.NoError(t, err)
	var decoded []map[string]any
	require.NoError(t, json.Unmarshal(bytes, &decoded))
	assert.Empty(t, decoded)
}

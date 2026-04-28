package fs

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReadWriteListFileTools(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	writeTool := &WriteFileTool{BasePath: dir, Backup: true}
	_, err := writeTool.Execute(ctx, map[string]interface{}{
		"path":    "hello.txt",
		"content": "hi relurpify",
	})
	assert.NoError(t, err)

	readTool := &ReadFileTool{BasePath: dir}
	readRes, err := readTool.Execute(ctx, map[string]interface{}{"path": "hello.txt"})
	assert.NoError(t, err)
	assert.Equal(t, "hi relurpify", readRes.Data["content"])

	listTool := &ListFilesTool{BasePath: dir}
	listRes, err := listTool.Execute(ctx, map[string]interface{}{
		"directory": ".",
		"pattern":   "*.txt",
	})
	assert.NoError(t, err)
	files := listRes.Data["files"].([]string)
	assert.Len(t, files, 1)
	assert.Equal(t, filepath.Join(dir, "hello.txt"), files[0])
}

func TestFileToolsHonorSandboxProtectedPaths(t *testing.T) {
	dir := t.TempDir()
	protected := filepath.Join(dir, "relurpify_cfg", "agent.manifest.yaml")
	assert.NoError(t, os.MkdirAll(filepath.Dir(protected), 0o755))
	assert.NoError(t, os.WriteFile(protected, []byte("secret"), 0o644))

	scope := NewFileScopePolicy(dir, []string{protected})

	readTool := &ReadFileTool{BasePath: dir}
	readTool.SetSandboxScope(scope)
	_, err := readTool.Execute(context.Background(), map[string]interface{}{"path": protected})
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrFileScopeProtectedPath)

	writeTool := &WriteFileTool{BasePath: dir}
	writeTool.SetSandboxScope(scope)
	_, err = writeTool.Execute(context.Background(), map[string]interface{}{
		"path":    protected,
		"content": "mutate",
	})
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrFileScopeProtectedPath)
}

func TestSearchInFilesTool(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "code.go")
	assert.NoError(t, os.WriteFile(file, []byte("package main\n// TODO: fix bug\n"), 0o644))

	tool := &SearchInFilesTool{BasePath: dir}
	res, err := tool.Execute(context.Background(), map[string]interface{}{
		"directory": ".",
		"pattern":   "TODO",
	})
	assert.NoError(t, err)
	bytes, err := json.Marshal(res.Data["matches"])
	assert.NoError(t, err)
	var decoded []map[string]interface{}
	assert.NoError(t, json.Unmarshal(bytes, &decoded))
	assert.NotEmpty(t, decoded)
}

func TestSearchInFilesToolDefaultsDirectory(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.c")
	assert.NoError(t, os.WriteFile(file, []byte("#include <stdio.h>\n"), 0o644))

	tool := &SearchInFilesTool{BasePath: dir}
	res, err := tool.Execute(context.Background(), map[string]interface{}{
		"pattern": "#include",
	})
	assert.NoError(t, err)
	bytes, err := json.Marshal(res.Data["matches"])
	assert.NoError(t, err)
	var decoded []map[string]interface{}
	assert.NoError(t, json.Unmarshal(bytes, &decoded))
	assert.NotEmpty(t, decoded)
}

func TestListFilesToolMatchesRecursiveRelativePatterns(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "src", "nested", "lib.rs")
	assert.NoError(t, os.MkdirAll(filepath.Dir(target), 0o755))
	assert.NoError(t, os.WriteFile(target, []byte("pub fn demo() {}\n"), 0o644))

	tool := &ListFilesTool{BasePath: dir}
	res, err := tool.Execute(context.Background(), map[string]interface{}{
		"directory": ".",
		"pattern":   "**/*.rs",
	})
	assert.NoError(t, err)
	files := res.Data["files"].([]string)
	assert.Contains(t, files, target)
}

func TestListFilesToolDefaultsDirectory(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "README.md")
	assert.NoError(t, os.WriteFile(target, []byte("# docs\n"), 0o644))

	tool := &ListFilesTool{BasePath: dir}
	res, err := tool.Execute(context.Background(), map[string]interface{}{
		"pattern": "*.md",
	})
	assert.NoError(t, err)
	files := res.Data["files"].([]string)
	assert.Contains(t, files, target)
}

func TestListFilesToolSkipsGeneratedDirectories(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "src", "main.rs")
	generated := filepath.Join(dir, "target", "debug", "build.rs")
	assert.NoError(t, os.MkdirAll(filepath.Dir(source), 0o755))
	assert.NoError(t, os.MkdirAll(filepath.Dir(generated), 0o755))
	assert.NoError(t, os.WriteFile(source, []byte("fn main() {}\n"), 0o644))
	assert.NoError(t, os.WriteFile(generated, []byte("fn generated() {}\n"), 0o644))

	tool := &ListFilesTool{BasePath: dir}
	res, err := tool.Execute(context.Background(), map[string]interface{}{
		"directory": ".",
		"pattern":   "**/*.rs",
	})
	assert.NoError(t, err)
	files := res.Data["files"].([]string)
	assert.Contains(t, files, source)
	assert.NotContains(t, files, generated)
}

func TestSearchInFilesToolSkipsGeneratedDirectories(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "src", "main.rs")
	generated := filepath.Join(dir, "target", "debug", "build.rs")
	assert.NoError(t, os.MkdirAll(filepath.Dir(source), 0o755))
	assert.NoError(t, os.MkdirAll(filepath.Dir(generated), 0o755))
	assert.NoError(t, os.WriteFile(source, []byte("// TODO: source\n"), 0o644))
	assert.NoError(t, os.WriteFile(generated, []byte("// TODO: generated\n"), 0o644))

	tool := &SearchInFilesTool{BasePath: dir}
	res, err := tool.Execute(context.Background(), map[string]interface{}{
		"directory": ".",
		"pattern":   "TODO",
	})
	assert.NoError(t, err)
	bytes, err := json.Marshal(res.Data["matches"])
	assert.NoError(t, err)
	var decoded []map[string]interface{}
	assert.NoError(t, json.Unmarshal(bytes, &decoded))
	assert.Len(t, decoded, 1)
	assert.Equal(t, source, decoded[0]["file"])
}

func TestSearchInFilesToolDefaultsToCaseInsensitiveMatching(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	assert.NoError(t, os.WriteFile(file, []byte("TODO: fix bug\n"), 0o644))

	tool := &SearchInFilesTool{BasePath: dir}
	res, err := tool.Execute(context.Background(), map[string]interface{}{
		"directory": ".",
		"pattern":   "todo",
	})
	assert.NoError(t, err)
	bytes, err := json.Marshal(res.Data["matches"])
	assert.NoError(t, err)
	var decoded []map[string]interface{}
	assert.NoError(t, json.Unmarshal(bytes, &decoded))
	assert.Len(t, decoded, 1)
	assert.Equal(t, file, decoded[0]["file"])
}

func TestSearchInFilesToolSupportsCaseSensitiveMatching(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	assert.NoError(t, os.WriteFile(file, []byte("TODO: fix bug\n"), 0o644))

	tool := &SearchInFilesTool{BasePath: dir}
	res, err := tool.Execute(context.Background(), map[string]interface{}{
		"directory":      ".",
		"pattern":        "todo",
		"case_sensitive": true,
	})
	assert.NoError(t, err)
	bytes, err := json.Marshal(res.Data["matches"])
	assert.NoError(t, err)
	var decoded []map[string]interface{}
	assert.NoError(t, json.Unmarshal(bytes, &decoded))
	assert.Len(t, decoded, 0)
}

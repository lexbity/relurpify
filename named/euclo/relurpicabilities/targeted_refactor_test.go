package relurpicabilities

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/context/knowledge/ast"
)

func newTargetedRefactorTestDeps(t *testing.T) (SymbolQuerier, EdgeStore, WorkspaceFiles, IndexRefresher, string) {
	t.Helper()
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "sample.go")
	source := "package sample\n\nfunc Hello() string {\n\treturn \"old\"\n}\n"
	require.NoError(t, os.WriteFile(path, []byte(source), 0o644))

	store, err := ast.NewTestStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	manager := ast.NewIndexManager(store, ast.IndexConfig{WorkspacePath: tmpDir})
	require.NoError(t, manager.IndexFile(context.Background(), path))

	return manager, store, &workspaceFileSystem{workspace: tmpDir}, manager, path
}

func TestTargetedRefactorPreviewUsesExplicitReplacement(t *testing.T) {
	querier, estore, files, refresher, path := newTargetedRefactorTestDeps(t)
	handler := NewTargetedRefactorHandler(querier, estore, files, refresher, nil)

	result, err := handler.Invoke(context.Background(), nil, map[string]any{
		"symbol":         "Hello",
		"transformation": "rename the greeting helper body",
		"replacement":    "func Hello() string {\n\treturn \"goodbye\"\n}",
		"preview":        true,
	})
	require.NoError(t, err)
	require.True(t, result.Success)

	data := result.Data
	require.NotNil(t, data)
	require.Equal(t, true, data["preview"])
	require.Equal(t, false, data["applied"])
	require.Contains(t, data["before"].(string), "return \"old\"")
	require.Contains(t, data["after"].(string), "return \"goodbye\"")

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(content), "return \"old\"")
	require.NotContains(t, string(content), "return \"goodbye\"")
}

func TestTargetedRefactorWritesReplacementAndRefreshesIndex(t *testing.T) {
	querier, estore, files, refresher, path := newTargetedRefactorTestDeps(t)
	handler := NewTargetedRefactorHandler(querier, estore, files, refresher, nil)

	result, err := handler.Invoke(context.Background(), nil, map[string]any{
		"symbol":         "Hello",
		"transformation": "replace the helper body",
		"replacement":    "func Hello() string {\n\treturn \"goodbye\"\n}",
	})
	require.NoError(t, err)
	require.True(t, result.Success)

	data := result.Data
	require.NotNil(t, data)
	require.Equal(t, true, data["applied"])
	require.Equal(t, false, data["preview"])

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(content), "return \"goodbye\"")
	require.NotContains(t, string(content), "return \"old\"")
}

func TestTargetedRefactorResolvesMostSpecificNode(t *testing.T) {
	now := time.Now()
	handler := &TargetedRefactorHandler{}
	nodes := []*ast.Node{
		{ID: "a", FileID: "file-a", Name: "Hello", StartLine: 3, EndLine: 7, UpdatedAt: now},
		{ID: "b", FileID: "file-a", Name: "Hello", StartLine: 4, EndLine: 5, UpdatedAt: now},
	}

	target, err := handler.selectTargetNode(nodes, "")
	require.NoError(t, err)
	require.Equal(t, "b", target.ID)
}

func TestTargetedRefactorRequiresFileHintForAmbiguousSymbols(t *testing.T) {
	now := time.Now()
	handler := &TargetedRefactorHandler{}
	nodes := []*ast.Node{
		{ID: "a", FileID: "file-a", Name: "Hello", StartLine: 3, EndLine: 7, UpdatedAt: now},
		{ID: "b", FileID: "file-b", Name: "Hello", StartLine: 4, EndLine: 5, UpdatedAt: now},
	}

	_, err := handler.selectTargetNode(nodes, "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "ambiguous")
}

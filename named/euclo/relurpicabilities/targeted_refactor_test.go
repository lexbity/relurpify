package relurpicabilities

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/ast"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/sandbox"
	"github.com/stretchr/testify/require"
)

func TestTargetedRefactorPreviewUsesExplicitReplacement(t *testing.T) {
	env, path := newTargetedRefactorTestEnv(t)
	handler := NewTargetedRefactorHandler(env)

	result, err := handler.Invoke(context.Background(), nil, map[string]interface{}{
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
	env, path := newTargetedRefactorTestEnv(t)
	handler := NewTargetedRefactorHandler(env)

	result, err := handler.Invoke(context.Background(), nil, map[string]interface{}{
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

func newTargetedRefactorTestEnv(t *testing.T) (agentenv.WorkspaceEnvironment, string) {
	t.Helper()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "sample.go")
	source := "package sample\n\nfunc Hello() string {\n\treturn \"old\"\n}\n"
	require.NoError(t, os.WriteFile(path, []byte(source), 0o644))

	store, err := ast.NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	manager := ast.NewIndexManager(store, ast.IndexConfig{WorkspacePath: tmpDir})
	require.NoError(t, manager.IndexFile(path))

	env := agentenv.WorkspaceEnvironment{
		Config:            &core.Config{},
		Registry:          capability.NewCapabilityRegistry(),
		IndexManager:      manager,
		CommandRunner:     nil,
		PermissionManager: nil,
		Model:             nil,
		CommandPolicy:     sandbox.CommandPolicyFunc(func(ctx context.Context, req sandbox.CommandRequest) error { return nil }),
		FileScope:         sandbox.NewFileScopePolicy(tmpDir, nil),
	}
	return env, path
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

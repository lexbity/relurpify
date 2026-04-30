package relurpicabilities

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/ast"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/sandbox"
	"github.com/stretchr/testify/require"
)

type phase12RecordingRunner struct {
	stdout string
	stderr string
	err    error
}

func (r *phase12RecordingRunner) Run(ctx context.Context, req sandbox.CommandRequest) (string, string, error) {
	return r.stdout, r.stderr, r.err
}

func newPhase12IndexedEnv(t *testing.T, files map[string]string) (agentenv.WorkspaceEnvironment, map[string]string) {
	t.Helper()
	tmpDir := t.TempDir()
	store, err := ast.NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	manager := ast.NewIndexManager(store, ast.IndexConfig{WorkspacePath: tmpDir})
	paths := make(map[string]string, len(files))
	for relPath, content := range files {
		absPath := filepath.Join(tmpDir, relPath)
		require.NoError(t, os.MkdirAll(filepath.Dir(absPath), 0o755))
		require.NoError(t, os.WriteFile(absPath, []byte(content), 0o644))
		require.NoError(t, manager.IndexFile(absPath))
		paths[relPath] = absPath
	}

	env := agentenv.WorkspaceEnvironment{
		Config:        &core.Config{},
		Registry:      capability.NewCapabilityRegistry(),
		IndexManager:  manager,
		CommandPolicy: sandbox.CommandPolicyFunc(func(ctx context.Context, req sandbox.CommandRequest) error { return nil }),
		FileScope:     sandbox.NewFileScopePolicy(tmpDir, nil),
	}
	return env, paths
}

func TestPhase12Descriptors(t *testing.T) {
	tests := []struct {
		name string
		desc core.CapabilityDescriptor
	}{
		{"targeted_refactor", NewTargetedRefactorHandler(agentenv.WorkspaceEnvironment{}).Descriptor(context.Background(), nil)},
		{"rename_symbol", NewRenameSymbolHandler(agentenv.WorkspaceEnvironment{}).Descriptor(context.Background(), nil)},
		{"api_compat", NewAPICompatHandler(agentenv.WorkspaceEnvironment{}).Descriptor(context.Background(), nil)},
		{"boundary_report", NewBoundaryReportHandler(agentenv.WorkspaceEnvironment{}).Descriptor(context.Background(), nil)},
		{"coverage_check", NewCoverageCheckHandler(agentenv.WorkspaceEnvironment{}).Descriptor(context.Background(), nil)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, core.CapabilityKindTool, tc.desc.Kind)
			require.Equal(t, core.CapabilityRuntimeFamilyRelurpic, tc.desc.RuntimeFamily)
		})
	}
}

func TestRegisterAllIncludesTier2Handlers(t *testing.T) {
	env := agentenv.WorkspaceEnvironment{Registry: capability.NewCapabilityRegistry()}
	require.NoError(t, RegisterAll(env))
	for _, id := range []string{
		"euclo:cap.targeted_refactor",
		"euclo:cap.rename_symbol",
		"euclo:cap.api_compat",
		"euclo:cap.boundary_report",
		"euclo:cap.coverage_check",
	} {
		require.Truef(t, env.Registry.HasCapability(id), "expected %s to be registered", id)
	}
}

func TestTargetedRefactorRequiresWritePermission(t *testing.T) {
	env, paths := newPhase12IndexedEnv(t, map[string]string{
		"sample.go": "package sample\n\nfunc Hello() string {\n\treturn \"old\"\n}\n",
	})
	env.CommandPolicy = sandbox.CommandPolicyFunc(func(ctx context.Context, req sandbox.CommandRequest) error {
		return errors.New("denied")
	})
	env.FileScope = nil

	handler := NewTargetedRefactorHandler(env)
	result, err := handler.Invoke(context.Background(), nil, map[string]interface{}{
		"symbol":         "Hello",
		"transformation": "rename the greeting helper body",
		"replacement":    "func Hello() string {\n\treturn \"new\"\n}",
	})
	require.Error(t, err)
	require.NotNil(t, result)
	require.False(t, result.Success)

	content, err := os.ReadFile(paths["sample.go"])
	require.NoError(t, err)
	require.Contains(t, string(content), `return "old"`)
	require.NotContains(t, string(content), `return "new"`)
}

func TestTargetedRefactorRespectsFileScopeProtection(t *testing.T) {
	env, paths := newPhase12IndexedEnv(t, map[string]string{
		"sample.go": "package sample\n\nfunc Hello() string {\n\treturn \"old\"\n}\n",
	})
	env.CommandPolicy = sandbox.CommandPolicyFunc(func(ctx context.Context, req sandbox.CommandRequest) error { return nil })
	env.FileScope = sandbox.NewFileScopePolicy(paths["sample.go"], []string{paths["sample.go"]})

	handler := NewTargetedRefactorHandler(env)
	result, err := handler.Invoke(context.Background(), nil, map[string]interface{}{
		"symbol":         "Hello",
		"transformation": "rename the greeting helper body",
		"replacement":    "func Hello() string {\n\treturn \"new\"\n}",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.Success)

	content, err := os.ReadFile(paths["sample.go"])
	require.NoError(t, err)
	require.Contains(t, string(content), `return "old"`)
	require.NotContains(t, string(content), `return "new"`)
}

func TestRenameSymbolFindsAllOccurrences(t *testing.T) {
	env, paths := newPhase12IndexedEnv(t, map[string]string{
		"a.go": "package sample\n\nfunc Hello() {}\n",
		"b.go": "package sample\n\nfunc Hello() {}\n",
	})

	handler := NewRenameSymbolHandler(env)
	result, err := handler.Invoke(context.Background(), nil, map[string]interface{}{
		"from": "Hello",
		"to":   "World",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Success)

	for _, relPath := range []string{"a.go", "b.go"} {
		content, err := os.ReadFile(paths[relPath])
		require.NoError(t, err)
		require.Contains(t, string(content), "World")
		require.NotContains(t, string(content), "Hello")
	}
}

func TestCoverageCheckParsesOutput(t *testing.T) {
	handler := NewCoverageCheckHandler(agentenv.WorkspaceEnvironment{
		CommandRunner: &phase12RecordingRunner{
			stdout: "ok   github.com/example/foo  0.013s  coverage: 82.5% of statements\nok   github.com/example/bar  0.011s  coverage: 61.0% of statements\n",
		},
		CommandPolicy: sandbox.CommandPolicyFunc(func(ctx context.Context, req sandbox.CommandRequest) error { return nil }),
	})

	result, err := handler.Invoke(context.Background(), nil, map[string]interface{}{
		"package":   "./...",
		"threshold": 80.0,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Success)

	coverage, ok := result.Data["coverage"].(map[string]float64)
	require.True(t, ok, "coverage data has unexpected type %T", result.Data["coverage"])
	require.InDelta(t, 82.5, coverage["github.com/example/foo"], 0.001)
	require.InDelta(t, 61.0, coverage["github.com/example/bar"], 0.001)

	passed, ok := result.Data["passed"].(bool)
	require.True(t, ok)
	require.False(t, passed)
}

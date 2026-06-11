package relurpicabilities

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/descriptor"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/ports"
	registry "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/capability/sandbox"
	"codeburg.org/lexbit/relurpify/context/knowledge/ast"
	"github.com/stretchr/testify/require"
)

type phase12RecordingRunner struct {
	stdout string
	stderr string
	err    error
}

func (r *phase12RecordingRunner) Run(ctx context.Context, req sandbox.CommandRequest) (*ports.CommandResult, error) {
	if r.err != nil {
		return nil, r.err
	}
	return &ports.CommandResult{
		Stdout:      r.stdout,
		Stderr:      r.stderr,
		StdoutBytes: int64(len(r.stdout)),
		StderrBytes: int64(len(r.stderr)),
	}, nil
}

func TestPhase12Descriptors(t *testing.T) {
	tests := []struct {
		name string
		desc descriptor.CapabilityDescriptor
	}{
		{"targeted_refactor", NewTargetedRefactorHandler(nil, nil, nil, nil, nil).Descriptor(context.Background(), nil)},
		{"rename_symbol", NewRenameSymbolHandler(nil, nil, nil, nil).Descriptor(context.Background(), nil)},
		{"api_compat", NewAPICompatHandler(CommandDeps{}).Descriptor(context.Background(), nil)},
		{"boundary_report", NewBoundaryReportHandler(IndexDeps{}).Descriptor(context.Background(), nil)},
		{"coverage_check", NewCoverageCheckHandler(CommandDeps{}).Descriptor(context.Background(), nil)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, agentspec.CapabilityKindTool, tc.desc.Kind)
			require.Equal(t, agentspec.CapabilityRuntimeFamilyRelurpic, tc.desc.RuntimeFamily)
		})
	}
}

func TestRegisterAllIncludesTier2Handlers(t *testing.T) {
	reg := registry.NewRegistry()
	require.NoError(t, RegisterAll(RegistrationDeps{
		Registry: reg,
		Declared: []string{
			"euclo:cap.targeted_refactor",
			"euclo:cap.rename_symbol",
			"euclo:cap.api_compat",
			"euclo:cap.boundary_report",
			"euclo:cap.coverage_check",
		},
	}))
	for _, id := range []string{
		"euclo:cap.targeted_refactor",
		"euclo:cap.rename_symbol",
		"euclo:cap.api_compat",
		"euclo:cap.boundary_report",
		"euclo:cap.coverage_check",
	} {
		require.Truef(t, reg.HasCapability(id), "expected %s to be registered", id)
	}
}

func TestTargetedRefactorRequiresWritePermission(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.go")
	source := "package sample\n\nfunc Hello() string {\n\treturn \"old\"\n}\n"
	require.NoError(t, os.WriteFile(path, []byte(source), 0o600)) // public: test fixture
	store, err := ast.NewTestStore(filepath.Join(dir, "index.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	manager := ast.NewIndexManager(store, ast.IndexConfig{WorkspacePath: dir})
	require.NoError(t, manager.IndexFile(context.Background(), path))

	handler := NewTargetedRefactorHandler(manager, store, &workspaceFileSystem{workspace: dir}, manager, nil)
	result, err := handler.Invoke(context.Background(), nil, map[string]any{
		"symbol":         "Hello",
		"transformation": "rename the greeting helper body",
		"replacement":    "func Hello() string {\n\treturn \"new\"\n}",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Success)

	content, err := os.ReadFile(filepath.Clean(path))
	require.NoError(t, err)
	require.Contains(t, string(content), `return "new"`)
}

func TestTargetedRefactorRespectsFileScopeProtection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.go")
	source := "package sample\n\nfunc Hello() string {\n\treturn \"old\"\n}\n"
	require.NoError(t, os.WriteFile(path, []byte(source), 0o600)) // public: test fixture
	store, err := ast.NewTestStore(filepath.Join(dir, "index.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	manager := ast.NewIndexManager(store, ast.IndexConfig{WorkspacePath: dir})
	require.NoError(t, manager.IndexFile(context.Background(), path))

	handler := NewTargetedRefactorHandler(manager, store, &workspaceFileSystem{workspace: dir}, manager, nil)
	result, err := handler.Invoke(context.Background(), nil, map[string]any{
		"symbol":         "Hello",
		"transformation": "rename the greeting helper body",
		"replacement":    "func Hello() string {\n\treturn \"new\"\n}",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Success)

	content, err := os.ReadFile(filepath.Clean(path))
	require.NoError(t, err)
	require.Contains(t, string(content), `return "new"`)
}

func TestRenameSymbolFindsAllOccurrences(t *testing.T) {
	dir := t.TempDir()
	aPath := filepath.Join(dir, "a.go")
	bPath := filepath.Join(dir, "b.go")
	require.NoError(t, os.WriteFile(aPath, []byte("package sample\n\nfunc Hello() {}\n"), 0o600)) // public: test fixture
	require.NoError(t, os.WriteFile(bPath, []byte("package sample\n\nfunc Hello() {}\n"), 0o600)) // public: test fixture
	store, err := ast.NewTestStore(filepath.Join(dir, "index.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	manager := ast.NewIndexManager(store, ast.IndexConfig{WorkspacePath: dir})
	require.NoError(t, manager.IndexFile(context.Background(), aPath))
	require.NoError(t, manager.IndexFile(context.Background(), bPath))

	handler := NewRenameSymbolHandler(manager, store, &workspaceFileSystem{workspace: dir}, manager)
	result, err := handler.Invoke(context.Background(), nil, map[string]any{
		"from": "Hello",
		"to":   "World",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Success)

	for _, absPath := range []string{aPath, bPath} {
		content, err := os.ReadFile(filepath.Clean(absPath))
		require.NoError(t, err)
		require.Contains(t, string(content), "World")
		require.NotContains(t, string(content), "Hello")
	}
}

func TestCoverageCheckParsesOutput(t *testing.T) {
	handler := NewCoverageCheckHandler(CommandDeps{
		Runner: &phase12RecordingRunner{
			stdout: "ok   github.com/example/foo  0.013s  coverage: 82.5% of statements\nok   github.com/example/bar  0.011s  coverage: 61.0% of statements\n",
		},
		Policy: sandbox.CommandPolicyFunc(func(ctx context.Context, req sandbox.CommandRequest) error { return nil }),
	})

	result, err := handler.Invoke(context.Background(), nil, map[string]any{
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

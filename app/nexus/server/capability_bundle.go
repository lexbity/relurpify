package server

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"codeburg.org/lexbit/relurpify/framework/ast"
	fauthorization "codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/graphdb"
	"codeburg.org/lexbit/relurpify/framework/manifest"
	"codeburg.org/lexbit/relurpify/framework/search"
	fsandbox "codeburg.org/lexbit/relurpify/framework/sandbox"
	"codeburg.org/lexbit/relurpify/platform/contracts"
	platformfs "codeburg.org/lexbit/relurpify/platform/fs"
	platformgit "codeburg.org/lexbit/relurpify/platform/git"
	platformsearch "codeburg.org/lexbit/relurpify/platform/search"
	platformshell "codeburg.org/lexbit/relurpify/platform/shell"
)

var (
	newCapabilityRegistryFn  = capability.NewRegistry
	platformFileOperationsFn = platformfs.FileOperations
	newSimilarityToolFn      = func(workspace string) core.Tool { return &platformsearch.SimilarityTool{BasePath: workspace} }
	newSemanticSearchToolFn  = func(workspace string) core.Tool { return &platformsearch.SemanticSearchTool{BasePath: workspace} }
	newGitCommandToolFn      = func(workspace, command string, runner contracts.CommandRunner) core.Tool {
		return &platformgit.GitCommandTool{RepoPath: workspace, Command: command, Runner: runner}
	}
	platformShellCommandLineToolsFn = platformshell.CommandLineTools
	newASTSQLiteStoreFn             = ast.NewSQLiteStore
	newGraphDBFn                    = graphdb.Open
	attachASTSymbolProviderFn       = ast.AttachASTSymbolProvider
	cleanupCapabilityBundleFn       = func(store *ast.SQLiteStore, manager *ast.IndexManager) {
		if manager != nil {
			_ = manager.Close()
			return
		}
		if store != nil {
			_ = store.Close()
		}
	}
)

type sandboxCommandRunnerAdapter struct {
	inner fsandbox.CommandRunner
}

func (a sandboxCommandRunnerAdapter) Run(ctx context.Context, req contracts.CommandRequest) (string, string, error) {
	if a.inner == nil {
		return "", "", fmt.Errorf("command runner unavailable")
	}
	return a.inner.Run(ctx, fsandbox.CommandRequest{
		Workdir: req.Workdir,
		Args:    req.Args,
		Env:     req.Env,
		Input:   req.Input,
		Timeout: req.Timeout,
	})
}

// CapabilityBundle groups the runtime-scoped capability registry and the
// shared indexing/search services built alongside it.
type CapabilityBundle struct {
	Registry     *capability.Registry
	IndexManager *ast.IndexManager
	SearchEngine *search.SearchEngine
}

// CapabilityRegistryOptions carries optional manifest/runtime policies into capability construction.
type CapabilityRegistryOptions struct {
	Context           context.Context
	AgentID           string
	PermissionManager *fauthorization.PermissionManager
	AgentSpec         *core.AgentRuntimeSpec
	ProtectedPaths    []string
	InferenceEndpoint string
	InferenceModel    string
	SkipASTIndex      bool
}

// BuildBuiltinCapabilityBundle registers builtin tools plus the workspace AST/search stack.
func BuildBuiltinCapabilityBundle(workspace string, runner fsandbox.CommandRunner, opts ...CapabilityRegistryOptions) (bundle *CapabilityBundle, err error) {
	if workspace == "" {
		workspace = "."
	}
	if runner == nil {
		return nil, fmt.Errorf("command runner required")
	}
	var cfg CapabilityRegistryOptions
	if len(opts) > 0 {
		cfg = opts[0]
	}
	buildCtx := cfg.Context
	if buildCtx == nil {
		buildCtx = context.Background()
	}
	registry := newCapabilityRegistryFn()
	var store *ast.SQLiteStore
	var manager *ast.IndexManager
	defer func() {
		if err != nil {
			cleanupCapabilityBundleFn(store, manager)
		}
	}()
	if cfg.PermissionManager != nil {
		registry.UsePermissionManager(cfg.AgentID, cfg.PermissionManager)
	}
	if cfg.AgentSpec != nil {
		registry.UseAgentSpec(cfg.AgentID, cfg.AgentSpec)
	}
	if len(cfg.ProtectedPaths) > 0 {
		registry.UseSandboxScope(fsandbox.NewFileScopePolicy(workspace, cfg.ProtectedPaths))
	}
	register := func(tool core.Tool) error {
		return registry.Register(tool)
	}
	runnerAdapter := sandboxCommandRunnerAdapter{inner: runner}
	for _, tool := range platformFileOperationsFn(workspace) {
		if err := register(tool); err != nil {
			return nil, err
		}
	}
	for _, tool := range []core.Tool{
		newSimilarityToolFn(workspace),
		newSemanticSearchToolFn(workspace),
	} {
		if err := register(tool); err != nil {
			return nil, err
		}
	}
	for _, tool := range []core.Tool{
		newGitCommandToolFn(workspace, "diff", runnerAdapter),
		newGitCommandToolFn(workspace, "history", runnerAdapter),
		newGitCommandToolFn(workspace, "branch", runnerAdapter),
		newGitCommandToolFn(workspace, "commit", runnerAdapter),
		newGitCommandToolFn(workspace, "blame", runnerAdapter),
	} {
		if err := register(tool); err != nil {
			return nil, err
		}
	}
	for _, tool := range platformShellCommandLineToolsFn(workspace, runnerAdapter) {
		if err := register(tool); err != nil {
			return nil, err
		}
	}
	paths := manifest.New(workspace)
	if err := os.MkdirAll(paths.ASTIndexDir(), 0o755); err != nil {
		return nil, err
	}
	store, err = newASTSQLiteStoreFn(paths.ASTIndexDB())
	if err != nil {
		return nil, err
	}
	manager = ast.NewIndexManager(store, ast.IndexConfig{
		WorkspacePath:   workspace,
		ParallelWorkers: 4,
	})
	graphEngine, err := newGraphDBFn(graphdb.DefaultOptions(filepath.Join(paths.MemoryDir(), "graphdb")))
	if err != nil {
		return nil, err
	}
	manager.GraphDB = graphEngine
	if cfg.PermissionManager != nil {
		manager.SetPathFilter(func(path string, isDir bool) bool {
			action := core.FileSystemRead
			if isDir {
				action = core.FileSystemList
			}
			return cfg.PermissionManager.CheckFileAccess(context.Background(), cfg.AgentID, action, path) == nil
		})
	}
	attachASTSymbolProviderFn(manager, registry)
	if err := register(ast.NewASTTool(manager)); err != nil {
		return nil, err
	}
	searchEngine := search.NewSearchEngine(nil, nil)
	if cfg.SkipASTIndex {
		return &CapabilityBundle{Registry: registry, IndexManager: manager, SearchEngine: searchEngine}, nil
	}
	if err := manager.StartIndexing(buildCtx); err != nil {
		if logger := log.Default(); logger != nil {
			logger.Printf("runtime bootstrap warning: code index build incomplete: %v", err)
		}
	}
	return &CapabilityBundle{
		Registry:     registry,
		IndexManager: manager,
		SearchEngine: searchEngine,
	}, nil
}

package ayenitd

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/ast"
	fauthorization "codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/config"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/graphdb"
	"codeburg.org/lexbit/relurpify/framework/memory"
	fsandbox "codeburg.org/lexbit/relurpify/framework/sandbox"
	"codeburg.org/lexbit/relurpify/framework/search"
	platformast "codeburg.org/lexbit/relurpify/platform/ast"
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
	newGitCommandToolFn      = func(workspace, command string, runner fsandbox.CommandRunner) core.Tool {
		return &platformgit.GitCommandTool{RepoPath: workspace, Command: command, Runner: runner}
	}
	platformShellCommandLineToolsFn = platformshell.CommandLineTools
	newASTSQLiteStoreFn             = ast.NewSQLiteStore
	newGraphDBFn                    = graphdb.Open
	newCodeIndexFn                  = memory.NewCodeIndex
	buildCodeIndexFn                = func(ci *memory.CodeIndex, ctx context.Context) error { return ci.BuildIndex(ctx) }
	saveCodeIndexFn                 = func(ci *memory.CodeIndex) error { return ci.Save() }
	startIndexingFn                 = func(m *ast.IndexManager, ctx context.Context) error { return m.StartIndexing(ctx) }
	newSearchEngineFn               = search.NewSearchEngine
	attachASTSymbolProviderFn       = platformast.AttachASTSymbolProvider
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

// BuildBuiltinCapabilityBundle is extracted from runtime.BuildBuiltinCapabilityBundle.
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
		if err := registry.Register(tool); err != nil {
			return err
		}
		return nil
	}
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
		newGitCommandToolFn(workspace, "diff", runner),
		newGitCommandToolFn(workspace, "history", runner),
		newGitCommandToolFn(workspace, "branch", runner),
		newGitCommandToolFn(workspace, "commit", runner),
		newGitCommandToolFn(workspace, "blame", runner),
	} {
		if err := register(tool); err != nil {
			return nil, err
		}
	}
	for _, tool := range platformShellCommandLineToolsFn(workspace, runner) {
		if err := register(tool); err != nil {
			return nil, err
		}
	}
	paths := config.New(workspace)
	indexDir := paths.ASTIndexDir()
	if err := os.MkdirAll(indexDir, 0o755); err != nil {
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
	if err := register(platformast.NewASTTool(manager)); err != nil {
		return nil, err
	}
	codeIndex, err := newCodeIndexFn(workspace, filepath.Join(paths.MemoryDir(), "code_index.json"))
	if err != nil {
		return nil, err
	}
	if cfg.SkipASTIndex {
		searchEngine := newSearchEngineFn(nil, codeIndex)
		if searchEngine == nil {
			return nil, fmt.Errorf("search engine initialization failed")
		}
		return &CapabilityBundle{
			Registry:     registry,
			IndexManager: manager,
			SearchEngine: searchEngine,
		}, nil
	}
	if cfg.PermissionManager != nil {
		codeIndex.SetPathFilter(func(path string, isDir bool) bool {
			action := core.FileSystemRead
			if isDir {
				action = core.FileSystemList
			}
			return cfg.PermissionManager.CheckFileAccess(context.Background(), cfg.AgentID, action, path) == nil
		})
	}
	if err := buildCodeIndexFn(codeIndex, buildCtx); err != nil {
		if !shouldIgnoreBootstrapIndexError(err) {
			return nil, err
		}
		log.Printf("runtime bootstrap warning: code index build incomplete: %v", err)
	}
	if err := saveCodeIndexFn(codeIndex); err != nil {
		return nil, err
	}
	if err := startIndexingFn(manager, buildCtx); err != nil {
		return nil, err
	}
	// TODO: semantic store and embedder (omitted for brevity)
	searchEngine := newSearchEngineFn(nil, codeIndex)
	if searchEngine == nil {
		return nil, fmt.Errorf("search engine initialization failed")
	}
	return &CapabilityBundle{
		Registry:     registry,
		IndexManager: manager,
		SearchEngine: searchEngine,
	}, nil
}

func shouldIgnoreBootstrapIndexError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "no parser for ")
}

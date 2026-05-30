package services

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/ast"
	fauthorization "codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/cfgload"
	"codeburg.org/lexbit/relurpify/framework/graphdb"
	fsandbox "codeburg.org/lexbit/relurpify/framework/sandbox"
	"codeburg.org/lexbit/relurpify/framework/search"
	"codeburg.org/lexbit/relurpify/platform/contracts"
	platformfs "codeburg.org/lexbit/relurpify/platform/fs"
	platformgit "codeburg.org/lexbit/relurpify/platform/git"
	platformsearch "codeburg.org/lexbit/relurpify/platform/search"
	platformshell "codeburg.org/lexbit/relurpify/platform/shell"
)

var (
	newCapabilityRegistryFn  = capability.NewRegistry
	platformFileOperationsFn = platformfs.FileOperations
	newSimilarityToolFn      = func(workspace string) contracts.Tool { return &platformsearch.SimilarityTool{BasePath: workspace} }
	newSemanticSearchToolFn  = func(workspace string) contracts.Tool { return &platformsearch.SemanticSearchTool{BasePath: workspace} }
	newGitCommandToolFn      = func(workspace, command string, runner contracts.CommandRunner) contracts.Tool {
		return &platformgit.GitCommandTool{RepoPath: workspace, Command: command, Runner: runner}
	}
	platformShellCommandLineToolsFn = platformshell.CommandLineTools
	newASTSQLiteStoreFn             = ast.NewSQLiteStore
	newGraphDBFn                    = graphdb.Open
	startIndexingFn                 = func(m *ast.IndexManager, ctx context.Context) error { return m.StartIndexing(ctx) }
	newSearchEngineFn               = search.NewSearchEngine
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
	AgentSpec         *agentspec.AgentRuntimeSpec
	ProtectedPaths    []string
	InferenceEndpoint string
	InferenceModel    string
	SkipASTIndex      bool
}

// BuildBuiltinCapabilityBundle constructs a complete capability bundle with platform tools and AST indexing.
// The runner must be an *fsandbox.AuthorizedRunner — a verified, policy-wrapped runner.
// Passing a bare CommandRunner is a compile error.
func BuildBuiltinCapabilityBundle(workspace string, runner *fsandbox.AuthorizedRunner, opts ...CapabilityRegistryOptions) (bundle *CapabilityBundle, err error) {
	if workspace == "" {
		workspace = "."
	}
	if runner == nil {
		return nil, fmt.Errorf("authorized command runner required")
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
	platformCfg, err := cfgload.LoadPlatformConfig(workspace)
	if err != nil {
		return nil, err
	}
	toolManifests := platformCfg.ToolManifests
	toolRegistry := platformCfg.ToolRegistry
	if toolRegistry == nil {
		return nil, fmt.Errorf("platform tool registry missing")
	}
	registry.UseToolAdmission(capability.NewToolAdmissionPolicy(toolManifests))
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
	register := func(tool contracts.Tool) error {
		if err := registry.Register(tool); err != nil {
			return err
		}
		return nil
	}
	available := make(map[string]contracts.Tool)
	addTools := func(tools ...contracts.Tool) {
		for _, tool := range tools {
			if tool == nil {
				continue
			}
			available[contracts.NormalizeToolName(tool.Name())] = tool
		}
	}
	addTools(platformFileOperationsFn(workspace)...)
	addTools(newSimilarityToolFn(workspace), newSemanticSearchToolFn(workspace))
	addTools(
		newGitCommandToolFn(workspace, "diff", commandRunnerAdapter{runner: runner}),
		newGitCommandToolFn(workspace, "history", commandRunnerAdapter{runner: runner}),
		newGitCommandToolFn(workspace, "branch", commandRunnerAdapter{runner: runner}),
		newGitCommandToolFn(workspace, "commit", commandRunnerAdapter{runner: runner}),
		newGitCommandToolFn(workspace, "blame", commandRunnerAdapter{runner: runner}),
	)
	addTools(platformShellCommandLineToolsFn(workspace, commandRunnerAdapter{runner: runner}, toolRegistry)...)
	paths := cfgload.New(workspace)
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
	fileScope := fsandbox.NewFileScopePolicy(workspace, cfg.ProtectedPaths)
	manager.SetFileScope(fileScope)
	manager.SetPathFilter(func(path string, isDir bool) bool {
		action := contracts.FileSystemRead
		if isDir {
			action = contracts.FileSystemList
		}
		if fileScope.Check(action, path) != nil {
			return false
		}
		if cfg.PermissionManager == nil {
			return true
		}
		return cfg.PermissionManager.CheckFileAccess(context.Background(), cfg.AgentID, action, path) == nil
	})
	attachASTSymbolProviderFn(manager, registry)
	addTools(ast.NewASTTool(manager))
	declared := make(map[string]struct{}, len(toolManifests))
	for _, manifest := range toolManifests {
		declared[contracts.NormalizeToolName(manifest.Name)] = struct{}{}
	}
	for name := range declared {
		if _, ok := available[name]; !ok {
			log.Printf("tool admission warning: manifest %q has no registered Go implementation", name)
		}
	}
	for _, tool := range available {
		if err := register(tool); err != nil {
			return nil, err
		}
	}
	if err := startIndexingFn(manager, buildCtx); err != nil {
		if !shouldIgnoreBootstrapIndexError(err) {
			return nil, err
		}
		log.Printf("runtime bootstrap warning: AST index build incomplete: %v", err)
	}
	// TODO: semantic store and embedder (omitted for brevity)
	searchEngine := newSearchEngineFn(nil, nil)
	if searchEngine == nil {
		return nil, fmt.Errorf("search engine initialization failed")
	}
	return &CapabilityBundle{
		Registry:     registry,
		IndexManager: manager,
		SearchEngine: searchEngine,
	}, nil
}

// BuildMinimalToolRegistry constructs a capability registry with all shell CLI
// tools registered. Unlike BuildBuiltinCapabilityBundle, it does not set up
// AST indexing, search, or git tools — only the CLI tool wrappers. This is
// suitable for the tool-exec CLI command which needs a throwaway registry.
//
// This function exists to keep the app layer from importing platform/shell
// directly (layer violation).
func BuildMinimalToolRegistry(workspace string, runner fsandbox.CommandRunner) (*capability.Registry, error) {
	capReg := newCapabilityRegistryFn()
	tools := platformShellCommandLineToolsFn(workspace, commandRunnerAdapter{runner: runner}, nil)
	for _, tool := range tools {
		if err := capReg.Register(tool); err != nil {
			return nil, fmt.Errorf("register tool %s: %w", tool.Name(), err)
		}
	}
	return capReg, nil
}

type commandRunnerAdapter struct {
	runner fsandbox.CommandRunner
}

func (a commandRunnerAdapter) Run(ctx context.Context, req contracts.CommandRequest) (*contracts.CommandResult, error) {
	if a.runner == nil {
		return nil, fmt.Errorf("command runner missing")
	}
	return a.runner.Run(ctx, fsandbox.CommandRequest{
		Workdir: req.Workdir,
		Args:    append([]string(nil), req.Args...),
		Env:     append([]string(nil), req.Env...),
		Input:   req.Input,
		Timeout: req.Timeout,
	})
}

func shouldIgnoreBootstrapIndexError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "no parser for ")
}

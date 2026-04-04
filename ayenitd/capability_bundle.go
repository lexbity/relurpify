package ayenitd

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/lexcodex/relurpify/framework/ast"
	fauthorization "github.com/lexcodex/relurpify/framework/authorization"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/config"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graphdb"
	"github.com/lexcodex/relurpify/framework/memory"
	fsandbox "github.com/lexcodex/relurpify/framework/sandbox"
	"github.com/lexcodex/relurpify/framework/search"
	platformast "github.com/lexcodex/relurpify/platform/ast"
	platformfs "github.com/lexcodex/relurpify/platform/fs"
	platformgit "github.com/lexcodex/relurpify/platform/git"
	platformsearch "github.com/lexcodex/relurpify/platform/search"
	platformshell "github.com/lexcodex/relurpify/platform/shell"
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
	OllamaEndpoint    string
	OllamaModel       string
	SkipASTIndex      bool
}

// BuildBuiltinCapabilityBundle is extracted from runtime.BuildBuiltinCapabilityBundle.
func BuildBuiltinCapabilityBundle(workspace string, runner fsandbox.CommandRunner, opts ...CapabilityRegistryOptions) (*CapabilityBundle, error) {
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
	registry := capability.NewRegistry()
	if cfg.PermissionManager != nil {
		registry.UsePermissionManager(cfg.AgentID, cfg.PermissionManager)
	}
	if cfg.AgentSpec != nil {
		registry.UseAgentSpec(cfg.AgentID, cfg.AgentSpec)
	}
	register := func(tool core.Tool) error {
		if err := registry.Register(tool); err != nil {
			return err
		}
		return nil
	}
	for _, tool := range platformfs.FileOperations(workspace) {
		if err := register(tool); err != nil {
			return nil, err
		}
	}
	for _, tool := range []core.Tool{
		&platformsearch.SimilarityTool{BasePath: workspace},
		&platformsearch.SemanticSearchTool{BasePath: workspace},
	} {
		if err := register(tool); err != nil {
			return nil, err
		}
	}
	for _, tool := range []core.Tool{
		&platformgit.GitCommandTool{RepoPath: workspace, Command: "diff", Runner: runner},
		&platformgit.GitCommandTool{RepoPath: workspace, Command: "history", Runner: runner},
		&platformgit.GitCommandTool{RepoPath: workspace, Command: "branch", Runner: runner},
		&platformgit.GitCommandTool{RepoPath: workspace, Command: "commit", Runner: runner},
		&platformgit.GitCommandTool{RepoPath: workspace, Command: "blame", Runner: runner},
	} {
		if err := register(tool); err != nil {
			return nil, err
		}
	}
	for _, tool := range platformshell.CommandLineTools(workspace, runner) {
		if err := register(tool); err != nil {
			return nil, err
		}
	}
	paths := config.New(workspace)
	indexDir := paths.ASTIndexDir()
	if err := os.MkdirAll(indexDir, 0o755); err != nil {
		return nil, err
	}
	store, err := ast.NewSQLiteStore(paths.ASTIndexDB())
	if err != nil {
		return nil, err
	}
	manager := ast.NewIndexManager(store, ast.IndexConfig{
		WorkspacePath:   workspace,
		ParallelWorkers: 4,
	})
	graphEngine, err := graphdb.Open(graphdb.DefaultOptions(filepath.Join(paths.MemoryDir(), "graphdb")))
	if err != nil {
		_ = store.Close()
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
	platformast.AttachASTSymbolProvider(manager, registry)
	if err := register(platformast.NewASTTool(manager)); err != nil {
		return nil, err
	}
	codeIndex, err := memory.NewCodeIndex(workspace, filepath.Join(paths.MemoryDir(), "code_index.json"))
	if err != nil {
		return nil, err
	}
	if cfg.SkipASTIndex {
		searchEngine := search.NewSearchEngine(nil, codeIndex)
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
	if err := codeIndex.BuildIndex(buildCtx); err != nil {
		if !shouldIgnoreBootstrapIndexError(err) {
			return nil, err
		}
		log.Printf("runtime bootstrap warning: code index build incomplete: %v", err)
	}
	if err := codeIndex.Save(); err != nil {
		return nil, err
	}
	if err := manager.StartIndexing(buildCtx); err != nil {
		return nil, err
	}
	// TODO: semantic store and embedder (omitted for brevity)
	searchEngine := search.NewSearchEngine(nil, codeIndex)
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

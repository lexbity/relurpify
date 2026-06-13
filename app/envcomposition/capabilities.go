package envcomposition

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/ports"
	regpkg "codeburg.org/lexbit/relurpify/capability/registry"
	fsandbox "codeburg.org/lexbit/relurpify/capability/sandbox"
	"codeburg.org/lexbit/relurpify/capability/toolcapabilities"
	"codeburg.org/lexbit/relurpify/context/knowledge/ast"
	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
	"codeburg.org/lexbit/relurpify/context/knowledge/search"
	"codeburg.org/lexbit/relurpify/governance/permissions"
	"codeburg.org/lexbit/relurpify/platform/fs"
	platformsearch "codeburg.org/lexbit/relurpify/platform/search"
	"codeburg.org/lexbit/relurpify/platform/tools/composite"
	"codeburg.org/lexbit/relurpify/platform/tools/subprocess"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

var (
	newCapabilityRegistryFn   = regpkg.NewRegistry
	newASTIndexStoreFn        = func(g *graphdb.Engine) ast.IndexStore { return ast.NewGraphIndexStore(g) }
	newGraphDBFn              = graphdb.Open
	startIndexingFn           = func(m *ast.IndexManager, ctx context.Context) error { return m.StartIndexing(ctx) }
	newSearchEngineFn         = search.NewSearchEngine
	attachASTSymbolProviderFn = ast.AttachASTSymbolProvider
	cleanupCapabilityBundleFn = func(ctx context.Context, g *graphdb.Engine, manager *ast.IndexManager) {
		if manager != nil {
			_ = manager.Close(ctx)
		}
		if g != nil {
			_ = g.Close(ctx)
		}
	}
)

// CapabilityRuntime groups the runtime-scoped capability registry and the
// shared indexing/search services built alongside it.
type CapabilityRuntime struct {
	Registry     *regpkg.CapabilityRegistry
	IndexManager *ast.IndexManager
	SearchEngine *search.SearchEngine
}

// CapabilityRuntimeOptions carries optional manifest/runtime policies into capability construction.
type CapabilityRuntimeOptions struct {
	Context           context.Context
	AgentID           string
	PermissionManager PermissionManager
	AgentSpec         *agentspec.AgentRuntimeSpec
	ProtectedPaths    []string
	InferenceEndpoint string
	InferenceModel    string
	SkipASTIndex      bool
}

// PermissionManager is the permission surface consumed during capability construction.
type PermissionManager interface {
	regpkg.PermissionManagerHandle
	CheckFileAccess(context.Context, string, permissions.FileSystemAction, string) error
	SetEventLogger(func(context.Context, permissions.PermissionDescriptor, string, string, map[string]any))
	DefaultPolicy() string
}

// BuildCapabilityRuntime constructs a complete capability runtime with platform tools and AST indexing.
// The runner must be an *fsandbox.AuthorizedRunner — a verified, policy-wrapped runner.
// Passing a bare CommandRunner is a compile error.
func BuildCapabilityRuntime(ctx context.Context, workspace string, runner *fsandbox.AuthorizedRunner, opts ...CapabilityRuntimeOptions) (runtime *CapabilityRuntime, err error) {
	if workspace == "" {
		workspace = "."
	}
	if runner == nil {
		return nil, fmt.Errorf("authorized command runner required")
	}
	var cfg CapabilityRuntimeOptions
	if len(opts) > 0 {
		cfg = opts[0]
	}
	registry := newCapabilityRegistryFn()
	var astEngine *graphdb.Engine
	var manager *ast.IndexManager
	platformCfg, err := config.LoadPlatformConfig(workspace)
	if err != nil {
		return nil, err
	}
	toolManifests := convertToolManifests(platformCfg.ToolManifests)
	registry.UseToolAdmission(regpkg.NewToolAdmissionPolicy(toolManifests))
	defer func() {
		if err != nil {
			cleanupCapabilityBundleFn(ctx, astEngine, manager)
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
	manifestTools := toolcapabilities.Build(workspace, commandRunnerAdapter{runner: runner}, toolManifests,
		toolcapabilities.StrictMode(),
		toolcapabilities.WithBackendBuilder("subprocess", subprocess.BackendBuilder()),
		toolcapabilities.WithBackendBuilder("composite", composite.BackendBuilder()),
	)

	for _, tool := range manifestTools {
		if setter, ok := tool.(interface{ SetCommandRunner(ports.CommandRunner) }); ok {
			setter.SetCommandRunner(commandRunnerAdapter{runner: runner})
		}
	}

	register := func(ctx context.Context, tool ports.Tool) error {
		if err := registry.Register(ctx, tool); err != nil {
			return err
		}
		return nil
	}
	available := make(map[string]ports.Tool)
	addTools := func(tools ...ports.Tool) {
		for _, tool := range tools {
			if tool == nil {
				continue
			}
			available[toolcapabilities.NormalizeToolName(tool.Name())] = tool
		}
	}
	addTools(manifestTools...)

	addTools(
		&platformsearch.SimilarityTool{BasePath: workspace},
		&platformsearch.SemanticSearchTool{BasePath: workspace},
	)

	paths := config.New(workspace)
	indexDir := paths.ASTIndexDir()
	if err := os.MkdirAll(indexDir, fs.PublicDirMode); err != nil { // public: AST index dir
		return nil, err
	}

	// Create a Badger‑backed graphdb engine for AST index storage.
	astEngine, err = newGraphDBFn(ctx, graphdb.DefaultOptions(indexDir))
	if err != nil {
		return nil, err
	}
	store := newASTIndexStoreFn(astEngine)
	manager = ast.NewIndexManager(store, ast.IndexConfig{
		WorkspacePath:   workspace,
		ParallelWorkers: 4,
	})
	manager.GraphDB = astEngine
	fileScope := fsandbox.NewFileScopePolicy(workspace, cfg.ProtectedPaths)
	manager.SetFileScope(fileScope)
	manager.SetPathFilter(func(path string, isDir bool) bool {
		action := permissions.FileSystemRead
		if isDir {
			action = permissions.FileSystemList
		}
		if fileScope.Check(action, path) != nil {
			return false
		}
		if cfg.PermissionManager == nil {
			return true
		}
		return cfg.PermissionManager.CheckFileAccess(ctx, cfg.AgentID, action, path) == nil
	})
	attachASTSymbolProviderFn(manager, registry)
	addTools(ast.NewASTTool(manager))
	for _, tool := range available {
		if err := register(ctx, tool); err != nil {
			return nil, err
		}
	}
	if err := startIndexingFn(manager, ctx); err != nil {
		if !shouldIgnoreBootstrapIndexError(err) {
			return nil, err
		}
		log.Printf("runtime bootstrap warning: AST index build incomplete: %v", err)
	}
	searchEngine := newSearchEngineFn(nil, nil)
	if searchEngine == nil {
		return nil, fmt.Errorf("search engine initialization failed")
	}
	return &CapabilityRuntime{
		Registry:     registry,
		IndexManager: manager,
		SearchEngine: searchEngine,
	}, nil
}

// BuildMinimalToolRegistry constructs a capability registry with all shell CLI
// tools registered. Unlike BuildCapabilityRuntime, it does not set up
// AST indexing, search, or git tools — only the CLI tool wrappers. This is
// suitable for the tool-exec CLI command which needs a throwaway registry.
func BuildMinimalToolRegistry(ctx context.Context, workspace string, runner fsandbox.CommandRunner) (*regpkg.CapabilityRegistry, error) {
	capReg := newCapabilityRegistryFn()

	manifestDir := config.DefaultToolManifestDir(workspace)
	manifests, err := config.LoadToolManifests(manifestDir)
	if err != nil {
		return nil, fmt.Errorf("load tool manifests: %w", err)
	}
	tools := toolcapabilities.Build(workspace, commandRunnerAdapter{runner: runner}, convertToolManifests(manifests),
		toolcapabilities.StrictMode(),
		toolcapabilities.WithBackendBuilder("subprocess", subprocess.BackendBuilder()),
		toolcapabilities.WithBackendBuilder("composite", composite.BackendBuilder()),
	)

	for _, tool := range tools {
		if setter, ok := tool.(interface{ SetCommandRunner(ports.CommandRunner) }); ok {
			setter.SetCommandRunner(commandRunnerAdapter{runner: runner})
		}
	}

	for _, tool := range tools {
		if err := capReg.Register(ctx, tool); err != nil {
			return nil, fmt.Errorf("register tool %s: %w", tool.Name(), err)
		}
	}
	return capReg, nil
}

func convertToolManifests(in []*config.ToolManifest) []*ports.ToolManifest {
	if len(in) == 0 {
		return nil
	}
	out := make([]*ports.ToolManifest, 0, len(in))
	for _, manifest := range in {
		if manifest == nil {
			continue
		}
		out = append(out, &ports.ToolManifest{
			Name:          manifest.Name,
			Version:       manifest.Version,
			Family:        manifest.Family,
			Intent:        append([]string(nil), manifest.Intent...),
			Description:   manifest.Description,
			Guidance:      convertToolManifestGuidance(manifest.Guidance),
			Parameters:    convertToolParameters(manifest.Parameters),
			Execution:     convertToolManifestExecution(manifest.Execution),
			Returns:       convertToolManifestReturns(manifest.Returns),
			Errors:        copyStringMap(manifest.Errors),
			Capability:    convertToolManifestCapability(manifest.Capability),
			RateLimit:     convertToolRateLimit(manifest.RateLimit),
			Composition:   convertToolManifestComposition(manifest.Composition),
			SourcePath:    manifest.SourcePath,
			CanonicalName: manifest.CanonicalName,
		})
	}
	return out
}

// ToPortsToolManifest converts the local userconfig tool DTO into the ports-layer tool manifest.
func ToPortsToolManifest(in config.ToolManifest) ports.ToolManifest {
	return ports.ToolManifest{
		Name:          in.Name,
		Version:       in.Version,
		Family:        in.Family,
		Intent:        append([]string(nil), in.Intent...),
		Description:   in.Description,
		Guidance:      convertToolManifestGuidance(in.Guidance),
		Parameters:    convertToolParameters(in.Parameters),
		Execution:     convertToolManifestExecution(in.Execution),
		Returns:       convertToolManifestReturns(in.Returns),
		Errors:        copyStringMap(in.Errors),
		Capability:    convertToolManifestCapability(in.Capability),
		RateLimit:     convertToolRateLimit(in.RateLimit),
		Composition:   convertToolManifestComposition(in.Composition),
		SourcePath:    in.SourcePath,
		CanonicalName: in.CanonicalName,
	}
}

func convertToolParameters(in []config.ToolParameter) []ports.ToolParameter {
	if len(in) == 0 {
		return nil
	}
	out := make([]ports.ToolParameter, len(in))
	for i, param := range in {
		out[i] = ports.ToolParameter{
			Name:        param.Name,
			Type:        ports.ToolParameterType(param.Type),
			Description: param.Description,
			Required:    param.Required,
			Default:     param.Default,
		}
	}
	return out
}

func convertToolManifestExecution(in config.ToolManifestExecution) ports.ToolManifestExecution {
	return ports.ToolManifestExecution{
		Backend:          ports.ToolBackend(in.Backend),
		Implementation:   in.Implementation,
		Command:          convertToolManifestCommand(in.Command),
		PlatformVariants: convertToolManifestCommandMap(in.PlatformVariants),
		Sandbox:          convertToolManifestSandbox(in.Sandbox),
		Stdin:            in.Stdin,
		DefaultArgs:      append([]string(nil), in.DefaultArgs...),
		AllowStdin:       in.AllowStdin,
		SupportsWorkdir:  in.SupportsWorkdir,
	}
}

func convertToolManifestCommandMap(in map[string]config.ToolManifestCommand) map[string]ports.ToolManifestCommand {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]ports.ToolManifestCommand, len(in))
	for key, value := range in {
		out[key] = *convertToolManifestCommand(&value)
	}
	return out
}

func convertToolManifestCommand(in *config.ToolManifestCommand) *ports.ToolManifestCommand {
	if in == nil {
		return nil
	}
	return &ports.ToolManifestCommand{
		Base:  append([]string(nil), in.Base...),
		Args:  append([]string(nil), in.Args...),
		Flags: convertToolManifestFlags(in.Flags),
	}
}

func convertToolManifestFlags(in map[string]config.ToolManifestFlag) map[string]ports.ToolManifestFlag {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]ports.ToolManifestFlag, len(in))
	for key, value := range in {
		out[key] = ports.ToolManifestFlag{
			WhenTrue:  append([]string(nil), value.WhenTrue...),
			WhenFalse: append([]string(nil), value.WhenFalse...),
			Param:     value.Param,
			Style:     value.Style,
			Type:      value.Type,
			Repeat:    value.Repeat,
		}
	}
	return out
}

func convertToolManifestComposition(in *config.ToolManifestComposition) *ports.ToolManifestComposition {
	if in == nil {
		return nil
	}
	out := &ports.ToolManifestComposition{Steps: make([]ports.ToolManifestCompositionStep, len(in.Steps))}
	for i, step := range in.Steps {
		out.Steps[i] = ports.ToolManifestCompositionStep{
			Tool:  step.Tool,
			Args:  copyAnyMap(step.Args),
			Alias: step.Alias,
		}
	}
	return out
}

func convertToolManifestSandbox(in *config.ToolManifestSandbox) *ports.ToolManifestSandbox {
	if in == nil {
		return nil
	}
	return &ports.ToolManifestSandbox{
		AllowedRoot:    in.AllowedRoot,
		TimeoutSeconds: in.TimeoutSeconds,
		NetworkAccess:  in.NetworkAccess,
		AllowFlags:     in.AllowFlags,
		MemoryMB:       in.MemoryMB,
		PidsLimit:      in.PidsLimit,
		CPUs:           in.CPUs,
		AllowHosts:     append([]string(nil), in.AllowHosts...),
	}
}

func convertToolManifestGuidance(in *config.ToolManifestGuidance) *ports.ToolManifestGuidance {
	if in == nil {
		return nil
	}
	return &ports.ToolManifestGuidance{
		UseWhen:   append([]string(nil), in.UseWhen...),
		AvoidWhen: append([]string(nil), in.AvoidWhen...),
	}
}

func convertToolManifestReturns(in config.ToolManifestReturns) ports.ToolManifestReturns {
	return ports.ToolManifestReturns{
		Type:     in.Type,
		Chunking: convertToolManifestReturnsChunking(in.Chunking),
	}
}

func convertToolManifestReturnsChunking(in *config.ToolManifestReturnsChunking) *ports.ToolManifestReturnsChunking {
	if in == nil {
		return nil
	}
	return &ports.ToolManifestReturnsChunking{
		Mode:      in.Mode,
		ItemPath:  in.ItemPath,
		RefFields: append([]string(nil), in.RefFields...),
	}
}

func convertToolManifestCapability(in config.ToolManifestCapability) ports.ToolManifestCapability {
	return ports.ToolManifestCapability{
		TrustClass:  in.TrustClass,
		RiskClass:   append([]string(nil), in.RiskClass...),
		EffectClass: append([]string(nil), in.EffectClass...),
	}
}

func convertToolRateLimit(in *config.ToolRateLimit) *ports.ToolRateLimit {
	if in == nil {
		return nil
	}
	return &ports.ToolRateLimit{PerSecond: in.PerSecond, Burst: in.Burst}
}

func copyStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func copyAnyMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

type commandRunnerAdapter struct {
	runner fsandbox.CommandRunner
}

func (a commandRunnerAdapter) Run(ctx context.Context, req ports.CommandRequest) (*ports.CommandResult, error) {
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

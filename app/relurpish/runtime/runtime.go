package runtime

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/agents"
	nexusdb "codeburg.org/lexbit/relurpify/app/nexus/db"
	"codeburg.org/lexbit/relurpify/ayenitd"
	"codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/agentlifecycle"
	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/ast"
	fauthorization "codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/graphdb"
	"codeburg.org/lexbit/relurpify/framework/manifest"
	"codeburg.org/lexbit/relurpify/framework/memory"

	// // memorydb "codeburg.org/lexbit/relurpify/framework/memory/db" // TODO: package does not exist // TODO: package does not exist
	fsandbox "codeburg.org/lexbit/relurpify/framework/sandbox"
	"codeburg.org/lexbit/relurpify/framework/search"
	frameworkskills "codeburg.org/lexbit/relurpify/framework/skills"
	"codeburg.org/lexbit/relurpify/framework/telemetry"
	"codeburg.org/lexbit/relurpify/platform/contracts"
	platformfs "codeburg.org/lexbit/relurpify/platform/fs"
	platformgit "codeburg.org/lexbit/relurpify/platform/git"
	"codeburg.org/lexbit/relurpify/platform/llm"
	platformsearch "codeburg.org/lexbit/relurpify/platform/search"
	platformshell "codeburg.org/lexbit/relurpify/platform/shell"
	"codeburg.org/lexbit/relurpify/relurpnet/identity"
	"codeburg.org/lexbit/relurpify/relurpnet/mcp/protocol"
)

// Runtime wires the relurpish CLI, Bubble Tea UI, and API server to the shared
// agent fruntime. It centralizes tool registration, manifests, sandbox
// registration, and log management.
type Runtime struct {
	Config               Config
	Tools                *capability.Registry
	Memory               *memory.WorkingMemoryStore
	Agent                agentgraph.WorkflowExecutor
	Model                contracts.LanguageModel
	IndexManager         *ast.IndexManager
	GraphDB              *graphdb.Engine
	SearchEngine         *search.SearchEngine
	AgentLifecycle       agentlifecycle.Repository
	Registration         *fauthorization.AgentRegistration
	Delegations          *fauthorization.DelegationManager
	AgentSpec            *agentspec.AgentRuntimeSpec
	AgentDefinitions     map[string]*agentspec.AgentDefinition
	CapabilityAdmissions []capability.AdmissionResult
	EffectiveContract    *manifest.EffectiveAgentContract
	CompiledPolicy       *manifest.CompiledPolicyBundle
	Telemetry            core.Telemetry
	Logger               *log.Logger
	Workspace            WorkspaceConfig
	Backend              llm.ManagedBackend
	ProfileResolution    llm.ProfileResolution
	ServiceManager       *ayenitd.ServiceManager
	NexusNodeProvider    core.NodeProvider
	NexusClient          *NexusClient

	logFile     io.Closer
	eventLog    io.Closer
	hitlCancel  func()
	nexusCancel func()

	serverMu     sync.Mutex
	serverCancel context.CancelFunc
	providersMu  sync.Mutex
	providers    []runtimeProviderRecord
	delegationMu sync.Mutex
	delegationBG *backgroundDelegationProvider
	mcpMu        sync.Mutex
	mcpElicit    MCPElicitationHandler
}

type MCPElicitationHandler interface {
	HandleMCPElicitation(ctx context.Context, params protocol.ElicitationParams) (*protocol.ElicitationResult, error)
}

// New builds a fruntime for the TUI and status surfaces.
func New(ctx context.Context, cfg Config) (*Runtime, error) {
	if err := cfg.Normalize(); err != nil {
		return nil, err
	}

	// Load workspace YAML to get AllowedCapabilities and Nexus config before
	// calling ayenitd.Open — Open will handle model/agent-name overrides
	// internally, but AllowedCapabilities is a runtime-level concern.
	var workspaceCfg WorkspaceConfig
	var allowedCapabilities []core.CapabilitySelector
	if cfg.ConfigPath != "" {
		if loaded, err := LoadWorkspaceConfig(cfg.ConfigPath); err == nil {
			workspaceCfg = loaded
			if workspaceCfg.Provider != "" && cfg.InferenceProvider == "" {
				cfg.InferenceProvider = workspaceCfg.Provider
			}
			if workspaceCfg.Model != "" && cfg.InferenceModel == "" {
				cfg.InferenceModel = workspaceCfg.Model
			}
			if workspaceCfg.SandboxBackend != "" && cfg.SandboxBackend == "" {
				cfg.SandboxBackend = workspaceCfg.SandboxBackend
			}
			if len(workspaceCfg.Agents) > 0 && cfg.AgentName == "" {
				cfg.AgentName = workspaceCfg.Agents[0]
			}
			allowedCapabilities = append(allowedCapabilities, workspaceCfg.AllowedCapabilities...)
		}
		// Missing config file is not an error — workspace may not be initialized yet.
	}

	// Delegate all workspace initialization to ayenitd.Open().
	ws, err := ayenitd.Open(ctx, ayenitd.WorkspaceConfig{
		Workspace:                  cfg.Workspace,
		ManifestPath:               cfg.ManifestPath,
		InferenceProvider:          cfg.InferenceProvider,
		InferenceEndpoint:          cfg.InferenceEndpoint,
		InferenceModel:             cfg.InferenceModel,
		InferenceAPIKey:            cfg.InferenceAPIKey,
		InferenceNativeToolCalling: cfg.InferenceNativeToolCalling,
		ConfigPath:                 cfg.ConfigPath,
		AgentsDir:                  cfg.AgentsDir,
		AgentName:                  cfg.AgentName,
		LogPath:                    cfg.LogPath,
		TelemetryPath:              cfg.TelemetryPath,
		EventsPath:                 cfg.EventsPath,
		MemoryPath:                 cfg.MemoryPath,
		MaxIterations:              8,
		HITLTimeout:                cfg.HITLTimeout,
		AuditLimit:                 cfg.AuditLimit,
		SandboxBackend:             cfg.SandboxBackend,
		Sandbox:                    cfg.Sandbox,
		AllowedCapabilities:        allowedCapabilities,
	})
	if err != nil {
		return nil, err
	}

	// Transfer closer ownership from Workspace to Runtime so that rt.Close()
	// manages the lifecycle directly. ws.Close() is not called.
	logFile, _ := ws.StealClosers()

	env := ws.Environment
	registration := ws.Registration
	logger := ws.Logger
	baseTelemetry := ws.Telemetry
	profileResolution := ws.ProfileResolution
	if registration != nil && registration.Permissions != nil {
		var agentSpec *agentspec.AgentRuntimeSpec
		if registration.Manifest != nil {
			agentSpec = registration.Manifest.Spec.Agent
		}
		cfg.CommandPolicy = fauthorization.NewCommandAuthorizationPolicy(registration.Permissions, registration.ID, agentSpec, "runtime")
	}

	// Extend telemetry with an event log sink (uses app/nexus/db which ayenitd
	// cannot import without a cycle).
	var eventTelemetry telemetry.EventTelemetry
	var eventLogCloser io.Closer
	if cfg.EventsPath != "" {
		if err := os.MkdirAll(filepath.Dir(cfg.EventsPath), 0o755); err == nil {
			if eventLog, err := nexusdb.NewSQLiteEventLog(cfg.EventsPath); err == nil {
				eventTelemetry = telemetry.EventTelemetry{
					Log:       eventLog,
					Partition: "local",
					Actor:     identity.EventActor{Kind: "agent", ID: registration.ID, Label: cfg.AgentLabel()},
				}
				eventLogCloser = eventLog
				// Re-wire the permission event logger with full event log support.
				if registration.Permissions != nil {
					registration.Permissions.SetEventLogger(func(ctx context.Context, desc contracts.PermissionDescriptor, effect, reason string, fields map[string]interface{}) {
						payload := map[string]interface{}{
							"permission_type": desc.Type,
							"action":          desc.Action,
							"resource":        desc.Resource,
							"effect":          effect,
							"reason":          reason,
							"metadata":        fields,
						}
						if data, err := json.Marshal(payload); err == nil {
							_, _ = eventLog.Append(ctx, "local", []core.FrameworkEvent{{
								Timestamp: time.Now().UTC(),
								Type:      core.FrameworkEventPolicyEvaluated,
								Payload:   data,
								Actor:     identity.EventActor{Kind: "agent", ID: registration.ID, Label: cfg.AgentLabel()},
								Partition: "local",
							}})
						}
					})
				}
				if registration.ManifestSnapshot != nil {
					emitManifestReloadedEvent(ctx, eventLog, registration.ID, cfg.AgentLabel(), registration.ManifestSnapshot)
				}
			} else if logger != nil {
				logger.Printf("warning: failed to init event log: %v", err)
			}
		}
	}

	// Assemble the final telemetry (base + event log if available).
	var combinedTelemetry core.Telemetry
	if eventTelemetry.Log != nil {
		if mt, ok := baseTelemetry.(telemetry.MultiplexTelemetry); ok {
			mt.Sinks = append(mt.Sinks, eventTelemetry)
			combinedTelemetry = mt
		} else {
			combinedTelemetry = telemetry.MultiplexTelemetry{Sinks: []core.Telemetry{baseTelemetry, eventTelemetry}}
		}
	} else {
		combinedTelemetry = baseTelemetry
	}

	// Register relurpic capabilities (subagent-backed; cannot be done in ayenitd).
	agentEnv := agents.AgentEnvironment{
		Config:       env.Config,
		Model:        env.Model,
		Registry:     env.Registry,
		IndexManager: env.IndexManager,
		SearchEngine: env.SearchEngine,
		Memory:       env.WorkingMemory,
	}

	// Use WorkflowStore interface directly
	rt := &Runtime{
		Config:               cfg,
		Tools:                env.Registry,
		Memory:               env.WorkingMemory,
		Model:                env.Model,
		IndexManager:         env.IndexManager,
		GraphDB:              graphDBFromIndexManager(env.IndexManager),
		SearchEngine:         env.SearchEngine,
		AgentLifecycle:       env.AgentLifecycle,
		Logger:               logger,
		logFile:              logFile,
		eventLog:             eventLogCloser,
		Workspace:            workspaceCfg,
		Backend:              ws.Backend,
		ProfileResolution:    profileResolution,
		ServiceManager:       ws.ServiceManager,
		Registration:         registration,
		Delegations:          fauthorization.NewDelegationManager(),
		AgentSpec:            ws.AgentSpec,
		AgentDefinitions:     ws.AgentDefinitions,
		CapabilityAdmissions: ws.CapabilityAdmissions,
		EffectiveContract:    ws.EffectiveContract,
		CompiledPolicy:       ws.CompiledPolicy,
		Telemetry:            combinedTelemetry,
	}
	if eventTelemetry.Log != nil && registration.HITL != nil {
		ch, cancel := registration.HITL.Subscribe(32)
		rt.hitlCancel = cancel
		go func() {
			for ev := range ch {
				eventTelemetry.EmitHITLEvent(ev)
			}
		}()
	}
	rt.Delegations.SetObserver(rt.observeDelegationSnapshot)
	if err := RegisterBuiltinProviders(ctx, rt); err != nil {
		_ = rt.Close()
		return nil, fmt.Errorf("register builtin providers: %w", err)
	}
	if err := registerNexusGatewayProvider(ctx, rt); err != nil {
		_ = rt.Close()
		return nil, fmt.Errorf("register nexus gateway provider: %w", err)
	}
	if err := registerLocalNexusNodeProvider(ctx, rt); err != nil {
		_ = rt.Close()
		return nil, fmt.Errorf("register local nexus node: %w", err)
	}

	agent := instantiateAgent(cfg, agentEnv, ws.AgentDefinitions)
	rt.wireRuntimeAgentDependencies(agent)

	// Enforce the effective (post-definition) tool policies before initializing.
	if env.Config != nil && env.Config.AgentSpec != nil {
		env.Registry.UseAgentSpec(registration.ID, env.Config.AgentSpec)
	}

	rt.Agent = agent
	return rt, nil
}

// Close releases resources managed by fruntime.
func (r *Runtime) Close() error {
	if r == nil {
		return nil
	}
	var errs []error

	r.serverMu.Lock()
	cancel := r.serverCancel
	r.serverCancel = nil
	r.serverMu.Unlock()
	if cancel != nil {
		cancel()
	}

	providers := r.registeredProviders()
	for i := len(providers) - 1; i >= 0; i-- {
		if err := providers[i].Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if r.AgentLifecycle != nil {
		if err := r.AgentLifecycle.Close(); err != nil {
			errs = append(errs, err)
		}
		r.AgentLifecycle = nil
	}
	if r.Backend != nil {
		if err := r.Backend.Close(); err != nil {
			errs = append(errs, err)
		}
		r.Backend = nil
	}
	if r.IndexManager != nil {
		if err := r.IndexManager.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if r.logFile != nil {
		if err := r.logFile.Close(); err != nil {
			errs = append(errs, err)
		}
		r.logFile = nil
	}
	if r.hitlCancel != nil {
		r.hitlCancel()
		r.hitlCancel = nil
	}
	if r.nexusCancel != nil {
		r.nexusCancel()
		r.nexusCancel = nil
	}
	if r.eventLog != nil {
		if err := r.eventLog.Close(); err != nil {
			errs = append(errs, err)
		}
		r.eventLog = nil
	}
	return errors.Join(errs...)
}

// AvailableAgents lists known agent presets and definitions.
func (r *Runtime) AvailableAgents() []string {
	seen := map[string]struct{}{
		"coding":     {},
		"planner":    {},
		"react":      {},
		"reflection": {},
		"expert":     {},
	}
	if r != nil {
		for name := range r.AgentDefinitions {
			if name == "" {
				continue
			}
			seen[name] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// SwitchAgent reinitializes the runtime with a new agent preset.
func (r *Runtime) SwitchAgent(name string) error {
	if r == nil {
		return errors.New("runtime unavailable")
	}
	if name == "" {
		return errors.New("agent name required")
	}
	if r.Registration == nil || r.Registration.Manifest == nil || r.Registration.Manifest.Spec.Agent == nil {
		return errors.New("agent manifest missing")
	}
	effectiveContract, compiledPolicy, agentDefs, err := r.resolveEffectiveContractForAgent(name)
	if err != nil {
		return err
	}
	return r.applyResolvedAgentState(name, effectiveContract, compiledPolicy, agentDefs)
}

// ReloadEffectiveContract reapplies the effective contract and compiled policy
// for the currently selected agent using the same consolidated resolution path
// as startup and SwitchAgent.
func (r *Runtime) ReloadEffectiveContract() error {
	if r == nil {
		return errors.New("runtime unavailable")
	}
	name := strings.TrimSpace(r.Config.AgentName)
	if name == "" && r.Registration != nil {
		name = strings.TrimSpace(r.Registration.ID)
	}
	if name == "" {
		return errors.New("agent name required")
	}
	effectiveContract, compiledPolicy, agentDefs, err := r.resolveEffectiveContractForAgent(name)
	if err != nil {
		return err
	}
	return r.applyResolvedAgentState(name, effectiveContract, compiledPolicy, agentDefs)
}

func (r *Runtime) applyResolvedAgentState(name string, effectiveContract *manifest.EffectiveAgentContract, compiledPolicy *manifest.CompiledPolicyBundle, agentDefs map[string]*agentspec.AgentDefinition) error {
	if r == nil {
		return errors.New("runtime unavailable")
	}
	if effectiveContract == nil || effectiveContract.AgentSpec == nil {
		return errors.New("effective contract missing agent spec")
	}
	if compiledPolicy == nil || compiledPolicy.Engine == nil {
		return errors.New("compiled policy missing")
	}
	cfg := r.Config
	cfg.AgentName = name
	if effectiveContract.AgentSpec != nil && effectiveContract.AgentSpec.Model.Name != "" && effectiveContract.AgentSpec.Model.Name != cfg.InferenceModel {
		return fmt.Errorf("agent %s requires model %s; restart to switch models", name, effectiveContract.AgentSpec.Model.Name)
	}
	if err := ensureStableSkillCapabilityTopology(r.EffectiveContract, effectiveContract); err != nil {
		return err
	}
	agentCfg := &core.Config{
		Name:              cfg.AgentLabel(),
		Model:             cfg.InferenceModel,
		MaxIterations:     8,
		NativeToolCalling: effectiveContract.AgentSpec.NativeToolCallingEnabled(),
		AgentSpec:         effectiveContract.AgentSpec,
		Telemetry:         r.Telemetry,
	}
	agent := instantiateAgent(cfg, agents.AgentEnvironment{
		Model:        r.Model,
		Registry:     r.Tools,
		IndexManager: r.IndexManager,
		SearchEngine: r.SearchEngine,
		Memory:       r.Memory,
		Config:       agentCfg,
	}, agentDefs)
	if agent == nil {
		return fmt.Errorf("agent %s not available", name)
	}
	r.wireRuntimeAgentDependencies(agent)
	r.Tools.UseAgentSpec(r.Registration.ID, effectiveContract.AgentSpec)
	r.Registration.Policy = nil
	r.Agent = agent
	r.AgentSpec = effectiveContract.AgentSpec
	r.AgentDefinitions = agentDefs
	r.EffectiveContract = effectiveContract
	r.CompiledPolicy = compiledPolicy
	r.CapabilityAdmissions = nil
	r.syncSkillContextPaths(effectiveContract.SkillResults)
	r.Config.AgentName = name
	return nil
}

// CapabilityRegistryOptions carries optional manifest/runtime policies into capability construction.
type CapabilityRegistryOptions struct {
	Context           context.Context
	AgentID           string
	PermissionManager *fauthorization.PermissionManager
	AgentSpec         *agentspec.AgentRuntimeSpec
	InferenceEndpoint string
	InferenceModel    string
	SkipASTIndex      bool
}

// CapabilityBundle groups the runtime-scoped capability registry and the
// shared indexing/search services built alongside it.
type CapabilityBundle struct {
	Registry     *capability.Registry
	IndexManager *ast.IndexManager
	SearchEngine *search.SearchEngine
}

// Close releases bundle-owned resources.
func (b *CapabilityBundle) Close() error {
	if b == nil || b.IndexManager == nil {
		return nil
	}
	return b.IndexManager.Close()
}

// BuildBuiltinCapabilityBundle registers builtin tool capabilities scoped to
// the workspace without resolving a full runtime contract. It is intended for
// tests and low-level tooling that only need the builtin capability bundle.
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
	paths := manifest.New(workspace)
	registry.UseSandboxScope(fsandbox.NewFileScopePolicy(workspace, paths.GovernanceRoots(paths.ManifestFile(), paths.ConfigFile(), paths.NexusConfigFile(), paths.PolicyRulesFile(), paths.ModelProfilesDir())))
	register := func(tool contracts.Tool) error {
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
	for _, tool := range []contracts.Tool{
		&platformsearch.SimilarityTool{BasePath: workspace},
		&platformsearch.SemanticSearchTool{BasePath: workspace},
	} {
		if err := register(tool); err != nil {
			return nil, err
		}
	}
	for _, tool := range []contracts.Tool{
		&platformgit.GitCommandTool{RepoPath: workspace, Command: "diff", Runner: sandboxCommandRunnerAdapter{runner: runner}},
		&platformgit.GitCommandTool{RepoPath: workspace, Command: "history", Runner: sandboxCommandRunnerAdapter{runner: runner}},
		&platformgit.GitCommandTool{RepoPath: workspace, Command: "branch", Runner: sandboxCommandRunnerAdapter{runner: runner}},
		&platformgit.GitCommandTool{RepoPath: workspace, Command: "commit", Runner: sandboxCommandRunnerAdapter{runner: runner}},
		&platformgit.GitCommandTool{RepoPath: workspace, Command: "blame", Runner: sandboxCommandRunnerAdapter{runner: runner}},
	} {
		if err := register(tool); err != nil {
			return nil, err
		}
	}
	for _, tool := range platformshell.CommandLineTools(workspace, sandboxCommandRunnerAdapter{runner: runner}) {
		if err := register(tool); err != nil {
			return nil, err
		}
	}
	paths = manifest.New(workspace)
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
	fileScope := fsandbox.NewFileScopePolicy(workspace, paths.GovernanceRoots())
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
	ast.AttachASTSymbolProvider(manager, registry)
	if err := register(ast.NewASTTool(manager)); err != nil {
		return nil, err
	}
	if err := manager.StartIndexing(buildCtx); err != nil {
		return nil, err
	}
	searchEngine := search.NewSearchEngine(nil, nil)
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
	return errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded) ||
		strings.Contains(err.Error(), "no parser for ")
}

func toCapabilityCandidates(input []frameworkskills.SkillCapabilityCandidate) []capability.Candidate {
	out := make([]capability.Candidate, 0, len(input))
	for _, candidate := range input {
		out = append(out, capability.Candidate{
			Descriptor:      candidate.Descriptor,
			PromptHandler:   candidate.PromptHandler,
			ResourceHandler: candidate.ResourceHandler,
		})
	}
	return out
}

// LoadAgentDefinitions scans the directory for YAML files and parses them.
func LoadAgentDefinitions(dir string) (map[string]*agentspec.AgentDefinition, error) {
	defs := make(map[string]*agentspec.AgentDefinition)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() || (!strings.HasSuffix(entry.Name(), ".yaml") && !strings.HasSuffix(entry.Name(), ".yml")) {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		def, err := agentspec.LoadAgentDefinition(path)
		if err != nil {
			if errors.Is(err, agentspec.ErrNotAgentDefinition) {
				continue
			}
			return nil, fmt.Errorf("load %s: %w", entry.Name(), err)
		}
		if def.Name == "" {
			def.Name = strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		}
		defs[def.Name] = def
	}
	return defs, nil
}

// instantiateAgent picks the concrete agent implementation for the CLI preset.
func instantiateAgent(cfg Config, env agents.AgentEnvironment, defs map[string]*agentspec.AgentDefinition) agentgraph.WorkflowExecutor {
	paths := manifest.New(cfg.Workspace)
	// Check file-based definitions first
	if def, ok := defs[cfg.AgentName]; ok {
		spec := env.Config.AgentSpec
		if spec == nil {
			spec = &def.Spec
			env.Config.AgentSpec = spec
		}
		env.Config.NativeToolCalling = spec.NativeToolCallingEnabled()
		if spec.Model.Name != "" {
			env.Config.Model = spec.Model.Name
		}

		return instantiateDefinitionAgent(cfg, def, env)
	}

	workspaceEnv := agents.ToWorkspace(env)
	builder := agents.NewAgentBuilder().WithEnvironment(&workspaceEnv)
	switch cfg.AgentLabel() {
	case "planner":
		agent, _ := builder.Build("planner")
		return configureBuiltAgent(agent, paths)
	case "react":
		agent, _ := builder.Build("react")
		return configureBuiltAgent(agent, paths)
	case "reflection":
		agent, _ := builder.Build("reflection")
		return configureBuiltAgent(agent, paths)
	default:
		agent, _ := builder.Build("react")
		return configureBuiltAgent(agent, paths)
	}
}

func instantiateDefinitionAgent(cfg Config, def *agentspec.AgentDefinition, env agents.AgentEnvironment) agentgraph.WorkflowExecutor {
	paths := manifest.New(cfg.Workspace)
	spec := def.Spec
	if env.Config != nil && env.Config.AgentSpec != nil {
		spec = *env.Config.AgentSpec
	}
	workspaceEnv := agents.ToWorkspace(env)
	agent, err := agents.BuildFromSpec(&workspaceEnv, spec)
	if err != nil {
		agent, _ = agents.BuildFromSpec(&workspaceEnv, agentspec.AgentRuntimeSpec{Implementation: "react"})
	}
	return configureBuiltAgent(agent, paths)
}

func (r *Runtime) resolveEffectiveContractForAgent(name string) (*manifest.EffectiveAgentContract, *manifest.CompiledPolicyBundle, map[string]*agentspec.AgentDefinition, error) {
	agentDefs := r.AgentDefinitions
	if r.Config.AgentsDir != "" {
		loaded, err := LoadAgentDefinitions(r.Config.AgentsDir)
		if err != nil && !os.IsNotExist(err) {
			return nil, nil, nil, fmt.Errorf("load agent definitions: %w", err)
		}
		if loaded != nil {
			agentDefs = loaded
		}
	}
	effectiveContract, err := manifest.ResolveEffectiveAgentContract(r.Config.Workspace, r.Registration.Manifest, manifest.ResolveOptions{
		AgentOverlays: selectedAgentDefinitionOverlays(name, agentDefs),
	}, nil)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("resolve effective contract: %w", err)
	}
	compiledPolicy, err := manifest.BuildFromSpec(effectiveContract.AgentID, effectiveContract.AgentSpec, nil, nil)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("compile effective policy: %w", err)
	}
	return effectiveContract, compiledPolicy, agentDefs, nil
}

func ensureStableSkillCapabilityTopology(current, next *manifest.EffectiveAgentContract) error {
	return nil
}

func skillCapabilityIDSet(contract *manifest.EffectiveAgentContract) map[string]struct{} {
	_ = contract
	return nil
}

func (r *Runtime) syncSkillContextPaths(results []manifest.SkillResolution) {
	_ = r
	_ = results
}

func configureBuiltAgent(agent agentgraph.WorkflowExecutor, paths manifest.Paths) agentgraph.WorkflowExecutor {
	_ = paths
	return agent
}

func (r *Runtime) wireRuntimeAgentDependencies(agent agentgraph.WorkflowExecutor) {
	_ = r
	_ = agent
}

// RunTask executes a task against the configured agent while preserving shared
// context state for future status screens.
func (r *Runtime) RunTask(ctx context.Context, task *core.Task) (*core.Result, error) {
	if task == nil {
		return nil, errors.New("task required")
	}
	env := contextdata.NewEnvelope(task.ID, "")
	env.NodeID = "runtime"
	if task.Context != nil {
		for key, value := range task.Context {
			env.SetWorkingValue(key, value, contextdata.MemoryClassTask)
		}
	}
	if task.Metadata != nil {
		for key, value := range task.Metadata {
			env.SetWorkingValue("meta."+key, value, contextdata.MemoryClassTask)
		}
	}
	return r.Agent.Execute(ctx, task, env)
}

// ExecuteInstruction convenience helper.
func (r *Runtime) ExecuteInstruction(ctx context.Context, instruction string, taskType core.TaskType, metadata map[string]any) (*core.Result, error) {
	if taskType == "" {
		taskType = core.TaskTypeExecute
	}

	task := &core.Task{
		ID:          fmt.Sprintf("chat-%d", time.Now().UnixNano()),
		Instruction: instruction,
		Type:        string(taskType),
		Context:     metadata,
		Metadata:    metadata,
	}
	if task.Context == nil {
		task.Context = make(map[string]any)
	}
	task.Context["workspace"] = r.Config.Workspace
	return r.RunTask(ctx, task)
}

// ExecuteInstructionStream is like ExecuteInstruction but wires a streaming
// callback so the LLM emits tokens incrementally via callback as they arrive.
func (r *Runtime) ExecuteInstructionStream(ctx context.Context, instruction string, taskType core.TaskType, metadata map[string]any, callback func(string)) (*core.Result, error) {
	if metadata == nil {
		metadata = make(map[string]any)
	}
	metadata["stream_callback"] = callback
	return r.ExecuteInstruction(ctx, instruction, taskType, metadata)
}

// StartServer is a no-op stub. The inline HTTP API server was removed as part
// of the nexus gateway migration; API access now goes through the nexus server.
// The returned stop function is a no-op.
func (r *Runtime) StartServer(_ context.Context, _ string) (func(context.Context) error, error) {
	r.serverMu.Lock()
	defer r.serverMu.Unlock()
	if r.serverCancel != nil {
		return nil, errors.New("server already running")
	}
	noop := context.CancelFunc(func() {})
	r.serverCancel = noop
	stopFn := func(_ context.Context) error {
		r.serverMu.Lock()
		r.serverCancel = nil
		r.serverMu.Unlock()
		return nil
	}
	return stopFn, nil
}

// ServerRunning reports whether the HTTP server is active.
func (r *Runtime) ServerRunning() bool {
	r.serverMu.Lock()
	defer r.serverMu.Unlock()
	return r.serverCancel != nil
}

// PendingHITL exposes outstanding permission requests.
func (r *Runtime) PendingHITL() []*fauthorization.PermissionRequest {
	if r.Registration == nil || r.Registration.HITL == nil {
		return nil
	}
	return r.Registration.HITL.PendingRequests()
}

func emitManifestReloadedEvent(ctx context.Context, eventLog *nexusdb.SQLiteEventLog, agentID, label string, snapshot *manifest.AgentManifestSnapshot) {
	if eventLog == nil || snapshot == nil {
		return
	}
	payload := map[string]interface{}{
		"manifest_path": snapshot.SourcePath,
		"fingerprint":   hex.EncodeToString(snapshot.Fingerprint[:]),
		"warnings":      append([]string(nil), snapshot.Warnings...),
	}
	if data, err := json.Marshal(payload); err == nil {
		_, _ = eventLog.Append(ctx, "local", []core.FrameworkEvent{{
			Timestamp: time.Now().UTC(),
			Type:      core.FrameworkEventManifestReloaded,
			Payload:   data,
			Actor:     identity.EventActor{Kind: "agent", ID: agentID, Label: label},
			Partition: "local",
		}})
	}
}

// SubscribeHITL streams HITL lifecycle events (requested/resolved/expired).
// The returned cancel function can be called to unsubscribe.
func (r *Runtime) SubscribeHITL() (<-chan fauthorization.HITLEvent, func()) {
	if r == nil || r.Registration == nil || r.Registration.HITL == nil {
		ch := make(chan fauthorization.HITLEvent)
		close(ch)
		return ch, func() {}
	}
	return r.Registration.HITL.Subscribe(32)
}

// ApproveHITL approves a pending request with the supplied scope.
func (r *Runtime) ApproveHITL(requestID, approver string, scope fauthorization.GrantScope, duration time.Duration) error {
	if r.Registration == nil || r.Registration.HITL == nil {
		return errors.New("hitl broker unavailable")
	}
	if scope == "" {
		scope = fauthorization.GrantScopeOneTime
	}
	var expiresAt time.Time
	if duration > 0 {
		expiresAt = time.Now().Add(duration)
	}
	decision := fauthorization.PermissionDecision{
		RequestID:  requestID,
		Approved:   true,
		ApprovedBy: approver,
		Scope:      scope,
		ExpiresAt:  expiresAt,
	}
	return r.Registration.HITL.Approve(decision)
}

// DenyHITL rejects a pending request.
func (r *Runtime) DenyHITL(requestID, reason string) error {
	if r.Registration == nil || r.Registration.HITL == nil {
		return errors.New("hitl broker unavailable")
	}
	return r.Registration.HITL.Deny(requestID, reason)
}

func (r *Runtime) SetMCPElicitationHandler(handler MCPElicitationHandler) {
	if r == nil {
		return
	}
	r.mcpMu.Lock()
	defer r.mcpMu.Unlock()
	r.mcpElicit = handler
}

func (r *Runtime) handleMCPElicitation(ctx context.Context, params protocol.ElicitationParams) (*protocol.ElicitationResult, error) {
	if r == nil {
		return &protocol.ElicitationResult{Action: "decline"}, nil
	}
	r.mcpMu.Lock()
	handler := r.mcpElicit
	r.mcpMu.Unlock()
	if handler == nil {
		return &protocol.ElicitationResult{Action: "decline"}, nil
	}
	return handler.HandleMCPElicitation(ctx, params)
}

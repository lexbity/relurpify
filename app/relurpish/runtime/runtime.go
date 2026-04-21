package runtime

import (
	"context"
	"database/sql"
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
	relurpic "codeburg.org/lexbit/relurpify/agents/relurpic"
	nexusdb "codeburg.org/lexbit/relurpify/app/nexus/db"
	archaeoarch "codeburg.org/lexbit/relurpify/archaeo/archaeology"
	relurpishbindings "codeburg.org/lexbit/relurpify/archaeo/bindings/relurpish"
	archaeodomain "codeburg.org/lexbit/relurpify/archaeo/domain"
	archaeolearning "codeburg.org/lexbit/relurpify/archaeo/learning"
	archaeoprojections "codeburg.org/lexbit/relurpify/archaeo/projections"
	archaeoretrieval "codeburg.org/lexbit/relurpify/archaeo/retrieval"
	archaeotensions "codeburg.org/lexbit/relurpify/archaeo/tensions"
	"codeburg.org/lexbit/relurpify/ayenitd"
	"codeburg.org/lexbit/relurpify/framework/ast"
	fauthorization "codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/capabilityplan"
	"codeburg.org/lexbit/relurpify/framework/config"
	contractpkg "codeburg.org/lexbit/relurpify/framework/contract"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/graph"
	"codeburg.org/lexbit/relurpify/framework/graphdb"
	"codeburg.org/lexbit/relurpify/framework/guidance"
	"codeburg.org/lexbit/relurpify/framework/manifest"
	"codeburg.org/lexbit/relurpify/framework/memory"
	memorydb "codeburg.org/lexbit/relurpify/framework/memory/db"
	frameworkplan "codeburg.org/lexbit/relurpify/framework/plan"
	"codeburg.org/lexbit/relurpify/framework/policybundle"
	"codeburg.org/lexbit/relurpify/framework/retrieval"
	fsandbox "codeburg.org/lexbit/relurpify/framework/sandbox"
	"codeburg.org/lexbit/relurpify/framework/search"
	frameworkskills "codeburg.org/lexbit/relurpify/framework/skills"
	"codeburg.org/lexbit/relurpify/framework/telemetry"
	"codeburg.org/lexbit/relurpify/named/euclo"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
	platformast "codeburg.org/lexbit/relurpify/platform/ast"
	platformfs "codeburg.org/lexbit/relurpify/platform/fs"
	platformgit "codeburg.org/lexbit/relurpify/platform/git"
	"codeburg.org/lexbit/relurpify/platform/llm"
	platformsearch "codeburg.org/lexbit/relurpify/platform/search"
	platformshell "codeburg.org/lexbit/relurpify/platform/shell"
	"codeburg.org/lexbit/relurpify/relurpnet/mcp/protocol"
)

// Runtime wires the relurpish CLI, Bubble Tea UI, and API server to the shared
// agent fruntime. It centralizes tool registration, manifests, sandbox
// registration, and log management.
type Runtime struct {
	Config               Config
	Tools                *capability.Registry
	Memory               memory.MemoryStore
	Context              *core.Context
	Agent                graph.Agent
	Model                core.LanguageModel
	IndexManager         *ast.IndexManager
	GraphDB              *graphdb.Engine
	SearchEngine         *search.SearchEngine
	WorkflowStore        *memorydb.SQLiteWorkflowStateStore
	PlanStore            frameworkplan.PlanStore
	GuidanceBroker       *guidance.GuidanceBroker
	LearningBroker       *archaeolearning.Broker
	Registration         *fauthorization.AgentRegistration
	Delegations          *fauthorization.DelegationManager
	AgentSpec            *core.AgentRuntimeSpec
	AgentDefinitions     map[string]*core.AgentDefinition
	CapabilityAdmissions []capabilityplan.AdmissionResult
	EffectiveContract    *contractpkg.EffectiveAgentContract
	CompiledPolicy       *policybundle.CompiledPolicyBundle
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
		var agentSpec *core.AgentRuntimeSpec
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
					Actor:     core.EventActor{Kind: "agent", ID: registration.ID, Label: cfg.AgentLabel()},
				}
				eventLogCloser = eventLog
				// Re-wire the permission event logger with full event log support.
				if registration.Permissions != nil {
					registration.Permissions.SetEventLogger(func(ctx context.Context, desc core.PermissionDescriptor, effect, reason string, fields map[string]interface{}) {
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
								Actor:     core.EventActor{Kind: "agent", ID: registration.ID, Label: cfg.AgentLabel()},
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
	learningBroker := archaeolearning.NewBroker(0)
	agentEnv := agents.AgentEnvironment{
		Config:        env.Config,
		CommandPolicy: env.CommandPolicy,
		Model:         env.Model,
		Registry:      env.Registry,
		IndexManager:  env.IndexManager,
		SearchEngine:  env.SearchEngine,
		Memory:        env.Memory,
	}
	if err := agents.RegisterBuiltinRelurpicCapabilitiesWithOptions(
		env.Registry,
		env.Model,
		env.Config,
		agents.WithIndexManager(env.IndexManager),
		agents.WithGraphDB(graphDBFromIndexManager(env.IndexManager)),
		agents.WithRetrievalDB(env.RetrievalDB),
		agents.WithPlanStore(env.PlanStore),
		agents.WithGuidanceBroker(env.GuidanceBroker),
		agents.WithWorkflowStore(env.WorkflowStore),
	); err != nil {
		logFile.Close()
		return nil, fmt.Errorf("register relurpic capabilities: %w", err)
	}
	if err := agents.RegisterAgentCapabilities(env.Registry, agentEnv); err != nil {
		logFile.Close()
		return nil, fmt.Errorf("register agent capabilities: %w", err)
	}

	// Type-assert WorkflowStore to the concrete SQLite type for rt.WorkflowStore.
	var workflowStore *memorydb.SQLiteWorkflowStateStore
	if ws, ok := env.WorkflowStore.(*memorydb.SQLiteWorkflowStateStore); ok {
		workflowStore = ws
	}

	rt := &Runtime{
		Config:               cfg,
		Tools:                env.Registry,
		Memory:               env.Memory,
		Context:              core.NewContext(),
		Model:                env.Model,
		IndexManager:         env.IndexManager,
		GraphDB:              graphDBFromIndexManager(env.IndexManager),
		SearchEngine:         env.SearchEngine,
		WorkflowStore:        workflowStore,
		PlanStore:            env.PlanStore,
		GuidanceBroker:       env.GuidanceBroker,
		LearningBroker:       learningBroker,
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
	for _, skill := range ws.SkillResults {
		if !skill.Applied || skill.Paths.Root == "" {
			continue
		}
		rt.Context.Set(fmt.Sprintf("skill.%s.path", skill.Name), skill.Paths.Root)
	}
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

	if r.Context != nil && r.Context.Registry() != nil {
		if err := r.Context.Registry().CloseAll(); err != nil {
			errs = append(errs, err)
		}
	}
	if r.WorkflowStore != nil {
		if err := r.WorkflowStore.Close(); err != nil {
			errs = append(errs, err)
		}
		r.WorkflowStore = nil
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

// SetInteractionEmitter injects a live interaction emitter into the active euclo agent.
// Type-asserts the active agent to *euclo.Agent; sets agent.Emitter = e.
// Silent no-op if the agent is not *euclo.Agent.
func (r *Runtime) SetInteractionEmitter(e interaction.FrameEmitter) {
	if r == nil || r.Agent == nil {
		return
	}
	if eucloAgent, ok := r.Agent.(*euclo.Agent); ok {
		eucloAgent.Emitter = e
	}
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

func (r *Runtime) applyResolvedAgentState(name string, effectiveContract *contractpkg.EffectiveAgentContract, compiledPolicy *policybundle.CompiledPolicyBundle, agentDefs map[string]*core.AgentDefinition) error {
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
		InferenceEndpoint: cfg.InferenceEndpoint,
		MaxIterations:     8,
		NativeToolCalling: effectiveContract.AgentSpec.NativeToolCallingEnabled(),
		AgentSpec:         effectiveContract.AgentSpec,
		Telemetry:         r.Telemetry,
	}
	agent := instantiateAgent(cfg, agents.AgentEnvironment{
		Model:         r.Model,
		Registry:      r.Tools,
		IndexManager:  r.IndexManager,
		SearchEngine:  r.SearchEngine,
		Memory:        r.Memory,
		Config:        agentCfg,
		CommandPolicy: cfg.CommandPolicy,
	}, agentDefs)
	if agent == nil {
		return fmt.Errorf("agent %s not available", name)
	}
	r.wireRuntimeAgentDependencies(agent)
	r.Tools.UseAgentSpec(r.Registration.ID, effectiveContract.AgentSpec)
	r.Tools.SetPolicyEngine(compiledPolicy.Engine)
	r.Registration.Policy = compiledPolicy.Engine
	r.Agent = agent
	r.AgentSpec = effectiveContract.AgentSpec
	r.AgentDefinitions = agentDefs
	r.EffectiveContract = effectiveContract
	r.CompiledPolicy = compiledPolicy
	r.CapabilityAdmissions = capabilityplan.EvaluateCandidates(
		toCapabilityPlanCandidates(frameworkskills.EnumerateSkillCapabilities(effectiveContract.ResolvedSkills)),
		core.EffectiveAllowedCapabilitySelectors(effectiveContract.AgentSpec),
	)
	r.syncSkillContextPaths(effectiveContract.SkillResults)
	r.Config.AgentName = name
	return nil
}

// CapabilityRegistryOptions carries optional manifest/runtime policies into capability construction.
type CapabilityRegistryOptions struct {
	Context           context.Context
	AgentID           string
	PermissionManager *fauthorization.PermissionManager
	AgentSpec         *core.AgentRuntimeSpec
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
	paths := config.New(workspace)
	registry.UseSandboxScope(fsandbox.NewFileScopePolicy(workspace, paths.GovernanceRoots(paths.ManifestFile(), paths.ConfigFile(), paths.NexusConfigFile(), paths.PolicyRulesFile(), paths.ModelProfilesDir())))
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
	paths = config.New(workspace)
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
	if cfg.SkipASTIndex {
		// Test-only fast path: agenttests can skip AST/bootstrap indexing when
		// isolating execution behavior from slow workspace indexing and embedding.
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
	if err := manager.StartIndexing(buildCtx); err != nil {
		return nil, err
	}
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

func embedderCfgFromRuntimeConfig(cfg Config, model string) retrieval.EmbedderConfig {
	return retrieval.EmbedderConfig{
		Provider: firstNonEmpty(strings.TrimSpace(cfg.EmbeddingProvider), strings.TrimSpace(cfg.InferenceProvider)),
		Endpoint: firstNonEmpty(strings.TrimSpace(cfg.EmbeddingEndpoint), strings.TrimSpace(cfg.InferenceEndpoint)),
		Model:    firstNonEmpty(strings.TrimSpace(cfg.EmbeddingModel), strings.TrimSpace(model), strings.TrimSpace(cfg.InferenceModel)),
	}
}

func openRuntimeStores(workspace string) (*memorydb.SQLiteWorkflowStateStore, frameworkplan.PlanStore, io.Closer, error) {
	paths := config.New(workspace)
	if err := os.MkdirAll(paths.SessionsDir(), 0o755); err != nil {
		return nil, nil, nil, fmt.Errorf("create sessions directory: %w", err)
	}
	if err := os.MkdirAll(paths.MemoryDir(), 0o755); err != nil {
		return nil, nil, nil, fmt.Errorf("create memory directory: %w", err)
	}

	workflowStore, err := memorydb.NewSQLiteWorkflowStateStore(paths.WorkflowStateFile())
	if err != nil {
		return nil, nil, nil, fmt.Errorf("open workflow state store: %w", err)
	}
	planStore, err := frameworkplan.NewSQLitePlanStore(workflowStore.DB())
	if err != nil {
		_ = workflowStore.Close()
		return nil, nil, nil, fmt.Errorf("open living plan store: %w", err)
	}

	return workflowStore, planStore, nil, nil
}

func shouldIgnoreBootstrapIndexError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded) ||
		strings.Contains(err.Error(), "no parser for ")
}

func toCapabilityPlanCandidates(input []frameworkskills.SkillCapabilityCandidate) []capabilityplan.Candidate {
	out := make([]capabilityplan.Candidate, 0, len(input))
	for _, candidate := range input {
		out = append(out, capabilityplan.Candidate{
			Descriptor:      candidate.Descriptor,
			PromptHandler:   candidate.PromptHandler,
			ResourceHandler: candidate.ResourceHandler,
		})
	}
	return out
}

// LoadAgentDefinitions scans the directory for YAML files and parses them.
func LoadAgentDefinitions(dir string) (map[string]*core.AgentDefinition, error) {
	defs := make(map[string]*core.AgentDefinition)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() || (!strings.HasSuffix(entry.Name(), ".yaml") && !strings.HasSuffix(entry.Name(), ".yml")) {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		def, err := core.LoadAgentDefinition(path)
		if err != nil {
			if errors.Is(err, core.ErrNotAgentDefinition) {
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
func instantiateAgent(cfg Config, env agents.AgentEnvironment, defs map[string]*core.AgentDefinition) graph.Agent {
	paths := config.New(cfg.Workspace)
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

	builder := agents.NewAgentBuilder().WithEnvironment(&env)
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

func instantiateDefinitionAgent(cfg Config, def *core.AgentDefinition, env agents.AgentEnvironment) graph.Agent {
	paths := config.New(cfg.Workspace)
	spec := def.Spec
	if env.Config != nil && env.Config.AgentSpec != nil {
		spec = *env.Config.AgentSpec
	}
	agent, err := agents.BuildFromSpec(env, spec)
	if err != nil {
		agent, _ = agents.BuildFromSpec(env, core.AgentRuntimeSpec{Implementation: "react"})
	}
	return configureBuiltAgent(agent, paths)
}

func (r *Runtime) resolveEffectiveContractForAgent(name string) (*contractpkg.EffectiveAgentContract, *policybundle.CompiledPolicyBundle, map[string]*core.AgentDefinition, error) {
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
	effectiveContract, err := contractpkg.ResolveEffectiveAgentContract(r.Config.Workspace, r.Registration.Manifest, contractpkg.ResolveOptions{
		AgentOverlays: selectedAgentDefinitionOverlays(name, agentDefs),
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("resolve effective contract: %w", err)
	}
	compiledPolicy, err := policybundle.BuildFromSpec(effectiveContract.AgentID, effectiveContract.AgentSpec, r.Registration.Permissions)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("compile effective policy: %w", err)
	}
	return effectiveContract, compiledPolicy, agentDefs, nil
}

func ensureStableSkillCapabilityTopology(current, next *contractpkg.EffectiveAgentContract) error {
	currentIDs := skillCapabilityIDSet(current)
	nextIDs := skillCapabilityIDSet(next)
	if len(currentIDs) != len(nextIDs) {
		return fmt.Errorf("agent switch changes skill capability topology; restart required to rebuild runtime registry")
	}
	for id := range currentIDs {
		if _, ok := nextIDs[id]; !ok {
			return fmt.Errorf("agent switch changes skill capability topology; restart required to rebuild runtime registry")
		}
	}
	return nil
}

func skillCapabilityIDSet(contract *contractpkg.EffectiveAgentContract) map[string]struct{} {
	if contract == nil {
		return nil
	}
	candidates := frameworkskills.EnumerateSkillCapabilities(contract.ResolvedSkills)
	if len(candidates) == 0 {
		return nil
	}
	ids := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		if candidate.Descriptor.ID == "" {
			continue
		}
		ids[candidate.Descriptor.ID] = struct{}{}
	}
	return ids
}

func (r *Runtime) syncSkillContextPaths(results []frameworkskills.SkillResolution) {
	if r == nil || r.Context == nil {
		return
	}
	for _, skill := range results {
		if !skill.Applied {
			continue
		}
		r.Context.Set(fmt.Sprintf("skill.%s.path", skill.Name), skill.Paths.Root)
	}
}

func configureBuiltAgent(agent graph.Agent, paths config.Paths) graph.Agent {
	switch typed := agent.(type) {
	case *agents.ReActAgent:
		typed.CheckpointPath = paths.CheckpointsDir()
	case *agents.PlannerAgent:
		typed.CheckpointPath = paths.CheckpointsDir()
	case *agents.ArchitectAgent:
		typed.CheckpointPath = paths.CheckpointsDir()
		typed.WorkflowStatePath = paths.WorkflowStateFile()
	case *agents.HTNAgent:
		typed.CheckpointPath = paths.WorkflowStateFile()
	case *agents.PipelineAgent:
		typed.WorkflowStatePath = paths.WorkflowStateFile()
	}
	if reflection, ok := agent.(*agents.ReflectionAgent); ok {
		if delegate, ok := reflection.Delegate.(*agents.ReActAgent); ok {
			delegate.CheckpointPath = paths.CheckpointsDir()
		}
	}
	return agent
}

func (r *Runtime) wireRuntimeAgentDependencies(agent graph.Agent) {
	if r == nil || agent == nil {
		return
	}
	eucloAgent, ok := agent.(*euclo.Agent)
	if !ok {
		return
	}
	if eucloAgent.GraphDB == nil {
		eucloAgent.GraphDB = r.GraphDB
	}
	if eucloAgent.RetrievalDB == nil && r.WorkflowStore != nil {
		eucloAgent.RetrievalDB = r.WorkflowStore.DB()
	}
	if eucloAgent.PlanStore == nil {
		eucloAgent.PlanStore = r.PlanStore
	}
	if eucloAgent.WorkflowStore == nil {
		eucloAgent.WorkflowStore = r.WorkflowStore
	}
	if eucloAgent.GuidanceBroker == nil {
		eucloAgent.GuidanceBroker = r.GuidanceBroker
	}
	if eucloAgent.LearningBroker == nil {
		eucloAgent.LearningBroker = r.LearningBroker
	}
	if eucloAgent.DeferralPolicy.MaxBlastRadiusForDefer == 0 && len(eucloAgent.DeferralPolicy.DeferrableKinds) == 0 {
		eucloAgent.DeferralPolicy = guidance.DefaultDeferralPolicy()
	}
}

// RunTask executes a task against the configured agent while preserving shared
// context state for future status screens.
func (r *Runtime) RunTask(ctx context.Context, task *core.Task) (*core.Result, error) {
	if task == nil {
		return nil, errors.New("task required")
	}
	state := r.Context.Clone()
	state.Set("task.id", task.ID)
	state.Set("task.type", string(task.Type))
	state.Set("task.instruction", task.Instruction)
	scope := task.ID
	if scope == "" {
		scope = state.GetString("task.id")
	}
	if scope != "" {
		defer state.ClearHandleScope(scope)
	}
	if task.Context != nil {
		if source, ok := task.Context["source"]; ok {
			state.Set("task.source", fmt.Sprint(source))
		}
		if sessionKey, ok := task.Context["session_key"]; ok && strings.TrimSpace(fmt.Sprint(sessionKey)) != "" {
			normalized := strings.TrimSpace(fmt.Sprint(sessionKey))
			state.Set("session_key", normalized)
			state.Set("nexus.session_key", normalized)
		}
	}
	res, err := r.Agent.Execute(ctx, task, state)
	if err == nil {
		r.Context.Merge(state)
	}
	return res, err
}

// ExecuteInstruction convenience helper.
func (r *Runtime) ExecuteInstruction(ctx context.Context, instruction string, taskType core.TaskType, metadata map[string]any) (*core.Result, error) {
	if taskType == "" {
		taskType = core.TaskTypeCodeModification
	}

	metaStrings := make(map[string]string)
	if metadata != nil {
		for k, v := range metadata {
			if s, ok := v.(string); ok {
				metaStrings[k] = s
			}
		}
	}

	task := &core.Task{
		ID:          fmt.Sprintf("chat-%d", time.Now().UnixNano()),
		Instruction: instruction,
		Type:        taskType,
		Context:     metadata,
		Metadata:    metaStrings,
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
			Actor:     core.EventActor{Kind: "agent", ID: agentID, Label: label},
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

func (r *Runtime) PendingGuidance() []*guidance.GuidanceRequest {
	if r == nil || r.GuidanceBroker == nil {
		return nil
	}
	return r.GuidanceBroker.PendingRequests()
}

func (r *Runtime) ResolveGuidance(requestID, choiceID, freetext string) error {
	if r == nil || r.GuidanceBroker == nil {
		return errors.New("guidance broker unavailable")
	}
	return r.GuidanceBroker.Resolve(guidance.GuidanceDecision{
		RequestID: requestID,
		ChoiceID:  choiceID,
		Freetext:  freetext,
		DecidedBy: "tui",
		DecidedAt: time.Now().UTC(),
	})
}

func (r *Runtime) SubscribeGuidance() (<-chan guidance.GuidanceEvent, func()) {
	if r == nil || r.GuidanceBroker == nil {
		ch := make(chan guidance.GuidanceEvent)
		close(ch)
		return ch, func() {}
	}
	return r.GuidanceBroker.Subscribe(32)
}

func (r *Runtime) PendingLearning() []archaeolearning.Interaction {
	if r == nil || r.LearningBroker == nil {
		return nil
	}
	return r.LearningBroker.PendingInteractions()
}

func (r *Runtime) relurpishBinding() relurpishbindings.Runtime {
	if r == nil {
		return relurpishbindings.Runtime{}
	}
	return relurpishbindings.Runtime{
		WorkflowStore:  r.WorkflowStore,
		PlanStore:      r.PlanStore,
		Retrieval:      archaeoretrieval.NewSQLStore(workflowDB(r.WorkflowStore)),
		LearningBroker: r.LearningBroker,
	}
}

func workflowDB(store *memorydb.SQLiteWorkflowStateStore) *sql.DB {
	if store == nil {
		return nil
	}
	return store.DB()
}

func (r *Runtime) ActiveExploration(workspaceID string) (*archaeoarch.SessionView, error) {
	return r.relurpishBinding().ActiveExploration(context.Background(), workspaceID)
}

func (r *Runtime) ExplorationView(explorationID string) (*archaeoarch.SessionView, error) {
	return r.relurpishBinding().ExplorationView(context.Background(), explorationID)
}

func (r *Runtime) PlanVersions(workflowID string) ([]archaeodomain.VersionedLivingPlan, error) {
	return r.relurpishBinding().PlanVersions(context.Background(), workflowID)
}

func (r *Runtime) ActivePlanVersion(workflowID string) (*archaeodomain.VersionedLivingPlan, error) {
	return r.relurpishBinding().ActivePlanVersion(context.Background(), workflowID)
}

func (r *Runtime) ComparePlanVersions(workflowID string, fromVersion, toVersion int) (map[string]any, error) {
	return r.relurpishBinding().ComparePlanVersions(context.Background(), workflowID, fromVersion, toVersion)
}

func (r *Runtime) TensionsByWorkflow(workflowID string) ([]archaeodomain.Tension, error) {
	return r.relurpishBinding().TensionsByWorkflow(context.Background(), workflowID)
}

func (r *Runtime) TensionsByExploration(explorationID string) ([]archaeodomain.Tension, error) {
	return r.relurpishBinding().TensionsByExploration(context.Background(), explorationID)
}

func (r *Runtime) UpdateTensionStatus(workflowID, tensionID string, status archaeodomain.TensionStatus, commentRefs []string) (*archaeodomain.Tension, error) {
	return r.relurpishBinding().UpdateTensionStatus(context.Background(), workflowID, tensionID, status, commentRefs)
}

func (r *Runtime) TensionSummaryByWorkflow(workflowID string) (*archaeodomain.TensionSummary, error) {
	return r.relurpishBinding().TensionSummaryByWorkflow(context.Background(), workflowID)
}

func (r *Runtime) TensionSummaryByExploration(explorationID string) (*archaeodomain.TensionSummary, error) {
	return r.relurpishBinding().TensionSummaryByExploration(context.Background(), explorationID)
}

func (r *Runtime) WorkflowProjection(workflowID string) (*archaeoprojections.WorkflowReadModel, error) {
	return r.relurpishBinding().WorkflowProjection(context.Background(), workflowID)
}

func (r *Runtime) ExplorationProjection(workflowID string) (*archaeoprojections.ExplorationProjection, error) {
	return r.relurpishBinding().ExplorationProjection(context.Background(), workflowID)
}

func (r *Runtime) LearningQueueProjection(workflowID string) (*archaeoprojections.LearningQueueProjection, error) {
	return r.relurpishBinding().LearningQueueProjection(context.Background(), workflowID)
}

func (r *Runtime) ActivePlanProjection(workflowID string) (*archaeoprojections.ActivePlanProjection, error) {
	return r.relurpishBinding().ActivePlanProjection(context.Background(), workflowID)
}

func (r *Runtime) WorkflowTimeline(workflowID string) ([]archaeodomain.TimelineEvent, error) {
	return r.relurpishBinding().WorkflowTimeline(context.Background(), workflowID)
}

func (r *Runtime) SubscribeWorkflowProjection(workflowID string) (<-chan archaeoprojections.ProjectionEvent, func()) {
	return r.relurpishBinding().SubscribeWorkflowProjection(workflowID, 16)
}

func (r *Runtime) ResolveLearning(workflowID string, input archaeolearning.ResolveInput) error {
	if strings.TrimSpace(input.WorkflowID) == "" {
		input.WorkflowID = workflowID
	}
	if strings.TrimSpace(input.WorkflowID) == "" {
		return errors.New("workflow id required")
	}
	_, err := r.relurpishBinding().ResolveLearning(context.Background(), input)
	return err
}

func (r *Runtime) SubscribeLearning() (<-chan archaeolearning.Event, func()) {
	if r == nil || r.LearningBroker == nil {
		ch := make(chan archaeolearning.Event)
		close(ch)
		return ch, func() {}
	}
	return r.LearningBroker.Subscribe(32)
}

func (r *Runtime) PendingDeferrals() []guidance.EngineeringObservation {
	eucloAgent, ok := r.currentEucloAgent()
	if !ok || eucloAgent.DeferralPlan == nil {
		return nil
	}
	return eucloAgent.DeferralPlan.PendingObservations()
}

func (r *Runtime) ResolveDeferral(observationID string) error {
	eucloAgent, ok := r.currentEucloAgent()
	if !ok || eucloAgent.DeferralPlan == nil {
		return errors.New("deferral plan unavailable")
	}
	eucloAgent.DeferralPlan.ResolveObservation(observationID)
	return nil
}

// AddBlobToPlan creates a plan step from the given blob and links the blob to
// that step. If workflowID is empty the most recently created workflow is used.
func (r *Runtime) AddBlobToPlan(ctx context.Context, workflowID, blobID string) error {
	if r == nil {
		return fmt.Errorf("runtime unavailable")
	}
	workflowID, err := r.resolveWorkflowID(ctx, workflowID)
	if err != nil {
		return err
	}
	binding := r.relurpishBinding()

	// Look in tensions first.
	tensions, err := binding.TensionsByWorkflow(ctx, workflowID)
	if err == nil {
		for i := range tensions {
			t := &tensions[i]
			if t.ID == blobID {
				return r.linkTensionToPlan(ctx, workflowID, t, binding)
			}
		}
	}

	// Fall back to learning queue.
	lq, err := binding.LearningQueueProjection(ctx, workflowID)
	if err == nil && lq != nil {
		for _, item := range lq.PendingLearning {
			if item.ID == blobID {
				return r.linkLearningToPlan(ctx, workflowID, blobID, item.Title, item.Description)
			}
		}
	}

	return fmt.Errorf("blob %q not found in workflow %s", blobID, workflowID)
}

// RemoveBlobFromPlan removes the plan step that was created for the given blob
// and unlinks the blob from that step.
func (r *Runtime) RemoveBlobFromPlan(ctx context.Context, workflowID, blobID string) error {
	if r == nil || r.PlanStore == nil {
		return fmt.Errorf("runtime unavailable")
	}
	workflowID, err := r.resolveWorkflowID(ctx, workflowID)
	if err != nil {
		return err
	}
	plan, err := r.PlanStore.LoadPlanByWorkflow(ctx, workflowID)
	if err != nil || plan == nil {
		return nil // nothing to remove
	}

	stepID := "step-" + blobID
	if _, exists := plan.Steps[stepID]; !exists {
		return nil // step not in plan
	}
	delete(plan.Steps, stepID)
	newOrder := make([]string, 0, len(plan.StepOrder))
	for _, sid := range plan.StepOrder {
		if sid != stepID {
			newOrder = append(newOrder, sid)
		}
	}
	plan.StepOrder = newOrder
	plan.UpdatedAt = time.Now()
	if err := r.PlanStore.SavePlan(ctx, plan); err != nil {
		return fmt.Errorf("save plan: %w", err)
	}

	// Unlink tension if applicable.
	binding := r.relurpishBinding()
	tensions, _ := binding.TensionsByWorkflow(ctx, workflowID)
	for i := range tensions {
		t := &tensions[i]
		if t.ID == blobID {
			t.RelatedPlanStepIDs = removeStringFromSlice(t.RelatedPlanStepIDs, stepID)
			binding.TensionService().Update(ctx, t) //nolint:errcheck
			break
		}
	}
	return nil
}

func (r *Runtime) linkTensionToPlan(ctx context.Context, workflowID string, t *archaeodomain.Tension, binding relurpishbindings.Runtime) error {
	plan, err := r.loadOrCreateActivePlan(ctx, workflowID)
	if err != nil {
		return err
	}
	stepID := "step-" + t.ID
	if _, exists := plan.Steps[stepID]; exists {
		return nil // already added
	}
	now := time.Now()
	step := &frameworkplan.PlanStep{
		ID:          stepID,
		Description: t.Description,
		Scope:       append([]string(nil), t.AnchorRefs...),
		Status:      frameworkplan.PlanStepPending,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	plan.Steps[stepID] = step
	plan.StepOrder = append(plan.StepOrder, stepID)
	plan.UpdatedAt = now
	if err := r.PlanStore.SavePlan(ctx, plan); err != nil {
		return fmt.Errorf("save plan: %w", err)
	}
	t.RelatedPlanStepIDs = appendStringUnique(t.RelatedPlanStepIDs, stepID)
	binding.TensionService().Update(ctx, t) //nolint:errcheck
	return nil
}

func (r *Runtime) linkLearningToPlan(ctx context.Context, workflowID, blobID, title, description string) error {
	plan, err := r.loadOrCreateActivePlan(ctx, workflowID)
	if err != nil {
		return err
	}
	stepID := "step-" + blobID
	if _, exists := plan.Steps[stepID]; exists {
		return nil
	}
	now := time.Now()
	desc := title
	if description != "" {
		desc = description
	}
	step := &frameworkplan.PlanStep{
		ID:          stepID,
		Description: desc,
		Status:      frameworkplan.PlanStepPending,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	plan.Steps[stepID] = step
	plan.StepOrder = append(plan.StepOrder, stepID)
	plan.UpdatedAt = now
	return r.PlanStore.SavePlan(ctx, plan)
}

func (r *Runtime) loadOrCreateActivePlan(ctx context.Context, workflowID string) (*frameworkplan.LivingPlan, error) {
	if r.PlanStore == nil {
		return nil, fmt.Errorf("plan store unavailable")
	}
	plan, err := r.PlanStore.LoadPlanByWorkflow(ctx, workflowID)
	if err == nil && plan != nil {
		return plan, nil
	}
	now := time.Now()
	return &frameworkplan.LivingPlan{
		ID:         "plan-" + workflowID,
		WorkflowID: workflowID,
		Title:      "Working Plan",
		Steps:      map[string]*frameworkplan.PlanStep{},
		StepOrder:  []string{},
		Version:    1,
		CreatedAt:  now,
		UpdatedAt:  now,
	}, nil
}

func (r *Runtime) resolveWorkflowID(ctx context.Context, workflowID string) (string, error) {
	if workflowID != "" {
		return workflowID, nil
	}
	if r.WorkflowStore == nil {
		return "", fmt.Errorf("workflow store unavailable")
	}
	records, err := r.WorkflowStore.ListWorkflows(ctx, 1)
	if err != nil || len(records) == 0 {
		return "", fmt.Errorf("no active workflow")
	}
	return records[0].WorkflowID, nil
}

func appendStringUnique(slice []string, s string) []string {
	for _, v := range slice {
		if v == s {
			return slice
		}
	}
	return append(slice, s)
}

func removeStringFromSlice(slice []string, s string) []string {
	out := slice[:0]
	for _, v := range slice {
		if v != s {
			out = append(out, v)
		}
	}
	return out
}

func (r *Runtime) currentEucloAgent() (*euclo.Agent, bool) {
	if r == nil || r.Agent == nil {
		return nil, false
	}
	eucloAgent, ok := r.Agent.(*euclo.Agent)
	return eucloAgent, ok
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

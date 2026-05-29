package agentenv

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"codeburg.org/lexbit/relurpify/framework/agentlifecycle"
	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/ast"
	fauthorization "codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/cfgload"
	cfgsecurity "codeburg.org/lexbit/relurpify/framework/cfgload/security"
	"codeburg.org/lexbit/relurpify/framework/compiler"
	"codeburg.org/lexbit/relurpify/framework/contextpolicy"
	"codeburg.org/lexbit/relurpify/framework/contextstream"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/event"
	"codeburg.org/lexbit/relurpify/framework/graphdb"
	"codeburg.org/lexbit/relurpify/framework/jobs"
	"codeburg.org/lexbit/relurpify/framework/knowledge"
	"codeburg.org/lexbit/relurpify/framework/memory"
	"codeburg.org/lexbit/relurpify/framework/retrieval"
	fsandbox "codeburg.org/lexbit/relurpify/framework/sandbox"
	"codeburg.org/lexbit/relurpify/framework/search"
	"codeburg.org/lexbit/relurpify/framework/services"
	"codeburg.org/lexbit/relurpify/framework/telemetry"
	"codeburg.org/lexbit/relurpify/platform/contracts"
	"codeburg.org/lexbit/relurpify/platform/llm"
)

// Workspace is a live, initialized workspace session. It holds all open
// resources. Close() must be called when the session ends. Restart() may
// be used to cleanly stop and re-start services without rebuilding stores.
type Workspace struct {
	Environment       WorkspaceEnvironment
	Registration      *fauthorization.AgentRegistration
	Backend           llm.ManagedBackend
	ProfileResolution llm.ProfileResolution

	// Internals held for Close()/Restart()
	logFile  io.Closer
	eventLog io.Closer

	// Derived fields for callers that need them
	AgentSpec            *agentspec.AgentRuntimeSpec
	AgentDefinitions     map[string]*agentspec.AgentDefinition
	EffectiveContract    *cfgload.EffectiveAgentContract
	CompiledPolicy       *fauthorization.CompiledPolicyBundle
	PolicyEngine         fauthorization.PolicyEngine
	CapabilityAdmissions []capability.AdmissionResult
	SkillResults         []cfgload.SkillResolution

	// Observability
	Telemetry core.Telemetry
	Logger    *log.Logger

	// Service management (new for dynamic lifecycle)
	ServiceManager *ServiceManager
}

// Close releases all resources held by the Workspace. This includes:
// 1. Stopping all services via ServiceManager (clearing registry)
// 2. Closing database stores, files, and loggers
func (w *Workspace) Close() error {
	var errs []error

	// Stop all registered services first, but keep closing owned resources even
	// if service shutdown fails.
	if w.ServiceManager != nil {
		if err := w.ServiceManager.Clear(); err != nil {
			errs = append(errs, fmt.Errorf("stop services: %w", err))
		}
	}

	if w.Environment.Scheduler != nil {
		w.Environment.Scheduler.Stop()
	}

	if w.Backend != nil {
		if err := w.Backend.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close backend: %w", err))
		}
	}

	if w.eventLog != nil {
		if err := w.eventLog.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close event log: %w", err))
		}
	}

	if w.logFile != nil {
		if err := w.logFile.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close log file: %w", err))
		}
	}

	return errors.Join(errs...)
}

// Restart cleanly stops all services and immediately re-starts them. This
// is useful for "ping" the workspace or applying configuration changes
// without dropping out of Open().
func (w *Workspace) Restart(ctx context.Context) error {
	log.Printf("workspace: stopping services for restart")
	if err := w.stopServices(); err != nil {
		return fmt.Errorf("stop services for restart: %w", err)
	}
	if w.ServiceManager == nil {
		return fmt.Errorf("service manager unavailable")
	}
	log.Printf("workspace: restarting services")
	return w.ServiceManager.StartAll(ctx)
}

// GetService returns a specific service by ID if registered. Returns nil if
// not found. Useful for accessing the Scheduler or custom workers.
func (w *Workspace) GetService(id string) Service {
	if w.ServiceManager == nil {
		return nil
	}
	return w.ServiceManager.Get(id)
}

// ListServices returns a copy of all registered service IDs. Safe for concurrent
// calls since internal state is locked.
func (w *Workspace) ListServices() []string {
	if w.ServiceManager == nil {
		return nil
	}
	sm := w.ServiceManager
	sm.Mu.Lock()
	defer sm.Mu.Unlock()

	result := make([]string, 0, len(sm.Registry))
	for id := range sm.Registry {
		result = append(result, id)
	}
	return result
}

// stopServices stops all running services but does not clear the registry or close stores.
func (w *Workspace) stopServices() error {
	// Stop all registered services first
	if w.ServiceManager != nil {
		if err := w.ServiceManager.StopAll(); err != nil {
			return fmt.Errorf("stop services: %w", err)
		}
	}

	if w.Environment.Scheduler != nil {
		w.Environment.Scheduler.Stop()
	}
	return nil
}

// AgentBootstrapOptions provides configuration for agent runtime bootstrapping.
type AgentBootstrapOptions struct {
	Context             context.Context
	AgentID             string
	AgentName           string
	ConfigName          string
	AgentSpec           *agentspec.AgentRuntimeSpec
	ManifestSnapshot    *cfgload.AgentManifestSnapshot
	SecurityBundle      *cfgsecurity.Bundle
	ProfileResolution   llm.ProfileResolution
	AgentDefinitions    map[string]*agentspec.AgentDefinition
	PermissionManager   *fauthorization.PermissionManager
	Runner              fsandbox.CommandRunner
	SandboxBackend      string
	Model               contracts.LanguageModel
	Backend             llm.ManagedBackend
	InferenceModel      string
	Telemetry           core.Telemetry
	SkipASTIndex        bool
	MaxIterations       int
	AllowedCapabilities []agentspec.CapabilitySelector
	DebugLLM            bool
	DebugAgent          bool
	AgentLifecycle      agentlifecycle.Repository
}

// BootstrappedAgentRuntime contains the results of bootstrapping an agent runtime.
type BootstrappedAgentRuntime struct {
	Registry             *capability.Registry
	IndexManager         *ast.IndexManager
	SearchEngine         *search.SearchEngine
	AgentSpec            *agentspec.AgentRuntimeSpec
	AgentConfig          *core.Config
	Backend              llm.ManagedBackend
	Environment          WorkspaceEnvironment
	AgentDefinitions     map[string]*agentspec.AgentDefinition
	SkillResults         []cfgload.SkillResolution
	CapabilityAdmissions []capability.AdmissionResult
	Contract             *cfgload.EffectiveAgentContract
	CompiledPolicy       *fauthorization.CompiledPolicyBundle
	PolicyEngine         fauthorization.PolicyEngine
}

// BootstrapAgentRuntime bootstraps the agent runtime including loading agent definitions,
// resolving effective contracts, and building the capability bundle.
func BootstrapAgentRuntime(workspace string, opts AgentBootstrapOptions) (*BootstrappedAgentRuntime, error) {
	if workspace == "" {
		return nil, fmt.Errorf("workspace required")
	}
	if opts.ManifestSnapshot == nil {
		if opts.AgentSpec == nil {
			return nil, fmt.Errorf("either manifest snapshot or agent spec required")
		}
		opts.ManifestSnapshot = synthesizeManifestSnapshot(opts.AgentName, opts.AgentSpec)
	}
	if opts.ManifestSnapshot.Manifest == nil {
		return nil, fmt.Errorf("agent manifest missing")
	}
	if opts.ManifestSnapshot.Manifest.Spec.Agent == nil && opts.AgentSpec == nil {
		return nil, fmt.Errorf("agent manifest missing spec.agent configuration")
	}
	// opts.Runner may be nil — buildSecuredRuntime will build one from scratch
	// using SecurityBundle and SandboxBackend when no ExistingRunner is set.
	if opts.SecurityBundle == nil {
		return nil, fmt.Errorf("security bundle required")
	}

	agentDefs := opts.AgentDefinitions

	manifestForResolution := opts.ManifestSnapshot.Manifest
	if opts.AgentSpec != nil {
		clone := *opts.ManifestSnapshot.Manifest
		clone.Spec.Agent = opts.AgentSpec
		manifestForResolution = &clone
	}
	resolveOpts := cfgload.ResolveOptions{
		AgentOverlays: agentspec.AgentSpecOverlaysForName(opts.AgentName, agentDefs),
	}
	effectiveContract, err := cfgload.ResolveEffectiveAgentContract(workspace, manifestForResolution, resolveOpts, nil)
	if err != nil {
		return nil, err
	}
	agentSpec := effectiveContract.AgentSpec
	skillResults := append([]cfgload.SkillResolution{}, effectiveContract.SkillResults...)
	fileScope := fsandbox.NewFileScopePolicy(workspace, append([]string(nil), opts.SecurityBundle.Sandbox.ProtectedPaths...))

	resolvedModel := opts.InferenceModel
	if resolvedModel == "" {
		resolvedModel = agentSpec.Model.Name
	}

	manifest := opts.ManifestSnapshot.Manifest
	sr, err := buildSecuredRuntime(opts.Context, SecuredRuntimeInput{
		Context:            opts.Context,
		Workspace:          workspace,
		SandboxBackend:     opts.SandboxBackend,
		AgentID:            opts.AgentID,
		AgentSpec:          agentSpec,
		PermissionManager:  opts.PermissionManager,
		SecurityBundle:     opts.SecurityBundle,
		ExistingRunner:     opts.Runner,
		Manifest:           manifest,
	})
	if err != nil {
		return nil, fmt.Errorf("build secured runtime: %w", err)
	}

	capabilities, err := services.BuildBuiltinCapabilityBundle(workspace, sr.Runner, services.CapabilityRegistryOptions{
		Context:           opts.Context,
		AgentID:           opts.AgentID,
		PermissionManager: opts.PermissionManager,
		AgentSpec:         agentSpec,
		ProtectedPaths:    append([]string(nil), opts.SecurityBundle.Sandbox.ProtectedPaths...),
		SkipASTIndex:      opts.SkipASTIndex,
	})
	if err != nil {
		return nil, err
	}
	registry := capabilities.Registry
	indexManager := capabilities.IndexManager
	searchEngine := capabilities.SearchEngine
	if opts.Telemetry != nil {
		registry.UseTelemetry(opts.Telemetry)
	}
	if opts.PermissionManager != nil {
		registry.UsePermissionManager(opts.AgentID, opts.PermissionManager)
	}
	if opts.ProfileResolution.Profile != nil {
		registry.SetModelProfile(opts.ProfileResolution.Profile)
	}
	compiledPolicy, err := fauthorization.BuildFromContract(effectiveContract, sr.PolicyEngine, nil)
	if err != nil {
		return nil, fmt.Errorf("build compiled policy: %w", err)
	}
	registry.SetPolicyEngine(sr.PolicyEngine)

	maxIterations := opts.MaxIterations
	if maxIterations <= 0 {
		maxIterations = 8
	}
	configName := opts.ConfigName
	if configName == "" {
		configName = opts.ManifestSnapshot.Manifest.Metadata.Name
	}
	agentCfg := &core.Config{
		Name:              configName,
		Model:             resolvedModel,
		MaxIterations:     maxIterations,
		NativeToolCalling: agentSpec.NativeToolCallingEnabled(),
		AgentSpec:         agentSpec,
		DebugLLM:          opts.DebugLLM,
		DebugAgent:        opts.DebugAgent,
		Telemetry:         opts.Telemetry,
	}
	registry.UseAgentSpec(opts.AgentID, agentSpec)
	admissionResults, err := capability.AdmitCandidates(
		registry,
		nil,
		agentspec.EffectiveAllowedCapabilitySelectors(agentSpec),
	)
	if err != nil {
		return nil, fmt.Errorf("admit skill capabilities: %w", err)
	}

	// Create working memory store
	wm := memory.NewWorkingMemoryStore()

	env := WorkspaceEnvironment{
		Config:                        agentCfg,
		Model:                         opts.Model,
		CommandRunner:                 sr.Runner,
		JobSubmitter:                  jobs.NoopSubmitter{},
		CommandPolicy:                 sr.CommandPolicy,
		FileScope:                     fileScope,
		Registry:                      registry,
		PermissionManager:             opts.PermissionManager,
		IndexManager:                  indexManager,
		SearchEngine:                  searchEngine,
		WorkingMemory:                 wm,
		KnowledgeStore:                nil, // Will be populated in Open
		Retriever:                     nil, // Will be populated in Open
		Compiler:                      nil, // Will be populated in Open
		EventLog:                      nil,
		Scheduler:                     nil,
		ServiceManager:                nil,
		VerificationPlanner:           nil,
		CompatibilitySurfaceExtractor: nil,
	}

	return &BootstrappedAgentRuntime{
		Registry:             registry,
		IndexManager:         indexManager,
		SearchEngine:         searchEngine,
		AgentSpec:            agentSpec,
		AgentConfig:          agentCfg,
		Backend:              opts.Backend,
		Environment:          env,
		AgentDefinitions:     agentDefs,
		SkillResults:         skillResults,
		CapabilityAdmissions: admissionResults,
		Contract:             effectiveContract,
		CompiledPolicy:       compiledPolicy,
		PolicyEngine:         sr.PolicyEngine,
	}, nil
}

// OpenWorkspace initializes a complete workspace session: platform checks,
// store opening, service graph construction, agent registration, and
// background indexing. The returned *Workspace is ready for agent
// construction.
//
// OpenWorkspace is the single composition root for all Relurpify entry
// points. app/relurpish, app/dev-agent-cli, and integration tests all call
// OpenWorkspace.
//
// Feature assembly is governed by cfg.Scope. Security and capability
// assembly are unconditional; LLM backend, knowledge, services, and
// telemetry-sink construction are gated behind their respective scope flags.
// A zero-value scope defaults to ScopeFull for backward compatibility.
func OpenWorkspace(ctx context.Context, cfg WorkspaceConfig, secrets llm.ProviderSecrets, regFuncs AgentRegistrationFuncs) (*Workspace, error) {
	if cfg.Workspace == "" {
		return nil, fmt.Errorf("workspace required")
	}

	// Embedded scope (nexus) has relaxed requirements: no LoadedConfig,
	// ManifestSnapshot, or ProfileResolution needed. Full scope requires
	// the complete config tree.
	isEmbedded := cfg.Scope == ScopeEmbeddedAgent

	if !isEmbedded {
		if cfg.LoadedConfig == nil {
			return nil, fmt.Errorf("loaded config required")
		}
		if cfg.ManifestSnapshot == nil {
			return nil, fmt.Errorf("manifest snapshot required")
		}
		if cfg.ProfileResolution.Profile == nil {
			return nil, fmt.Errorf("profile resolution required")
		}
	}
	if cfg.SecurityBundle == nil {
		return nil, fmt.Errorf("security bundle required")
	}

	if !isEmbedded {
		workspaceCfg := cfg.LoadedConfig.Workspace
		if workspaceCfg.SourcePath == "" {
			return nil, fmt.Errorf("loaded workspace config missing source path")
		}
		if len(workspaceCfg.DefaultsUsed) > 0 {
			for _, usage := range workspaceCfg.DefaultsUsed {
				log.Printf("WARN config: using default value file=%s key=%s default=%v", workspaceCfg.SourcePath, usage.Key, usage.Value)
			}
		}
		if cfg.ManifestPath == "" {
			cfg.ManifestPath = cfg.ManifestSnapshot.SourcePath
		}
		cfg.StateDir = workspaceCfg.StateDir()
		if cfg.SandboxBackend == "" {
			cfg.SandboxBackend = stringValue(workspaceCfg.Sandbox.Backend)
		}
		if cfg.InferenceModel == "" {
			cfg.InferenceModel = workspaceCfg.Model.Name
		}
	} else {
		// Embedded scope defaults.
		if cfg.StateDir == "" {
			cfg.StateDir = cfgload.DefaultWorkspaceStateDir(cfg.Workspace)
		}
	}

	defaultStateDir := cfgload.DefaultWorkspaceStateDir(cfg.Workspace)
	defaultLogPath := filepath.Join(defaultStateDir, "logs", "agentenv.log")
	defaultTelemetryPath := filepath.Join(defaultStateDir, "telemetry", "agentenv.jsonl")
	defaultEventsPath := filepath.Join(defaultStateDir, "events.db")
	defaultMemoryPath := filepath.Join(defaultStateDir, "memory")
	if cfg.LogPath == "" || filepath.Clean(cfg.LogPath) == filepath.Clean(defaultLogPath) {
		cfg.LogPath = filepath.Join(cfg.StateDir, "logs", "agentenv.log")
	}
	if cfg.TelemetryPath == "" || filepath.Clean(cfg.TelemetryPath) == filepath.Clean(defaultTelemetryPath) {
		cfg.TelemetryPath = filepath.Join(cfg.StateDir, "telemetry", "agentenv.jsonl")
	}
	if cfg.EventsPath == "" || filepath.Clean(cfg.EventsPath) == filepath.Clean(defaultEventsPath) {
		cfg.EventsPath = filepath.Join(cfg.StateDir, "events.db")
	}
	if cfg.MemoryPath == "" || filepath.Clean(cfg.MemoryPath) == filepath.Clean(defaultMemoryPath) {
		cfg.MemoryPath = filepath.Join(cfg.StateDir, "memory")
	}

	if !cfg.Scope.TelemetrySinks {
		cfg.TelemetryPath = ""
	}

	// Phase A: Configuration Validation
	if err := validateConfig(cfg); err != nil {
		return nil, fmt.Errorf("invalid workspace config: %w", err)
	}

	// Phase B: LLM Backend (gated by Scope.LLMBackend)
	var backend llm.ManagedBackend
	if cfg.Scope.LLMBackend {
		var beErr error
		backend, beErr = llm.New(llm.ProviderConfigFromRuntimeConfig(cfg), secrets)
		if beErr != nil {
			return nil, fmt.Errorf("build inference backend: %w", beErr)
		}
	}

	// Phase C: Log and Telemetry Setup (logger always on; JSON sink gated by
	// cfg.Scope.TelemetrySinks, enforced via empty cfg.TelemetryPath above)
	logFile, logger, tel, err := setupTelemetry(cfg)
	if err != nil {
		return nil, err
	}

	// Phase C.5: Event Log Setup (gated by Scope.Services)
	var eventLog event.Log
	if cfg.Scope.Services && cfg.EventLogFactory != nil && cfg.EventsPath != "" {
		eventLog, err = cfg.EventLogFactory(cfg.EventsPath)
		if err != nil {
			logFile.Close()
			return nil, fmt.Errorf("create event log: %w", err)
		}
	}

	// Phase D: KnowledgeStore initialization deferred until after BootstrapAgentRuntime
	// where the graphdb.Engine is available from IndexManager.

	// Phase E: Agent Registration + Authorization
	// For embedded scope, synthesize a minimal manifest snapshot if none provided.
	if cfg.ManifestSnapshot == nil && cfg.AgentSpec != nil {
		cfg.ManifestSnapshot = synthesizeManifestSnapshot(cfg.AgentName, cfg.AgentSpec)
	}
	registration, err := fauthorization.RegisterAgent(ctx, fauthorization.RuntimeConfig{
		ManifestPath:     cfg.ManifestPath,
		ManifestSnapshot: cfg.ManifestSnapshot,
		SecurityBundle:   cfg.SecurityBundle,
		ConfigPath:       cfg.ConfigPath,
		Backend:          cfg.SandboxBackend,
		AuditLimit:       cfg.AuditLimit,
		BaseFS:           cfg.Workspace,
		StateDir:         cfg.StateDir,
		HITLTimeout:      cfg.HITLTimeout,
	})
	if err != nil {
		logFile.Close()
		return nil, fmt.Errorf("sandbox registration failed: %w", err)
	}

	// Phase F: Capability Bundle + Agent Environment
	// Build CommandRunnerConfig from manifest
	var runnerConfig *contracts.CommandRunnerConfig
	if registration.Manifest != nil {
		runnerConfig = &contracts.CommandRunnerConfig{
			Image:           registration.Manifest.Spec.Image,
			RunAsUser:       registration.Manifest.Spec.Security.RunAsUser,
			ReadOnlyRoot:    registration.Manifest.Spec.Security.ReadOnlyRoot,
			NoNewPrivileges: registration.Manifest.Spec.Security.NoNewPrivileges,
			Workspace:       cfg.Workspace,
		}
	}
	runner, err := fsandbox.NewCommandRunner(runnerConfig, registration.Runtime)
	if err != nil {
		logFile.Close()
		return nil, err
	}

	// Resolve model from manifest if not overridden in manifest.
	inferenceModel := cfg.InferenceModel
	if registration.Manifest != nil && registration.Manifest.Spec.Agent != nil {
		if specModel := registration.Manifest.Spec.Agent.Model.Name; specModel != "" && inferenceModel == "" {
			inferenceModel = specModel
		}
	}

	var model contracts.LanguageModel
	var logLLM bool
	profileResolution := cfg.ProfileResolution
	if cfg.Scope.LLMBackend && backend != nil {
		_ = llm.ApplyProfile(backend, profileResolution.Profile)

		logLLM = cfg.DebugLLM
		if registration.Manifest != nil && registration.Manifest.Spec.Agent != nil {
			if registration.Manifest.Spec.Agent.Logging != nil && registration.Manifest.Spec.Agent.Logging.LLM != nil {
				logLLM = *registration.Manifest.Spec.Agent.Logging.LLM
			}
		}
		backend.SetDebugLogging(logLLM)
		model = llm.NewInstrumentedModel(backend.Model(), llmTelemetryAdapter{inner: tel}, logLLM)
		_ = llm.ApplyProfile(model, profileResolution.Profile)
	}

	// Wire permission event logger if event telemetry is available.
	if et, ok := tel.(interface {
		EmitPermissionEvent(ctx context.Context, desc contracts.PermissionDescriptor, effect, reason string, fields map[string]interface{})
	}); ok {
		if registration.Permissions != nil {
			registration.Permissions.SetEventLogger(func(ctx context.Context, desc contracts.PermissionDescriptor, effect, reason string, fields map[string]interface{}) {
				et.EmitPermissionEvent(ctx, desc, effect, reason, fields)
			})
		}
	}

	// Phase G: Bootstrap Agent Runtime
	boot, err := BootstrapAgentRuntime(cfg.Workspace, AgentBootstrapOptions{
		Context:             ctx,
		AgentID:             registration.ID,
		AgentName:           cfg.AgentName,
		ConfigName:          cfg.AgentName,
		ManifestSnapshot:    cfg.ManifestSnapshot,
		SecurityBundle:      cfg.SecurityBundle,
		ProfileResolution:   profileResolution,
		AgentDefinitions:    cfg.AgentDefinitions,
		PermissionManager:   registration.Permissions,
		Runner:              runner,
		Model:               model,
		Backend:             backend,
		InferenceModel:      inferenceModel,
		Telemetry:           tel,
		MaxIterations:       cfg.MaxIterations,
		SkipASTIndex:        cfg.SkipASTIndex,
		AllowedCapabilities: cfg.AllowedCapabilities,
		DebugLLM:            logLLM,
		DebugAgent:          cfg.DebugAgent,
	})
	if err != nil {
		logFile.Close()
		return nil, err
	}

	// Apply policy engine.
	if boot.PolicyEngine != nil {
		registration.Policy = boot.PolicyEngine
		boot.Environment.Registry.SetPolicyEngine(boot.PolicyEngine)
	}

	// Phase G.5: Prompt Registry
	// BuildPromptRegistry loads .prompt files from relurpify_cfg/prompts/.
	// Provider registration is deferred to named-agent Initialize() calls.
	promptRegistry, err := services.BuildPromptRegistry(cfg.Workspace, tel)
	if err != nil {
		logFile.Close()
		return nil, fmt.Errorf("build prompt registry: %w", err)
	}
	boot.Environment.PromptRegistry = promptRegistry
	logger.Printf("agentenv: prompt registry loaded: %d prompts", promptRegistry.Count())

	// Phase H: ServiceManager, Scheduler, Knowledge, and Retrieval
	// (gated by Scope.Services and Scope.Knowledge)
	env := boot.Environment
	env.PermissionManager = registration.Permissions

	var sm *ServiceManager
	if cfg.Scope.Services {
		scheduler := NewServiceScheduler()
		env.Scheduler = scheduler
		sm = NewServiceManager()
		sm.RegisterWithInfo("scheduler", scheduler, ServiceRegistrationInfo{
			Source: "framework/agentenv/workspace.go",
			Owner:  "framework",
			Notes:  []string{"workspace scheduler", "owned by workspace runtime"},
		})
		env.ServiceManager = sm
	}

	if cfg.Scope.Knowledge && env.IndexManager != nil && env.IndexManager.GraphDB != nil {
		bkcEvents := &knowledge.EventBus{}
		knowledgeStore, err := openKnowledgeStore(env.IndexManager.GraphDB)
		if err != nil {
			logFile.Close()
			return nil, fmt.Errorf("open knowledge store: %w", err)
		}
		env.KnowledgeStore = knowledgeStore
		env.KnowledgeEvents = bkcEvents

		policyBundle, err := contextpolicy.Compile(registration.Manifest, nil, contextpolicy.DefaultContextPolicy())
		if err != nil {
			logFile.Close()
			return nil, fmt.Errorf("compile context policy: %w", err)
		}
		rankerRegistry := retrieval.NewRankerRegistry()
		rankerRegistry.Register(&retrieval.KeywordRanker{K1: 1.2, B: 0.75})
		rankerRegistry.Register(&retrieval.RecencyRanker{HalfLifeHours: 24.0})
		rankerRegistry.Register(&retrieval.ASTProximityRanker{Index: env.IndexManager})
		rankerRegistry.Register(&retrieval.TrustRanker{})
		retriever := retrieval.NewRetriever(rankerRegistry, knowledgeStore).WithPolicy(policyBundle)
		env.Retriever = retriever
		env.Compiler = compiler.NewCompiler(retriever, policyBundle, knowledgeStore)
		env.StreamTrigger = contextstream.NewTrigger(env.Compiler)
	}

	ws := &Workspace{
		Environment:          env,
		Registration:         registration,
		Backend:              backend,
		ProfileResolution:    profileResolution,
		logFile:              logFile,
		eventLog:             eventLog,
		AgentSpec:            boot.AgentSpec,
		AgentDefinitions:     boot.AgentDefinitions,
		EffectiveContract:    boot.Contract,
		CompiledPolicy:       boot.CompiledPolicy,
		PolicyEngine:         boot.PolicyEngine,
		CapabilityAdmissions: boot.CapabilityAdmissions,
		SkillResults:         boot.SkillResults,
		Telemetry:            tel,
		Logger:               logger,
		ServiceManager:       sm,
	}

	// Call agent registration functions
	if regFuncs.RegisterCapabilities != nil {
		if err := regFuncs.RegisterCapabilities(env); err != nil {
			if env.IndexManager != nil {
				_ = env.IndexManager.Close()
			}
			logFile.Close()
			return nil, fmt.Errorf("agent capability registration: %w", err)
		}
	}

	if regFuncs.RegisterPromptProviders != nil {
		if err := regFuncs.RegisterPromptProviders(env); err != nil {
			if env.IndexManager != nil {
				_ = env.IndexManager.Close()
			}
			logFile.Close()
			return nil, fmt.Errorf("agent prompt provider registration: %w", err)
		}
	}

	logger.Printf("agentenv: workspace opened successfully")
	return ws, nil
}

type llmTelemetryAdapter struct {
	inner core.Telemetry
}

func (a llmTelemetryAdapter) Emit(event contracts.Event) {
	if a.inner == nil {
		return
	}
	a.inner.Emit(core.Event{
		Type:      core.EventType(event.Type),
		TaskID:    event.TaskID,
		Message:   event.Message,
		Timestamp: event.Timestamp,
		Metadata:  event.Metadata,
	})
}

func validateConfig(cfg WorkspaceConfig) error {
	if cfg.Workspace == "" {
		return fmt.Errorf("Workspace is required")
	}
	if cfg.Scope != ScopeEmbeddedAgent {
		if cfg.ManifestPath == "" {
			return fmt.Errorf("ManifestPath is required")
		}
		if cfg.InferenceEndpoint == "" {
			return fmt.Errorf("InferenceEndpoint is required")
		}
	}
	return nil
}

func stringValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

// setupTelemetry opens the log file, creates a logger, and assembles the
// telemetry sink chain (logger + optional JSON file). Returns the log file
// (which must be closed by the caller), the logger, and the assembled telemetry.
func setupTelemetry(cfg WorkspaceConfig) (*os.File, *log.Logger, core.Telemetry, error) {
	logPath := cfg.LogPath
	if logPath == "" {
		if cfg.StateDir != "" {
			logPath = filepath.Join(cfg.StateDir, "logs", "agentenv.log")
		} else {
			logPath = filepath.Join(cfg.Workspace, ".relurpify_state", "logs", "agentenv.log")
		}
	}
	telemetryPath := cfg.TelemetryPath
	if telemetryPath == "" {
		if cfg.StateDir != "" {
			telemetryPath = filepath.Join(cfg.StateDir, "telemetry", "agentenv.jsonl")
		} else {
			telemetryPath = filepath.Join(cfg.Workspace, ".relurpify_state", "telemetry", "agentenv.jsonl")
		}
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return nil, nil, nil, fmt.Errorf("create log directory: %w", err)
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("open log: %w", err)
	}
	logger := log.New(logFile, "agentenv ", log.LstdFlags|log.Lmicroseconds)

	var sinks []core.Telemetry
	sinks = append(sinks, telemetry.LoggerTelemetry{Logger: logger})

	if telemetryPath != "" {
		if err := os.MkdirAll(filepath.Dir(telemetryPath), 0o755); err == nil {
			if fileSink, err := telemetry.NewJSONFileTelemetry(telemetryPath); err == nil {
				sinks = append(sinks, fileSink)
			} else {
				logger.Printf("warning: failed to init json telemetry: %v", err)
			}
		}
	}

	return logFile, logger, telemetry.MultiplexTelemetry{Sinks: sinks}, nil
}

// openKnowledgeStore opens the knowledge store with the given graphdb engine.
func openKnowledgeStore(engine *graphdb.Engine) (*knowledge.ChunkStore, error) {
	if engine == nil {
		return nil, fmt.Errorf("graphdb engine required")
	}
	return &knowledge.ChunkStore{Graph: engine}, nil
}

package agentenv

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/agentlifecycle"
	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/ast"
	fauthorization "codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/compiler"
	"codeburg.org/lexbit/relurpify/framework/contextpolicy"
	"codeburg.org/lexbit/relurpify/framework/contextstream"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/event"
	"codeburg.org/lexbit/relurpify/framework/graphdb"
	"codeburg.org/lexbit/relurpify/framework/jobs"
	"codeburg.org/lexbit/relurpify/framework/knowledge"
	"codeburg.org/lexbit/relurpify/framework/manifest"
	"codeburg.org/lexbit/relurpify/framework/memory"
	"codeburg.org/lexbit/relurpify/framework/retrieval"
	fsandbox "codeburg.org/lexbit/relurpify/framework/sandbox"
	"codeburg.org/lexbit/relurpify/framework/search"
	"codeburg.org/lexbit/relurpify/framework/services"
	"codeburg.org/lexbit/relurpify/framework/telemetry"
	"codeburg.org/lexbit/relurpify/platform/contracts"
	"codeburg.org/lexbit/relurpify/platform/llm"
	"gopkg.in/yaml.v3"
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
	EffectiveContract    *manifest.EffectiveAgentContract
	CompiledPolicy       *manifest.CompiledPolicyBundle
	PolicyEngine         fauthorization.PolicyEngine
	CapabilityAdmissions []capability.AdmissionResult
	SkillResults         []manifest.SkillResolution

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
	AgentsDir           string
	AgentSpec           *agentspec.AgentRuntimeSpec
	Manifest            *manifest.AgentManifest
	PermissionManager   *fauthorization.PermissionManager
	Runner              fsandbox.CommandRunner
	Model               contracts.LanguageModel
	Backend             llm.ManagedBackend
	InferenceModel      string
	Telemetry           core.Telemetry
	SkipASTIndex        bool
	MaxIterations       int
	AllowedCapabilities []core.CapabilitySelector
	DebugLLM            bool
	DebugAgent          bool
	AgentLifecycle      agentlifecycle.Repository
	ModelProfile        *contracts.ModelProfile
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
	SkillResults         []manifest.SkillResolution
	CapabilityAdmissions []capability.AdmissionResult
	Contract             *manifest.EffectiveAgentContract
	CompiledPolicy       *manifest.CompiledPolicyBundle
	PolicyEngine         fauthorization.PolicyEngine
}

// BootstrapAgentRuntime bootstraps the agent runtime including loading agent definitions,
// resolving effective contracts, and building the capability bundle.
func BootstrapAgentRuntime(workspace string, opts AgentBootstrapOptions) (*BootstrappedAgentRuntime, error) {
	if workspace == "" {
		return nil, fmt.Errorf("workspace required")
	}
	if opts.Manifest == nil {
		return nil, fmt.Errorf("agent manifest required")
	}
	if opts.Manifest.Spec.Agent == nil && opts.AgentSpec == nil {
		return nil, fmt.Errorf("agent manifest missing spec.agent configuration")
	}
	if opts.Runner == nil {
		return nil, fmt.Errorf("command runner required")
	}

	var agentDefs map[string]*agentspec.AgentDefinition
	var err error
	if opts.AgentsDir != "" {
		agentDefs, err = loadAgentDefinitions(opts.AgentsDir)
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("load agent definitions: %w", err)
		}
	}

	manifestForResolution := opts.Manifest
	if opts.AgentSpec != nil {
		clone := *opts.Manifest
		clone.Spec.Agent = opts.AgentSpec
		manifestForResolution = &clone
	}
	resolveOpts := manifest.ResolveOptions{
		AgentOverlays: selectedAgentDefinitionOverlays(opts.AgentName, agentDefs),
	}
	effectiveContract, err := manifest.ResolveEffectiveAgentContract(workspace, manifestForResolution, resolveOpts, nil)
	if err != nil {
		return nil, err
	}
	agentSpec := effectiveContract.AgentSpec
	skillResults := append([]manifest.SkillResolution{}, effectiveContract.SkillResults...)
	workspacePaths := manifest.New(workspace)
	fileScope := fsandbox.NewFileScopePolicy(workspace, workspacePaths.GovernanceRoots(
		workspacePaths.ManifestFile(),
		workspacePaths.ConfigFile(),
		workspacePaths.NexusConfigFile(),
		workspacePaths.PolicyRulesFile(),
		workspacePaths.ModelProfilesDir(),
	))

	resolvedModel := opts.InferenceModel
	if resolvedModel == "" {
		resolvedModel = agentSpec.Model.Name
	}

	runner := opts.Runner
	if runner != nil {
		runner = fsandbox.NewEnforcingCommandRunner(runner, fauthorization.NewCommandAuthorizationPolicy(opts.PermissionManager, opts.AgentID, agentSpec, "sandbox"))
	}

	capabilities, err := services.BuildBuiltinCapabilityBundle(workspace, runner, services.CapabilityRegistryOptions{
		Context:           opts.Context,
		AgentID:           opts.AgentID,
		PermissionManager: opts.PermissionManager,
		AgentSpec:         agentSpec,
		ProtectedPaths:    manifest.New(workspace).GovernanceRoots(manifest.New(workspace).ManifestFile(), manifest.New(workspace).ConfigFile(), manifest.New(workspace).NexusConfigFile(), manifest.New(workspace).PolicyRulesFile(), manifest.New(workspace).ModelProfilesDir()),
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
	policyEngine, err := fauthorization.FromAgentSpecWithConfig(effectiveContract.AgentSpec, effectiveContract.AgentID, opts.PermissionManager)
	if err != nil {
		return nil, fmt.Errorf("compile effective policy: %w", err)
	}
	registry.SetPolicyEngine(policyEngine)

	maxIterations := opts.MaxIterations
	if maxIterations <= 0 {
		maxIterations = 8
	}
	configName := opts.ConfigName
	if configName == "" {
		configName = opts.Manifest.Metadata.Name
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
	if opts.ModelProfile != nil {
		registry.SetModelProfile(opts.ModelProfile)
	}
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
		CommandRunner:                 runner,
		JobSubmitter:                  jobs.NoopSubmitter{},
		CommandPolicy:                 fauthorization.NewCommandAuthorizationPolicy(opts.PermissionManager, opts.AgentID, agentSpec, "workspace"),
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
		PolicyEngine:         policyEngine,
	}, nil
}

func loadAgentDefinitions(dir string) (map[string]*agentspec.AgentDefinition, error) {
	defs := make(map[string]*agentspec.AgentDefinition)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		path := filepath.Join(dir, name)
		def, err := agentspec.LoadAgentDefinition(path)
		if err != nil {
			if errors.Is(err, agentspec.ErrNotAgentDefinition) {
				continue
			}
			return nil, fmt.Errorf("load %s: %w", name, err)
		}
		if def.Name == "" {
			def.Name = strings.TrimSuffix(name, filepath.Ext(name))
		}
		defs[def.Name] = def
	}
	return defs, nil
}

func selectedAgentDefinitionOverlays(agentName string, defs map[string]*agentspec.AgentDefinition) []agentspec.AgentSpecOverlay {
	if defs == nil {
		return nil
	}
	def, ok := defs[agentName]
	if !ok || def == nil {
		return nil
	}
	return []agentspec.AgentSpecOverlay{agentspec.AgentSpecOverlayFromSpec(&def.Spec)}
}

// Open initializes a complete workspace session: platform checks, store
// opening, service graph construction, agent registration, and background
// indexing. The returned *Workspace is ready for agent construction.
//
// Open is the single composition root for all Relurpify entry points.
// app/relurpish, app/dev-agent-cli, and integration tests all call Open().
func Open(ctx context.Context, cfg WorkspaceConfig, regFuncs AgentRegistrationFuncs) (*Workspace, error) {
	// Resolve workspace YAML overrides before probing or opening stores.
	cfg = resolveWorkspaceConfigOverrides(cfg)

	// Phase A: Configuration Validation
	if err := validateConfig(cfg); err != nil {
		return nil, fmt.Errorf("invalid workspace config: %w", err)
	}

	backend, err := llm.New(llm.ProviderConfigFromRuntimeConfig(cfg))
	if err != nil {
		return nil, fmt.Errorf("build inference backend: %w", err)
	}

	// Phase C: Log and Telemetry Setup
	logFile, logger, tel, err := setupTelemetry(cfg)
	if err != nil {
		return nil, err
	}

	// Phase C.5: Event Log Setup (if factory provided)
	var eventLog event.Log
	if cfg.EventLogFactory != nil && cfg.EventsPath != "" {
		eventLog, err = cfg.EventLogFactory(cfg.EventsPath)
		if err != nil {
			logFile.Close()
			return nil, fmt.Errorf("create event log: %w", err)
		}
	}

	// Phase D: KnowledgeStore initialization deferred until after BootstrapAgentRuntime
	// where the graphdb.Engine is available from IndexManager.

	// Phase E: Agent Registration + Authorization
	manifestSnapshot, err := manifest.LoadAgentManifestSnapshot(cfg.ManifestPath)
	if err != nil {
		logFile.Close()
		return nil, fmt.Errorf("load manifest snapshot: %w", err)
	}
	registration, err := fauthorization.RegisterAgent(ctx, fauthorization.RuntimeConfig{
		ManifestPath:     cfg.ManifestPath,
		ManifestSnapshot: manifestSnapshot,
		ConfigPath:       cfg.ConfigPath,
		Backend:          cfg.SandboxBackend,
		AuditLimit:       cfg.AuditLimit,
		BaseFS:           cfg.Workspace,
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

	profileRegistry, err := llm.NewProfileRegistry(manifest.New(cfg.Workspace).ModelProfilesDir())
	if err != nil {
		logFile.Close()
		return nil, fmt.Errorf("load model profiles: %w", err)
	}
	profileResolution := profileRegistry.Resolve(cfg.InferenceProvider, inferenceModel)
	_ = llm.ApplyProfile(backend, profileResolution.Profile)

	logLLM := cfg.DebugLLM
	if registration.Manifest != nil && registration.Manifest.Spec.Agent != nil {
		if registration.Manifest.Spec.Agent.Logging != nil && registration.Manifest.Spec.Agent.Logging.LLM != nil {
			logLLM = *registration.Manifest.Spec.Agent.Logging.LLM
		}
	}
	backend.SetDebugLogging(logLLM)
	model := llm.NewInstrumentedModel(backend.Model(), llmTelemetryAdapter{inner: tel}, logLLM)
	_ = llm.ApplyProfile(model, profileResolution.Profile)

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

	// Phase G: Create ServiceManager and Bootstrap
	scheduler := NewServiceScheduler()

	boot, err := BootstrapAgentRuntime(cfg.Workspace, AgentBootstrapOptions{
		Context:             ctx,
		AgentID:             registration.ID,
		AgentName:           cfg.AgentName,
		ConfigName:          cfg.AgentName,
		AgentsDir:           cfg.AgentsDir,
		Manifest:            registration.Manifest,
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
		ModelProfile:        profileResolution.Profile,
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

	// Phase H: ServiceManager Setup & Scheduler Registration
	env := boot.Environment
	sm := NewServiceManager()
	bkcEvents := &knowledge.EventBus{}
	sm.RegisterWithInfo("scheduler", scheduler, ServiceRegistrationInfo{
		Source: "framework/agentenv/workspace.go",
		Owner:  "framework",
		Notes:  []string{"workspace scheduler", "owned by workspace runtime"},
	})

	// Initialize KnowledgeStore now that GraphDB is available
	knowledgeStore, err := openKnowledgeStore(env.IndexManager.GraphDB)
	if err != nil {
		logFile.Close()
		return nil, fmt.Errorf("open knowledge store: %w", err)
	}

	env.Scheduler = scheduler
	env.PermissionManager = registration.Permissions
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

	// Attach ServiceManager to environment (for direct access)
	env.ServiceManager = sm

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

// resolveWorkspaceConfig loads the workspace YAML (if ConfigPath is
// set) and applies model and agent-name overrides. Errors are silently ignored
// so that a missing or malformed config file does not prevent startup.
func resolveWorkspaceConfigOverrides(cfg WorkspaceConfig) WorkspaceConfig {
	if cfg.ConfigPath == "" {
		return cfg
	}
	type yamlCfg struct {
		Provider     string   `json:"provider" yaml:"provider"`
		Model        string   `json:"model" yaml:"model"`
		Backend      string   `json:"sandbox_backend" yaml:"sandbox_backend"`
		Agent        string   `json:"agent" yaml:"agent"`
		Agents       []string `json:"agents" yaml:"agents"`
		DefaultModel struct {
			Name string `json:"name" yaml:"name"`
		} `json:"default_model" yaml:"default_model"`
	}
	data, err := os.ReadFile(cfg.ConfigPath)
	if err != nil {
		return cfg
	}
	// Try JSON first (YAML is a superset, but we keep it simple here).
	var yc yamlCfg
	if err := yaml.Unmarshal(data, &yc); err == nil {
		if yc.Provider != "" && cfg.InferenceProvider == "" {
			cfg.InferenceProvider = yc.Provider
		}
		if yc.Model != "" && cfg.InferenceModel == "" {
			cfg.InferenceModel = yc.Model
		}
		if yc.Backend != "" && cfg.SandboxBackend == "" {
			cfg.SandboxBackend = yc.Backend
		}
		if yc.DefaultModel.Name != "" && cfg.InferenceModel == "" {
			cfg.InferenceModel = yc.DefaultModel.Name
		}
		if yc.Agent != "" && cfg.AgentName == "" {
			cfg.AgentName = yc.Agent
		}
		if len(yc.Agents) > 0 && cfg.AgentName == "" {
			cfg.AgentName = yc.Agents[0]
		}
	}
	return cfg
}

func validateConfig(cfg WorkspaceConfig) error {
	if cfg.Workspace == "" {
		return fmt.Errorf("Workspace is required")
	}
	if cfg.ManifestPath == "" {
		return fmt.Errorf("ManifestPath is required")
	}
	if cfg.InferenceEndpoint == "" {
		return fmt.Errorf("InferenceEndpoint is required")
	}
	return nil
}

// setupTelemetry opens the log file, creates a logger, and assembles the
// telemetry sink chain (logger + optional JSON file). Returns the log file
// (which must be closed by the caller), the logger, and the assembled telemetry.
func setupTelemetry(cfg WorkspaceConfig) (*os.File, *log.Logger, core.Telemetry, error) {
	logPath := cfg.LogPath
	if logPath == "" {
		paths := manifest.New(cfg.Workspace)
		logPath = filepath.Join(paths.LogsDir(), "agentenv.log")
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

	if cfg.TelemetryPath != "" {
		if err := os.MkdirAll(filepath.Dir(cfg.TelemetryPath), 0o755); err == nil {
			if fileSink, err := telemetry.NewJSONFileTelemetry(cfg.TelemetryPath); err == nil {
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

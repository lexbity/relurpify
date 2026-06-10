package session

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	regpkg "codeburg.org/lexbit/relurpify/capability/registry"
	fsandbox "codeburg.org/lexbit/relurpify/capability/sandbox"
	"codeburg.org/lexbit/relurpify/context/knowledge/ast"
	"codeburg.org/lexbit/relurpify/context/knowledge/memory"
	"codeburg.org/lexbit/relurpify/context/knowledge/search"
	"codeburg.org/lexbit/relurpify/context/persistence/artifactstore"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentlifecycle"
	"codeburg.org/lexbit/relurpify/execution/workspace"
	"codeburg.org/lexbit/relurpify/governance/permissions"
	"codeburg.org/lexbit/relurpify/governance/policy"
	"codeburg.org/lexbit/relurpify/jobs"
	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/telemetry"
	"codeburg.org/lexbit/relurpify/telemetry/event"
	"codeburg.org/lexbit/relurpify/userconfig/config"
	cfgsecurity "codeburg.org/lexbit/relurpify/userconfig/config/security"
	"codeburg.org/lexbit/relurpify/userconfig/modelselect"
)

// Workspace is a live, initialized workspace session. It holds all open
// resources. Close() must be called when the session ends. Restart() may
// be used to cleanly stop and re-start services without rebuilding stores.
type Workspace struct {
	Environment       agentEnv
	Registration      *Registration
	Backend           model.ModelBackend
	ProfileResolution modelselect.ProfileResolution

	// Internals held for Close()/Restart()
	logFile  io.Closer
	eventLog io.Closer

	// Derived fields for callers that need them
	AgentSpec            *agentspec.AgentRuntimeSpec
	EffectiveContract    *config.EffectiveAgentContract
	CompiledPolicy       *CompiledPolicy
	PolicyEngine         regpkg.PolicyEngine
	CapabilityAdmissions []regpkg.AdmissionResult

	// Observability
	Telemetry telemetry.Telemetry
	Logger    *log.Logger

	// Service management (new for dynamic lifecycle)
	ServiceManager *serviceManager
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

	if w.Environment.ArtifactStore != nil {
		if err := w.Environment.ArtifactStore.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close artifact store: %w", err))
		}
	}

	if w.eventLog != nil {
		if err := w.eventLog.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close event log: %w", err))
		}
	}

	// Close IndexManager if present (allocated in BootstrapAgentRuntime).
	if w.Environment.IndexManager != nil {
		if err := w.Environment.IndexManager.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close index manager: %w", err))
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
	Context              context.Context
	AgentID              string
	AgentName            string
	ConfigName           string
	AgentSpec            *agentspec.AgentRuntimeSpec
	ManifestSnapshot     *config.AgentManifestSnapshot
	SecurityBundle       *cfgsecurity.Bundle
	ProfileResolution    modelselect.ProfileResolution
	PermissionManager    permissions.PermissionManager
	Runner               fsandbox.CommandRunner
	CommandPolicy        fsandbox.CommandPolicy
	SandboxBackend       string
	Model                model.LanguageModel
	Backend              model.ModelBackend
	InferenceModel       string
	Telemetry            telemetry.Telemetry
	SkipASTIndex         bool
	MaxIterations        int
	AllowedCapabilities  []agentspec.CapabilitySelector
	DebugLLM             bool
	DebugAgent           bool
	AgentLifecycle       agentlifecycle.Repository
	CapabilityAdmissions []regpkg.AdmissionResult
	Contract             *config.EffectiveAgentContract
	CompiledPolicy       *CompiledPolicy
	PolicyEngine         regpkg.PolicyEngine
	// Pre-built capability product. App composition is responsible for building
	// the capability runtime; agentenv only consumes it.
	CapabilityRegistry     *regpkg.CapabilityRegistry
	CapabilityIndexManager *ast.IndexManager
	CapabilitySearchEngine *search.SearchEngine
}

// BootstrappedAgentRuntime contains the results of bootstrapping an agent runtime.
type BootstrappedAgentRuntime struct {
	AgentSpec            *agentspec.AgentRuntimeSpec
	AgentConfig          *execution.Config
	Backend              model.ModelBackend
	Environment          agentEnv
	Registry             *regpkg.CapabilityRegistry
	IndexManager         *ast.IndexManager
	SearchEngine         *search.SearchEngine
	CapabilityAdmissions []regpkg.AdmissionResult
	Contract             *config.EffectiveAgentContract
	CompiledPolicy       *CompiledPolicy
	PolicyEngine         regpkg.PolicyEngine
}

// BootstrapAgentRuntime bootstraps the agent runtime including resolving the
// effective contract and building the capability bundle.
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
	if opts.SecurityBundle == nil {
		return nil, fmt.Errorf("security bundle required")
	}

	manifestForResolution := opts.ManifestSnapshot.Manifest
	if opts.AgentSpec != nil {
		clone := *opts.ManifestSnapshot.Manifest
		clone.Spec.Agent = opts.AgentSpec
		manifestForResolution = &clone
	}
	resolveOpts := config.ResolveOptions{}
	effectiveContract, err := config.ResolveEffectiveAgentContract(workspace, manifestForResolution, resolveOpts)
	if err != nil {
		return nil, err
	}
	agentSpec := effectiveContract.AgentSpec
	fileScope := fsandbox.NewFileScopePolicy(workspace, append([]string(nil), opts.SecurityBundle.Sandbox.ProtectedPaths...))

	resolvedModel := opts.InferenceModel
	if resolvedModel == "" {
		resolvedModel = agentSpec.Model.Name
	}

	authRunner, ok := opts.Runner.(*fsandbox.AuthorizedRunner)
	if !ok || authRunner == nil {
		return nil, fmt.Errorf("authorized command runner required")
	}
	if opts.PolicyEngine == nil {
		return nil, fmt.Errorf("policy engine required")
	}

	if opts.CapabilityRegistry == nil {
		return nil, fmt.Errorf("app-composed capability runtime required")
	}
	registry := opts.CapabilityRegistry
	indexManager := opts.CapabilityIndexManager
	searchEngine := opts.CapabilitySearchEngine
	if opts.Telemetry != nil {
		registry.UseTelemetry(opts.Telemetry)
	}
	if opts.PermissionManager != nil {
		if h, ok := opts.PermissionManager.(regpkg.PermissionManagerHandle); ok {
			registry.UsePermissionManager(opts.AgentID, h)
		}
	}
	if opts.ProfileResolution.Profile != nil {
		registry.SetModelProfile(opts.ProfileResolution.Profile)
	}
	compiledPolicy, err := buildCompiledPolicy(effectiveContract, opts.PolicyEngine, nil)
	if err != nil {
		return nil, fmt.Errorf("build compiled policy: %w", err)
	}
	registry.SetPolicyEngine(opts.PolicyEngine)

	maxIterations := opts.MaxIterations
	if maxIterations <= 0 {
		maxIterations = 8
	}
	configName := opts.ConfigName
	if configName == "" {
		configName = opts.ManifestSnapshot.Manifest.Metadata.Name
	}
	agentCfg := &execution.Config{
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
	admissionResults, err := regpkg.AdmitCandidates(
		registry,
		nil,
		agentspec.EffectiveAllowedCapabilitySelectors(agentSpec),
	)
	if err != nil {
		return nil, fmt.Errorf("admit skill capabilities: %w", err)
	}

	// Create working memory store
	wm := memory.NewWorkingMemoryStore()

	env := agentEnv{
		Config:            agentCfg,
		Model:             opts.Model,
		CommandRunner:     authRunner,
		JobSubmitter:      jobs.NoopSubmitter{},
		CommandPolicy:     opts.CommandPolicy,
		FileScope:         fileScope,
		Registry:          registry,
		PermissionManager: opts.PermissionManager,
		IndexManager:      indexManager,
		SearchEngine:      searchEngine,
		WorkingMemory:     wm,
		KnowledgeStore:    nil, // Will be populated in Open
		Retriever:         nil, // Will be populated in Open
		Compiler:          nil, // Will be populated in Open
		EventLog:          nil,
		Scheduler:         nil,
		ServiceManager:    nil,
	}

	return &BootstrappedAgentRuntime{
		Registry:             registry,
		IndexManager:         indexManager,
		SearchEngine:         searchEngine,
		AgentSpec:            agentSpec,
		AgentConfig:          agentCfg,
		Backend:              opts.Backend,
		Environment:          env,
		CapabilityAdmissions: admissionResults,
		Contract:             effectiveContract,
		CompiledPolicy:       compiledPolicy,
		PolicyEngine:         opts.PolicyEngine,
	}, nil
}

func buildCompiledPolicy(contract *config.EffectiveAgentContract, engine regpkg.PolicyEngine, rules []policy.PolicyRule) (*CompiledPolicy, error) {
	if contract == nil {
		return nil, fmt.Errorf("effective agent contract required")
	}
	if contract.AgentID == "" {
		return nil, fmt.Errorf("agent id required")
	}
	if contract.AgentSpec == nil {
		return nil, fmt.Errorf("agent spec required")
	}
	return &CompiledPolicy{
		AgentID: contract.AgentID,
		Spec:    contract.AgentSpec,
		Rules:   rules,
		Engine:  engine,
	}, nil
}

// OpenWorkspace consumes app-composed registration/security products and opens
// the execution workspace. It initializes platform checks, stores, capability
// registration, background indexing, and (depending on scope) LLM backend,
// knowledge, services, and telemetry.
//
// Feature assembly is governed by cfg.Scope. Security and capability
// assembly are unconditional; LLM backend, knowledge, services, and
// telemetry-sink construction are gated behind their respective scope
// flags. A zero-value scope defaults to ScopeFull for backward
// compatibility.
//
// Builder taxonomy (design decision 8):
//
//	OpenWorkspace         — session lifecycle (this function)
//	BootstrapAgentRuntime  — agent runtime assembly
//	Build*                 — single leaf components (capabilities, prompts)
//
// Scope mechanism (design decision 9):
//
//	cfg.Scope is a WorkspaceScope field, not a positional parameter.
//	ScopeFull = every optional layer.
//	ScopeEmbeddedAgent = security + capabilities only.
func OpenWorkspace(ctx context.Context, cfg WorkspaceConfig) (_ *Workspace, err error) {
	var cleanup CloseStack
	defer func() {
		if err != nil {
			if closeErr := cleanup.Close(ctx); closeErr != nil {
				log.Printf("workspace: cleanup error during failed open: %v", closeErr)
			}
		}
	}()

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
			cfg.StateDir = workspace.StateDir(cfg.Workspace)
		}
	}

	defaultStateDir := workspace.StateDir(cfg.Workspace)
	defaultLogPath := filepath.Join(defaultStateDir, "logs", "workspace.log")
	defaultTelemetryPath := filepath.Join(defaultStateDir, "telemetry", "workspace.jsonl")
	defaultEventsPath := filepath.Join(defaultStateDir, "events.db")
	defaultMemoryPath := filepath.Join(defaultStateDir, "memory")
	if cfg.LogPath == "" || filepath.Clean(cfg.LogPath) == filepath.Clean(defaultLogPath) {
		cfg.LogPath = filepath.Join(cfg.StateDir, "logs", "workspace.log")
	}
	if cfg.TelemetryPath == "" || filepath.Clean(cfg.TelemetryPath) == filepath.Clean(defaultTelemetryPath) {
		cfg.TelemetryPath = filepath.Join(cfg.StateDir, "telemetry", "workspace.jsonl")
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
	var backend model.ModelBackend
	if cfg.Scope.LLMBackend {
		if cfg.ModelProduct == nil || cfg.ModelProduct.Backend == nil {
			return nil, fmt.Errorf("app-composed model runtime required")
		}
		backend = cfg.ModelProduct.Backend
	}

	// Phase C: Log and Telemetry Setup (logger always on; JSON sink gated by
	// cfg.Scope.TelemetrySinks, enforced via empty cfg.TelemetryPath above)
	logFile, logger, tel, err := setupTelemetry(cfg)
	if err != nil {
		return nil, err
	}
	cleanup.Add(func(ctx context.Context) error { return logFile.Close() })

	// Phase C.5: Event Log Setup (gated by Scope.Services)
	var eventLog event.Log
	if cfg.Scope.Services && cfg.EventLogFactory != nil && cfg.EventsPath != "" {
		eventLog, err = cfg.EventLogFactory(cfg.EventsPath)
		if err != nil {
			return nil, fmt.Errorf("create event log: %w", err)
		}
		cleanup.Add(func(ctx context.Context) error { return eventLog.Close() })
	}

	// Phase D: KnowledgeStore initialization deferred until after BootstrapAgentRuntime
	// where the graphdb.Engine is available from IndexManager.

	// Phase E: App-composed registration + security products
	// For embedded scope, synthesize a minimal manifest snapshot if none provided.
	if cfg.ManifestSnapshot == nil && cfg.AgentSpec != nil {
		cfg.ManifestSnapshot = synthesizeManifestSnapshot(cfg.AgentName, cfg.AgentSpec)
	}
	registration := cfg.Registration
	if registration == nil {
		return nil, fmt.Errorf("app-composed registration required")
	}

	// Phase F: Capability Bundle + Agent Environment
	if cfg.SecurityRuntime == nil {
		return nil, fmt.Errorf("app-composed security runtime required")
	}
	if cfg.SecurityRuntime.Runner == nil {
		return nil, fmt.Errorf("security runtime missing runner")
	}
	if cfg.SecurityRuntime.PolicyEngine == nil {
		return nil, fmt.Errorf("security runtime missing policy engine")
	}
	runner := cfg.SecurityRuntime.Runner

	// Resolve model from manifest if not overridden in manifest.
	inferenceModel := cfg.InferenceModel
	if registration.Manifest != nil && registration.Manifest.Spec.Agent != nil {
		if specModel := registration.Manifest.Spec.Agent.Model.Name; specModel != "" && inferenceModel == "" {
			inferenceModel = specModel
		}
	}

	var model model.LanguageModel
	var logLLM bool
	profileResolution := cfg.ProfileResolution
	if cfg.Scope.LLMBackend && backend != nil {
		logLLM = cfg.DebugLLM
		if registration.Manifest != nil && registration.Manifest.Spec.Agent != nil {
			if registration.Manifest.Spec.Agent.Logging != nil && registration.Manifest.Spec.Agent.Logging.LLM != nil {
				logLLM = *registration.Manifest.Spec.Agent.Logging.LLM
			}
		}
		backend.SetDebugLogging(logLLM)
		if cfg.ModelProduct.ModelFactory != nil {
			model = cfg.ModelProduct.ModelFactory(tel, logLLM)
		} else {
			model = backend.Model()
		}
	}

	// Wire permission event logger if event telemetry is available.
	if et, ok := tel.(interface {
		EmitPermissionEvent(ctx context.Context, desc permissions.PermissionDescriptor, effect, reason string, fields map[string]any)
	}); ok {
		if registration.Permissions != nil {
			registration.Permissions.SetEventLogger(func(ctx context.Context, desc permissions.PermissionDescriptor, effect, reason string, fields map[string]any) {
				et.EmitPermissionEvent(ctx, desc, effect, reason, fields)
			})
		}
	}

	// Phase G: Bootstrap Agent Runtime
	bootstrapOpts := AgentBootstrapOptions{
		Context:             ctx,
		AgentID:             registration.ID,
		AgentName:           cfg.AgentName,
		ConfigName:          cfg.AgentName,
		ManifestSnapshot:    cfg.ManifestSnapshot,
		SecurityBundle:      cfg.SecurityBundle,
		ProfileResolution:   profileResolution,
		PermissionManager:   registration.Permissions,
		Runner:              runner,
		CommandPolicy:       cfg.SecurityRuntime.CommandPolicy,
		Model:               model,
		Backend:             backend,
		InferenceModel:      inferenceModel,
		Telemetry:           tel,
		MaxIterations:       cfg.MaxIterations,
		SkipASTIndex:        cfg.SkipASTIndex,
		AllowedCapabilities: cfg.AllowedCapabilities,
		DebugLLM:            logLLM,
		DebugAgent:          cfg.DebugAgent,
		PolicyEngine:        cfg.SecurityRuntime.PolicyEngine,
	}
	if cfg.CapabilityProduct != nil {
		bootstrapOpts.CapabilityRegistry = cfg.CapabilityProduct.Registry
		bootstrapOpts.CapabilityIndexManager = cfg.CapabilityProduct.IndexManager
		bootstrapOpts.CapabilitySearchEngine = cfg.CapabilityProduct.SearchEngine
	}
	boot, err := BootstrapAgentRuntime(cfg.Workspace, bootstrapOpts)
	if err != nil {
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
	promptRegistry, err := BuildPromptRegistry(cfg.Workspace, tel)
	if err != nil {
		return nil, fmt.Errorf("build prompt registry: %w", err)
	}
	boot.Environment.PromptRegistry = promptRegistry
	logger.Printf("workspace: prompt registry loaded: %d prompts", promptRegistry.Count())

	// Phase H: ServiceManager, Scheduler, Knowledge, and Retrieval
	// (gated by Scope.Services and Scope.Knowledge)
	env := boot.Environment
	env.PermissionManager = registration.Permissions

	// Phase H.5: Artifact Store — per-session durable storage for tool output.
	artifactStore, err := artifactstore.NewDiskStore(cfg.Workspace, 0)
	if err != nil {
		return nil, fmt.Errorf("create artifact store: %w", err)
	}
	cleanup.Add(func(ctx context.Context) error { return artifactStore.Close() })
	env.ArtifactStore = artifactStore

	var sm *serviceManager
	if cfg.Scope.Services {
		scheduler := NewServiceScheduler()
		env.Scheduler = scheduler
		sm = NewServiceManager()
		sm.RegisterWithInfo("scheduler", scheduler, ServiceRegistrationInfo{
			Source: "execution/session/workspace.go",
			Owner:  "execution",
			Notes:  []string{"workspace scheduler", "owned by workspace runtime"},
		})
		env.ServiceManager = sm
		cleanup.Add(func(ctx context.Context) error {
			return sm.Clear()
		})
	}

	if cfg.Scope.Knowledge {
		if cfg.KnowledgeProduct == nil {
			return nil, fmt.Errorf("app-composed knowledge runtime required")
		}
		env.KnowledgeStore = cfg.KnowledgeProduct.KnowledgeStore
		env.KnowledgeEvents = cfg.KnowledgeProduct.KnowledgeEvents
		env.Retriever = cfg.KnowledgeProduct.Retriever
		env.Compiler = cfg.KnowledgeProduct.Compiler
		env.StreamTrigger = cfg.KnowledgeProduct.StreamTrigger
	}

	ws := &Workspace{
		Environment:          env,
		Registration:         registration,
		Backend:              backend,
		ProfileResolution:    profileResolution,
		logFile:              logFile,
		eventLog:             eventLog,
		AgentSpec:            boot.AgentSpec,
		EffectiveContract:    boot.Contract,
		CompiledPolicy:       boot.CompiledPolicy,
		PolicyEngine:         boot.PolicyEngine,
		CapabilityAdmissions: boot.CapabilityAdmissions,
		Telemetry:            tel,
		Logger:               logger,
		ServiceManager:       sm,
	}

	logger.Printf("workspace: workspace opened successfully")
	return ws, nil
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
func setupTelemetry(cfg WorkspaceConfig) (*os.File, *log.Logger, telemetry.Telemetry, error) {
	logPath := cfg.LogPath
	if logPath == "" {
		if cfg.StateDir != "" {
			logPath = filepath.Join(cfg.StateDir, "logs", "workspace.log")
		} else {
			logPath = filepath.Join(cfg.Workspace, workspace.StateDirName, "logs", "workspace.log")
		}
	}
	telemetryPath := cfg.TelemetryPath
	if telemetryPath == "" {
		if cfg.StateDir != "" {
			telemetryPath = filepath.Join(cfg.StateDir, "telemetry", "workspace.jsonl")
		} else {
			telemetryPath = filepath.Join(cfg.Workspace, workspace.StateDirName, "telemetry", "workspace.jsonl")
		}
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return nil, nil, nil, fmt.Errorf("create log directory: %w", err)
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("open log: %w", err)
	}
	logger := log.New(logFile, "workspace ", log.LstdFlags|log.Lmicroseconds)

	var sinks []telemetry.Telemetry
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

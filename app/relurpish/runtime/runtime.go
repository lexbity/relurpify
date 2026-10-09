package runtime

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/app/envcomposition"
	"codeburg.org/lexbit/relurpify/ayenitd"
	"codeburg.org/lexbit/relurpify/capability/agentspec"
	aconvert "codeburg.org/lexbit/relurpify/capability/agentspec/convert"
	registry "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/capability/sandbox"
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/knowledge/ast"
	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
	"codeburg.org/lexbit/relurpify/context/knowledge/memory"
	"codeburg.org/lexbit/relurpify/context/knowledge/search"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentgraph"
	"codeburg.org/lexbit/relurpify/execution/agentlifecycle"
	"codeburg.org/lexbit/relurpify/execution/compiler"
	"codeburg.org/lexbit/relurpify/execution/session"
	"codeburg.org/lexbit/relurpify/execution/workspace"
	fauthorization "codeburg.org/lexbit/relurpify/governance/authorization"
	"codeburg.org/lexbit/relurpify/governance/permissions"
	"codeburg.org/lexbit/relurpify/governance/policy"
	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/named/euclo"
	"codeburg.org/lexbit/relurpify/named/euclo/euclocontract"
	intentcontext "codeburg.org/lexbit/relurpify/named/euclo/intentcontext"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
	euclopolicy "codeburg.org/lexbit/relurpify/named/euclo/policy"
	euclostate "codeburg.org/lexbit/relurpify/named/euclo/state"
	"codeburg.org/lexbit/relurpify/platform/llm"
	"codeburg.org/lexbit/relurpify/platform/observability"
	"codeburg.org/lexbit/relurpify/telemetry"
	"codeburg.org/lexbit/relurpify/telemetry/event"
	"codeburg.org/lexbit/relurpify/userconfig/config"
	"codeburg.org/lexbit/relurpify/userconfig/modelselect"
)

// Runtime wires the relurpish CLI, Bubble Tea UI, and API server to the shared
// agent fruntime. It centralizes tool registration, manifests, sandbox
// registration, and log management.
type Runtime struct {
	Config           Config
	Workspace        *session.Workspace
	Session          *session.WorkspaceSession
	Tools            *registry.CapabilityRegistry
	Memory           *memory.WorkingMemoryStore
	Agent            agentgraph.WorkflowExecutor
	Model            model.LanguageModel
	Compiler         *compiler.Compiler
	IndexManager     *ast.IndexManager
	GraphDB          *graphdb.Engine
	SearchEngine     *search.SearchEngine
	AgentLifecycle   agentlifecycle.Repository
	Delegations      *fauthorization.DelegationManager
	WorkspaceConfig  config.RuntimeWorkspaceConfig
	documentSnapshot *config.DocumentSnapshot
	secrets          config.Secrets
	registration     *fauthorization.AgentRegistration
	modelBackend     llm.ManagedBackend

	// sessionID is the process-scoped correlation session created with the
	// runtime. It is stamped onto every turn's RunContext and mirrored onto the
	// turn envelope so telemetry events group into one session (FR-1, FR-2).
	sessionID   string
	sessionIDMu sync.Mutex

	// activeWorkflowID is the lifecycle workflow of the run currently
	// executing (set by RunTask/ResumeSession from the run record created in
	// agentlifecycle, cleared when that run completes). The autosave reads it
	// so session records carry a resumable ID.
	activeWorkflowMu sync.RWMutex
	activeWorkflowID string

	execSink *telemetry.BroadcastSink

	providersMu          sync.Mutex
	providers            []runtimeProviderRecord
	interactionMu        sync.Mutex
	interactionEnvelopes map[string]*contextdata.Envelope
	delegationMu         sync.Mutex
	delegationBG         *backgroundDelegationProvider
}

// AgentWorkspace returns the execution workspace for this Runtime.
func (r *Runtime) AgentWorkspace() *session.Workspace {
	return r.Workspace
}

// ProviderSecrets returns env-only provider credentials for backend construction.
func (r *Runtime) ProviderSecrets() llm.ProviderSecrets {
	if r == nil {
		return llm.ProviderSecrets{}
	}
	return llm.ProviderSecrets{APIKey: r.secrets.LLMAPIKey}
}

// Secrets returns the env-only runtime secret set.
func (r *Runtime) Secrets() config.Secrets {
	if r == nil {
		return config.Secrets{}
	}
	return r.secrets
}

// New builds a runtime for the TUI and status surfaces.
// Recoverable failures (config parse errors, sandbox backend unavailable,
// model backend down) produce a degraded runtime with deny-all scope
// instead of failing startup.
// Programming errors may still panic or return an error from lower layers.
func New(ctx context.Context, cfg Config, secrets config.Secrets) (*Runtime, error) {
	rt, err := buildRuntime(ctx, cfg, secrets)
	if err != nil {
		return newDegradedRuntime(ctx, cfg, err), nil
	}
	return rt, nil
}

// buildRuntime is the full construction path. When it fails, New() produces
// a degraded Runtime so the TUI always launches.
func buildRuntime(ctx context.Context, cfg Config, secrets config.Secrets) (*Runtime, error) {
	// Save flag-provided values before env overrides and config loading for
	// precedence: flag > env > config > default. We snapshot before
	// Normalize fills in defaults from config files.
	preProvider := cfg.InferenceProvider
	preModel := cfg.InferenceModel
	preSandboxBackend := cfg.SandboxBackend
	preTapePath := cfg.InferenceTapePath
	preEndpoint := cfg.InferenceEndpoint

	envOverrides, err := config.LoadEnvOverrides(cfg.EnvOverrides)
	if err != nil {
		return nil, fmt.Errorf("load env overrides: %w", err)
	}
	if envOverrides.WorkspaceRoot != "" {
		cfg.Workspace = envOverrides.WorkspaceRoot
	}
	if envOverrides.ModelProvider != "" {
		cfg.InferenceProvider = envOverrides.ModelProvider
		preProvider = envOverrides.ModelProvider
	}
	if envOverrides.ModelName != "" {
		cfg.InferenceModel = envOverrides.ModelName
		preModel = envOverrides.ModelName
	}
	if envOverrides.SandboxBackend != "" {
		cfg.SandboxBackend = envOverrides.SandboxBackend
		preSandboxBackend = envOverrides.SandboxBackend
	}
	if err := cfg.Normalize(); err != nil {
		return nil, err
	}
	if envOverrides.OllamaHost != "" {
		cfg.InferenceEndpoint = envOverrides.OllamaHost
	}
	// Track whether the endpoint was explicitly set by flag or env override;
	// catalog default only applies when this is false.
	endpointExplicit := preEndpoint != "" || envOverrides.OllamaHost != ""
	if envOverrides.LogLevel != "" {
		cfg.RecordingMode = envOverrides.LogLevel
	}
	if cfg.Editor == "" {
		cfg.Editor = envOverrides.Editor
	}
	if cfg.SharedRoot == "" {
		cfg.SharedRoot = config.ResolveSharedRoot(envOverrides.XDGDataHome)
	}
	loadedConfig, _, err := config.Load(config.LoadOptions{
		WorkspaceRoot:         cfg.Workspace,
		EnvOverrides:          cfg.EnvOverrides,
		SubprocessToolFactory: cfg.SubprocessToolFactory,
	})
	if err != nil {
		return nil, fmt.Errorf("load workspace config bundle: %w", err)
	}

	// Load workspace YAML to get model/provider/sandbox preferences before
	// composing the runtime. The V1 config format (relurpify/workspace/v1)
	// is the canonical nested format. A missing file is non-blocking
	// (uninitialized workspace); a present but invalid file is blocking.
	var workspaceCfg config.RuntimeWorkspaceConfig
	var allowedCapabilities []agentspec.CapabilitySelector
	backendFactory := cfg.SandboxBackendFactory
	if backendFactory == nil {
		backendFactory = envcomposition.NewSandboxBackendFactory()
	}
	if cfg.ConfigPath != "" {
		v1Cfg, v1Err := config.LoadRuntimeWorkspaceConfigV1(cfg.ConfigPath)
		if v1Err == nil {
			// Apply config values only when flag/env did not set them (pre-empty).
			if v1Cfg.Model.Provider != "" && preProvider == "" {
				cfg.InferenceProvider = v1Cfg.Model.Provider
			}
			if v1Cfg.Model.Name != "" && preModel == "" {
				cfg.InferenceModel = v1Cfg.Model.Name
			}
			if v1Cfg.Sandbox.Backend != "" && preSandboxBackend == "" {
				cfg.SandboxBackend = v1Cfg.Sandbox.Backend
			}
			if strings.TrimSpace(v1Cfg.Audit.Enforcement) != "" && strings.EqualFold(strings.TrimSpace(cfg.AuditEnforcement), "strict") {
				cfg.AuditEnforcement = v1Cfg.Audit.Enforcement
			}
		}
		// Also inspect the flat RuntimeWorkspaceConfig for fields still
		// honored from older workspace files (TapePath, Agents,
		// AllowedCapabilities, runtime state).
		// Provider/model/sandbox_backend are also taken from the flat file
		// when the V1 parse does not supply them.
		if loaded, err := config.LoadRuntimeWorkspaceConfig(cfg.ConfigPath); err == nil {
			workspaceCfg = loaded
			if loaded.TapePath != "" && preTapePath == "" {
				cfg.InferenceTapePath = loaded.TapePath
			}
			if loaded.Provider != "" && preProvider == "" {
				cfg.InferenceProvider = loaded.Provider
			}
			if loaded.Model != "" && preModel == "" {
				cfg.InferenceModel = loaded.Model
			}
			if loaded.SandboxBackend != "" && preSandboxBackend == "" {
				cfg.SandboxBackend = loaded.SandboxBackend
			}
			if len(loaded.Agents) > 0 && cfg.AgentName == "" {
				cfg.AgentName = loaded.Agents[0]
			}
			allowedCapabilities = append(allowedCapabilities, convertRuntimeCapabilitySelectors(loaded.AllowedCapabilities)...)
		}
	} // end if cfg.ConfigPath != ""
	if strings.EqualFold(strings.TrimSpace(cfg.InferenceProvider), "tape") && strings.TrimSpace(cfg.InferenceTapePath) == "" {
		cfg.InferenceTapePath = config.DefaultWorkspaceStateTapeFile(cfg.Workspace)
	}

	// Build provider registry from the catalog and resolve the selected provider.
	// Defaults for Kind, Endpoint, Timeout, and NativeToolCalling come from the
	// catalog definition; explicit flag/env values take precedence.
	providerReg, _ := buildProviderRegistry(loadedConfig.Model.Providers)

	inferenceKind := cfg.InferenceProvider // name-as-kind fallback
	inferenceEndpoint := cfg.InferenceEndpoint
	inferenceTimeout := time.Duration(0)
	inferenceNativeToolCalling := cfg.InferenceNativeToolCalling

	if providerReg != nil {
		if def, found := providerReg.Resolve(cfg.InferenceProvider); found {
			inferenceKind = def.Kind
			if !endpointExplicit {
				inferenceEndpoint = def.Endpoint
			}
			if def.RequestTimeoutSeconds > 0 {
				inferenceTimeout = time.Duration(def.RequestTimeoutSeconds) * time.Second
			}
			inferenceNativeToolCalling = def.NativeToolCalling
		}
	}

	// App-level environment composition starts here. The runtime consumes the
	// resulting products directly.
	contract, err := config.OverlaySecurityBundle(euclocontract.DefaultContract(), &loadedConfig.Security)
	if err != nil {
		return nil, fmt.Errorf("overlay security bundle: %w", err)
	}
	docSnapshot := builtinDocumentSnapshot(contract, cfg.Workspace)
	agentSpec := aconvert.ConvertAgentSpec(contract.AgentSpec)
	contractPerms := contract.Permissions
	securityBundle := loadedConfig.Security
	profileRegistry, err := modelselect.BuildProfileRegistry(loadedConfig.Model.Profiles)
	if err != nil {
		return nil, fmt.Errorf("load model profiles: %w", err)
	}
	profileResolution := profileRegistry.Resolve(cfg.InferenceProvider, cfg.InferenceModel)
	backendProfile := aconvert.ConvertProfileConfig(profileResolution.Profile)
	registration, err := fauthorization.RegisterAgent(ctx, fauthorization.RuntimeConfig{
		DocumentSnapshot: docSnapshot,
		AgentSpec:        contract.AgentSpec,
		Permissions:      contractPerms,
		Security: fauthorization.SandboxSecurity{
			RunAsUser:       contract.Security.RunAsUser,
			ReadOnlyRoot:    contract.Security.ReadOnlyRoot,
			NoNewPrivileges: contract.Security.NoNewPrivileges,
		},
		Image:            "",
		Runtime:          "",
		ProtectedPaths:   securityBundle.Sandbox.ProtectedPaths,
		ConfigPath:       cfg.ConfigPath,
		Backend:          cfg.SandboxBackend,
		BackendFactory:   backendFactory,
		AuditLimit:       cfg.AuditLimit,
		AuditEnforcement: cfg.AuditEnforcement,
		BaseFS:           cfg.Workspace,
		StateDir:         config.DefaultWorkspaceStateDir(cfg.Workspace),
		HITLTimeout:      cfg.HITLTimeout,
		WorkspaceID:      filepath.Base(cfg.Workspace),
		AgentName:        contract.AgentID,
	})
	if err != nil {
		return nil, fmt.Errorf("compose authorization registration: %w", err)
	}
	var securityRuntime *session.RuntimeSecurity
	var capabilityProduct *session.CapabilityProduct
	var knowledgeProduct *session.KnowledgeProduct
	var modelProduct *envcomposition.ModelRuntime
	securityProduct, err := envcomposition.BuildSecurityRuntime(ctx, envcomposition.SecurityRuntimeInput{
		Context:           ctx,
		Workspace:         cfg.Workspace,
		SandboxBackend:    cfg.SandboxBackend,
		Runtime:           "",
		Image:             "",
		AgentID:           registration.ID,
		AgentSpec:         agentSpec,
		SecurityBundle:    &securityBundle,
		Security:          contract.Security,
		PermissionManager: registration.Permissions,
		ExistingRunner:    cfg.SecurityRunner,
		// Sandbox posture events (ceiling exceed, protected-path escape, image
		// pin status) ride the workspace log sink until the full telemetry
		// chain is assembled at workspace open (SBH-1 D-12/13/14).
		Events: telemetry.LoggerTelemetry{Logger: log.Default()},
	})
	if err != nil {
		return nil, fmt.Errorf("compose security runtime: %w", err)
	}
	securityRuntime = &session.RuntimeSecurity{
		Runner:        securityProduct.Runner,
		PolicyEngine:  securityProduct.PolicyEngine,
		CommandPolicy: securityProduct.CommandPolicy,
		Permissions:   securityProduct.Permissions,
		RunnerConfig:  securityProduct.RunnerConfig,
	}
	capProduct, err := envcomposition.BuildCapabilityRuntime(ctx, cfg.Workspace, securityProduct.Runner, envcomposition.CapabilityRuntimeOptions{
		AgentID:           registration.ID,
		PermissionManager: registration.Permissions,
		AgentSpec:         agentSpec,
		ProtectedPaths:    securityBundle.Sandbox.ProtectedPaths,
		InferenceEndpoint: cfg.InferenceEndpoint,
		InferenceModel:    cfg.InferenceModel,
		PrivateEgress:     envcomposition.NewPrivateEgressApprover(registration.Permissions),
		NetworkIsolation:  &cfg.Sandbox.NetworkIsolation,
	})
	if err != nil {
		return nil, fmt.Errorf("compose capability runtime: %w", err)
	}
	capabilityProduct = &session.CapabilityProduct{
		Registry:     capProduct.Registry,
		IndexManager: capProduct.IndexManager,
		SearchEngine: capProduct.SearchEngine,
	}
	knowledgeRuntime, err := envcomposition.BuildKnowledgeRuntime(envcomposition.KnowledgeRuntimeInput{
		GraphDB:       capProduct.IndexManager.GraphDB,
		Index:         capProduct.IndexManager,
		WorkspaceRoot: cfg.Workspace,
	})
	if err != nil {
		return nil, fmt.Errorf("compose knowledge runtime: %w", err)
	}
	knowledgeProduct = &session.KnowledgeProduct{
		KnowledgeStore:  knowledgeRuntime.KnowledgeStore,
		KnowledgeEvents: knowledgeRuntime.KnowledgeEvents,
		Retriever:       knowledgeRuntime.Retriever,
		Compiler:        knowledgeRuntime.Compiler,
		StreamTrigger:   knowledgeRuntime.StreamTrigger,
		Grounding:       knowledgeRuntime.Grounding,
		Invalidation:    knowledgeRuntime.Invalidation,
		Drain:           knowledgeRuntime.Drain,
		Health:          knowledgeRuntime.Health,
		CloseRetriever:  knowledgeRuntime.CloseRetriever,
	}
	modelProduct, err = envcomposition.BuildModelRuntime(envcomposition.ModelRuntimeInput{
		Provider:          cfg.InferenceProvider,
		Kind:              inferenceKind,
		Endpoint:          inferenceEndpoint,
		ModelName:         cfg.InferenceModel,
		TapePath:          cfg.InferenceTapePath,
		NativeToolCalling: inferenceNativeToolCalling,
		Timeout:           inferenceTimeout,
		Secrets:           llm.ProviderSecrets{APIKey: secrets.LLMAPIKey},
		Profile:           backendProfile,
	})
	if err != nil {
		return nil, fmt.Errorf("compose model runtime: %w", err)
	}
	if cfg.ModelFactoryWrapper != nil && modelProduct.ModelFactory != nil {
		modelProduct.ModelFactory = cfg.ModelFactoryWrapper(modelProduct.ModelFactory)
	}
	registrationView := &session.Registration{
		ID:          registration.ID,
		AgentSpec:   agentSpec,
		Permissions: registration.Permissions,
		Policy:      registration.Policy,
		Audit:       registration.Audit,
		HITL:        registration.HITL,
	}
	ws, err := session.OpenWorkspace(ctx, session.WorkspaceConfig{
		Workspace:                  cfg.Workspace,
		InferenceProvider:          cfg.InferenceProvider,
		InferenceEndpoint:          inferenceEndpoint,
		InferenceModel:             cfg.InferenceModel,
		InferenceNativeToolCalling: inferenceNativeToolCalling,
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
		AllowedCapabilities:        allowedCapabilities,
		Strict:                     envOverrides.Strict,
		LoadedConfig:               loadedConfig,
		DocumentSnapshot:           docSnapshot,
		Contract:                   contract,
		ProfileResolution:          profileResolution,
		SecurityBundle:             &securityBundle,
		Registration:               registrationView,
		SecurityRuntime:            securityRuntime,
		CapabilityProduct:          capabilityProduct,
		KnowledgeProduct:           knowledgeProduct,
		ModelProduct: &model.ModelProduct{
			Backend:      modelProduct.Backend,
			ModelFactory: modelProduct.ModelFactory,
		},
		EventLogFactory: openRuntimeEventLogFactory,
		Scope:           session.ScopeFull,
	})
	if err != nil {
		return nil, err
	}

	sess := session.NewSessionFromWorkspace(ws, cfg.Workspace)
	env := ws.Environment
	logger := ws.Logger
	baseTelemetry := ws.Telemetry
	if registration != nil && registration.Permissions != nil {
		bashCfg := &fauthorization.BashConfig{Default: permissions.DecisionAsk}
		if spec, ok := registration.AgentSpec.(*config.AgentSpec); ok && spec != nil {
			decision, derr := permissions.DecisionOr(string(spec.Bash.Default), permissions.DecisionAsk)
			if derr != nil {
				return nil, fmt.Errorf("agent bash default: %w", derr)
			}
			bashCfg = &fauthorization.BashConfig{
				AllowPatterns: spec.Bash.AllowPatterns,
				DenyPatterns:  spec.Bash.DenyPatterns,
				Default:       decision,
			}
		}
		authPolicy := fauthorization.NewCommandAuthorizationPolicy(registration.Permissions, registration.ID, bashCfg, "runtime")
		cfg.CommandPolicy = sandbox.CommandPolicyFunc(func(ctx context.Context, req sandbox.CommandRequest) error {
			return authPolicy.CheckCommand(ctx, req.Args, req.Env)
		})
	}

	// The causal event log mirror lives in the workspace telemetry chain
	// (opened via the composition root's EventLogFactory, FR-4). Surface a
	// warning when it is not available so operators see the JSONL-only
	// downgrade (NFR-4), and stamp the contract-fingerprint reload event.
	if env.EventLog == nil && cfg.EventsPath != "" {
		logger.Printf("warning: framework event log not available from workspace (JSONL-only telemetry)")
	} else if env.EventLog != nil && docSnapshot != nil && docSnapshot.SourcePath != "" {
		emitDocumentReloadedEvent(ctx, env.EventLog, registration.ID, cfg.AgentLabel(), docSnapshot)
	}

	execSink := telemetry.NewBroadcastSink()
	ws.Telemetry = telemetry.MultiplexTelemetry{
		Sinks: []telemetry.Telemetry{baseTelemetry, execSink},
	}

	// Register relurpic capabilities (subagent-backed; cannot be done in ayenitd).

	// Use WorkflowStore interface directly
	rt := &Runtime{
		Config:               cfg,
		Workspace:            ws,
		Session:              sess,
		Tools:                env.Registry,
		Memory:               env.WorkingMemory,
		Model:                env.Model,
		Compiler:             env.Compiler,
		IndexManager:         env.IndexManager,
		GraphDB:              graphDBFromIndexManager(env.IndexManager),
		SearchEngine:         env.SearchEngine,
		AgentLifecycle:       env.AgentLifecycle,
		WorkspaceConfig:      workspaceCfg,
		documentSnapshot:     docSnapshot,
		Delegations:          fauthorization.NewDelegationManager(),
		interactionEnvelopes: make(map[string]*contextdata.Envelope),
		secrets:              secrets,
		registration:         registration,
		modelBackend:         modelProduct.Backend,
		execSink:             execSink,
	}
	rt.Delegations.SetObserver(rt.observeDelegationSnapshot)
	if err := RegisterBuiltinProviders(ctx, rt); err != nil {
		_ = rt.Close(ctx)
		return nil, fmt.Errorf("register builtin providers: %w", err)
	}
	// Nexus gateway/node-provider registration is not wired in this runtime.

	if ws != nil && ws.Telemetry != nil {
		ev := telemetry.Event{
			Type:      telemetry.EventStateChange,
			Timestamp: time.Now().UTC(),
			Message:   "backend_selected",
			Metadata:  map[string]any{"provider": cfg.InferenceProvider},
		}
		telemetry.StampCorrelation(ctx, &ev)
		ws.Telemetry.Emit(ev)
	}

	agent, err := instantiateAgent(rt.paradigmDeps(), rt.hitlBroker())
	if err != nil {
		_ = rt.Close(ctx)
		return nil, fmt.Errorf("instantiate agent: %w", err)
	}

	// Enforce the effective (post-definition) tool policies before initializing.
	if env.Config != nil && env.Config.AgentSpec != nil {
		env.Registry.UseAgentSpec(registration.ID, env.Config.AgentSpec)
	}

	rt.Agent = agent
	emitAgentStartupEvent(ctx, env.EventLog, "local", registration.ID, cfg.AgentLabel(), agent)
	emitContractResolvedEvent(ctx, env.EventLog, "local", registration.ID, cfg.AgentLabel(), docSnapshot)
	if err := ayenitd.RegisterWorkspaceServices(ctx, ayenitd.WorkspaceConfig{Workspace: cfg.Workspace}, sess, rt.Tools, registration, ayenitd.WorkspaceServiceDeps{
		WorkspaceRoot: cfg.Workspace,
		EventBus:      env.KnowledgeEvents,
		IndexManager:  env.IndexManager,
		CommandPolicy: env.CommandPolicy,
		Telemetry:     rt.Workspace.Telemetry,
	}); err != nil {
		_ = rt.Close(ctx)
		return nil, fmt.Errorf("register workspace services: %w", err)
	}
	if err := ayenitd.StartWorkspaceServices(ctx, sess); err != nil {
		_ = rt.Close(ctx)
		return nil, fmt.Errorf("start workspace services: %w", err)
	}
	syncWorkspaceReadiness(rt)
	return rt, nil
}

func syncWorkspaceReadiness(rt *Runtime) {
	if rt == nil || rt.Workspace == nil {
		return
	}
	ws := rt.Workspace
	ws.Readiness.SandboxReady = rt.Tools != nil
	ws.Readiness.ModelReady = rt.Model != nil
	if ws.Readiness.Ready() {
		ws.Readiness.Degraded = false
	}
}

func newDegradedRuntime(ctx context.Context, cfg Config, reason error) *Runtime {
	ws := session.DegradedWorkspace(reason.Error())
	ws.Registration = &session.Registration{ID: "degraded"}

	id, _ := workspace.New(cfg.Workspace)
	sess := &session.WorkspaceSession{
		ID:        "degraded",
		Workspace: id,
	}

	rt := &Runtime{
		Config:    cfg,
		Workspace: ws,
		Session:   sess,
		Tools:     registry.NewRegistry(),
	}

	// Emit boot.degraded as a structured telemetry event (FR-16 / AC-10). A
	// degraded boot has no runtime telemetry chain, so a dedicated sink records
	// the structured event — always to stderr, best-effort to the workspace
	// JSONL — and the plain log line preserves a fast human-readable trace.
	ws.Telemetry = degradedTelemetrySink(cfg)
	emitBootDegraded(ctx, ws, reason)
	log.Printf("runtime degraded: reason=%q degraded=true", reason.Error())
	return rt
}

// degradedTelemetrySink assembles the telemetry sink for a degraded boot: a
// logger-backed sink (always) plus a best-effort JSONL file under the workspace
// state telemetry path so boot.degraded is durably queryable (AC-10). JSONL
// failure is non-fatal; a degraded boot never extends further on telemetry.
func degradedTelemetrySink(cfg Config) telemetry.Telemetry {
	sinks := []telemetry.Telemetry{telemetry.LoggerTelemetry{Logger: log.Default()}}
	stateDir := workspace.StateDir(cfg.Workspace)
	path := filepath.Join(stateDir, "telemetry", "workspace.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err == nil { // public: degraded telemetry dir
		if fileSink, err := telemetry.NewJSONFileTelemetry(path); err == nil {
			sinks = append(sinks, fileSink)
		}
	}
	if len(sinks) == 1 {
		return sinks[0]
	}
	return telemetry.MultiplexTelemetry{Sinks: sinks}
}

// emitBootDegraded emits the boot.degraded telemetry event on the degraded
// workspace's sink. The event is deliberately structured: reason, sandbox
// readiness, model readiness, and the degraded flag travel as metadata so the
// failure mode is diagnosable from the durable trail, not just a log line.
func emitBootDegraded(ctx context.Context, ws *session.Workspace, reason error) {
	if ws == nil || ws.Telemetry == nil || reason == nil {
		return
	}
	ev := telemetry.Event{
		Type:      telemetry.EventBootDegraded,
		Message:   "boot degraded",
		Timestamp: time.Now().UTC(),
		Metadata: map[string]any{
			"reason":        reason.Error(),
			"sandbox_ready": false,
			"model_ready":   false,
			"degraded":      true,
		},
	}
	// NFR-6: route through the sanctioned correlation stamper even though a
	// degraded boot carries no run context — consistency over special-casing.
	telemetry.StampCorrelation(ctx, &ev)
	ws.Telemetry.Emit(ev)
}

// Close releases resources managed by fruntime.
func (r *Runtime) Close(ctx context.Context) error {
	if r == nil {
		return nil
	}
	var errs []error

	providers := r.registeredProviders()
	for i := len(providers) - 1; i >= 0; i-- {
		if err := providers[i].Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if r.execSink != nil {
		r.execSink.Close()
		r.execSink = nil
	}

	// Close workspace (handles backend, services, logs, etc.)
	if r.Workspace != nil {
		if err := r.Workspace.Close(ctx); err != nil {
			errs = append(errs, err)
		}
		r.Workspace = nil
	}
	r.interactionMu.Lock()
	r.interactionEnvelopes = make(map[string]*contextdata.Envelope)
	r.interactionMu.Unlock()

	return errors.Join(errs...)
}

// ManifestFingerprint returns the fingerprint of the loaded manifest snapshot
// when available.
func (r *Runtime) ManifestFingerprint() string {
	if r == nil || r.registration == nil {
		return ""
	}
	ds, ok := r.registration.DocumentSnapshot.(*config.DocumentSnapshot)
	if !ok || ds == nil {
		return ""
	}
	return fmt.Sprintf("%x", ds.Fingerprint)
}

// ManifestDeprecationNotices returns manifest deprecation notices when available.
func (r *Runtime) ManifestDeprecationNotices() []string {
	if r == nil || r.registration == nil {
		return nil
	}
	ds, ok := r.registration.DocumentSnapshot.(*config.DocumentSnapshot)
	if !ok || ds == nil {
		return nil
	}
	return append([]string(nil), ds.Warnings...)
}

// AvailableAgents lists known agent presets and definitions.
func (r *Runtime) AvailableAgents() []string {
	return []string{AgentLabelEuclo}
}

// SwitchAgent reinitializes the runtime with a new agent preset.
func (r *Runtime) SwitchAgent(name string) error {
	if r == nil {
		return errors.New("runtime unavailable")
	}
	if name == "" {
		return errors.New("agent name required")
	}
	if r.Workspace.Registration == nil || r.Workspace.Registration.AgentSpec == nil {
		return errors.New("agent manifest missing")
	}
	effectiveContract, compiledPolicy, err := r.resolveEffectiveContractForAgent(name)
	if err != nil {
		return err
	}
	return r.applyResolvedAgentState(name, effectiveContract, compiledPolicy)
}

// ReloadEffectiveContract reapplies the effective contract and compiled policy
// for the currently selected agent using the same consolidated resolution path
// as startup and SwitchAgent.
func (r *Runtime) ReloadEffectiveContract() error {
	if r == nil {
		return errors.New("runtime unavailable")
	}
	name := strings.TrimSpace(r.Config.AgentName)
	if name == "" && r.Workspace.Registration != nil {
		name = strings.TrimSpace(r.Workspace.Registration.ID)
	}
	if name == "" {
		return errors.New("agent name required")
	}
	effectiveContract, compiledPolicy, err := r.resolveEffectiveContractForAgent(name)
	if err != nil {
		return err
	}
	return r.applyResolvedAgentState(name, effectiveContract, compiledPolicy)
}

func (r *Runtime) applyResolvedAgentState(name string, effectiveContract *config.EffectiveAgentContract, compiledPolicy *session.CompiledPolicy) error {
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
	agentSpecCap := aconvert.ConvertAgentSpec(effectiveContract.AgentSpec)
	if agentSpecCap == nil {
		return fmt.Errorf("agent spec required")
	}
	agentCfg := &execution.Config{
		Name:              cfg.AgentLabel(),
		Model:             cfg.InferenceModel,
		MaxIterations:     8,
		NativeToolCalling: agentSpecCap.NativeToolCallingEnabled(),
		AgentSpec:         agentSpecCap,
		Telemetry:         r.Workspace.Telemetry,
	}
	agent, err := instantiateAgent(r.switchAgentDeps(agentCfg), r.hitlBroker())
	if err != nil {
		return fmt.Errorf("instantiate agent %q: %w", name, err)
	}
	r.Tools.UseAgentSpec(r.Workspace.Registration.ID, agentSpecCap)
	r.Workspace.Registration.Policy = nil
	r.Agent = agent
	r.Workspace.AgentSpec = agentSpecCap
	r.Workspace.EffectiveContract = effectiveContract
	r.Workspace.CompiledPolicy = compiledPolicy
	r.Workspace.CapabilityAdmissions = nil
	r.Config.AgentName = name
	return nil
}

// builtinDocumentSnapshot creates a minimal DocumentSnapshot for the built-in
// euclo contract. The fingerprint is computed from the contract and the raw
// security bundle bytes via config.ContractFingerprint.
func builtinDocumentSnapshot(contract *config.EffectiveAgentContract, workspace string) *config.DocumentSnapshot {
	if contract == nil {
		return nil
	}
	return &config.DocumentSnapshot{
		Document: &config.Document{
			APIVersion: "relurpify.io/v1",
			Kind:       "AgentManifest",
			Metadata:   config.DocumentMetadata{Name: contract.AgentID},
		},
		Fingerprint: config.ContractFingerprint(contract, workspace),
		SourcePath:  "",
		LoadedAt:    time.Now().UTC(),
	}
}

// instantiateAgent builds the euclo workflow executor. The HITL broker is
// required by the execution graph; passing nil makes graph construction fail
// closed at execution time.
func instantiateAgent(deps *paradigm.Deps, hitl euclopolicy.HITLBroker) (agentgraph.WorkflowExecutor, error) {
	if deps == nil || deps.Registry == nil {
		return nil, fmt.Errorf("instantiate euclo: capability registry is required")
	}
	return euclo.New(
		deps,
		euclo.WithCheckpointRepository(deps.AgentLifecycle),
		euclo.WithHITLBroker(hitl),
	), nil
}

// hitlBroker returns the HITL broker registered at boot, or nil when the
// runtime is degraded. A nil broker makes euclo graph construction fail closed.
func (r *Runtime) hitlBroker() euclopolicy.HITLBroker {
	if r == nil || r.registration == nil {
		return nil
	}
	return r.registration.HITL
}

func (r *Runtime) paradigmDeps() *paradigm.Deps {
	return &paradigm.Deps{
		Config:            r.Workspace.Environment.Config,
		Model:             r.Model,
		Registry:          r.Tools,
		PermissionChecker: r.permissionChecker(),
		CommandRunner:     r.Workspace.Environment.CommandRunner,
		CommandPolicy:     r.Workspace.Environment.CommandPolicy,
		WorkingMemory:     r.Memory,
		IndexManager:      r.IndexManager,
		SearchEngine:      r.SearchEngine,
		StreamTrigger:     r.Workspace.Environment.StreamTrigger,
		OutputIngester:    r.Workspace.Environment.OutputIngester,
		IngestOutputs:     r.Workspace.Environment.IngestOutputs,
		Grounder:          r.Workspace.Environment.Grounding,
		EpochDrain:        r.Workspace.Environment.KnowledgeDrain,
		PromptRegistry:    r.Workspace.Environment.PromptRegistry,
		AgentLifecycle:    r.AgentLifecycle,
		Telemetry:         r.Workspace.Telemetry,
	}
}

// permissionChecker returns the agent authorization bundle's capability
// checker, or nil when the runtime is degraded. Governed paradigms fail
// closed on a nil checker.
func (r *Runtime) permissionChecker() permissions.CapabilityChecker {
	if r == nil || r.registration == nil {
		return nil
	}
	return r.registration.Permissions
}

func (r *Runtime) switchAgentDeps(agentCfg *execution.Config) *paradigm.Deps {
	deps := r.paradigmDeps()
	if deps == nil {
		return nil
	}
	deps.Config = agentCfg
	return deps
}

func emitAgentStartupEvent(ctx context.Context, eventLog event.Log, partition, agentID, label string, agent agentgraph.WorkflowExecutor) {
	if eventLog == nil {
		return
	}
	if strings.TrimSpace(partition) == "" {
		partition = "local"
	}
	payload := map[string]any{
		"agent_id":      agentID,
		"agent_label":   label,
		"executor_type": fmt.Sprintf("%T", agent),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_, _ = eventLog.Append(ctx, partition, []event.FrameworkEvent{{
		Timestamp: time.Now().UTC(),
		Type:      event.EventAgentRunStarted,
		Payload:   data,
		Actor:     observability.Actor{Kind: "agent", ID: agentID, Label: label},
		Partition: partition,
	}})
}

func emitContractResolvedEvent(ctx context.Context, eventLog event.Log, partition, agentID, label string, snapshot *config.DocumentSnapshot) {
	if eventLog == nil || snapshot == nil {
		return
	}
	if strings.TrimSpace(partition) == "" {
		partition = "local"
	}
	payload := map[string]any{
		"contract_source": "builtin+split",
		"fingerprint":     fmt.Sprintf("%x", snapshot.Fingerprint),
		"agent_id":        agentID,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_, _ = eventLog.Append(ctx, partition, []event.FrameworkEvent{{
		Timestamp: time.Now().UTC(),
		Type:      event.EventContractResolved,
		Payload:   data,
		Actor:     observability.Actor{Kind: "agent", ID: agentID, Label: label},
		Partition: partition,
	}})
}

func (r *Runtime) resolveEffectiveContractForAgent(name string) (*config.EffectiveAgentContract, *session.CompiledPolicy, error) {
	if strings.TrimSpace(name) == "" {
		return nil, nil, fmt.Errorf("agent name required")
	}
	// S2: euclo is the only agent; use the built-in contract.
	effectiveContract := euclocontract.DefaultContract()
	if effectiveContract.AgentID == "" {
		return nil, nil, fmt.Errorf("agent id required for agent %q", name)
	}
	agentSpecCap := aconvert.ConvertAgentSpec(effectiveContract.AgentSpec)
	if agentSpecCap == nil {
		return nil, nil, fmt.Errorf("agent spec required for agent %q", name)
	}
	compiledPolicy := &session.CompiledPolicy{
		AgentID: effectiveContract.AgentID,
		Spec:    agentSpecCap,
		Engine:  r.Workspace.PolicyEngine,
	}
	return effectiveContract, compiledPolicy, nil
}

// RunTask executes a task against the configured agent while preserving shared
// context state for future status screens.
func (r *Runtime) RunTask(ctx context.Context, task *execution.Task) (*execution.Result, error) {
	if task == nil {
		return nil, errors.New("task required")
	}
	return r.executeTask(ctx, task)
}

// ActiveWorkflowID returns the lifecycle workflow ID of the run currently
// executing, or "" when no run is active. The TUI autosave persists it so
// session records are resumable.
func (r *Runtime) ActiveWorkflowID() string {
	if r == nil {
		return ""
	}
	r.activeWorkflowMu.RLock()
	defer r.activeWorkflowMu.RUnlock()
	return r.activeWorkflowID
}

func (r *Runtime) setActiveWorkflowID(id string) {
	r.activeWorkflowMu.Lock()
	r.activeWorkflowID = id
	r.activeWorkflowMu.Unlock()
}

// clearActiveWorkflowID clears the active ID only when it still names the
// run that is finishing, so a newer run's ID is never cleared by an older
// one's defer.
func (r *Runtime) clearActiveWorkflowID(id string) {
	if id == "" {
		return
	}
	r.activeWorkflowMu.Lock()
	if r.activeWorkflowID == id {
		r.activeWorkflowID = ""
	}
	r.activeWorkflowMu.Unlock()
}

// ErrRuntimeDegraded reports a task submission against a runtime that failed to
// build. It is errors.As/Is-compatible; Reason carries the boot failure text.
// The error flows through the existing turn error channel so the user sees an
// honest rejection instead of the nil-agent panic a degraded runtime would
// otherwise produce.
type ErrRuntimeDegraded struct{ Reason string }

func (e *ErrRuntimeDegraded) Error() string {
	return "runtime degraded, task rejected: " + e.Reason
}

// Is matches the degraded category, so a caller can classify a failure with
// errors.Is(err, &ErrRuntimeDegraded{}) without holding the originating value.
func (e *ErrRuntimeDegraded) Is(target error) bool {
	_, ok := target.(*ErrRuntimeDegraded)
	return ok
}

// degradationReason resolves the boot failure that produced a degraded
// runtime. A degraded workspace stores the reason on Readiness (set by
// session.DegradedWorkspace); a runtime assembled without one falls back to a
// stable constant so ErrRuntimeDegraded.Error() is never bare.
func (r *Runtime) degradationReason() string {
	if r != nil && r.Workspace != nil {
		if reason := strings.TrimSpace(r.Workspace.Readiness.Reason); reason != "" {
			return reason
		}
	}
	return "boot failed"
}

// emitTaskRejected records the rejection on the runtime's telemetry sink. The
// guard in executeTask runs before envelope assembly and lifecycle
// bookkeeping, so this event is the only record a rejected task leaves behind.
func (r *Runtime) emitTaskRejected(ctx context.Context, taskID, reason string) {
	if r == nil || r.Workspace == nil || r.Workspace.Telemetry == nil {
		return
	}
	ev := telemetry.Event{
		Type:      telemetry.EventTaskRejected,
		Message:   "task rejected",
		Timestamp: time.Now().UTC(),
		TaskID:    strings.TrimSpace(taskID),
		Metadata:  map[string]any{"reason": reason},
	}
	telemetry.StampCorrelation(ctx, &ev)
	r.Workspace.Telemetry.Emit(ev)
}

// executeTask is the single turn-execution path shared by RunTask and
// ResumeSession so the two cannot diverge: envelope assembly, lifecycle
// bookkeeping (workflow + run records, active workflow ID tracking), agent
// execution, and working-memory eviction.
func (r *Runtime) executeTask(ctx context.Context, task *execution.Task) (*execution.Result, error) {
	if r.Agent == nil {
		// A degraded boot deliberately produces Agent == nil (newDegradedRuntime).
		// Reject before envelope assembly and lifecycle bookkeeping so the task
		// leaves no interaction envelope and no workflow/run records.
		reason := r.degradationReason()
		r.emitTaskRejected(ctx, task.ID, reason)
		return nil, &ErrRuntimeDegraded{Reason: reason}
	}
	env := contextdata.NewEnvelope(task.ID, r.ensureSessionID())
	env.SetNodeID("runtime")
	if task.Context != nil {
		for key, value := range task.Context {
			env.SetWorkingValueWithClass(key, value, contextdata.MemoryClassTask)
		}
	}
	if task.Metadata != nil {
		for key, value := range task.Metadata {
			env.SetWorkingValueWithClass("meta."+key, value, contextdata.MemoryClassTask)
		}
	}
	r.trackInteractionEnvelope(task.ID, env)
	if err := r.Agent.Initialize(&execution.Config{Workspace: r.Config.Workspace}); err != nil {
		return nil, fmt.Errorf("initialize agent: %w", err)
	}
	workflowID, runID := r.beginWorkflow(task)
	result, err := r.Agent.Execute(r.beginTurn(ctx, env), task, env)
	// Task completion ends the task's working-memory lifetime: the result is
	// in hand, so the per-task entries are released (idempotent no-op when
	// the checkpoint boundary already evicted them).
	if r.Memory != nil {
		r.Memory.Evict(task.ID)
	}
	r.endWorkflow(workflowID, runID, err)
	if result != nil && workflowID != "" {
		if result.Metadata == nil {
			result.Metadata = make(map[string]any)
		}
		result.Metadata["workflow_id"] = workflowID
	}
	return result, err
}

// beginWorkflow creates the lifecycle records for one executed turn and
// marks its workflow as the runtime's active one. When no lifecycle
// repository is wired (degraded boot, tests) the turn executes untracked.
func (r *Runtime) beginWorkflow(task *execution.Task) (workflowID, runID string) {
	if r.AgentLifecycle == nil {
		return "", ""
	}
	ctx := context.Background()
	workflowID = "wf-" + observability.NewRunID()
	meta := map[string]any{"instruction": task.Instruction}
	if task.Type != "" {
		meta["type"] = task.Type
	}
	now := time.Now()
	if err := r.AgentLifecycle.CreateWorkflow(ctx, agentlifecycle.WorkflowRecord{
		WorkflowID: workflowID,
		CreatedAt:  now,
		UpdatedAt:  now,
		Metadata:   meta,
	}); err != nil {
		return "", ""
	}
	runID = "run-" + observability.NewRunID()
	if err := r.AgentLifecycle.CreateRun(ctx, agentlifecycle.WorkflowRunRecord{
		RunID:      runID,
		WorkflowID: workflowID,
		Status:     "running",
		StartedAt:  now,
		Metadata:   map[string]any{"instruction": task.Instruction},
	}); err != nil {
		return workflowID, ""
	}
	r.setActiveWorkflowID(workflowID)
	return workflowID, runID
}

// endWorkflow finalizes the lifecycle records of a completed turn and clears
// the active workflow ID.
func (r *Runtime) endWorkflow(workflowID, runID string, runErr error) {
	if workflowID == "" {
		return
	}
	r.clearActiveWorkflowID(workflowID)
	if r.AgentLifecycle == nil || runID == "" {
		return
	}
	status := "completed"
	if runErr != nil {
		status = "failed"
	}
	_ = r.AgentLifecycle.UpdateRunStatus(context.Background(), runID, status)
}

// ensureSessionID returns the runtime-scoped correlation session ID, generating
// one on first use so Runtime values constructed directly (tests, degraded
// boots) still correlate.
func (r *Runtime) ensureSessionID() string {
	r.sessionIDMu.Lock()
	defer r.sessionIDMu.Unlock()
	if r.sessionID == "" {
		r.sessionID = observability.NewSessionID()
	}
	return r.sessionID
}

// beginTurn starts a new correlation scope for one turn: it generates a fresh
// RunID and TraceID, attaches them to ctx for every downstream emitter to read
// via observability.RunContextFromContext, and mirrors the session identity onto
// the envelope. Called once per turn (including interaction resumes).
func (r *Runtime) beginTurn(ctx context.Context, env *contextdata.Envelope) context.Context {
	sessionID := r.ensureSessionID()
	if env != nil && env.SessionIDSnapshot() != "" {
		sessionID = env.SessionIDSnapshot()
	}
	if env != nil {
		env.SetSessionID(sessionID)
	}
	agentID := ""
	if r.registration != nil {
		agentID = r.registration.ID
	}
	// Attach the envelope to the context in addition to the run context
	// so every emitter inside the turn — including the LLM
	// instrumentation, which reads the envelope for task attribution —
	// sees the same task and session identifiers.
	if env != nil {
		ctx = contextdata.WithEnvelope(ctx, env)
	}
	return observability.WithRunContext(ctx, observability.RunContext{
		SessionID: sessionID,
		RunID:     observability.NewRunID(),
		TraceID:   observability.NewTraceID(),
		AgentID:   agentID,
	})
}

func (r *Runtime) submitTurn(ctx context.Context, instruction string, taskType execution.TaskType, metadata map[string]any, callback func(string)) (*execution.Result, error) {
	if taskType == "" {
		taskType = execution.TaskTypeExecute
	}
	if callback != nil {
		if metadata == nil {
			metadata = make(map[string]any)
		}
		metadata["stream_callback"] = callback
	}

	task := &execution.Task{
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

// ResolveInteractionFrame writes a UI response back into the live envelope for
// the given task and frame, then persists Euclo clarification state when the
// frame is clarification-scoped.
func (r *Runtime) ResolveInteractionFrame(ctx context.Context, taskID, frameID, choice, freetext string) error {
	if r == nil {
		return fmt.Errorf("runtime unavailable")
	}
	env := r.interactionEnvelope(taskID)
	if env == nil {
		return fmt.Errorf("interaction envelope for task %q not available", taskID)
	}
	frame, ok := findInteractionFrame(env, frameID)
	if !ok || frame == nil {
		return fmt.Errorf("interaction frame %q not found", frameID)
	}
	answer := strings.TrimSpace(choice)
	if answer == "" {
		answer = strings.TrimSpace(freetext)
	}
	if answer == "" {
		answer = defaultInteractionAnswer(frame)
	}
	extra := map[string]any{
		"task_id":      strings.TrimSpace(taskID),
		"frame_id":     strings.TrimSpace(frame.ID),
		"frame_type":   string(frame.Type),
		"resolved_via": "relurpish",
	}
	if strings.TrimSpace(freetext) != "" {
		extra["freetext"] = strings.TrimSpace(freetext)
	}
	frame.SetResponse(answer, extra, "relurpish", time.Now().UTC())
	if err := r.persistInteractionResolution(ctx, env, frame); err != nil {
		return err
	}
	if interaction.ShouldResumeExecution(frame.Type) {
		if _, err := r.resumeInteractionTask(ctx, env); err != nil {
			return fmt.Errorf("resume interaction task: %w", err)
		}
	}
	return nil
}

// SubmitTurn is the canonical turn submission entry used by the TUI.
func (r *Runtime) SubmitTurn(ctx context.Context, instruction string, taskType execution.TaskType, metadata map[string]any, callback func(string)) (*execution.Result, error) {
	return r.submitTurn(ctx, instruction, taskType, metadata, callback)
}

// ExecuteInstruction convenience helper.
func (r *Runtime) ExecuteInstruction(ctx context.Context, instruction string, taskType execution.TaskType, metadata map[string]any) (*execution.Result, error) {
	return r.submitTurn(ctx, instruction, taskType, metadata, nil)
}

// ExecuteInstructionStream is like ExecuteInstruction but wires a streaming
// callback so the LLM emits tokens incrementally via callback as they arrive.
func (r *Runtime) ExecuteInstructionStream(ctx context.Context, instruction string, taskType execution.TaskType, metadata map[string]any, callback func(string)) (*execution.Result, error) {
	return r.submitTurn(ctx, instruction, taskType, metadata, callback)
}

func (r *Runtime) trackInteractionEnvelope(taskID string, env *contextdata.Envelope) {
	if r == nil || env == nil {
		return
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return
	}
	r.interactionMu.Lock()
	if r.interactionEnvelopes == nil {
		r.interactionEnvelopes = make(map[string]*contextdata.Envelope)
	}
	r.interactionEnvelopes[taskID] = env
	r.interactionMu.Unlock()
}

func (r *Runtime) interactionEnvelope(taskID string) *contextdata.Envelope {
	if r == nil {
		return nil
	}
	r.interactionMu.Lock()
	defer r.interactionMu.Unlock()
	return r.interactionEnvelopes[strings.TrimSpace(taskID)]
}

func (r *Runtime) persistInteractionResolution(ctx context.Context, env *contextdata.Envelope, frame *interaction.InteractionFrame) error {
	if env == nil || frame == nil {
		return nil
	}
	if frame.Type != interaction.FrameIntentClarification {
		env.SetWorkingValueWithClass("euclo.interaction.frame_requested", false, contextdata.MemoryClassTask)
		return nil
	}
	store := intentcontext.NewStateStore()
	state, err := store.Read(ctx, env)
	if err != nil {
		return err
	}
	if state == nil {
		state = intentcontext.NewState(env.TaskIDSnapshot(), env.SessionIDSnapshot())
	}
	turn := interaction.ClarificationTurnFromFrame(frame, state.StateVersion)
	if turn != nil {
		state.Turns = append(state.Turns, *turn)
		state.CurrentTurnID = turn.TurnID
		state.StateVersion = intentcontext.NextStateVersion(state.StateVersion)
		state.LastUpdatedAt = time.Now().UTC()
		if frame.Resume != nil && strings.TrimSpace(frame.Resume.ActiveThoughtRecipeID) != "" {
			state.ActiveThoughtRecipeID = strings.TrimSpace(frame.Resume.ActiveThoughtRecipeID)
		}
	}
	if err := store.Write(ctx, env, state); err != nil {
		return err
	}
	env.SetWorkingValueWithClass("euclo.interaction.frame_requested", false, contextdata.MemoryClassTask)
	return nil
}

func (r *Runtime) resumeInteractionTask(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error) {
	if r == nil {
		return nil, fmt.Errorf("runtime unavailable")
	}
	if env == nil {
		return nil, fmt.Errorf("interaction envelope unavailable")
	}
	value, ok := contextdata.GetTyped[any](env, euclostate.KeyTaskInput)
	if !ok || value == nil {
		return nil, fmt.Errorf("task input unavailable for resume")
	}
	task, ok := value.(*execution.Task)
	if !ok || task == nil {
		return nil, fmt.Errorf("task input has unexpected type %T", value)
	}
	if r.Agent == nil {
		return nil, &ErrRuntimeDegraded{Reason: r.degradationReason()}
	}
	return r.Agent.Execute(r.beginTurn(ctx, env), task, env)
}

func findInteractionFrame(env *contextdata.Envelope, frameID string) (*interaction.InteractionFrame, bool) {
	if env == nil {
		return nil, false
	}
	frameID = strings.TrimSpace(frameID)
	if frameID == "" {
		return nil, false
	}
	for _, key := range env.WorkingMemoryKeys() {
		value, ok := contextdata.GetTyped[any](env, key)
		if !ok {
			continue
		}
		frame, ok := value.(*interaction.InteractionFrame)
		if !ok || frame == nil {
			continue
		}
		if strings.TrimSpace(frame.ID) == frameID {
			return frame, true
		}
	}
	return nil, false
}

func defaultInteractionAnswer(frame *interaction.InteractionFrame) string {
	if frame == nil {
		return ""
	}
	if slot := strings.TrimSpace(frame.DefaultChoice); slot != "" {
		return slot
	}
	for _, slot := range frame.Slots {
		if slot.Default && strings.TrimSpace(slot.ID) != "" {
			return strings.TrimSpace(slot.ID)
		}
	}
	if len(frame.Slots) > 0 {
		return strings.TrimSpace(frame.Slots[0].ID)
	}
	if len(frame.Choices) > 0 {
		return strings.TrimSpace(frame.Choices[0])
	}
	return ""
}

// ServerRunning reports whether the HTTP server is active.
func (r *Runtime) ServerRunning() bool {
	_ = r
	return false
}

// PendingHITL exposes outstanding permission requests.
func (r *Runtime) PendingHITL() []*fauthorization.PermissionRequest {
	if r.registration == nil || r.registration.HITL == nil {
		return nil
	}
	return r.registration.HITL.PendingRequests()
}

func emitDocumentReloadedEvent(ctx context.Context, eventLog event.Log, agentID, label string, snapshot *config.DocumentSnapshot) {
	if eventLog == nil || snapshot == nil {
		return
	}
	payload := map[string]any{
		"document_path": snapshot.SourcePath,
		"fingerprint":   hex.EncodeToString(snapshot.Fingerprint[:]),
		"warnings":      append([]string(nil), snapshot.Warnings...),
	}
	if data, err := json.Marshal(payload); err == nil {
		_, _ = eventLog.Append(ctx, "local", []event.FrameworkEvent{{
			Timestamp: time.Now().UTC(),
			Type:      event.EventManifestReloaded,
			Payload:   data,
			Actor:     observability.Actor{Kind: "agent", ID: agentID, Label: label},
			Partition: "local",
		}})
	}
}

// SubscribeHITL streams HITL lifecycle events (requested/resolved/expired).
// The returned cancel function can be called to unsubscribe.
func (r *Runtime) SubscribeHITL() (<-chan fauthorization.HITLEvent, func()) {
	if r == nil || r.registration == nil || r.registration.HITL == nil {
		ch := make(chan fauthorization.HITLEvent)
		close(ch)
		return ch, func() {}
	}
	return r.registration.HITL.Subscribe(32)
}

// SubscribeExecutionEvents streams execution lifecycle events (euclo step
// started/completed, recipe selected, branch resolved, tool edits, etc.)
// from the telemetry broadcast sink. Returns a receive channel and a cancel
// function. After Close, Subscribe returns an already-closed channel.
func (r *Runtime) SubscribeExecutionEvents() (<-chan telemetry.Event, func()) {
	if r == nil || r.execSink == nil {
		ch := make(chan telemetry.Event)
		close(ch)
		return ch, func() {}
	}
	return r.execSink.Subscribe(256)
}

// ApproveHITL approves a pending request with the supplied scope.
func (r *Runtime) ApproveHITL(requestID, approver string, scope policy.GrantScope, duration time.Duration) error {
	if r.registration == nil || r.registration.HITL == nil {
		return errors.New("hitl broker unavailable")
	}
	if scope == "" {
		scope = policy.GrantScopeOneTime
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
	return r.registration.HITL.Approve(decision)
}

// DenyHITL rejects a pending request.
func (r *Runtime) DenyHITL(requestID, reason string) error {
	if r.registration == nil || r.registration.HITL == nil {
		return errors.New("hitl broker unavailable")
	}
	return r.registration.HITL.Deny(requestID, reason)
}

package runtime

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
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

	hitlCancel func()

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
// Construction is total: recoverable failures (config parse errors,
// sandbox backend unavailable, model backend down) produce a degraded
// runtime with deny-all scope instead of returning an error.
// Truly fatal programmer errors (nil deref) still return an error.
func New(ctx context.Context, cfg Config, secrets config.Secrets) (*Runtime, error) {
	rt, err := buildRuntime(ctx, cfg, secrets)
	if err != nil {
		return newDegradedRuntime(ctx, cfg, secrets, err), nil
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
	// calling ayenitd.Open. The V1 config format (relurpify/workspace/v1)
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
		}
		// Also try the flat legacy RuntimeWorkspaceConfig for fields not
		// in V1 (TapePath, Agents, AllowedCapabilities, runtime state).
		// Also applies flat provider/model/sandbox_backend as fallback
		// when V1 parsing fails (backward compat with existing files).
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

	// App-level environment composition starts here. agentenv consumes the
	// resulting products while the old environment object is being dissolved.
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
		Image:          "",
		Runtime:        "",
		ProtectedPaths: securityBundle.Sandbox.ProtectedPaths,
		ConfigPath:     cfg.ConfigPath,
		Backend:        cfg.SandboxBackend,
		BackendFactory: backendFactory,
		AuditLimit:     cfg.AuditLimit,
		BaseFS:         cfg.Workspace,
		StateDir:       config.DefaultWorkspaceStateDir(cfg.Workspace),
		HITLTimeout:    cfg.HITLTimeout,
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
		GraphDB: capProduct.IndexManager.GraphDB,
		Index:   capProduct.IndexManager,
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
		Scope: session.ScopeFull,
	})
	if err != nil {
		return nil, err
	}

	sess := session.NewSessionFromWorkspace(ws, cfg.Workspace)
	env := ws.Environment
	logger := ws.Logger
	baseTelemetry := ws.Telemetry
	if registration != nil && registration.Permissions != nil {
		var bashCfg *fauthorization.BashConfig
		if spec, ok := registration.AgentSpec.(*config.AgentSpec); ok && spec != nil {
			bashCfg = &fauthorization.BashConfig{
				AllowPatterns: spec.Bash.AllowPatterns,
				DenyPatterns:  spec.Bash.DenyPatterns,
				Default:       string(spec.Bash.Default),
			}
		}
		authPolicy := fauthorization.NewCommandAuthorizationPolicy(registration.Permissions, registration.ID, bashCfg, "runtime")
		cfg.CommandPolicy = sandbox.CommandPolicyFunc(func(ctx context.Context, req sandbox.CommandRequest) error {
			return authPolicy.CheckCommand(ctx, req.Args, req.Env)
		})
	}

	// Extend telemetry with an event log sink. The event log is now created
	// by execution/agentenv via EventLogFactory, so we just need to wire it into
	// the telemetry chain.
	var eventTelemetry telemetry.EventTelemetry
	if cfg.EventsPath != "" && registration != nil {
		// The event log is now owned by Workspace and will be closed by Workspace.Close()
		// We need to get it from the Workspace's Environment
		if env.EventLog != nil {
			eventTelemetry = telemetry.EventTelemetry{
				Log:       env.EventLog,
				Partition: "local",
				Actor:     observability.Actor{Kind: "agent", ID: registration.ID, Label: cfg.AgentLabel()},
			}
			// Re-wire the permission event logger with full event log support.
			if registration.Permissions != nil {
				registration.Permissions.SetEventLogger(func(ctx context.Context, desc permissions.PermissionDescriptor, effect, reason string, fields map[string]any) {
					payload := map[string]any{
						"permission_type": desc.Type,
						"action":          desc.Action,
						"resource":        desc.Resource,
						"effect":          effect,
						"reason":          reason,
						"metadata":        fields,
					}
					if data, err := json.Marshal(payload); err == nil {
						_, _ = env.EventLog.Append(ctx, "local", []event.FrameworkEvent{{
							Timestamp: time.Now().UTC(),
							Type:      event.EventPolicyEvaluated,
							Payload:   data,
							Actor:     observability.Actor{Kind: "agent", ID: registration.ID, Label: cfg.AgentLabel()},
							Partition: "local",
						}})
					}
				})
			}
			// S2: built-in contract has no source path; skip reload event.
			// S8: replace with contract-fingerprint event.
			if docSnapshot != nil && docSnapshot.SourcePath != "" {
				emitDocumentReloadedEvent(ctx, env.EventLog, registration.ID, cfg.AgentLabel(), docSnapshot)
			}
		} else if logger != nil {
			logger.Printf("warning: event log not available from workspace")
		}
	}

	// Assemble the final telemetry (base + event log if available).
	if eventTelemetry.Log != nil {
		if mt, ok := baseTelemetry.(telemetry.MultiplexTelemetry); ok {
			mt.Sinks = append(mt.Sinks, eventTelemetry)
		}
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
	if eventTelemetry.Log != nil && registration.HITL != nil {
		ch, cancel := registration.HITL.Subscribe(32)
		rt.hitlCancel = cancel
		go func(ctx context.Context) {
			for ev := range ch {
				resolved := ev.Type == fauthorization.HITLEventResolved || ev.Type == fauthorization.HITLEventExpired
				eventTelemetry.EmitHITLEvent(ctx, resolved, ev)
			}
		}(ctx)
	}
	rt.Delegations.SetObserver(rt.observeDelegationSnapshot)
	if err := RegisterBuiltinProviders(ctx, rt); err != nil {
		_ = rt.Close(ctx)
		return nil, fmt.Errorf("register builtin providers: %w", err)
	}
	// Nexus gateway and node provider registration removed (app/nexus shelved)

	if ws != nil && ws.Telemetry != nil {
		ws.Telemetry.Emit(telemetry.Event{
			Type:      telemetry.EventStateChange,
			Timestamp: time.Now().UTC(),
			Message:   "backend_selected",
			Metadata:  map[string]any{"provider": cfg.InferenceProvider},
		})
	}

	agent, err := instantiateAgent(rt.paradigmDeps())
	if err != nil {
		_ = rt.Close(ctx)
		return nil, fmt.Errorf("instantiate agent: %w", err)
	}

	// Enforce the effective (post-definition) tool policies before initializing.
	if env.Config != nil && env.Config.AgentSpec != nil {
		env.Registry.UseAgentSpec(registration.ID, env.Config.AgentSpec)
	}

	rt.Agent = agent
	emitAgentStartupEvent(ctx, eventTelemetry.Log, eventTelemetry.Partition, registration.ID, cfg.AgentLabel(), agent)
	emitContractResolvedEvent(ctx, eventTelemetry.Log, eventTelemetry.Partition, registration.ID, cfg.AgentLabel(), docSnapshot)
	if err := ayenitd.RegisterWorkspaceServices(ctx, ayenitd.WorkspaceConfig{Workspace: cfg.Workspace}, sess, rt.Tools, registration); err != nil {
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

func newDegradedRuntime(ctx context.Context, cfg Config, secrets config.Secrets, reason error) *Runtime {
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

	// Emit boot.degraded observability event (NFR-4).
	log.Printf("boot.degraded{reason=%q sandbox_ready=false model_ready=false degraded=true}",
		reason.Error())
	_ = ctx
	_ = secrets
	return rt
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

	if r.hitlCancel != nil {
		r.hitlCancel()
		r.hitlCancel = nil
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
	agent, err := instantiateAgent(r.switchAgentDeps(agentCfg))
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

// instantiateAgent builds the euclo workflow executor.
func instantiateAgent(deps *paradigm.Deps) (agentgraph.WorkflowExecutor, error) {
	if deps == nil || deps.Registry == nil {
		return nil, fmt.Errorf("instantiate euclo: capability registry is required")
	}
	return euclo.New(deps, euclo.WithCheckpointRepository(deps.AgentLifecycle)), nil
}

func (r *Runtime) paradigmDeps() *paradigm.Deps {
	return &paradigm.Deps{
		Config:         r.Workspace.Environment.Config,
		Model:          r.Model,
		Registry:       r.Tools,
		CommandRunner:  r.Workspace.Environment.CommandRunner,
		CommandPolicy:  r.Workspace.Environment.CommandPolicy,
		WorkingMemory:  r.Memory,
		IndexManager:   r.IndexManager,
		SearchEngine:   r.SearchEngine,
		StreamTrigger:  r.Workspace.Environment.StreamTrigger,
		OutputIngester: r.Workspace.Environment.OutputIngester,
		IngestOutputs:  r.Workspace.Environment.IngestOutputs,
		PromptRegistry: r.Workspace.Environment.PromptRegistry,
		AgentLifecycle: r.AgentLifecycle,
		Telemetry:      r.Workspace.Telemetry,
	}
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
	env := contextdata.NewEnvelope(task.ID, "")
	env.NodeID = "runtime"
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
	return r.Agent.Execute(ctx, task, env)
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
		state = intentcontext.NewState(env.TaskID, env.SessionID)
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
		return nil, fmt.Errorf("agent unavailable for resume")
	}
	return r.Agent.Execute(ctx, task, env)
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

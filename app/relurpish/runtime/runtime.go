package runtime

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/app/envcomposition"
	"codeburg.org/lexbit/relurpify/ayenitd"
	"codeburg.org/lexbit/relurpify/capability/agentspec"
	registry "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/capability/sandbox"
	agents "codeburg.org/lexbit/relurpify/cognitionzoo"
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/knowledge/ast"
	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
	"codeburg.org/lexbit/relurpify/context/knowledge/memory"
	"codeburg.org/lexbit/relurpify/context/knowledge/search"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentgraph"
	"codeburg.org/lexbit/relurpify/execution/agentlifecycle"
	"codeburg.org/lexbit/relurpify/execution/session"
	fauthorization "codeburg.org/lexbit/relurpify/governance/authorization"
	"codeburg.org/lexbit/relurpify/governance/identity"
	"codeburg.org/lexbit/relurpify/governance/permissions"
	"codeburg.org/lexbit/relurpify/governance/policy"
	"codeburg.org/lexbit/relurpify/model"
	intentcontext "codeburg.org/lexbit/relurpify/named/euclo/intentcontext"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
	"codeburg.org/lexbit/relurpify/platform/llm"
	"codeburg.org/lexbit/relurpify/telemetry"
	"codeburg.org/lexbit/relurpify/telemetry/event"
	"codeburg.org/lexbit/relurpify/userconfig/config"
	"codeburg.org/lexbit/relurpify/userconfig/modelselect"
	"gopkg.in/yaml.v3"
)

// Runtime wires the relurpish CLI, Bubble Tea UI, and API server to the shared
// agent fruntime. It centralizes tool registration, manifests, sandbox
// registration, and log management.
type Runtime struct {
	Config          Config
	Workspace       *session.Workspace
	Session         *session.WorkspaceSession
	Tools           *registry.CapabilityRegistry
	Memory          *memory.WorkingMemoryStore
	Agent           agentgraph.WorkflowExecutor
	Model           model.LanguageModel
	IndexManager    *ast.IndexManager
	GraphDB         *graphdb.Engine
	SearchEngine    *search.SearchEngine
	AgentLifecycle  agentlifecycle.Repository
	Delegations     *fauthorization.DelegationManager
	WorkspaceConfig config.RuntimeWorkspaceConfig
	secrets         config.Secrets
	registration    *fauthorization.AgentRegistration
	modelBackend    llm.ManagedBackend

	hitlCancel func()

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

// New builds a fruntime for the TUI and status surfaces.
func New(ctx context.Context, cfg Config, secrets config.Secrets) (*Runtime, error) {
	envOverrides, err := config.LoadEnvOverrides(cfg.EnvOverrides)
	if err != nil {
		return nil, fmt.Errorf("load env overrides: %w", err)
	}
	if envOverrides.WorkspaceRoot != "" {
		cfg.Workspace = envOverrides.WorkspaceRoot
	}
	if err := cfg.Normalize(); err != nil {
		return nil, err
	}
	if envOverrides.ModelProvider != "" {
		cfg.InferenceProvider = envOverrides.ModelProvider
	}
	if envOverrides.ModelName != "" {
		cfg.InferenceModel = envOverrides.ModelName
	}
	if envOverrides.SandboxBackend != "" {
		cfg.SandboxBackend = envOverrides.SandboxBackend
	}
	if envOverrides.OllamaHost != "" {
		cfg.InferenceEndpoint = envOverrides.OllamaHost
	}
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

	// Load workspace YAML to get AllowedCapabilities and Nexus config before
	// calling ayenitd.Open — Open will handle model/agent-name overrides
	// internally, but AllowedCapabilities is a runtime-level concern.
	var workspaceCfg config.RuntimeWorkspaceConfig
	var allowedCapabilities []agentspec.CapabilitySelector
	if cfg.ConfigPath != "" {
		if loaded, err := config.LoadRuntimeWorkspaceConfig(cfg.ConfigPath); err == nil {
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
			allowedCapabilities = append(allowedCapabilities, convertRuntimeCapabilitySelectors(workspaceCfg.AllowedCapabilities)...)
		}
		// Missing config file is not an error — workspace may not be initialized yet.
	}

	// App-level environment composition starts here. agentenv consumes the
	// resulting products while the old environment object is being dissolved.
	docSnapshot, err := config.LoadDocument(cfg.ManifestPath)
	if err != nil {
		return nil, fmt.Errorf("load manifest: %w", err)
	}
	var spec config.ManifestSpec
	specNode := &yaml.Node{Kind: yaml.MappingNode}
	for k, v := range docSnapshot.Document.Spec {
		specNode.Content = append(specNode.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: k},
			&v,
		)
	}
	if err := specNode.Decode(&spec); err != nil {
		return nil, fmt.Errorf("decode manifest spec: %w", err)
	}
	manifestSnapshot := &config.ManifestSnapshot{
		Spec:        spec,
		Fingerprint: docSnapshot.Fingerprint,
		LoadedAt:    docSnapshot.LoadedAt,
		SourcePath:  docSnapshot.SourcePath,
		Warnings:    docSnapshot.Warnings,
	}
	securityBundle := loadedConfig.Security
	profileRegistry, err := modelselect.BuildProfileRegistry(loadedConfig.Model.Profiles)
	if err != nil {
		return nil, fmt.Errorf("load model profiles: %w", err)
	}
	profileResolution := profileRegistry.Resolve(cfg.InferenceProvider, cfg.InferenceModel)
	registration, err := fauthorization.RegisterAgent(ctx, fauthorization.RuntimeConfig{
		ManifestPath:     cfg.ManifestPath,
		ManifestSnapshot: manifestSnapshot,
		SecurityBundle:   &securityBundle,
		ConfigPath:       cfg.ConfigPath,
		Backend:          cfg.SandboxBackend,
		BackendFactory:   envcomposition.NewSandboxBackendFactory(),
		AuditLimit:       cfg.AuditLimit,
		BaseFS:           cfg.Workspace,
		StateDir:         config.DefaultWorkspaceStateDir(cfg.Workspace),
		HITLTimeout:      cfg.HITLTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("compose authorization registration: %w", err)
	}
	var securityRuntime *session.RuntimeSecurity
	var capabilityProduct *session.CapabilityProduct
	var knowledgeProduct *session.KnowledgeProduct
	var modelProduct *envcomposition.ModelRuntime
	if manifestSnapshot != nil {
		agentSpec := manifestSnapshot.Spec.Agent
		securityProduct, err := envcomposition.BuildSecurityRuntime(ctx, envcomposition.SecurityRuntimeInput{
			Context:           ctx,
			Workspace:         cfg.Workspace,
			SandboxBackend:    cfg.SandboxBackend,
			AgentID:           registration.ID,
			AgentSpec:         agentSpec,
			SecurityBundle:    &securityBundle,
			ManifestSpec:      &manifestSnapshot.Spec,
			PermissionManager: registration.Permissions,
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
		capProduct, err := envcomposition.BuildCapabilityRuntime(cfg.Workspace, securityProduct.Runner, envcomposition.CapabilityRuntimeOptions{
			Context:           ctx,
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
			Endpoint:          cfg.InferenceEndpoint,
			ModelName:         cfg.InferenceModel,
			NativeToolCalling: cfg.InferenceNativeToolCalling,
			Secrets:           llm.ProviderSecrets{APIKey: secrets.LLMAPIKey},
			Profile:           profileResolution.Profile,
		})
		if err != nil {
			return nil, fmt.Errorf("compose model runtime: %w", err)
		}
	}
	registrationView := &session.Registration{
		ID:               registration.ID,
		ManifestSpec:     registration.ManifestSpec,
		ManifestSnapshot: registration.ManifestSnapshot,
		Permissions:      registration.Permissions,
		Policy:           registration.Policy,
		Audit:            registration.Audit,
		HITL:             registration.HITL,
	}
	ws, err := session.OpenWorkspace(ctx, session.WorkspaceConfig{
		Workspace:                  cfg.Workspace,
		ManifestPath:               cfg.ManifestPath,
		InferenceProvider:          cfg.InferenceProvider,
		InferenceEndpoint:          cfg.InferenceEndpoint,
		InferenceModel:             cfg.InferenceModel,
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
		AllowedCapabilities:        allowedCapabilities,
		Strict:                     envOverrides.Strict,
		LoadedConfig:               loadedConfig,
		ManifestSnapshot:           manifestSnapshot,
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
		if registration.ManifestSpec != nil && registration.ManifestSpec.Agent != nil {
			spec := registration.ManifestSpec.Agent
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
				Actor:     identity.EventActor{Kind: "agent", ID: registration.ID, Label: cfg.AgentLabel()},
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
							Actor:     identity.EventActor{Kind: "agent", ID: registration.ID, Label: cfg.AgentLabel()},
							Partition: "local",
						}})
					}
				})
			}
			if registration.ManifestSnapshot != nil {
				emitManifestReloadedEvent(ctx, env.EventLog, registration.ID, cfg.AgentLabel(), registration.ManifestSnapshot)
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

	// Register relurpic capabilities (subagent-backed; cannot be done in ayenitd).

	// Use WorkflowStore interface directly
	rt := &Runtime{
		Config:               cfg,
		Workspace:            ws,
		Session:              sess,
		Tools:                env.Registry,
		Memory:               env.WorkingMemory,
		Model:                env.Model,
		IndexManager:         env.IndexManager,
		GraphDB:              graphDBFromIndexManager(env.IndexManager),
		SearchEngine:         env.SearchEngine,
		AgentLifecycle:       env.AgentLifecycle,
		WorkspaceConfig:      workspaceCfg,
		Delegations:          fauthorization.NewDelegationManager(),
		interactionEnvelopes: make(map[string]*contextdata.Envelope),
		secrets:              secrets,
		registration:         registration,
		modelBackend:         modelProduct.Backend,
	}
	if eventTelemetry.Log != nil && registration.HITL != nil {
		ch, cancel := registration.HITL.Subscribe(32)
		rt.hitlCancel = cancel
		go func() {
			for ev := range ch {
				resolved := ev.Type == fauthorization.HITLEventResolved || ev.Type == fauthorization.HITLEventExpired
				eventTelemetry.EmitHITLEvent(resolved, ev)
			}
		}()
	}
	rt.Delegations.SetObserver(rt.observeDelegationSnapshot)
	if err := RegisterBuiltinProviders(ctx, rt); err != nil {
		_ = rt.Close()
		return nil, fmt.Errorf("register builtin providers: %w", err)
	}
	// Nexus gateway and node provider registration removed (app/nexus shelved)

	agent := instantiateAgent(cfg, rt.paradigmDeps())
	rt.wireRuntimeAgentDependencies(agent)

	// Enforce the effective (post-definition) tool policies before initializing.
	if env.Config != nil && env.Config.AgentSpec != nil {
		env.Registry.UseAgentSpec(registration.ID, env.Config.AgentSpec)
	}

	rt.Agent = agent
	if err := ayenitd.RegisterWorkspaceServices(ctx, ayenitd.WorkspaceConfig{Workspace: cfg.Workspace}, sess, rt.Tools, registration); err != nil {
		_ = rt.Close()
		return nil, fmt.Errorf("register workspace services: %w", err)
	}
	if err := ayenitd.StartWorkspaceServices(ctx, sess); err != nil {
		_ = rt.Close()
		return nil, fmt.Errorf("start workspace services: %w", err)
	}
	return rt, nil
}

// Close releases resources managed by fruntime.
func (r *Runtime) Close() error {
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

	// Close workspace (handles backend, services, logs, etc.)
	if r.Workspace != nil {
		if err := r.Workspace.Close(); err != nil {
			errs = append(errs, err)
		}
		r.Workspace = nil
	}
	r.interactionMu.Lock()
	r.interactionEnvelopes = make(map[string]*contextdata.Envelope)
	r.interactionMu.Unlock()

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
	if r.Workspace.Registration == nil || r.Workspace.Registration.ManifestSpec == nil || r.Workspace.Registration.ManifestSpec.Agent == nil {
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
	agentCfg := &execution.Config{
		Name:              cfg.AgentLabel(),
		Model:             cfg.InferenceModel,
		MaxIterations:     8,
		NativeToolCalling: effectiveContract.AgentSpec.NativeToolCallingEnabled(),
		AgentSpec:         effectiveContract.AgentSpec,
		Telemetry:         r.Workspace.Telemetry,
	}
	agent := instantiateAgent(cfg, &paradigm.Deps{
		Model:         r.Model,
		Registry:      r.Tools,
		IndexManager:  r.IndexManager,
		SearchEngine:  r.SearchEngine,
		WorkingMemory: r.Memory,
		Config:        agentCfg,
	})
	if agent == nil {
		return fmt.Errorf("agent %s not available", name)
	}
	r.wireRuntimeAgentDependencies(agent)
	r.Tools.UseAgentSpec(r.Workspace.Registration.ID, effectiveContract.AgentSpec)
	r.Workspace.Registration.Policy = nil
	r.Agent = agent
	r.Workspace.AgentSpec = effectiveContract.AgentSpec
	r.Workspace.EffectiveContract = effectiveContract
	r.Workspace.CompiledPolicy = compiledPolicy
	r.Workspace.CapabilityAdmissions = nil
	r.Config.AgentName = name
	return nil
}

// instantiateAgent picks the concrete agent implementation for the CLI preset.
func instantiateAgent(cfg Config, deps *paradigm.Deps) agentgraph.WorkflowExecutor {
	paths := config.New(cfg.Workspace)
	builder := agents.NewAgentBuilder().WithDeps(deps)
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

func (r *Runtime) paradigmDeps() *paradigm.Deps {
	return &paradigm.Deps{
		Config:         r.Workspace.Environment.Config,
		Model:          r.Model,
		Registry:       r.Tools,
		WorkingMemory:  r.Memory,
		IndexManager:   r.IndexManager,
		SearchEngine:   r.SearchEngine,
		StreamTrigger:  r.Workspace.Environment.StreamTrigger,
		OutputIngester: r.Workspace.Environment.OutputIngester,
		IngestOutputs:  r.Workspace.Environment.IngestOutputs,
		PromptRegistry: r.Workspace.Environment.PromptRegistry,
		AgentLifecycle: r.AgentLifecycle,
	}
}

func (r *Runtime) resolveEffectiveContractForAgent(name string) (*config.EffectiveAgentContract, *session.CompiledPolicy, error) {
	effectiveContract, err := config.ResolveEffectiveAgentContract(r.Config.Workspace, r.Workspace.Registration.ManifestSpec, config.ResolveOptions{})
	if err != nil {
		return nil, nil, fmt.Errorf("resolve effective contract: %w", err)
	}
	if effectiveContract.AgentID == "" {
		return nil, nil, fmt.Errorf("agent id required")
	}
	if effectiveContract.AgentSpec == nil {
		return nil, nil, fmt.Errorf("agent spec required")
	}
	compiledPolicy := &session.CompiledPolicy{
		AgentID: effectiveContract.AgentID,
		Spec:    effectiveContract.AgentSpec,
		Engine:  r.Workspace.PolicyEngine,
	}
	return effectiveContract, compiledPolicy, nil
}

func configureBuiltAgent(agent agentgraph.WorkflowExecutor, paths config.Paths) agentgraph.WorkflowExecutor {
	_ = paths
	return agent
}

func (r *Runtime) wireRuntimeAgentDependencies(agent agentgraph.WorkflowExecutor) {
	_ = r
	_ = agent
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
	return r.Agent.Execute(ctx, task, env)
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

// ExecuteInstruction convenience helper.
func (r *Runtime) ExecuteInstruction(ctx context.Context, instruction string, taskType execution.TaskType, metadata map[string]any) (*execution.Result, error) {
	if taskType == "" {
		taskType = execution.TaskTypeExecute
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

// ExecuteInstructionStream is like ExecuteInstruction but wires a streaming
// callback so the LLM emits tokens incrementally via callback as they arrive.
func (r *Runtime) ExecuteInstructionStream(ctx context.Context, instruction string, taskType execution.TaskType, metadata map[string]any, callback func(string)) (*execution.Result, error) {
	if metadata == nil {
		metadata = make(map[string]any)
	}
	metadata["stream_callback"] = callback
	return r.ExecuteInstruction(ctx, instruction, taskType, metadata)
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
	value, ok := env.GetWorkingValue("task.input")
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
		value, ok := env.GetWorkingValue(key)
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

func emitManifestReloadedEvent(ctx context.Context, eventLog event.Log, agentID, label string, snapshot *config.ManifestSnapshot) {
	if eventLog == nil || snapshot == nil {
		return
	}
	payload := map[string]any{
		"manifest_path": snapshot.SourcePath,
		"fingerprint":   hex.EncodeToString(snapshot.Fingerprint[:]),
		"warnings":      append([]string(nil), snapshot.Warnings...),
	}
	if data, err := json.Marshal(payload); err == nil {
		_, _ = eventLog.Append(ctx, "local", []event.FrameworkEvent{{
			Timestamp: time.Now().UTC(),
			Type:      event.EventManifestReloaded,
			Payload:   data,
			Actor:     identity.EventActor{Kind: "agent", ID: agentID, Label: label},
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

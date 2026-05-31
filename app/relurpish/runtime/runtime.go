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

	"codeburg.org/lexbit/relurpify/agents"
	nexusdb "codeburg.org/lexbit/relurpify/app/nexus/db"
	"codeburg.org/lexbit/relurpify/ayenitd"
	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/agentlifecycle"
	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/ast"
	fauthorization "codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/cfgload"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/event"
	"codeburg.org/lexbit/relurpify/framework/graphdb"
	"codeburg.org/lexbit/relurpify/framework/llmconfig"
	"codeburg.org/lexbit/relurpify/framework/memory"
	"codeburg.org/lexbit/relurpify/framework/search"
	"codeburg.org/lexbit/relurpify/framework/telemetry"
	"codeburg.org/lexbit/relurpify/named/euclo"
	intentcontext "codeburg.org/lexbit/relurpify/named/euclo/intentcontext"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
	"codeburg.org/lexbit/relurpify/platform/contracts"
	"codeburg.org/lexbit/relurpify/platform/llm"
	"codeburg.org/lexbit/relurpify/relurpnet/identity"
	"codeburg.org/lexbit/relurpify/relurpnet/mcp/protocol"
)

// Runtime wires the relurpish CLI, Bubble Tea UI, and API server to the shared
// agent fruntime. It centralizes tool registration, manifests, sandbox
// registration, and log management.
type Runtime struct {
	Config            Config
	Workspace         *agentenv.Workspace
	Tools             *capability.Registry
	Memory            *memory.WorkingMemoryStore
	Agent             agentgraph.WorkflowExecutor
	Model             contracts.LanguageModel
	IndexManager      *ast.IndexManager
	GraphDB           *graphdb.Engine
	SearchEngine      *search.SearchEngine
	AgentLifecycle    agentlifecycle.Repository
	Delegations       *fauthorization.DelegationManager
	WorkspaceConfig   cfgload.RuntimeWorkspaceConfig
	NexusNodeProvider core.NodeProvider
	NexusClient       *NexusClient
	secrets           cfgload.Secrets

	hitlCancel  func()
	nexusCancel func()

	providersMu          sync.Mutex
	providers            []runtimeProviderRecord
	interactionMu        sync.Mutex
	interactionEnvelopes map[string]*contextdata.Envelope
	delegationMu         sync.Mutex
	delegationBG         *backgroundDelegationProvider
	mcpMu                sync.Mutex
	mcpElicit            MCPElicitationHandler
}

// AgentWorkspace returns the framework/agentenv Workspace for this Runtime.
func (r *Runtime) AgentWorkspace() *agentenv.Workspace {
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
func (r *Runtime) Secrets() cfgload.Secrets {
	if r == nil {
		return cfgload.Secrets{}
	}
	return r.secrets
}

type MCPElicitationHandler interface {
	HandleMCPElicitation(ctx context.Context, params protocol.ElicitationParams) (*protocol.ElicitationResult, error)
}

// New builds a fruntime for the TUI and status surfaces.
func New(ctx context.Context, cfg Config, secrets cfgload.Secrets) (*Runtime, error) {
	envOverrides, err := cfgload.LoadEnvOverrides(cfg.EnvOverrides)
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
		cfg.SharedRoot = cfgload.ResolveSharedRoot(envOverrides.XDGDataHome)
	}
	if report := cfgload.ValidateWorkspaceTree(cfg.Workspace); report.HasErrors() {
		return nil, report
	}

	loadedConfig, _, err := cfgload.Load(cfgload.LoadOptions{
		WorkspaceRoot: cfg.Workspace,
		EnvOverrides:  cfg.EnvOverrides,
	})
	if err != nil {
		return nil, fmt.Errorf("load workspace config bundle: %w", err)
	}

	// Load workspace YAML to get AllowedCapabilities and Nexus config before
	// calling ayenitd.Open — Open will handle model/agent-name overrides
	// internally, but AllowedCapabilities is a runtime-level concern.
	var workspaceCfg cfgload.RuntimeWorkspaceConfig
	var allowedCapabilities []agentspec.CapabilitySelector
	if cfg.ConfigPath != "" {
		if loaded, err := cfgload.LoadRuntimeWorkspaceConfig(cfg.ConfigPath); err == nil {
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

	// Delegate all workspace initialization to framework/agentenv.OpenWorkspace().
	// app/relurpish does not build its own workspace environment.
	manifestSnapshot, err := cfgload.LoadAgentManifestSnapshot(cfg.ManifestPath)
	if err != nil {
		return nil, fmt.Errorf("load manifest snapshot: %w", err)
	}
	securityBundle := loadedConfig.Security
	profileRegistry, err := llmconfig.ProfileRegistryFromConfigs(loadedConfig.Model.Profiles)
	if err != nil {
		return nil, fmt.Errorf("load model profiles: %w", err)
	}
	profileResolution := profileRegistry.Resolve(cfg.InferenceProvider, cfg.InferenceModel)
	agentDefs, err := cfgload.LoadAgentDefinitions(cfg.Workspace, cfg.AgentsDir)
	if err != nil {
		return nil, fmt.Errorf("load agent definitions: %w", err)
	}
	ws, err := agentenv.OpenWorkspace(ctx, agentenv.WorkspaceConfig{
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
		AgentDefinitions:           agentDefs,
		SecurityBundle:             &securityBundle,
		Scope:                      agentenv.ScopeFull,
		EventLogFactory: func(path string) (event.Log, error) {
			return nexusdb.NewSQLiteEventLog(path)
		},
	}, llm.ProviderSecrets{APIKey: secrets.LLMAPIKey}, euclo.GetRegistrationFuncs())
	if err != nil {
		return nil, err
	}

	env := ws.Environment
	registration := ws.Registration
	logger := ws.Logger
	baseTelemetry := ws.Telemetry
	if registration != nil && registration.Permissions != nil {
		var agentSpec *agentspec.AgentRuntimeSpec
		if registration.Manifest != nil {
			agentSpec = registration.Manifest.Spec.Agent
		}
		cfg.CommandPolicy = fauthorization.NewCommandAuthorizationPolicy(registration.Permissions, registration.ID, agentSpec, "runtime")
	}

	// Extend telemetry with an event log sink. The event log is now created
	// by framework/agentenv via EventLogFactory, so we just need to wire it into
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
						_, _ = env.EventLog.Append(ctx, "local", []core.FrameworkEvent{{
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
		} else {
			baseTelemetry = telemetry.MultiplexTelemetry{Sinks: []core.Telemetry{baseTelemetry, eventTelemetry}}
		}
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
		Workspace:            ws,
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
	if err := ayenitd.RegisterWorkspaceServices(ctx, ayenitd.WorkspaceConfig{Workspace: cfg.Workspace}, rt.Workspace); err != nil {
		_ = rt.Close()
		return nil, fmt.Errorf("register workspace services: %w", err)
	}
	if err := ayenitd.StartWorkspaceServices(ctx, rt.Workspace); err != nil {
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
	if r.nexusCancel != nil {
		r.nexusCancel()
		r.nexusCancel = nil
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
	if r != nil {
		for name := range r.Workspace.AgentDefinitions {
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
	if r.Workspace.Registration == nil || r.Workspace.Registration.Manifest == nil || r.Workspace.Registration.Manifest.Spec.Agent == nil {
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
	if name == "" && r.Workspace.Registration != nil {
		name = strings.TrimSpace(r.Workspace.Registration.ID)
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

func (r *Runtime) applyResolvedAgentState(name string, effectiveContract *cfgload.EffectiveAgentContract, compiledPolicy *fauthorization.CompiledPolicyBundle, agentDefs map[string]*agentspec.AgentDefinition) error {
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
	if err := ensureStableSkillCapabilityTopology(r.Workspace.EffectiveContract, effectiveContract); err != nil {
		return err
	}
	agentCfg := &core.Config{
		Name:              cfg.AgentLabel(),
		Model:             cfg.InferenceModel,
		MaxIterations:     8,
		NativeToolCalling: effectiveContract.AgentSpec.NativeToolCallingEnabled(),
		AgentSpec:         effectiveContract.AgentSpec,
		Telemetry:         r.Workspace.Telemetry,
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
	r.Tools.UseAgentSpec(r.Workspace.Registration.ID, effectiveContract.AgentSpec)
	r.Workspace.Registration.Policy = nil
	r.Agent = agent
	r.Workspace.AgentSpec = effectiveContract.AgentSpec
	r.Workspace.AgentDefinitions = agentDefs
	r.Workspace.EffectiveContract = effectiveContract
	r.Workspace.CompiledPolicy = compiledPolicy
	r.Workspace.CapabilityAdmissions = nil
	r.syncSkillContextPaths(effectiveContract.SkillResults)
	r.Config.AgentName = name
	return nil
}

// instantiateAgent picks the concrete agent implementation for the CLI preset.
func instantiateAgent(cfg Config, env agents.AgentEnvironment, defs map[string]*agentspec.AgentDefinition) agentgraph.WorkflowExecutor {
	paths := cfgload.New(cfg.Workspace)
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
	paths := cfgload.New(cfg.Workspace)
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

func (r *Runtime) resolveEffectiveContractForAgent(name string) (*cfgload.EffectiveAgentContract, *fauthorization.CompiledPolicyBundle, map[string]*agentspec.AgentDefinition, error) {
	agentDefs := r.Workspace.AgentDefinitions
	effectiveContract, err := cfgload.ResolveEffectiveAgentContract(r.Config.Workspace, r.Workspace.Registration.Manifest, cfgload.ResolveOptions{
		AgentOverlays: agentspec.AgentSpecOverlaysForName(name, agentDefs),
	}, nil)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("resolve effective contract: %w", err)
	}
	compiledPolicy, err := fauthorization.BuildFromSpec(effectiveContract.AgentID, effectiveContract.AgentSpec, nil, nil)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("compile effective policy: %w", err)
	}
	return effectiveContract, compiledPolicy, agentDefs, nil
}

func ensureStableSkillCapabilityTopology(current, next *cfgload.EffectiveAgentContract) error {
	return nil
}

func skillCapabilityIDSet(contract *cfgload.EffectiveAgentContract) map[string]struct{} {
	_ = contract
	return nil
}

func (r *Runtime) syncSkillContextPaths(results []cfgload.SkillResolution) {
	_ = r
	_ = results
}

func configureBuiltAgent(agent agentgraph.WorkflowExecutor, paths cfgload.Paths) agentgraph.WorkflowExecutor {
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
		env.SetWorkingValue("euclo.interaction.frame_requested", false, contextdata.MemoryClassTask)
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
	env.SetWorkingValue("euclo.interaction.frame_requested", false, contextdata.MemoryClassTask)
	return nil
}

func (r *Runtime) resumeInteractionTask(ctx context.Context, env *contextdata.Envelope) (*core.Result, error) {
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
	task, ok := value.(*core.Task)
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
	if r.Workspace.Registration == nil || r.Workspace.Registration.HITL == nil {
		return nil
	}
	return r.Workspace.Registration.HITL.PendingRequests()
}

func emitManifestReloadedEvent(ctx context.Context, eventLog event.Log, agentID, label string, snapshot *cfgload.AgentManifestSnapshot) {
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
	if r == nil || r.Workspace.Registration == nil || r.Workspace.Registration.HITL == nil {
		ch := make(chan fauthorization.HITLEvent)
		close(ch)
		return ch, func() {}
	}
	return r.Workspace.Registration.HITL.Subscribe(32)
}

// ApproveHITL approves a pending request with the supplied scope.
func (r *Runtime) ApproveHITL(requestID, approver string, scope fauthorization.GrantScope, duration time.Duration) error {
	if r.Workspace.Registration == nil || r.Workspace.Registration.HITL == nil {
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
	return r.Workspace.Registration.HITL.Approve(decision)
}

// DenyHITL rejects a pending request.
func (r *Runtime) DenyHITL(requestID, reason string) error {
	if r.Workspace.Registration == nil || r.Workspace.Registration.HITL == nil {
		return errors.New("hitl broker unavailable")
	}
	return r.Workspace.Registration.HITL.Deny(requestID, reason)
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

package runtime

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/agents"
	nexusdb "codeburg.org/lexbit/relurpify/app/nexus/db"
	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/agentlifecycle"
	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/ast"
	fauthorization "codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/event"
	"codeburg.org/lexbit/relurpify/framework/graphdb"
	"codeburg.org/lexbit/relurpify/framework/manifest"
	"codeburg.org/lexbit/relurpify/framework/memory"
	"codeburg.org/lexbit/relurpify/framework/search"
	frameworkskills "codeburg.org/lexbit/relurpify/framework/skills"
	"codeburg.org/lexbit/relurpify/framework/telemetry"
	"codeburg.org/lexbit/relurpify/named/euclo"
	"codeburg.org/lexbit/relurpify/platform/contracts"
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
	WorkspaceConfig   WorkspaceConfig
	NexusNodeProvider core.NodeProvider
	NexusClient       *NexusClient

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

// AgentWorkspace returns the framework/agentenv Workspace for this Runtime.
func (r *Runtime) AgentWorkspace() *agentenv.Workspace {
	return r.Workspace
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

	// Delegate all workspace initialization to framework/agentenv.Open().
	ws, err := agentenv.Open(ctx, agentenv.WorkspaceConfig{
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
		AllowedCapabilities:        allowedCapabilities,
		EventLogFactory: func(path string) (event.Log, error) {
			return nexusdb.NewSQLiteEventLog(path)
		},
	}, euclo.GetRegistrationFuncs())
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
		Config:          cfg,
		Workspace:       ws,
		Tools:           env.Registry,
		Memory:          env.WorkingMemory,
		Model:           env.Model,
		IndexManager:    env.IndexManager,
		GraphDB:         graphDBFromIndexManager(env.IndexManager),
		SearchEngine:    env.SearchEngine,
		AgentLifecycle:  env.AgentLifecycle,
		WorkspaceConfig: workspaceCfg,
		Delegations:     fauthorization.NewDelegationManager(),
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
	agentDefs := r.Workspace.AgentDefinitions
	if r.Config.AgentsDir != "" {
		loaded, err := LoadAgentDefinitions(r.Config.AgentsDir)
		if err != nil && !os.IsNotExist(err) {
			return nil, nil, nil, fmt.Errorf("load agent definitions: %w", err)
		}
		if loaded != nil {
			agentDefs = loaded
		}
	}
	effectiveContract, err := manifest.ResolveEffectiveAgentContract(r.Config.Workspace, r.Workspace.Registration.Manifest, manifest.ResolveOptions{
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
	if r.Workspace.Registration == nil || r.Workspace.Registration.HITL == nil {
		return nil
	}
	return r.Workspace.Registration.HITL.PendingRequests()
}

func emitManifestReloadedEvent(ctx context.Context, eventLog event.Log, agentID, label string, snapshot *manifest.AgentManifestSnapshot) {
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

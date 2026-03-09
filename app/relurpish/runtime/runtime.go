package runtime

import (
	"context"
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

	"github.com/lexcodex/relurpify/agents"
	"github.com/lexcodex/relurpify/framework/ast"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/lexcodex/relurpify/framework/mcp/protocol"
	"github.com/lexcodex/relurpify/framework/memory"
	fruntime "github.com/lexcodex/relurpify/framework/runtime"
	"github.com/lexcodex/relurpify/framework/telemetry"
	"github.com/lexcodex/relurpify/framework/workspacecfg"
	"github.com/lexcodex/relurpify/llm"
	"github.com/lexcodex/relurpify/server"
	"github.com/lexcodex/relurpify/tools"
)

// Runtime wires the relurpish CLI, Bubble Tea UI, and API server to the shared
// agent fruntime. It centralizes tool registration, manifests, sandbox
// registration, and log management.
type Runtime struct {
	Config           Config
	Tools            *capability.Registry
	Memory           memory.MemoryStore
	Context          *core.Context
	Agent            graph.Agent
	Model            core.LanguageModel
	IndexManager     *ast.IndexManager
	Registration     *fruntime.AgentRegistration
	Delegations      *fruntime.DelegationManager
	AgentSpec        *core.AgentRuntimeSpec
	AgentDefinitions map[string]*core.AgentDefinition
	Telemetry        core.Telemetry
	Logger           *log.Logger
	Workspace        WorkspaceConfig

	logFile io.Closer

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
	if err := os.MkdirAll(filepath.Dir(cfg.LogPath), 0o755); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}
	logFile, err := os.OpenFile(cfg.LogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log: %w", err)
	}
	logger := log.New(logFile, "relurpish ", log.LstdFlags|log.Lmicroseconds)

	memory, err := memory.NewHybridMemory(cfg.MemoryPath)
	if err != nil {
		logFile.Close()
		return nil, fmt.Errorf("memory init: %w", err)
	}

	var workspaceCfg WorkspaceConfig
	var allowedCapabilities []core.CapabilitySelector
	if cfg.ConfigPath != "" {
		if loaded, err := LoadWorkspaceConfig(cfg.ConfigPath); err == nil {
			workspaceCfg = loaded
			if workspaceCfg.Model != "" {
				cfg.OllamaModel = workspaceCfg.Model
			}
			if len(workspaceCfg.Agents) > 0 {
				cfg.AgentName = workspaceCfg.Agents[0]
			}
			allowedCapabilities = append(allowedCapabilities, workspaceCfg.AllowedCapabilities...)
		} else if !errors.Is(err, os.ErrNotExist) {
			logger.Printf("workspace config load failed: %v", err)
		}
	}

	registration, err := fruntime.RegisterAgent(ctx, fruntime.RuntimeConfig{
		ManifestPath: cfg.ManifestPath,
		Sandbox:      cfg.Sandbox,
		AuditLimit:   cfg.AuditLimit,
		BaseFS:       cfg.Workspace,
		HITLTimeout:  cfg.HITLTimeout,
	})
	if err != nil {
		logFile.Close()
		return nil, fmt.Errorf("sandbox registration failed: %w", err)
	}
	if registration.Manifest == nil || registration.Manifest.Spec.Agent == nil {
		logFile.Close()
		return nil, fmt.Errorf("agent manifest missing spec.agent configuration")
	}
	agentSpec := agents.ApplyManifestDefaults(registration.Manifest.Spec.Agent, registration.Manifest.Spec.Defaults)
	if agentSpec.Model.Name == "" {
		logFile.Close()
		return nil, fmt.Errorf("agent manifest missing spec.agent.model.name")
	}
	runner, err := fruntime.NewSandboxCommandRunner(registration.Manifest, registration.Runtime, cfg.Workspace)
	if err != nil {
		logFile.Close()
		return nil, err
	}
	registry, indexManager, err := BuildCapabilityRegistry(cfg.Workspace, runner, CapabilityRegistryOptions{
		AgentID:           registration.ID,
		PermissionManager: registration.Permissions,
		AgentSpec:         agentSpec,
	})
	if err != nil {
		logFile.Close()
		return nil, err
	}
	agentSpec, skillResults := agents.ApplySkills(cfg.Workspace, agentSpec, registration.Manifest.Spec.Skills, registry, registration.Permissions, registration.ID)
	if cfg.AgentName == "" {
		cfg.AgentName = registration.Manifest.Metadata.Name
	}

	if cfg.OllamaModel == "" {
		cfg.OllamaModel = agentSpec.Model.Name
	}
	if cfg.OllamaModel == "" {
		logFile.Close()
		return nil, fmt.Errorf("ollama model not configured; update %s", cfg.ManifestPath)
	}

	// Load all agent definitions from the agents directory
	agentDefs, err := LoadAgentDefinitions(cfg.AgentsDir)
	if err != nil && !os.IsNotExist(err) {
		// Log warning but proceed with builtin agents
		logger.Printf("warning: failed to load agent definitions: %v", err)
	}

	// Setup Telemetry
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
	telemetry := telemetry.MultiplexTelemetry{Sinks: sinks}
	registry.UseTelemetry(telemetry)

	logLLM := false
	if agentSpec.Logging != nil && agentSpec.Logging.LLM != nil {
		logLLM = *agentSpec.Logging.LLM
	}
	modelClient := llm.NewClient(cfg.OllamaEndpoint, cfg.OllamaModel)
	modelClient.SetDebugLogging(logLLM)
	model := llm.NewInstrumentedModel(modelClient, telemetry, logLLM)

	// Create base config derived from manifest + CLI args
	agentCfg := &core.Config{
		Name:              cfg.AgentLabel(),
		Model:             cfg.OllamaModel,
		OllamaEndpoint:    cfg.OllamaEndpoint,
		MaxIterations:     8,
		OllamaToolCalling: agentSpec.ToolCallingEnabled(),
		AgentSpec:         agentSpec, // Default to manifest spec
		Telemetry:         telemetry,
	}
	registry.UseAgentSpec(registration.ID, agentSpec)
	if err := agents.RegisterBuiltinRelurpicCapabilities(registry, model, agentCfg); err != nil {
		logFile.Close()
		return nil, fmt.Errorf("register relurpic capabilities: %w", err)
	}

	rt := &Runtime{
		Config:           cfg,
		Tools:            registry,
		Memory:           memory,
		Context:          core.NewContext(),
		Model:            model,
		IndexManager:     indexManager,
		Logger:           logger,
		logFile:          logFile,
		Workspace:        workspaceCfg,
		Registration:     registration,
		Delegations:      fruntime.NewDelegationManager(),
		AgentSpec:        agentSpec,
		AgentDefinitions: agentDefs,
		Telemetry:        telemetry,
	}
	rt.Delegations.SetObserver(rt.observeDelegationSnapshot)
	for _, skill := range skillResults {
		if !skill.Applied || skill.Paths.Root == "" {
			continue
		}
		rt.Context.Set(fmt.Sprintf("skill.%s.path", skill.Name), skill.Paths.Root)
	}
	if err := RegisterBuiltinProviders(ctx, rt); err != nil {
		logFile.Close()
		return nil, fmt.Errorf("register builtin providers: %w", err)
	}

	agent := instantiateAgent(cfg, model, registry, memory, agentDefs, agentCfg, indexManager)

	// Enforce the effective (post-definition) tool policies before initializing.
	if agentCfg.AgentSpec != nil {
		registry.UseAgentSpec(registration.ID, agentCfg.AgentSpec)
	}

	if err := agent.Initialize(agentCfg); err != nil {
		_ = rt.Close()
		return nil, fmt.Errorf("initialize agent: %w", err)
	}
	if reflection, ok := agent.(*agents.ReflectionAgent); ok {
		if reflection.Delegate != nil {
			_ = reflection.Delegate.Initialize(agentCfg)
		}
	}
	if len(allowedCapabilities) > 0 {
		registry.RestrictToCapabilities(allowedCapabilities)
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
	cfg := r.Config
	cfg.AgentName = name
	baseSpec := agents.ApplyManifestDefaults(r.Registration.Manifest.Spec.Agent, r.Registration.Manifest.Spec.Defaults)
	agentCfg := &core.Config{
		Name:              cfg.AgentLabel(),
		Model:             cfg.OllamaModel,
		OllamaEndpoint:    cfg.OllamaEndpoint,
		MaxIterations:     8,
		OllamaToolCalling: baseSpec.ToolCallingEnabled(),
		AgentSpec:         baseSpec,
		Telemetry:         r.Telemetry,
	}
	agent := instantiateAgent(cfg, r.Model, r.Tools, r.Memory, r.AgentDefinitions, agentCfg, r.IndexManager)
	if agent == nil {
		return fmt.Errorf("agent %s not available", name)
	}
	if agentCfg.Model != cfg.OllamaModel {
		return fmt.Errorf("agent %s requires model %s; restart to switch models", name, agentCfg.Model)
	}
	if agentCfg.AgentSpec != nil {
		r.Tools.UseAgentSpec(r.Registration.ID, agentCfg.AgentSpec)
	}
	if err := agent.Initialize(agentCfg); err != nil {
		return err
	}
	if reflection, ok := agent.(*agents.ReflectionAgent); ok {
		if reflection.Delegate != nil {
			_ = reflection.Delegate.Initialize(agentCfg)
		}
	}
	r.Agent = agent
	r.Config.AgentName = name
	return nil
}

// CapabilityRegistryOptions carries optional manifest/runtime policies into capability construction.
type CapabilityRegistryOptions struct {
	AgentID           string
	PermissionManager *fruntime.PermissionManager
	AgentSpec         *core.AgentRuntimeSpec
}

// BuildCapabilityRegistry registers builtin tool capabilities scoped to the workspace.
func BuildCapabilityRegistry(workspace string, runner fruntime.CommandRunner, opts ...CapabilityRegistryOptions) (*capability.Registry, *ast.IndexManager, error) {
	if workspace == "" {
		workspace = "."
	}
	if runner == nil {
		return nil, nil, fmt.Errorf("command runner required")
	}
	var cfg CapabilityRegistryOptions
	if len(opts) > 0 {
		cfg = opts[0]
	}
	registry := capability.NewRegistry()
	if cfg.PermissionManager != nil {
		registry.UsePermissionManager(cfg.AgentID, cfg.PermissionManager)
	}
	if cfg.AgentSpec != nil {
		registry.UseAgentSpec(cfg.AgentID, cfg.AgentSpec)
	}
	register := func(tool core.Tool) error {
		if err := registry.Register(tool); err != nil {
			return err
		}
		return nil
	}
	for _, tool := range tools.FileOperations(workspace) {
		if err := register(tool); err != nil {
			return nil, nil, err
		}
	}
	for _, tool := range []core.Tool{
		&tools.SimilarityTool{BasePath: workspace},
		&tools.SemanticSearchTool{BasePath: workspace},
	} {
		if err := register(tool); err != nil {
			return nil, nil, err
		}
	}
	for _, tool := range []core.Tool{
		&tools.GitCommandTool{RepoPath: workspace, Command: "diff", Runner: runner},
		&tools.GitCommandTool{RepoPath: workspace, Command: "history", Runner: runner},
		&tools.GitCommandTool{RepoPath: workspace, Command: "branch", Runner: runner},
		&tools.GitCommandTool{RepoPath: workspace, Command: "commit", Runner: runner},
		&tools.GitCommandTool{RepoPath: workspace, Command: "blame", Runner: runner},
	} {
		if err := register(tool); err != nil {
			return nil, nil, err
		}
	}
	for _, tool := range tools.CommandLineTools(workspace, runner) {
		if err := register(tool); err != nil {
			return nil, nil, err
		}
	}
	paths := workspacecfg.New(workspace)
	indexDir := paths.ASTIndexDir()
	if err := os.MkdirAll(indexDir, 0o755); err != nil {
		return nil, nil, err
	}
	store, err := ast.NewSQLiteStore(paths.ASTIndexDB())
	if err != nil {
		return nil, nil, err
	}
	manager := ast.NewIndexManager(store, ast.IndexConfig{
		WorkspacePath:   workspace,
		ParallelWorkers: 4,
	})
	if cfg.PermissionManager != nil {
		manager.SetPathFilter(func(path string, isDir bool) bool {
			action := core.FileSystemRead
			if isDir {
				action = core.FileSystemList
			}
			return cfg.PermissionManager.CheckFileAccess(context.Background(), cfg.AgentID, action, path) == nil
		})
	}
	tools.AttachASTSymbolProvider(manager, registry)
	if err := register(tools.NewASTTool(manager)); err != nil {
		return nil, nil, err
	}
	go manager.IndexWorkspace()
	return registry, manager, nil
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
func instantiateAgent(cfg Config, model core.LanguageModel, registry *capability.Registry, mem memory.MemoryStore, defs map[string]*core.AgentDefinition, agentCfg *core.Config, indexManager *ast.IndexManager) graph.Agent {
	paths := workspacecfg.New(cfg.Workspace)
	workflowStatePath := paths.WorkflowStateFile()
	// Check file-based definitions first
	if def, ok := defs[cfg.AgentName]; ok {
		// Update config with the definition's spec
		agentCfg.AgentSpec = &def.Spec
		agentCfg.OllamaToolCalling = def.Spec.ToolCallingEnabled()
		if def.Spec.Model.Name != "" {
			agentCfg.Model = def.Spec.Model.Name
		}

		return instantiateDefinitionAgent(cfg, def, model, registry, mem, indexManager)
	}

	switch cfg.AgentLabel() {
	case "planner":
		return &agents.PlannerAgent{Model: model, Tools: registry, Memory: mem}
	case "react":
		return &agents.ReActAgent{Model: model, Tools: registry, Memory: mem, IndexManager: indexManager, CheckpointPath: paths.CheckpointsDir()}
	case "reflection":
		return &agents.ReflectionAgent{
			Reviewer: model,
			Delegate: &agents.CodingAgent{Model: model, Tools: registry, Memory: mem, IndexManager: indexManager, CheckpointPath: paths.CheckpointsDir(), WorkflowStatePath: workflowStatePath},
		}
	default:
		return &agents.CodingAgent{Model: model, Tools: registry, Memory: mem, IndexManager: indexManager, CheckpointPath: paths.CheckpointsDir(), WorkflowStatePath: workflowStatePath}
	}
}

func instantiateDefinitionAgent(cfg Config, def *core.AgentDefinition, model core.LanguageModel, registry *capability.Registry, mem memory.MemoryStore, indexManager *ast.IndexManager) graph.Agent {
	paths := workspacecfg.New(cfg.Workspace)
	checkpointPath := paths.CheckpointsDir()
	workflowStatePath := paths.WorkflowStateFile()
	implementation := strings.ToLower(strings.TrimSpace(def.Spec.Implementation))
	switch implementation {
	case "planner":
		return &agents.PlannerAgent{Model: model, Tools: registry, Memory: mem}
	case "react":
		return &agents.ReActAgent{Model: model, Tools: registry, Memory: mem, IndexManager: indexManager, CheckpointPath: checkpointPath}
	case "reflection":
		return &agents.ReflectionAgent{
			Reviewer: model,
			Delegate: &agents.CodingAgent{Model: model, Tools: registry, Memory: mem, IndexManager: indexManager, CheckpointPath: checkpointPath, WorkflowStatePath: workflowStatePath},
		}
	case "architect":
		return &agents.ArchitectAgent{
			Model:             model,
			PlannerTools:      registry,
			ExecutorTools:     registry,
			Memory:            mem,
			IndexManager:      indexManager,
			CheckpointPath:    checkpointPath,
			WorkflowStatePath: workflowStatePath,
		}
	case "eternal":
		return &agents.EternalAgent{Model: model}
	case "coding", "expert", "":
		return &agents.CodingAgent{Model: model, Tools: registry, Memory: mem, IndexManager: indexManager, CheckpointPath: checkpointPath, WorkflowStatePath: workflowStatePath}
	default:
		return &agents.CodingAgent{Model: model, Tools: registry, Memory: mem, IndexManager: indexManager, CheckpointPath: checkpointPath, WorkflowStatePath: workflowStatePath}
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

// StartServer launches the HTTP API server. The returned stop function shuts
// the server down using the provided context.
func (r *Runtime) StartServer(ctx context.Context, addr string) (func(context.Context) error, error) {
	r.serverMu.Lock()
	defer r.serverMu.Unlock()
	if r.serverCancel != nil {
		return nil, errors.New("server already running")
	}
	if addr == "" {
		addr = r.Config.ServerAddr
	}
	api := &server.APIServer{
		Agent:             r.Agent,
		Context:           r.Context,
		Inspector:         r,
		Logger:            r.Logger,
		WorkflowStatePath: workspacecfg.New(r.Config.Workspace).WorkflowStateFile(),
	}
	serverCtx, cancel := context.WithCancel(ctx)
	errCh := make(chan error, 1)
	go func() {
		errCh <- api.ServeContext(serverCtx, addr)
	}()
	r.serverCancel = cancel
	stopFn := func(shutdownCtx context.Context) error {
		r.serverMu.Lock()
		if r.serverCancel == nil {
			r.serverMu.Unlock()
			return nil
		}
		r.serverCancel()
		r.serverCancel = nil
		r.serverMu.Unlock()
		select {
		case err := <-errCh:
			if err == nil || errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		case <-shutdownCtx.Done():
			return shutdownCtx.Err()
		}
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
func (r *Runtime) PendingHITL() []*fruntime.PermissionRequest {
	if r.Registration == nil || r.Registration.HITL == nil {
		return nil
	}
	return r.Registration.HITL.PendingRequests()
}

// SubscribeHITL streams HITL lifecycle events (requested/resolved/expired).
// The returned cancel function can be called to unsubscribe.
func (r *Runtime) SubscribeHITL() (<-chan fruntime.HITLEvent, func()) {
	if r == nil || r.Registration == nil || r.Registration.HITL == nil {
		ch := make(chan fruntime.HITLEvent)
		close(ch)
		return ch, func() {}
	}
	return r.Registration.HITL.Subscribe(32)
}

// ApproveHITL approves a pending request with the supplied scope.
func (r *Runtime) ApproveHITL(requestID, approver string, scope fruntime.GrantScope, duration time.Duration) error {
	if r.Registration == nil || r.Registration.HITL == nil {
		return errors.New("hitl broker unavailable")
	}
	if scope == "" {
		scope = fruntime.GrantScopeOneTime
	}
	var expiresAt time.Time
	if duration > 0 {
		expiresAt = time.Now().Add(duration)
	}
	decision := fruntime.PermissionDecision{
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

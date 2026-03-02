package runtime

import (
	"context"
	"errors"
	"fmt"
	"github.com/lexcodex/relurpify/agents"
	"github.com/lexcodex/relurpify/framework/ast"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/lexcodex/relurpify/framework/memory"
	fruntime "github.com/lexcodex/relurpify/framework/runtime"
	"github.com/lexcodex/relurpify/framework/telemetry"
	"github.com/lexcodex/relurpify/framework/toolsys"
	"github.com/lexcodex/relurpify/llm"
	"github.com/lexcodex/relurpify/server"
	"github.com/lexcodex/relurpify/tools"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Runtime wires the relurpish CLI, Bubble Tea UI, and API server to the shared
// agent fruntime. It centralizes tool registration, manifests, sandbox
// registration, and log management.
type Runtime struct {
	Config           Config
	Tools            *toolsys.ToolRegistry
	Memory           memory.MemoryStore
	Context          *core.Context
	Agent            graph.Agent
	Model            core.LanguageModel
	Registration     *fruntime.AgentRegistration
	AgentDefinitions map[string]*core.AgentDefinition
	Telemetry        core.Telemetry
	Logger           *log.Logger
	Workspace        WorkspaceConfig

	logFile io.Closer

	serverMu     sync.Mutex
	serverCancel context.CancelFunc
}

// New builds a fruntime. It always returns a usable Runtime instance even when
// sandbox or manifest verification fails so that the wizard/status views can
// surface actionable diagnostics.
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
	var allowedTools []string
	if cfg.ConfigPath != "" {
		if loaded, err := LoadWorkspaceConfig(cfg.ConfigPath); err == nil {
			workspaceCfg = loaded
			if workspaceCfg.Model != "" {
				cfg.OllamaModel = workspaceCfg.Model
			}
			if len(workspaceCfg.Agents) > 0 {
				cfg.AgentName = workspaceCfg.Agents[0]
			}
			allowedTools = append(allowedTools, workspaceCfg.AllowedTools...)
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
	registry, err := BuildToolRegistry(cfg.Workspace, runner, ToolRegistryOptions{
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

	agent := instantiateAgent(cfg, model, registry, memory, agentDefs, agentCfg)

	// Enforce the effective (post-definition) tool policies before initializing.
	if agentCfg.AgentSpec != nil {
		registry.UseAgentSpec(registration.ID, agentCfg.AgentSpec)
	}

	if err := agent.Initialize(agentCfg); err != nil {
		logFile.Close()
		return nil, fmt.Errorf("initialize agent: %w", err)
	}
	if reflection, ok := agent.(*agents.ReflectionAgent); ok {
		if reflection.Delegate != nil {
			_ = reflection.Delegate.Initialize(agentCfg)
		}
	}
	if len(allowedTools) > 0 {
		registry.RestrictTo(allowedTools)
	}
	rt := &Runtime{
		Config:           cfg,
		Tools:            registry,
		Memory:           memory,
		Context:          core.NewContext(),
		Agent:            agent,
		Model:            model,
		Logger:           logger,
		logFile:          logFile,
		Workspace:        workspaceCfg,
		Registration:     registration,
		AgentDefinitions: agentDefs,
		Telemetry:        telemetry,
	}
	for _, skill := range skillResults {
		if !skill.Applied || skill.Paths.Root == "" {
			continue
		}
		rt.Context.Set(fmt.Sprintf("skill.%s.path", skill.Name), skill.Paths.Root)
	}
	return rt, nil
}

// Close releases resources managed by fruntime.
func (r *Runtime) Close() error {
	if r.logFile != nil {
		return r.logFile.Close()
	}
	return nil
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
	agent := instantiateAgent(cfg, r.Model, r.Tools, r.Memory, r.AgentDefinitions, agentCfg)
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

// ToolRegistryOptions carries optional manifest/runtime policies into tool construction.
type ToolRegistryOptions struct {
	AgentID           string
	PermissionManager *fruntime.PermissionManager
	AgentSpec         *core.AgentRuntimeSpec
}

// BuildToolRegistry registers builtin tools scoped to the workspace.
func BuildToolRegistry(workspace string, runner fruntime.CommandRunner, opts ...ToolRegistryOptions) (*toolsys.ToolRegistry, error) {
	if workspace == "" {
		workspace = "."
	}
	if runner == nil {
		return nil, fmt.Errorf("command runner required")
	}
	var cfg ToolRegistryOptions
	if len(opts) > 0 {
		cfg = opts[0]
	}
	registry := toolsys.NewToolRegistry()
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
			return nil, err
		}
	}
	for _, tool := range []core.Tool{
		&tools.GrepTool{BasePath: workspace},
		&tools.SimilarityTool{BasePath: workspace},
		&tools.SemanticSearchTool{BasePath: workspace},
	} {
		if err := register(tool); err != nil {
			return nil, err
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
			return nil, err
		}
	}
	for _, tool := range []core.Tool{
		&tools.RunTestsTool{Command: []string{"go", "test", "./..."}, Workdir: workspace, Timeout: 10 * time.Minute, Runner: runner},
		&tools.RunLinterTool{Command: []string{"golangci-lint", "run"}, Workdir: workspace, Timeout: 5 * time.Minute, Runner: runner},
		&tools.RunBuildTool{Command: []string{"go", "build", "./..."}, Workdir: workspace, Timeout: 10 * time.Minute, Runner: runner},
		&tools.ExecuteCodeTool{Command: []string{"bash", "-c"}, Workdir: workspace, Timeout: 1 * time.Minute, Runner: runner},
	} {
		if err := register(tool); err != nil {
			return nil, err
		}
	}
	for _, tool := range tools.CommandLineTools(workspace, runner) {
		if err := register(tool); err != nil {
			return nil, err
		}
	}
	indexDir := filepath.Join(workspace, "relurpify_cfg", "memory", "ast_index")
	if err := os.MkdirAll(indexDir, 0o755); err != nil {
		return nil, err
	}
	store, err := ast.NewSQLiteStore(filepath.Join(indexDir, "index.db"))
	if err != nil {
		return nil, err
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
		return nil, err
	}
	go manager.IndexWorkspace()
	return registry, nil
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
func instantiateAgent(cfg Config, model core.LanguageModel, registry *toolsys.ToolRegistry, memory memory.MemoryStore, defs map[string]*core.AgentDefinition, agentCfg *core.Config) graph.Agent {
	// Check file-based definitions first
	if def, ok := defs[cfg.AgentName]; ok {
		// Update config with the definition's spec
		agentCfg.AgentSpec = &def.Spec
		agentCfg.OllamaToolCalling = def.Spec.ToolCallingEnabled()
		if def.Spec.Model.Name != "" {
			agentCfg.Model = def.Spec.Model.Name
		}

		// Use the Implementation field to pick struct
		switch def.Spec.Implementation {
		case "planner":
			return &agents.PlannerAgent{Model: model, Tools: registry, Memory: memory}
		case "react":
			return &agents.ReActAgent{Model: model, Tools: registry, Memory: memory}
		case "eternal":
			return &agents.EternalAgent{Model: model}
		// TODO: Add support for creating agents directly from 'def' struct fields (system prompt, etc)
		// For now we map them to existing Go structs.
		default:
			// Fallback to ReAct if unspecified but defined
			return &agents.ReActAgent{Model: model, Tools: registry, Memory: memory, Mode: string(def.Spec.Mode)}
		}
	}

	switch cfg.AgentLabel() {
	case "planner":
		return &agents.PlannerAgent{Model: model, Tools: registry, Memory: memory}
	case "react":
		return &agents.ReActAgent{Model: model, Tools: registry, Memory: memory}
	case "reflection":
		return &agents.ReflectionAgent{
			Reviewer: model,
			Delegate: &agents.CodingAgent{Model: model, Tools: registry, Memory: memory},
		}
	case "expert":
		return &agents.ExpertCoderAgent{Model: model, Tools: registry, Memory: memory}
	default:
		return &agents.CodingAgent{Model: model, Tools: registry, Memory: memory}
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
	return r.RunTask(ctx, task)
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
	api := &server.APIServer{Agent: r.Agent, Context: r.Context, Logger: r.Logger}
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
	decision := fruntime.PermissionDecision{
		RequestID:  requestID,
		Approved:   true,
		ApprovedBy: approver,
		Scope:      scope,
		ExpiresAt:  time.Now().Add(duration),
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

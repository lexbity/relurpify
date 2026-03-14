package runtime

import (
	"context"
	"encoding/json"
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
	"github.com/lexcodex/relurpify/app/nexus/db"
	"github.com/lexcodex/relurpify/framework/ast"
	fauthorization "github.com/lexcodex/relurpify/framework/authorization"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/capabilityplan"
	"github.com/lexcodex/relurpify/framework/config"
	contractpkg "github.com/lexcodex/relurpify/framework/contract"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/middleware/mcp/protocol"
	"github.com/lexcodex/relurpify/framework/policybundle"
	"github.com/lexcodex/relurpify/framework/retrieval"
	fsandbox "github.com/lexcodex/relurpify/framework/sandbox"
	"github.com/lexcodex/relurpify/framework/search"
	"github.com/lexcodex/relurpify/framework/telemetry"
	platformast "github.com/lexcodex/relurpify/platform/ast"
	platformfs "github.com/lexcodex/relurpify/platform/fs"
	platformgit "github.com/lexcodex/relurpify/platform/git"
	"github.com/lexcodex/relurpify/platform/llm"
	platformsearch "github.com/lexcodex/relurpify/platform/search"
	platformshell "github.com/lexcodex/relurpify/platform/shell"
)

// Runtime wires the relurpish CLI, Bubble Tea UI, and API server to the shared
// agent fruntime. It centralizes tool registration, manifests, sandbox
// registration, and log management.
type Runtime struct {
	Config               Config
	Tools                *capability.Registry
	Memory               memory.MemoryStore
	Context              *core.Context
	Agent                graph.Agent
	Model                core.LanguageModel
	IndexManager         *ast.IndexManager
	SearchEngine         *search.SearchEngine
	Registration         *fauthorization.AgentRegistration
	Delegations          *fauthorization.DelegationManager
	AgentSpec            *core.AgentRuntimeSpec
	AgentDefinitions     map[string]*core.AgentDefinition
	CapabilityAdmissions []capabilityplan.AdmissionResult
	EffectiveContract    *contractpkg.EffectiveAgentContract
	CompiledPolicy       *policybundle.CompiledPolicyBundle
	Telemetry            core.Telemetry
	Logger               *log.Logger
	Workspace            WorkspaceConfig
	NexusNodeProvider    core.NodeProvider
	NexusClient          *NexusClient

	logFile  io.Closer
	eventLog io.Closer

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

	memStore, err := memory.NewHybridMemory(cfg.MemoryPath)
	if err != nil {
		logFile.Close()
		return nil, fmt.Errorf("memory init: %w", err)
	}
	memStore = memStore.WithVectorStore(memory.NewInMemoryVectorStore())

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

	registration, err := fauthorization.RegisterAgent(ctx, fauthorization.RuntimeConfig{
		ManifestPath: cfg.ManifestPath,
		ConfigPath:   cfg.ConfigPath,
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
	agentSpec := agents.ApplyManifestDefaultsForAgent(registration.Manifest.Metadata.Name, registration.Manifest.Spec.Agent, registration.Manifest.Spec.Defaults)
	if agentSpec.Model.Name == "" {
		logFile.Close()
		return nil, fmt.Errorf("agent manifest missing spec.agent.model.name")
	}
	if cfg.OllamaModel == "" {
		cfg.OllamaModel = agentSpec.Model.Name
	}
	if cfg.OllamaModel == "" {
		logFile.Close()
		return nil, fmt.Errorf("ollama model not configured; update %s", cfg.ManifestPath)
	}
	runner, err := fsandbox.NewSandboxCommandRunner(registration.Manifest, registration.Runtime, cfg.Workspace)
	if err != nil {
		logFile.Close()
		return nil, err
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
	var eventTelemetry telemetry.EventTelemetry
	if cfg.EventsPath != "" {
		if err := os.MkdirAll(filepath.Dir(cfg.EventsPath), 0o755); err == nil {
			if eventLog, err := db.NewSQLiteEventLog(cfg.EventsPath); err == nil {
				eventTelemetry = telemetry.EventTelemetry{
					Log:       eventLog,
					Partition: "local",
					Actor:     core.EventActor{Kind: "agent", ID: registration.ID, Label: cfg.AgentLabel()},
				}
				sinks = append(sinks, eventTelemetry)
				if registration.Permissions != nil {
					registration.Permissions.SetEventLogger(func(ctx context.Context, desc core.PermissionDescriptor, effect, reason string, fields map[string]interface{}) {
						payload := map[string]interface{}{
							"permission_type": desc.Type,
							"action":          desc.Action,
							"resource":        desc.Resource,
							"effect":          effect,
							"reason":          reason,
							"metadata":        fields,
						}
						if data, err := json.Marshal(payload); err == nil {
							_, _ = eventLog.Append(ctx, "local", []core.FrameworkEvent{{
								Timestamp: time.Now().UTC(),
								Type:      core.FrameworkEventPolicyEvaluated,
								Payload:   data,
								Actor:     core.EventActor{Kind: "agent", ID: registration.ID, Label: cfg.AgentLabel()},
								Partition: "local",
							}})
						}
					})
				}
			} else {
				logger.Printf("warning: failed to init event log: %v", err)
			}
		}
	}
	telemetry := telemetry.MultiplexTelemetry{Sinks: sinks}

	logLLM := false
	if agentSpec.Logging != nil && agentSpec.Logging.LLM != nil {
		logLLM = *agentSpec.Logging.LLM
	}
	modelClient := llm.NewClient(cfg.OllamaEndpoint, cfg.OllamaModel)
	modelClient.SetDebugLogging(logLLM)
	model := llm.NewInstrumentedModel(modelClient, telemetry, logLLM)

	if cfg.AgentName == "" {
		cfg.AgentName = registration.Manifest.Metadata.Name
	}
	boot, err := BootstrapAgentRuntime(cfg.Workspace, AgentBootstrapOptions{
		Context:             ctx,
		AgentID:             registration.ID,
		AgentName:           cfg.AgentName,
		ConfigName:          cfg.AgentLabel(),
		AgentsDir:           cfg.AgentsDir,
		Manifest:            registration.Manifest,
		PermissionManager:   registration.Permissions,
		Runner:              runner,
		Model:               model,
		Memory:              memStore,
		Telemetry:           telemetry,
		OllamaEndpoint:      cfg.OllamaEndpoint,
		OllamaModel:         cfg.OllamaModel,
		MaxIterations:       8,
		AllowedCapabilities: allowedCapabilities,
		DebugLLM:            logLLM,
	})
	if err != nil {
		logFile.Close()
		return nil, err
	}
	registry := boot.Registry
	indexManager := boot.IndexManager
	searchEngine := boot.SearchEngine
	agentSpec = boot.AgentSpec
	agentCfg := boot.AgentConfig
	agentEnv := boot.Environment
	agentDefs := boot.AgentDefinitions
	skillResults := boot.SkillResults
	compiledPolicy := boot.CompiledPolicy
	if compiledPolicy == nil {
		var err error
		contract := boot.Contract
		if contract == nil {
			contract = &contractpkg.EffectiveAgentContract{
				AgentID:   registration.ID,
				AgentSpec: agentSpec,
			}
		}
		compiledPolicy, err = policybundle.BuildFromContract(contract, registration.Permissions)
		if err != nil {
			logFile.Close()
			return nil, fmt.Errorf("compile effective policy: %w", err)
		}
	}
	registration.Policy = compiledPolicy.Engine
	registry.SetPolicyEngine(compiledPolicy.Engine)

	rt := &Runtime{
		Config:               cfg,
		Tools:                registry,
		Memory:               memStore,
		Context:              core.NewContext(),
		Model:                model,
		IndexManager:         indexManager,
		SearchEngine:         searchEngine,
		Logger:               logger,
		logFile:              logFile,
		eventLog:             io.Closer(nil),
		Workspace:            workspaceCfg,
		Registration:         registration,
		Delegations:          fauthorization.NewDelegationManager(),
		AgentSpec:            agentSpec,
		AgentDefinitions:     agentDefs,
		CapabilityAdmissions: boot.CapabilityAdmissions,
		EffectiveContract:    boot.Contract,
		CompiledPolicy:       compiledPolicy,
		Telemetry:            telemetry,
	}
	if eventTelemetry.Log != nil {
		if closer, ok := eventTelemetry.Log.(io.Closer); ok {
			rt.eventLog = closer
		}
		if registration.HITL != nil {
			ch, cancel := registration.HITL.Subscribe(32)
			rt.hitlCancel = cancel
			go func() {
				for ev := range ch {
					eventTelemetry.EmitHITLEvent(ev)
				}
			}()
		}
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
	if err := registerNexusGatewayProvider(ctx, rt); err != nil {
		_ = rt.Close()
		return nil, fmt.Errorf("register nexus gateway provider: %w", err)
	}
	if err := registerLocalNexusNodeProvider(ctx, rt); err != nil {
		_ = rt.Close()
		return nil, fmt.Errorf("register local nexus node: %w", err)
	}

	agent := instantiateAgent(cfg, agentEnv, agentDefs)

	// Enforce the effective (post-definition) tool policies before initializing.
	if agentCfg.AgentSpec != nil {
		registry.UseAgentSpec(registration.ID, agentCfg.AgentSpec)
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
	if r.hitlCancel != nil {
		r.hitlCancel()
		r.hitlCancel = nil
	}
	if r.nexusCancel != nil {
		r.nexusCancel()
		r.nexusCancel = nil
	}
	if r.eventLog != nil {
		if err := r.eventLog.Close(); err != nil {
			errs = append(errs, err)
		}
		r.eventLog = nil
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
	if name == "" && r.Registration != nil {
		name = strings.TrimSpace(r.Registration.ID)
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

func (r *Runtime) applyResolvedAgentState(name string, effectiveContract *contractpkg.EffectiveAgentContract, compiledPolicy *policybundle.CompiledPolicyBundle, agentDefs map[string]*core.AgentDefinition) error {
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
	if effectiveContract.AgentSpec != nil && effectiveContract.AgentSpec.Model.Name != "" && effectiveContract.AgentSpec.Model.Name != cfg.OllamaModel {
		return fmt.Errorf("agent %s requires model %s; restart to switch models", name, effectiveContract.AgentSpec.Model.Name)
	}
	if err := ensureStableSkillCapabilityTopology(r.EffectiveContract, effectiveContract); err != nil {
		return err
	}
	agentCfg := &core.Config{
		Name:              cfg.AgentLabel(),
		Model:             cfg.OllamaModel,
		OllamaEndpoint:    cfg.OllamaEndpoint,
		MaxIterations:     8,
		OllamaToolCalling: effectiveContract.AgentSpec.ToolCallingEnabled(),
		AgentSpec:         effectiveContract.AgentSpec,
		Telemetry:         r.Telemetry,
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
	r.Tools.UseAgentSpec(r.Registration.ID, effectiveContract.AgentSpec)
	r.Tools.SetPolicyEngine(compiledPolicy.Engine)
	r.Registration.Policy = compiledPolicy.Engine
	r.Agent = agent
	r.AgentSpec = effectiveContract.AgentSpec
	r.AgentDefinitions = agentDefs
	r.EffectiveContract = effectiveContract
	r.CompiledPolicy = compiledPolicy
	r.CapabilityAdmissions = capabilityplan.EvaluateSkillCapabilities(
		effectiveContract.ResolvedSkills,
		core.EffectiveAllowedCapabilitySelectors(effectiveContract.AgentSpec),
	)
	r.syncSkillContextPaths(effectiveContract.SkillResults)
	r.Config.AgentName = name
	return nil
}

// CapabilityRegistryOptions carries optional manifest/runtime policies into capability construction.
type CapabilityRegistryOptions struct {
	Context           context.Context
	AgentID           string
	PermissionManager *fauthorization.PermissionManager
	AgentSpec         *core.AgentRuntimeSpec
	OllamaEndpoint    string
	OllamaModel       string
	SkipASTIndex      bool
}

// CapabilityBundle groups the runtime-scoped capability registry and the
// shared indexing/search services built alongside it.
type CapabilityBundle struct {
	Registry     *capability.Registry
	IndexManager *ast.IndexManager
	SearchEngine *search.SearchEngine
}

// BuildBuiltinCapabilityBundle registers builtin tool capabilities scoped to
// the workspace without resolving a full runtime contract. It is intended for
// tests and low-level tooling that only need the builtin capability bundle.
func BuildBuiltinCapabilityBundle(workspace string, runner fsandbox.CommandRunner, opts ...CapabilityRegistryOptions) (*CapabilityBundle, error) {
	if workspace == "" {
		workspace = "."
	}
	if runner == nil {
		return nil, fmt.Errorf("command runner required")
	}
	var cfg CapabilityRegistryOptions
	if len(opts) > 0 {
		cfg = opts[0]
	}
	buildCtx := cfg.Context
	if buildCtx == nil {
		buildCtx = context.Background()
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
	for _, tool := range platformfs.FileOperations(workspace) {
		if err := register(tool); err != nil {
			return nil, err
		}
	}
	for _, tool := range []core.Tool{
		&platformsearch.SimilarityTool{BasePath: workspace},
		&platformsearch.SemanticSearchTool{BasePath: workspace},
	} {
		if err := register(tool); err != nil {
			return nil, err
		}
	}
	for _, tool := range []core.Tool{
		&platformgit.GitCommandTool{RepoPath: workspace, Command: "diff", Runner: runner},
		&platformgit.GitCommandTool{RepoPath: workspace, Command: "history", Runner: runner},
		&platformgit.GitCommandTool{RepoPath: workspace, Command: "branch", Runner: runner},
		&platformgit.GitCommandTool{RepoPath: workspace, Command: "commit", Runner: runner},
		&platformgit.GitCommandTool{RepoPath: workspace, Command: "blame", Runner: runner},
	} {
		if err := register(tool); err != nil {
			return nil, err
		}
	}
	for _, tool := range platformshell.CommandLineTools(workspace, runner) {
		if err := register(tool); err != nil {
			return nil, err
		}
	}
	paths := config.New(workspace)
	indexDir := paths.ASTIndexDir()
	if err := os.MkdirAll(indexDir, 0o755); err != nil {
		return nil, err
	}
	store, err := ast.NewSQLiteStore(paths.ASTIndexDB())
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
	platformast.AttachASTSymbolProvider(manager, registry)
	if err := register(platformast.NewASTTool(manager)); err != nil {
		return nil, err
	}
	codeIndex, err := memory.NewCodeIndex(workspace, filepath.Join(paths.MemoryDir(), "code_index.json"))
	if err != nil {
		return nil, err
	}
	if cfg.PermissionManager != nil {
		codeIndex.SetPathFilter(func(path string, isDir bool) bool {
			action := core.FileSystemRead
			if isDir {
				action = core.FileSystemList
			}
			return cfg.PermissionManager.CheckFileAccess(context.Background(), cfg.AgentID, action, path) == nil
		})
	}
	if err := codeIndex.BuildIndex(buildCtx); err != nil {
		if !shouldIgnoreBootstrapIndexError(err) {
			return nil, err
		}
		log.Printf("runtime bootstrap warning: code index build incomplete: %v", err)
	}
	if err := codeIndex.Save(); err != nil {
		return nil, err
	}
	if cfg.SkipASTIndex {
		// Test-only fast path: agenttests can skip AST/bootstrap indexing when
		// isolating execution behavior from slow workspace indexing and embedding.
		searchEngine := search.NewSearchEngine(nil, codeIndex)
		if searchEngine == nil {
			return nil, fmt.Errorf("search engine initialization failed")
		}
		return &CapabilityBundle{
			Registry:     registry,
			IndexManager: manager,
			SearchEngine: searchEngine,
		}, nil
	}
	if err := manager.StartIndexing(buildCtx); err != nil {
		return nil, err
	}
	if err := manager.WaitUntilReady(buildCtx); err != nil {
		if !shouldIgnoreBootstrapIndexError(err) {
			return nil, err
		}
		log.Printf("runtime bootstrap warning: AST index readiness incomplete: %v", err)
	}
	var semanticStore search.SemanticStore
	if strings.TrimSpace(cfg.OllamaModel) != "" {
		retrievalDB, err := openRetrievalDB(paths.RetrievalDB())
		if err != nil {
			return nil, err
		}
		embedder := retrieval.NewOllamaEmbedder(cfg.OllamaEndpoint, cfg.OllamaModel)
		if err := ingestCodeIndex(buildCtx, workspace, codeIndex, retrieval.NewIngestionPipeline(retrievalDB, embedder)); err != nil {
			if !shouldIgnoreBootstrapIndexError(err) {
				return nil, err
			}
			log.Printf("runtime bootstrap warning: semantic ingestion incomplete: %v", err)
		} else {
			semanticStore = &retrieverSemanticAdapter{retriever: retrieval.NewRetriever(retrievalDB, embedder)}
		}
	}
	searchEngine := search.NewSearchEngine(semanticStore, codeIndex)
	if searchEngine == nil {
		return nil, fmt.Errorf("search engine initialization failed")
	}
	return &CapabilityBundle{
		Registry:     registry,
		IndexManager: manager,
		SearchEngine: searchEngine,
	}, nil
}

func shouldIgnoreBootstrapIndexError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded) ||
		strings.Contains(err.Error(), "no parser for ")
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
func instantiateAgent(cfg Config, env agents.AgentEnvironment, defs map[string]*core.AgentDefinition) graph.Agent {
	paths := config.New(cfg.Workspace)
	// Check file-based definitions first
	if def, ok := defs[cfg.AgentName]; ok {
		spec := env.Config.AgentSpec
		if spec == nil {
			spec = &def.Spec
			env.Config.AgentSpec = spec
		}
		env.Config.OllamaToolCalling = spec.ToolCallingEnabled()
		if spec.Model.Name != "" {
			env.Config.Model = spec.Model.Name
		}

		return instantiateDefinitionAgent(cfg, def, env)
	}

	builder := agents.NewAgentBuilder().WithEnvironment(&env)
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

func instantiateDefinitionAgent(cfg Config, def *core.AgentDefinition, env agents.AgentEnvironment) graph.Agent {
	paths := config.New(cfg.Workspace)
	spec := def.Spec
	if env.Config != nil && env.Config.AgentSpec != nil {
		spec = *env.Config.AgentSpec
	}
	agent, err := agents.BuildFromSpec(env, spec)
	if err != nil {
		agent, _ = agents.BuildFromSpec(env, core.AgentRuntimeSpec{Implementation: "react"})
	}
	return configureBuiltAgent(agent, paths)
}

func (r *Runtime) resolveEffectiveContractForAgent(name string) (*contractpkg.EffectiveAgentContract, *policybundle.CompiledPolicyBundle, map[string]*core.AgentDefinition, error) {
	agentDefs := r.AgentDefinitions
	if r.Config.AgentsDir != "" {
		loaded, err := LoadAgentDefinitions(r.Config.AgentsDir)
		if err != nil && !os.IsNotExist(err) {
			return nil, nil, nil, fmt.Errorf("load agent definitions: %w", err)
		}
		if loaded != nil {
			agentDefs = loaded
		}
	}
	effectiveContract, err := contractpkg.ResolveEffectiveAgentContract(r.Config.Workspace, r.Registration.Manifest, contractpkg.ResolveOptions{
		AgentOverlays: selectedAgentDefinitionOverlays(name, agentDefs),
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("resolve effective contract: %w", err)
	}
	compiledPolicy, err := policybundle.BuildFromContract(effectiveContract, r.Registration.Permissions)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("compile effective policy: %w", err)
	}
	return effectiveContract, compiledPolicy, agentDefs, nil
}

func ensureStableSkillCapabilityTopology(current, next *contractpkg.EffectiveAgentContract) error {
	currentIDs := skillCapabilityIDSet(current)
	nextIDs := skillCapabilityIDSet(next)
	if len(currentIDs) != len(nextIDs) {
		return fmt.Errorf("agent switch changes skill capability topology; restart required to rebuild runtime registry")
	}
	for id := range currentIDs {
		if _, ok := nextIDs[id]; !ok {
			return fmt.Errorf("agent switch changes skill capability topology; restart required to rebuild runtime registry")
		}
	}
	return nil
}

func skillCapabilityIDSet(contract *contractpkg.EffectiveAgentContract) map[string]struct{} {
	if contract == nil {
		return nil
	}
	candidates := agents.EnumerateSkillCapabilities(contract.ResolvedSkills)
	if len(candidates) == 0 {
		return nil
	}
	ids := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		if candidate.Descriptor.ID == "" {
			continue
		}
		ids[candidate.Descriptor.ID] = struct{}{}
	}
	return ids
}

func (r *Runtime) syncSkillContextPaths(results []agents.SkillResolution) {
	if r == nil || r.Context == nil {
		return
	}
	for _, skill := range results {
		if !skill.Applied {
			continue
		}
		r.Context.Set(fmt.Sprintf("skill.%s.path", skill.Name), skill.Paths.Root)
	}
}

func configureBuiltAgent(agent graph.Agent, paths config.Paths) graph.Agent {
	switch typed := agent.(type) {
	case *agents.ReActAgent:
		typed.CheckpointPath = paths.CheckpointsDir()
	case *agents.PlannerAgent:
		typed.CheckpointPath = paths.CheckpointsDir()
	case *agents.ArchitectAgent:
		typed.CheckpointPath = paths.CheckpointsDir()
		typed.WorkflowStatePath = paths.WorkflowStateFile()
	case *agents.HTNAgent:
		typed.CheckpointPath = paths.WorkflowStateFile()
	case *agents.PipelineAgent:
		typed.WorkflowStatePath = paths.WorkflowStateFile()
	}
	if reflection, ok := agent.(*agents.ReflectionAgent); ok {
		if delegate, ok := reflection.Delegate.(*agents.ReActAgent); ok {
			delegate.CheckpointPath = paths.CheckpointsDir()
		}
	}
	return agent
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
		if sessionKey, ok := task.Context["session_key"]; ok && strings.TrimSpace(fmt.Sprint(sessionKey)) != "" {
			normalized := strings.TrimSpace(fmt.Sprint(sessionKey))
			state.Set("session_key", normalized)
			state.Set("nexus.session_key", normalized)
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
	if r.Registration == nil || r.Registration.HITL == nil {
		return nil
	}
	return r.Registration.HITL.PendingRequests()
}

// SubscribeHITL streams HITL lifecycle events (requested/resolved/expired).
// The returned cancel function can be called to unsubscribe.
func (r *Runtime) SubscribeHITL() (<-chan fauthorization.HITLEvent, func()) {
	if r == nil || r.Registration == nil || r.Registration.HITL == nil {
		ch := make(chan fauthorization.HITLEvent)
		close(ch)
		return ch, func() {}
	}
	return r.Registration.HITL.Subscribe(32)
}

// ApproveHITL approves a pending request with the supplied scope.
func (r *Runtime) ApproveHITL(requestID, approver string, scope fauthorization.GrantScope, duration time.Duration) error {
	if r.Registration == nil || r.Registration.HITL == nil {
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

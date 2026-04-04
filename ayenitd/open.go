package ayenitd

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	fauthorization "github.com/lexcodex/relurpify/framework/authorization"
	"github.com/lexcodex/relurpify/framework/config"
	"github.com/lexcodex/relurpify/framework/guidance"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/retrieval"
	fsandbox "github.com/lexcodex/relurpify/framework/sandbox"
	"github.com/lexcodex/relurpify/platform/llm"
)

// Open initializes a complete workspace session: platform checks, store
// opening, service graph construction, agent registration, and background
// indexing. The returned *Workspace is ready for agent construction.
//
// Open is the single composition root for all Relurpify entry points.
// app/relurpish, app/dev-agent-cli, and integration tests all call Open().
func Open(ctx context.Context, cfg WorkspaceConfig) (*Workspace, error) {
	// Phase A: Configuration Validation
	if err := validateConfig(cfg); err != nil {
		return nil, fmt.Errorf("invalid workspace config: %w", err)
	}

	// Phase B: Platform Runtime Checks
	results := ProbeWorkspace(cfg)
	for _, r := range results {
		if r.Required && !r.OK {
			return nil, fmt.Errorf("platform check failed: %s", r.Message)
		}
	}

	// Phase C: Log and Telemetry Setup
	logFile, logger, err := setupLogging(cfg)
	if err != nil {
		return nil, err
	}

	// Phase D: Store Initialization
	workflowStore, planStore, patternStore, commentStore, patternDB, err := openRuntimeStores(cfg.Workspace)
	if err != nil {
		logFile.Close()
		return nil, fmt.Errorf("open runtime stores: %w", err)
	}

	// Phase E: Agent Registration + Authorization
	var sandboxCfg fsandbox.SandboxConfig
	if cfg.Sandbox {
		// Use default sandbox configuration when sandbox is enabled
		sandboxCfg = fsandbox.SandboxConfig{
			// Set appropriate defaults
			NetworkIsolation: true,
			ReadOnlyRoot:     false,
		}
	}
	registration, err := fauthorization.RegisterAgent(ctx, fauthorization.RuntimeConfig{
		ManifestPath: cfg.ManifestPath,
		ConfigPath:   cfg.ConfigPath,
		Sandbox:      sandboxCfg,
		AuditLimit:   cfg.AuditLimit,
		BaseFS:       cfg.Workspace,
		HITLTimeout:  cfg.HITLTimeout,
	})
	if err != nil {
		patternDB.Close()
		workflowStore.Close()
		logFile.Close()
		return nil, fmt.Errorf("sandbox registration failed: %w", err)
	}

	// Phase F: Capability Bundle + Agent Environment
	runner, err := fsandbox.NewSandboxCommandRunner(registration.Manifest, registration.Runtime, cfg.Workspace)
	if err != nil {
		patternDB.Close()
		workflowStore.Close()
		logFile.Close()
		return nil, err
	}
	memStore, err := memory.NewHybridMemory(cfg.MemoryPath)
	if err != nil {
		patternDB.Close()
		workflowStore.Close()
		logFile.Close()
		return nil, fmt.Errorf("memory init: %w", err)
	}
	memStore = memStore.WithVectorStore(memory.NewInMemoryVectorStore())

	modelClient := llm.NewClient(cfg.OllamaEndpoint, cfg.OllamaModel)
	modelClient.SetDebugLogging(cfg.DebugLLM)
	model := llm.NewInstrumentedModel(modelClient, nil, cfg.DebugLLM)
	guidanceBroker := guidance.NewGuidanceBroker(0)

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
		Telemetry:           nil, // TODO: telemetry
		OllamaEndpoint:      cfg.OllamaEndpoint,
		OllamaModel:         cfg.OllamaModel,
		MaxIterations:       cfg.MaxIterations,
		SkipASTIndex:        cfg.SkipASTIndex,
		AllowedCapabilities: cfg.AllowedCapabilities,
		DebugLLM:            cfg.DebugLLM,
		DebugAgent:          cfg.DebugAgent,
		PatternStore:        patternStore,
		CommentStore:        commentStore,
		RetrievalDB:         workflowStore.DB(),
		PlanStore:           planStore,
		GuidanceBroker:      guidanceBroker,
		WorkflowStore:       workflowStore,
	})
	if err != nil {
		patternDB.Close()
		workflowStore.Close()
		logFile.Close()
		return nil, err
	}

	// Phase H: Embedder Initialization
	embedder := retrieval.NewOllamaEmbedder(cfg.OllamaEndpoint, cfg.OllamaModel)

	// Phase I: Scheduler Start
	scheduler := NewServiceScheduler()
	// TODO: load cron-from-memory jobs
	scheduler.Start(ctx)

	// Build WorkspaceEnvironment
	env := boot.Environment
	env.Embedder = embedder
	env.Scheduler = scheduler
	env.GuidanceBroker = guidanceBroker
	env.PermissionManager = registration.Permissions
	env.CheckpointStore = nil // TODO: implement in framework

	ws := &Workspace{
		Environment:       env,
		Registration:      registration,
		logFile:           logFile,
		patternDB:         patternDB,
		AgentSpec:         boot.AgentSpec,
		AgentDefinitions:  boot.AgentDefinitions,
		CompiledPolicy:    boot.CompiledPolicy,
		EffectiveContract: boot.Contract,
	}
	// Store logger for cleanup
	ws.eventLog = nil // TODO: set up event log

	logger.Printf("ayenitd: workspace opened successfully")
	return ws, nil
}

func validateConfig(cfg WorkspaceConfig) error {
	if cfg.Workspace == "" {
		return fmt.Errorf("Workspace is required")
	}
	if cfg.ManifestPath == "" {
		return fmt.Errorf("ManifestPath is required")
	}
	if cfg.OllamaEndpoint == "" {
		return fmt.Errorf("OllamaEndpoint is required")
	}
	if cfg.OllamaModel == "" {
		return fmt.Errorf("OllamaModel is required")
	}
	return nil
}

func setupLogging(cfg WorkspaceConfig) (*os.File, *log.Logger, error) {
	logPath := cfg.LogPath
	if logPath == "" {
		// Default log path
		paths := config.New(cfg.Workspace)
		logPath = filepath.Join(paths.LogsDir(), "ayenitd.log")
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return nil, nil, fmt.Errorf("create log directory: %w", err)
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, nil, fmt.Errorf("open log: %w", err)
	}
	logger := log.New(logFile, "ayenitd ", log.LstdFlags|log.Lmicroseconds)
	return logFile, logger, nil
}


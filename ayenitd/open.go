package ayenitd

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	fauthorization "github.com/lexcodex/relurpify/framework/authorization"
	"github.com/lexcodex/relurpify/framework/config"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/guidance"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/retrieval"
	fsandbox "github.com/lexcodex/relurpify/framework/sandbox"
	"github.com/lexcodex/relurpify/framework/telemetry"
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

	// Resolve workspace YAML overrides before probing or opening stores.
	cfg = resolveWorkspaceConfigOverrides(cfg)

	// Phase B: Platform Runtime Checks
	results := ProbeWorkspace(cfg)
	for _, r := range results {
		if r.Required && !r.OK {
			return nil, fmt.Errorf("platform check failed: %s", r.Message)
		}
	}

	// Phase C: Log and Telemetry Setup
	logFile, logger, tel, err := setupTelemetry(cfg)
	if err != nil {
		return nil, err
	}

	// Phase D: Store Initialization
	workflowStore, planStore, patternStore, commentStore, knowledgeStore, patternDB, err := openRuntimeStores(cfg.Workspace)
	if err != nil {
		logFile.Close()
		return nil, fmt.Errorf("open runtime stores: %w", err)
	}

	// Phase E: Agent Registration + Authorization
	registration, err := fauthorization.RegisterAgent(ctx, fauthorization.RuntimeConfig{
		ManifestPath: cfg.ManifestPath,
		ConfigPath:   cfg.ConfigPath,
		Sandbox:      cfg.Sandbox,
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

	// Resolve model from manifest if not overridden in config.
	ollamaModel := cfg.OllamaModel
	if registration.Manifest != nil && registration.Manifest.Spec.Agent != nil {
		if specModel := registration.Manifest.Spec.Agent.Model.Name; specModel != "" && ollamaModel == "" {
			ollamaModel = specModel
		}
	}

	logLLM := cfg.DebugLLM
	if registration.Manifest != nil && registration.Manifest.Spec.Agent != nil {
		if registration.Manifest.Spec.Agent.Logging != nil && registration.Manifest.Spec.Agent.Logging.LLM != nil {
			logLLM = *registration.Manifest.Spec.Agent.Logging.LLM
		}
	}
	modelClient := llm.NewClient(cfg.OllamaEndpoint, ollamaModel)
	modelClient.SetDebugLogging(logLLM)
	model := llm.NewInstrumentedModel(modelClient, tel, logLLM)
	guidanceBroker := guidance.NewGuidanceBroker(0)

	// Wire permission event logger if event telemetry is available.
	if et, ok := tel.(interface {
		EmitPermissionEvent(ctx context.Context, desc core.PermissionDescriptor, effect, reason string, fields map[string]interface{})
	}); ok {
		if registration.Permissions != nil {
			registration.Permissions.SetEventLogger(func(ctx context.Context, desc core.PermissionDescriptor, effect, reason string, fields map[string]interface{}) {
				et.EmitPermissionEvent(ctx, desc, effect, reason, fields)
			})
		}
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
		Telemetry:           tel,
		OllamaEndpoint:      cfg.OllamaEndpoint,
		OllamaModel:         ollamaModel,
		MaxIterations:       cfg.MaxIterations,
		SkipASTIndex:        cfg.SkipASTIndex,
		AllowedCapabilities: cfg.AllowedCapabilities,
		DebugLLM:            logLLM,
		DebugAgent:          cfg.DebugAgent,
		PatternStore:        patternStore,
		CommentStore:        commentStore,
		RetrievalDB:         workflowStore.DB(),
		PlanStore:           planStore,
		GuidanceBroker:      guidanceBroker,
		WorkflowStore:       workflowStore,
		KnowledgeStore:      knowledgeStore,
	})
	if err != nil {
		patternDB.Close()
		workflowStore.Close()
		logFile.Close()
		return nil, err
	}

	// Apply compiled policy engine.
	if boot.CompiledPolicy != nil {
		registration.Policy = boot.CompiledPolicy.Engine
		boot.Environment.Registry.SetPolicyEngine(boot.CompiledPolicy.Engine)
	}

	// Phase H: Embedder Initialization
	embedder := retrieval.NewOllamaEmbedder(cfg.OllamaEndpoint, ollamaModel)

	// Phase I: Scheduler Start
	scheduler := NewServiceScheduler()
	if err := scheduler.LoadJobsFromMemory(ctx, memStore); err != nil {
		logger.Printf("scheduler: failed to load jobs from memory: %v", err)
	}
	if cfg.ReindexInterval > 0 {
		scheduler.Register(ScheduledJob{
			ID:       "reindex-workspace",
			Interval: cfg.ReindexInterval,
			Source:   "internal",
			Action: func(ctx context.Context) error {
				logger.Printf("scheduler: re-indexing workspace")
				if boot.Environment.IndexManager != nil {
					go func() {
						if err := boot.Environment.IndexManager.StartIndexing(ctx); err != nil {
							logger.Printf("scheduler: re-index failed: %v", err)
						} else {
							logger.Printf("scheduler: re-index completed")
						}
					}()
				}
				return nil
			},
		})
		logger.Printf("scheduler: registered re-index job (interval: %s)", cfg.ReindexInterval)
	}
	scheduler.Start(ctx)

	// Build WorkspaceEnvironment
	env := boot.Environment
	env.Embedder = embedder
	env.Scheduler = scheduler
	env.GuidanceBroker = guidanceBroker
	env.PermissionManager = registration.Permissions
	env.CheckpointStore = nil // TODO: implement in framework
	env.KnowledgeStore = knowledgeStore

	ws := &Workspace{
		Environment:          env,
		Registration:         registration,
		logFile:              logFile,
		patternDB:            patternDB,
		AgentSpec:            boot.AgentSpec,
		AgentDefinitions:     boot.AgentDefinitions,
		CompiledPolicy:       boot.CompiledPolicy,
		EffectiveContract:    boot.Contract,
		CapabilityAdmissions: boot.CapabilityAdmissions,
		SkillResults:         boot.SkillResults,
		Telemetry:            tel,
		Logger:               logger,
	}

	logger.Printf("ayenitd: workspace opened successfully")
	return ws, nil
}

// resolveWorkspaceConfigOverrides loads the workspace YAML (if ConfigPath is
// set) and applies model and agent-name overrides. Errors are silently ignored
// so that a missing or malformed config file does not prevent startup.
func resolveWorkspaceConfigOverrides(cfg WorkspaceConfig) WorkspaceConfig {
	if cfg.ConfigPath == "" {
		return cfg
	}
	type yamlCfg struct {
		Model  string   `json:"model" yaml:"model"`
		Agents []string `json:"agents" yaml:"agents"`
	}
	data, err := os.ReadFile(cfg.ConfigPath)
	if err != nil {
		return cfg
	}
	// Try JSON first (YAML is a superset, but we keep it simple here).
	var yc yamlCfg
	if jsonErr := json.Unmarshal(data, &yc); jsonErr == nil {
		if yc.Model != "" && cfg.OllamaModel == "" {
			cfg.OllamaModel = yc.Model
		}
		if len(yc.Agents) > 0 && cfg.AgentName == "" {
			cfg.AgentName = yc.Agents[0]
		}
	}
	return cfg
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

// setupTelemetry opens the log file, creates a logger, and assembles the
// telemetry sink chain (logger + optional JSON file). Returns the log file
// (which must be closed by the caller), the logger, and the assembled telemetry.
func setupTelemetry(cfg WorkspaceConfig) (*os.File, *log.Logger, core.Telemetry, error) {
	logPath := cfg.LogPath
	if logPath == "" {
		paths := config.New(cfg.Workspace)
		logPath = filepath.Join(paths.LogsDir(), "ayenitd.log")
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return nil, nil, nil, fmt.Errorf("create log directory: %w", err)
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("open log: %w", err)
	}
	logger := log.New(logFile, "ayenitd ", log.LstdFlags|log.Lmicroseconds)

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

	return logFile, logger, telemetry.MultiplexTelemetry{Sinks: sinks}, nil
}

// permissionEventLogger wraps a core.Telemetry to provide a SetEventLogger
// callback. Used to emit permission audit events when telemetry is a multiplex.
type permissionEventLogger struct {
	log     *log.Logger
	agentID string
}

func (l *permissionEventLogger) emit(ctx context.Context, desc core.PermissionDescriptor, effect, reason string, fields map[string]interface{}) {
	if l.log == nil {
		return
	}
	l.log.Printf("permission: agent=%s type=%s action=%s resource=%s effect=%s reason=%s",
		l.agentID, desc.Type, desc.Action, desc.Resource, effect, reason)
}

// fakeNow returns the current time for scheduler tick alignment.
func fakeNow() time.Time { return time.Now() }

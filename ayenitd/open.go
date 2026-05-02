package ayenitd

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	nexusdb "codeburg.org/lexbit/relurpify/app/nexus/db"
	fauthorization "codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/compiler"
	"codeburg.org/lexbit/relurpify/framework/contextpolicy"
	"codeburg.org/lexbit/relurpify/framework/contextstream"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/knowledge"
	"codeburg.org/lexbit/relurpify/framework/manifest"
	"codeburg.org/lexbit/relurpify/framework/retrieval"
	fsandbox "codeburg.org/lexbit/relurpify/framework/sandbox"
	"codeburg.org/lexbit/relurpify/framework/telemetry"
	"codeburg.org/lexbit/relurpify/platform/contracts"
	"codeburg.org/lexbit/relurpify/platform/llm"
	"codeburg.org/lexbit/relurpify/relurpnet/identity"
	"gopkg.in/yaml.v3"
)

var (
	newLLMBackendFn             = llm.New
	applyProfileFn              = llm.ApplyProfile
	probeWorkspaceFn            = ProbeWorkspace
	setupTelemetryFn            = setupTelemetry
	openKnowledgeStoreFn        = openKnowledgeStore
	loadAgentManifestSnapshotFn = manifest.LoadAgentManifestSnapshot
	registerAgentFn             = fauthorization.RegisterAgent
	newCommandRunnerFn          = fsandbox.NewCommandRunner
	newProfileRegistryFn        = llm.NewProfileRegistry
	newInstrumentedModelFn      = func(inner contracts.LanguageModel, telemetry contracts.Telemetry, debug bool) contracts.LanguageModel {
		return llm.NewInstrumentedModel(inner, telemetry, debug)
	}
	bootstrapAgentRuntimeFn           = BootstrapAgentRuntime
	registerBrowserWorkspaceServiceFn = registerBrowserWorkspaceService
	newServiceSchedulerFn             = NewServiceScheduler
)

// Open initializes a complete workspace session: platform checks, store
// opening, service graph construction, agent registration, and background
// indexing. The returned *Workspace is ready for agent construction.
//
// Open is the single composition root for all Relurpify entry points.
// app/relurpish, app/dev-agent-cli, and integration tests all call Open().
func Open(ctx context.Context, cfg WorkspaceConfig) (*Workspace, error) {
	// Resolve workspace YAML overrides before probing or opening stores.
	cfg = resolveWorkspaceConfigOverrides(cfg)

	// Phase A: Configuration Validation
	if err := validateConfig(cfg); err != nil {
		return nil, fmt.Errorf("invalid workspace config: %w", err)
	}

	backend, err := newLLMBackendFn(llm.ProviderConfigFromRuntimeConfig(cfg))
	if err != nil {
		return nil, fmt.Errorf("build inference backend: %w", err)
	}
	// Phase B: Platform Runtime Checks
	results := probeWorkspaceFn(cfg, backend)
	for _, r := range results {
		if r.Required && !r.OK {
			return nil, fmt.Errorf("platform check failed: %s", r.Message)
		}
	}

	// Phase C: Log and Telemetry Setup
	logFile, logger, tel, err := setupTelemetryFn(cfg)
	if err != nil {
		return nil, err
	}

	// Phase D: KnowledgeStore initialization deferred until after BootstrapAgentRuntime
	// where the graphdb.Engine is available from IndexManager.

	// Phase E: Agent Registration + Authorization
	manifestSnapshot, err := loadAgentManifestSnapshotFn(cfg.ManifestPath)
	if err != nil {
		logFile.Close()
		return nil, fmt.Errorf("load manifest snapshot: %w", err)
	}
	registration, err := registerAgentFn(ctx, fauthorization.RuntimeConfig{
		ManifestPath:     cfg.ManifestPath,
		ManifestSnapshot: manifestSnapshot,
		ConfigPath:       cfg.ConfigPath,
		Backend:          cfg.SandboxBackend,
		Sandbox:          cfg.Sandbox,
		AuditLimit:       cfg.AuditLimit,
		BaseFS:           cfg.Workspace,
		HITLTimeout:      cfg.HITLTimeout,
	})
	if err != nil {
		logFile.Close()
		return nil, fmt.Errorf("sandbox registration failed: %w", err)
	}

	// Optional SQLite event log for observability and permission auditing.
	var eventLog *nexusdb.SQLiteEventLog
	if cfg.EventsPath != "" {
		if err := os.MkdirAll(filepath.Dir(cfg.EventsPath), 0o755); err == nil {
			if logStore, err := nexusdb.NewSQLiteEventLog(cfg.EventsPath); err == nil {
				eventLog = logStore
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
							_, _ = eventLog.Append(ctx, "local", []core.FrameworkEvent{{
								Timestamp: time.Now().UTC(),
								Type:      core.FrameworkEventPolicyEvaluated,
								Payload:   data,
								Actor:     identity.EventActor{Kind: "agent", ID: registration.ID, Label: cfg.AgentLabel()},
								Partition: "local",
							}})
						}
					})
				}
				if manifestSnapshot != nil {
					payload := map[string]interface{}{
						"manifest_path": manifestSnapshot.SourcePath,
						"fingerprint":   fmt.Sprintf("%x", manifestSnapshot.Fingerprint),
						"warnings":      append([]string(nil), manifestSnapshot.Warnings...),
					}
					if data, err := json.Marshal(payload); err == nil {
						_, _ = eventLog.Append(ctx, "local", []core.FrameworkEvent{{
							Timestamp: time.Now().UTC(),
							Type:      core.FrameworkEventManifestReloaded,
							Payload:   data,
							Actor:     identity.EventActor{Kind: "agent", ID: registration.ID, Label: cfg.AgentLabel()},
							Partition: "local",
						}})
					}
				}
			} else if logger != nil {
				logger.Printf("warning: failed to init event log: %v", err)
			}
		}
	}

	// Phase F: Capability Bundle + Agent Environment
	// Build CommandRunnerConfig from manifest
	var runnerConfig *contracts.CommandRunnerConfig
	if registration.Manifest != nil {
		runnerConfig = &contracts.CommandRunnerConfig{
			Image:           registration.Manifest.Spec.Image,
			RunAsUser:       registration.Manifest.Spec.Security.RunAsUser,
			ReadOnlyRoot:    registration.Manifest.Spec.Security.ReadOnlyRoot,
			NoNewPrivileges: registration.Manifest.Spec.Security.NoNewPrivileges,
			Workspace:       cfg.Workspace,
		}
	}
	runner, err := newCommandRunnerFn(runnerConfig, registration.Runtime)
	if err != nil {
		logFile.Close()
		return nil, err
	}

	// Resolve model from manifest if not overridden in manifest.
	inferenceModel := cfg.InferenceModel
	if registration.Manifest != nil && registration.Manifest.Spec.Agent != nil {
		if specModel := registration.Manifest.Spec.Agent.Model.Name; specModel != "" && inferenceModel == "" {
			inferenceModel = specModel
		}
	}

	profileRegistry, err := newProfileRegistryFn(manifest.New(cfg.Workspace).ModelProfilesDir())
	if err != nil {
		logFile.Close()
		return nil, fmt.Errorf("load model profiles: %w", err)
	}
	profileResolution := profileRegistry.Resolve(cfg.InferenceProvider, inferenceModel)
	_ = applyProfileFn(backend, profileResolution.Profile)

	logLLM := cfg.DebugLLM
	if registration.Manifest != nil && registration.Manifest.Spec.Agent != nil {
		if registration.Manifest.Spec.Agent.Logging != nil && registration.Manifest.Spec.Agent.Logging.LLM != nil {
			logLLM = *registration.Manifest.Spec.Agent.Logging.LLM
		}
	}
	backend.SetDebugLogging(logLLM)
	model := newInstrumentedModelFn(backend.Model(), llmTelemetryAdapter{inner: tel}, logLLM)
	_ = applyProfileFn(model, profileResolution.Profile)
	// Wire permission event logger if event telemetry is available.
	if et, ok := tel.(interface {
		EmitPermissionEvent(ctx context.Context, desc contracts.PermissionDescriptor, effect, reason string, fields map[string]interface{})
	}); ok {
		if registration.Permissions != nil {
			registration.Permissions.SetEventLogger(func(ctx context.Context, desc contracts.PermissionDescriptor, effect, reason string, fields map[string]interface{}) {
				et.EmitPermissionEvent(ctx, desc, effect, reason, fields)
			})
		}
	}

	// Phase G: Create ServiceManager and Bootstrap
	scheduler := newServiceSchedulerFn()

	boot, err := bootstrapAgentRuntimeFn(cfg.Workspace, AgentBootstrapOptions{
		Context:             ctx,
		AgentID:             registration.ID,
		AgentName:           cfg.AgentName,
		ConfigName:          cfg.AgentLabel(),
		AgentsDir:           cfg.AgentsDir,
		Manifest:            registration.Manifest,
		PermissionManager:   registration.Permissions,
		Runner:              runner,
		Model:               model,
		Backend:             backend,
		InferenceModel:      inferenceModel,
		Telemetry:           tel,
		MaxIterations:       cfg.MaxIterations,
		SkipASTIndex:        cfg.SkipASTIndex,
		AllowedCapabilities: cfg.AllowedCapabilities,
		DebugLLM:            logLLM,
		DebugAgent:          cfg.DebugAgent,
	})
	if err != nil {
		logFile.Close()
		return nil, err
	}

	// Apply policy engine.
	if boot.PolicyEngine != nil {
		registration.Policy = boot.PolicyEngine
		boot.Environment.Registry.SetPolicyEngine(boot.PolicyEngine)
	}

	// Phase H: Embedder Initialization

	// Phase I: ServiceManager Setup & Scheduler Registration
	env := boot.Environment
	sm := NewServiceManager()
	bkcEvents := &knowledge.EventBus{}
	sm.Register("scheduler", scheduler)
	if env.IndexManager != nil {
		sm.Register("bkc.workspace_bootstrap", &WorkspaceBootstrapService{
			IndexManager:  env.IndexManager,
			EventBus:      bkcEvents,
			WorkspaceRoot: cfg.Workspace,
		})
		if env.IndexManager.GraphDB != nil {
			sm.Register("bkc.invalidation", &knowledge.InvalidationPass{
				Store: &knowledge.ChunkStore{Graph: env.IndexManager.GraphDB},
				Staleness: &knowledge.StalenessManager{
					Store:     &knowledge.ChunkStore{Graph: env.IndexManager.GraphDB},
					Propagate: true,
					MaxDepth:  3,
				},
				Events:        bkcEvents,
				WorkspaceRoot: cfg.Workspace,
			})
		}
	}
	sm.Register("bkc.git_watcher", &GitWatcherService{
		WorkspaceRoot: cfg.Workspace,
		EventBus:      bkcEvents,
		Policy:        fauthorization.NewCommandAuthorizationPolicy(registration.Permissions, registration.ID, registration.Manifest.Spec.Agent, "git-watcher"),
	})

	// Register additional services here if needed:
	// sm.Register("custom-worker", &CustomWorker{})

	// Initialize KnowledgeStore now that GraphDB is available
	knowledgeStore, err := openKnowledgeStoreFn(env.IndexManager.GraphDB)
	if err != nil {
		logFile.Close()
		return nil, fmt.Errorf("open knowledge store: %w", err)
	}

	env.Scheduler = scheduler
	env.PermissionManager = registration.Permissions
	env.KnowledgeStore = knowledgeStore
	env.KnowledgeEvents = bkcEvents

	policyBundle, err := contextpolicy.Compile(registration.Manifest, nil, contextpolicy.DefaultContextPolicy())
	if err != nil {
		logFile.Close()
		return nil, fmt.Errorf("compile context policy: %w", err)
	}
	rankerRegistry := retrieval.NewRankerRegistry()
	rankerRegistry.Register(&retrieval.KeywordRanker{K1: 1.2, B: 0.75})
	rankerRegistry.Register(&retrieval.RecencyRanker{HalfLifeHours: 24.0})
	rankerRegistry.Register(&retrieval.ASTProximityRanker{Index: env.IndexManager})
	rankerRegistry.Register(&retrieval.TrustRanker{})
	retriever := retrieval.NewRetriever(rankerRegistry, knowledgeStore).WithPolicy(policyBundle)
	env.Retriever = retriever
	env.Compiler = compiler.NewCompiler(retriever, policyBundle, knowledgeStore)
	env.StreamTrigger = contextstream.NewTrigger(env.Compiler)

	// Attach ServiceManager to environment (for direct access)
	env.ServiceManager = sm

	ws := &Workspace{
		Environment:          env,
		Registration:         registration,
		Backend:              backend,
		ProfileResolution:    profileResolution,
		logFile:              logFile,
		eventLog:             eventLog,
		AgentSpec:            boot.AgentSpec,
		AgentDefinitions:     boot.AgentDefinitions,
		EffectiveContract:    boot.Contract,
		CompiledPolicy:       boot.CompiledPolicy,
		PolicyEngine:         boot.PolicyEngine,
		CapabilityAdmissions: boot.CapabilityAdmissions,
		SkillResults:         boot.SkillResults,
		Telemetry:            tel,
		Logger:               logger,
		ServiceManager:       sm,
	}

	if err := registerBrowserWorkspaceServiceFn(ctx, cfg, registration, env.Registry, sm, tel); err != nil {
		_ = ws.Close()
		return nil, err
	}

	logger.Printf("ayenitd: workspace opened successfully")
	return ws, nil
}

type embedderConfig struct {
	Provider string
	Endpoint string
	Model    string
}

func embedderCfgFromConfig(cfg WorkspaceConfig, model string) embedderConfig {
	provider := strings.TrimSpace(cfg.InferenceProvider)
	endpoint := strings.TrimSpace(cfg.InferenceEndpoint)
	selectedModel := strings.TrimSpace(cfg.InferenceModel)

	if provider == "" {
		provider = "ollama"
	}
	if selectedModel == "" {
		selectedModel = strings.TrimSpace(model)
	}
	return embedderConfig{
		Provider: provider,
		Endpoint: endpoint,
		Model:    selectedModel,
	}
}

type llmTelemetryAdapter struct {
	inner core.Telemetry
}

func (a llmTelemetryAdapter) Emit(event contracts.Event) {
	if a.inner == nil {
		return
	}
	a.inner.Emit(core.Event{
		Type:      core.EventType(event.Type),
		TaskID:    event.TaskID,
		Message:   event.Message,
		Timestamp: event.Timestamp,
		Metadata:  event.Metadata,
	})
}

// resolveWorkspaceConfig loads the workspace YAML (if ConfigPath is
// set) and applies model and agent-name overrides. Errors are silently ignored
// so that a missing or malformed config file does not prevent startup.
func resolveWorkspaceConfigOverrides(cfg WorkspaceConfig) WorkspaceConfig {
	if cfg.ConfigPath == "" {
		return cfg
	}
	type yamlCfg struct {
		Provider     string   `json:"provider" yaml:"provider"`
		Model        string   `json:"model" yaml:"model"`
		Backend      string   `json:"sandbox_backend" yaml:"sandbox_backend"`
		Agent        string   `json:"agent" yaml:"agent"`
		Agents       []string `json:"agents" yaml:"agents"`
		DefaultModel struct {
			Name string `json:"name" yaml:"name"`
		} `json:"default_model" yaml:"default_model"`
	}
	data, err := os.ReadFile(cfg.ConfigPath)
	if err != nil {
		return cfg
	}
	// Try JSON first (YAML is a superset, but we keep it simple here).
	var yc yamlCfg
	if err := yaml.Unmarshal(data, &yc); err == nil {
		if yc.Provider != "" && cfg.InferenceProvider == "" {
			cfg.InferenceProvider = yc.Provider
		}
		if yc.Model != "" && cfg.InferenceModel == "" {
			cfg.InferenceModel = yc.Model
		}
		if yc.Backend != "" && cfg.SandboxBackend == "" {
			cfg.SandboxBackend = yc.Backend
		}
		if yc.DefaultModel.Name != "" && cfg.InferenceModel == "" {
			cfg.InferenceModel = yc.DefaultModel.Name
		}
		if yc.Agent != "" && cfg.AgentName == "" {
			cfg.AgentName = yc.Agent
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
	if cfg.InferenceEndpoint == "" {
		return fmt.Errorf("InferenceEndpoint is required")
	}
	return nil
}

// setupTelemetry opens the log file, creates a logger, and assembles the
// telemetry sink chain (logger + optional JSON file). Returns the log file
// (which must be closed by the caller), the logger, and the assembled telemetry.
func setupTelemetry(cfg WorkspaceConfig) (*os.File, *log.Logger, core.Telemetry, error) {
	logPath := cfg.LogPath
	if logPath == "" {
		paths := manifest.New(cfg.Workspace)
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

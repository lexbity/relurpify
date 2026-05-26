package agentenv

import (
	"context"
	"fmt"
	"time"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	fauthorization "codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/cfgload"
	cfgsecurity "codeburg.org/lexbit/relurpify/framework/cfgload/security"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/event"
	"codeburg.org/lexbit/relurpify/framework/jobs"
	"codeburg.org/lexbit/relurpify/framework/memory"
	"codeburg.org/lexbit/relurpify/framework/prompt"
	"codeburg.org/lexbit/relurpify/framework/sandbox"
	"codeburg.org/lexbit/relurpify/framework/services"
	"codeburg.org/lexbit/relurpify/platform/llm"
)

// WorkspaceConfig provides configuration for workspace environment construction.
// This is extracted from ayenitd.WorkspaceConfig to avoid ayenitd dependency
// when using framework/agentenv as the composition root.
type WorkspaceConfig struct {
	Workspace                  string
	ManifestPath               string
	ConfigPath                 string
	StateDir                   string
	InferenceProvider          string
	InferenceEndpoint          string
	InferenceModel             string
	InferenceNativeToolCalling bool
	AgentName                  string
	AgentsDir                  string
	SandboxBackend             string
	Sandbox                    string
	AuditLimit                 int
	HITLTimeout                time.Duration
	LogPath                    string
	TelemetryPath              string
	EventsPath                 string
	MemoryPath                 string
	SkipASTIndex               bool
	MaxIterations              int
	AllowedCapabilities        []core.CapabilitySelector
	DebugLLM                   bool
	DebugAgent                 bool
	Strict                     bool
	// Agent specification for policy engine and capability registration
	AgentSpec *agentspec.AgentRuntimeSpec
	// Permission manager for authorization
	PermissionManager *fauthorization.PermissionManager
	// Agent ID for permission tracking
	AgentID string
	// Telemetry for instrumentation
	Telemetry core.Telemetry
	// LoadedConfig contains the resolved config tree produced by cfgload.
	LoadedConfig *cfgload.AppConfig
	// ManifestSnapshot contains the selected agent manifest snapshot.
	ManifestSnapshot *cfgload.AgentManifestSnapshot
	// ProfileResolution is the pre-resolved model profile selected by the caller.
	ProfileResolution llm.ProfileResolution
	// AgentDefinitions are the loaded agent definition overlays selected by the caller.
	AgentDefinitions map[string]*agentspec.AgentDefinition
	// SecurityBundle contains the loaded security policy bundle.
	SecurityBundle *cfgsecurity.Bundle
	// EventLogFactory creates an event.Log implementation for the workspace.
	// If nil, no event log is created. This allows apps to inject app-specific
	// event log implementations (e.g., app/nexus/db) without framework dependencies.
	EventLogFactory func(path string) (event.Log, error)
}

// InferenceProviderValue returns the inference provider
func (cfg WorkspaceConfig) InferenceProviderValue() string {
	return cfg.InferenceProvider
}

// InferenceEndpointValue returns the inference endpoint
func (cfg WorkspaceConfig) InferenceEndpointValue() string {
	return cfg.InferenceEndpoint
}

// InferenceModelValue returns the inference model
func (cfg WorkspaceConfig) InferenceModelValue() string {
	return cfg.InferenceModel
}

// InferenceNativeToolCallingValue returns whether native tool calling is enabled
func (cfg WorkspaceConfig) InferenceNativeToolCallingValue() bool {
	return cfg.InferenceNativeToolCalling
}

// BuildWorkspaceEnvironment constructs a complete WorkspaceEnvironment with all framework services.
// This is the composition root for environment construction, owned by framework/agentenv
// rather than ayenitd. Entry points call this function and then wire in ayenitd-specific services.
//
// Construction phases:
// 1. Build capability bundle using framework/services with permission manager and agent spec
// 2. Build prompt registry using framework/services
// 3. Construct WorkspaceEnvironment with all fields populated
// 4. Apply policy engine to capability registry
// 5. Call agent registration functions (if provided)
//
// Returns WorkspaceEnvironment with framework services populated. ayenitd-specific services
// (ServiceManager, Scheduler, KnowledgeStore, Retriever, Compiler) are left nil for entry points to populate.
func BuildWorkspaceEnvironment(ctx context.Context, cfg WorkspaceConfig, securityBundle *cfgsecurity.Bundle, regFuncs AgentRegistrationFuncs) (*WorkspaceEnvironment, error) {
	if cfg.Workspace == "" {
		return nil, fmt.Errorf("workspace required")
	}
	if securityBundle == nil {
		return nil, fmt.Errorf("security bundle required")
	}

	// Phase 1: Build capability bundle with permission manager and agent spec
	runner := sandbox.NewLocalCommandRunner(
		cfg.Workspace,
		cfgsecurity.SubprocessEnvAllowlist(securityBundle.Sandbox),
		nil,
	)

	capabilities, err := services.BuildBuiltinCapabilityBundle(cfg.Workspace, runner, services.CapabilityRegistryOptions{
		Context:           ctx,
		AgentID:           cfg.AgentID,
		PermissionManager: cfg.PermissionManager,
		AgentSpec:         cfg.AgentSpec,
		ProtectedPaths:    append([]string(nil), securityBundle.Sandbox.ProtectedPaths...),
		SkipASTIndex:      cfg.SkipASTIndex,
	})
	if err != nil {
		return nil, fmt.Errorf("build capability bundle: %w", err)
	}

	// Phase 2: Apply telemetry to registry
	if cfg.Telemetry != nil {
		capabilities.Registry.UseTelemetry(cfg.Telemetry)
	}
	if cfg.PermissionManager != nil {
		capabilities.Registry.UsePermissionManager(cfg.AgentID, cfg.PermissionManager)
	}

	// Phase 3: Build prompt registry
	var promptRegistry prompt.Registry
	promptRegistry, err = services.BuildPromptRegistry(cfg.Workspace, cfg.Telemetry)
	if err != nil {
		// Clean up capability bundle on failure
		if capabilities.IndexManager != nil {
			_ = capabilities.IndexManager.Close()
		}
		return nil, fmt.Errorf("build prompt registry: %w", err)
	}

	// Phase 4: Construct WorkspaceEnvironment
	fileScope := sandbox.NewFileScopePolicy(cfg.Workspace, append([]string(nil), securityBundle.Sandbox.ProtectedPaths...))

	// Create working memory store
	wm := memory.NewWorkingMemoryStore()

	// Create default config
	agentCfg := &core.Config{
		Name:              cfg.AgentName,
		Model:             cfg.InferenceModel,
		MaxIterations:     cfg.MaxIterations,
		NativeToolCalling: false,
		AgentSpec:         cfg.AgentSpec,
		DebugLLM:          cfg.DebugLLM,
		DebugAgent:        cfg.DebugAgent,
	}
	if cfg.AgentSpec != nil {
		agentCfg.NativeToolCalling = cfg.AgentSpec.NativeToolCallingEnabled()
	}
	if agentCfg.MaxIterations <= 0 {
		agentCfg.MaxIterations = 8
	}

	env := &WorkspaceEnvironment{
		Config:            agentCfg,
		CommandRunner:     runner,
		JobSubmitter:      jobs.NoopSubmitter{},
		FileScope:         fileScope,
		Registry:          capabilities.Registry,
		IndexManager:      capabilities.IndexManager,
		SearchEngine:      capabilities.SearchEngine,
		WorkingMemory:     wm,
		PromptRegistry:    promptRegistry,
		PermissionManager: cfg.PermissionManager,
		// ayenitd-specific services left nil for entry points to populate
		Model:                         nil,
		CommandPolicy:                 nil,
		KnowledgeStore:                nil,
		Retriever:                     nil,
		Compiler:                      nil,
		EventLog:                      nil,
		Scheduler:                     nil,
		ServiceManager:                nil,
		VerificationPlanner:           nil,
		CompatibilitySurfaceExtractor: nil,
	}

	// Phase 5: Call agent registration functions
	if regFuncs.RegisterCapabilities != nil {
		if err := regFuncs.RegisterCapabilities(*env); err != nil {
			if env.IndexManager != nil {
				_ = env.IndexManager.Close()
			}
			return nil, fmt.Errorf("agent capability registration: %w", err)
		}
	}

	if regFuncs.RegisterPromptProviders != nil {
		if err := regFuncs.RegisterPromptProviders(*env); err != nil {
			if env.IndexManager != nil {
				_ = env.IndexManager.Close()
			}
			return nil, fmt.Errorf("agent prompt provider registration: %w", err)
		}
	}

	return env, nil
}

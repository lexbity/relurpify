package runtime

import (
	"context"
	"fmt"

	"codeburg.org/lexbit/relurpify/agents"
	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/agentlifecycle"
	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/ast"
	fauthorization "codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/graphdb"
	"codeburg.org/lexbit/relurpify/framework/manifest"
	"codeburg.org/lexbit/relurpify/framework/memory"
	fsandbox "codeburg.org/lexbit/relurpify/framework/sandbox"
	"codeburg.org/lexbit/relurpify/framework/search"
	"codeburg.org/lexbit/relurpify/platform/contracts"
	"codeburg.org/lexbit/relurpify/platform/llm"
)

// StartupState captures the first-screen startup decision for the TUI shell.
type StartupState struct {
	Report      DoctorReport
	ActiveAgent string
	ActiveTab   string
	Locked      bool
}

type AgentBootstrapOptions struct {
	Context             context.Context
	AgentID             string
	AgentName           string
	ConfigName          string
	AgentsDir           string
	AgentSpec           *agentspec.AgentRuntimeSpec
	Manifest            *manifest.AgentManifest
	PermissionManager   *fauthorization.PermissionManager
	Runner              fsandbox.CommandRunner
	Model               contracts.LanguageModel
	Backend             llm.ManagedBackend
	InferenceModel      string
	Telemetry           core.Telemetry
	SkipASTIndex        bool
	MaxIterations       int
	AllowedCapabilities []core.CapabilitySelector
	DebugLLM            bool
	DebugAgent          bool
	AgentLifecycle      agentlifecycle.Repository
}

type BootstrappedAgentRuntime struct {
	Registry             *capability.Registry
	IndexManager         *ast.IndexManager
	SearchEngine         *search.SearchEngine
	Memory               *memory.WorkingMemoryStore
	AgentSpec            *agentspec.AgentRuntimeSpec
	AgentConfig          *core.Config
	Backend              llm.ManagedBackend
	Environment          agents.AgentEnvironment
	AgentDefinitions     map[string]*agentspec.AgentDefinition
	SkillResults         []manifest.SkillResolution
	CapabilityAdmissions []capability.AdmissionResult
	Contract             *manifest.EffectiveAgentContract
	CompiledPolicy       *manifest.CompiledPolicyBundle
}

// BootstrapAgentRuntime delegates to agentenv.BootstrapAgentRuntime and then
// registers relurpic and agent capabilities on top. agentenv intentionally omits
// relurpic capabilities because named agents register their own. app/relurpish
// registers them here.
func BootstrapAgentRuntime(workspace string, opts AgentBootstrapOptions) (*BootstrappedAgentRuntime, error) {
	boot, err := agentenv.BootstrapAgentRuntime(workspace, agentenv.AgentBootstrapOptions{
		Context:             opts.Context,
		AgentID:             opts.AgentID,
		AgentName:           opts.AgentName,
		ConfigName:          opts.ConfigName,
		AgentsDir:           opts.AgentsDir,
		AgentSpec:           opts.AgentSpec,
		Manifest:            opts.Manifest,
		PermissionManager:   opts.PermissionManager,
		Runner:              opts.Runner,
		Model:               opts.Model,
		Backend:             opts.Backend,
		InferenceModel:      opts.InferenceModel,
		Telemetry:           opts.Telemetry,
		SkipASTIndex:        opts.SkipASTIndex,
		MaxIterations:       opts.MaxIterations,
		AllowedCapabilities: opts.AllowedCapabilities,
		DebugLLM:            opts.DebugLLM,
		DebugAgent:          opts.DebugAgent,
		AgentLifecycle:      opts.AgentLifecycle,
	})
	if err != nil {
		return nil, err
	}

	profileRegistry, err := llm.NewProfileRegistry(manifest.New(workspace).ModelProfilesDir())
	if err != nil {
		return nil, fmt.Errorf("load model profiles: %w", err)
	}
	provider := ""
	if boot.AgentSpec != nil {
		provider = boot.AgentSpec.Model.Provider
	}
	modelName := opts.InferenceModel
	if modelName == "" && boot.AgentConfig != nil {
		modelName = boot.AgentConfig.Model
	}
	profileResolution := profileRegistry.Resolve(provider, modelName)
	_ = llm.ApplyProfile(boot.Backend, profileResolution.Profile)
	_ = llm.ApplyProfile(boot.Environment.Model, profileResolution.Profile)

	env := agents.AgentEnvironment{
		Config:       boot.Environment.Config,
		Model:        boot.Environment.Model,
		Registry:     boot.Environment.Registry,
		IndexManager: boot.Environment.IndexManager,
		SearchEngine: boot.Environment.SearchEngine,
		Memory:       boot.Environment.WorkingMemory,
	}

	return &BootstrappedAgentRuntime{
		Registry:             boot.Registry,
		IndexManager:         boot.IndexManager,
		SearchEngine:         boot.SearchEngine,
		Memory:               boot.Environment.WorkingMemory,
		AgentSpec:            boot.AgentSpec,
		AgentConfig:          boot.AgentConfig,
		Backend:              boot.Backend,
		Environment:          env,
		AgentDefinitions:     boot.AgentDefinitions,
		SkillResults:         boot.SkillResults,
		CapabilityAdmissions: boot.CapabilityAdmissions,
		Contract:             boot.Contract,
		CompiledPolicy:       boot.CompiledPolicy,
	}, nil
}

// ReloadRuntimeForWorkspace rebuilds the runtime against a new workspace root
// and closes the previous runtime only after the replacement succeeds.
func ReloadRuntimeForWorkspace(ctx context.Context, current *Runtime, workspace string) (*Runtime, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if current == nil {
		return New(ctx, ConfigForWorkspace(Config{}, workspace))
	}
	cfg := ConfigForWorkspace(current.Config, workspace)
	newRT, err := New(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := current.Close(); err != nil {
		_ = newRT.Close()
		return nil, err
	}
	return newRT, nil
}

// ConfigForWorkspace derives a normalized runtime config for a new workspace
// while preserving the caller's runtime selections.
func ConfigForWorkspace(current Config, workspace string) Config {
	cfg := current
	cfg.Workspace = workspace
	paths := manifest.New(workspace)
	cfg.ManifestPath = paths.ManifestFile()
	cfg.AgentsDir = paths.AgentsDir()
	cfg.MemoryPath = paths.MemoryDir()
	cfg.LogPath = paths.LogFile("relurpish.log")
	cfg.TelemetryPath = paths.TelemetryFile("")
	cfg.EventsPath = paths.EventsFile()
	cfg.ConfigPath = paths.ConfigFile()
	return cfg
}

// BootstrapStartupState prepares the workspace for the TUI startup flow.
//
// If the workspace has not been initialized yet, starter templates are copied in
// place before the runtime is created. The returned report reflects the final
// post-bootstrap state used to decide whether the shell should start locked in
// Doctor mode or auto-promote to Euclo chat.
func BootstrapStartupState(ctx context.Context, cfg Config) (StartupState, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	state := StartupState{
		ActiveAgent: "euclo",
		ActiveTab:   "chat",
	}
	report := BuildDoctorReport(ctx, cfg)
	if report.NeedsInitialization() {
		if err := InitializeWorkspaceFromTemplates(cfg, false); err != nil {
			state.Report = report
			state.Locked = true
			state.ActiveAgent = "none"
			state.ActiveTab = "doctor"
			return state, err
		}
		report = BuildDoctorReport(ctx, cfg)
	}
	state.Report = report
	if report.HasBlockingIssues() {
		state.Locked = true
		state.ActiveAgent = "none"
		state.ActiveTab = "doctor"
	}
	return state, nil
}

func graphDBFromIndexManager(indexManager *ast.IndexManager) *graphdb.Engine {
	if indexManager == nil {
		return nil
	}
	return indexManager.GraphDB
}

func selectedAgentDefinitionOverlays(agentName string, defs map[string]*agentspec.AgentDefinition) []agentspec.AgentSpecOverlay {
	if defs == nil {
		return nil
	}
	def, ok := defs[agentName]
	if !ok || def == nil {
		return nil
	}
	return []agentspec.AgentSpecOverlay{agentspec.AgentSpecOverlayFromSpec(&def.Spec)}
}

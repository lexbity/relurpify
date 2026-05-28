package runtime

import (
	"context"
	"path/filepath"

	"codeburg.org/lexbit/relurpify/agents"
	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/agentlifecycle"
	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/ast"
	fauthorization "codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/cfgload"
	cfgsecurity "codeburg.org/lexbit/relurpify/framework/cfgload/security"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/graphdb"
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
	AgentSpec           *agentspec.AgentRuntimeSpec
	ManifestSnapshot    *cfgload.AgentManifestSnapshot
	SecurityBundle      *cfgsecurity.Bundle
	ProfileResolution   llm.ProfileResolution
	AgentDefinitions    map[string]*agentspec.AgentDefinition
	PermissionManager   *fauthorization.PermissionManager
	Runner              fsandbox.CommandRunner
	Model               contracts.LanguageModel
	Backend             llm.ManagedBackend
	InferenceModel      string
	Telemetry           core.Telemetry
	SkipASTIndex        bool
	MaxIterations       int
	AllowedCapabilities []agentspec.CapabilitySelector
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
	SkillResults         []cfgload.SkillResolution
	CapabilityAdmissions []capability.AdmissionResult
	Contract             *cfgload.EffectiveAgentContract
	CompiledPolicy       *fauthorization.CompiledPolicyBundle
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
		AgentSpec:           opts.AgentSpec,
		ManifestSnapshot:    opts.ManifestSnapshot,
		SecurityBundle:      opts.SecurityBundle,
		ProfileResolution:   opts.ProfileResolution,
		AgentDefinitions:    opts.AgentDefinitions,
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
	_ = llm.ApplyProfile(boot.Backend, opts.ProfileResolution.Profile)
	_ = llm.ApplyProfile(boot.Environment.Model, opts.ProfileResolution.Profile)

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
		return New(ctx, ConfigForWorkspace(Config{}, workspace), cfgload.Secrets{})
	}
	cfg := ConfigForWorkspace(current.Config, workspace)
	newRT, err := New(ctx, cfg, current.secrets)
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
	paths := cfgload.New(workspace)
	agentName := cfg.AgentName
	if agentName == "" {
		agentName = "coding"
	}
	cfg.ManifestPath = filepath.Join(paths.AgentsDir(), agentName+".yaml")
	cfg.AgentsDir = paths.AgentsDir()
	cfg.MemoryPath = cfgload.DefaultWorkspaceStateMemoryDir(workspace)
	cfg.LogPath = filepath.Join(cfgload.DefaultWorkspaceStateLogsDir(workspace), "relurpish.log")
	cfg.TelemetryPath = filepath.Join(cfgload.DefaultWorkspaceStateTelemetryDir(workspace), "telemetry.jsonl")
	cfg.EventsPath = cfgload.DefaultWorkspaceStateEventsFile(workspace)
	cfg.ConfigPath = cfgload.DefaultWorkspaceStateConfigPath(workspace)
	return cfg
}

// BootstrapStartupState prepares the workspace for the TUI startup flow.
//
// If the workspace has not been initialized yet, starter templates are copied in
// place before the runtime is created. The returned report reflects the final
// post-bootstrap state used to decide whether the shell should start locked in
// Doctor mode or auto-promote to Euclo chat.
func BootstrapStartupState(ctx context.Context, cfg Config, secrets cfgload.Secrets) (StartupState, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	state := StartupState{
		ActiveAgent: "euclo",
		ActiveTab:   "chat",
	}
	report := BuildDoctorReport(ctx, cfg, secrets)
	if report.NeedsInitialization() {
		if err := InitializeWorkspaceFromTemplates(cfg, false); err != nil {
			state.Report = report
			state.Locked = true
			state.ActiveAgent = "none"
			state.ActiveTab = "doctor"
			return state, err
		}
		report = BuildDoctorReport(ctx, cfg, secrets)
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

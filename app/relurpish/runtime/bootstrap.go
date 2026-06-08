package runtime

import (
	"context"
	"path/filepath"

	"codeburg.org/lexbit/relurpify/agents"
	"codeburg.org/lexbit/relurpify/capability"
	"codeburg.org/lexbit/relurpify/capability/agentspec"
	fsandbox "codeburg.org/lexbit/relurpify/capability/sandbox"
	"codeburg.org/lexbit/relurpify/context/knowledge/ast"
	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
	"codeburg.org/lexbit/relurpify/context/knowledge/memory"
	"codeburg.org/lexbit/relurpify/context/knowledge/search"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentenv"
	"codeburg.org/lexbit/relurpify/execution/agentlifecycle"
	fauthorization "codeburg.org/lexbit/relurpify/governance/authorization"
	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/platform/llm"
	"codeburg.org/lexbit/relurpify/telemetry"
	"codeburg.org/lexbit/relurpify/userconfig/config"
	cfgsecurity "codeburg.org/lexbit/relurpify/userconfig/config/security"
	"codeburg.org/lexbit/relurpify/userconfig/modelselect"
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
	ManifestSnapshot    *config.AgentManifestSnapshot
	SecurityBundle      *cfgsecurity.Bundle
	ProfileResolution   modelselect.ProfileResolution
	PermissionManager   *fauthorization.PermissionManager
	Runner              fsandbox.CommandRunner
	Model               model.LanguageModel
	Backend             llm.ManagedBackend
	InferenceModel      string
	Telemetry           telemetry.Telemetry
	SkipASTIndex        bool
	MaxIterations       int
	AllowedCapabilities []agentspec.CapabilitySelector
	DebugLLM            bool
	DebugAgent          bool
	AgentLifecycle      agentlifecycle.Repository
}

type BootstrappedAgentRuntime struct {
	Registry             *capability.CapabilityRegistry
	IndexManager         *ast.IndexManager
	SearchEngine         *search.SearchEngine
	Memory               *memory.WorkingMemoryStore
	AgentSpec            *agentspec.AgentRuntimeSpec
	AgentConfig          *execution.Config
	Backend              llm.ManagedBackend
	Environment          agents.AgentEnvironment
	CapabilityAdmissions []capability.AdmissionResult
	Contract             *config.EffectiveAgentContract
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
		return New(ctx, ConfigForWorkspace(Config{}, workspace), config.Secrets{})
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
	paths := config.New(workspace)
	agentName := cfg.AgentName
	if agentName == "" {
		agentName = "coding"
	}
	cfg.ManifestPath = filepath.Join(paths.AgentsDir(), agentName+".yaml")
	cfg.AgentsDir = paths.AgentsDir()
	cfg.MemoryPath = config.DefaultWorkspaceStateMemoryDir(workspace)
	cfg.LogPath = filepath.Join(config.DefaultWorkspaceStateLogsDir(workspace), "relurpish.log")
	cfg.TelemetryPath = filepath.Join(config.DefaultWorkspaceStateTelemetryDir(workspace), "telemetry.jsonl")
	cfg.EventsPath = config.DefaultWorkspaceStateEventsFile(workspace)
	cfg.ConfigPath = config.DefaultWorkspaceStateConfigPath(workspace)
	return cfg
}

// BootstrapStartupState prepares the workspace for the TUI startup flow.
//
// If the workspace has not been initialized yet, starter templates are copied in
// place before the runtime is created. The returned report reflects the final
// post-bootstrap state used to decide whether the shell should start locked in
// Doctor mode or auto-promote to Euclo chat.
func BootstrapStartupState(ctx context.Context, cfg Config, secrets config.Secrets) (StartupState, error) {
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

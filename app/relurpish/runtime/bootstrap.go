package runtime

import (
	"context"
	"database/sql"
	"fmt"

	"codeburg.org/lexbit/relurpify/agents"
	"codeburg.org/lexbit/relurpify/ayenitd"
	"codeburg.org/lexbit/relurpify/framework/ast"
	fauthorization "codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/capabilityplan"
	"codeburg.org/lexbit/relurpify/framework/config"
	contractpkg "codeburg.org/lexbit/relurpify/framework/contract"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/graphdb"
	"codeburg.org/lexbit/relurpify/framework/guidance"
	"codeburg.org/lexbit/relurpify/framework/manifest"
	"codeburg.org/lexbit/relurpify/framework/memory"
	"codeburg.org/lexbit/relurpify/framework/patterns"
	frameworkplan "codeburg.org/lexbit/relurpify/framework/plan"
	"codeburg.org/lexbit/relurpify/framework/policybundle"
	fsandbox "codeburg.org/lexbit/relurpify/framework/sandbox"
	"codeburg.org/lexbit/relurpify/framework/search"
	frameworkskills "codeburg.org/lexbit/relurpify/framework/skills"
	"codeburg.org/lexbit/relurpify/platform/llm"
)

type AgentBootstrapOptions struct {
	Context             context.Context
	AgentID             string
	AgentName           string
	ConfigName          string
	AgentsDir           string
	AgentSpec           *core.AgentRuntimeSpec
	Manifest            *manifest.AgentManifest
	PermissionManager   *fauthorization.PermissionManager
	Runner              fsandbox.CommandRunner
	Model               core.LanguageModel
	Backend             llm.ManagedBackend
	InferenceModel      string
	Memory              memory.MemoryStore
	Telemetry           core.Telemetry
	SkipASTIndex        bool
	MaxIterations       int
	AllowedCapabilities []core.CapabilitySelector
	DebugLLM            bool
	DebugAgent          bool
	PatternStore        patterns.PatternStore
	CommentStore        patterns.CommentStore
	RetrievalDB         *sql.DB
	PlanStore           frameworkplan.PlanStore
	GuidanceBroker      *guidance.GuidanceBroker
	WorkflowStore       memory.WorkflowStateStore
}

type BootstrappedAgentRuntime struct {
	Registry             *capability.Registry
	IndexManager         *ast.IndexManager
	SearchEngine         *search.SearchEngine
	Memory               memory.MemoryStore
	AgentSpec            *core.AgentRuntimeSpec
	AgentConfig          *core.Config
	Backend              llm.ManagedBackend
	Environment          agents.AgentEnvironment
	AgentDefinitions     map[string]*core.AgentDefinition
	SkillResults         []frameworkskills.SkillResolution
	CapabilityAdmissions []capabilityplan.AdmissionResult
	Contract             *contractpkg.EffectiveAgentContract
	CompiledPolicy       *policybundle.CompiledPolicyBundle
}

// BootstrapAgentRuntime delegates to ayenitd.BootstrapAgentRuntime and then
// registers relurpic and agent capabilities on top. ayenitd intentionally omits
// relurpic capabilities because named agents register their own. app/relurpish
// uses this bootstrap path directly (not through named agents), so we add them
// here.
func BootstrapAgentRuntime(workspace string, opts AgentBootstrapOptions) (*BootstrappedAgentRuntime, error) {
	boot, err := ayenitd.BootstrapAgentRuntime(workspace, ayenitd.AgentBootstrapOptions{
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
		Memory:              opts.Memory,
		Telemetry:           opts.Telemetry,
		SkipASTIndex:        opts.SkipASTIndex,
		MaxIterations:       opts.MaxIterations,
		AllowedCapabilities: opts.AllowedCapabilities,
		DebugLLM:            opts.DebugLLM,
		DebugAgent:          opts.DebugAgent,
		PatternStore:        opts.PatternStore,
		CommentStore:        opts.CommentStore,
		RetrievalDB:         opts.RetrievalDB,
		PlanStore:           opts.PlanStore,
		GuidanceBroker:      opts.GuidanceBroker,
		WorkflowStore:       opts.WorkflowStore,
	})
	if err != nil {
		return nil, err
	}

	profileRegistry, err := llm.NewProfileRegistry(config.New(workspace).ModelProfilesDir())
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

	if err := agents.RegisterBuiltinRelurpicCapabilitiesWithOptions(
		boot.Registry,
		opts.Model,
		boot.AgentConfig,
		agents.WithIndexManager(boot.IndexManager),
		agents.WithGraphDB(graphDBFromIndexManager(boot.IndexManager)),
		agents.WithPatternStore(opts.PatternStore),
		agents.WithCommentStore(opts.CommentStore),
		agents.WithRetrievalDB(opts.RetrievalDB),
		agents.WithPlanStore(opts.PlanStore),
		agents.WithGuidanceBroker(opts.GuidanceBroker),
		agents.WithWorkflowStore(opts.WorkflowStore),
	); err != nil {
		return nil, fmt.Errorf("register relurpic capabilities: %w", err)
	}

	env := agents.AgentEnvironment{
		Config:        boot.Environment.Config,
		Model:         boot.Environment.Model,
		CommandPolicy: boot.Environment.CommandPolicy,
		Registry:      boot.Environment.Registry,
		IndexManager:  boot.Environment.IndexManager,
		SearchEngine:  boot.Environment.SearchEngine,
		Memory:        boot.Environment.Memory,
		// New: thread stores from workspace environment
		WorkflowStore: boot.Environment.WorkflowStore,
		PlanStore:     boot.Environment.PlanStore,
	}
	if err := agents.RegisterAgentCapabilities(boot.Registry, env); err != nil {
		return nil, fmt.Errorf("register agent capabilities: %w", err)
	}

	return &BootstrappedAgentRuntime{
		Registry:             boot.Registry,
		IndexManager:         boot.IndexManager,
		SearchEngine:         boot.SearchEngine,
		Memory:               boot.Memory,
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

func graphDBFromIndexManager(indexManager *ast.IndexManager) *graphdb.Engine {
	if indexManager == nil {
		return nil
	}
	return indexManager.GraphDB
}

func selectedAgentDefinitionOverlays(agentName string, defs map[string]*core.AgentDefinition) []core.AgentSpecOverlay {
	if defs == nil {
		return nil
	}
	def, ok := defs[agentName]
	if !ok || def == nil {
		return nil
	}
	return []core.AgentSpecOverlay{core.AgentSpecOverlayFromSpec(&def.Spec)}
}

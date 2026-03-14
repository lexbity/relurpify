package runtime

import (
	"context"
	"fmt"
	"os"

	"github.com/lexcodex/relurpify/agents"
	"github.com/lexcodex/relurpify/framework/ast"
	fauthorization "github.com/lexcodex/relurpify/framework/authorization"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/capabilityplan"
	contractpkg "github.com/lexcodex/relurpify/framework/contract"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/manifest"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/policybundle"
	fsandbox "github.com/lexcodex/relurpify/framework/sandbox"
	"github.com/lexcodex/relurpify/framework/search"
	frameworkskills "github.com/lexcodex/relurpify/framework/skills"
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
	Memory              memory.MemoryStore
	Telemetry           core.Telemetry
	OllamaEndpoint      string
	OllamaModel         string
	SkipASTIndex        bool
	MaxIterations       int
	AllowedCapabilities []core.CapabilitySelector
	DebugLLM            bool
	DebugAgent          bool
}

type BootstrappedAgentRuntime struct {
	Registry             *capability.Registry
	IndexManager         *ast.IndexManager
	SearchEngine         *search.SearchEngine
	Memory               memory.MemoryStore
	AgentSpec            *core.AgentRuntimeSpec
	AgentConfig          *core.Config
	Environment          agents.AgentEnvironment
	AgentDefinitions     map[string]*core.AgentDefinition
	SkillResults         []frameworkskills.SkillResolution
	CapabilityAdmissions []capabilityplan.AdmissionResult
	Contract             *contractpkg.EffectiveAgentContract
	CompiledPolicy       *policybundle.CompiledPolicyBundle
}

func BootstrapAgentRuntime(workspace string, opts AgentBootstrapOptions) (*BootstrappedAgentRuntime, error) {
	if workspace == "" {
		return nil, fmt.Errorf("workspace required")
	}
	if opts.Manifest == nil {
		return nil, fmt.Errorf("agent manifest required")
	}
	if opts.Manifest.Spec.Agent == nil && opts.AgentSpec == nil {
		return nil, fmt.Errorf("agent manifest missing spec.agent configuration")
	}
	if opts.Runner == nil {
		return nil, fmt.Errorf("command runner required")
	}

	var agentDefs map[string]*core.AgentDefinition
	var err error
	if opts.AgentsDir != "" {
		agentDefs, err = LoadAgentDefinitions(opts.AgentsDir)
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("load agent definitions: %w", err)
		}
	}

	manifestForResolution := opts.Manifest
	if opts.AgentSpec != nil {
		clone := *opts.Manifest
		clone.Spec.Agent = opts.AgentSpec
		manifestForResolution = &clone
	}
	resolveOpts := contractpkg.ResolveOptions{
		AgentOverlays: selectedAgentDefinitionOverlays(opts.AgentName, agentDefs),
	}
	effectiveContract, err := contractpkg.ResolveEffectiveAgentContract(workspace, manifestForResolution, resolveOpts)
	if err != nil {
		return nil, err
	}
	agentSpec := effectiveContract.AgentSpec
	skillResults := append([]frameworkskills.SkillResolution{}, effectiveContract.SkillResults...)
	resolvedSkills := append([]frameworkskills.ResolvedSkill{}, effectiveContract.ResolvedSkills...)

	resolvedModel := opts.OllamaModel
	if resolvedModel == "" {
		resolvedModel = agentSpec.Model.Name
	}

	capabilities, err := BuildBuiltinCapabilityBundle(workspace, opts.Runner, CapabilityRegistryOptions{
		Context:           opts.Context,
		AgentID:           opts.AgentID,
		PermissionManager: opts.PermissionManager,
		AgentSpec:         agentSpec,
		OllamaEndpoint:    opts.OllamaEndpoint,
		OllamaModel:       resolvedModel,
		SkipASTIndex:      opts.SkipASTIndex,
	})
	if err != nil {
		return nil, err
	}
	registry := capabilities.Registry
	indexManager := capabilities.IndexManager
	searchEngine := capabilities.SearchEngine
	if opts.Telemetry != nil {
		registry.UseTelemetry(opts.Telemetry)
	}
	if opts.PermissionManager != nil {
		registry.UsePermissionManager(opts.AgentID, opts.PermissionManager)
	}
	compiledPolicy, err := policybundle.BuildFromSpec(effectiveContract.AgentID, effectiveContract.AgentSpec, opts.PermissionManager)
	if err != nil {
		return nil, fmt.Errorf("compile effective policy: %w", err)
	}
	registry.SetPolicyEngine(compiledPolicy.Engine)

	maxIterations := opts.MaxIterations
	if maxIterations <= 0 {
		maxIterations = 8
	}
	configName := opts.ConfigName
	if configName == "" {
		configName = opts.Manifest.Metadata.Name
	}
	agentCfg := &core.Config{
		Name:              configName,
		Model:             resolvedModel,
		OllamaEndpoint:    opts.OllamaEndpoint,
		MaxIterations:     maxIterations,
		OllamaToolCalling: agentSpec.ToolCallingEnabled(),
		AgentSpec:         agentSpec,
		DebugLLM:          opts.DebugLLM,
		DebugAgent:        opts.DebugAgent,
		Telemetry:         opts.Telemetry,
	}
	registry.UseAgentSpec(opts.AgentID, agentSpec)
	admissionResults, err := capabilityplan.AdmitCandidates(
		registry,
		toCapabilityPlanCandidates(frameworkskills.EnumerateSkillCapabilities(resolvedSkills)),
		core.EffectiveAllowedCapabilitySelectors(agentSpec),
	)
	if err != nil {
		return nil, fmt.Errorf("admit skill capabilities: %w", err)
	}
	if err := agents.RegisterBuiltinRelurpicCapabilities(registry, opts.Model, agentCfg); err != nil {
		return nil, fmt.Errorf("register relurpic capabilities: %w", err)
	}
	env := agents.AgentEnvironment{
		Model:        opts.Model,
		Registry:     registry,
		IndexManager: indexManager,
		SearchEngine: searchEngine,
		Memory:       opts.Memory,
		Config:       agentCfg,
	}
	if err := agents.RegisterAgentCapabilities(registry, env); err != nil {
		return nil, fmt.Errorf("register agent capabilities: %w", err)
	}
	if len(opts.AllowedCapabilities) > 0 {
		registry.RestrictToCapabilities(opts.AllowedCapabilities)
	}

	return &BootstrappedAgentRuntime{
		Registry:             registry,
		IndexManager:         indexManager,
		SearchEngine:         searchEngine,
		Memory:               opts.Memory,
		AgentSpec:            agentSpec,
		AgentConfig:          agentCfg,
		Environment:          env,
		AgentDefinitions:     agentDefs,
		SkillResults:         skillResults,
		CapabilityAdmissions: admissionResults,
		Contract:             effectiveContract,
		CompiledPolicy:       compiledPolicy,
	}, nil
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

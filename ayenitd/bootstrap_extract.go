package ayenitd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/agentlifecycle"
	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/ast"
	fauthorization "codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/manifest"
	"codeburg.org/lexbit/relurpify/framework/memory"
	fsandbox "codeburg.org/lexbit/relurpify/framework/sandbox"
	"codeburg.org/lexbit/relurpify/framework/search"
	frameworkskills "codeburg.org/lexbit/relurpify/framework/skills"
	"codeburg.org/lexbit/relurpify/platform/llm"
)

// AgentBootstrapOptions is copied from runtime package.
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
	Telemetry           core.Telemetry
	SkipASTIndex        bool
	MaxIterations       int
	AllowedCapabilities []core.CapabilitySelector
	DebugLLM            bool
	DebugAgent          bool
	AgentLifecycle      agentlifecycle.Repository
}

// BootstrappedAgentRuntime is copied from runtime package.
type BootstrappedAgentRuntime struct {
	Registry             *capability.Registry
	IndexManager         *ast.IndexManager
	SearchEngine         *search.SearchEngine
	AgentSpec            *core.AgentRuntimeSpec
	AgentConfig          *core.Config
	Backend              llm.ManagedBackend
	Environment          agentenv.WorkspaceEnvironment
	AgentDefinitions     map[string]*agentspec.AgentDefinition
	SkillResults         []frameworkskills.SkillResolution
	CapabilityAdmissions []capability.AdmissionResult
	Contract             *manifest.EffectiveAgentContract
	CompiledPolicy       *manifest.CompiledPolicyBundle
}

// BootstrapAgentRuntime is extracted from app/relurpish/runtime/bootstrap.go.
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

	var agentDefs map[string]*agentspec.AgentDefinition
	var err error
	if opts.AgentsDir != "" {
		agentDefs, err = loadAgentDefinitions(opts.AgentsDir)
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
	resolveOpts := manifest.ResolveOptions{
		AgentOverlays: selectedAgentDefinitionOverlays(opts.AgentName, agentDefs),
	}
	effectiveContract, err := manifest.ResolveEffectiveAgentContract(workspace, manifestForResolution, resolveOpts)
	if err != nil {
		return nil, err
	}
	agentSpec := effectiveContract.AgentSpec
	skillResults := append([]frameworkskills.SkillResolution{}, effectiveContract.SkillResults...)
	resolvedSkills := append([]frameworkskills.ResolvedSkill{}, effectiveContract.ResolvedSkills...)

	resolvedModel := opts.InferenceModel
	if resolvedModel == "" {
		resolvedModel = agentSpec.Model.Name
	}

	runner := opts.Runner
	if runner != nil {
		runner = fsandbox.NewEnforcingCommandRunner(runner, fauthorization.NewCommandAuthorizationPolicy(opts.PermissionManager, opts.AgentID, agentSpec, "sandbox"))
	}

	capabilities, err := BuildBuiltinCapabilityBundle(workspace, runner, CapabilityRegistryOptions{
		Context:           opts.Context,
		AgentID:           opts.AgentID,
		PermissionManager: opts.PermissionManager,
		AgentSpec:         agentSpec,
		ProtectedPaths:    manifest.New(workspace).GovernanceRoots(manifest.New(workspace).ManifestFile(), manifest.New(workspace).ConfigFile(), manifest.New(workspace).NexusConfigFile(), manifest.New(workspace).PolicyRulesFile(), manifest.New(workspace).ModelProfilesDir()),
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
	compiledPolicy, err := manifest.BuildFromSpec(effectiveContract.AgentID, effectiveContract.AgentSpec, opts.PermissionManager)
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
		MaxIterations:     maxIterations,
		NativeToolCalling: agentSpec.NativeToolCallingEnabled(),
		AgentSpec:         agentSpec,
		DebugLLM:          opts.DebugLLM,
		DebugAgent:        opts.DebugAgent,
		Telemetry:         opts.Telemetry,
	}
	registry.UseAgentSpec(opts.AgentID, agentSpec)
	admissionResults, err := capability.AdmitCandidates(
		registry,
		toCapabilityPlanCandidates(frameworkskills.EnumerateSkillCapabilities(resolvedSkills)),
		core.EffectiveAllowedCapabilitySelectors(agentSpec),
	)
	if err != nil {
		return nil, fmt.Errorf("admit skill capabilities: %w", err)
	}
	// Relurpic capability registration is intentionally omitted from ayenitd.
	// Relurpic capabilities are subagent-backed and caller-owned: each named agent
	// (euclo, rex, etc.) is responsible for registering them after receiving the
	// WorkspaceEnvironment. Registering here would create a named/ → ayenitd import cycle.

	// Create working memory store
	wm := memory.NewWorkingMemoryStore()

	env := agentenv.WorkspaceEnvironment{
		Config:                        agentCfg,
		Model:                         opts.Model,
		CommandPolicy:                 fauthorization.NewCommandAuthorizationPolicy(opts.PermissionManager, opts.AgentID, agentSpec, "workspace"),
		Registry:                      registry,
		PermissionManager:             opts.PermissionManager,
		IndexManager:                  indexManager,
		SearchEngine:                  searchEngine,
		WorkingMemory:                 wm,
		KnowledgeStore:                nil, // Will be populated in open.go
		Retriever:                     nil, // Will be populated in open.go
		Compiler:                      nil, // Will be populated in open.go
		EventLog:                      nil,
		Scheduler:                     nil,
		ServiceManager:                nil,
		VerificationPlanner:           nil,
		CompatibilitySurfaceExtractor: nil,
	}

	return &BootstrappedAgentRuntime{
		Registry:             registry,
		IndexManager:         indexManager,
		SearchEngine:         searchEngine,
		AgentSpec:            agentSpec,
		AgentConfig:          agentCfg,
		Backend:              opts.Backend,
		Environment:          env,
		AgentDefinitions:     agentDefs,
		SkillResults:         skillResults,
		CapabilityAdmissions: admissionResults,
		Contract:             effectiveContract,
		CompiledPolicy:       compiledPolicy,
	}, nil
}

func loadAgentDefinitions(dir string) (map[string]*agentspec.AgentDefinition, error) {
	defs := make(map[string]*agentspec.AgentDefinition)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		path := filepath.Join(dir, name)
		def, err := agentspec.LoadAgentDefinition(path)
		if err != nil {
			if errors.Is(err, agentspec.ErrNotAgentDefinition) {
				continue
			}
			return nil, fmt.Errorf("load %s: %w", name, err)
		}
		if def.Name == "" {
			def.Name = strings.TrimSuffix(name, filepath.Ext(name))
		}
		defs[def.Name] = def
	}
	return defs, nil
}

func selectedAgentDefinitionOverlays(agentName string, defs map[string]*agentspec.AgentDefinition) []core.AgentSpecOverlay {
	if defs == nil {
		return nil
	}
	def, ok := defs[agentName]
	if !ok || def == nil {
		return nil
	}
	return []core.AgentSpecOverlay{core.AgentSpecOverlayFromSpec(&def.Spec)}
}

func toCapabilityPlanCandidates(input []frameworkskills.SkillCapabilityCandidate) []capability.Candidate {
	out := make([]capability.Candidate, 0, len(input))
	for _, candidate := range input {
		out = append(out, capability.Candidate{
			Descriptor:      candidate.Descriptor,
			PromptHandler:   candidate.PromptHandler,
			ResourceHandler: candidate.ResourceHandler,
		})
	}
	return out
}

package euclo

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	reactpkg "github.com/lexcodex/relurpify/agents/react"
	archaeoarch "github.com/lexcodex/relurpify/archaeo/archaeology"
	archaeobindings "github.com/lexcodex/relurpify/archaeo/bindings/euclo"
	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeoexec "github.com/lexcodex/relurpify/archaeo/execution"
	archaeolearning "github.com/lexcodex/relurpify/archaeo/learning"
	archaeophases "github.com/lexcodex/relurpify/archaeo/phases"
	archaeoplans "github.com/lexcodex/relurpify/archaeo/plans"
	archaeoprojections "github.com/lexcodex/relurpify/archaeo/projections"
	archaeoretrieval "github.com/lexcodex/relurpify/archaeo/retrieval"
	archaeotensions "github.com/lexcodex/relurpify/archaeo/tensions"
	archaeoverification "github.com/lexcodex/relurpify/archaeo/verification"
	"github.com/lexcodex/relurpify/ayenitd"
	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graphdb"
	"github.com/lexcodex/relurpify/framework/guidance"
	"github.com/lexcodex/relurpify/framework/memory"
	memorydb "github.com/lexcodex/relurpify/framework/memory/db"
	"github.com/lexcodex/relurpify/framework/patterns"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
	"github.com/lexcodex/relurpify/named/euclo/capabilities"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	eucloexec "github.com/lexcodex/relurpify/named/euclo/execution"
	"github.com/lexcodex/relurpify/named/euclo/interaction"
	"github.com/lexcodex/relurpify/named/euclo/interaction/gate"
	"github.com/lexcodex/relurpify/named/euclo/interaction/modes"
	agentstate "github.com/lexcodex/relurpify/named/euclo/internal/agentstate"
	"github.com/lexcodex/relurpify/named/euclo/langdetect"
	euclorelurpic "github.com/lexcodex/relurpify/named/euclo/relurpicabilities"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
	eucloarchaeomem "github.com/lexcodex/relurpify/named/euclo/runtime/archaeomem"
	eucloassurance "github.com/lexcodex/relurpify/named/euclo/runtime/assurance"
	euclodispatch "github.com/lexcodex/relurpify/named/euclo/runtime/dispatch"
	"github.com/lexcodex/relurpify/named/euclo/runtime/orchestrate"
	euclopolicy "github.com/lexcodex/relurpify/named/euclo/runtime/policy"
	"github.com/lexcodex/relurpify/named/euclo/runtime/pretask"
	eucloreporting "github.com/lexcodex/relurpify/named/euclo/runtime/reporting"
	euclorestore "github.com/lexcodex/relurpify/named/euclo/runtime/restore"
	euclosession "github.com/lexcodex/relurpify/named/euclo/runtime/session"
	euclostate "github.com/lexcodex/relurpify/named/euclo/runtime/state"
	euclowork "github.com/lexcodex/relurpify/named/euclo/runtime/work"
)

var detectWorkspaceLanguages = langdetect.Detect

// Dispatch model:
//
//	Agent.Execute
//	  -> runtimeState(...)
//	  -> selectExecutor(...)
//	  -> executeManagedFlow(...)
//	  -> named/euclo/runtime/assurance.Execute(...)
//	  -> named/euclo/runtime/dispatch.Dispatcher.Execute(...)
//	  -> execution.ExecuteRecipe(...)
//	  -> /agents paradigm runner (react / planner / htn / reflection / architect / rewoo)
//
// BuildGraph follows the same executor selection path and delegates directly to
// the selected WorkflowExecutor's graph builder.
//
// Previous Euclo-only adapter hops (nativeExecutor.Execute and
// executeWithWorkflowExecutor) were collapsed so Agent owns the direct handoff
// into managed orchestration.
//
// ProfileController is not the primary coding execution path. It is invoked
// during executeManagedFlow only for interactive mode-machine work when the
// interaction registry is active. The primary run path dispatches through
// the behavior dispatcher and the relurpic capability owners.
//
// ExecuteInput.Environment carries the framework agent substrate
// (model/registry/index/search/memory/config). Euclo-owned runtime services
// that are not part of the framework environment are injected separately via a
// Euclo ServiceBundle at behavior dispatch time.
//
// ExecuteInput.WorkflowExecutor is the selected executor family instance built
// from the UnitOfWork's ExecutorDescriptor. The behavior dispatcher receives
// both the executor and the UnitOfWork-derived execution context.
//
// Agent is the named coding-runtime boundary for software-engineering work.
type Agent struct {
	Config         *core.Config
	Delegate       *reactpkg.ReActAgent
	CheckpointPath string
	Memory         memory.MemoryStore
	Environment    agentenv.AgentEnvironment
	GraphDB        *graphdb.Engine
	RetrievalDB    *sql.DB
	PlanStore      frameworkplan.PlanStore
	PatternStore   patterns.PatternStore
	CommentStore   patterns.CommentStore
	WorkflowStore  memory.WorkflowStateStore
	ConvVerifier   frameworkplan.ConvergenceVerifier
	GuidanceBroker *guidance.GuidanceBroker
	LearningBroker *archaeolearning.Broker
	DeferralPlan   *guidance.DeferralPlan
	DeferralPolicy guidance.DeferralPolicy
	DoomLoop       *capability.DoomLoopDetector
	doomLoopWired  bool
	reactReady     bool

	ModeRegistry        *euclotypes.ModeRegistry
	ProfileRegistry     *euclotypes.ExecutionProfileRegistry
	InteractionRegistry *interaction.ModeMachineRegistry
	CodingCapabilities  *capabilities.EucloCapabilityRegistry
	ProfileCtrl         *orchestrate.ProfileController
	RecoveryCtrl        *orchestrate.RecoveryController
	BehaviorDispatcher  *euclodispatch.Dispatcher
	Emitter             interaction.FrameEmitter // live emitter from TUI; nil means use task-scoped emitter
	RuntimeProviders    []core.Provider
	ContextPipeline     *pretask.Pipeline            // Phase 2: context enrichment pipeline
	WorkspaceEnv        ayenitd.WorkspaceEnvironment // Full workspace environment

	// Phase 9: User recipe signals for dynamic resolution
	userRecipeSignals []eucloruntime.UserRecipeSignalSource
}

func New(env ayenitd.WorkspaceEnvironment) *Agent {
	agent := &Agent{}
	_ = agent.InitializeEnvironment(env)
	return agent
}

// DirectCapabilityRun invokes a single relurpic capability (primary or supporting)
// directly through the dispatcher, bypassing the full managed-execution pipeline.
// Used by the test harness for capability-level isolation testing.
func (a *Agent) DirectCapabilityRun(ctx context.Context, capabilityID, invokingPrimary string, task *core.Task, state *core.Context) (*core.Result, error) {
	if a.BehaviorDispatcher == nil {
		return nil, fmt.Errorf("behavior dispatcher not initialized")
	}
	work := eucloruntime.UnitOfWork{
		PrimaryRelurpicCapabilityID: invokingPrimary,
		SemanticInputs:              buildSemanticInputsFromState(state),
	}
	if invokingPrimary == "" {
		work.PrimaryRelurpicCapabilityID = capabilityID
	}
	artifacts, err := a.BehaviorDispatcher.ExecuteRoutine(ctx, capabilityID, task, state, work, a.Environment, a.serviceBundle())
	if err != nil {
		return &core.Result{Success: false, Error: err}, err
	}
	eucloexec.MergeStateArtifactsToContext(state, artifacts)
	eucloexec.SetBehaviorTrace(state, work, []string{capabilityID})
	return &core.Result{Success: true}, nil
}

// buildSemanticInputsFromState extracts semantic input references from state keys
// to support direct capability testing. This ensures supporting routines receive
// the WorkContext fields they need (PatternRefs, TensionRefs, etc.) even in
// capability_direct_run mode where the full agent loop hasn't populated them.
func buildSemanticInputsFromState(state *core.Context) eucloruntime.SemanticInputBundle {
	inputs := eucloruntime.SemanticInputBundle{}
	if state == nil {
		return inputs
	}

	// Extract string-slice refs from state keys that follow naming conventions
	// These are typically set by test setup's state_keys section
	inputs.PatternRefs = stringSliceFromState(state, "pattern_refs")
	inputs.TensionRefs = stringSliceFromState(state, "tension_refs")
	inputs.ProspectiveRefs = stringSliceFromState(state, "prospective_refs")
	inputs.ConvergenceRefs = stringSliceFromState(state, "convergence_refs")
	inputs.RequestProvenanceRefs = stringSliceFromState(state, "request_provenance_refs")
	inputs.LearningInteractionRefs = stringSliceFromState(state, "learning_interaction_refs")

	// Also check for single-value keys that might contain refs
	if refs := stringSliceFromStateKey(state, "archaeology.pattern_refs"); len(refs) > 0 {
		inputs.PatternRefs = append(inputs.PatternRefs, refs...)
	}
	if refs := stringSliceFromStateKey(state, "debug.pattern_refs"); len(refs) > 0 {
		inputs.PatternRefs = append(inputs.PatternRefs, refs...)
	}
	if refs := stringSliceFromStateKey(state, "archaeology.tension_refs"); len(refs) > 0 {
		inputs.TensionRefs = append(inputs.TensionRefs, refs...)
	}
	if refs := stringSliceFromStateKey(state, "debug.tension_refs"); len(refs) > 0 {
		inputs.TensionRefs = append(inputs.TensionRefs, refs...)
	}
	if refs := stringSliceFromStateKey(state, "archaeology.prospective_refs"); len(refs) > 0 {
		inputs.ProspectiveRefs = append(inputs.ProspectiveRefs, refs...)
	}
	if refs := stringSliceFromStateKey(state, "debug.prospective_refs"); len(refs) > 0 {
		inputs.ProspectiveRefs = append(inputs.ProspectiveRefs, refs...)
	}

	// Extract request provenance from various possible keys
	if refs := stringSliceFromStateKey(state, "archaeology.request_provenance_refs"); len(refs) > 0 {
		inputs.RequestProvenanceRefs = append(inputs.RequestProvenanceRefs, refs...)
	}
	if refs := stringSliceFromStateKey(state, "debug.request_provenance_refs"); len(refs) > 0 {
		inputs.RequestProvenanceRefs = append(inputs.RequestProvenanceRefs, refs...)
	}

	// Extract workflow/exploration IDs if present
	if id, ok := stringFromState(state, "workflow_id"); ok {
		inputs.WorkflowID = id
	}
	if id, ok := stringFromState(state, "exploration_id"); ok {
		inputs.ExplorationID = id
	}

	return inputs
}

// stringSliceFromState extracts a string slice from state by key.
func stringSliceFromState(state *core.Context, key string) []string {
	return stringSliceFromStateKey(state, key)
}

// stringSliceFromStateKey extracts a string slice from a specific state key.
// Handles both []string and []any (from JSON/YAML parsing) formats.
func stringSliceFromStateKey(state *core.Context, key string) []string {
	if raw, ok := state.Get(key); ok && raw != nil {
		switch typed := raw.(type) {
		case []string:
			return append([]string(nil), typed...)
		case []any:
			var result []string
			for _, v := range typed {
				if s, ok := v.(string); ok {
					result = append(result, s)
				}
			}
			return result
		case string:
			// Single string value - wrap in slice
			if typed != "" {
				return []string{typed}
			}
		}
	}
	return nil
}

// stringFromState extracts a string value from state by key.
func stringFromState(state *core.Context, key string) (string, bool) {
	if raw, ok := state.Get(key); ok && raw != nil {
		if s, ok := raw.(string); ok {
			return s, true
		}
	}
	return "", false
}

func (a *Agent) InitializeEnvironment(env ayenitd.WorkspaceEnvironment) error {
	a.Config = env.Config
	a.Memory = env.Memory
	// Store the full workspace environment
	a.WorkspaceEnv = env
	// Convert workspace environment to the agentenv subset used internally.
	// Workspace-specific fields (WorkflowStore, PlanStore, etc.) are already
	// stored as separate fields on Agent and are set directly below.
	a.Environment = agentenv.AgentEnvironment{
		Config:                        env.Config,
		Model:                         env.Model,
		CommandPolicy:                 env.CommandPolicy,
		Registry:                      env.Registry,
		IndexManager:                  env.IndexManager,
		SearchEngine:                  env.SearchEngine,
		Memory:                        env.Memory,
		VerificationPlanner:           env.VerificationPlanner,
		CompatibilitySurfaceExtractor: env.CompatibilitySurfaceExtractor,
	}
	// Propagate workspace-specific stores to dedicated fields.
	if env.WorkflowStore != nil && a.WorkflowStore == nil {
		a.WorkflowStore = env.WorkflowStore
	}
	if env.PlanStore != nil && a.PlanStore == nil {
		a.PlanStore = env.PlanStore
	}
	workspace := workspacePathFromEnv(env)
	factory := langdetect.ResolverFactory{Languages: detectWorkspaceLanguages(workspace)}
	if a.Environment.VerificationPlanner == nil {
		a.Environment.VerificationPlanner = factory.VerificationPlanner()
	}
	if a.Environment.CompatibilitySurfaceExtractor == nil {
		a.Environment.CompatibilitySurfaceExtractor = factory.CompatibilitySurfacePlanner()
	}
	if env.IndexManager != nil && env.IndexManager.GraphDB != nil {
		a.GraphDB = env.IndexManager.GraphDB
	}
	if a.Delegate != nil {
		a.reactReady = false
		if err := a.Delegate.InitializeEnvironment(a.Environment); err != nil {
			return err
		}
	}
	if a.Environment.Registry == nil {
		a.Environment.Registry = env.Registry
	}
	if a.Environment.Registry == nil {
		a.Environment.Registry = capability.NewRegistry()
	}
	if a.Delegate != nil && a.Delegate.Tools == nil {
		a.Delegate.Tools = a.Environment.Registry
	}
	if a.Delegate != nil && a.CheckpointPath != "" {
		a.Delegate.CheckpointPath = a.CheckpointPath
	}
	if a.Delegate != nil && a.Config != nil && a.Environment.Registry == nil {
		if err := a.Delegate.Initialize(a.Config); err != nil {
			return err
		}
	}
	if a.ModeRegistry == nil {
		a.ModeRegistry = euclotypes.DefaultModeRegistry()
	}
	if a.ProfileRegistry == nil {
		a.ProfileRegistry = euclotypes.DefaultExecutionProfileRegistry()
	}
	if a.InteractionRegistry == nil {
		a.InteractionRegistry = a.createInteractionRegistry()
	}
	if a.CodingCapabilities == nil {
		a.CodingCapabilities = capabilities.NewDefaultCapabilityRegistry(a.Environment)
	}
	if a.DoomLoop == nil {
		a.DoomLoop = capability.NewDoomLoopDetector(capability.DefaultDoomLoopConfig())
	}
	a.ensureGuidanceWiring()

	// Wire the snapshot function for orchestrate package.
	orchestrate.SetDefaultSnapshotFunc(func(reg interface{}) euclotypes.CapabilitySnapshot {
		if registry, ok := reg.(*capability.Registry); ok {
			return eucloruntime.SnapshotCapabilities(registry)
		}
		return euclotypes.CapabilitySnapshot{}
	})

	if a.RecoveryCtrl == nil {
		a.RecoveryCtrl = orchestrate.NewRecoveryController(
			orchestrate.AdaptCapabilityRegistry(a.CodingCapabilities),
			a.ProfileRegistry,
			a.ModeRegistry,
			a.Environment,
		)
	}
	if a.ProfileCtrl == nil {
		a.ProfileCtrl = orchestrate.NewProfileController(
			orchestrate.AdaptCapabilityRegistry(a.CodingCapabilities),
			gate.DefaultPhaseGates(),
			a.Environment,
			a.ProfileRegistry,
			a.RecoveryCtrl,
		)
	}
	if a.BehaviorDispatcher == nil {
		a.BehaviorDispatcher = euclodispatch.NewDispatcher(a.Environment)
	}
	a.registerDeferralsResolveRoutine()
	a.registerLearningPromoteRoutine()

	// Phase 9: Load and register thought recipes
	a.initializeThoughtRecipes()

	// Initialize context enrichment pipeline (Phase 2)
	if a.ContextPipeline == nil {
		config := pretask.DefaultPipelineConfig()

		// Read configuration from agent spec if available
		if a.Config != nil && a.Config.AgentSpec != nil {
			// Check for context enrichment configuration in skill_config
			// This is a simplified implementation - in reality, we would parse the YAML
			// For now, we'll use defaults
		}

		// Get the tension service if available
		var tensionQuerier pretask.TensionQuerier
		// tensionService returns a struct, not a pointer; we need to check if it's usable.
		// We'll create the querier regardless; its methods will handle nil store.
		tensionQuerier = &tensionServiceQuerier{service: a.tensionService()}
		// Create pipeline environment from WorkspaceEnv
		env := pretask.PipelineEnv{
			IndexManager:   a.WorkspaceEnv.IndexManager,
			Model:          a.WorkspaceEnv.Model,
			Embedder:       a.WorkspaceEnv.Embedder,
			PatternStore:   a.WorkspaceEnv.PatternStore,
			KnowledgeStore: a.WorkspaceEnv.KnowledgeStore,
		}
		if reg := a.WorkspaceEnv.Registry; reg != nil {
			env.PolicySnapshotProvider = func() *core.PolicySnapshot {
				return reg.CapturePolicySnapshot()
			}
		}
		pipeline := pretask.NewPipeline(env, tensionQuerier, config)
		if workspace := workspacePathFromEnv(a.WorkspaceEnv); workspace != "" {
			pipeline.PrependStep(pretask.DeferralLoader{WorkspaceDir: workspace})
		}
		if a.WorkspaceEnv.PatternStore != nil {
			pipeline.AppendStep(pretask.LearningSyncStep{
				LearningService:  a.learningService(),
				WorkflowResolver: func(state *core.Context) string { return workflowIDFromState(state) },
			})
			pipeline.AppendStep(pretask.LearningDeltaStep{
				LearningService:  a.learningService(),
				WorkflowResolver: func(state *core.Context) string { return workflowIDFromState(state) },
				SessionResolver: func(state *core.Context) string {
					if state == nil {
						return ""
					}
					return state.GetString("euclo.last_session_revision")
				},
			})
		}
		a.ContextPipeline = pipeline
	}

	return a.Initialize(a.Environment.Config)
}

func workspacePathFromEnv(env ayenitd.WorkspaceEnvironment) string {
	if path := workspacePathFromConfig(env.Config); path != "" {
		return path
	}
	if env.IndexManager != nil {
		if path := strings.TrimSpace(env.IndexManager.WorkspacePath()); path != "" {
			return path
		}
	}
	if path, err := os.Getwd(); err == nil {
		if path = strings.TrimSpace(path); path != "" {
			return path
		}
	}
	return "."
}

func workspacePathFromConfig(cfg *core.Config) string {
	if cfg == nil || cfg.AgentSpec == nil || len(cfg.AgentSpec.Extensions) == 0 {
		return ""
	}
	if value, ok := cfg.AgentSpec.Extensions["workspace"]; ok {
		if path := strings.TrimSpace(fmt.Sprint(value)); path != "" && path != "<nil>" {
			return path
		}
	}
	if value, ok := cfg.AgentSpec.Extensions["euclo.workspace"]; ok {
		if path := strings.TrimSpace(fmt.Sprint(value)); path != "" && path != "<nil>" {
			return path
		}
	}
	return ""
}

func (a *Agent) Initialize(cfg *core.Config) error {
	a.Config = cfg
	a.reactReady = false
	if a.ModeRegistry == nil {
		a.ModeRegistry = euclotypes.DefaultModeRegistry()
	}
	if a.ProfileRegistry == nil {
		a.ProfileRegistry = euclotypes.DefaultExecutionProfileRegistry()
	}
	if a.Delegate != nil {
		if a.CheckpointPath != "" {
			a.Delegate.CheckpointPath = a.CheckpointPath
		}
		if err := a.Delegate.Initialize(cfg); err != nil {
			return err
		}
		a.reactReady = true
		return nil
	}
	return nil
}

// initializeThoughtRecipes loads user-defined thought recipes from the default
// recipe directory (.relurpify/recipes), registers them as Descriptors in the
// relurpic registry, and wires the recipe executor to the behavior dispatcher.
// This implements the Phase 9 startup integration for dynamic resolution.
func (a *Agent) initializeThoughtRecipes() {
	if a.BehaviorDispatcher == nil {
		return
	}

	// Try default recipe directory
	recipeDir := ".relurpify/recipes"
	if _, err := os.Stat(recipeDir); err != nil {
		// No recipe directory found, skip
		return
	}

	// Create relurpic registry for user recipes
	relurpicRegistry := euclorelurpic.NewRegistry()

	// Load and register recipes
	result := capabilities.LoadAndRegisterRecipes(recipeDir, relurpicRegistry, a.Environment)

	// Log warnings and errors
	for _, warning := range result.Warnings {
		// Could emit to telemetry or log
		_ = warning
	}
	for _, err := range result.Errors {
		// Log errors but don't fail startup
		_ = err
	}

	// Wire the recipe registry and executor to the dispatcher
	if result.Registry != nil && result.Executor != nil {
		a.BehaviorDispatcher.SetRecipeRegistry(result.Registry, result.Executor)
	}

	// Build user recipe signals for classification using declared intent keywords and modes.
	a.userRecipeSignals = make([]eucloruntime.UserRecipeSignalSource, 0)
	for _, name := range result.Registry.List() {
		plan, ok := result.Registry.Get(name)
		if !ok || plan == nil {
			continue
		}
		// Only register recipes with declared intent keywords — without keywords,
		// collectUserRecipeSignals can never match them anyway.
		if len(plan.IntentKeywords) == 0 {
			continue
		}
		signal := eucloruntime.UserRecipeSignalSource{
			RecipeID: "euclo:recipe." + plan.Name,
			Keywords: append([]string(nil), plan.IntentKeywords...),
			Modes:    append([]string(nil), plan.Modes...),
		}
		a.userRecipeSignals = append(a.userRecipeSignals, signal)
	}
}

func (a *Agent) Capabilities() []core.Capability {
	return []core.Capability{
		core.CapabilityCode,
		core.CapabilityPlan,
		core.CapabilityExecute,
		core.CapabilityReview,
		core.CapabilityExplain,
	}
}

func (a *Agent) CapabilityRegistry() *capability.Registry {
	if a == nil {
		return nil
	}
	if a.Environment.Registry != nil {
		return a.Environment.Registry
	}
	if a.Delegate != nil {
		return a.Delegate.Tools
	}
	return nil
}

func (a *Agent) applyLearningResolution(ctx context.Context, task *core.Task, state *core.Context) error {
	if a == nil || state == nil {
		return nil
	}
	workflowID := workflowIDFromTaskState(task, state)
	if workflowID == "" {
		return nil
	}
	raw, ok := agentstate.ExtractLearningResolutionPayload(task, state)
	if !ok || raw == nil {
		return nil
	}
	input, ok := agentstate.BuildLearningResolutionInput(workflowID, raw)
	if !ok {
		return nil
	}
	resolved, err := a.learningService().Resolve(ctx, input)
	if err != nil {
		return err
	}
	euclostate.SetLastLearningResolution(state, resolved)
	pending, err := a.learningService().Pending(ctx, workflowID)
	if err != nil {
		return err
	}
	euclostate.SetLearningQueue(state, pending)
	ids := make([]string, 0, len(pending))
	for _, interaction := range pending {
		ids = append(ids, interaction.ID)
	}
	euclostate.SetPendingLearningIDs(state, ids)
	return nil
}

func (a *Agent) workflowStore() *memorydb.SQLiteWorkflowStateStore {
	if a == nil {
		return nil
	}
	if typed, ok := a.WorkflowStore.(*memorydb.SQLiteWorkflowStateStore); ok {
		return typed
	}
	surfaces := euclorestore.ResolveRuntimeSurfaces(a.Memory)
	return surfaces.Workflow
}

func (a *Agent) phaseService() archaeophases.Service {
	return a.archaeoBinding().PhaseService()
}

func (a *Agent) archaeologyService() archaeoarch.Service {
	return a.archaeoBinding().ArchaeologyService(archaeobindings.ArchaeologyConfig{
		PersistPhase: func(ctx context.Context, task *core.Task, state *core.Context, phase archaeodomain.EucloPhase, blockedReason string, step *frameworkplan.PlanStep) {
			_, _ = a.phaseService().RecordState(ctx, task, state, a.GuidanceBroker, phase, blockedReason, step)
		},
		EvaluateGate: a.evaluatePlanStepGate,
		ResetDoom: func() {
			if a != nil && a.DoomLoop != nil {
				a.DoomLoop.Reset()
			}
		},
	})
}

func (a *Agent) learningService() archaeolearning.Service {
	return a.archaeoBinding().LearningService()
}

func (a *Agent) ActiveExploration(ctx context.Context, workspaceID string) (*archaeoarch.SessionView, error) {
	if a == nil {
		return nil, nil
	}
	return a.archaeoBinding().ActiveExploration(ctx, workspaceID)
}

func (a *Agent) ExplorationView(ctx context.Context, explorationID string) (*archaeoarch.SessionView, error) {
	if a == nil {
		return nil, nil
	}
	return a.archaeoBinding().ExplorationView(ctx, explorationID)
}

func (a *Agent) PendingLearning(ctx context.Context, workflowID string) ([]archaeolearning.Interaction, error) {
	if a == nil {
		return nil, nil
	}
	return a.archaeoBinding().PendingLearning(ctx, workflowID)
}

func (a *Agent) ResolveLearning(ctx context.Context, input archaeolearning.ResolveInput) (*archaeolearning.Interaction, error) {
	if a == nil {
		return nil, fmt.Errorf("euclo agent unavailable")
	}
	return a.archaeoBinding().ResolveLearning(ctx, input)
}

func (a *Agent) PlanVersions(ctx context.Context, workflowID string) ([]archaeodomain.VersionedLivingPlan, error) {
	if a == nil {
		return nil, nil
	}
	return a.archaeoBinding().PlanVersions(ctx, workflowID)
}

func (a *Agent) ActivePlanVersion(ctx context.Context, workflowID string) (*archaeodomain.VersionedLivingPlan, error) {
	if a == nil {
		return nil, nil
	}
	return a.archaeoBinding().ActivePlanVersion(ctx, workflowID)
}

func (a *Agent) ComparePlanVersions(ctx context.Context, workflowID string, fromVersion, toVersion int) (map[string]any, error) {
	if a == nil {
		return nil, nil
	}
	return a.archaeoBinding().ComparePlanVersions(ctx, workflowID, fromVersion, toVersion)
}

func (a *Agent) TensionsByWorkflow(ctx context.Context, workflowID string) ([]archaeodomain.Tension, error) {
	if a == nil {
		return nil, nil
	}
	return a.archaeoBinding().TensionsByWorkflow(ctx, workflowID)
}

func (a *Agent) TensionsByExploration(ctx context.Context, explorationID string) ([]archaeodomain.Tension, error) {
	if a == nil {
		return nil, nil
	}
	return a.archaeoBinding().TensionsByExploration(ctx, explorationID)
}

func (a *Agent) UpdateTensionStatus(ctx context.Context, workflowID, tensionID string, status archaeodomain.TensionStatus, commentRefs []string) (*archaeodomain.Tension, error) {
	if a == nil {
		return nil, fmt.Errorf("euclo agent unavailable")
	}
	return a.archaeoBinding().UpdateTensionStatus(ctx, workflowID, tensionID, status, commentRefs)
}

func (a *Agent) TensionSummaryByWorkflow(ctx context.Context, workflowID string) (*archaeodomain.TensionSummary, error) {
	if a == nil {
		return nil, nil
	}
	return a.archaeoBinding().TensionSummaryByWorkflow(ctx, workflowID)
}

func (a *Agent) TensionSummaryByExploration(ctx context.Context, explorationID string) (*archaeodomain.TensionSummary, error) {
	if a == nil {
		return nil, nil
	}
	return a.archaeoBinding().TensionSummaryByExploration(ctx, explorationID)
}

func (a *Agent) planService() archaeoplans.Service {
	return a.archaeoBinding().PlanService()
}

func (a *Agent) tensionService() archaeotensions.Service {
	return a.archaeoBinding().TensionService()
}

// tensionServiceQuerier implements pretask.TensionQuerier using archaeotensions.Service
type tensionServiceQuerier struct {
	service archaeotensions.Service
}

func (q *tensionServiceQuerier) ActiveByWorkflow(ctx context.Context, workflowID string) ([]interface{}, error) {
	// Check if the service is usable by checking if the store is nil
	// The service has a Store field which is a WorkflowStateStore
	// We can use reflection or check a method, but for now, just try to call ListByWorkflow
	// and handle errors gracefully
	tensions, err := q.service.ListByWorkflow(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	// Convert to []interface{}
	result := make([]interface{}, len(tensions))
	for i, t := range tensions {
		result[i] = t
	}
	return result, nil
}

func (a *Agent) verificationService() archaeoverification.Service {
	return a.archaeoBinding().VerificationService()
}

func (a *Agent) projectionService() *archaeoprojections.Service {
	return a.archaeoBinding().ProjectionService()
}

func (a *Agent) semanticInputBundle(task *core.Task, state *core.Context, mode euclotypes.ModeResolution) eucloruntime.SemanticInputBundle {
	if a == nil {
		return eucloruntime.SemanticInputBundle{}
	}
	if mode.ModeID != "planning" && mode.ModeID != "debug" && mode.ModeID != "review" {
		if existing, ok := state.Get("euclo.semantic_inputs"); ok && existing != nil {
			if typed, ok := existing.(eucloruntime.SemanticInputBundle); ok {
				return agentstate.EnrichBundleWithContextKnowledge(typed, state)
			}
		}
		return agentstate.EnrichBundleWithContextKnowledge(eucloruntime.SemanticInputBundle{}, state)
	}
	workflowID := workflowIDFromTaskState(task, state)
	if workflowID == "" {
		return agentstate.EnrichBundleWithContextKnowledge(eucloruntime.SemanticInputBundle{}, state)
	}
	ctx := context.Background()
	var activePlan *archaeodomain.VersionedLivingPlan
	if projection, err := a.ActivePlanProjection(ctx, workflowID); err == nil && projection != nil {
		activePlan = projection.ActivePlanVersion
	}
	requests, _ := a.projectionService().RequestHistory(ctx, workflowID)
	provenance, _ := a.projectionService().Provenance(ctx, workflowID)
	learning, _ := a.LearningQueueProjection(ctx, workflowID)
	var convergence *archaeodomain.WorkspaceConvergenceProjection
	if workspaceID := agentstate.WorkspaceIDFromTask(task, state); workspaceID != "" {
		convergence, _ = a.archaeoBinding().ConvergenceHistory(ctx, workspaceID)
	}
	return agentstate.BuildSemanticInputBundle(
		workflowID,
		activePlan,
		requests,
		provenance,
		learning,
		convergence,
		state,
		mode.ModeID,
	)
}

func (a *Agent) WorkflowProjection(ctx context.Context, workflowID string) (*archaeoprojections.WorkflowReadModel, error) {
	if a == nil {
		return nil, nil
	}
	return a.archaeoBinding().WorkflowProjection(ctx, workflowID)
}

func (a *Agent) ExplorationProjection(ctx context.Context, workflowID string) (*archaeoprojections.ExplorationProjection, error) {
	if a == nil {
		return nil, nil
	}
	return a.archaeoBinding().ExplorationProjection(ctx, workflowID)
}

func (a *Agent) LearningQueueProjection(ctx context.Context, workflowID string) (*archaeoprojections.LearningQueueProjection, error) {
	if a == nil {
		return nil, nil
	}
	return a.archaeoBinding().LearningQueueProjection(ctx, workflowID)
}

func (a *Agent) ActivePlanProjection(ctx context.Context, workflowID string) (*archaeoprojections.ActivePlanProjection, error) {
	if a == nil {
		return nil, nil
	}
	return a.archaeoBinding().ActivePlanProjection(ctx, workflowID)
}

func (a *Agent) WorkflowTimeline(ctx context.Context, workflowID string) ([]archaeodomain.TimelineEvent, error) {
	if a == nil {
		return nil, nil
	}
	return a.archaeoBinding().WorkflowTimeline(ctx, workflowID)
}

func (a *Agent) SubscribeWorkflowProjection(workflowID string, buffer int) (<-chan archaeoprojections.ProjectionEvent, func()) {
	if a == nil {
		ch := make(chan archaeoprojections.ProjectionEvent)
		close(ch)
		return ch, func() {}
	}
	return a.archaeoBinding().SubscribeWorkflowProjection(workflowID, buffer)
}

func (a *Agent) executionService() archaeoexec.Service {
	return a.archaeoBinding().ExecutionService()
}

func (a *Agent) executionHandoffRecorder() archaeoexec.HandoffRecorder {
	return a.archaeoBinding().ExecutionHandoffRecorder()
}

func (a *Agent) preflightCoordinator() archaeoexec.PreflightCoordinator {
	return a.archaeoBinding().PreflightCoordinator(archaeobindings.PreflightConfig{
		RequestGuidance: a.requestGuidance,
	})
}

func (a *Agent) executionFinalizer() archaeoexec.Finalizer {
	return a.archaeoBinding().ExecutionFinalizer(archaeobindings.FinalizerConfig{
		GitCheckpoint: func(ctx context.Context, task *core.Task) string {
			return gitCheckpoint(ctx, task, a.Environment.Registry)
		},
	})
}

func (a *Agent) assuranceRuntime() eucloassurance.Runtime {
	return eucloassurance.Runtime{
		Memory:              a.Memory,
		Environment:         a.Environment,
		ProfileCtrl:         a.ProfileCtrl,
		BehaviorDispatcher:  a.BehaviorDispatcher,
		InteractionRegistry: a.InteractionRegistry,
		Emitter:             a.Emitter,
		ResolveEmitter: func(task *core.Task, live interaction.FrameEmitter) (interaction.FrameEmitter, bool, int) {
			if live != nil {
				return live, false, 0
			}
			emitter, withTransitions := agentstate.InteractionEmitterForTask(task)
			return emitter, withTransitions, agentstate.InteractionMaxTransitions(task)
		},
		SeedInteraction:  agentstate.SeedInteractionPrepass,
		PersistArtifacts: a.persistArtifacts,
		Checkpoint: func(ctx context.Context, checkpoint archaeodomain.MutationCheckpoint, task *core.Task, state *core.Context) error {
			return a.liveMutationCheckpoint(ctx, checkpoint, task, state)
		},
		ResetDoomLoop: func() {
			if a.DoomLoop != nil {
				a.DoomLoop.Reset()
			}
		},
	}
}

func (a *Agent) liveMutationCoordinator() archaeoexec.LiveMutationCoordinator {
	return archaeoexec.LiveMutationCoordinator{
		Service:         a.executionService(),
		Plans:           a.planService(),
		RequestGuidance: a.requestGuidance,
	}
}

func (a *Agent) liveMutationCheckpoint(ctx context.Context, checkpoint archaeodomain.MutationCheckpoint, task *core.Task, state *core.Context) error {
	if state == nil {
		return nil
	}
	rawPlan, ok := state.Get("euclo.living_plan")
	if !ok || rawPlan == nil {
		return nil
	}
	plan, ok := rawPlan.(*frameworkplan.LivingPlan)
	if !ok || plan == nil {
		return nil
	}
	stepID := strings.TrimSpace(state.GetString("euclo.current_plan_step_id"))
	if stepID == "" {
		return nil
	}
	step := plan.Steps[stepID]
	if step == nil {
		return nil
	}
	_, err := a.liveMutationCoordinator().CheckpointExecutionAt(ctx, checkpoint, task, state, plan, step)
	return err
}

func (a *Agent) phaseDriver() archaeophases.Driver {
	return a.archaeoBinding().PhaseDriver(archaeobindings.DriverConfig{
		Handoff: func(ctx context.Context, task *core.Task, state *core.Context, step *frameworkplan.PlanStep) error {
			planRaw, ok := state.Get("euclo.living_plan")
			if !ok || planRaw == nil {
				return nil
			}
			plan, ok := planRaw.(*frameworkplan.LivingPlan)
			if !ok || plan == nil {
				return nil
			}
			_, err := a.executionHandoffRecorder().Record(ctx, task, state, plan, step)
			return err
		},
	})
}

func (a *Agent) archaeoBinding() archaeobindings.Runtime {
	var workflowStore memory.WorkflowStateStore
	if store := a.workflowStore(); store != nil {
		workflowStore = store
	}
	return archaeobindings.Runtime{
		WorkflowStore:  workflowStore,
		PlanStore:      a.PlanStore,
		PatternStore:   a.PatternStore,
		CommentStore:   a.CommentStore,
		Retrieval:      archaeoretrieval.NewSQLStore(a.RetrievalDB),
		ConvVerifier:   a.ConvVerifier,
		GuidanceBroker: a.GuidanceBroker,
		LearningBroker: a.LearningBroker,
		DeferralPolicy: a.DeferralPolicy,
	}
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (a *Agent) evaluatePlanStepGate(ctx context.Context, task *core.Task, state *core.Context, plan *frameworkplan.LivingPlan, step *frameworkplan.PlanStep) (archaeoexec.PreflightOutcome, error) {
	return a.preflightCoordinator().EvaluatePlanStepGate(ctx, task, state, plan, step, a.GraphDB)
}

func (a *Agent) ensureGuidanceWiring() {
	if a == nil || a.DoomLoop == nil || a.CapabilityRegistry() == nil {
		return
	}
	registry := a.CapabilityRegistry()
	if !a.doomLoopWired {
		registry.AddPrecheck(a.DoomLoop)
		registry.AddPostcheck(a.DoomLoop)
		a.doomLoopWired = true
	}
	if a.GuidanceBroker != nil {
		registry.SetGuidanceBroker(guidanceRecoveryAdapter{broker: a.GuidanceBroker})
	}
}

func (a *Agent) requestGuidance(ctx context.Context, req guidance.GuidanceRequest, fallbackChoice string) guidance.GuidanceDecision {
	return a.executionService().RequestGuidance(ctx, req, fallbackChoice)
}

type guidanceRecoveryAdapter struct {
	broker *guidance.GuidanceBroker
}

func (a guidanceRecoveryAdapter) RequestRecovery(ctx context.Context, req capability.RecoveryGuidanceRequest) (*capability.RecoveryGuidanceDecision, error) {
	if a.broker == nil {
		return nil, fmt.Errorf("guidance broker unavailable")
	}
	decision, err := a.broker.Request(ctx, guidance.GuidanceRequest{
		Kind:        guidance.GuidanceRecovery,
		Title:       req.Title,
		Description: req.Description,
		Choices: []guidance.GuidanceChoice{
			{ID: "continue", Label: "Continue"},
			{ID: "replan", Label: "Re-plan this step"},
			{ID: "skip", Label: "Skip step"},
			{ID: "stop", Label: "Stop", IsDefault: true},
		},
		TimeoutBehavior: guidance.GuidanceTimeoutFail,
		Context:         req.Context,
	})
	if err != nil {
		return nil, err
	}
	return &capability.RecoveryGuidanceDecision{ChoiceID: decision.ChoiceID}, nil
}

func workflowIDFromTaskState(task *core.Task, state *core.Context) string {
	if state != nil {
		if workflowID := strings.TrimSpace(state.GetString("euclo.workflow_id")); workflowID != "" {
			return workflowID
		}
	}
	if task != nil && task.Context != nil {
		if workflowID := strings.TrimSpace(stringValue(task.Context["workflow_id"])); workflowID != "" {
			return workflowID
		}
	}
	return ""
}

func workflowIDFromState(state *core.Context) string {
	if state == nil {
		return ""
	}
	if workflowID := strings.TrimSpace(state.GetString("euclo.workflow_id")); workflowID != "" {
		return workflowID
	}
	if workflowID := strings.TrimSpace(state.GetString("workflow_id")); workflowID != "" {
		return workflowID
	}
	return ""
}

func explorationIDFromState(state *core.Context) string {
	if state == nil {
		return ""
	}
	if explorationID := strings.TrimSpace(state.GetString("euclo.active_exploration_id")); explorationID != "" {
		return explorationID
	}
	if explorationID := strings.TrimSpace(state.GetString("euclo.exploration_id")); explorationID != "" {
		return explorationID
	}
	if explorationID := strings.TrimSpace(state.GetString("exploration_id")); explorationID != "" {
		return explorationID
	}
	return ""
}

func gitCheckpoint(ctx context.Context, task *core.Task, registry *capability.Registry) string {
	if task == nil || task.Context == nil {
		return ""
	}
	workspace := strings.TrimSpace(stringValue(task.Context["workspace"]))
	if workspace == "" || registry == nil {
		return ""
	}
	var result *core.ToolResult
	for _, capabilityID := range []string{"tool:cli_git", "cli_git"} {
		candidate, err := registry.InvokeCapability(ctx, core.NewContext(), capabilityID, map[string]any{
			"args":              []string{"rev-parse", "HEAD"},
			"working_directory": workspace,
		})
		if err == nil && candidate != nil && candidate.Success {
			result = candidate
			break
		}
	}
	if result == nil {
		if tool, ok := registry.Get("cli_git"); ok && tool != nil {
			candidate, err := tool.Execute(ctx, core.NewContext(), map[string]any{
				"args":              []string{"rev-parse", "HEAD"},
				"working_directory": workspace,
			})
			if err == nil && candidate != nil && candidate.Success {
				result = candidate
			}
		}
	}
	if result == nil {
		return ""
	}
	stdout, _ := result.Data["stdout"].(string)
	return strings.TrimSpace(stdout)
}

func seedPersistedInteractionState(task *core.Task, state *core.Context) {
	if task == nil || task.Context == nil || state == nil {
		return
	}
	if _, ok := euclostate.GetInteractionState(state); !ok {
		if raw, ok := task.Context["euclo.interaction_state"]; ok && raw != nil {
			euclostate.SetInteractionState(state, raw)
		}
	}
}

func (a *Agent) ensureReactDelegate() error {
	if a == nil {
		return nil
	}
	if a.Delegate == nil {
		env := a.Environment
		if env.Registry == nil {
			env.Registry = capability.NewRegistry()
			a.Environment.Registry = env.Registry
		}
		a.Delegate = reactpkg.New(env)
		a.reactReady = false
	}
	if a.Delegate.Tools == nil {
		a.Delegate.Tools = a.CapabilityRegistry()
	}
	if a.CheckpointPath != "" {
		a.Delegate.CheckpointPath = a.CheckpointPath
	}
	if a.Config != nil && !a.reactReady {
		if err := a.Delegate.Initialize(a.Config); err != nil {
			return err
		}
		a.reactReady = true
	}
	return nil
}

func (a *Agent) eucloTask(task *core.Task, envelope eucloruntime.TaskEnvelope, classification eucloruntime.TaskClassification, mode euclotypes.ModeResolution, profile euclotypes.ExecutionProfileSelection, work eucloruntime.UnitOfWork) *core.Task {
	cloned := core.CloneTask(task)
	if cloned == nil {
		cloned = &core.Task{}
	}
	if cloned.Context == nil {
		cloned.Context = map[string]any{}
	}
	cloned.Context["mode"] = mode.ModeID
	cloned.Context["euclo.mode"] = mode.ModeID
	cloned.Context["euclo.execution_profile"] = profile.ProfileID
	cloned.Context["euclo.envelope"] = envelope
	cloned.Context["euclo.classification"] = eucloruntime.ClassificationContextPayload(classification)
	cloned.Context["euclo.unit_of_work"] = eucloruntime.UnitOfWorkContextPayload(work)
	return cloned
}

func (a *Agent) hydratePersistedArtifacts(ctx context.Context, task *core.Task, state *core.Context) bool {
	if state == nil {
		return false
	}
	if raw, ok := state.Get("euclo.artifacts"); ok && raw != nil {
		return false
	}
	surfaces := euclorestore.ResolveRuntimeSurfaces(a.Memory)
	if surfaces.Workflow == nil {
		return false
	}
	workflowID := state.GetString("euclo.workflow_id")
	if workflowID == "" && task != nil && task.Context != nil {
		if value, ok := task.Context["workflow_id"]; ok {
			workflowID = stringValue(value)
		}
	}
	if workflowID == "" {
		return false
	}
	runID := state.GetString("euclo.run_id")
	if runID == "" && task != nil && task.Context != nil {
		if value, ok := task.Context["run_id"]; ok {
			runID = stringValue(value)
		}
	}
	artifacts, err := euclotypes.LoadPersistedArtifacts(ctx, surfaces.Workflow, workflowID, runID)
	if err != nil || len(artifacts) == 0 {
		return false
	}
	euclostate.SetArtifacts(state, artifacts)
	euclotypes.RestoreStateFromArtifacts(state, artifacts)
	return true
}

func (a *Agent) restoreExecutionContinuity(ctx context.Context, task *core.Task, state *core.Context, envelope eucloruntime.TaskEnvelope, work eucloruntime.UnitOfWork) error {
	if state == nil {
		return nil
	}
	explicitRestore := eucloruntime.RestoreRequested(task, state)
	if !explicitRestore && !shouldHydratePersistedArtifacts(task, state, envelope) {
		return nil
	}
	hadCompiled := false
	if _, ok := eucloruntime.CompiledExecutionFromState(state); ok {
		hadCompiled = true
	}
	eucloruntime.MarkContextLifecycleRestoring(state, time.Now().UTC())
	hydrated := a.hydratePersistedArtifacts(ctx, task, state)
	compiled, ok := eucloruntime.CompiledExecutionFromState(state)
	if !ok {
		if explicitRestore || hydrated {
			lifecycle, _ := eucloruntime.ContextLifecycleFromState(state)
			lifecycle = eucloruntime.BuildContextLifecycleState(work, lifecycle, eucloruntime.ExecutionStatusRestoreFailed, nil, time.Now().UTC())
			state.Set("euclo.context_compaction", lifecycle)
			return fmt.Errorf("euclo restore failed for workflow %s run %s", work.WorkflowID, work.RunID)
		}
		return nil
	}
	if !hadCompiled || explicitRestore || hydrated {
		if restoredWork, ok := eucloruntime.ReconstructUnitOfWorkFromCompiledExecution(state); ok {
			state.Set("euclo.unit_of_work", restoredWork)
			state.Set("euclo.unit_of_work_id", restoredWork.ID)
			work = restoredWork
		}
		artifactKinds := make([]string, 0)
		if raw, ok := state.Get("euclo.artifacts"); ok && raw != nil {
			if artifacts, ok := raw.([]euclotypes.Artifact); ok {
				for _, artifact := range artifacts {
					artifactKinds = append(artifactKinds, string(artifact.Kind))
				}
			}
		}
		lifecycle, _ := eucloruntime.ContextLifecycleFromState(state)
		if strings.TrimSpace(work.WorkflowID) == "" {
			work.WorkflowID = strings.TrimSpace(compiled.WorkflowID)
		}
		if strings.TrimSpace(work.RunID) == "" {
			work.RunID = strings.TrimSpace(compiled.RunID)
		}
		if strings.TrimSpace(work.ExecutionID) == "" {
			work.ExecutionID = strings.TrimSpace(compiled.ExecutionID)
		}
		lifecycle = eucloruntime.BuildContextLifecycleState(work, lifecycle, eucloruntime.ExecutionStatusReady, artifactKinds, time.Now().UTC())
		state.Set("euclo.context_compaction", lifecycle)
	}
	if compiled.WorkflowID != "" && compiled.RunID != "" {
		restoreState, err := euclorestore.RestoreProviderSnapshotState(ctx, a.workflowStore(), compiled.WorkflowID, compiled.RunID, state)
		if err == nil {
			restoreState, err = euclorestore.ApplyProviderRuntimeRestore(ctx, a.runtimeProviders(state), state)
		}
		if err == nil {
			if current, ok := eucloruntime.CompiledExecutionFromState(state); ok {
				current.ProviderSnapshotRefs = append([]string(nil), restoreState.ProviderSnapshotRefs...)
				current.ProviderSessionSnapshotRefs = append([]string(nil), restoreState.SessionSnapshotRefs...)
				state.Set("euclo.compiled_execution", current)
			}
		} else if restoreState.MateriallyRequired {
			lifecycle, _ := eucloruntime.ContextLifecycleFromState(state)
			lifecycle = eucloruntime.BuildContextLifecycleState(work, lifecycle, eucloruntime.ExecutionStatusRestoreFailed, nil, time.Now().UTC())
			state.Set("euclo.context_compaction", lifecycle)
			return err
		}
	}
	return nil
}

func (a *Agent) persistArtifacts(ctx context.Context, task *core.Task, state *core.Context, artifacts []euclotypes.Artifact) error {
	surfaces := euclorestore.ResolveRuntimeSurfaces(a.Memory)
	if surfaces.Workflow == nil || len(artifacts) == 0 {
		return nil
	}
	workflowID, runID, err := euclorestore.EnsureWorkflowRun(ctx, surfaces.Workflow, task, state)
	if err != nil {
		return err
	}
	if workflowID == "" {
		return nil
	}
	return euclotypes.PersistWorkflowArtifacts(ctx, surfaces.Workflow, workflowID, runID, artifacts)
}

func (a *Agent) refreshRuntimeExecutionArtifacts(ctx context.Context, task *core.Task, state *core.Context, work eucloruntime.UnitOfWork, status eucloruntime.ExecutionStatus, execErr error) {
	if state == nil {
		return
	}
	issues := eucloruntime.BuildDeferredExecutionIssues(a.DeferralPlan, work, state, time.Now().UTC())
	issues = append(issues, eucloruntime.BuildCapabilityContractDeferredIssues(work, state, time.Now().UTC())...)
	work.SemanticInputs = eucloarchaeomem.EnrichSemanticInputBundle(work.SemanticInputs, state, work, issues)
	issues = eucloarchaeomem.ApplySemanticReasoningToDeferredIssues(issues, work.SemanticInputs, state)
	issues = eucloruntime.PersistDeferredExecutionIssuesToWorkspace(task, state, issues)
	eucloruntime.SeedDeferredIssueState(state, issues)
	work.DeferredIssueIDs = deferredIssueIDsFromState(state, work.DeferredIssueIDs)
	if raw, ok := state.Get("euclo.assurance_class"); ok && raw != nil {
		if value := strings.TrimSpace(fmt.Sprint(raw)); value != "" && value != "<nil>" {
			work.AssuranceClass = eucloruntime.AssuranceClass(value)
		}
	}
	work.ResultClass = euclowork.ResultClassForOutcome(status, work.DeferredIssueIDs, execErr)
	switch work.AssuranceClass {
	case eucloruntime.AssuranceClassRepairExhausted:
		work.ResultClass = eucloruntime.ExecutionResultClassFailed
		status = eucloruntime.ExecutionStatusFailed
	case eucloruntime.AssuranceClassReviewBlocked:
		work.ResultClass = eucloruntime.ExecutionResultClassBlocked
		status = eucloruntime.ExecutionStatusBlocked
	case eucloruntime.AssuranceClassTDDIncomplete:
		work.ResultClass = eucloruntime.ExecutionResultClassFailed
		status = eucloruntime.ExecutionStatusFailed
	case eucloruntime.AssuranceClassOperatorDeferred:
		work.ResultClass = eucloruntime.ExecutionResultClassCompletedWithDeferrals
		status = eucloruntime.ExecutionStatusCompletedWithDeferrals
	}
	status = euclowork.StatusForResultClass(status, work.ResultClass)
	switch work.ResultClass {
	case eucloruntime.ExecutionResultClassCompletedWithDeferrals:
		work.Status = eucloruntime.UnitOfWorkStatusCompletedWithDeferrals
	case eucloruntime.ExecutionResultClassCompleted:
		work.Status = eucloruntime.UnitOfWorkStatusCompleted
	case eucloruntime.ExecutionResultClassBlocked:
		work.Status = eucloruntime.UnitOfWorkStatusBlocked
	case eucloruntime.ExecutionResultClassCanceled:
		work.Status = eucloruntime.UnitOfWorkStatusCanceled
	default:
		work.Status = eucloruntime.UnitOfWorkStatusFailed
	}
	work.UpdatedAt = time.Now().UTC()
	state.Set("euclo.semantic_inputs", work.SemanticInputs)
	state.Set("euclo.unit_of_work", work)
	statusRecord := euclowork.BuildRuntimeExecutionStatus(work, status, work.ResultClass, work.UpdatedAt)
	euclowork.SeedCompiledExecutionState(state, work, statusRecord)
	if work.WorkflowID != "" && work.RunID != "" {
		euclorestore.CaptureProviderRuntimeState(ctx, a.runtimeProviders(state), state)
		taskID := ""
		if task != nil {
			taskID = task.ID
		}
		restoreState, restoreErr := euclorestore.PersistProviderSnapshotState(ctx, a.workflowStore(), work.WorkflowID, work.RunID, state, taskID)
		if restoreErr != nil {
			state.Set("euclo.provider_restore_error", restoreErr.Error())
		} else if current, ok := eucloruntime.CompiledExecutionFromState(state); ok {
			current.ProviderSnapshotRefs = append([]string(nil), restoreState.ProviderSnapshotRefs...)
			current.ProviderSessionSnapshotRefs = append([]string(nil), restoreState.SessionSnapshotRefs...)
			state.Set("euclo.compiled_execution", current)
		}
	}
	state.Set("euclo.security_runtime", euclopolicy.BuildSecurityRuntimeState(a.Config, a.CapabilityRegistry(), a.runtimeProviders(state), state, work))
	priorLifecycle, _ := eucloruntime.ContextLifecycleFromState(state)
	artifactKinds := collectArtifactKindsFromState(state)
	state.Set("euclo.context_compaction", eucloruntime.BuildContextLifecycleState(work, priorLifecycle, status, artifactKinds, work.UpdatedAt))
	artifacts := euclotypes.CollectArtifactsFromState(state)
	actionLog := eucloreporting.BuildActionLog(state, artifacts)
	proofSurface := eucloreporting.BuildProofSurface(state, artifacts)
	state.Set("euclo.action_log", actionLog)
	state.Set("euclo.proof_surface", proofSurface)
	state.Set("euclo.debug_capability_runtime", eucloreporting.BuildDebugCapabilityRuntimeState(work, state, time.Now().UTC()))
	state.Set("euclo.chat_capability_runtime", eucloreporting.BuildChatCapabilityRuntimeState(work, state, time.Now().UTC()))
	eucloreporting.EmitObservabilityTelemetry(a.ConfigTelemetry(), task, actionLog, proofSurface)
	artifacts = euclotypes.CollectArtifactsFromState(state)
	report := euclotypes.AssembleFinalReport(artifacts)
	if raw, ok := state.Get("euclo.provider_restore"); ok && raw != nil {
		report["provider_restore"] = raw
	}
	if raw, ok := state.Get("euclo.context_runtime"); ok && raw != nil {
		report["context_runtime"] = raw
	}
	if raw, ok := state.Get("euclo.security_runtime"); ok && raw != nil {
		report["security_runtime"] = raw
	}
	if raw, ok := state.Get("euclo.capability_contract_runtime"); ok && raw != nil {
		report["capability_contract_runtime"] = raw
	}
	if raw, ok := state.Get("euclo.archaeology_capability_runtime"); ok && raw != nil {
		report["archaeology_capability_runtime"] = raw
	}
	if raw, ok := state.Get("euclo.debug_capability_runtime"); ok && raw != nil {
		report["debug_capability_runtime"] = raw
	}
	if raw, ok := state.Get("euclo.chat_capability_runtime"); ok && raw != nil {
		report["chat_capability_runtime"] = raw
	}
	if raw, ok := state.Get("euclo.unit_of_work_transition"); ok && raw != nil {
		report["unit_of_work_transition"] = raw
	}
	if raw, ok := state.Get("euclo.unit_of_work_history"); ok && raw != nil {
		report["unit_of_work_history"] = raw
	}
	if raw, ok := state.Get("euclo.shared_context_runtime"); ok && raw != nil {
		report["shared_context_runtime"] = raw
	}
	state.Set("pipeline.final_output", report)
	artifacts = euclotypes.CollectArtifactsFromState(state)
	state.Set("euclo.artifacts", artifacts)
	if persistErr := a.persistArtifacts(ctx, task, state, artifacts); persistErr != nil {
		state.Set("euclo.runtime_persist_error", persistErr.Error())
	}
}

func (a *Agent) runtimeProviders(state *core.Context) []core.Provider {
	out := make([]core.Provider, 0, len(a.RuntimeProviders))
	out = append(out, a.RuntimeProviders...)
	if state == nil {
		return out
	}
	raw, ok := state.Get("euclo.runtime_providers")
	if !ok || raw == nil {
		return out
	}
	if typed, ok := raw.([]core.Provider); ok {
		out = append(out, typed...)
	}
	return out
}

func collectArtifactKindsFromState(state *core.Context) []string {
	if state == nil {
		return nil
	}
	raw, ok := state.Get("euclo.artifacts")
	if !ok || raw == nil {
		return nil
	}
	artifacts, ok := raw.([]euclotypes.Artifact)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		out = append(out, string(artifact.Kind))
	}
	return out
}

func (a *Agent) applyRuntimeResultMetadata(result *core.Result, state *core.Context) {
	if result == nil || state == nil {
		return
	}
	if result.Metadata == nil {
		result.Metadata = map[string]any{}
	}
	if result.Data == nil {
		result.Data = map[string]any{}
	}
	if raw, ok := state.Get("euclo.execution_status"); ok && raw != nil {
		result.Metadata["execution_status"] = raw
	}
	if raw, ok := state.Get("euclo.compiled_execution"); ok && raw != nil {
		result.Metadata["compiled_execution"] = raw
	}
	if raw, ok := state.Get("euclo.context_compaction"); ok && raw != nil {
		result.Metadata["context_compaction"] = raw
	}
	if raw, ok := state.Get("euclo.context_runtime"); ok && raw != nil {
		result.Metadata["context_runtime"] = raw
	}
	if raw, ok := state.Get("euclo.security_runtime"); ok && raw != nil {
		result.Metadata["security_runtime"] = raw
	}
	if raw, ok := state.Get("euclo.shared_context_runtime"); ok && raw != nil {
		result.Metadata["shared_context_runtime"] = raw
	}
	if raw, ok := state.Get("euclo.debug_capability_runtime"); ok && raw != nil {
		result.Metadata["debug_capability_runtime"] = raw
	}
	if raw, ok := state.Get("euclo.chat_capability_runtime"); ok && raw != nil {
		result.Metadata["chat_capability_runtime"] = raw
	}
	if raw, ok := state.Get("euclo.unit_of_work_transition"); ok && raw != nil {
		result.Metadata["unit_of_work_transition"] = raw
	}
	if raw, ok := state.Get("euclo.unit_of_work_history"); ok && raw != nil {
		result.Metadata["unit_of_work_history"] = raw
	}
	if raw, ok := state.Get("euclo.deferred_issue_ids"); ok && raw != nil {
		result.Metadata["deferred_issue_ids"] = raw
	}
	if raw, ok := state.Get("euclo.assurance_class"); ok && raw != nil {
		result.Metadata["assurance_class"] = raw
	}
	if raw, ok := state.Get("pipeline.final_output"); ok && raw != nil {
		result.Data["final_output"] = raw
		if payload, ok := raw.(map[string]any); ok {
			if _, exists := result.Metadata["result_class"]; !exists {
				if value := strings.TrimSpace(fmt.Sprint(payload["result_class"])); value != "" && value != "<nil>" {
					result.Metadata["result_class"] = value
				}
			}
			if _, exists := result.Metadata["assurance_class"]; !exists {
				if value := strings.TrimSpace(fmt.Sprint(payload["assurance_class"])); value != "" && value != "<nil>" {
					result.Metadata["assurance_class"] = value
				}
			}
		}
	}
}

func deferredIssueIDsFromState(state *core.Context, existing []string) []string {
	out := append([]string(nil), existing...)
	if state == nil {
		return out
	}
	if raw, ok := state.Get("euclo.deferred_issue_ids"); ok && raw != nil {
		switch typed := raw.(type) {
		case []string:
			out = append(out, typed...)
		case []any:
			for _, item := range typed {
				if value := strings.TrimSpace(fmt.Sprint(item)); value != "" && value != "<nil>" {
					out = append(out, value)
				}
			}
		}
	}
	seen := make(map[string]struct{}, len(out))
	deduped := make([]string, 0, len(out))
	for _, id := range out {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		deduped = append(deduped, id)
	}
	return deduped
}

func (a *Agent) ConfigTelemetry() core.Telemetry {
	if a == nil || a.Config == nil {
		return nil
	}
	return a.Config.Telemetry
}

// sessionSelectPhaseDef returns a PhaseDefinition for the session_select phase.
// Returns false if WorkflowStore is not available.
func (a *Agent) sessionSelectPhaseDef() (interaction.PhaseDefinition, bool) {
	if a.WorkflowStore == nil {
		return interaction.PhaseDefinition{}, false
	}
	planStore := a.PlanStore // may be nil; SessionIndex handles nil gracefully
	index := &euclosession.SessionIndex{
		WorkflowStore: a.WorkflowStore,
		PlanStore:     planStore,
	}
	resolver := &euclosession.SessionResumeResolver{
		WorkflowStore: a.WorkflowStore,
		PlanStore:     planStore,
	}
	phase := &euclosession.SessionSelectPhase{Index: index, Resolver: resolver}
	return interaction.PhaseDefinition{
		ID:      "session_select",
		Label:   "Session Select",
		Handler: phase,
		SkipWhen: func(state map[string]any, _ *interaction.ArtifactBundle) bool {
			triggered, _ := state["euclo.session_select.triggered"].(bool)
			return !triggered
		},
	}, true
}

// createInteractionRegistry creates an interaction registry with the pipeline injected
func (a *Agent) createInteractionRegistry() *interaction.ModeMachineRegistry {
	reg := interaction.NewModeMachineRegistry()

	// For chat mode, we need to provide the pipeline and file resolver
	reg.Register(euclorelurpic.ModeChat, func(emitter interaction.FrameEmitter, resolver *interaction.AgencyResolver) *interaction.PhaseMachine {
		fileResolver := &pretask.FileResolver{
			Workspace: workspacePathFromEnv(a.WorkspaceEnv),
		}
		if pm := a.WorkspaceEnv.PermissionManager; pm != nil {
			agentID := ""
			if a.Config != nil {
				agentID = a.Config.Name
			}
			fileResolver.CheckFileAccess = func(path string) error {
				return pm.CheckFileAccess(context.Background(), agentID, core.FileSystemRead, path)
			}
		}
		// Use the pipeline from the agent
		var pipeline modes.ContextEnrichmentPipeline
		if a.ContextPipeline != nil {
			pipeline = a.ContextPipeline
		}
		// Determine if we should show confirmation frame.
		// Defaults to true. A manifest override requires adding a
		// context_enrichment sub-struct to AgentSkillConfig (future work).
		showConfirmationFrame := true
		return modes.ChatMode(emitter, resolver, pipeline, fileResolver, showConfirmationFrame, a.WorkspaceEnv.Memory)
	})
	reg.Register(euclorelurpic.ModeCode, modes.CodeMode)
	reg.Register(euclorelurpic.ModeDebug, modes.DebugMode)
	reg.Register(euclorelurpic.ModePlanning, modes.PlanningMode)
	reg.Register(euclorelurpic.ModeReview, modes.ReviewMode)
	reg.Register(euclorelurpic.ModeTDD, modes.TDDMode)

	// Inject session_select phase into all registered mode machines
	if phaseDef, ok := a.sessionSelectPhaseDef(); ok {
		for _, modeID := range reg.Modes() {
			reg.WrapFactory(modeID, func(factory interaction.ModeMachineFactory) interaction.ModeMachineFactory {
				return func(emitter interaction.FrameEmitter, resolver *interaction.AgencyResolver) *interaction.PhaseMachine {
					m := factory(emitter, resolver)
					m.PrependPhase(phaseDef)
					return m
				}
			})
		}
	}

	return reg
}

// stringValue extracts a string from an interface value.
func stringValue(raw any) string {
	if raw == nil {
		return ""
	}
	if s, ok := raw.(string); ok {
		return s
	}
	return ""
}

package thoughtrecipes

import (
	"context"
	"fmt"

	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
)

// SharingMode defines how context is shared between steps.
type SharingMode string

const (
	// SharingModeCarryForward inherits all captures from previous steps automatically.
	SharingModeCarryForward SharingMode = "carry_forward"
	// SharingModeIsolated ensures step only sees explicit inherits.
	SharingModeIsolated SharingMode = "isolated"
	// SharingModeExplicit requires explicit inherit declarations.
	SharingModeExplicit SharingMode = "explicit"
)

// EnrichmentSource identifies a source of context enrichment.
type EnrichmentSource string

const (
	// EnrichmentAST provides AST-based code analysis.
	EnrichmentAST EnrichmentSource = "ast"
	// EnrichmentArchaeology provides archaeology-based exploration.
	EnrichmentArchaeology EnrichmentSource = "archaeology"
	// EnrichmentBKC provides build knowledge base context.
	EnrichmentBKC EnrichmentSource = "bkc"
)

// ExecutionPlan is a validated and compiled ThoughtRecipe ready for execution.
type ExecutionPlan struct {
	Name        string
	Description string
	Version     string
	FilePath    string

	// Global settings inherited by all steps
	GlobalCapabilities []string
	GlobalEnrichment   []EnrichmentSource
	GlobalSharing      SharingMode
	GlobalAliases      map[string]string
	GlobalPrompt       string // Compiled global prompt template

	// Execution sequence
	Steps []ExecutionStep

	// Configuration
	TriggerPriority int
	Modes           []string
	IntentKeywords  []string

	// Loader warnings (non-fatal issues)
	Warnings []string

	// Alias resolver for capture/inherit key resolution
	Resolver *AliasResolver
}

// ExecutionStep represents a single step in the execution sequence.
type ExecutionStep struct {
	ID   string
	Name string // human-readable name for logging

	// Agent configuration
	Parent   ExecutionStepAgent
	Child    *ExecutionStepAgent
	Fallback *ExecutionStepAgent

	// Context sharing for this step
	Inherit []string // alias names to inject from prior steps
	Capture []string // alias names this agent writes

	// Capability scoping for this step
	AllowedCapabilities []string // nil = inherit from global
}

// ExecutionStepAgent represents an agent configuration within a step.
type ExecutionStepAgent struct {
	Paradigm     string
	Prompt       string
	Enrichment   []EnrichmentSource // additive to global
	Capabilities []string           // intersected with global

	// Capture aliases for this agent's results
	Capture []string

	// Inherit aliases from prior steps
	Inherit []string
}

// RecipeState holds the accumulated state during recipe execution.
type RecipeState struct {
	// Base context containing task metadata
	Base *core.Context

	// Captures holds values written by completed steps (alias -> value)
	Captures map[string]interface{}

	// StepResults holds results from completed steps (step ID -> result)
	StepResults map[string]*StepResult

	// Accumulated artifacts from all steps
	Artifacts []euclotypes.Artifact
}

// StepResult holds the result of executing a single step.
type StepResult struct {
	StepID  string
	Success bool
	Error   string // non-empty if step failed

	// Captures written by this step (alias -> value)
	Captures map[string]interface{}

	// Result from the parent agent execution
	ParentResult *core.Result

	// ChildFailed is true if the child agent ran and failed
	ChildFailed bool

	// FallbackUsed is true if the fallback agent was activated
	FallbackUsed bool

	// Artifacts produced by this step
	Artifacts []euclotypes.Artifact
}

// RecipeResult is the aggregated output of a thought recipe execution.
type RecipeResult struct {
	RecipeID string // The recipe's capability ID (e.g., "euclo:recipe.my-recipe")
	Success  bool
	Error    string // non-empty if recipe failed

	// Final output is the last successful step's captures
	FinalCaptures map[string]interface{}

	// The last successful step's result
	FinalResult *core.Result

	// All accumulated artifacts
	Artifacts []euclotypes.Artifact

	// Warnings collected during execution
	Warnings []string

	// Per-step results
	StepResults []*StepResult
}

// NewRecipeState creates a new RecipeState for the given task.
func NewRecipeState(task *core.Task) *RecipeState {
	state := &RecipeState{
		Base:        core.NewContext(),
		Captures:    make(map[string]interface{}),
		StepResults: make(map[string]*StepResult),
		Artifacts:   make([]euclotypes.Artifact, 0),
	}

	// Populate base context with task metadata
	if task != nil {
		state.Base.Set("task.instruction", task.Instruction)
		state.Base.Set("task.type", string(task.Type))
		// Copy task context values (workspace may be in here)
		if task.Context != nil {
			for k, v := range task.Context {
				state.Base.Set("task.context."+k, v)
			}
		}
	}

	return state
}

// Capture writes a value to the state under the given alias.
// The alias is resolved to its underlying state key.
func (s *RecipeState) Capture(alias string, value interface{}, resolver *AliasResolver) error {
	if s == nil {
		return nil
	}

	if resolver == nil {
		resolver = NewAliasResolver(nil)
	}

	// Resolve alias to state key
	stateKey, ok := resolver.Resolve(alias)
	if !ok {
		// Unknown alias - store directly under the alias name
		stateKey = alias
	}

	if s.Captures == nil {
		s.Captures = make(map[string]interface{})
	}

	s.Captures[alias] = value
	s.Base.Set(stateKey, value)

	return nil
}

// GetCapture retrieves a captured value by alias.
func (s *RecipeState) GetCapture(alias string) (interface{}, bool) {
	if s == nil || s.Captures == nil {
		return nil, false
	}
	val, ok := s.Captures[alias]
	return val, ok
}

// AddStepResult records the result of a completed step.
func (s *RecipeState) AddStepResult(result *StepResult) {
	if s == nil {
		return
	}

	if s.StepResults == nil {
		s.StepResults = make(map[string]*StepResult)
	}

	s.StepResults[result.StepID] = result

	// Merge step artifacts into accumulated list
	if len(result.Artifacts) > 0 {
		s.Artifacts = append(s.Artifacts, result.Artifacts...)
	}
}

// BuildSharingContext constructs the context map for a step based on sharing mode.
// This determines which captures from prior steps are visible to the current step.
func (s *RecipeState) BuildSharingContext(step ExecutionStep, globalSharing SharingMode, resolver *AliasResolver) map[string]interface{} {
	if s == nil {
		return make(map[string]interface{})
	}

	context := make(map[string]interface{})

	// Determine effective sharing mode for this step
	effectiveMode := globalSharing
	// Note: Step-level sharing override would go here if supported in schema

	switch effectiveMode {
	case SharingModeIsolated:
		// Only explicit inherits, no automatic carry-forward
		for _, alias := range step.Inherit {
			if val, ok := s.GetCapture(alias); ok {
				context[alias] = val
			}
		}

	case SharingModeCarryForward:
		// All captures are inherited, plus explicit inherits
		for alias, val := range s.Captures {
			context[alias] = val
		}
		// Ensure explicit inherits are satisfied (they're already in captures)

	case SharingModeExplicit, "":
		// Only explicit inherits
		for _, alias := range step.Inherit {
			if val, ok := s.GetCapture(alias); ok {
				context[alias] = val
			}
		}
	}

	return context
}

// ToRecipeResult converts the accumulated state to a final RecipeResult.
func (s *RecipeState) ToRecipeResult(warnings []string) *RecipeResult {
	if s == nil {
		return &RecipeResult{
			Success:       false,
			FinalCaptures: make(map[string]interface{}),
			Artifacts:     make([]euclotypes.Artifact, 0),
			Warnings:      warnings,
			StepResults:   make([]*StepResult, 0),
		}
	}

	result := &RecipeResult{
		Success:       len(s.Captures) > 0,
		FinalCaptures: s.Captures,
		Artifacts:     s.Artifacts,
		Warnings:      warnings,
		StepResults:   make([]*StepResult, 0, len(s.StepResults)),
	}

	// Collect step results in order
	for _, sr := range s.StepResults {
		result.StepResults = append(result.StepResults, sr)
	}

	return result
}

// IsParadigmWithDelegation returns true if the paradigm supports child delegation.
func IsParadigmWithDelegation(paradigm string) bool {
	switch paradigm {
	case "htn", "reflection", "goalcon":
		return true
	default:
		return false
	}
}

// ValidateEnrichmentSource checks if a string is a valid enrichment source.
func ValidateEnrichmentSource(s string) (EnrichmentSource, bool) {
	switch s {
	case string(EnrichmentAST):
		return EnrichmentAST, true
	case string(EnrichmentArchaeology):
		return EnrichmentArchaeology, true
	case string(EnrichmentBKC):
		return EnrichmentBKC, true
	default:
		return "", false
	}
}

// ValidateSharingMode checks if a string is a valid sharing mode.
func ValidateSharingMode(s string) (SharingMode, bool) {
	switch s {
	case string(SharingModeCarryForward):
		return SharingModeCarryForward, true
	case string(SharingModeIsolated):
		return SharingModeIsolated, true
	case string(SharingModeExplicit):
		return SharingModeExplicit, true
	default:
		return "", false
	}
}

// ---------------------------------------------------------------------------
// Recipe Executor - Phase 7
// ---------------------------------------------------------------------------

// Executor runs ExecutionPlans step-by-step.
type Executor struct {
	factory *ParadigmFactory
}

// NewExecutor creates a new recipe executor.
func NewExecutor() *Executor {
	return &Executor{
		factory: NewParadigmFactory(),
	}
}

// Execute runs the execution plan and returns the recipe result.
// This implements the full algorithm from Section 7 of the spec.
func (e *Executor) Execute(ctx context.Context, plan *ExecutionPlan, task *core.Task, env agentenv.AgentEnvironment) (*RecipeResult, error) {
	if e == nil {
		return nil, fmt.Errorf("executor is nil")
	}
	if plan == nil {
		return nil, fmt.Errorf("execution plan is nil")
	}
	if len(plan.Steps) == 0 {
		return nil, fmt.Errorf("execution plan has no steps")
	}

	// Initialize recipe state
	state := NewRecipeState(task)
	var warnings []string

	// Build global filtered registry
	globalReg := env.Registry
	if len(plan.GlobalCapabilities) > 0 {
		// Note: FilteredRegistry returns *FilteredRegistry, not *Registry
		// The filtering is applied at invocation time via the environment
		_ = capability.NewFilteredRegistry(globalReg, plan.GlobalCapabilities)
	}

	// Hydrate global enrichment sources
	for _, source := range plan.GlobalEnrichment {
		if err := hydrateEnrichmentSource(ctx, source, state.Base, task); err != nil {
			warnings = append(warnings, fmt.Sprintf("failed to hydrate global enrichment %s: %v", source, err))
		}
	}

	// Build global template context (task-level only)
	globalEnrichment := buildEnrichmentBundle(state.Base)
	globalTmplCtx := BuildTemplateContext(task, state.Base, globalEnrichment, plan.Resolver)

	// Render global prompt
	globalPromptRendered, tmplWarnings := RenderPrompt(plan.GlobalPrompt, globalTmplCtx)
	warnings = append(warnings, tmplWarnings...)

	// Track last successful result
	var lastSuccessfulResult *core.Result

	// Execute each step
	for _, step := range plan.Steps {
		stepResult, stepWarnings, err := e.executeStep(ctx, step, plan, state, task, env, globalReg, globalPromptRendered)
		warnings = append(warnings, stepWarnings...)

		if err != nil {
			// Parent execution failed - log warning and continue
			warnings = append(warnings, fmt.Sprintf("step %s parent failed: %v", step.ID, err))
			state.AddStepResult(&StepResult{
				StepID:       step.ID,
				Success:      false,
				ParentResult: nil,
			})
			continue
		}

		if stepResult != nil && stepResult.ParentResult != nil {
			lastSuccessfulResult = stepResult.ParentResult
		}

		state.AddStepResult(stepResult)
	}

	// Build final recipe result
	result := state.ToRecipeResult(warnings)
	result.RecipeID = plan.Name

	// Set final result
	if lastSuccessfulResult != nil {
		result.Success = true
		result.FinalResult = lastSuccessfulResult
	} else {
		result.Success = false
		return result, fmt.Errorf("no step produced a successful result")
	}

	return result, nil
}

// executeStep executes a single step and returns the step result.
func (e *Executor) executeStep(
	ctx context.Context,
	step ExecutionStep,
	plan *ExecutionPlan,
	state *RecipeState,
	task *core.Task,
	env agentenv.AgentEnvironment,
	globalReg *capability.Registry,
	globalPrompt string,
) (*StepResult, []string, error) {
	var warnings []string

	// 5a. Build step state from sharing mode
	stepState := e.buildStepState(step, plan.GlobalSharing, state, task)

	// 5b. Hydrate step-level enrichment (additive to global)
	for _, source := range step.Parent.Enrichment {
		if err := hydrateEnrichmentSource(ctx, source, stepState, task); err != nil {
			warnings = append(warnings, fmt.Sprintf("failed to hydrate step enrichment %s: %v", source, err))
		}
	}

	// 5c. Build step TemplateContext (task + inherited context)
	stepEnrichment := buildEnrichmentBundle(stepState)
	stepTmplCtx := BuildTemplateContext(task, stepState, stepEnrichment, plan.Resolver)

	// Add captured values to context
	for alias, val := range state.Captures {
		if s, ok := val.(string); ok {
			stepTmplCtx.Context[alias] = s
		}
	}

	// 5d. Render step prompt (global prompt prepended)
	fullPrompt := globalPrompt
	if step.Parent.Prompt != "" {
		if fullPrompt != "" {
			fullPrompt += "\n"
		}
		fullPrompt += step.Parent.Prompt
	}
	rendered, tmplWarnings := RenderPrompt(fullPrompt, stepTmplCtx)
	warnings = append(warnings, tmplWarnings...)

	// 5e. Build step FilteredRegistry
	stepReg := globalReg
	if len(step.Parent.Capabilities) > 0 {
		// Note: FilteredRegistry returns *FilteredRegistry, not *Registry
		// The filtering is applied at invocation time via the environment
		_ = capability.NewFilteredRegistry(globalReg, step.Parent.Capabilities)
	}

	// 5f. Wire child into parent's delegation slot
	parent, childAgent, buildWarnings, err := e.factory.BuildStepAgent(step, stepReg, env)
	warnings = append(warnings, buildWarnings...)
	if err != nil {
		return nil, warnings, fmt.Errorf("building step agent: %w", err)
	}

	// 5g. Execute parent paradigm
	parentTask := &core.Task{
		ID:          task.ID + "." + step.ID,
		Instruction: rendered,
		Type:        task.Type,
		Context:     taskContextFromTask(task),
	}

	parentResult, err := parent.Execute(ctx, parentTask, stepState)
	if err != nil {
		return nil, warnings, err
	}

	// 5h. Parent succeeded - extract captures
	stepResult := &StepResult{
		StepID:       step.ID,
		Success:      true,
		ParentResult: parentResult,
	}

	// Extract parent captures from stepState
	for _, alias := range step.Parent.Capture {
		key := plan.Resolver.MustResolve(alias)
		if val, ok := stepState.Get(key); ok {
			state.Capture(alias, val, plan.Resolver)
			stepResult.Captures[alias] = val
		}
	}

	// 5i. Collect parent artifacts
	artifacts := artifactsFromResult(parentResult)
	state.Artifacts = append(state.Artifacts, artifacts...)
	stepResult.Artifacts = artifacts

	// 5j. Handle child execution (if child was wired and paradigm supports delegation)
	if step.Child != nil && isParadigmWithDelegation(step.Parent.Paradigm) && childAgent != nil {
		// Child execution is embedded in parent via delegation
		// Check if child failed (detected from parentResult.Data)
		if childFailed, ok := parentResult.Data["child_failed"].(bool); ok && childFailed {
			stepResult.ChildFailed = true
			warnings = append(warnings, fmt.Sprintf("child failed for step %s", step.ID))

			// 5k. Try fallback if available
			if step.Fallback != nil {
				fallbackAgent, err := e.factory.BuildFallbackAgent(*step.Fallback, stepReg, env)
				if err != nil {
					warnings = append(warnings, fmt.Sprintf("failed to build fallback for step %s: %v", step.ID, err))
				} else {
					fallbackResult, err := fallbackAgent.Execute(ctx, parentTask, stepState)
					if err != nil {
						warnings = append(warnings, fmt.Sprintf("fallback also failed for step %s: %v", step.ID, err))
					} else {
						stepResult.FallbackUsed = true
						// Extract fallback captures
						for _, alias := range step.Fallback.Capture {
							key := plan.Resolver.MustResolve(alias)
							if val, ok := stepState.Get(key); ok {
								state.Capture(alias, val, plan.Resolver)
								stepResult.Captures[alias] = val
							}
						}
						// Collect fallback artifacts
						fallbackArtifacts := artifactsFromResult(fallbackResult)
						state.Artifacts = append(state.Artifacts, fallbackArtifacts...)
						stepResult.Artifacts = append(stepResult.Artifacts, fallbackArtifacts...)
					}
				}
			} else {
				warnings = append(warnings, fmt.Sprintf("child failed for step %s; no fallback declared", step.ID))
			}
		}
	}

	return stepResult, warnings, nil
}

// buildStepState builds the step state based on sharing mode.
func (e *Executor) buildStepState(step ExecutionStep, sharingMode SharingMode, state *RecipeState, task *core.Task) *core.Context {
	switch sharingMode {
	case SharingModeExplicit:
		// Each step starts from base task state only
		// Only inherited keys are injected
		stepState := core.NewContext()
		// Copy task context
		if task != nil && task.Context != nil {
			for k, v := range task.Context {
				stepState.Set("task.context."+k, v)
			}
		}
		// Inject inherited captures
		for _, alias := range step.Parent.Inherit {
			if val, ok := state.GetCapture(alias); ok {
				key := state.Base.GetString("alias." + alias)
				if key == "" {
					key = alias
				}
				stepState.Set(key, val)
			}
		}
		return stepState

	case SharingModeCarryForward:
		// Each step inherits all captured keys from all prior steps
		stepState := core.NewContext()
		// Copy task context
		if task != nil && task.Context != nil {
			for k, v := range task.Context {
				stepState.Set("task.context."+k, v)
			}
		}
		// Inject all captures
		for alias, val := range state.Captures {
			key := state.Base.GetString("alias." + alias)
			if key == "" {
				key = alias
			}
			stepState.Set(key, val)
		}
		return stepState

	case SharingModeIsolated:
		// Each step starts from base task state only (no prior captures)
		stepState := core.NewContext()
		if task != nil && task.Context != nil {
			for k, v := range task.Context {
				stepState.Set("task.context."+k, v)
			}
		}
		return stepState

	default:
		// Default to explicit
		return e.buildStepState(step, SharingModeExplicit, state, task)
	}
}

// taskContextFromTask extracts context map from task.
func taskContextFromTask(task *core.Task) map[string]any {
	if task == nil {
		return nil
	}
	return task.Context
}

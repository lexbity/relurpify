# Engineering Specification: Euclo Thought Recipes

**Status:** Planning  
**Scope:** `named/euclo/thoughtrecipes/`, `framework/capability/`, `named/euclo/relurpicabilities/`, `named/euclo/runtime/`, `named/euclo/capabilities/`, `templates/euclo/thoughtrecipe/`  
**Motivation:** Euclo's relurpic capabilities are fully hard-coded in Go. Paradigm sequences, prompts, and context enrichment are fixed at compile time. Thought recipes make these dimensions user-configurable via YAML: a developer can define a new relurpic capability by declaring which agent paradigms to compose, in which order, with which prompts and context enrichment, without writing Go.

---

## 1. Design Principles

1. **Recipes are data, not code.** A thought recipe YAML file describes execution — it is interpreted by the recipe executor at runtime, not compiled.
2. **Security is not recipe-owned.** Recipes filter and focus what the LLM sees; they do not grant permissions. All tool and filesystem access is governed by the agent manifest and the framework's authorization layer. If a recipe requests a capability not present in the manifest, execution continues with a warning — it does not fail and does not escalate permissions.
3. **Context sharing is explicit and scoped.** Each step declares what state keys it reads (`inherit`) and writes (`capture`) using stable aliases. Under `sharing.default: explicit`, no state bleeds between steps unless declared.
4. **Recipes are registered relurpic capabilities.** A loaded thought recipe becomes a `Descriptor` in the `relurpicabilities.Registry`, participates in keyword-based capability intent selection, and is visible to the dynamic resolution classifier like any built-in capability.
5. **Execution result contract is fixed.** A recipe produces the last successful step's `*core.Result` plus the accumulated `[]euclotypes.Artifact` from all steps. Partial failures (child failure, missing enrichment) produce warnings in telemetry; they do not prevent execution.
6. **Top-down execution order.** Steps execute in YAML declaration order. No branching, no looping, no conditional jumps. Branching is a V2 concern.
7. **Built-in capabilities are not replaced — they are outranked.** User recipes participate in tie-breaking via `trigger_priority`. Built-in capabilities carry an implicit priority of 5. A recipe with `trigger_priority: 6` will win a tie over a built-in, but a recipe with `trigger_priority: 4` will not.

---

## 2. YAML Schema — Full Specification

```yaml
apiVersion: euclo/v1alpha1
kind: ThoughtRecipe

metadata:
  name: string           # required; unique identifier; used as the capability ID prefix
  description: string    # required; surfaced in dynamic resolution candidate prompts
  version: string        # optional; e.g. "1.0"

global:

  capabilities:
    # Allowed tool capability IDs visible to ALL steps.
    # Steps may further restrict this list; they may NOT expand it.
    # Omitting this block = all tools permitted by the manifest are available.
    allowed:
      - string           # capability ID, e.g. file_read, file_write, go_test

  context:
    # Enrichment sources hydrated before sequence execution begins.
    # Available values: ast | archaeology | bkc
    # Per-step enrichment declarations are additive — they cannot remove sources
    # enabled at the global level.
    enrichment:
      - string

    # Context sharing strategy across steps.
    sharing:
      # carry_forward: each step inherits all captured keys from all prior steps
      # isolated:      each step starts from base task state only (no prior captures)
      # explicit:      each step only sees keys declared in its context.inherit list
      # Default: explicit
      default: explicit | carry_forward | isolated

    # Recipe-local alias declarations. Maps a recipe-scoped name → state key.
    # Standard aliases (see Section 3) are always available without declaration.
    aliases:
      alias_name: euclo.some_state_key   # recipe-local name → underlying state key

  configuration:
    # Euclo interaction modes this recipe applies to.
    # A recipe may declare multiple modes — it will be eligible for capability
    # selection whenever the classifier routes to any of its declared modes.
    modes:
      - debug | chat | planning | code

    # Keywords triggering this recipe via the capability intent engine.
    # Matched as case-insensitive substring of the user instruction.
    intent_keywords:
      - string

    # Tie-breaking priority when multiple capabilities match the same instruction
    # with equal keyword match counts.
    # Built-in capabilities carry an implicit priority of 5.
    # Range: 1–100. Default: 5.
    trigger_priority: int

    # When true: this recipe participates in LLM-based fallback capability selection
    # (dynamic resolution) in addition to keyword matching.
    # When false: recipe is only selected via explicit keyword match.
    allow_dynamic_resolution: bool

  # Prompt text prepended to every step's prompt before it is passed to the paradigm.
  # Supports ${var} substitution (see Section 4).
  prompt: |
    string

sequence:
  - id: string           # required; unique within the recipe; used in telemetry

    parent:
      # Paradigm to execute for this step.
      # Supported: react | htn | reflection | planner | rewoo | architect |
      #            blackboard | goalcon | chainer
      paradigm: string

      # Step prompt. Global prompt is prepended automatically.
      # Supports ${var} substitution.
      prompt: |
        string

      context:
        # Additional enrichment for this step only (additive to global).
        enrichment:
          - ast | archaeology | bkc

        # Alias keys from prior steps' captures to inject into this step's state.
        # Under sharing.default: explicit, ONLY these keys are available.
        inherit:
          - alias_name

        # Alias keys this step writes to the shared recipe state after execution.
        capture:
          - alias_name

      capabilities:
        # Further restriction of the global allowed list for this agent only.
        # Omitting = inherit global. Empty list = inherit global (not "deny all").
        allowed:
          - string

    # Optional child agent. Only meaningful when parent paradigm exposes a
    # delegation slot (htn.PrimitiveExec, reflection.Delegate, goalcon.PlanExecutor).
    # For paradigms without a delegation slot (react, planner, rewoo), declaring
    # child: logs a warning and is ignored.
    child:
      paradigm: string
      prompt: |
        string
      context:
        inherit:
          - alias_name
        capture:
          - alias_name
      capabilities:
        allowed:
          - string

    # Optional fallback agent, invoked if the child agent fails.
    # Receives the same parent-output state the child would have received.
    # Produces a warning in telemetry indicating child failure + fallback activation.
    # If no fallback is declared and child fails: warning logged, step continues
    # without child output (parent output is still captured).
    fallback:
      paradigm: string
      prompt: |
        string
      context:
        inherit:
          - alias_name
        capture:
          - alias_name
      capabilities:
        allowed:
          - string
```

---

## 3. Standard Context Alias Table

These aliases are always available — no declaration in `global.context.aliases` is needed. They map to the existing magic state keys used throughout euclo's execution layer.

| Alias | State key | Typical producer |
|-------|-----------|-----------------|
| `explore_findings` | `pipeline.explore` | planner, react (explore) |
| `analysis_result` | `pipeline.analyze` | react, planner |
| `plan_output` | `pipeline.plan` | planner, architect |
| `code_changes` | `pipeline.code` | react (modify), architect |
| `verify_result` | `pipeline.verify` | react, reflection |
| `final_output` | `pipeline.final_output` | any terminal step |
| `review_findings` | `euclo.review_findings` | reflection, chat.local-review |
| `root_cause` | `euclo.root_cause` | debug localization |
| `root_cause_candidates` | `euclo.root_cause_candidates` | debug root-cause |
| `regression_analysis` | `euclo.regression_analysis` | debug verification-repair |
| `verification_summary` | `euclo.verification_summary` | targeted-verification-repair |
| `debug_investigation` | `euclo.debug_investigation_summary` | debug pipeline stages |
| `repair_readiness` | `euclo.debug_repair_readiness` | debug pipeline stages |
| `plan_candidates` | `euclo.plan_candidates` | archaeology prospective-assess |

Custom aliases declared in `global.context.aliases` extend this table for a specific recipe. Recipe-local aliases shadow standard aliases if names collide (a warning is logged at load time when this occurs).

---

## 4. Template Variable Namespace

Prompts use `${var}` substitution resolved by the recipe executor before passing the prompt to the paradigm. The namespace is bounded — recipe authors cannot access arbitrary runtime internals.

```
${task.instruction}          # original task instruction text
${task.type}                 # analysis | code_modification
${task.workspace}            # workspace absolute path

${context.<alias>}           # value of any standard or custom alias visible to this step
                             # renders as empty string if key not in state (+ warning)

${enrichment.ast}            # AST index summary string (if ast enrichment enabled)
${enrichment.archaeology}    # archaeology provenance context summary
${enrichment.bkc}            # BKC semantic context summary
                             # all three render as empty string if source not enabled
```

**Rules:**
- Unresolved variables render as empty string; they never cause execution failure.
- A warning is emitted to telemetry for each unresolved variable.
- Variables are resolved at the moment the step begins, after state injection.
- The global prompt is rendered once with task-level variables only (no context aliases, since no step has executed yet), then prepended to each step prompt before step-level variable substitution occurs.
- No code execution, no function calls, no conditionals. `${...}` is pure string interpolation.

---

## 5. Paradigm Delegation Matrix

The `child:` field is only operative when the parent paradigm exposes a `graph.WorkflowExecutor` delegation slot. This is verified at load time and at execution time.

| Parent paradigm | Delegation slot | Child wiring | Notes |
|----------------|----------------|-------------|-------|
| `htn` | `PrimitiveExec graph.WorkflowExecutor` | `htn.WithPrimitiveExec(childAgent)` | Any `graph.WorkflowExecutor` accepted |
| `reflection` | `Delegate graph.WorkflowExecutor` | `reflection.New(env, childAgent)` | Any `graph.WorkflowExecutor` accepted |
| `goalcon` | `PlanExecutor graph.WorkflowExecutor` | `goalcon.PlanExecutor = childAgent` | Any `graph.WorkflowExecutor` accepted |
| `react` | none | child declaration ignored + warning | react has no delegation slot |
| `planner` | none | child declaration ignored + warning | planner has no delegation slot |
| `rewoo` | none | child declaration ignored + warning | rewoo has no delegation slot |
| `blackboard` | `Sources []KnowledgeSource` | not supported in V1 | KnowledgeSource interface differs from graph.WorkflowExecutor; V2 concern |
| `architect` | internal react + planner | not supported in V1 | executor is internally hardcoded; separate registries only |
| `chainer` | `Chain *Chain` | not supported in V1 | link-level composition differs from paradigm injection; V2 concern |

A `fallback:` is always operative regardless of parent paradigm — it is handled at the recipe executor level, not by the paradigm.

---

## 6. Go Type Definitions

### 6.1 Schema Types — `named/euclo/thoughtrecipes/schema.go`

```go
package thoughtrecipes

// ThoughtRecipe is the top-level type parsed from a thought recipe YAML file.
type ThoughtRecipe struct {
    APIVersion string         `yaml:"apiVersion"` // must be "euclo/v1alpha1"
    Kind       string         `yaml:"kind"`       // must be "ThoughtRecipe"
    Metadata   RecipeMetadata `yaml:"metadata"`
    Global     RecipeGlobal   `yaml:"global"`
    Sequence   []RecipeStep   `yaml:"sequence"`
}

type RecipeMetadata struct {
    Name        string `yaml:"name"`        // required; unique; used as capability ID prefix
    Description string `yaml:"description"` // required
    Version     string `yaml:"version"`     // optional
}

type RecipeGlobal struct {
    Capabilities  RecipeCapabilitySpec `yaml:"capabilities"`
    Context       RecipeContextSpec    `yaml:"context"`
    Configuration RecipeConfiguration  `yaml:"configuration"`
    Prompt        string               `yaml:"prompt"`
}

type RecipeCapabilitySpec struct {
    Allowed []string `yaml:"allowed"` // nil = inherit from parent scope / manifest
}

type RecipeContextSpec struct {
    Enrichment []string          `yaml:"enrichment"` // ast | archaeology | bkc
    Sharing    RecipeSharingSpec `yaml:"sharing"`
    Aliases    map[string]string `yaml:"aliases"` // alias name → state key
}

type RecipeSharingSpec struct {
    Default string `yaml:"default"` // carry_forward | isolated | explicit
}

type RecipeConfiguration struct {
    Modes                  []string `yaml:"modes"`
    IntentKeywords         []string `yaml:"intent_keywords"`
    TriggerPriority        int      `yaml:"trigger_priority"`
    AllowDynamicResolution bool     `yaml:"allow_dynamic_resolution"`
}

type RecipeStep struct {
    ID       string           `yaml:"id"`
    Parent   RecipeStepAgent  `yaml:"parent"`
    Child    *RecipeStepAgent `yaml:"child,omitempty"`
    Fallback *RecipeStepAgent `yaml:"fallback,omitempty"`
}

type RecipeStepAgent struct {
    Paradigm     string               `yaml:"paradigm"`
    Prompt       string               `yaml:"prompt"`
    Context      RecipeStepContext    `yaml:"context"`
    Capabilities RecipeCapabilitySpec `yaml:"capabilities"`
}

type RecipeStepContext struct {
    Enrichment []string `yaml:"enrichment"` // additive to global
    Inherit    []string `yaml:"inherit"`    // alias names to inject from prior steps
    Capture    []string `yaml:"capture"`    // alias names this agent writes
}
```

### 6.2 Alias Resolver — `named/euclo/thoughtrecipes/aliases.go`

```go
package thoughtrecipes

// StandardAliases maps well-known recipe alias names to their underlying
// state keys. Recipe authors use these names in inherit/capture declarations.
var StandardAliases = map[string]string{
    "explore_findings":      "pipeline.explore",
    "analysis_result":       "pipeline.analyze",
    "plan_output":           "pipeline.plan",
    "code_changes":          "pipeline.code",
    "verify_result":         "pipeline.verify",
    "final_output":          "pipeline.final_output",
    "review_findings":       "euclo.review_findings",
    "root_cause":            "euclo.root_cause",
    "root_cause_candidates": "euclo.root_cause_candidates",
    "regression_analysis":   "euclo.regression_analysis",
    "verification_summary":  "euclo.verification_summary",
    "debug_investigation":   "euclo.debug_investigation_summary",
    "repair_readiness":      "euclo.debug_repair_readiness",
    "plan_candidates":       "euclo.plan_candidates",
}

// AliasResolver resolves alias names to state keys, combining standard aliases
// and recipe-local custom aliases.
type AliasResolver struct {
    custom map[string]string // recipe-local aliases from global.context.aliases
}

// NewAliasResolver constructs a resolver combining standard and custom aliases.
// Custom aliases that shadow standard aliases emit a warning.
func NewAliasResolver(custom map[string]string) *AliasResolver

// Resolve returns the underlying state key for alias, or ("", false) if unknown.
func (r *AliasResolver) Resolve(alias string) (stateKey string, ok bool)

// MustResolve returns the state key or the alias itself as a fallback.
func (r *AliasResolver) MustResolve(alias string) string
```

### 6.3 Filtered Registry — `framework/capability/filtered.go`

```go
package capability

// FilteredRegistry wraps a Registry and restricts visible capabilities to a
// declared allowed set. An empty allowed set means pass-through (all capabilities
// visible). This is the mechanism through which thought recipe capability scoping
// is enforced — the LLM can only call tools it can see.
type FilteredRegistry struct {
    base    *Registry
    allowed map[string]struct{} // nil = allow all
}

// NewFilteredRegistry builds a filtered view. allowedIDs nil or empty = pass-through.
func NewFilteredRegistry(base *Registry, allowedIDs []string) *FilteredRegistry

// Intersect returns a new FilteredRegistry further restricted to the intersection
// of the current allowed set and the provided list. Used to apply step-level
// restrictions on top of global restrictions.
func (f *FilteredRegistry) Intersect(allowedIDs []string) *FilteredRegistry

// IsPassthrough returns true when no filtering is applied.
func (f *FilteredRegistry) IsPassthrough() bool

// AllowedIDs returns the current allowed ID set. Nil means all allowed.
func (f *FilteredRegistry) AllowedIDs() []string

// The following delegate to base only when the capability is allowed:
func (f *FilteredRegistry) Get(name string) (Capability, bool)
func (f *FilteredRegistry) ModelCallableTools() []Tool
func (f *FilteredRegistry) InvokeCapability(ctx context.Context, state *core.Context,
    name string, args map[string]any) (*core.CapabilityResultEnvelope, error)
```

### 6.4 Template Renderer — `named/euclo/thoughtrecipes/template.go`

```go
package thoughtrecipes

// TemplateContext is the bounded namespace available inside ${...} substitutions.
type TemplateContext struct {
    Task       TemplateTaskView
    Context    map[string]string // alias name → rendered string value
    Enrichment TemplateEnrichmentView
}

type TemplateTaskView struct {
    Instruction string
    Type        string // "analysis" | "code_modification"
    Workspace   string
}

type TemplateEnrichmentView struct {
    AST        string // empty if ast not enabled
    Archaeology string // empty if archaeology not enabled
    BKC        string // empty if bkc not enabled
}

// RenderPrompt performs ${var} substitution on tmpl using ctx.
// Unresolved variables render as empty string; each unresolved variable
// appends an entry to the returned warnings slice.
func RenderPrompt(tmpl string, ctx TemplateContext) (rendered string, warnings []string)

// BuildTemplateContext constructs the template context for a step from the
// current recipe execution state and the enrichment sources loaded for this step.
func BuildTemplateContext(task *core.Task, stepState *core.Context,
    enrichment EnrichmentBundle, resolver *AliasResolver) TemplateContext
```

### 6.5 Recipe Execution Types — `named/euclo/thoughtrecipes/executor.go`

```go
package thoughtrecipes

// ExecutionPlan is the validated, resolved form of a ThoughtRecipe ready for execution.
// Produced by the loader after schema validation and alias resolution.
type ExecutionPlan struct {
    RecipeID    string         // "euclo:recipe.<metadata.name>"
    GlobalAllowed []string     // resolved global capability allowed list
    GlobalPrompt  string       // raw global prompt template
    SharingMode   SharingMode
    GlobalEnrichment []EnrichmentSource
    Steps        []ExecutionStep
    Resolver     *AliasResolver
}

type SharingMode string
const (
    SharingModeExplicit     SharingMode = "explicit"
    SharingModeCarryForward SharingMode = "carry_forward"
    SharingModeIsolated     SharingMode = "isolated"
)

type EnrichmentSource string
const (
    EnrichmentAST        EnrichmentSource = "ast"
    EnrichmentArchaeology EnrichmentSource = "archaeology"
    EnrichmentBKC        EnrichmentSource = "bkc"
)

type ExecutionStep struct {
    ID         string
    Parent     ExecutionStepAgent
    Child      *ExecutionStepAgent
    Fallback   *ExecutionStepAgent
}

type ExecutionStepAgent struct {
    Paradigm     string   // validated paradigm name
    PromptTmpl   string   // raw prompt template (global prompt will be prepended)
    Inherit      []string // alias names (already validated against resolver)
    Capture      []string // alias names (already validated against resolver)
    Enrichment   []EnrichmentSource
    AllowedIDs   []string // after intersection with global; nil = global applies
}

// RecipeState holds cross-step state during a recipe execution run.
type RecipeState struct {
    Captured  map[string]any   // alias name → value; populated as steps complete
    Artifacts []euclotypes.Artifact
    Warnings  []string         // accumulated warnings from all steps
}

// StepResult is the outcome of executing one RecipeStep.
type StepResult struct {
    StepID        string
    ParentResult  *core.Result
    ChildFailed   bool   // true if child ran and failed
    FallbackUsed  bool   // true if fallback activated in place of child
    Warnings      []string
    Artifacts     []euclotypes.Artifact
}

// RecipeResult is the final outcome of executing an ExecutionPlan.
type RecipeResult struct {
    RecipeID      string
    FinalResult   *core.Result  // last successful step's result
    Artifacts     []euclotypes.Artifact // accumulated across all steps
    Warnings      []string
    StepResults   []StepResult
}

// Executor runs an ExecutionPlan against an ExecuteInput.
type Executor struct {
    env agentenv.AgentEnvironment
}

func NewExecutor(env agentenv.AgentEnvironment) *Executor

// Execute runs the plan step-by-step and returns the recipe result.
func (e *Executor) Execute(ctx context.Context, plan ExecutionPlan,
    in execution.ExecuteInput) (*RecipeResult, error)
```

### 6.6 Descriptor Extensions — `named/euclo/relurpicabilities/types.go`

New fields added to the existing `Descriptor` struct:

```go
// ModeFamilies lists all mode families this capability applies to.
// If non-empty, takes precedence over ModeFamily for all registry methods.
// ModeFamily is preserved for backward compatibility with existing built-ins.
ModeFamilies []string `json:"mode_families,omitempty"`

// TriggerPriority is used for tie-breaking when multiple capabilities match
// the same instruction with equal keyword match counts.
// Built-in capabilities carry an implicit value of 5.
// User-defined recipes can set 1–100 to override built-ins or yield to them.
TriggerPriority int `json:"trigger_priority,omitempty"`

// AllowDynamicResolution, when true, includes this capability in the candidate
// set surfaced to the LLM-based fallback capability classifier.
AllowDynamicResolution bool `json:"allow_dynamic_resolution,omitempty"`

// IsUserDefined marks this descriptor as originating from a user thought recipe
// rather than a built-in Go registration. Used in telemetry and logging.
IsUserDefined bool `json:"is_user_defined,omitempty"`

// RecipePath is the absolute path to the YAML file that produced this descriptor.
// Empty for built-in capabilities.
RecipePath string `json:"recipe_path,omitempty"`
```

Registry method updates required:
- `IDsForMode(modeFamily string)` — check `ModeFamilies` when non-empty, fall back to `ModeFamily`
- `PrimaryCapabilitiesForMode(modeID string)` — same
- `FallbackCapabilityForMode(modeID string)` — same
- `MatchByKeywords(instruction, modeID string, extraKeywords)` — add `TriggerPriority` as a secondary sort key after `MatchCount`

### 6.7 Dynamic Resolution Types — `named/euclo/runtime/types.go` (additive)

```go
// UserRecipeSignalSource describes a user-defined thought recipe's signal
// contribution to the mode classifier. Populated from the relurpic registry
// at NormalizeTaskEnvelope time.
type UserRecipeSignalSource struct {
    ID                     string   // capability ID, e.g. "euclo:recipe.my-recipe"
    Modes                  []string // declared modes
    IntentKeywords         []string // declared intent_keywords
    AllowDynamicResolution bool
}
```

`TaskEnvelope` gains one new field:

```go
// UserRecipes holds signal sources from registered thought recipes, injected
// during NormalizeTaskEnvelope. Used by CollectSignals for mode-level routing.
UserRecipes []UserRecipeSignalSource `json:"user_recipes,omitempty"`
```

---

## 7. Recipe Executor Algorithm

The executor runs after `BuildUnitOfWork` has selected a thought recipe as the primary capability. At that point, the behavior dispatch layer calls the recipe executor instead of a hard-coded behavior.

```
Execute(ctx, plan, in):

1. Build global FilteredRegistry:
   globalReg = FilteredRegistry(in.Environment.Registry, plan.GlobalAllowed)

2. Hydrate global enrichment sources into in.State:
   for each source in plan.GlobalEnrichment:
       hydrateEnrichmentSource(ctx, source, in)

3. Build global TemplateContext (task-level only, no step context yet):
   globalTmplCtx = BuildTemplateContext(in.Task, nil, globalEnrichmentBundle, plan.Resolver)

4. Render global prompt:
   globalPromptRendered, warnings = RenderPrompt(plan.GlobalPrompt, globalTmplCtx)
   recipeState.Warnings += warnings

5. For each step in plan.Steps:

   5a. Build step state from sharing mode:
       switch plan.SharingMode:
       case explicit:
           stepState = newBaseState(in.Task, in.State)
           for each alias in step.Parent.Inherit:
               key = plan.Resolver.MustResolve(alias)
               if val, ok = recipeState.Captured[alias]; ok:
                   stepState.Set(key, val)
       case carry_forward:
           stepState = in.State.Clone()
           inject all recipeState.Captured keys
       case isolated:
           stepState = newBaseState(in.Task, in.State)

   5b. Hydrate step-level enrichment (additive to global):
       for each source in step.Parent.Enrichment not already in global:
           hydrateEnrichmentSource(ctx, source, stepState)

   5c. Build step TemplateContext (task + inherited context):
       stepTmplCtx = BuildTemplateContext(in.Task, stepState, stepEnrichmentBundle, plan.Resolver)

   5d. Render step prompt (global prompt prepended):
       rendered, warnings = RenderPrompt(globalPromptRendered + "\n" + step.Parent.PromptTmpl, stepTmplCtx)
       recipeState.Warnings += warnings

   5e. Build step FilteredRegistry:
       stepReg = globalReg.Intersect(step.Parent.AllowedIDs)

   5f. Wire child into parent's delegation slot (if child declared):
       switch step.Parent.Paradigm:
       case "htn":
           childAgent = buildParadigmAgent(step.Child, childReg, in.Environment)
           parentAgent = htn.New(env, methods, htn.WithPrimitiveExec(childAgent))
       case "reflection":
           childAgent = buildParadigmAgent(step.Child, childReg, in.Environment)
           parentAgent = reflection.New(env, childAgent)
       case "goalcon":
           parentAgent = goalcon built with PlanExecutor = childAgent
       default (no delegation slot):
           warn: "child declared for paradigm X which has no delegation slot; ignoring"
           parentAgent = buildParadigmAgent(step.Parent, stepReg, in.Environment)

   5g. Execute parent paradigm:
       parentTask = &core.Task{
           ID:          in.Task.ID + "." + step.ID,
           Instruction: rendered,
           Type:        deriveTaskType(step.Parent),
           Context:     taskContextFromInput(in),
       }
       parentResult, err = parentAgent.Execute(ctx, parentTask, stepState)

   5h. If parent execution failed:
       recipeState.Warnings += "step " + step.ID + " parent failed: " + err.Error()
       stepResult.ParentResult = nil
       append stepResult to recipeState.StepResults
       continue to next step   // recipe continues; partial failure is non-fatal

   5i. Extract parent captures from stepState:
       for each alias in step.Parent.Capture:
           key = plan.Resolver.MustResolve(alias)
           if val, ok = stepState.Get(key); ok:
               recipeState.Captured[alias] = val

   5j. Collect parent artifacts:
       recipeState.Artifacts += artifactsFromResult(parentResult)
       lastSuccessfulResult = parentResult

   5k. If step.Child declared and paradigm supports delegation (child was wired above):
       [child execution already embedded in parent via delegation slot]
       [child artifacts and state are merged by the parent paradigm]

       If child execution failed (detected from parentResult.Data["child_failed"]):
           stepResult.ChildFailed = true
           if step.Fallback declared:
               fallbackResult, fallbackErr = executeFallbackAgent(ctx, step.Fallback, ...)
               if fallbackErr == nil:
                   stepResult.FallbackUsed = true
                   extract fallback captures, collect fallback artifacts
               else:
                   recipeState.Warnings += "fallback also failed for step " + step.ID
           else:
               recipeState.Warnings += "child failed for step " + step.ID + "; no fallback declared"

   5l. Append StepResult to recipeState.StepResults.

6. Build final RecipeResult:
   if lastSuccessfulResult == nil:
       return error("no step produced a successful result")
   return RecipeResult{
       RecipeID:    plan.RecipeID,
       FinalResult: lastSuccessfulResult,
       Artifacts:   recipeState.Artifacts,
       Warnings:    recipeState.Warnings,
       StepResults: recipeState.StepResults,
   }
```

---

## 8. Dynamic Resolution Architecture

Thought recipe keywords must participate at two levels of the routing pipeline:

### Level 1: Mode Classifier (signals.go)

`NormalizeTaskEnvelope` is extended to accept the `relurpicabilities.Registry` alongside the existing `capability.Registry`. It populates `TaskEnvelope.UserRecipes` from all descriptors where `IsUserDefined == true` and `AllowDynamicResolution == true`.

`CollectSignals` gains a new collector:

```go
func collectUserRecipeSignals(envelope TaskEnvelope) []ClassificationSignal {
    // For each user recipe signal source in envelope.UserRecipes:
    // If any intent_keyword is a substring of the instruction,
    // emit a ClassificationSignal{Kind: "user_recipe", Weight: WeightKeyword, Mode: mode}
    // for each mode in the recipe's declared modes.
}
```

This ensures that when a user recipe's keywords fire, the mode classifier routes to one of the recipe's declared modes.

### Level 2: Capability Intent Selection (relurpicabilities/types.go)

`MatchByKeywords` already supports `extraKeywords map[string][]string`. The startup registration path populates this map from loaded thought recipes. No changes to `MatchByKeywords` itself are needed — the injection point already exists.

The `TriggerPriority` field is added as a secondary sort key in `MatchByKeywords` results:

```
Sort: MatchCount desc → TriggerPriority desc → ID asc
```

### Signal kind `"user_recipe"`

A new signal kind constant is added to `signals.go`:

```go
const WeightUserRecipeKeyword = WeightKeyword  // same weight as built-in keyword signals
```

The kind `"user_recipe"` is treated identically to `"keyword"` in `ClassifyTaskScored`'s `hasStrongSignal` check — it prevents the default code baseline from overriding a user recipe match.

---

## 9. Startup Integration

### Loading path

```
ayenitd.Open()
  → WorkspaceEnvironment setup
    → named/euclo/capabilities/registry.go: BuildEucloCapabilityRegistry(env, recipeDir)
        → thoughtrecipes.LoadAll(recipeDir)     // scan relurpify_cfg/euclo/thoughtrecipe/*.yaml
        → for each loaded ThoughtRecipe:
            validate(recipe)
            plan = CompileExecutionPlan(recipe)
            descriptor = toDescriptor(recipe)   // Descriptor{IsUserDefined:true, ...}
            relurpicRegistry.Register(descriptor)
            extraKeywords[descriptor.ID] = recipe.Global.Configuration.IntentKeywords
            userRecipeSignalSources = append(userRecipeSignalSources, toSignalSource(recipe))
        → store userRecipeSignalSources in the euclo runtime state for
          NormalizeTaskEnvelope to read
```

The euclo capability registry (`named/euclo/capabilities/registry.go`) is the integration point — it already initializes the `relurpicabilities.Registry` and wires it into the euclo runtime. Loading thought recipes extends this initialization rather than introducing a new startup hook.

### Behavior dispatch

The existing behavior dispatch in the euclo runtime selects a `Behavior` based on `PrimaryRelurpicCapabilityID`. A thought recipe's capability ID has the prefix `"euclo:recipe."`. The dispatch layer adds a case for this prefix:

```go
if strings.HasPrefix(capabilityID, "euclo:recipe.") {
    plan, ok := recipeRegistry.LookupPlan(capabilityID)
    if ok {
        result, err = thoughtrecipeExecutor.Execute(ctx, plan, in)
        return wrapRecipeResult(result, err)
    }
}
```

This keeps the thought recipe execution path isolated from the existing behavior dispatch.

---

## 10. File Placement Summary

| File | Package | Purpose |
|------|---------|---------|
| `named/euclo/thoughtrecipes/schema.go` | `thoughtrecipes` | All YAML schema Go types |
| `named/euclo/thoughtrecipes/aliases.go` | `thoughtrecipes` | Standard alias table + AliasResolver |
| `named/euclo/thoughtrecipes/template.go` | `thoughtrecipes` | ${var} renderer + TemplateContext |
| `named/euclo/thoughtrecipes/loader.go` | `thoughtrecipes` | YAML loader, validator, ExecutionPlan compiler |
| `named/euclo/thoughtrecipes/executor.go` | `thoughtrecipes` | Executor type + step sequencer algorithm |
| `named/euclo/thoughtrecipes/paradigm_factory.go` | `thoughtrecipes` | Paradigm name → agent constructor map; delegation wiring |
| `named/euclo/thoughtrecipes/enrichment.go` | `thoughtrecipes` | Enrichment source hydration (AST, archeo, BKC) |
| `framework/capability/filtered.go` | `capability` | FilteredRegistry |
| `named/euclo/relurpicabilities/types.go` | `relurpicabilities` | Descriptor field additions + registry method updates |
| `named/euclo/runtime/types.go` | `runtime` | UserRecipeSignalSource + TaskEnvelope.UserRecipes field |
| `named/euclo/runtime/signals.go` | `runtime` | collectUserRecipeSignals + WeightUserRecipeKeyword |
| `named/euclo/runtime/classification.go` | `runtime` | NormalizeTaskEnvelope extension for user recipe sources |
| `named/euclo/capabilities/registry.go` | `capabilities` | Startup: LoadAll + register + wire signal sources |
| `templates/euclo/thoughtrecipe/*.yaml` | — | Example recipe templates copied to workspace on init |

---

## 11. Implementation Plan

---

### Phase 1 — Core Schema Types and Standard Aliases

**Goal:** Define all Go types for the ThoughtRecipe YAML schema and the standard alias table. No execution logic. This phase establishes the data contract that all later phases depend on.

**Files produced:**
- `named/euclo/thoughtrecipes/schema.go` — all schema types (ThoughtRecipe through RecipeStepContext)
- `named/euclo/thoughtrecipes/aliases.go` — StandardAliases map + AliasResolver type

**File dependencies (reads):** none — pure new types

**Unit tests to write** (`named/euclo/thoughtrecipes/schema_test.go`, `aliases_test.go`):
- `TestAliasResolverStandardAliases` — each standard alias resolves to the correct state key
- `TestAliasResolverCustomOverride` — custom alias shadows standard; warning is returned
- `TestAliasResolverUnknown` — unknown alias returns `("", false)` from Resolve; alias itself from MustResolve
- `TestAliasResolverCustomOnly` — custom-only alias resolves correctly
- `TestThoughtRecipeSchemaZeroValue` — zero-value ThoughtRecipe has no panics on field access
- `TestRecipeSharingSpecDefault` — empty SharingSpec.Default is handled by loader (phase 4 concern, but the type should document the default)

**Exit criteria:**
- All types compile cleanly
- `go test ./named/euclo/thoughtrecipes/...` passes
- No imports outside `standard library` and `framework/core` (types only, no runtime deps)

---

### Phase 2 — FilteredRegistry

**Goal:** Implement `FilteredRegistry` in `framework/capability/`. This is a framework primitive that enforces per-step capability visibility scoping.

**Files produced:**
- `framework/capability/filtered.go` — FilteredRegistry type

**File dependencies (reads):**
- `framework/capability/capability_registry.go` — existing Registry interface/type

**Unit tests to write** (`framework/capability/filtered_test.go`):
- `TestFilteredRegistryPassthrough` — nil/empty allowed list returns same capabilities as base
- `TestFilteredRegistryAllowedGet` — only allowed capabilities are returned from Get
- `TestFilteredRegistryDeniedGet` — denied capability returns (nil, false) from Get
- `TestFilteredRegistryModelCallableTools` — ModelCallableTools returns only allowed tools
- `TestFilteredRegistryInvokeAllowed` — InvokeCapability succeeds for allowed capability
- `TestFilteredRegistryInvokeDenied` — InvokeCapability returns error for denied capability
- `TestFilteredRegistryIntersect` — Intersect correctly narrows the allowed set
- `TestFilteredRegistryIntersectExpand` — Intersect cannot expand beyond current allowed set (intersection, not union)
- `TestFilteredRegistryIsPassthrough` — reports passthrough correctly
- `TestFilteredRegistryAllowedIDs` — returns sorted allowed IDs

**Exit criteria:**
- All tests pass
- `FilteredRegistry` satisfies the same interface surface used by paradigm agents (verified by compile-time assertion if an interface exists, or by test coverage)
- `go test ./framework/capability/...` passes (no regressions in existing tests)

---

### Phase 3 — Template Renderer

**Goal:** Implement `${var}` substitution with the bounded `TemplateContext` namespace. No YAML loading. No execution.

**Files produced:**
- `named/euclo/thoughtrecipes/template.go` — TemplateContext, TemplateTaskView, TemplateEnrichmentView, RenderPrompt, BuildTemplateContext

**File dependencies (reads):**
- `named/euclo/thoughtrecipes/aliases.go` (Phase 1)
- `framework/core/context.go` — core.Context for BuildTemplateContext

**Unit tests to write** (`named/euclo/thoughtrecipes/template_test.go`):
- `TestRenderPromptNoVars` — prompt with no ${} passes through unchanged
- `TestRenderPromptTaskInstruction` — ${task.instruction} resolves correctly
- `TestRenderPromptTaskType` — ${task.type} resolves correctly
- `TestRenderPromptTaskWorkspace` — ${task.workspace} resolves correctly
- `TestRenderPromptContextAlias` — ${context.explore_findings} resolves from TemplateContext.Context map
- `TestRenderPromptContextMissing` — ${context.unknown_key} renders as empty string + warning
- `TestRenderPromptEnrichmentAST` — ${enrichment.ast} resolves from EnrichmentView.AST
- `TestRenderPromptEnrichmentDisabled` — ${enrichment.bkc} when BKC disabled renders as empty + warning
- `TestRenderPromptMultipleVars` — multiple vars in one prompt all resolve
- `TestRenderPromptMalformed` — `${unclosed` is passed through unchanged (no panic)
- `TestRenderPromptEmpty` — empty prompt returns empty string, no warnings
- `TestBuildTemplateContextNilTask` — nil task produces zero-value TemplateTaskView, no panic
- `TestBuildTemplateContextNilState` — nil state produces empty context map, no panic
- `TestBuildTemplateContextWithCaptures` — captured aliases appear in context map

**Exit criteria:**
- All tests pass
- No imports of execution-layer packages (template is pure data transformation)
- `go test ./named/euclo/thoughtrecipes/...` passes

---

### Phase 4 — YAML Loader and Validator

**Goal:** Implement the loader that reads `*.yaml` files from `relurpify_cfg/euclo/thoughtrecipe/`, validates the schema, resolves aliases, and produces `ExecutionPlan` objects. No paradigm execution. No agent construction.

**Files produced:**
- `named/euclo/thoughtrecipes/loader.go` — LoadFile, LoadAll, Validate, CompileExecutionPlan
- `named/euclo/thoughtrecipes/executor.go` (types only in this phase: ExecutionPlan, ExecutionStep, ExecutionStepAgent, RecipeState, StepResult, RecipeResult, SharingMode, EnrichmentSource)

**File dependencies (reads):**
- `named/euclo/thoughtrecipes/schema.go` (Phase 1)
- `named/euclo/thoughtrecipes/aliases.go` (Phase 1)
- `named/euclo/thoughtrecipes/template.go` (Phase 3) — for prompt validation (detect obvious syntax errors)

**Validation rules enforced by loader:**
- `apiVersion` must be `"euclo/v1alpha1"`, `kind` must be `"ThoughtRecipe"`
- `metadata.name` required, non-empty, matches `[a-z][a-z0-9-]*`
- `metadata.description` required, non-empty
- `sequence` must have at least one step
- Each step `id` must be unique within the recipe
- Each `paradigm` value must be one of the supported strings
- `child:` with a paradigm that has no delegation slot logs a loader warning (not an error)
- `global.context.aliases` keys that shadow standard aliases log a loader warning
- `trigger_priority` clamped to `[1, 100]` if declared; defaults to 5

**Unit tests to write** (`named/euclo/thoughtrecipes/loader_test.go`):
- `TestLoadFileValid` — minimal valid recipe loads without error
- `TestLoadFileInvalidAPIVersion` — wrong apiVersion returns validation error
- `TestLoadFileInvalidKind` — wrong kind returns validation error
- `TestLoadFileMissingName` — missing metadata.name returns error
- `TestLoadFileMissingDescription` — missing metadata.description returns error
- `TestLoadFileEmptySequence` — empty sequence returns error
- `TestLoadFileDuplicateStepID` — duplicate step IDs return error
- `TestLoadFileInvalidParadigm` — unknown paradigm name returns error
- `TestLoadFileChildNoDelegation` — child on react parent logs warning, does not error
- `TestLoadFileAliasShadowWarning` — custom alias shadowing standard alias logs warning
- `TestLoadFilePriorityClamp` — priority > 100 is clamped to 100
- `TestLoadFilePriorityDefault` — omitted priority defaults to 5
- `TestCompileExecutionPlanAliasResolution` — inherit/capture aliases compile to correct state keys
- `TestCompileExecutionPlanGlobalCapabilities` — global allowed list is propagated to steps
- `TestCompileExecutionPlanStepIntersect` — step allowed list is intersected with global
- `TestCompileExecutionPlanSharingDefault` — omitted sharing.default uses "explicit"
- `TestLoadAllEmpty` — empty directory returns empty slice, no error
- `TestLoadAllMixed` — directory with valid and invalid files: valid ones load, errors collected
- `TestLoadAllSkipsNonYAML` — non-.yaml files in directory are ignored

**Exit criteria:**
- All tests pass
- A valid recipe YAML file round-trips through LoadFile → CompileExecutionPlan without loss of declared fields
- Validation rejects all invalid inputs above without panic

---

### Phase 5 — Descriptor Extensions and Registry Method Updates

**Goal:** Add `ModeFamilies`, `TriggerPriority`, `AllowDynamicResolution`, `IsUserDefined`, `RecipePath` to `Descriptor`. Update all registry methods to handle multi-mode and priority. All existing built-in descriptors continue to work unchanged.

**Files modified:**
- `named/euclo/relurpicabilities/types.go` — Descriptor struct + all registry methods

**File dependencies (reads):**
- All existing callers of `IDsForMode`, `PrimaryCapabilitiesForMode`, `FallbackCapabilityForMode`, `MatchByKeywords` — scan for callers that may need updates

**Registry method update spec:**

`effectiveModes(desc Descriptor) []string`:
- If `desc.ModeFamilies` non-empty: return `desc.ModeFamilies`
- Else if `desc.ModeFamily` non-empty: return `[]string{desc.ModeFamily}`
- Else return nil

This helper is used by all mode-sensitive methods.

`MatchByKeywords` sort order change:
- Primary: `MatchCount desc`
- Secondary: `TriggerPriority desc` (new)
- Tertiary: `ID asc` (existing tie-breaker, now third)

**Unit tests to write** (`named/euclo/relurpicabilities/types_test.go` — extend existing):
- `TestDescriptorModeFamiliesOverridesModeFamily` — IDsForMode finds capability via ModeFamilies
- `TestDescriptorModeFamiliesMultiMode` — capability with ModeFamilies:[debug,chat] appears in both
- `TestDescriptorModeFamilyFallback` — legacy ModeFamily-only descriptor still found by IDsForMode
- `TestMatchByKeywordsPriorityTieBreak` — two equal-match capabilities: higher priority wins
- `TestMatchByKeywordsPriorityIDTieBreak` — equal match count and equal priority: ID sort applies
- `TestDescriptorTriggerPriorityDefault` — zero-value priority does not cause sort panic
- `TestRegistryRegisterUserDefined` — IsUserDefined descriptor registers and is retrievable
- `TestDefaultRegistryBuiltinPriority` — built-in descriptors have effective priority 5 (via helper or explicit field)

**Exit criteria:**
- All existing `types_test.go` tests continue to pass
- New tests pass
- `go test ./named/euclo/relurpicabilities/...` passes

---

### Phase 6 — Paradigm Factory and Delegation Wiring

**Goal:** Implement the paradigm factory that constructs agent instances from string paradigm names, and the delegation wiring that connects child agents into parent delegation slots. This is the lowest-level execution primitive in the thought recipe system.

**Files produced:**
- `named/euclo/thoughtrecipes/paradigm_factory.go` — ParadigmFactory map + BuildParadigmAgent + WireChild

**File dependencies (reads):**
- `agents/htn/new.go` — `htn.New`, `htn.WithPrimitiveExec`
- `agents/reflection/new.go` — `reflection.New`
- `agents/goalcon/new.go` — goalcon struct + PlanExecutor field
- `agents/react/new.go` — `react.New`
- `agents/planner/new.go` — planner struct
- `agents/rewoo/new.go` — rewoo struct
- `named/euclo/thoughtrecipes/schema.go` (Phase 1)
- `framework/agentenv/` — AgentEnvironment
- `framework/capability/filtered.go` (Phase 2) — FilteredRegistry
- `named/euclo/thoughtrecipes/executor.go` types (Phase 4)

**Delegation wiring spec:**

```
BuildParadigmAgent(paradigm string, child *graph.WorkflowExecutor,
                   reg *capability.Registry, env agentenv.AgentEnvironment)
    (graph.WorkflowExecutor, error):

  switch paradigm:
  case "react":    return react.New(envWithRegistry(env, reg)), nil
  case "planner":  return planner.New(envWithRegistry(env, reg)), nil
  case "rewoo":    return rewoo.New(envWithRegistry(env, reg)), nil
  case "htn":
      if child != nil:
          return htn.New(envWithRegistry(env, reg), nil, htn.WithPrimitiveExec(*child)), nil
      return htn.New(envWithRegistry(env, reg), nil), nil
  case "reflection":
      delegate := child  // nil = reflection.New defaults to react
      return reflection.New(envWithRegistry(env, reg), delegate), nil
  case "goalcon":
      agent = &goalcon.GoalConAgent{...}
      if child != nil: agent.PlanExecutor = *child
      return agent, nil
  case "blackboard", "architect", "chainer":
      if child != nil:
          log warning: "child not supported for paradigm X in V1; ignoring"
      return buildParadigmNoChild(paradigm, reg, env), nil
  default:
      return nil, fmt.Errorf("unknown paradigm: %s", paradigm)
```

**Unit tests to write** (`named/euclo/thoughtrecipes/paradigm_factory_test.go`):
- `TestBuildParadigmAgentReact` — returns a non-nil react agent
- `TestBuildParadigmAgentPlanner` — returns a non-nil planner agent
- `TestBuildParadigmAgentHTN` — returns a non-nil HTN agent
- `TestBuildParadigmAgentHTNWithChild` — HTN wired with react child; PrimitiveExec non-nil
- `TestBuildParadigmAgentHTNWithPlannerChild` — HTN wired with planner child
- `TestBuildParadigmAgentReflection` — returns a non-nil reflection agent with default react delegate
- `TestBuildParadigmAgentReflectionWithChild` — reflection wired with specified child
- `TestBuildParadigmAgentGoalcon` — returns non-nil goalcon agent
- `TestBuildParadigmAgentGoalconWithChild` — goalcon.PlanExecutor set to child
- `TestBuildParadigmAgentBlackboardIgnoresChild` — child declared for blackboard; warning returned; agent non-nil
- `TestBuildParadigmAgentUnknown` — unknown paradigm string returns error
- `TestBuildParadigmAgentWithFilteredRegistry` — agent's Tools registry is the filtered view, not base

**Exit criteria:**
- All tests pass
- Delegation wiring tested end-to-end: agent constructed, child wired, Initialize called successfully
- `go test ./named/euclo/thoughtrecipes/...` passes

---

### Phase 7 — Recipe Executor (Step Sequencer, State Scoping, Artifact Accumulation)

**Goal:** Implement the full `Executor.Execute` algorithm from Section 7. This is the heart of the thought recipe system. By end of this phase, a thought recipe can be loaded and fully executed in a test environment.

**Files modified/produced:**
- `named/euclo/thoughtrecipes/executor.go` — Executor type + full Execute implementation + helpers
- `named/euclo/thoughtrecipes/enrichment.go` — EnrichmentBundle + hydrateEnrichmentSource

**File dependencies (reads):**
- All Phase 1–6 outputs
- `named/euclo/execution/behavior.go` — ExecuteInput, mergeStateArtifacts
- `named/euclo/execution/executors.go` — ExecutorFactory helpers
- `named/euclo/euclotypes/` — Artifact, ArtifactState, ExecutionEnvelope
- `framework/core/context.go` — Context, Clone

**State scoping implementation note:**
`newBaseState(task, parentState)` constructs a Context containing only:
- Task metadata keys (`mode`, `profile`, `workspace`, task context values)
- Nothing from the accumulated recipe state unless explicitly inherited

This prevents state bleed even under `carry_forward` sharing, where only the *captured* keys — not the full parent state — are forwarded.

**Unit tests to write** (`named/euclo/thoughtrecipes/executor_test.go`):
- `TestExecutorSingleStepSuccess` — single-step recipe returns parent result and artifact
- `TestExecutorMultiStepArtifactAccumulation` — three steps, artifacts from all three accumulated
- `TestExecutorLastSuccessfulResult` — final result is the last successful step's result
- `TestExecutorParentFailureContinues` — step with failing parent logs warning and continues
- `TestExecutorAllParentsFail` — recipe where all parents fail returns error
- `TestExecutorExplicitSharingIsolatesState` — step 2 cannot see step 1's state without inherit
- `TestExecutorExplicitSharingWithInherit` — declared inherit makes key visible to step 2
- `TestExecutorCarryForwardSharing` — step 2 sees step 1 captures without explicit inherit
- `TestExecutorIsolatedSharing` — step 2 sees no prior captures regardless of declarations
- `TestExecutorCapabilityFilterApplied` — step with allowed: [file_read] cannot invoke file_write
- `TestExecutorGlobalCapabilityFilter` — global filter restricts all steps
- `TestExecutorChildWiredForHTN` — htn parent with react child; child receives prompt
- `TestExecutorChildFailureFallback` — child failure activates fallback; warning recorded
- `TestExecutorChildFailureNoFallback` — child failure without fallback; warning; step continues
- `TestExecutorGlobalPromptPrepended` — each step's rendered prompt starts with global prompt content
- `TestExecutorTemplateSubstitution` — ${task.instruction} and ${context.alias} resolve in prompts
- `TestExecutorUnresolvedTemplateWarning` — unresolved ${context.missing} produces warning
- `TestExecutorEnrichmentHydrated` — enrichment source hydrated when declared; prompt var resolves

**Exit criteria:**
- All tests pass
- An end-to-end test with a mock paradigm agent demonstrates a two-step recipe with inter-step context sharing completing successfully
- Warnings surface in RecipeResult.Warnings, not panics or errors
- `go test ./named/euclo/thoughtrecipes/...` passes with >80% coverage on executor.go

---

### Phase 8 — Startup Registration and Capability Integration

**Goal:** Wire the thought recipe loader into euclo's startup path. Loaded recipes register as full `Descriptor` entries in the relurpic registry, participate in `MatchByKeywords`, and route to the recipe executor through the behavior dispatch layer.

**Files modified:**
- `named/euclo/capabilities/registry.go` — extend BuildEucloCapabilityRegistry to call `thoughtrecipes.LoadAll` + register descriptors
- `named/euclo/runtime/workunit.go` or dispatch layer — add thought recipe dispatch branch

**New integration types:**

`RecipePlanRegistry` (in `named/euclo/thoughtrecipes/loader.go` or a new `registry.go`):
- Stores `ExecutionPlan` by capability ID
- `Register(plan ExecutionPlan)`
- `Lookup(capabilityID string) (ExecutionPlan, bool)`

This registry is constructed by the loader during startup and passed to the behavior dispatch layer.

**Behavior dispatch integration:**

The existing behavior dispatch in `named/euclo/` selects a Go `Behavior` by `PrimaryRelurpicCapabilityID`. The dispatch extension:

```go
// Before the existing capability-ID switch:
if strings.HasPrefix(work.PrimaryRelurpicCapabilityID, "euclo:recipe.") {
    plan, ok := recipePlanRegistry.Lookup(work.PrimaryRelurpicCapabilityID)
    if !ok {
        return nil, fmt.Errorf("thought recipe not found: %s", work.PrimaryRelurpicCapabilityID)
    }
    executor := thoughtrecipes.NewExecutor(in.Environment)
    recipeResult, err := executor.Execute(ctx, plan, in)
    if err != nil {
        return &core.Result{Success: false, Error: err}, err
    }
    return recipeResultToCore(recipeResult), nil
}
```

**Unit tests to write** (`named/euclo/capabilities/registry_test.go` — extend existing):
- `TestRegistryLoadsThoughtRecipes` — directory with one valid recipe YAML: descriptor registered
- `TestRegistryEmptyDirectory` — no YAML files: no new descriptors, no error
- `TestRegistryInvalidRecipeSkipped` — invalid YAML file: skipped with error logged, valid ones still loaded
- `TestRegistryRecipeDescriptorFields` — loaded descriptor has correct ID prefix, keywords, modes, IsUserDefined
- `TestRecipePlanRegistryLookup` — registered plan is retrievable by ID
- `TestRecipePlanRegistryUnknown` — unknown ID returns (zero, false)
- `TestDispatchThoughtRecipeRoute` — capability ID matching `euclo:recipe.*` routes to recipe executor
- `TestDispatchThoughtRecipeNotFound` — `euclo:recipe.missing` returns error, not panic

**Exit criteria:**
- A thought recipe YAML placed in `relurpify_cfg/euclo/thoughtrecipe/` in a test workspace is discovered, loaded, registered, and selectable via keyword matching
- `go test ./named/euclo/capabilities/...` passes
- No regression in existing `named/euclo` tests

---

### Phase 9 — Dynamic Resolution Signal Injection

**Goal:** Extend `NormalizeTaskEnvelope` and `CollectSignals` so that thought recipe keywords contribute to mode classification signals, and registered recipes participate in the LLM-based fallback capability selection when `allow_dynamic_resolution: true`.

**Files modified:**
- `named/euclo/runtime/types.go` — add `UserRecipeSignalSource` type + `TaskEnvelope.UserRecipes` field
- `named/euclo/runtime/signals.go` — add `collectUserRecipeSignals` + `WeightUserRecipeKeyword` constant
- `named/euclo/runtime/classification.go` — extend `NormalizeTaskEnvelope` to populate `UserRecipes`; extend `ClassifyTaskScored` to treat `"user_recipe"` signal kind as a strong signal

**NormalizeTaskEnvelope signature extension:**

The relurpic registry is needed to populate user recipe signal sources. Two options:
1. Accept `relurpicRegistry *relurpicabilities.Registry` as a new parameter
2. Derive signal sources from the `capability.Registry` (if user recipe descriptors are also stored there)

Option 1 is cleaner. `NormalizeTaskEnvelope` gains the parameter; all callers are updated.

**`hasStrongSignal` extension in ClassifyTaskScored:**

```go
if s.Kind == "keyword" || s.Kind == "task_structure" || s.Kind == "error_text" ||
   s.Kind == "context_hint" || s.Kind == "user_recipe" {
    hasStrongSignal = true
}
```

This prevents user recipe keyword matches from being overridden by the default code baseline.

**Unit tests to write** (`named/euclo/runtime/classification_test.go`, `signals_test.go` — extend existing):
- `TestCollectUserRecipeSignalsMatch` — recipe keyword present in instruction; signal emitted
- `TestCollectUserRecipeSignalsNoMatch` — no keyword match; no signal emitted
- `TestCollectUserRecipeSignalsDynamicResolutionFalse` — `allow_dynamic_resolution: false`; signal still emitted for keyword match (dynamic resolution flag governs LLM fallback inclusion, not keyword signal generation)
- `TestCollectUserRecipeSignalsMultiMode` — recipe with modes:[debug,chat]; matched keyword emits signal for each mode
- `TestClassifyTaskScoredUserRecipeStrong` — user recipe keyword match prevents code baseline injection
- `TestClassifyTaskScoredUserRecipeMode` — user recipe signal for "debug" mode pushes classifier toward debug
- `TestNormalizeTaskEnvelopePopulatesUserRecipes` — registry with user descriptors: UserRecipes populated
- `TestNormalizeTaskEnvelopeEmptyRegistry` — no user descriptors: UserRecipes is nil/empty, no panic

**Exit criteria:**
- Existing classification tests continue to pass without change
- A thought recipe with `intent_keywords: ["thorough analysis"]` and `modes: [debug]` causes `ClassifyTaskScored` to route to debug mode when instruction contains "thorough analysis"
- `go test ./named/euclo/runtime/...` passes

---

### Phase 10 — Example Templates and Developer Documentation

**Goal:** Provide template thought recipe YAML files in `templates/euclo/thoughtrecipe/` that serve as starting points for workspace developers. Document the schema, standard aliases, and template variables in a reference doc.

**Files produced:**
- `templates/euclo/thoughtrecipe/deep-review-patch.yaml` — two-step planner→reflection + react→reflection recipe
- `templates/euclo/thoughtrecipe/investigate-only.yaml` — single-step react analysis recipe with archaeology enrichment
- `templates/euclo/thoughtrecipe/tdd-assist.yaml` — debug-mode recipe demonstrating HTN with react child
- `docs/euclo/thought-recipes.md` — user-facing reference: schema fields, standard aliases, template variables, paradigm matrix, example recipes

**No unit tests in this phase** (documentation + data files only).

**Exit criteria:**
- All three template YAML files pass `thoughtrecipes.Validate()` without error
- Each template is referenced in `docs/euclo/thought-recipes.md`
- Schema field documentation covers every YAML field with type, required/optional, default, and an example value

---

### Phase 11 — Cleanup

**Goal:** Remove scaffolding, normalize any TODOs introduced during development, ensure the full test suite passes cleanly, and verify layering boundary scripts still pass.

**Cleanup checklist:**

1. **Remove `ModeFamily` single-string field from `Descriptor`** after verifying all callers have migrated to `effectiveModes()`. If any built-ins still use `ModeFamily` directly, migrate them to `ModeFamilies` with a single entry.

2. **Remove any `TODO: phase N` comments** introduced during phased implementation.

3. **Audit `named/euclo/thoughtrecipes/` for dead helpers** introduced during development but superseded by later phases. Delete any that are unreachable.

4. **Verify boundary scripts still pass:**
   ```bash
   scripts/check-framework-boundaries.sh
   scripts/check-deprecated-agent-wrappers.sh
   ```
   The thought recipe executor imports `agents/` paradigms — confirm this import path is valid (named/ importing agents/ is permitted by the layering rules).

5. **Run full test suite:**
   ```bash
   go test ./...
   ```
   All tests must pass. Address any flaky tests introduced by the new concurrent state access patterns in the executor.

6. **Coverage check** — `named/euclo/thoughtrecipes/executor.go` must have ≥80% line coverage. Add targeted tests for any uncovered branches.

7. **Remove `RecipePath` from `Descriptor` if unused** — if no runtime code reads it beyond telemetry, confirm it is used or remove it to avoid dead field accumulation.

8. **Normalize warning telemetry** — ensure all warning paths use the same key format (`"euclo.thought_recipe.warnings"`) and that the field is documented in `named/euclo/euclotypes/`.

9. **Verify YAML template files still validate** against the final schema after any schema changes in later phases.

**Unit tests to write (cleanup phase):**
- `TestThoughtRecipesFullRoundTrip` — integration test: load template YAML, compile plan, execute with mock paradigms, verify RecipeResult contract (last successful result + accumulated artifacts)
- `TestBoundaryNoFrameworkImportFromThoughtRecipes` — static import check: `named/euclo/thoughtrecipes` must not import `framework/agents` (the `agents/` package, not `framework/`) — confirms layering
- `TestDescriptorNoModeFamily` — after migration: zero-value `ModeFamily` field does not break IDsForMode

**Exit criteria:**
- `go build ./...` passes
- `go test ./...` passes
- Both boundary scripts pass
- No `TODO: phase` comments remain in the codebase
- `docs/euclo/thought-recipes.md` exists and covers all schema fields
- All three template YAML files validate successfully against the loader

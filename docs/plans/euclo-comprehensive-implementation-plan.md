# Euclo Completion Engineering Specification

**Status:** active 
**Date:** 2026-05-05  
**Scope:** Complete implementation of `named/euclo` per reimplementation spec  
**Prerequisite:** Review of `/docs/plans/euclo-reimplementation-spec.md` and `/docs/research/euclo_implementation_gap_analysis.md`  

---

## Executive Summary

This specification tracks the Euclo reimplementation against the current codebase. The intake path, service registration, state accessors, classifier, routing fixes, and top-level agent wiring now compile and pass package tests. The remaining work is concentrated in the later policy, interaction, telemetry, ingestion, and graph-composition paths, which still contain simplified or stubbed behavior.

1. **Structural cleanup:** `named/euclo/services` is present and delegates framework registration cleanly.
2. **No stubs, no shims:** This remains the target, but some downstream nodes are still simplified.
3. **Prompt integration:** Recipe resolution still uses `framework/prompt` and the Euclo prompt providers.
4. **Service registration:** The `framework/agentenv` + `framework/services` split is reflected in the code.

## 0. Current Codebase Audit

Implemented and passing:

1. `named/euclo/services` and `named/euclo/registration.go`
2. `named/euclo/state`
3. `named/euclo/intake` core path, including the LLM classifier and pipeline
4. `named/euclo/orchestrate/dispatching.go` and `route_fork.go`
5. `named/euclo/agent.go` initialization flow
6. Most of `named/euclo/recipes`

Present but simplified or stubbed:

1. `named/euclo/ingestion/node.go`
2. `named/euclo/policy/gates.go`
3. `named/euclo/interaction/emit.go` and `resume.go`
4. `named/euclo/reporting/telemetry.go`
5. `named/euclo/orchestrate/background.go`

Validation:

1. `go test ./named/euclo/...` passes with a writable Go cache.
2. The intake package no longer depends on the nonexistent `named/euclo/intake/capability` subpackage.

---

## 1. Architecture Context

### 1.1 Framework Services Architecture

The codebase has been refactored to separate workspace environment definition from service implementation:

```
framework/agentenv/          # Defines interfaces and WorkspaceEnvironment
├── environment.go           # WorkspaceEnvironment struct
├── service.go              # Service, ServiceManager, ServiceScheduler interfaces
├── registration.go         # AgentRegistrationFuncs pattern
└── workspace.go            # Workspace operations

framework/services/          # Implements base framework services
├── capability_bundle.go    # Capability bundle management
├── prompt_registry.go      # Prompt registry integration
└── doc.go

ayenitd/                     # Service runner only
├── service/                 # Service implementations (browser, etc.)
└── ...runtime wiring

named/euclo/services/        # NEW: Euclo-specific service registrations
├── registration.go         # Euclo's AgentRegistrationFuncs
├── capabilities.go         # Capability provider wiring
├── prompts.go             # Prompt provider wiring
└── recipes.go             # Recipe loader wiring
```

### 1.2 Prompt System Integration

Recipes use `framework/prompt` for template resolution:

```go
// framework/prompt/types.go

type PromptConfig struct {
    APIVersion        string
    ID                string
    Name              string
    Description       string
    Extends           string
    Tags              Tags
    Variables         map[string]VariableDecl
    Blocks            []PromptBlock
    SourcePath        string
    ParentResolved    *PromptConfig
}

type RuntimeContext struct {
    Variables    map[string]string
    State        map[string]any
    Envelope     *contextdata.Envelope
    Paradigm     string
    ConsumerID   string
    Task         *core.Task
    Tools        []contracts.Tool
    Capabilities []core.CapabilityDescriptor
    AgentSpec    *agentspec.AgentSpec
}

type Registry interface {
    RegisterProvider(name string, provider ContextProvider) error
    Resolve(promptID string, ctx RuntimeContext) (string, error)
}
```

Recipes reference prompts via `RecipeStep.PromptID`, resolved at execution time against the `PromptRegistry` in `WorkspaceEnvironment`.

---

## 2. Package Structure (Target)

```
named/euclo/
├── agent.go                   # WorkflowExecutor facade - COMPLETE
├── config.go                  # EucloConfig - ADD: missing fields
├── doc.go
├── registration.go            # DEPRECATED: move to services/
│
├── services/                  # NEW PACKAGE - Euclo framework integration
│   ├── doc.go
│   ├── register.go            # Main registration entrypoint
│   ├── capabilities.go        # Capability provider registrations
│   ├── prompts.go            # Prompt provider registrations  
│   └── recipes.go            # Recipe loader registration
│
├── intake/
│   ├── doc.go
│   ├── normalize.go           # COMPLETE
│   ├── signals.go             # COMPLETE
│   ├── classify.go            # COMPLETE
│   ├── capability_classifier.go # IMPLEMENT: LLM-backed Tier-2
│   ├── stream_request.go      # COMPLETE
│   ├── pipeline.go            # IMPLEMENT: full pipeline execution
│   └── types.go               # COMPLETE
│
├── families/
│   ├── doc.go
│   ├── registry.go            # COMPLETE
│   ├── builtin.go             # COMPLETE
│   ├── selection.go           # COMPLETE
│   └── types.go               # COMPLETE
│
├── ingestion/
│   ├── doc.go
│   ├── spec.go                # COMPLETE
│   ├── node.go                # IMPLEMENT: use framework/ingestion.Pipeline
│   ├── workspace.go           # COMPLETE
│   └── types.go               # COMPLETE
│
├── recipes/
│   ├── doc.go
│   ├── schema.go              # COMPLETE
│   ├── loader.go              # IMPLEMENT: full loader with validation
│   ├── registry.go            # IMPLEMENT: full registry
│   ├── compiler.go            # IMPLEMENT: ThoughtRecipe → ExecutionPlan
│   ├── graph_builder.go       # IMPLEMENT: ExecutionPlan → agentgraph.Graph
│   ├── step_node.go           # COMPLETE (uses prompt registry)
│   ├── aliases.go             # IMPLEMENT: alias resolution
│   └── types.go               # ADD: ExecutionStep fields
│
├── capabilities/
│   ├── doc.go
│   ├── registry.go            # COMPLETE
│   ├── families.go            # COMPLETE
│   ├── selection.go           # IMPLEMENT: keyword matching
│   ├── invocation.go          # COMPLETE
│   └── types.go               # COMPLETE
│
├── interaction/
│   ├── doc.go
│   ├── frames.go              # COMPLETE
│   ├── slots.go               # COMPLETE
│   ├── emit.go                # IMPLEMENT: frame emission
│   ├── resume.go              # IMPLEMENT: frame resume
│   └── types.go               # COMPLETE
│
├── orchestrate/
│   ├── doc.go
│   ├── dispatcher.go          # COMPLETE
│   ├── dispatching.go         # FIX: routeMatchesFamily empty family bug
│   ├── route_fork.go          # IMPLEMENT: ConditionalNode pattern
│   ├── recipe_executor.go     # IMPLEMENT: full execution
│   ├── capability_executor.go # IMPLEMENT: sequence handling
│   ├── background.go          # IMPLEMENT: BackgroundJobNode
│   ├── merge.go               # IMPLEMENT: envelope merge
│   ├── graph.go               # REIMPLEMENT: proper node wiring
│   └── types.go               # ADD: RouteResult fields
│
├── policy/
│   ├── doc.go
│   ├── decision.go            # COMPLETE
│   ├── evaluator.go           # IMPLEMENT: full evaluation
│   ├── gates.go               # IMPLEMENT: authorization integration
│   └── types.go               # COMPLETE
│
├── reporting/
│   ├── doc.go
│   ├── telemetry.go           # IMPLEMENT: typed emit helpers
│   ├── events.go              # IMPLEMENT: event constructors
│   ├── outcome.go             # IMPLEMENT: outcome classification
│   └── types.go               # COMPLETE
│
├── state/
│   ├── doc.go
│   ├── keys.go                # COMPLETE
│   ├── accessors.go           # COMPLETE
│   └── types.go               # COMPLETE
│
├── promptprovider/            # COMPLETE - Euclo-specific prompt providers
│   ├── doc.go
│   ├── register.go
│   ├── recipe_step_context.go
│   ├── recipe_plan_goal.go
│   └── recipe_prior_step.go
│
├── recipetemplates/           # COMPLETE - Built-in recipe YAMLs
│   ├── doc.go
│   ├── loader.go
│   └── ...recipe files
│
└── relurpicabilities/         # COMPLETE - Capability implementations
    ├── doc.go
    ├── register.go
    └── ...capability handlers
```

Note: the package tree above is the original target layout. The current codebase audit in section 0 reflects the actual implementation status.

---

## 3. Implementation Specifications

### 3.1 NEW: `named/euclo/services` Package

**Purpose:** Centralize Euclo's framework service registration, replacing the ad-hoc `registration.go` pattern.

**File: `services/doc.go`**
```go
// Package services provides Euclo's framework service registrations.
//
// Euclo integrates with the framework through the agentenv.AgentRegistrationFuncs
// pattern defined in framework/agentenv. This package implements the registration
// functions that wire Euclo's capabilities, prompt providers, and recipes into the
// workspace environment.
//
// The registrations are called by the composition root (ayenitd) during workspace
// initialization, avoiding circular dependencies between named/euclo and ayenitd.
package services
```

**File: `services/register.go`**
```go
package services

import (
	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/named/euclo/recipe"
)

// Registration provides Euclo's service registration functions.
// This is the primary entrypoint for framework integration.
type Registration struct {
	capabilityRegistrar CapabilityRegistrar
	promptRegistrar     PromptRegistrar
	recipeLoader        RecipeLoader
}

// NewRegistration creates a new Registration with default implementations.
func NewRegistration() *Registration {
	return &Registration{
		capabilityRegistrar: &defaultCapabilityRegistrar{},
		promptRegistrar:     &defaultPromptRegistrar{},
		recipeLoader:        &defaultRecipeLoader{},
	}
}

// AgentRegistrationFuncs returns the registration functions for agentenv.
func (r *Registration) AgentRegistrationFuncs() agentenv.AgentRegistrationFuncs {
	return agentenv.AgentRegistrationFuncs{
		RegisterCapabilities:    r.registerCapabilities,
		RegisterPromptProviders: r.registerPromptProviders,
		LoadRecipes:             r.loadRecipes,
	}
}

func (r *Registration) registerCapabilities(env agentenv.WorkspaceEnvironment) error {
	return r.capabilityRegistrar.RegisterAll(env)
}

func (r *Registration) registerPromptProviders(env agentenv.WorkspaceEnvironment) error {
	return r.promptRegistrar.RegisterAll(env)
}

func (r *Registration) loadRecipes() (interface{}, error) {
	return r.recipeLoader.LoadAll()
}

// Option configures the Registration.
type Option func(*Registration)

// WithCapabilityRegistrar sets a custom capability registrar.
func WithCapabilityRegistrar(cr CapabilityRegistrar) Option {
	return func(r *Registration) {
		r.capabilityRegistrar = cr
	}
}

// WithPromptRegistrar sets a custom prompt registrar.
func WithPromptRegistrar(pr PromptRegistrar) Option {
	return func(r *Registration) {
		r.promptRegistrar = pr
	}
}

// WithRecipeLoader sets a custom recipe loader.
func WithRecipeLoader(rl RecipeLoader) Option {
	return func(r *Registration) {
		r.recipeLoader = rl
	}
}

// CapabilityRegistrar abstracts capability registration.
type CapabilityRegistrar interface {
	RegisterAll(env agentenv.WorkspaceEnvironment) error
}

// PromptRegistrar abstracts prompt provider registration.
type PromptRegistrar interface {
	RegisterAll(env agentenv.WorkspaceEnvironment) error
}

// RecipeLoader abstracts recipe loading.
type RecipeLoader interface {
	LoadAll() (*recipe.RecipeRegistry, error)
}
```

**File: `services/capabilities.go`**
```go
package services

import (
	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/named/euclo/relurpicabilities"
)

type defaultCapabilityRegistrar struct{}

func (r *defaultCapabilityRegistrar) RegisterAll(env agentenv.WorkspaceEnvironment) error {
	if env.Registry == nil {
		return nil // Capabilities require a registry; skip if not available
	}
	return relurpicabilities.RegisterAll(env)
}
```

**File: `services/prompts.go`**
```go
package services

import (
	"codeburg.org/lexbit/relurpify/agents/promptprovider"
	"codeburg.org/lexbit/relurpify/framework/agentenv"
	eucloprovider "codeburg.org/lexbit/relurpify/named/euclo/promptprovider"
)

type defaultPromptRegistrar struct{}

func (r *defaultPromptRegistrar) RegisterAll(env agentenv.WorkspaceEnvironment) error {
	if env.PromptRegistry == nil {
		return nil // Prompts require a registry; skip if not available
	}
	
	// Register generic paradigm providers
	if err := promptprovider.RegisterAll(env.PromptRegistry); err != nil {
		return err
	}
	
	// Register Euclo-specific providers
	if err := eucloprovider.RegisterAll(env.PromptRegistry); err != nil {
		return err
	}
	
	return nil
}
```

**File: `services/recipes.go`**
```go
package services

import (
	"codeburg.org/lexbit/relurpify/named/euclo/recipe"
	"codeburg.org/lexbit/relurpify/named/euclo/recipetemplates"
)

type defaultRecipeLoader struct{}

func (r *defaultRecipeLoader) LoadAll() (*recipe.RecipeRegistry, error) {
	return recipetemplates.LoadAll()
}
```

**Migration: Update `named/euclo/registration.go`**
```go
package euclo

import (
	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/named/euclo/services"
)

// GetRegistrationFuncs returns AgentRegistrationFuncs for Euclo.
// Deprecated: Use services.NewRegistration().AgentRegistrationFuncs() instead.
func GetRegistrationFuncs() agentenv.AgentRegistrationFuncs {
	return services.NewRegistration().AgentRegistrationFuncs()
}
```

---

### 3.2 FIX: Task Seeding in `agent.go`

**File: `named/euclo/agent.go:Execute()`**

Replace the current implementation (lines 98-124):

```go
func (a *Agent) Execute(ctx context.Context, task *core.Task, env *contextdata.Envelope) (*core.Result, error) {
	if !a.initialized {
		if err := a.Initialize(nil); err != nil {
			return nil, fmt.Errorf("failed to initialize agent: %w", err)
		}
	}

	if a.env.StreamTrigger != nil {
		ctx = contextstream.WithTrigger(ctx, a.env.StreamTrigger)
	}

	a.captureResumeState(env)
	a.seedResumeState(env)
	defer a.clearResumeState()

	// SEED TASK INTO ENVELOPE
	// This makes the task available to all graph nodes via envelope working memory
	if env != nil && task != nil {
		env.SetWorkingValue("task.input", task, contextdata.MemoryClassTask)
		env.SetWorkingValue("task.id", task.ID, contextdata.MemoryClassTask)
		env.SetWorkingValue("task.instruction", task.Instruction, contextdata.MemoryClassTask)
		env.SetWorkingValue("task.type", task.Type, contextdata.MemoryClassTask)
		env.SetWorkingValue("task.context", task.Context, contextdata.MemoryClassTask)
		if task.Metadata != nil {
			env.SetWorkingValue("task.metadata", task.Metadata, contextdata.MemoryClassTask)
		}
	}

	graph, err := a.BuildGraph(task)
	if err != nil {
		return nil, fmt.Errorf("failed to build execution graph: %w", err)
	}

	result, err := graph.Execute(ctx, env)
	if err != nil {
		return nil, fmt.Errorf("graph execution failed: %w", err)
	}

	return result, nil
}
```

---

### 3.3 IMPLEMENT: Full `intake/pipeline.go`

**Current (stub):** Lines 54-89 hardcode `FamilyImplementation`

**Implementation:**

```go
package intake

import (
	"context"
	"fmt"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/contextstream"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/euclo/families"
	"codeburg.org/lexbit/relurpify/named/euclo/state"
)

// IntakePipelineNode performs the full intake pipeline: normalize → tier1 → stream → tier2.
type IntakePipelineNode struct {
	id                string
	registry          *families.KeywordFamilyRegistry
	maxStreamTokens   int
	defaultStreamMode contextstream.Mode
	streamTrigger     *contextstream.Trigger
	classifier        CapabilityClassifier
}

// NewIntakePipelineNode creates a new intake pipeline node.
func NewIntakePipelineNode(
	id string,
	registry *families.KeywordFamilyRegistry,
	maxStreamTokens int,
	defaultStreamMode contextstream.Mode,
	trigger *contextstream.Trigger,
	classifier CapabilityClassifier,
) *IntakePipelineNode {
	return &IntakePipelineNode{
		id:                id,
		registry:          registry,
		maxStreamTokens:   maxStreamTokens,
		defaultStreamMode: defaultStreamMode,
		streamTrigger:     trigger,
		classifier:        classifier,
	}
}

// ID returns the node ID.
func (n *IntakePipelineNode) ID() string {
	return n.id
}

// Type returns the node type.
func (n *IntakePipelineNode) Type() agentgraph.NodeType {
	return agentgraph.NodeTypeSystem
}

// Contract returns the node contract.
func (n *IntakePipelineNode) Contract() agentgraph.NodeContract {
	return agentgraph.NodeContract{
		SideEffectClass: agentgraph.SideEffectNone,
		Idempotency:     agentgraph.IdempotencyReplaySafe,
	}
}

// Execute performs the intake pipeline.
func (n *IntakePipelineNode) Execute(ctx context.Context, env *contextdata.Envelope) (*agentgraph.Result, error) {
	// 1. Extract task from envelope
	taskVal, ok := env.GetWorkingValue("task.input")
	if !ok {
		return nil, fmt.Errorf("no task input in envelope")
	}
	task, ok := taskVal.(*core.Task)
	if !ok {
		return nil, fmt.Errorf("task.input is not *core.Task")
	}

	// 2. Normalize task to TaskEnvelope
	taskEnvelope := NormalizeTask(task)

	// 3. Perform tier-1 classification (deterministic)
	scoredClassification := ClassifyTaskScored(taskEnvelope, n.registry, nil)

	// 4. Build stream request if family has template
	family, _ := n.registry.Lookup(scoredClassification.WinningFamily)
	var streamResult *contextstream.Result
	
	if family.RetrievalTemplate != "" && n.streamTrigger != nil {
		// Determine stream mode: background is only safe when tier-2 is bypassed
		streamMode := n.defaultStreamMode
		if scoredClassification.ClassificationSource != "override" && streamMode == contextstream.ModeBackground {
			// Tier-2 depends on stream result - must use blocking mode
			streamMode = contextstream.ModeBlocking
		}

		streamReq := BuildStreamRequest(
			families.FamilySelection{WinningFamily: scoredClassification.WinningFamily},
			taskEnvelope.Instruction,
			n.maxStreamTokens,
			streamMode,
		)

		// Execute stream
		var streamErr error
		if streamMode == contextstream.ModeBackground {
			streamResult, streamErr = n.streamTrigger.RequestBackground(ctx, streamReq)
		} else {
			streamResult, streamErr = n.streamTrigger.Request(ctx, streamReq)
		}
		
		if streamErr != nil {
			// Stream failure is non-fatal; log and continue with empty context
			// The classifier will work with what it has
			streamResult = &contextstream.Result{Query: streamReq.Query}
		}
	}

	// 5. Perform tier-2 classification (LLM-backed)
	var capabilitySequence []string
	var capabilityOperator string
	
	if n.classifier != nil {
		// Use LLM-based classifier with streamed context
		var classifyErr error
		capabilitySequence, capabilityOperator, classifyErr = n.classifier.Classify(
			ctx,
			taskEnvelope.Instruction,
			scoredClassification.WinningFamily,
			streamResult.Context,
			taskEnvelope.NegativeConstraints,
		)
		if classifyErr != nil {
			// Fall back to family fallback capability
			capabilitySequence = []string{family.FallbackCapability}
			capabilityOperator = "OR"
		}
	} else {
		// No classifier available - use family metadata
		if len(family.CapabilitySequence) > 0 {
			capabilitySequence = family.CapabilitySequence
			capabilityOperator = "OR"
		} else {
			capabilitySequence = []string{family.FallbackCapability}
			capabilityOperator = "OR"
		}
	}

	// 6. Build IntentClassification
	classification := &IntentClassification{
		WinningFamily:              scoredClassification.WinningFamily,
		FamilyCandidates:           scoredClassification.FamilyCandidates,
		Confidence:                 scoredClassification.Confidence,
		Ambiguous:                  scoredClassification.Ambiguous,
		Signals:                    scoredClassification.Signals,
		NegativeConstraints:        taskEnvelope.NegativeConstraints,
		CapabilitySequence:         capabilitySequence,
		CapabilityOperator:         capabilityOperator,
		ClassificationSource:     scoredClassification.Source,
		MixedIntent:                scoredClassification.Ambiguous,
		EditPermitted:              taskEnvelope.EditPermitted,
		RequiresVerification:       family.DefaultVerification == VerificationRequired,
		Scope:                      scoredClassification.Scope,
		RiskLevel:                  scoredClassification.RiskLevel,
		ReasonCodes:                scoredClassification.ReasonCodes,
	}

	// 7. Write results to envelope
	state.SetTaskEnvelope(env, taskEnvelope)
	state.SetIntentClassification(env, classification)
	state.SetFamilySelection(env, &families.FamilySelection{
		WinningFamily: classification.WinningFamily,
		Confidence:    classification.Confidence,
		Ambiguous:     classification.Ambiguous,
	})
	state.SetStreamResult(env, streamResult)
	state.SetCapabilitySequence(env, capabilitySequence)
	state.SetCapabilityOperator(env, capabilityOperator)
	state.SetClassificationSource(env, classification.ClassificationSource)
	state.SetNegativeConstraints(env, taskEnvelope.NegativeConstraints)

	// 8. Return result for graph routing
	return &agentgraph.Result{
		NodeID:  n.id,
		Success: true,
		Data: map[string]any{
			"winning_family":     classification.WinningFamily,
			"confidence":         classification.Confidence,
			"ambiguous":          classification.Ambiguous,
			"capability_count":   len(capabilitySequence),
			"has_stream_result":  streamResult != nil,
		},
	}, nil
}

// CapabilityClassifier defines the interface for LLM-backed tier-2 classification.
type CapabilityClassifier interface {
	Classify(
		ctx context.Context,
		instruction string,
		familyID string,
		streamedContext string,
		negativeConstraints []string,
	) ([]string, string, error)
}
```

---

### 3.4 NEW: LLM Capability Classifier

**File: `intake/capability_classifier.go`**

```go
package intake

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/core"
)

// LLMCapabilityClassifier performs LLM-based tier-2 capability selection.
type LLMCapabilityClassifier struct {
	model         core.LanguageModel
	maxTokens     int
	temperature   float64
}

// NewLLMCapabilityClassifier creates a new LLM-based classifier.
func NewLLMCapabilityClassifier(model core.LanguageModel) *LLMCapabilityClassifier {
	return &LLMCapabilityClassifier{
		model:       model,
		maxTokens:   512,
		temperature: 0.1, // Low temperature for deterministic selection
	}
}

// Classify performs LLM-based capability selection within a family.
func (c *LLMCapabilityClassifier) Classify(
	ctx context.Context,
	instruction string,
	familyID string,
	streamedContext string,
	negativeConstraints []string,
) ([]string, string, error) {
	if c.model == nil {
		return nil, "", fmt.Errorf("no language model available")
	}

	// Build constrained prompt
	prompt := c.buildPrompt(instruction, familyID, streamedContext, negativeConstraints)

	// Call LLM
	response, err := c.model.Complete(ctx, core.CompletionRequest{
		Prompt:      prompt,
		MaxTokens:   c.maxTokens,
		Temperature: c.temperature,
	})
	if err != nil {
		return nil, "", fmt.Errorf("LLM completion failed: %w", err)
	}

	// Parse response
	return c.parseResponse(response.Text)
}

func (c *LLMCapabilityClassifier) buildPrompt(
	instruction string,
	familyID string,
	streamedContext string,
	negativeConstraints []string,
) string {
	var b strings.Builder
	
	b.WriteString("You are a task classifier for a coding assistant. ")
	b.WriteString("Select the most appropriate capabilities from the provided list.\n\n")
	
	b.WriteString("Task: ")
	b.WriteString(instruction)
	b.WriteString("\n\n")
	
	if streamedContext != "" {
		b.WriteString("Context:\n")
		b.WriteString(streamedContext)
		b.WriteString("\n\n")
	}
	
	if len(negativeConstraints) > 0 {
		b.WriteString("Constraints (DO NOT use capabilities that violate these):\n")
		for _, constraint := range negativeConstraints {
			b.WriteString("- ")
			b.WriteString(constraint)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	
	b.WriteString("Select one or more capabilities from this list. ")
	b.WriteString("Respond with ONLY a JSON object in this format:\n")
	b.WriteString(`{"capabilities": ["cap_id_1", "cap_id_2"], "operator": "AND"}`)
	b.WriteString("\n\n")
	b.WriteString("Use operator \"AND\" if all capabilities must execute in sequence. ")
	b.WriteString("Use operator \"OR\" if the first successful capability should stop execution.\n")
	
	return b.String()
}

func (c *LLMCapabilityClassifier) parseResponse(text string) ([]string, string, error) {
	// Extract JSON from response
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start == -1 || end == -1 || end <= start {
		return nil, "", fmt.Errorf("no JSON object found in response")
	}
	
	jsonStr := text[start : end+1]
	
	var result struct {
		Capabilities []string `json:"capabilities"`
		Operator     string   `json:"operator"`
	}
	
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, "", fmt.Errorf("failed to parse JSON response: %w", err)
	}
	
	// Normalize operator
	result.Operator = strings.ToUpper(strings.TrimSpace(result.Operator))
	if result.Operator != "AND" {
		result.Operator = "OR" // Default to OR
	}
	
	// Validate capabilities
	if len(result.Capabilities) == 0 {
		return nil, "", fmt.Errorf("no capabilities selected")
	}
	
	return result.Capabilities, result.Operator, nil
}
```

---

### 3.5 FIX: Routing Bug in `dispatching.go`

**Current (buggy):** Lines 399-409 return `true` for empty family

**File: `orchestrate/dispatching.go`**

Replace `routeMatchesFamily`:

```go
func routeMatchesFamily(desc core.CapabilityDescriptor, family, instruction string) bool {
	family = strings.ToLower(strings.TrimSpace(family))
	instruction = strings.ToLower(strings.TrimSpace(instruction))
	
	// If no family specified, match by instruction analysis
	if family == "" {
		return matchesInstructionAnalysis(desc, instruction)
	}
	
	// Otherwise require explicit family match
	return strings.Contains(strings.ToLower(desc.ID), family) ||
		descHasFamilyTag(desc, family)
}

func matchesInstructionAnalysis(desc core.CapabilityDescriptor, instruction string) bool {
	// Analysis keywords → query/lookup capabilities (read-only)
	analysisTerms := []string{"explain", "what is", "describe", "how does", "what does", "analyze", "review"}
	for _, term := range analysisTerms {
		if strings.Contains(instruction, term) {
			return isAnalysisCapability(desc)
		}
	}
	
	// Mutation keywords → modification capabilities
	mutationTerms := []string{"fix", "repair", "refactor", "migrate", "implement", "add", "create", "change", "update"}
	for _, term := range mutationTerms {
		if strings.Contains(instruction, term) {
			return isMutationCapability(desc)
		}
	}
	
	// Default: include all for further ranking
	return true
}

func isAnalysisCapability(desc core.CapabilityDescriptor) bool {
	// Analysis capabilities have these traits
	for _, effect := range desc.EffectClasses {
		switch effect {
		case core.EffectClassReadOnly, core.EffectClassQuery:
			return true
		}
	}
	// Check ID patterns
	id := strings.ToLower(desc.ID)
	return strings.Contains(id, "query") ||
		strings.Contains(id, "analyze") ||
		strings.Contains(id, "review") ||
		strings.Contains(id, "lookup") ||
		strings.Contains(id, "search")
}

func isMutationCapability(desc core.CapabilityDescriptor) bool {
	// Mutation capabilities have these traits
	for _, effect := range desc.EffectClasses {
		switch effect {
		case core.EffectClassFileWrite, core.EffectClassFileDelete, core.EffectClassAPIChange:
			return true
		}
	}
	return false
}

func descHasFamilyTag(desc core.CapabilityDescriptor, family string) bool {
	// Check if descriptor has family tag/metadata
	if tags, ok := desc.Metadata["family"].([]string); ok {
		for _, tag := range tags {
			if strings.EqualFold(tag, family) {
				return true
			}
		}
	}
	return false
}
```

---

### 3.6 IMPLEMENT: Route Fork ConditionalNode

**File: `orchestrate/route_fork.go`**

Replace the entire file:

```go
package orchestrate

import (
	"context"
	"fmt"

	"codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/named/euclo/state"
)

// RouteForkNode is a ConditionalNode that reads the route kind from the envelope
// and returns the target node ID. This avoids the ambiguous-transition error
// that occurs when two conditional serial edges depart from the same node.
type RouteForkNode struct {
	id string
}

// NewRouteForkNode creates a new route fork node.
func NewRouteForkNode(id string) *RouteForkNode {
	return &RouteForkNode{id: id}
}

// ID returns the node ID.
func (n *RouteForkNode) ID() string {
	return n.id
}

// Type returns the node type.
func (n *RouteForkNode) Type() agentgraph.NodeType {
	return agentgraph.NodeTypeConditional
}

// Contract returns the node contract.
func (n *RouteForkNode) Contract() agentgraph.NodeContract {
	return agentgraph.NodeContract{
		SideEffectClass: agentgraph.SideEffectNone,
		Idempotency:     agentgraph.IdempotencyReplaySafe,
	}
}

// Execute reads the route kind from the envelope and returns the target node ID.
// The returned NextNode value is used by ConditionalNode edges to determine routing.
func (n *RouteForkNode) Execute(ctx context.Context, env *contextdata.Envelope) (*agentgraph.Result, error) {
	// Read route kind from envelope (written by DispatchNode)
	routeKind := state.GetString(env, state.KeyDispatchRouteKind)
	if routeKind == "" {
		// Fallback: check for legacy key
		routeKind = state.GetString(env, "euclo.dispatch.route_kind")
	}
	
	if routeKind == "" {
		return nil, fmt.Errorf("route_kind not found in envelope; dispatch node must run first")
	}
	
	// Validate route kind
	switch routeKind {
	case "recipe", "capability":
		// Valid
	default:
		return nil, fmt.Errorf("invalid route_kind: %s (expected 'recipe' or 'capability')", routeKind)
	}
	
	return &agentgraph.Result{
		NodeID:   n.id,
		Success:  true,
		NextNode: routeKind, // "recipe" or "capability" - used by ConditionalNode edges
	}, nil
}
```

**Add to `state/keys.go`:**
```go
const (
	// ... existing keys ...
	KeyDispatchRouteKind = "euclo.dispatch.route_kind"
)
```

**Add to `state/accessors.go`:**
```go
func GetString(env *contextdata.Envelope, key string) string {
	if env == nil {
		return ""
	}
	val, ok := env.GetWorkingValue(key)
	if !ok {
		return ""
	}
	if s, ok := val.(string); ok {
		return s
	}
	return ""
}
```

---

### 3.7 REIMPLEMENT: Root Graph Wiring

**File: `orchestrate/graph.go`**

Replace the entire file with proper node wiring:

```go
package orchestrate

import (
	"context"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/contextstream"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/euclo/families"
	"codeburg.org/lexbit/relurpify/named/euclo/intake"
	"codeburg.org/lexbit/relurpify/named/euclo/policy"
	recipepkg "codeburg.org/lexbit/relurpify/named/euclo/recipes"
)

// RootGraph wires together orchestration nodes using the agentgraph runtime.
type RootGraph struct {
	graph *agentgraph.Graph
}

// RootGraphConfig configures dependency wiring for the root graph.
type RootGraphConfig struct {
	Env                   agentenv.WorkspaceEnvironment
	CapabilityRegistry    *capability.CapabilityRegistry
	RecipeRegistry        *recipepkg.RecipeRegistry
	FamilyRegistry        *families.KeywordFamilyRegistry
	Workspace             string
	StreamTrigger         *contextstream.Trigger
	MaxStreamTokens       int
	DefaultStreamMode     contextstream.Mode
	CapabilityClassifier  intake.CapabilityClassifier
	PermissionManager     policy.PermissionManager
	HITLBroker            policy.HITLBroker
}

// RootGraphOption mutates RootGraphConfig.
type RootGraphOption func(*RootGraphConfig)

// WithWorkspaceEnvironment wires the workspace environment.
func WithWorkspaceEnvironment(env agentenv.WorkspaceEnvironment) RootGraphOption {
	return func(opts *RootGraphConfig) {
		opts.Env = env
	}
}

// WithCapabilityRegistry wires the capability registry.
func WithCapabilityRegistry(reg *capability.CapabilityRegistry) RootGraphOption {
	return func(opts *RootGraphConfig) {
		opts.CapabilityRegistry = reg
	}
}

// WithRecipeRegistry wires the recipe registry.
func WithRecipeRegistry(reg *recipepkg.RecipeRegistry) RootGraphOption {
	return func(opts *RootGraphConfig) {
		opts.RecipeRegistry = reg
	}
}

// WithFamilyRegistry wires the family registry.
func WithFamilyRegistry(reg *families.KeywordFamilyRegistry) RootGraphOption {
	return func(opts *RootGraphConfig) {
		opts.FamilyRegistry = reg
	}
}

// WithWorkspace wires the workspace root.
func WithWorkspace(workspace string) RootGraphOption {
	return func(opts *RootGraphConfig) {
		opts.Workspace = strings.TrimSpace(workspace)
	}
}

// WithStreamTrigger wires the context stream trigger.
func WithStreamTrigger(trigger *contextstream.Trigger) RootGraphOption {
	return func(opts *RootGraphConfig) {
		opts.StreamTrigger = trigger
	}
}

// WithMaxStreamTokens sets the stream token budget.
func WithMaxStreamTokens(tokens int) RootGraphOption {
	return func(opts *RootGraphConfig) {
		opts.MaxStreamTokens = tokens
	}
}

// WithDefaultStreamMode sets the default stream mode.
func WithDefaultStreamMode(mode contextstream.Mode) RootGraphOption {
	return func(opts *RootGraphConfig) {
		opts.DefaultStreamMode = mode
	}
}

// WithCapabilityClassifier sets the LLM-based classifier.
func WithCapabilityClassifier(classifier intake.CapabilityClassifier) RootGraphOption {
	return func(opts *RootGraphConfig) {
		opts.CapabilityClassifier = classifier
	}
}

// WithPermissionManager sets the permission manager for policy gates.
func WithPermissionManager(pm policy.PermissionManager) RootGraphOption {
	return func(opts *RootGraphConfig) {
		opts.PermissionManager = pm
	}
}

// WithHITLBroker sets the HITL broker for policy gates.
func WithHITLBroker(broker policy.HITLBroker) RootGraphOption {
	return func(opts *RootGraphConfig) {
		opts.HITLBroker = broker
	}
}

// NewRootGraph creates a new root graph with all components wired together.
func NewRootGraph(opts ...RootGraphOption) (*RootGraph, error) {
	cfg := RootGraphConfig{
		MaxStreamTokens:   8192,
		DefaultStreamMode: contextstream.ModeBlocking,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	// Validate required dependencies
	if cfg.FamilyRegistry == nil {
		return nil, fmt.Errorf("FamilyRegistry is required")
	}

	g := agentgraph.NewGraph()

	// Build nodes
	nodes := buildNodes(g, cfg)

	// Add nodes to graph
	for _, node := range nodes {
		if err := g.AddNode(node); err != nil {
			return nil, fmt.Errorf("add node %s: %w", node.ID(), err)
		}
	}

	// Wire edges
	if err := wireEdges(g, cfg); err != nil {
		return nil, fmt.Errorf("wire edges: %w", err)
	}

	// Validate graph
	if err := g.Validate(); err != nil {
		return nil, fmt.Errorf("graph validation failed: %w", err)
	}

	return &RootGraph{graph: g}, nil
}

func buildNodes(g *agentgraph.Graph, cfg RootGraphConfig) []agentgraph.Node {
	// Intake node - performs tier-1 and tier-2 classification
	intakeNode := intake.NewIntakePipelineNode(
		"euclo.intake",
		cfg.FamilyRegistry,
		cfg.MaxStreamTokens,
		cfg.DefaultStreamMode,
		cfg.StreamTrigger,
		cfg.CapabilityClassifier,
	)

	// Family selection check (conditional - if ambiguous, needs interaction)
	familySelectNode := newStageNode("euclo.family_select", agentgraph.NodeTypeSystem, func(ctx context.Context, env *contextdata.Envelope) (*core.Result, error) {
		// Family selection is done by intake; this node checks if interaction needed
		return &core.Result{NodeID: "euclo.family_select", Success: true}, nil
	})

	// Ingestion node - ingests user files and workspace
	ingestionNode := NewIngestionNode("euclo.ingest", cfg.Env)

	// Stream trigger node - optional context enrichment
	streamNode := NewStreamTriggerNode("euclo.stream", cfg.StreamTrigger, cfg.MaxStreamTokens)

	// Capability classification (tier-2) - now part of intake, but can be separate node
	capClassifyNode := newStageNode("euclo.capability_classify", agentgraph.NodeTypeSystem, func(ctx context.Context, env *contextdata.Envelope) (*core.Result, error) {
		// Tier-2 is done by intake; this node is for routing logic if needed
		return &core.Result{NodeID: "euclo.capability_classify", Success: true}, nil
	})

	// Interaction check - conditional node for low confidence
	interactionCheckNode := agentgraph.NewConditionalNode("euclo.interaction_check", func(result *core.Result, env *contextdata.Envelope) bool {
		// Check if interaction frame should be emitted
		classification, ok := state.GetIntentClassification(env)
		if !ok {
			return false
		}
		// Emit frame if ambiguous or low confidence
		return classification.Ambiguous || classification.Confidence < 0.7
	})

	// Interaction frame node - emits HITL frame when needed
	interactionFrameNode := NewInteractionFrameNode("euclo.interaction_frame")

	// Policy gate - checks authorization
	policyGate := policy.NewGateNode("euclo.policy_gate", cfg.PermissionManager, cfg.HITLBroker)

	// Dispatch node - resolves route
	dispatchNode := NewDispatcher("euclo.dispatch").
		WithWorkspace(cfg.Workspace).
		WithCapabilityRegistry(cfg.CapabilityRegistry).
		WithRecipeRegistry(cfg.RecipeRegistry)

	// Route fork - ConditionalNode that reads dispatch result
	routeForkNode := NewRouteForkNode("euclo.route_fork")

	// Recipe executor
	recipeExec := NewRecipeExecutorNode("euclo.execute_recipe").
		WithWorkspaceEnvironment(cfg.Env).
		WithIngestionPipeline(nil)
	if cfg.RecipeRegistry != nil {
		recipeExec.WithRecipeRegistry(cfg.RecipeRegistry)
	}

	// Capability executor
	capabilityExec := NewCapabilityExecutionNode("euclo.execute_capability")
	if cfg.CapabilityRegistry != nil {
		capabilityExec.WithCapabilityRegistry(cfg.CapabilityRegistry)
	}

	// Merge node
	mergeNode := newStageNode("euclo.merge", agentgraph.NodeTypeSystem, func(ctx context.Context, env *contextdata.Envelope) (*core.Result, error) {
		// Merge recipe/capability results into parent envelope
		return &core.Result{NodeID: "euclo.merge", Success: true}, nil
	})

	// Report node
	reportNode := newStageNode("euclo.report", agentgraph.NodeTypeSystem, func(ctx context.Context, env *contextdata.Envelope) (*core.Result, error) {
		// Emit final telemetry and outcome frame
		return &core.Result{NodeID: "euclo.report", Success: true}, nil
	})

	// Terminal node
	terminalNode := agentgraph.NewTerminalNode("euclo.done")

	return []agentgraph.Node{
		intakeNode,
		familySelectNode,
		ingestionNode,
		streamNode,
		capClassifyNode,
		interactionCheckNode,
		interactionFrameNode,
		policyGate,
		dispatchNode,
		routeForkNode,
		recipeExec,
		capabilityExec,
		mergeNode,
		reportNode,
		terminalNode,
	}
}

func wireEdges(g *agentgraph.Graph, cfg RootGraphConfig) error {
	// Standard path: intake → family_select → ingest? → stream? → capability_classify
	// → interaction_check → interaction_frame? → policy_gate → dispatch → route_fork
	// → (recipe | capability) → merge → report → done

	// Core sequence
	if err := g.AddEdge("euclo.intake", "euclo.family_select", nil); err != nil {
		return err
	}

	// Family select to ingestion (always present, ingestion node decides if it runs)
	if err := g.AddEdge("euclo.family_select", "euclo.ingest", nil); err != nil {
		return err
	}

	// Ingestion to stream (stream node checks if needed)
	if err := g.AddEdge("euclo.ingest", "euclo.stream", nil); err != nil {
		return err
	}

	// Stream to capability classification
	if err := g.AddEdge("euclo.stream", "euclo.capability_classify", nil); err != nil {
		return err
	}

	// Capability classification to interaction check
	if err := g.AddEdge("euclo.capability_classify", "euclo.interaction_check", nil); err != nil {
		return err
	}

	// Interaction check is conditional: if true, go to interaction_frame; else skip to policy_gate
	// Conditional edges from interaction_check
	if err := g.AddConditionalEdge("euclo.interaction_check", "euclo.interaction_frame", func(result *core.Result, env *contextdata.Envelope) bool {
		return result != nil && result.Data != nil && result.Data["needs_interaction"] == true
	}); err != nil {
		return err
	}
	if err := g.AddConditionalEdge("euclo.interaction_check", "euclo.policy_gate", func(result *core.Result, env *contextdata.Envelope) bool {
		return result == nil || result.Data == nil || result.Data["needs_interaction"] != true
	}); err != nil {
		return err
	}

	// Interaction frame to policy gate
	if err := g.AddEdge("euclo.interaction_frame", "euclo.policy_gate", nil); err != nil {
		return err
	}

	// Policy gate to dispatch
	if err := g.AddEdge("euclo.policy_gate", "euclo.dispatch", nil); err != nil {
		return err
	}

	// Dispatch to route fork
	if err := g.AddEdge("euclo.dispatch", "euclo.route_fork", nil); err != nil {
		return err
	}

	// Route fork to recipe executor (conditional)
	if err := g.AddConditionalEdge("euclo.route_fork", "euclo.execute_recipe", func(result *core.Result, env *contextdata.Envelope) bool {
		return result != nil && result.NextNode == "recipe"
	}); err != nil {
		return err
	}

	// Route fork to capability executor (conditional)
	if err := g.AddConditionalEdge("euclo.route_fork", "euclo.execute_capability", func(result *core.Result, env *contextdata.Envelope) bool {
		return result != nil && result.NextNode == "capability"
	}); err != nil {
		return err
	}

	// Both executors to merge
	if err := g.AddEdge("euclo.execute_recipe", "euclo.merge", nil); err != nil {
		return err
	}
	if err := g.AddEdge("euclo.execute_capability", "euclo.merge", nil); err != nil {
		return err
	}

	// Merge to report
	if err := g.AddEdge("euclo.merge", "euclo.report", nil); err != nil {
		return err
	}

	// Report to terminal
	if err := g.AddEdge("euclo.report", "euclo.done", nil); err != nil {
		return err
	}

	return nil
}

// Graph returns the underlying agentgraph graph.
func (g *RootGraph) Graph() *agentgraph.Graph {
	if g == nil {
		return nil
	}
	return g.graph
}

// Execute runs the root graph orchestration.
func (g *RootGraph) Execute(ctx context.Context, env *contextdata.Envelope) (*agentgraph.Result, error) {
	if g == nil || g.graph == nil {
		return nil, fmt.Errorf("root graph not initialized")
	}
	return g.graph.Execute(ctx, env)
}

// SetStart sets the start node for the graph (used for resume).
func (g *RootGraph) SetStart(nodeID string) error {
	if g == nil || g.graph == nil {
		return fmt.Errorf("root graph not initialized")
	}
	return g.graph.SetStart(nodeID)
}

type stageNode struct {
	id       string
	nodeType agentgraph.NodeType
	execFn   func(context.Context, *contextdata.Envelope) (*core.Result, error)
}

func newStageNode(id string, nodeType agentgraph.NodeType, execFn func(context.Context, *contextdata.Envelope) (*core.Result, error)) *stageNode {
	return &stageNode{id: id, nodeType: nodeType, execFn: execFn}
}

func (n *stageNode) ID() string                { return n.id }
func (n *stageNode) Type() agentgraph.NodeType { return n.nodeType }
func (n *stageNode) Execute(ctx context.Context, env *contextdata.Envelope) (*core.Result, error) {
	return n.execFn(ctx, env)
}
```

---

### 3.8 IMPLEMENT: Policy Gate with Authorization

**File: `policy/gates.go`**

Replace the entire file:

```go
package policy

import (
	"context"
	"errors"
	"fmt"
	"time"

	"codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/event"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
	"codeburg.org/lexbit/relurpify/named/euclo/state"
)

// PermissionManager abstracts authorization checking.
type PermissionManager interface {
	Check(ctx context.Context, decision PolicyDecision) authorization.Decision
}

// HITLBroker abstracts HITL approval workflows.
type HITLBroker interface {
	Wait(ctx context.Context, frameID string) (*interaction.HITLDecision, error)
}

// GateNode implements policy enforcement via authorization.PermissionManager.
// It reads PolicyDecision from envelope, checks authorization, and handles
// HITL workflows via authorization.HITLBroker.
type GateNode struct {
	id                string
	permissionManager PermissionManager
	hitlBroker        HITLBroker
	eventLog          event.Log
	timeout           time.Duration
}

// NewGateNode creates a new policy gate node.
func NewGateNode(id string, pm PermissionManager, broker HITLBroker) *GateNode {
	return &GateNode{
		id:                id,
		permissionManager: pm,
		hitlBroker:        broker,
		timeout:           5 * time.Minute,
	}
}

// WithTimeout sets the HITL timeout.
func (n *GateNode) WithTimeout(timeout time.Duration) *GateNode {
	n.timeout = timeout
	return n
}

// WithEventLog sets the event log for frame emission.
func (n *GateNode) WithEventLog(log event.Log) *GateNode {
	n.eventLog = log
	return n
}

// ID returns the node ID.
func (n *GateNode) ID() string {
	return n.id
}

// Type returns the node type.
func (n *GateNode) Type() agentgraph.NodeType {
	return agentgraph.NodeTypeTool
}

// Contract returns the node contract.
func (n *GateNode) Contract() agentgraph.NodeContract {
	// Determine contract based on whether HITL is required
	// This is dynamic; default to Human side effect for safety
	return agentgraph.NodeContract{
		SideEffectClass: agentgraph.SideEffectHuman,
		Idempotency:     agentgraph.IdempotencySingleShot,
	}
}

// Execute performs policy checking and HITL handling.
func (n *GateNode) Execute(ctx context.Context, env *contextdata.Envelope) (*agentgraph.Result, error) {
	// Read PolicyDecision from envelope
	decision, ok := state.GetPolicyDecision(env)
	if !ok {
		// No policy decision set; default to allow (for backwards compatibility)
		return &agentgraph.Result{
			NodeID:  n.id,
			Success: true,
			Data:    map[string]any{"gate": "skipped", "reason": "no_policy_decision"},
		}, nil
	}

	// Check authorization
	authDecision := n.permissionManager.Check(ctx, authorization.PermissionRequest{
		Resource:   decision.RouteID,
		Action:     "execute",
		RiskClass:  decision.RiskClass,
		EffectClasses: decision.EffectClasses,
	})

	switch authDecision {
	case authorization.Allow:
		return &agentgraph.Result{
			NodeID:  n.id,
			Success: true,
			Data:    map[string]any{"gate": "passed", "decision": "allow"},
		}, nil

	case authorization.Deny:
		return &agentgraph.Result{
			NodeID:  n.id,
			Success: false,
			Err:     errors.New("policy denied: " + decision.Justification),
			Data:    map[string]any{"gate": "denied", "decision": "deny"},
		}, nil

	case authorization.Ask:
		// HITL required - emit frame and wait
		return n.handleHITL(ctx, env, decision)

	default:
		return nil, fmt.Errorf("unknown authorization decision: %v", authDecision)
	}
}

func (n *GateNode) handleHITL(ctx context.Context, env *contextdata.Envelope, decision PolicyDecision) (*agentgraph.Result, error) {
	// Create HITL frame
	frame := interaction.NewHITLApprovalFrame(interaction.HITLApprovalPayload{
		RouteID:       decision.RouteID,
		RiskClass:     string(decision.RiskClass),
		EffectClasses: stringifyEffectClasses(decision.EffectClasses),
		Justification: decision.Justification,
		MutationPermitted: decision.MutationPermitted,
	})

	// Emit frame
	if err := interaction.EmitFrame(ctx, frame, env, n.eventLog); err != nil {
		return nil, fmt.Errorf("failed to emit HITL frame: %w", err)
	}

	// Wait for approval with timeout
	waitCtx, cancel := context.WithTimeout(ctx, n.timeout)
	defer cancel()

	approval, err := n.hitlBroker.Wait(waitCtx, frame.ID)
	if err != nil {
		return &agentgraph.Result{
			NodeID:  n.id,
			Success: false,
			Err:     fmt.Errorf("HITL wait failed: %w", err),
			Data:    map[string]any{"gate": "hitl_error", "error": err.Error()},
		}, nil
	}

	if !approval.Approved {
		return &agentgraph.Result{
			NodeID:  n.id,
			Success: false,
			Err:     errors.New("HITL approval rejected by user"),
			Data:    map[string]any{"gate": "hitl_rejected"},
		}, nil
	}

	// Approved - proceed
	return &agentgraph.Result{
		NodeID:  n.id,
		Success: true,
		Data:    map[string]any{"gate": "passed", "decision": "hitl_approved"},
	}, nil
}

func stringifyEffectClasses(classes []core.EffectClass) []string {
	result := make([]string, len(classes))
	for i, c := range classes {
		result[i] = string(c)
	}
	return result
}
```

**Add to `state/accessors.go`:**
```go
func GetPolicyDecision(env *contextdata.Envelope) (policy.PolicyDecision, bool) {
	if env == nil {
		return policy.PolicyDecision{}, false
	}
	val, ok := env.GetWorkingValue(KeyPolicyDecision)
	if !ok {
		return policy.PolicyDecision{}, false
	}
	d, ok := val.(policy.PolicyDecision)
	return d, ok
}

func SetPolicyDecision(env *contextdata.Envelope, decision policy.PolicyDecision) {
	if env == nil {
		return
	}
	env.SetWorkingValue(KeyPolicyDecision, decision, contextdata.MemoryClassTask)
}
```

---

### 3.9 IMPLEMENT: Interaction Frame Emission

**File: `interaction/emit.go`**

Create/replace with:

```go
package interaction

import (
	"context"
	"fmt"
	"time"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/event"
)

// EmitFrame writes an interaction frame to the envelope and publishes to event.Log.
func EmitFrame(ctx context.Context, frame *InteractionFrame, env *contextdata.Envelope, eventLog event.Log) error {
	if env == nil {
		return fmt.Errorf("cannot emit frame: envelope is nil")
	}

	// Assign sequence number
	frame.Seq = getNextFrameSeq(env)
	frame.CreatedAt = time.Now()

	// Write to envelope working memory
	key := fmt.Sprintf("euclo.interaction.frame.%d", frame.Seq)
	env.SetWorkingValue(key, frame, contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.interaction.frame_seq", frame.Seq, contextdata.MemoryClassTask)

	// Publish to event log
	if eventLog != nil {
		eventLog.Publish(core.FrameworkEvent{
			Type:      "euclo.interaction.frame.v1",
			Timestamp: time.Now(),
			Payload: map[string]any{
				"frame_id":   frame.ID,
				"frame_type": frame.Type,
				"seq":        frame.Seq,
				"task_id":    frame.TaskID,
				"slots":      len(frame.Slots),
			},
		})
	}

	return nil
}

func getNextFrameSeq(env *contextdata.Envelope) int {
	if env == nil {
		return 0
	}
	val, ok := env.GetWorkingValue("euclo.interaction.frame_seq")
	if !ok {
		return 1
	}
	if seq, ok := val.(int); ok {
		return seq + 1
	}
	return 1
}

// ResumeFrame reconstructs the pending frame from envelope on restart.
func ResumeFrame(env *contextdata.Envelope) (*InteractionFrame, bool) {
	if env == nil {
		return nil, false
	}

	// Find highest-seq frame with nil Response
	seqVal, ok := env.GetWorkingValue("euclo.interaction.frame_seq")
	if !ok {
		return nil, false
	}
	currentSeq, ok := seqVal.(int)
	if !ok {
		return nil, false
	}

	// Scan backwards from current seq
	for seq := currentSeq; seq > 0; seq-- {
		key := fmt.Sprintf("euclo.interaction.frame.%d", seq)
		val, ok := env.GetWorkingValue(key)
		if !ok {
			continue
		}
		frame, ok := val.(*InteractionFrame)
		if !ok {
			continue
		}
		// Check if frame needs response
		if frame.Response == nil && frame.RespondedAt == nil {
			return frame, true
		}
	}

	return nil, false
}

// NewHITLApprovalFrame creates a new HITL approval frame.
func NewHITLApprovalFrame(payload HITLApprovalPayload) *InteractionFrame {
	return &InteractionFrame{
		ID:      generateFrameID("hitl"),
		Type:    FrameHITLApproval,
		Slots: []ActionSlot{
			{
				ID:      "approve",
				Label:   "Approve",
				Action:  "approve",
				Risk:    payload.RiskClass,
				Default: false,
			},
			{
				ID:      "reject",
				Label:   "Reject",
				Action:  "reject",
				Risk:    "low",
				Default: true,
			},
		},
		DefaultSlot: "reject",
		Payload: map[string]any{
			"route_id":       payload.RouteID,
			"risk_class":     payload.RiskClass,
			"effect_classes": payload.EffectClasses,
			"justification":  payload.Justification,
			"mutation":       payload.MutationPermitted,
		},
	}
}

// HITLApprovalPayload contains data for HITL approval frames.
type HITLApprovalPayload struct {
	RouteID       string
	RiskClass     string
	EffectClasses []string
	Justification string
	MutationPermitted bool
}

func generateFrameID(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
}
```

---

### 3.10 IMPLEMENT: Telemetry and Reporting

**File: `reporting/telemetry.go`**

```go
package reporting

import (
	"context"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
)

// EucloTelemetry wraps core.Telemetry with typed Emit helpers.
type EucloTelemetry struct {
	sink core.Telemetry
}

// NewEucloTelemetry creates a new telemetry wrapper.
func NewEucloTelemetry(sink core.Telemetry) *EucloTelemetry {
	if sink == nil {
		sink = &noopTelemetry{}
	}
	return &EucloTelemetry{sink: sink}
}

func (t *EucloTelemetry) EmitIntakeComplete(ctx context.Context, ev EventIntakeComplete) {
	t.emit(ctx, "euclo.intake.complete", ev)
}

func (t *EucloTelemetry) EmitFamilySelected(ctx context.Context, ev EventFamilySelected) {
	t.emit(ctx, "euclo.family.selected", ev)
}

func (t *EucloTelemetry) EmitIngestionComplete(ctx context.Context, ev EventIngestionComplete) {
	t.emit(ctx, "euclo.ingestion.complete", ev)
}

func (t *EucloTelemetry) EmitStreamRequested(ctx context.Context, ev EventStreamRequested) {
	t.emit(ctx, "euclo.stream.requested", ev)
}

func (t *EucloTelemetry) EmitCapabilityClassified(ctx context.Context, ev EventCapabilityClassified) {
	t.emit(ctx, "euclo.capability.classified", ev)
}

func (t *EucloTelemetry) EmitRouteSelected(ctx context.Context, ev EventRouteSelected) {
	t.emit(ctx, "euclo.route.selected", ev)
}

func (t *EucloTelemetry) EmitGateResult(ctx context.Context, ev EventGateResult) {
	t.emit(ctx, "euclo.gate.result", ev)
}

func (t *EucloTelemetry) EmitFrameEmitted(ctx context.Context, ev EventFrameEmitted) {
	t.emit(ctx, "euclo.frame.emitted", ev)
}

func (t *EucloTelemetry) EmitFrameResolved(ctx context.Context, ev EventFrameResolved) {
	t.emit(ctx, "euclo.frame.resolved", ev)
}

func (t *EucloTelemetry) EmitJobSubmitted(ctx context.Context, ev EventJobSubmitted) {
	t.emit(ctx, "euclo.job.submitted", ev)
}

func (t *EucloTelemetry) EmitJobCompleted(ctx context.Context, ev EventJobCompleted) {
	t.emit(ctx, "euclo.job.completed", ev)
}

func (t *EucloTelemetry) EmitStepCompleted(ctx context.Context, ev EventStepCompleted) {
	t.emit(ctx, "euclo.step.completed", ev)
}

func (t *EucloTelemetry) EmitExecutionComplete(ctx context.Context, ev EventExecutionComplete) {
	t.emit(ctx, "euclo.execution.complete", ev)
}

func (t *EucloTelemetry) emit(ctx context.Context, eventType string, payload any) {
	t.sink.Emit(ctx, core.TelemetryEvent{
		Type:      eventType,
		Timestamp: time.Now(),
		Payload:   payload,
	})
}

type noopTelemetry struct{}

func (n *noopTelemetry) Emit(ctx context.Context, event core.TelemetryEvent) error {
	return nil
}
```

**File: `reporting/events.go`**

```go
package reporting

import (
	"time"
)

// EventIntakeComplete signals intake pipeline completion.
type EventIntakeComplete struct {
	TaskID              string    `json:"task_id"`
	SessionID           string    `json:"session_id"`
	Seq                 int       `json:"seq"`
	OccurredAt          time.Time `json:"occurred_at"`
	WinningFamily       string    `json:"winning_family"`
	Confidence          float64   `json:"confidence"`
	Ambiguous           bool      `json:"ambiguous"`
	CapabilityCount     int       `json:"capability_count"`
	HasStreamResult     bool      `json:"has_stream_result"`
	ClassificationSource string   `json:"classification_source"`
}

// EventFamilySelected signals family selection.
type EventFamilySelected struct {
	TaskID       string    `json:"task_id"`
	SessionID    string    `json:"session_id"`
	Seq          int       `json:"seq"`
	OccurredAt   time.Time `json:"occurred_at"`
	FamilyID     string    `json:"family_id"`
	Confidence   float64   `json:"confidence"`
	Keywords     []string  `json:"keywords"`
}

// EventIngestionComplete signals file ingestion completion.
type EventIngestionComplete struct {
	TaskID      string    `json:"task_id"`
	SessionID   string    `json:"session_id"`
	Seq         int       `json:"seq"`
	OccurredAt  time.Time `json:"occurred_at"`
	Mode        string    `json:"mode"`
	FileCount   int       `json:"file_count"`
	ChunkCount  int       `json:"chunk_count"`
	ErrorCount  int       `json:"error_count"`
}

// EventStreamRequested signals a context stream request.
type EventStreamRequested struct {
	TaskID     string    `json:"task_id"`
	SessionID  string    `json:"session_id"`
	Seq        int       `json:"seq"`
	OccurredAt time.Time `json:"occurred_at"`
	Query      string    `json:"query"`
	MaxTokens  int       `json:"max_tokens"`
	Mode       string    `json:"mode"`
}

// EventCapabilityClassified signals tier-2 classification.
type EventCapabilityClassified struct {
	TaskID           string   `json:"task_id"`
	SessionID        string   `json:"session_id"`
	Seq              int      `json:"seq"`
	OccurredAt       time.Time `json:"occurred_at"`
	FamilyID         string   `json:"family_id"`
	Capabilities     []string `json:"capabilities"`
	Operator         string   `json:"operator"`
	LLMCalls         int      `json:"llm_calls"`
}

// EventRouteSelected signals route selection.
type EventRouteSelected struct {
	TaskID         string    `json:"task_id"`
	SessionID      string    `json:"session_id"`
	Seq            int       `json:"seq"`
	OccurredAt     time.Time `json:"occurred_at"`
	FamilyID       string    `json:"family_id"`
	RouteKind      string    `json:"route_kind"`
	RouteID        string    `json:"route_id"`
	CandidateCount int      `json:"candidate_count"`
	FallbackTaken  bool     `json:"fallback_taken"`
}

// EventGateResult signals policy gate outcome.
type EventGateResult struct {
	TaskID     string    `json:"task_id"`
	SessionID  string    `json:"session_id"`
	Seq        int       `json:"seq"`
	OccurredAt time.Time `json:"occurred_at"`
	GateID     string    `json:"gate_id"`
	Passed     bool      `json:"passed"`
	Decision   string    `json:"decision"` // "allow", "deny", "hitl_approved"
}

// EventFrameEmitted signals interaction frame emission.
type EventFrameEmitted struct {
	TaskID     string    `json:"task_id"`
	SessionID  string    `json:"session_id"`
	Seq        int       `json:"seq"`
	OccurredAt time.Time `json:"occurred_at"`
	FrameID    string    `json:"frame_id"`
	FrameType  string    `json:"frame_type"`
	SlotCount  int       `json:"slot_count"`
}

// EventFrameResolved signals interaction frame resolution.
type EventFrameResolved struct {
	TaskID      string    `json:"task_id"`
	SessionID   string    `json:"session_id"`
	Seq         int       `json:"seq"`
	OccurredAt  time.Time `json:"occurred_at"`
	FrameID     string    `json:"frame_id"`
	ChosenSlot  string    `json:"chosen_slot"`
	RespondedBy string    `json:"responded_by"`
}

// EventJobSubmitted signals background job submission.
type EventJobSubmitted struct {
	TaskID     string    `json:"task_id"`
	SessionID  string    `json:"session_id"`
	Seq        int       `json:"seq"`
	OccurredAt time.Time `json:"occurred_at"`
	JobID      string    `json:"job_id"`
	RouteID    string    `json:"route_id"`
	ExecutionMode string `json:"execution_mode"`
}

// EventJobCompleted signals background job completion.
type EventJobCompleted struct {
	TaskID      string    `json:"task_id"`
	SessionID   string    `json:"session_id"`
	Seq         int       `json:"seq"`
	OccurredAt  time.Time `json:"occurred_at"`
	JobID       string    `json:"job_id"`
	Status      string    `json:"status"` // "completed", "failed", "cancelled"
	Error       string    `json:"error,omitempty"`
	DurationMs  int64     `json:"duration_ms"`
}

// EventStepCompleted signals recipe step completion.
type EventStepCompleted struct {
	TaskID      string    `json:"task_id"`
	SessionID   string    `json:"session_id"`
	Seq         int       `json:"seq"`
	OccurredAt  time.Time `json:"occurred_at"`
	StepID      string    `json:"step_id"`
	RecipeID    string    `json:"recipe_id"`
	Paradigm    string    `json:"paradigm"`
	Success     bool      `json:"success"`
	DurationMs  int64     `json:"duration_ms"`
}

// EventExecutionComplete signals overall execution completion.
type EventExecutionComplete struct {
	TaskID      string    `json:"task_id"`
	SessionID   string    `json:"session_id"`
	Seq         int       `json:"seq"`
	OccurredAt  time.Time `json:"occurred_at"`
	Outcome     string    `json:"outcome"` // "success", "partial_success", "failed"
	OutcomeKind string    `json:"outcome_kind"`
	StepCount   int       `json:"step_count"`
	LLMCalls    int       `json:"llm_calls"`
	TokenUsage  int       `json:"token_usage"`
}
```

---

## 4. Test Requirements

All implementations must pass the following test categories:

### 4.1 Unit Tests (Per Package)

| Package | Test Coverage |
|---------|---------------|
| `services` | Registration, wire ordering, error handling |
| `intake` | Pipeline execution, stream mode enforcement, classifier integration |
| `orchestrate` | Graph wiring validation, route fork behavior, dispatch routing |
| `policy` | Gate outcomes (allow/deny/ask), HITL flow, timeout handling |
| `interaction` | Frame emission, sequence numbering, resume detection |
| `reporting` | Event emission ordering, payload completeness |

### 4.2 Integration Tests

- **Standard path:** Implementation → classification → dispatch → execution
- **Analysis path:** "explain" → review family → query capability
- **Recipe execution:** Multi-step with carry_forward, per-step gates
- **HITL flow:** Ask → frame emission → approval → continuation
- **Resume:** Prior classification in envelope → skip intake → continue

### 4.3 Boundary Enforcement

- No imports of `REFERENCE_ONLY/euclo_broken_legacy`
- No direct `platform/` imports in `named/euclo`
- No direct `archaeo/` imports in primary execution path

---

## 5. Migration Guide

### 5.1 Update `agent.go` Initialize

```go
func (a *Agent) Initialize(config *core.Config) error {
	if a.initialized {
		return nil
	}

	// Validate required services
	if a.env.Registry == nil {
		return fmt.Errorf("CapabilityRegistry is required but not provided")
	}
	if a.env.IndexManager == nil {
		return fmt.Errorf("IndexManager is required but not provided")
	}

	// Use new services package
	svcReg := services.NewRegistration()
	regFuncs := svcReg.AgentRegistrationFuncs()

	if err := regFuncs.RegisterCapabilities(a.env); err != nil {
		return fmt.Errorf("failed to register capabilities: %w", err)
	}

	if err := regFuncs.RegisterPromptProviders(a.env); err != nil {
		return fmt.Errorf("failed to register prompt providers: %w", err)
	}

	recipeReg, err := regFuncs.LoadRecipes()
	if err != nil {
		return fmt.Errorf("failed to load recipes: %w", err)
	}
	a.recipeRegistry = recipeReg.(*recipe.RecipeRegistry)

	// Build family registry
	a.familyRegistry = families.NewKeywordFamilyRegistry()
	families.RegisterBuiltins(a.familyRegistry)

	// Initialize capability classifier if model available
	if a.config.CapabilityClassifierModel != nil {
		a.capabilityClassifier = intake.NewLLMCapabilityClassifier(
			a.config.CapabilityClassifierModel,
		)
	}

	a.initialized = true
	return nil
}
```

### 5.2 Update `BuildGraph` to pass all dependencies

```go
func (a *Agent) BuildGraph(task *core.Task) (*agentgraph.Graph, error) {
	// ... initialization check ...

	resumeClassification, resumeRouteSelection := a.resumeStateSnapshot()
	
	rootGraph, err := orchestrate.NewRootGraph(
		orchestrate.WithWorkspaceEnvironment(a.env),
		orchestrate.WithWorkspace(workspaceRootPath(a.env)),
		orchestrate.WithCapabilityRegistry(a.env.Registry),
		orchestrate.WithRecipeRegistry(a.recipeRegistry),
		orchestrate.WithFamilyRegistry(a.familyRegistry),
		orchestrate.WithStreamTrigger(a.env.StreamTrigger),
		orchestrate.WithMaxStreamTokens(a.config.MaxStreamTokens),
		orchestrate.WithDefaultStreamMode(a.config.DefaultStreamMode),
		orchestrate.WithCapabilityClassifier(a.capabilityClassifier),
		orchestrate.WithPermissionManager(a.env.PermissionManager),
		orchestrate.WithHITLBroker(a.env.HITLBroker),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create root graph: %w", err)
	}

	graph := rootGraph.Graph()
	if graph == nil {
		return nil, fmt.Errorf("root graph is nil")
	}

	// Set start node based on resume state
	start := "euclo.intake"
	switch {
	case resumeClassification != nil && resumeRouteSelection != nil:
		start = "euclo.policy_gate"
	case resumeClassification != nil:
		start = "euclo.dispatch"
	}
	
	if err := rootGraph.SetStart(start); err != nil {
		return nil, fmt.Errorf("failed to set start node: %w", err)
	}

	return graph, nil
}
```

---

## 6. Acceptance Criteria

The implementation is complete when:

1. ✅ All stub implementations replaced with production code
2. ✅ `services` package created and integrated
3. ✅ Task seeding in `agent.go:Execute()` implemented
4. ✅ `IntakePipelineNode` performs real classification (tier-1 + tier-2)
5. ✅ `routeMatchesFamily` filters correctly by instruction analysis
6. ✅ `RouteForkNode` returns proper `NextNode` for conditional routing
7. ✅ Root graph validates with `graph.Validate()` returning nil
8. ✅ Policy gates integrate with `authorization.PermissionManager`
9. ✅ Interaction frames emit to envelope and `event.Log`
10. ✅ Telemetry events emit for all routing decisions
11. ✅ All Phase 15 integration tests pass
12. ✅ No imports of `REFERENCE_ONLY/` from production code
13. ✅ `go test ./named/euclo/...` passes

---

*End of Specification*


## Executive Summary

This document provides both the detailed engineering specifications and a 15-phase implementation plan to finish the remaining Euclo cleanup. The current codebase already has the service-registration layer, state accessors, intake pipeline, routing fixes, and agent initialization in place; the remaining gaps are concentrated in the downstream policy, interaction, telemetry, ingestion, and orchestration refinements.

**Core Principles:**
- No stubs, no shims, no aliasing, no compatibility layers
- Full production implementations only
- All TODOs and Phase markers removed

---

## 1. Architecture Context

### 1.1 Framework Services Architecture

```
framework/agentenv/          # Defines interfaces and WorkspaceEnvironment
├── environment.go           # WorkspaceEnvironment struct
├── service.go              # Service, ServiceManager interfaces
├── registration.go         # AgentRegistrationFuncs pattern

framework/services/          # Implements base framework services
├── capability_bundle.go    # Capability bundle management
├── prompt_registry.go      # Prompt registry integration

named/euclo/services/        # NEW: Euclo-specific service registrations
├── register.go            # Main registration entrypoint
├── capabilities.go         # Capability provider wiring
├── prompts.go             # Prompt provider wiring
└── recipes.go             # Recipe loader registration
```

---

## 2. Multi-Phase Implementation Plan (15 Phases)

### Phase 1: Foundation - Services Package

**Goal:** Establish `named/euclo/services` package for framework integration.

**Implementation Dependencies:**
- `framework/agentenv` (existing)
- `named/euclo/relurpicabilities` (existing)
- `named/euclo/promptprovider` (existing)
- `named/euclo/recipetemplates` (existing)

**Files to Implement:**
1. `named/euclo/services/register.go` - Registration struct with AgentRegistrationFuncs
2. `named/euclo/services/capabilities.go` - defaultCapabilityRegistrar
3. `named/euclo/services/prompts.go` - defaultPromptRegistrar
4. `named/euclo/services/recipes.go` - defaultRecipeLoader
5. `named/euclo/services/doc.go` - Package documentation
6. Update `named/euclo/registration.go` - Delegate to services package

**Unit Tests:**
- `services/register_test.go` - Test NewRegistration, options override
- `services/capabilities_test.go` - Test RegisterAll, nil registry handling
- `services/prompts_test.go` - Test provider registration, duplicate handling
- `services/recipes_test.go` - Test LoadAll, error propagation

**Exit Criteria:**
- [ ] All 6 files implemented with no TODOs
- [ ] `go test ./named/euclo/services/...` passes
- [ ] No circular imports between named/euclo and ayenitd
- [ ] Boundary check: No REFERENCE_ONLY/, platform/, archaeo/ imports

---

### Phase 2: State Management Enhancement

**Goal:** Add state accessors for routing, policy, and telemetry.

**Implementation Dependencies:**
- `framework/contextdata`
- `named/euclo/state/` (existing)

**Files to Implement:**
1. `named/euclo/state/keys.go` - Add KeyDispatchRouteKind, KeyPolicyDecision
2. `named/euclo/state/accessors.go` - Add GetString, GetPolicyDecision, SetPolicyDecision, GetDispatchRouteKind, SetDispatchRouteKind
3. `named/euclo/state/types.go` - Add ResumeState struct

**Unit Tests:**
- `state/accessors_test.go` - Test all accessor round-trips, nil envelope handling
- `state/keys_test.go` - Test key naming conventions

**Exit Criteria:**
- [ ] All accessor functions type-safe
- [ ] Unit tests cover nil envelope handling
- [ ] `go test ./named/euclo/state/...` passes

---

### Phase 3: LLM Capability Classifier

**Goal:** Implement LLM-backed tier-2 capability classifier.

**Implementation Dependencies:**
- `framework/core` for LanguageModel
- `named/euclo/intake/types.go`

**Files to Implement:**
1. `named/euclo/intake/capability_classifier.go` - CapabilityClassifier interface, LLMCapabilityClassifier struct, Classify method, buildPrompt, parseResponse
2. Update `named/euclo/intake/types.go` - Add interface definition

**Unit Tests:**
- `intake/capability_classifier_test.go` - Test Classify flow, parseResponse edge cases, JSON extraction, operator normalization

**Exit Criteria:**
- [ ] Classify performs LLM call and parses JSON response
- [ ] Handles malformed JSON gracefully
- [ ] `go test ./named/euclo/intake/... -run Classifier` passes

---

### Phase 4: Intake Pipeline Implementation

**Goal:** Replace stub intake pipeline with full tier-1 + tier-2 classification.

**Implementation Dependencies:**
- Phase 2 (state accessors)
- Phase 3 (LLM classifier)
- `framework/contextstream`
- `named/euclo/families`

**Files to Implement:**
1. `named/euclo/intake/pipeline.go` - Complete IntakePipelineNode.Execute with 8-step pipeline: task extraction, normalize, tier-1 classify, stream request, tier-2 classify, build IntentClassification, write to state, return result

**Unit Tests:**
- `intake/pipeline_test.go` - Test all 8 steps, stream mode enforcement, classifier fallback, state persistence
- `intake/pipeline_integration_test.go` - Test with mocked dependencies

**Exit Criteria:**
- [ ] Full pipeline implemented, no hardcoded values
- [ ] Stream mode enforcement (background → blocking when tier2 needed)
- [ ] All classification results written to envelope
- [ ] `go test ./named/euclo/intake/...` passes

---

### Phase 5: Routing Fixes

**Goal:** Fix `routeMatchesFamily` bug that returns true for empty family.

**Implementation Dependencies:**
- `named/euclo/orchestrate/dispatching.go`

**Files to Implement:**
1. `named/euclo/orchestrate/dispatching.go` - Replace routeMatchesFamily with: instruction analysis for empty family, keyword matching (analysis → query, mutation → modify), explicit family match otherwise

**Unit Tests:**
- `orchestrate/dispatching_test.go` - Test empty family + analysis instruction, empty family + mutation instruction, explicit family match, no match scenarios

**Exit Criteria:**
- [ ] Analysis keywords route to query/lookup capabilities
- [ ] Mutation keywords route to modification capabilities
- [ ] Empty family no longer matches all capabilities
- [ ] `go test ./named/euclo/orchestrate/... -run Route` passes

---

### Phase 6: Route Fork ConditionalNode

**Goal:** Implement RouteForkNode as proper ConditionalNode.

**Implementation Dependencies:**
- Phase 2 (state accessors)
- `framework/agentgraph`

**Files to Implement:**
1. `named/euclo/orchestrate/route_fork.go` - RouteForkNode struct, Execute reads route_kind from state, returns NextNode in Result

**Unit Tests:**
- `orchestrate/route_fork_test.go` - Test Execute returns "recipe" or "capability", error on missing route_kind, fallback to legacy key

**Exit Criteria:**
- [ ] Execute returns NextNode for conditional routing
- [ ] No ambiguous-transition errors
- [ ] `go test ./named/euclo/orchestrate/... -run Fork` passes

---

### Phase 7: Policy Gates with Authorization

**Goal:** Implement policy gate integration with framework authorization.

**Implementation Dependencies:**
- Phase 2 (state accessors)
- Phase 8 (interaction frames)
- `framework/authorization`

**Files to Implement:**
1. `named/euclo/policy/gates.go` - PermissionManager interface, HITLBroker interface, GateNode with Execute handling Allow/Deny/Ask, handleHITL for Ask flow

**Unit Tests:**
- `policy/gates_test.go` - Test Allow/Deny/Ask decisions, HITL flow, timeout handling, frame emission

**Exit Criteria:**
- [ ] GateNode integrates with authorization.PermissionManager
- [ ] HITL flow implemented with timeout
- [ ] All decision paths tested
- [ ] `go test ./named/euclo/policy/...` passes

---

### Phase 8: Interaction Frame System

**Goal:** Implement frame emission and resume detection.

**Implementation Dependencies:**
- `framework/event`
- `named/euclo/interaction/frames.go`

**Files to Implement:**
1. `named/euclo/interaction/emit.go` - EmitFrame, getNextFrameSeq, ResumeFrame, NewHITLApprovalFrame

**Unit Tests:**
- `interaction/emit_test.go` - Test sequence assignment, envelope writes, event publication, ResumeFrame detection

**Exit Criteria:**
- [ ] Frames emitted to envelope with sequences
- [ ] Events published to event.Log
- [ ] Resume detection works correctly
- [ ] `go test ./named/euclo/interaction/...` passes

---

### Phase 9: Telemetry and Reporting

**Goal:** Implement typed telemetry emit helpers.

**Implementation Dependencies:**
- `framework/core`

**Files to Implement:**
1. `named/euclo/reporting/telemetry.go` - EucloTelemetry with 12 Emit* methods
2. `named/euclo/reporting/events.go` - 16 Event* structs

**Unit Tests:**
- `reporting/telemetry_test.go` - Test all Emit methods, noop telemetry
- `reporting/events_test.go` - Test JSON marshaling

**Exit Criteria:**
- [ ] All 16 event types defined
- [ ] All Emit methods implemented
- [ ] `go test ./named/euclo/reporting/...` passes

---

### Phase 10: Graph Wiring and Orchestration

**Goal:** Reimplement root graph wiring with proper nodes and edges.

**Implementation Dependencies:**
- Phase 1 (services)
- Phase 4 (intake pipeline)
- Phase 6 (route fork)
- Phase 7 (policy gate)

**Files to Implement:**
1. `named/euclo/orchestrate/graph.go` - RootGraphConfig, 12 Option functions, NewRootGraph, buildNodes (15 nodes), wireEdges with conditional routing

**Unit Tests:**
- `orchestrate/graph_test.go` - Test all options, node creation, edge wiring, graph validation
- Integration tests for full graph execution

**Exit Criteria:**
- [ ] All 15 nodes created
- [ ] All edges wired correctly
- [ ] graph.Validate() returns nil
- [ ] `go test ./named/euclo/orchestrate/...` passes

---

### Phase 11: Task Seeding and Agent Integration

**Goal:** Implement task seeding and update agent integration.

**Implementation Dependencies:**
- Phase 1 (services)
- Phase 2 (state accessors)
- Phase 10 (root graph)

**Files to Implement:**
1. `named/euclo/agent.go` - Update Execute() with task seeding, Initialize() with services package, BuildGraph() with all options
2. `named/euclo/config.go` - Add CapabilityClassifierModel field

**Unit Tests:**
- `agent_test.go` - Test task seeding, Initialize flow, BuildGraph options, resume state handling

**Exit Criteria:**
- [ ] Execute seeds all task fields to envelope
- [ ] Initialize uses services package
- [ ] Resume state determines correct start node
- [ ] `go test ./named/euclo/... -run Agent` passes

---

### Phase 12: Recipe Compiler and Graph Builder

**Goal:** Implement recipe compilation and graph building.

**Implementation Dependencies:**
- `named/euclo/recipes/schema.go`
- `framework/agentgraph`

**Files to Implement:**
1. `named/euclo/recipes/compiler.go` - Compile ThoughtRecipe to ExecutionPlan
2. `named/euclo/recipes/graph_builder.go` - Build agentgraph.Graph from ExecutionPlan
3. `named/euclo/recipes/aliases.go` - Alias resolution
4. Update `named/euclo/recipes/types.go` - Add ExecutionPlan

**Unit Tests:**
- `recipes/compiler_test.go` - Test sequence, parallel, conditional compilation
- `recipes/graph_builder_test.go` - Test graph creation, edge wiring
- `recipes/aliases_test.go` - Test alias resolution

**Exit Criteria:**
- [ ] Compiler transforms recipes to execution plans
- [ ] GraphBuilder creates valid graphs
- [ ] Parallel and conditional groups work
- [ ] `go test ./named/euclo/recipes/...` passes

---

### Phase 13: Ingestion Pipeline Integration

**Goal:** Implement framework-owned ingestion.

**Implementation Dependencies:**
- `framework/ingestion`
- `framework/agentenv`

**Files to Implement:**
1. `named/euclo/ingestion/node.go` - IngestionNode using framework/ingestion.Pipeline

**Unit Tests:**
- `ingestion/node_test.go` - Test all ingestion modes, framework integration

**Exit Criteria:**
- [ ] IngestionNode uses framework pipeline
- [ ] All modes supported
- [ ] `go test ./named/euclo/ingestion/...` passes

---

### Phase 14: Background Job System

**Goal:** Implement background job submission.

**Implementation Dependencies:**
- `framework/agentenv/service.go`
- Phase 9 (telemetry)

**Files to Implement:**
1. `named/euclo/orchestrate/background.go` - BackgroundJobNode for async execution

**Unit Tests:**
- `orchestrate/background_test.go` - Test job submission, completion callbacks

**Exit Criteria:**
- [ ] BackgroundJobNode submits to scheduler
- [ ] Telemetry events emitted
- [ ] Completion handled
- [ ] `go test ./named/euclo/orchestrate/... -run Background` passes

---

### Phase 15: Integration and Validation

**Goal:** Full integration testing and validation.

**Implementation Dependencies:**
- All previous phases
- `testsuite/agenttest`

**Files to Implement:**
1. `named/euclo/integration_test.go` - Standard path, analysis path, recipe execution, HITL flow, resume, background jobs, error recovery
2. `named/euclo/boundary_test.go` - Import restriction verification

**Unit Tests:**
- Full end-to-end integration tests
- Boundary enforcement tests

**Exit Criteria (Final):**
- [ ] All 14 phases complete and passing
- [ ] `go test ./named/euclo/...` passes
- [ ] Integration tests cover all paths
- [ ] No REFERENCE_ONLY/ imports
- [ ] No direct platform/ imports
- [ ] No direct archaeo/ imports
- [ ] All TODOs resolved

---

## 3. Key Implementation Specifications

### 3.1 Services Package

See Section 4.1 in original engineering spec for full code. Key interfaces:

```go
type CapabilityRegistrar interface {
    RegisterAll(env agentenv.WorkspaceEnvironment) error
}
type PromptRegistrar interface {
    RegisterAll(env agentenv.WorkspaceEnvironment) error
}
type RecipeLoader interface {
    LoadAll() (*recipe.RecipeRegistry, error)
}
```

### 3.2 Task Seeding

```go
// In agent.go Execute()
if env != nil && task != nil {
    env.SetWorkingValue("task.input", task, contextdata.MemoryClassTask)
    env.SetWorkingValue("task.id", task.ID, contextdata.MemoryClassTask)
    env.SetWorkingValue("task.instruction", task.Instruction, contextdata.MemoryClassTask)
    env.SetWorkingValue("task.type", task.Type, contextdata.MemoryClassTask)
    env.SetWorkingValue("task.context", task.Context, contextdata.MemoryClassTask)
    if task.Metadata != nil {
        env.SetWorkingValue("task.metadata", task.Metadata, contextdata.MemoryClassTask)
    }
}
```

### 3.3 Routing Fix

```go
func routeMatchesFamily(desc core.CapabilityDescriptor, family, instruction string) bool {
    family = strings.ToLower(strings.TrimSpace(family))
    instruction = strings.ToLower(strings.TrimSpace(instruction))
    
    if family == "" {
        return matchesInstructionAnalysis(desc, instruction)
    }
    return strings.Contains(strings.ToLower(desc.ID), family) ||
        descHasFamilyTag(desc, family)
}
```

### 3.4 Route Fork

```go
func (n *RouteForkNode) Execute(ctx context.Context, env *contextdata.Envelope) (*agentgraph.Result, error) {
    routeKind := state.GetString(env, state.KeyDispatchRouteKind)
    if routeKind == "" {
        return nil, fmt.Errorf("route_kind not found")
    }
    return &agentgraph.Result{
        NodeID:   n.id,
        Success:  true,
        NextNode: routeKind, // "recipe" or "capability"
    }, nil
}
```

### 3.5 Policy Gate

```go
func (n *GateNode) Execute(ctx context.Context, env *contextdata.Envelope) (*agentgraph.Result, error) {
    decision, ok := state.GetPolicyDecision(env)
    if !ok {
        return &agentgraph.Result{NodeID: n.id, Success: true, Data: map[string]any{"gate": "skipped"}}, nil
    }
    
    authDecision := n.permissionManager.Check(ctx, authorization.PermissionRequest{
        Resource: decision.RouteID, Action: "execute", RiskClass: decision.RiskClass,
    })
    
    switch authDecision {
    case authorization.Allow:
        return &agentgraph.Result{NodeID: n.id, Success: true, Data: map[string]any{"gate": "passed"}}, nil
    case authorization.Deny:
        return &agentgraph.Result{NodeID: n.id, Success: false, Err: errors.New("denied")}, nil
    case authorization.Ask:
        return n.handleHITL(ctx, env, decision)
    }
    return nil, fmt.Errorf("unknown decision: %v", authDecision)
}
```

---

## 4. Acceptance Criteria

1. All stub implementations replaced with production code
2. Services package created and integrated
3. Task seeding in agent.go:Execute() implemented
4. IntakePipelineNode performs real classification (tier-1 + tier-2)
5. routeMatchesFamily filters correctly by instruction analysis
6. RouteForkNode returns proper NextNode for conditional routing
7. Root graph validates with graph.Validate() returning nil
8. Policy gates integrate with authorization.PermissionManager
9. Interaction frames emit to envelope and event.Log
10. Telemetry events emit for all routing decisions
11. All Phase 15 integration tests pass
12. `go test ./named/euclo/...` passes

---

*End of Plan*

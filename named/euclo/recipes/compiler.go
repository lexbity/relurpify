package recipe

import (
	"fmt"
	"strings"
)

// CompiledNode represents a compiled recipe step as a graph node.
// It is retained for legacy callers and tests.
type CompiledNode struct {
	ID           string
	Type         string
	Description  string
	Config       map[string]interface{}
	Captures     map[string]string
	Bindings     map[string]string
	Dependencies []string
}

// Compiler compiles recipes to graph nodes or execution plans.
type Compiler struct{}

// NewCompiler creates a new recipe compiler.
func NewCompiler() *Compiler {
	return &Compiler{}
}

// Compile compiles a legacy recipe to a list of graph nodes.
func (c *Compiler) Compile(recipe *ThoughtRecipe) ([]CompiledNode, error) {
	if recipe == nil {
		return nil, fmt.Errorf("recipe is nil")
	}
	if err := recipe.Validate(); err != nil {
		return nil, fmt.Errorf("recipe validation failed: %w", err)
	}

	steps := recipe.StepList()
	recipeKey := recipe.ID
	if strings.TrimSpace(recipeKey) == "" {
		recipeKey = recipe.EffectiveName()
	}
	nodes := make([]CompiledNode, len(steps))
	for i, step := range steps {
		node := CompiledNode{
			ID:           step.ID,
			Type:         step.Type,
			Description:  step.Description,
			Config:       step.Config,
			Captures:     step.Captures,
			Bindings:     step.Bindings,
			Dependencies: step.Dependencies,
		}
		if node.Bindings != nil {
			resolved := make(map[string]string, len(node.Bindings))
			for key, value := range node.Bindings {
				resolved[key] = resolveBinding(value)
			}
			node.Bindings = resolved
		}
		if node.Captures != nil {
			resolved := make(map[string]string, len(node.Captures))
			for key, value := range node.Captures {
				resolved[key] = resolveCaptureKey(recipeKey, key, value)
			}
			node.Captures = resolved
		}
		nodes[i] = node
	}
	return nodes, nil
}

// CompilePlan compiles a thought recipe into a spec-shaped execution plan.
func (c *Compiler) CompilePlan(recipe *ThoughtRecipe, resolver *AliasResolver) (*ExecutionPlan, error) {
	if recipe == nil {
		return nil, fmt.Errorf("recipe is nil")
	}
	if err := recipe.Validate(); err != nil {
		return nil, fmt.Errorf("recipe validation failed: %w", err)
	}
	if resolver == nil {
		resolver = NewAliasResolver(recipe)
	}

	steps := recipe.StepList()
	recipeKey := recipe.ID
	if strings.TrimSpace(recipeKey) == "" {
		recipeKey = recipe.EffectiveName()
	}
	plan := &ExecutionPlan{
		Recipe: recipe,
		Steps:  make([]ExecutionStep, 0, len(steps)),
	}
	for _, step := range steps {
		captures := resolveCaptures(step.Captures, resolver, recipeKey)
		for _, alias := range step.Parent.Context.Capture {
			alias = strings.TrimSpace(alias)
			if alias == "" {
				continue
			}
			if captures == nil {
				captures = make(map[string]string)
			}
			if _, exists := captures[alias]; !exists {
				captures[alias] = resolver.Resolve(alias)
			}
		}
		executionStep := ExecutionStep{
			ID:           step.ID,
			Paradigm:     step.Parent.Paradigm,
			Prompt:       step.Parent.Prompt,
			Mutation:     step.Mutation,
			HITL:         step.HITL,
			Stream:       cloneStreamSpec(step.Parent.Context.Stream),
			Ingest:       cloneIngestSpec(step.Parent.Context.Ingest),
			Fallback:     cloneStepAgent(step.Fallback),
			Inherit:      append([]string(nil), step.Parent.Context.Inherit...),
			Capture:      append([]string(nil), step.Parent.Context.Capture...),
			Dependencies: append([]string(nil), step.Dependencies...),
			Bindings:     resolveBindings(step.Bindings),
			Captures:     captures,
			Step:         step,
		}
		plan.Steps = append(plan.Steps, executionStep)
	}
	return plan, nil
}

// Compile is the spec-shaped compiler entry point.
func Compile(recipe *ThoughtRecipe, resolver *AliasResolver) (*ExecutionPlan, error) {
	return NewCompiler().CompilePlan(recipe, resolver)
}

func resolveBindings(bindings map[string]string) map[string]string {
	if len(bindings) == 0 {
		return nil
	}
	resolved := make(map[string]string, len(bindings))
	for key, value := range bindings {
		resolved[key] = resolveBinding(value)
	}
	return resolved
}

func resolveCaptures(captures map[string]string, resolver *AliasResolver, recipeName string) map[string]string {
	if len(captures) == 0 {
		return nil
	}
	resolved := make(map[string]string, len(captures))
	for key, value := range captures {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			resolved[key] = trimmed
			continue
		}
		if resolver != nil {
			resolved[key] = resolver.Resolve(key)
			continue
		}
		resolved[key] = resolveCaptureKey(recipeName, key, value)
	}
	return resolved
}

func cloneStreamSpec(spec *RecipeStreamSpec) *RecipeStreamSpec {
	if spec == nil {
		return nil
	}
	cp := *spec
	return &cp
}

func cloneIngestSpec(spec *RecipeIngestSpec) *RecipeIngestSpec {
	if spec == nil {
		return nil
	}
	cp := *spec
	return &cp
}

func cloneStepAgent(agent *RecipeStepAgent) *RecipeStepAgent {
	if agent == nil {
		return nil
	}
	cp := *agent
	if agent.Context.Stream != nil {
		cp.Context.Stream = cloneStreamSpec(agent.Context.Stream)
	}
	if agent.Context.Ingest != nil {
		cp.Context.Ingest = cloneIngestSpec(agent.Context.Ingest)
	}
	cp.Context.Inherit = append([]string(nil), agent.Context.Inherit...)
	cp.Context.Capture = append([]string(nil), agent.Context.Capture...)
	return &cp
}

// resolveBinding resolves a binding reference to an envelope key.
func resolveBinding(binding string) string {
	switch binding {
	case "{{instruction}}":
		return "euclo.task.envelope.instruction"
	case "{{context_hint}}":
		return "euclo.task.envelope.context_hint"
	case "{{family_hint}}":
		return "euclo.task.envelope.family_hint"
	default:
		return binding
	}
}

// resolveCaptureKey resolves a capture key to an envelope key.
func resolveCaptureKey(recipeName, captureName, value string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fmt.Sprintf("euclo.recipe.%s.%s", sanitizeAliasComponent(recipeName), sanitizeAliasComponent(captureName))
}

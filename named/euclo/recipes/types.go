package recipe

import (
	"codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
)

// CompiledRecipe is a compiled version of ThoughtRecipe with resolved bindings.
type CompiledRecipe struct {
	Recipe      *ThoughtRecipe
	Steps       []CompiledStep
	Parallel    []CompiledParallelGroup
	Conditional []CompiledConditionalGroup
}

// CompiledStep is a compiled recipe step with resolved configuration.
type CompiledStep struct {
	Step     *RecipeStep
	Node     agentgraph.Node
	Config   map[string]any
	Captures map[string]string
	Bindings map[string]string
}

// CompiledParallelGroup is a compiled parallel execution group.
type CompiledParallelGroup struct {
	Group *ParallelGroup
	Steps []CompiledStep
	Merge MergePolicy
}

// CompiledConditionalGroup is a compiled conditional execution group.
type CompiledConditionalGroup struct {
	Group     *ConditionalGroup
	Condition string
	IfSteps   []CompiledStep
	ElseSteps []CompiledStep
}

// RecipeExecutionContext provides context for recipe execution.
type RecipeExecutionContext struct {
	Env         *contextdata.Envelope
	Captured    map[string]any
	CurrentStep *CompiledStep
	RecipeID    string
}

// ExecutionPlan is the spec-shaped compilation result for a thought recipe.
type ExecutionPlan struct {
	Recipe *ThoughtRecipe
	Steps  []ExecutionStep
}

// ExecutionStep carries the graph-time data for a single compiled recipe step.
type ExecutionStep struct {
	ID           string
	Paradigm     string
	Prompt       string
	Mutation     string
	HITL         string
	Stream       *RecipeStreamSpec
	Ingest       *RecipeIngestSpec
	Fallback     *RecipeStepAgent
	Inherit      []string
	Capture      []string
	Dependencies []string
	Bindings     map[string]string
	Captures     map[string]string
	Step         RecipeStep
}

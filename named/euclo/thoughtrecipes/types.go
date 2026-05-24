package thoughtrecipe

import (
	"codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
)

// CompiledThoughtRecipe is a compiled version of ThoughtRecipe with resolved bindings.
type CompiledThoughtRecipe struct {
	ThoughtRecipe *ThoughtRecipe
	Steps         []CompiledStep
	Parallel      []CompiledParallelGroup
	Conditional   []CompiledConditionalGroup
}

// CompiledStep is a compiled thoughtrecipe step with resolved configuration.
type CompiledStep struct {
	Step                *ThoughtRecipeStep
	Node                agentgraph.Node
	Type                string
	ClarificationConfig *ClarificationStepConfig
	Config              map[string]any
}

// CompiledParallelGroup is a compiled parallel execution group.
type CompiledParallelGroup struct {
	Group *ParallelGroup
	Steps []CompiledStep
	Merge string
}

// CompiledConditionalGroup is a compiled conditional execution group.
type CompiledConditionalGroup struct {
	Group     *ConditionalGroup
	Condition string
	IfSteps   []CompiledStep
	ElseSteps []CompiledStep
}

// ThoughtRecipeExecutionContext provides context for thoughtrecipe execution.
type ThoughtRecipeExecutionContext struct {
	Env             *contextdata.Envelope
	Captured        map[string]any
	CurrentStep     *CompiledStep
	ThoughtRecipeID string
}

// ExecutionPlan is the spec-shaped compilation result for a DSL thoughtrecipe.
type ExecutionPlan struct {
	ThoughtRecipe *ThoughtRecipe
	Agents        map[string]AgentBinding
	ToolScopes    []ToolScopeFrame
	Steps         []ExecutionStep
	Routes        []CompiledRouteGroup
	Pipelines     []CompiledPipelineGroup
	Warnings      []SemanticWarning
	RouteKind     TriggerRouteKind

	Parallel    []CompiledParallelGroup
	Conditional []CompiledConditionalGroup
}

// AgentBinding captures a thoughtrecipe-local agent declaration lowered to a runtime paradigm.
type AgentBinding struct {
	Name     string
	Paradigm string
	Span     SourceSpan
}

// ExecutionStep carries the graph-time data for a single compiled thoughtrecipe step.
type ExecutionStep struct {
	ID                  string
	Type                string
	Paradigm            string
	ToolScopes          []ToolScopeFrame
	EffectiveToolNames  []string
	Question            string
	Choices             []string
	ChoiceSource        string
	PipelineStages      []PipelineStageSpec
	Goal                string
	Sources             []string
	Directives          []string
	CaptureBindings     []CaptureBinding
	CapabilityID        string
	Prompt              string
	PromptID            string
	Mutation            string
	HITL                string
	Stream              *ThoughtRecipeStreamSpec
	Ingest              *ThoughtRecipeIngestSpec
	Fallback            *ThoughtRecipeStepAgent
	Inherit             []string
	Capture             []string
	Dependencies        []string
	ClarificationConfig *ClarificationStepConfig
	Step                ThoughtRecipeStep
}

// ToolScopeFrame captures one lexical tool allowlist contribution.
type ToolScopeFrame struct {
	ScopeKind string
	ToolNames []string
	Span      SourceSpan
}

// CompiledRouteGroup is a compiled constrained route block.
type CompiledRouteGroup struct {
	Group    *RouteGroup
	Branches []CompiledRouteBranch
}

// CompiledRouteBranch is a compiled route branch with normalized predicate and body.
type CompiledRouteBranch struct {
	Predicate *RoutePredicate
	Steps     []ExecutionStep
	IsElse    bool
}

// RouteGroup identifies a lowered route block.
type RouteGroup struct {
	positioned
	ID string
}

// CompiledPipelineGroup is a compiled pipeline block with ordered stages.
type CompiledPipelineGroup struct {
	Group  *PipelineGroup
	Stages []CompiledPipelineStage
}

// CompiledPipelineStage is a compiled pipeline stage with lowered steps.
type CompiledPipelineStage struct {
	Stage *PipelineStage
	Steps []ExecutionStep
}

// PipelineGroup identifies a lowered pipeline block.
type PipelineGroup struct {
	positioned
	ID string
}

// PipelineStageSpec carries a runtime-friendly pipeline stage description.
type PipelineStageSpec struct {
	Name  string
	Span  SourceSpan
	Steps []ExecutionStep
}

// RoutePredicate is a normalized constrained route predicate.
type RoutePredicate struct {
	positioned
	Raw      string
	Kind     string
	Subject  PathExpr
	Operator string
	Value    ValueExpr
}

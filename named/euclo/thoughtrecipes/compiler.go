package thoughtrecipe

import (
	"fmt"
	"strings"
)

// CompiledNode represents a compiled thoughtrecipe step as a graph node.
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

// Compiler compiles thoughtrecipes to graph nodes or execution plans.
type Compiler struct{}

// NewCompiler creates a new thoughtrecipe compiler.
func NewCompiler() *Compiler {
	return &Compiler{}
}

// Compile reports that the legacy graph-node compiler has been removed.
func (c *Compiler) Compile(thoughtrecipe *ThoughtRecipe) ([]CompiledNode, error) {
	_ = c
	if thoughtrecipe == nil {
		return nil, fmt.Errorf("thoughtrecipe is nil")
	}
	return nil, fmt.Errorf("legacy thoughtrecipe compiler removed; lower the AST directly into an execution plan")
}

// CompilePlan reports that the synthetic legacy compiler path has been removed.
func (c *Compiler) CompilePlan(thoughtrecipe *ThoughtRecipe) (*ExecutionPlan, error) {
	_ = c
	if thoughtrecipe == nil {
		return nil, fmt.Errorf("thoughtrecipe is nil")
	}
	return nil, fmt.Errorf("legacy thoughtrecipe compiler removed; lower the AST directly into an execution plan")
}

func executionParadigmForStep(step ThoughtRecipeStep) string {
	if paradigm := strings.TrimSpace(step.Parent.Paradigm); paradigm != "" {
		return paradigm
	}
	if paradigm := strings.TrimSpace(step.Type); paradigm != "" && validateStepParadigm(paradigm) == nil {
		return paradigm
	}
	return ""
}

func cloneAnyMap(m map[string]any) map[string]any {
	if len(m) == 0 {
		return nil
	}
	cp := make(map[string]any, len(m))
	for k, v := range m {
		cp[k] = v
	}
	return cp
}

func cloneStreamSpec(spec *ThoughtRecipeStreamSpec) *ThoughtRecipeStreamSpec {
	if spec == nil {
		return nil
	}
	cp := *spec
	return &cp
}

func cloneIngestSpec(spec *ThoughtRecipeIngestSpec) *ThoughtRecipeIngestSpec {
	if spec == nil {
		return nil
	}
	cp := *spec
	return &cp
}

func cloneStepAgent(agent *ThoughtRecipeStepAgent) *ThoughtRecipeStepAgent {
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

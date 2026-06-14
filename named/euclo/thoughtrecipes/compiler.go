package thoughtrecipe

import (
	"strings"

	"codeburg.org/lexbit/relurpify/named/euclo/surface"
)

// CompiledNode represents a compiled thoughtrecipe step as a graph node.
// It is retained for legacy callers and tests.
type CompiledNode struct {
	ID           string
	Type         string
	Description  string
	Config       map[string]any
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

func executionParadigmForStep(step surface.ThoughtRecipeStep) string {
	if paradigm := strings.TrimSpace(step.Parent.Paradigm); paradigm != "" {
		return paradigm
	}
	if paradigm := strings.TrimSpace(step.Type); paradigm != "" && validateStepParadigm(paradigm) == nil {
		return paradigm
	}
	return ""
}

func cloneStreamSpec(spec *surface.ThoughtRecipeStreamSpec) *surface.ThoughtRecipeStreamSpec {
	if spec == nil {
		return nil
	}
	cp := *spec
	return &cp
}

func cloneIngestSpec(spec *surface.ThoughtRecipeIngestSpec) *surface.ThoughtRecipeIngestSpec {
	if spec == nil {
		return nil
	}
	cp := *spec
	return &cp
}

func cloneStepAgent(agent *surface.ThoughtRecipeStepAgent) *surface.ThoughtRecipeStepAgent {
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

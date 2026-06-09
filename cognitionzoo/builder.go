package agents

import (
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	blackboardpkg "codeburg.org/lexbit/relurpify/cognitionzoo/blackboard"
	chainerpkg "codeburg.org/lexbit/relurpify/cognitionzoo/chainer"
	goalconpkg "codeburg.org/lexbit/relurpify/cognitionzoo/goalcon"
	htnpkg "codeburg.org/lexbit/relurpify/cognitionzoo/htn"
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	pipelinepkg "codeburg.org/lexbit/relurpify/cognitionzoo/pipeline"
	plannerpkg "codeburg.org/lexbit/relurpify/cognitionzoo/planner"
	reactpkg "codeburg.org/lexbit/relurpify/cognitionzoo/react"
	reflectionpkg "codeburg.org/lexbit/relurpify/cognitionzoo/reflection"
	rewoopkg "codeburg.org/lexbit/relurpify/cognitionzoo/rewoo"
	graph "codeburg.org/lexbit/relurpify/execution/agentgraph"
)

// BuildFromSpec constructs a workflow executor from the runtime spec and wires
// the supplied paradigm dependencies into the selected implementation.
// This is a local dispatch table for generic agent paradigms only.
func BuildFromSpec(deps *paradigm.Deps, spec agentspec.AgentRuntimeSpec) (graph.WorkflowExecutor, error) {
	deps = depsWithSpec(deps, spec)
	agentType := strings.ToLower(strings.TrimSpace(spec.Implementation))
	if agentType == "" && spec.Composition != nil {
		agentType = strings.ToLower(strings.TrimSpace(spec.Composition.Type))
	}
	if agentType == "" {
		return nil, fmt.Errorf("agent implementation required")
	}

	switch agentType {
	case "react":
		return reactpkg.New(deps), nil
	case "pipeline":
		return pipelinepkg.New(deps), nil
	case "planner":
		return plannerpkg.New(deps), nil
	case "reflection":
		return reflectionpkg.New(deps, reactpkg.New(deps)), nil
	case "chainer":
		return chainerpkg.New(deps), nil
	case "htn":
		return htnpkg.New(deps, htnpkg.NewMethodLibrary()), nil
	case "blackboard":
		return blackboardpkg.New(deps), nil
	case "rewoo":
		return rewoopkg.New(deps), nil
	case "goalcon":
		return goalconpkg.New(deps, goalconpkg.DefaultOperatorRegistry()), nil
	default:
		return nil, fmt.Errorf("unknown agent type %q", agentType)
	}
}

func depsWithSpec(deps *paradigm.Deps, spec agentspec.AgentRuntimeSpec) *paradigm.Deps {
	if deps == nil {
		return &paradigm.Deps{IngestOutputs: spec.IngestOutputsEnabled()}
	}
	next := *deps
	next.IngestOutputs = spec.IngestOutputsEnabled()
	return &next
}

// AgentBuilder preserves the existing chaining API for callers that only need
// environment + implementation dispatch.
type AgentBuilder struct {
	deps *paradigm.Deps
}

func NewAgentBuilder() *AgentBuilder { return &AgentBuilder{} }

func (b *AgentBuilder) WithDeps(deps *paradigm.Deps) *AgentBuilder {
	b.deps = deps
	return b
}

func (b *AgentBuilder) Build(agentType string) (graph.WorkflowExecutor, error) {
	if b.deps == nil {
		return nil, fmt.Errorf("agent builder requires dependencies")
	}
	return BuildFromSpec(b.deps, agentspec.AgentRuntimeSpec{Implementation: agentType})
}

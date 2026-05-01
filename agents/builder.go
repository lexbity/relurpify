package agents

import (
	"fmt"
	"strings"

	blackboardpkg "codeburg.org/lexbit/relurpify/agents/blackboard"
	chainerpkg "codeburg.org/lexbit/relurpify/agents/chainer"
	goalconpkg "codeburg.org/lexbit/relurpify/agents/goalcon"
	htnpkg "codeburg.org/lexbit/relurpify/agents/htn"
	pipelinepkg "codeburg.org/lexbit/relurpify/agents/pipeline"
	plannerpkg "codeburg.org/lexbit/relurpify/agents/planner"
	reactpkg "codeburg.org/lexbit/relurpify/agents/react"
	reflectionpkg "codeburg.org/lexbit/relurpify/agents/reflection"
	rewoopkg "codeburg.org/lexbit/relurpify/agents/rewoo"
	"codeburg.org/lexbit/relurpify/framework/agentenv"
	graph "codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/core"
)

// BuildFromSpec constructs a workflow executor from the manifest/runtime spec and wires
// the shared environment through InitializeEnvironment when supported.
// This is a local dispatch table for generic agent paradigms only.
// Named agents (e.g., coding, rex, architect) are handled by named/factory.BuildFromSpec.
func BuildFromSpec(env *agentenv.WorkspaceEnvironment, spec core.AgentRuntimeSpec) (graph.WorkflowExecutor, error) {
	if env != nil {
		env.IngestOutputs = spec.IngestOutputsEnabled()
	}
	agentType := strings.ToLower(strings.TrimSpace(spec.Implementation))
	if agentType == "" && spec.Composition != nil {
		agentType = strings.ToLower(strings.TrimSpace(spec.Composition.Type))
	}
	if agentType == "" {
		return nil, fmt.Errorf("agent implementation required")
	}

	switch agentType {
	case "react":
		return reactpkg.New(env), nil
	case "pipeline":
		return pipelinepkg.New(env), nil
	case "planner":
		return plannerpkg.New(env), nil
	case "reflection":
		return reflectionpkg.New(env, reactpkg.New(env)), nil
	case "chainer":
		return chainerpkg.New(env), nil
	case "htn":
		return htnpkg.New(env, htnpkg.NewMethodLibrary()), nil
	case "blackboard":
		return blackboardpkg.New(env), nil
	case "rewoo":
		return rewoopkg.New(env), nil
	case "goalcon":
		return goalconpkg.New(env, goalconpkg.DefaultOperatorRegistry()), nil
	default:
		return nil, fmt.Errorf("unknown agent type %q (named agents should use named/factory.BuildFromSpec)", agentType)
	}
}

// AgentBuilder preserves the existing chaining API for callers that only need
// environment + implementation dispatch.
type AgentBuilder struct {
	env *agentenv.WorkspaceEnvironment
}

func NewAgentBuilder() *AgentBuilder { return &AgentBuilder{} }

func (b *AgentBuilder) WithEnvironment(env *agentenv.WorkspaceEnvironment) *AgentBuilder {
	b.env = env
	return b
}

func (b *AgentBuilder) Build(agentType string) (graph.WorkflowExecutor, error) {
	if b.env == nil {
		return nil, fmt.Errorf("agent builder requires environment")
	}
	return BuildFromSpec(b.env, core.AgentRuntimeSpec{Implementation: agentType})
}

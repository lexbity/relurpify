package agents

import (
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/graph"
	namedfactory "codeburg.org/lexbit/relurpify/named/factory"
)

// BuildFromSpec constructs a workflow executor from the manifest/runtime spec and wires
// the shared environment through InitializeEnvironment when supported.
func BuildFromSpec(env agentenv.AgentEnvironment, spec core.AgentRuntimeSpec) (graph.WorkflowExecutor, error) {
	agentType := strings.ToLower(strings.TrimSpace(spec.Implementation))
	if agentType == "" && spec.Composition != nil {
		agentType = strings.ToLower(strings.TrimSpace(spec.Composition.Type))
	}
	if agentType == "" {
		return nil, fmt.Errorf("agent implementation required")
	}
	return namedfactory.BuildFromSpec(env, spec)
}

// AgentBuilder preserves the existing chaining API for callers that only need
// environment + implementation dispatch.
type AgentBuilder struct {
	env *agentenv.AgentEnvironment
}

func NewAgentBuilder() *AgentBuilder { return &AgentBuilder{} }

func (b *AgentBuilder) WithEnvironment(env *agentenv.AgentEnvironment) *AgentBuilder {
	b.env = env
	return b
}

func (b *AgentBuilder) Build(agentType string) (graph.WorkflowExecutor, error) {
	if b.env == nil {
		return nil, fmt.Errorf("agent builder requires environment")
	}
	return BuildFromSpec(*b.env, core.AgentRuntimeSpec{Implementation: agentType})
}

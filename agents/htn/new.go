package htn

import (
	reactpkg "github.com/lexcodex/relurpify/agents/react"
	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/graph"
)

type Option func(*HTNAgent)

func WithPrimitiveExec(agent graph.Agent) Option {
	return func(htn *HTNAgent) {
		htn.PrimitiveExec = agent
	}
}

func New(env agentenv.AgentEnvironment, methods *MethodLibrary, opts ...Option) *HTNAgent {
	agent := &HTNAgent{Methods: methods}
	for _, opt := range opts {
		if opt != nil {
			opt(agent)
		}
	}
	if agent.PrimitiveExec == nil {
		agent.PrimitiveExec = reactpkg.New(env)
	}
	_ = agent.InitializeEnvironment(env)
	return agent
}

func (a *HTNAgent) InitializeEnvironment(env agentenv.AgentEnvironment) error {
	a.Model = env.Model
	a.Tools = env.Registry
	a.Memory = env.Memory
	a.Config = env.Config
	return a.Initialize(env.Config)
}

package blackboard

import "github.com/lexcodex/relurpify/framework/agentenv"

type Option func(*BlackboardAgent)

func WithSources(sources []KnowledgeSource) Option {
	return func(agent *BlackboardAgent) {
		agent.Sources = append([]KnowledgeSource{}, sources...)
	}
}

func WithMaxCycles(maxCycles int) Option {
	return func(agent *BlackboardAgent) {
		agent.MaxCycles = maxCycles
	}
}

func New(env agentenv.AgentEnvironment, opts ...Option) *BlackboardAgent {
	agent := &BlackboardAgent{}
	for _, opt := range opts {
		if opt != nil {
			opt(agent)
		}
	}
	_ = agent.InitializeEnvironment(env)
	return agent
}

func (a *BlackboardAgent) InitializeEnvironment(env agentenv.AgentEnvironment) error {
	a.Model = env.Model
	a.Tools = env.Registry
	a.Memory = env.Memory
	a.Config = env.Config
	return a.Initialize(env.Config)
}

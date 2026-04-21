package chainer

import (
	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/core"
)

type Option func(*ChainerAgent)

func WithChain(chain *Chain) Option {
	return func(agent *ChainerAgent) {
		agent.Chain = chain
	}
}

func WithChainBuilder(builder func(*core.Task) (*Chain, error)) Option {
	return func(agent *ChainerAgent) {
		agent.ChainBuilder = builder
	}
}

func New(env agentenv.AgentEnvironment, opts ...Option) *ChainerAgent {
	agent := &ChainerAgent{
		Chain: &Chain{Links: []Link{NewSummarizeLink("default", nil, "chainer.output")}},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(agent)
		}
	}
	_ = agent.InitializeEnvironment(env)
	return agent
}

func (a *ChainerAgent) InitializeEnvironment(env agentenv.AgentEnvironment) error {
	a.Model = env.Model
	a.Tools = env.Registry
	a.Memory = env.Memory
	a.Config = env.Config
	return a.Initialize(env.Config)
}

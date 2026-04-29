package chainer

import (
	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/contextstream"
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

// WithContextStreamTrigger wires an explicit streaming trigger into the chainer agent.
func WithContextStreamTrigger(trigger *contextstream.Trigger) Option {
	return func(a *ChainerAgent) {
		a.StreamTrigger = trigger
	}
}

// WithContextStreamMode sets whether chainer streaming blocks or runs in the background.
func WithContextStreamMode(mode contextstream.Mode) Option {
	return func(a *ChainerAgent) {
		a.StreamMode = mode
	}
}

// WithContextStreamQuery overrides the query sent to the streaming trigger.
func WithContextStreamQuery(query string) Option {
	return func(a *ChainerAgent) {
		a.StreamQuery = query
	}
}

// WithContextStreamMaxTokens overrides the chainer stream token budget.
func WithContextStreamMaxTokens(maxTokens int) Option {
	return func(a *ChainerAgent) {
		a.StreamMaxTokens = maxTokens
	}
}

func New(env *agentenv.WorkspaceEnvironment, opts ...Option) *ChainerAgent {
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

func (a *ChainerAgent) InitializeEnvironment(env *agentenv.WorkspaceEnvironment) error {
	a.Model = env.Model
	a.Tools = env.Registry
	a.Config = env.Config
	return a.Initialize(env.Config)
}

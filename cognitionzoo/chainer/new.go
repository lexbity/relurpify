package chainer

import (
	"fmt"

	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	"codeburg.org/lexbit/relurpify/context/contextstream"
	execution "codeburg.org/lexbit/relurpify/execution"
)

type Option func(*ChainerAgent)

func WithChain(chain *Chain) Option {
	return func(agent *ChainerAgent) {
		agent.Chain = chain
	}
}

func WithChainBuilder(builder func(*execution.Task) (*Chain, error)) Option {
	return func(agent *ChainerAgent) {
		agent.ChainBuilder = builder
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

func New(deps *paradigm.Deps, opts ...Option) *ChainerAgent {
	agent := &ChainerAgent{
		Chain: &Chain{Links: []Link{NewSummarizeLink("default", nil, "chainer.output")}},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(agent)
		}
	}
	_ = agent.InitializeDeps(deps)
	return agent
}

func (a *ChainerAgent) InitializeDeps(deps *paradigm.Deps) error {
	if deps == nil {
		return fmt.Errorf("chainer dependencies unavailable")
	}
	a.Model = deps.Model
	a.Tools = deps.Registry
	a.Config = deps.Config
	return a.Initialize(deps.Config)
}

package blackboard

import (
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	"codeburg.org/lexbit/relurpify/context/contextstream"
)

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

// WithContextStreamMode sets whether blackboard streaming blocks or runs in the background.
func WithContextStreamMode(mode contextstream.Mode) Option {
	return func(a *BlackboardAgent) {
		a.StreamMode = mode
	}
}

// WithContextStreamQuery overrides the query sent to the streaming trigger.
func WithContextStreamQuery(query string) Option {
	return func(a *BlackboardAgent) {
		a.StreamQuery = query
	}
}

// WithContextStreamMaxTokens overrides the blackboard stream token budget.
func WithContextStreamMaxTokens(maxTokens int) Option {
	return func(a *BlackboardAgent) {
		a.StreamMaxTokens = maxTokens
	}
}

func New(deps *paradigm.Deps, opts ...Option) *BlackboardAgent {
	agent := &BlackboardAgent{}
	for _, opt := range opts {
		if opt != nil {
			opt(agent)
		}
	}
	_ = agent.InitializeDeps(deps)
	return agent
}

func (a *BlackboardAgent) InitializeDeps(deps *paradigm.Deps) error {
	if deps == nil {
		return a.Initialize(nil)
	}
	a.Model = deps.Model
	a.Tools = deps.Registry
	a.Memory = deps.WorkingMemory
	a.Config = deps.Config
	return a.Initialize(deps.Config)
}

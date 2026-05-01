package blackboard

import (
	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/contextstream"
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

func New(env *agentenv.WorkspaceEnvironment, opts ...Option) *BlackboardAgent {
	agent := &BlackboardAgent{}
	for _, opt := range opts {
		if opt != nil {
			opt(agent)
		}
	}
	_ = agent.InitializeEnvironment(env)
	return agent
}

func (a *BlackboardAgent) InitializeEnvironment(env *agentenv.WorkspaceEnvironment) error {
	if env == nil {
		return a.Initialize(nil)
	}
	a.Model = env.Model
	a.Tools = env.Registry
	a.Memory = env.WorkingMemory
	a.Config = env.Config
	return a.Initialize(env.Config)
}

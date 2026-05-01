package planner

import (
	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/contextstream"
)

type Option func(*PlannerAgent)

func New(env *agentenv.WorkspaceEnvironment, opts ...Option) *PlannerAgent {
	agent := &PlannerAgent{}
	for _, opt := range opts {
		if opt != nil {
			opt(agent)
		}
	}
	_ = agent.InitializeEnvironment(env)
	return agent
}

func (a *PlannerAgent) InitializeEnvironment(env *agentenv.WorkspaceEnvironment) error {
	a.Model = env.Model
	a.Tools = env.Registry
	a.Memory = env.WorkingMemory
	a.Config = env.Config
	return a.Initialize(env.Config)
}

// WithContextStreamMode sets whether planner streaming blocks or runs in the background.
func WithContextStreamMode(mode contextstream.Mode) Option {
	return func(a *PlannerAgent) {
		a.StreamMode = mode
	}
}

// WithContextStreamQuery overrides the query sent to the streaming trigger.
func WithContextStreamQuery(query string) Option {
	return func(a *PlannerAgent) {
		a.StreamQuery = query
	}
}

// WithContextStreamMaxTokens overrides the planner stream token budget.
func WithContextStreamMaxTokens(maxTokens int) Option {
	return func(a *PlannerAgent) {
		a.StreamMaxTokens = maxTokens
	}
}

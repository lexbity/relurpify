package planner

import (
	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/contextstream"
)

type Option func(*PlannerAgent)

func New(env agentenv.AgentEnvironment, opts ...Option) *PlannerAgent {
	agent := &PlannerAgent{}
	for _, opt := range opts {
		if opt != nil {
			opt(agent)
		}
	}
	_ = agent.InitializeEnvironment(env)
	return agent
}

func (a *PlannerAgent) InitializeEnvironment(env agentenv.AgentEnvironment) error {
	a.Model = env.Model
	a.Tools = env.Registry
	a.Memory = env.Memory
	a.Config = env.Config
	return a.Initialize(env.Config)
}

// WithContextStreamTrigger wires an explicit streaming trigger into the planner.
func WithContextStreamTrigger(trigger *contextstream.Trigger) Option {
	return func(a *PlannerAgent) {
		a.StreamTrigger = trigger
	}
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

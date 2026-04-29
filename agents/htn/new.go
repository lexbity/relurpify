package htn

import (
	reactpkg "codeburg.org/lexbit/relurpify/agents/react"
	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/contextstream"
)

type Option func(*HTNAgent)

func WithPrimitiveExec(agent agentgraph.WorkflowExecutor) Option {
	return func(htn *HTNAgent) {
		htn.PrimitiveExec = agent
	}
}

// WithContextStreamTrigger wires an explicit streaming trigger into the HTN agent.
func WithContextStreamTrigger(trigger *contextstream.Trigger) Option {
	return func(a *HTNAgent) {
		a.StreamTrigger = trigger
	}
}

// WithContextStreamMode sets whether HTN streaming blocks or runs in the background.
func WithContextStreamMode(mode contextstream.Mode) Option {
	return func(a *HTNAgent) {
		a.StreamMode = mode
	}
}

// WithContextStreamQuery overrides the query sent to the streaming trigger.
func WithContextStreamQuery(query string) Option {
	return func(a *HTNAgent) {
		a.StreamQuery = query
	}
}

// WithContextStreamMaxTokens overrides the HTN stream token budget.
func WithContextStreamMaxTokens(maxTokens int) Option {
	return func(a *HTNAgent) {
		a.StreamMaxTokens = maxTokens
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
	a.Config = env.Config
	return a.Initialize(env.Config)
}

package goalcon

import (
	"codeburg.org/lexbit/relurpify/agents/goalcon/operators"
	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/contextstream"
)

type Option func(*GoalConAgent)

// DefaultOperatorRegistry returns a default operator registry.
func DefaultOperatorRegistry() *OperatorRegistry {
	return operators.DefaultOperatorRegistry()
}

// WithContextStreamMode sets whether goalcon streaming blocks or runs in the background.
func WithContextStreamMode(mode contextstream.Mode) Option {
	return func(a *GoalConAgent) {
		a.StreamMode = mode
	}
}

// WithContextStreamQuery overrides the query sent to the streaming trigger.
func WithContextStreamQuery(query string) Option {
	return func(a *GoalConAgent) {
		a.StreamQuery = query
	}
}

// WithContextStreamMaxTokens overrides the goalcon stream token budget.
func WithContextStreamMaxTokens(maxTokens int) Option {
	return func(a *GoalConAgent) {
		a.StreamMaxTokens = maxTokens
	}
}

func New(env *agentenv.WorkspaceEnvironment, operators *OperatorRegistry, opts ...Option) *GoalConAgent {
	agent := &GoalConAgent{Operators: operators}
	for _, opt := range opts {
		if opt != nil {
			opt(agent)
		}
	}
	_ = agent.InitializeEnvironment(env)
	return agent
}

func (a *GoalConAgent) InitializeEnvironment(env *agentenv.WorkspaceEnvironment) error {
	if env == nil {
		return a.Initialize(nil)
	}
	a.Model = env.Model
	a.Tools = env.Registry
	a.Memory = env.WorkingMemory
	a.Config = env.Config
	return a.Initialize(env.Config)
}

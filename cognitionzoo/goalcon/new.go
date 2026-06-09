package goalcon

import (
	"codeburg.org/lexbit/relurpify/cognitionzoo/goalcon/operators"
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	"codeburg.org/lexbit/relurpify/context/contextstream"
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

func New(deps *paradigm.Deps, operators *OperatorRegistry, opts ...Option) *GoalConAgent {
	agent := &GoalConAgent{Operators: operators}
	for _, opt := range opts {
		if opt != nil {
			opt(agent)
		}
	}
	_ = agent.InitializeDeps(deps)
	return agent
}

func (a *GoalConAgent) InitializeDeps(deps *paradigm.Deps) error {
	if deps == nil {
		return a.Initialize(nil)
	}
	a.Model = deps.Model
	a.Tools = deps.Registry
	a.Memory = deps.WorkingMemory
	a.Config = deps.Config
	return a.Initialize(deps.Config)
}

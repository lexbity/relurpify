package planner

import (
	"fmt"

	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	"codeburg.org/lexbit/relurpify/context/contextstream"
)

type Option func(*PlannerAgent)

func New(deps *paradigm.Deps, opts ...Option) *PlannerAgent {
	agent := &PlannerAgent{}
	for _, opt := range opts {
		if opt != nil {
			opt(agent)
		}
	}
	_ = agent.InitializeDeps(deps)
	return agent
}

func (a *PlannerAgent) InitializeDeps(deps *paradigm.Deps) error {
	if deps == nil {
		return fmt.Errorf("planner dependencies unavailable")
	}
	a.Model = deps.Model
	a.Tools = deps.Registry
	a.Memory = deps.WorkingMemory
	a.Config = deps.Config
	return a.Initialize(deps.Config)
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

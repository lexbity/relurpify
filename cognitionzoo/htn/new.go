package htn

import (
	"fmt"

	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	"codeburg.org/lexbit/relurpify/context/contextstream"
	"codeburg.org/lexbit/relurpify/execution/agentgraph"
)

type Option func(*HTNAgent)

func WithPrimitiveExec(agent agentgraph.WorkflowExecutor) Option {
	return func(htn *HTNAgent) {
		htn.PrimitiveExec = agent
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

func New(deps *paradigm.Deps, methods *MethodLibrary, opts ...Option) *HTNAgent {
	agent := &HTNAgent{Methods: methods}
	for _, opt := range opts {
		if opt != nil {
			opt(agent)
		}
	}
	if agent.PrimitiveExec == nil {
		agent.PrimitiveExec = &noopAgent{}
	}
	_ = agent.InitializeDeps(deps)
	return agent
}

func (a *HTNAgent) InitializeDeps(deps *paradigm.Deps) error {
	if deps == nil {
		return fmt.Errorf("htn dependencies unavailable")
	}
	a.Model = deps.Model
	a.Tools = deps.Registry
	a.Config = deps.Config
	return a.Initialize(deps.Config)
}

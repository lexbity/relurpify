package htn

import (
	"fmt"

	"codeburg.org/lexbit/relurpify/cognitionzoo/htn/runtime"
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

// WithAuthoredMethod installs a recipe-authored decomposition: exactly the
// authored tasks run, in declaration order, with zero LLM decomposition calls
// (D4). The method is validated at construction, so a spec-invalid authored
// task is an option-construction error the caller turns into a load error.
func WithAuthoredMethod(name string, tasks []AuthoredTask) (Option, error) {
	method, err := buildAuthoredMethod(name, tasks)
	if err != nil {
		return nil, err
	}
	return func(htn *HTNAgent) {
		if htn != nil {
			htn.authoredMethod = method
		}
	}, nil
}

// New builds an HTN agent with the given method library and options.
func New(deps *paradigm.Deps, methods *runtime.MethodLibrary, opts ...Option) *HTNAgent {
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
	a.StreamTrigger = deps.StreamTrigger
	return a.Initialize(deps.Config)
}

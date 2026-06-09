package reflection

import (
	"fmt"

	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	reactpkg "codeburg.org/lexbit/relurpify/cognitionzoo/react"
	graph "codeburg.org/lexbit/relurpify/execution/agentgraph"
)

type Option func(*ReflectionAgent)

func New(deps *paradigm.Deps, delegate graph.WorkflowExecutor, opts ...Option) *ReflectionAgent {
	if delegate == nil {
		delegate = reactpkg.New(deps)
	}
	agent := &ReflectionAgent{Delegate: delegate}
	for _, opt := range opts {
		if opt != nil {
			opt(agent)
		}
	}
	_ = agent.InitializeDeps(deps)
	return agent
}

func (a *ReflectionAgent) InitializeDeps(deps *paradigm.Deps) error {
	if deps == nil {
		return fmt.Errorf("reflection dependencies unavailable")
	}
	a.Reviewer = deps.Model
	a.Config = deps.Config
	if envAware, ok := a.Delegate.(interface {
		InitializeDeps(*paradigm.Deps) error
	}); ok {
		if err := envAware.InitializeDeps(deps); err != nil {
			return err
		}
	}
	return a.Initialize(deps.Config)
}

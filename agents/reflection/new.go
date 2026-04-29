package reflection

import (
	reactpkg "codeburg.org/lexbit/relurpify/agents/react"
	"codeburg.org/lexbit/relurpify/framework/agentenv"
	graph "codeburg.org/lexbit/relurpify/framework/agentgraph"
)

type Option func(*ReflectionAgent)

func New(env *agentenv.WorkspaceEnvironment, delegate graph.WorkflowExecutor, opts ...Option) *ReflectionAgent {
	if delegate == nil {
		delegate = reactpkg.New(env)
	}
	agent := &ReflectionAgent{Delegate: delegate}
	for _, opt := range opts {
		if opt != nil {
			opt(agent)
		}
	}
	_ = agent.InitializeEnvironment(env)
	return agent
}

func (a *ReflectionAgent) InitializeEnvironment(env *agentenv.WorkspaceEnvironment) error {
	a.Reviewer = env.Model
	a.Config = env.Config
	if envAware, ok := a.Delegate.(interface {
		InitializeEnvironment(*agentenv.WorkspaceEnvironment) error
	}); ok {
		if err := envAware.InitializeEnvironment(env); err != nil {
			return err
		}
	}
	return a.Initialize(env.Config)
}

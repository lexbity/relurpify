package architect

import (
	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/capability"
)

type Option func(*ArchitectAgent)

func WithPlannerTools(tools *capability.Registry) Option {
	return func(agent *ArchitectAgent) {
		agent.PlannerTools = tools
	}
}

func WithExecutorTools(tools *capability.Registry) Option {
	return func(agent *ArchitectAgent) {
		agent.ExecutorTools = tools
	}
}

func New(env agentenv.AgentEnvironment, opts ...Option) *ArchitectAgent {
	agent := &ArchitectAgent{}
	for _, opt := range opts {
		if opt != nil {
			opt(agent)
		}
	}
	_ = agent.InitializeEnvironment(env)
	return agent
}

func (a *ArchitectAgent) InitializeEnvironment(env agentenv.AgentEnvironment) error {
	a.Model = env.Model
	if a.PlannerTools == nil {
		a.PlannerTools = env.Registry
	}
	if a.ExecutorTools == nil {
		a.ExecutorTools = env.Registry
	}
	a.Memory = env.Memory
	a.Config = env.Config
	a.IndexManager = env.IndexManager
	a.SearchEngine = env.SearchEngine
	return a.Initialize(env.Config)
}

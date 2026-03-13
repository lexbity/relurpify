package planner

import "github.com/lexcodex/relurpify/framework/agentenv"

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

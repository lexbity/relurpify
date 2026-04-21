package react

import "codeburg.org/lexbit/relurpify/framework/agentenv"

type Option func(*ReActAgent)

func New(env agentenv.AgentEnvironment, opts ...Option) *ReActAgent {
	agent := &ReActAgent{}
	for _, opt := range opts {
		if opt != nil {
			opt(agent)
		}
	}
	_ = agent.InitializeEnvironment(env)
	return agent
}

func (a *ReActAgent) InitializeEnvironment(env agentenv.AgentEnvironment) error {
	a.Model = env.Model
	a.Tools = env.Registry
	a.Memory = env.Memory
	a.Config = env.Config
	a.IndexManager = env.IndexManager
	a.SearchEngine = env.SearchEngine
	return a.Initialize(env.Config)
}

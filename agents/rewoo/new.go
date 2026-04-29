package rewoo

import "codeburg.org/lexbit/relurpify/framework/agentenv"

type Option func(*RewooAgent)

func New(env *agentenv.WorkspaceEnvironment, opts ...Option) *RewooAgent {
	agent := &RewooAgent{}
	for _, opt := range opts {
		if opt != nil {
			opt(agent)
		}
	}
	_ = agent.InitializeEnvironment(env)
	return agent
}

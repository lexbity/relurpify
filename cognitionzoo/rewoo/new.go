package rewoo

import "codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"

type Option func(*RewooAgent)

func New(deps *paradigm.Deps, opts ...Option) *RewooAgent {
	agent := &RewooAgent{}
	for _, opt := range opts {
		if opt != nil {
			opt(agent)
		}
	}
	_ = agent.InitializeDeps(deps)
	return agent
}

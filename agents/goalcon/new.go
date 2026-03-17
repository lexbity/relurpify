package goalcon

import (
	"github.com/lexcodex/relurpify/agents/goalcon/operators"
	"github.com/lexcodex/relurpify/framework/agentenv"
)

type Option func(*GoalConAgent)

// DefaultOperatorRegistry returns a default operator registry.
func DefaultOperatorRegistry() *OperatorRegistry {
	return operators.DefaultOperatorRegistry()
}

func New(env agentenv.AgentEnvironment, operators *OperatorRegistry, opts ...Option) *GoalConAgent {
	agent := &GoalConAgent{Operators: operators}
	for _, opt := range opts {
		if opt != nil {
			opt(agent)
		}
	}
	_ = agent.InitializeEnvironment(env)
	return agent
}

func (a *GoalConAgent) InitializeEnvironment(env agentenv.AgentEnvironment) error {
	a.Model = env.Model
	a.Tools = env.Registry
	a.Memory = env.Memory
	a.Config = env.Config
	return a.Initialize(env.Config)
}

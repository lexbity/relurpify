package htn

import (
	htnpkg "codeburg.org/lexbit/relurpify/agents/htn"
	"codeburg.org/lexbit/relurpify/framework/agentenv"
)

type Runner = htnpkg.HTNAgent

func New(env agentenv.AgentEnvironment) *Runner {
	agent := &htnpkg.HTNAgent{}
	_ = agent.InitializeEnvironment(env)
	return agent
}

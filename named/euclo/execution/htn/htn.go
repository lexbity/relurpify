package htn

import (
	htnpkg "github.com/lexcodex/relurpify/agents/htn"
	"github.com/lexcodex/relurpify/framework/agentenv"
)

type Runner = htnpkg.HTNAgent

func New(env agentenv.AgentEnvironment) *Runner {
	agent := &htnpkg.HTNAgent{}
	_ = agent.InitializeEnvironment(env)
	return agent
}

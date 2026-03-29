package react

import (
	reactpkg "github.com/lexcodex/relurpify/agents/react"
	"github.com/lexcodex/relurpify/framework/agentenv"
)

type Runner = reactpkg.ReActAgent

func New(env agentenv.AgentEnvironment) *Runner {
	agent := &reactpkg.ReActAgent{}
	_ = agent.InitializeEnvironment(env)
	return agent
}

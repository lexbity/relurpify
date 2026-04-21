package react

import (
	reactpkg "codeburg.org/lexbit/relurpify/agents/react"
	"codeburg.org/lexbit/relurpify/framework/agentenv"
)

type Runner = reactpkg.ReActAgent

func New(env agentenv.AgentEnvironment) *Runner {
	agent := &reactpkg.ReActAgent{}
	_ = agent.InitializeEnvironment(env)
	return agent
}

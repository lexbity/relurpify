package planner

import (
	plannerpkg "codeburg.org/lexbit/relurpify/agents/planner"
	"codeburg.org/lexbit/relurpify/framework/agentenv"
)

type Runner = plannerpkg.PlannerAgent

func New(env agentenv.AgentEnvironment) *Runner {
	agent := &plannerpkg.PlannerAgent{}
	_ = agent.InitializeEnvironment(env)
	return agent
}

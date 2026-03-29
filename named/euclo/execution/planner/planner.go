package planner

import (
	plannerpkg "github.com/lexcodex/relurpify/agents/planner"
	"github.com/lexcodex/relurpify/framework/agentenv"
)

type Runner = plannerpkg.PlannerAgent

func New(env agentenv.AgentEnvironment) *Runner {
	agent := &plannerpkg.PlannerAgent{}
	_ = agent.InitializeEnvironment(env)
	return agent
}

package reflection

import (
	reflectionpkg "github.com/lexcodex/relurpify/agents/reflection"
	"github.com/lexcodex/relurpify/framework/agentenv"
	reactexec "github.com/lexcodex/relurpify/named/euclo/execution/react"
)

type Runner = reflectionpkg.ReflectionAgent

func New(env agentenv.AgentEnvironment) *Runner {
	return reflectionpkg.New(env, reactexec.New(env))
}

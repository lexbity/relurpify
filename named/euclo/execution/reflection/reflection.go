package reflection

import (
	reflectionpkg "codeburg.org/lexbit/relurpify/agents/reflection"
	"codeburg.org/lexbit/relurpify/framework/agentenv"
	reactexec "codeburg.org/lexbit/relurpify/named/euclo/execution/react"
)

type Runner = reflectionpkg.ReflectionAgent

func New(env agentenv.AgentEnvironment) *Runner {
	return reflectionpkg.New(env, reactexec.New(env))
}

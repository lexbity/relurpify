package rewoo

import (
	"context"

	rewoopkg "codeburg.org/lexbit/relurpify/agents/rewoo"
	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/core"
)

type Runner = rewoopkg.RewooAgent
type Option = rewoopkg.Option

func New(env agentenv.AgentEnvironment, opts ...Option) *Runner {
	return rewoopkg.New(env, opts...)
}

func Execute(ctx context.Context, env agentenv.AgentEnvironment, task *core.Task, state *core.Context, opts ...Option) (*core.Result, error) {
	return New(env, opts...).Execute(ctx, task, state)
}

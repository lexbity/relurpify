package execution

import (
	"context"

	architectpkg "codeburg.org/lexbit/relurpify/agents/architect"
	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/core"
)

type ArchitectRunner = architectpkg.ArchitectAgent
type ArchitectOption = architectpkg.Option

func NewArchitect(env agentenv.AgentEnvironment, opts ...ArchitectOption) *ArchitectRunner {
	return architectpkg.New(env, opts...)
}

func ExecuteArchitect(ctx context.Context, env agentenv.AgentEnvironment, task *core.Task, state *core.Context, opts ...ArchitectOption) (*core.Result, error) {
	return NewArchitect(env, opts...).Execute(ctx, task, state)
}

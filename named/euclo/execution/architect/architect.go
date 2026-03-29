package execution

import (
	"context"

	architectpkg "github.com/lexcodex/relurpify/agents/architect"
	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/core"
)

type ArchitectRunner = architectpkg.ArchitectAgent
type ArchitectOption = architectpkg.Option

func NewArchitect(env agentenv.AgentEnvironment, opts ...ArchitectOption) *ArchitectRunner {
	return architectpkg.New(env, opts...)
}

func ExecuteArchitect(ctx context.Context, env agentenv.AgentEnvironment, task *core.Task, state *core.Context, opts ...ArchitectOption) (*core.Result, error) {
	return NewArchitect(env, opts...).Execute(ctx, task, state)
}

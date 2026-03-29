package pipe

import (
	"context"

	pipelinepkg "github.com/lexcodex/relurpify/agents/pipeline"
	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/core"
	frameworkpipeline "github.com/lexcodex/relurpify/framework/pipeline"
)

type Runner = pipelinepkg.PipelineAgent

func New(env agentenv.AgentEnvironment, opts ...pipelinepkg.Option) *Runner {
	return pipelinepkg.New(env, opts...)
}

func ExecuteStages(ctx context.Context, env agentenv.AgentEnvironment, task *core.Task, state *core.Context, stages []frameworkpipeline.Stage, opts ...pipelinepkg.Option) (*core.Result, error) {
	options := append([]pipelinepkg.Option{pipelinepkg.WithStages(stages)}, opts...)
	return New(env, options...).Execute(ctx, task, state)
}

package pipe

import (
	"context"

	pipelinepkg "codeburg.org/lexbit/relurpify/agents/pipeline"
	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/core"
	frameworkpipeline "codeburg.org/lexbit/relurpify/framework/pipeline"
)

type Runner = pipelinepkg.PipelineAgent

func New(env agentenv.AgentEnvironment, opts ...pipelinepkg.Option) *Runner {
	return pipelinepkg.New(env, opts...)
}

func ExecuteStages(ctx context.Context, env agentenv.AgentEnvironment, task *core.Task, state *core.Context, stages []frameworkpipeline.Stage, opts ...pipelinepkg.Option) (*core.Result, error) {
	options := append([]pipelinepkg.Option{pipelinepkg.WithStages(stages)}, opts...)
	return New(env, options...).Execute(ctx, task, state)
}

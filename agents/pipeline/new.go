package pipeline

import (
	pipelinestages "github.com/lexcodex/relurpify/agents/pipeline/stages"
	"github.com/lexcodex/relurpify/framework/agentenv"
	frameworkpipeline "github.com/lexcodex/relurpify/framework/pipeline"
)

type Option func(*PipelineAgent)

func WithStages(stages []frameworkpipeline.Stage) Option {
	return func(agent *PipelineAgent) {
		agent.Stages = append([]frameworkpipeline.Stage{}, stages...)
	}
}

func WithStageFactory(factory PipelineStageFactory) Option {
	return func(agent *PipelineAgent) {
		agent.StageFactory = factory
	}
}

func New(env agentenv.AgentEnvironment, opts ...Option) *PipelineAgent {
	agent := &PipelineAgent{StageFactory: pipelinestages.CodingStageFactory{}}
	for _, opt := range opts {
		if opt != nil {
			opt(agent)
		}
	}
	_ = agent.InitializeEnvironment(env)
	return agent
}

func (a *PipelineAgent) InitializeEnvironment(env agentenv.AgentEnvironment) error {
	a.Model = env.Model
	a.Tools = env.Registry
	a.Config = env.Config
	return a.Initialize(env.Config)
}

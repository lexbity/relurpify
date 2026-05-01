package pipeline

import (
	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/contextstream"
)

type Option func(*PipelineAgent)

func WithStages(stages []Stage) Option {
	return func(agent *PipelineAgent) {
		agent.Stages = append([]Stage{}, stages...)
	}
}

func WithStageFactory(factory PipelineStageFactory) Option {
	return func(agent *PipelineAgent) {
		agent.StageFactory = factory
	}
}

// WithContextStreamMode sets whether pipeline streaming blocks or runs in the background.
func WithContextStreamMode(mode contextstream.Mode) Option {
	return func(a *PipelineAgent) {
		a.StreamMode = mode
	}
}

// WithContextStreamQuery overrides the query sent to the streaming trigger.
func WithContextStreamQuery(query string) Option {
	return func(a *PipelineAgent) {
		a.StreamQuery = query
	}
}

// WithContextStreamMaxTokens overrides the pipeline stream token budget.
func WithContextStreamMaxTokens(maxTokens int) Option {
	return func(a *PipelineAgent) {
		a.StreamMaxTokens = maxTokens
	}
}

func New(env *agentenv.WorkspaceEnvironment, opts ...Option) *PipelineAgent {
	agent := &PipelineAgent{}
	for _, opt := range opts {
		if opt != nil {
			opt(agent)
		}
	}
	_ = agent.InitializeEnvironment(env)
	return agent
}

func (a *PipelineAgent) InitializeEnvironment(env *agentenv.WorkspaceEnvironment) error {
	a.Model = env.Model
	a.Tools = env.Registry
	a.Config = env.Config
	return a.Initialize(env.Config)
}

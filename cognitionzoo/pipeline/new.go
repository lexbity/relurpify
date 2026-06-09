package pipeline

import (
	"fmt"

	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	"codeburg.org/lexbit/relurpify/context/contextstream"
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

func New(deps *paradigm.Deps, opts ...Option) *PipelineAgent {
	agent := &PipelineAgent{}
	for _, opt := range opts {
		if opt != nil {
			opt(agent)
		}
	}
	_ = agent.InitializeDeps(deps)
	return agent
}

func (a *PipelineAgent) InitializeDeps(deps *paradigm.Deps) error {
	if deps == nil {
		return fmt.Errorf("pipeline dependencies unavailable")
	}
	a.Model = deps.Model
	a.Tools = deps.Registry
	a.Config = deps.Config
	return a.Initialize(deps.Config)
}

package react

import (
	"fmt"

	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	"codeburg.org/lexbit/relurpify/context/contextstream"
)

type Option func(*ReActAgent)

func New(deps *paradigm.Deps, opts ...Option) *ReActAgent {
	agent := &ReActAgent{}
	for _, opt := range opts {
		if opt != nil {
			opt(agent)
		}
	}
	_ = agent.InitializeDeps(deps)
	return agent
}

// WithContextStreamMode sets whether react streaming blocks or runs in the background.
func WithContextStreamMode(mode contextstream.Mode) Option {
	return func(a *ReActAgent) {
		a.StreamMode = mode
	}
}

// WithContextStreamQuery overrides the query sent to the streaming trigger.
func WithContextStreamQuery(query string) Option {
	return func(a *ReActAgent) {
		a.StreamQuery = query
	}
}

// WithContextStreamMaxTokens overrides the react stream token budget.
func WithContextStreamMaxTokens(maxTokens int) Option {
	return func(a *ReActAgent) {
		a.StreamMaxTokens = maxTokens
	}
}

func (a *ReActAgent) InitializeDeps(deps *paradigm.Deps) error {
	if deps == nil {
		return fmt.Errorf("react dependencies unavailable")
	}
	a.Model = deps.Model
	a.Tools = deps.Registry
	a.Memory = deps.WorkingMemory
	a.Config = deps.Config
	a.IndexManager = deps.IndexManager
	a.SearchEngine = deps.SearchEngine
	a.StreamTrigger = deps.StreamTrigger
	a.OutputIngester = deps.OutputIngester
	a.IngestOutputs = deps.IngestOutputs
	a.PromptRegistry = deps.PromptRegistry
	return a.Initialize(deps.Config)
}

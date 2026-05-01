package react

import (
	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/contextstream"
)


type Option func(*ReActAgent)

func New(env *agentenv.WorkspaceEnvironment, opts ...Option) *ReActAgent {
	agent := &ReActAgent{}
	for _, opt := range opts {
		if opt != nil {
			opt(agent)
		}
	}
	_ = agent.InitializeEnvironment(env)
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

func (a *ReActAgent) InitializeEnvironment(env *agentenv.WorkspaceEnvironment) error {
	a.Model = env.Model
	a.Tools = env.Registry
	a.Memory = env.WorkingMemory
	a.Config = env.Config
	a.IndexManager = env.IndexManager
	a.SearchEngine = env.SearchEngine
	a.OutputIngester = env.OutputIngester
	a.IngestOutputs = env.IngestOutputs
	return a.Initialize(env.Config)
}

package thoughtrecipe

import (
	"strings"

	blackboardagent "codeburg.org/lexbit/relurpify/cognitionzoo/blackboard"
	chaineragent "codeburg.org/lexbit/relurpify/cognitionzoo/chainer"
	goalconagent "codeburg.org/lexbit/relurpify/cognitionzoo/goalcon"
	htnagent "codeburg.org/lexbit/relurpify/cognitionzoo/htn"
	pipelineagent "codeburg.org/lexbit/relurpify/cognitionzoo/pipeline"
	reactagent "codeburg.org/lexbit/relurpify/cognitionzoo/react"
	rewooagent "codeburg.org/lexbit/relurpify/cognitionzoo/rewoo"
	"codeburg.org/lexbit/relurpify/context/contextstream"
)

func (c *stepCore) streamOptions() []reactagent.Option {
	opts := make([]reactagent.Option, 0, 3)
	if c.step.Stream != nil {
		if mode := strings.TrimSpace(c.step.Stream.Mode); mode != "" {
			opts = append(opts, reactagent.WithContextStreamMode(contextstream.Mode(mode)))
		}
		if query := strings.TrimSpace(c.step.Stream.QueryTemplate); query != "" {
			opts = append(opts, reactagent.WithContextStreamQuery(query))
		}
		if c.step.Stream.MaxTokens > 0 {
			opts = append(opts, reactagent.WithContextStreamMaxTokens(c.step.Stream.MaxTokens))
		}
	}
	return opts
}

func (c *stepCore) streamOptionsHTN() []htnagent.Option {
	opts := make([]htnagent.Option, 0, 3)
	if c.step.Stream != nil {
		if mode := strings.TrimSpace(c.step.Stream.Mode); mode != "" {
			opts = append(opts, htnagent.WithContextStreamMode(contextstream.Mode(mode)))
		}
		if query := strings.TrimSpace(c.step.Stream.QueryTemplate); query != "" {
			opts = append(opts, htnagent.WithContextStreamQuery(query))
		}
		if c.step.Stream.MaxTokens > 0 {
			opts = append(opts, htnagent.WithContextStreamMaxTokens(c.step.Stream.MaxTokens))
		}
	}
	return opts
}

func (c *stepCore) streamOptionsBlackboard() []blackboardagent.Option {
	opts := make([]blackboardagent.Option, 0, 3)
	if c.step.Stream != nil {
		if mode := strings.TrimSpace(c.step.Stream.Mode); mode != "" {
			opts = append(opts, blackboardagent.WithContextStreamMode(contextstream.Mode(mode)))
		}
		if query := strings.TrimSpace(c.step.Stream.QueryTemplate); query != "" {
			opts = append(opts, blackboardagent.WithContextStreamQuery(query))
		}
		if c.step.Stream.MaxTokens > 0 {
			opts = append(opts, blackboardagent.WithContextStreamMaxTokens(c.step.Stream.MaxTokens))
		}
	}
	return opts
}

func (c *stepCore) streamOptionsChainer() []chaineragent.Option {
	opts := make([]chaineragent.Option, 0, 3)
	if c.step.Stream != nil {
		if mode := strings.TrimSpace(c.step.Stream.Mode); mode != "" {
			opts = append(opts, chaineragent.WithContextStreamMode(contextstream.Mode(mode)))
		}
		if query := strings.TrimSpace(c.step.Stream.QueryTemplate); query != "" {
			opts = append(opts, chaineragent.WithContextStreamQuery(query))
		}
		if c.step.Stream.MaxTokens > 0 {
			opts = append(opts, chaineragent.WithContextStreamMaxTokens(c.step.Stream.MaxTokens))
		}
	}
	return opts
}

func (c *stepCore) streamOptionsPipeline() []pipelineagent.Option {
	opts := make([]pipelineagent.Option, 0, 3)
	if c.step.Stream != nil {
		if mode := strings.TrimSpace(c.step.Stream.Mode); mode != "" {
			opts = append(opts, pipelineagent.WithContextStreamMode(contextstream.Mode(mode)))
		}
		if query := strings.TrimSpace(c.step.Stream.QueryTemplate); query != "" {
			opts = append(opts, pipelineagent.WithContextStreamQuery(query))
		}
		if c.step.Stream.MaxTokens > 0 {
			opts = append(opts, pipelineagent.WithContextStreamMaxTokens(c.step.Stream.MaxTokens))
		}
	}
	return opts
}

func (c *stepCore) streamOptionsGoalCon() []goalconagent.Option {
	opts := make([]goalconagent.Option, 0, 3)
	if c.step.Stream != nil {
		if mode := strings.TrimSpace(c.step.Stream.Mode); mode != "" {
			opts = append(opts, goalconagent.WithContextStreamMode(contextstream.Mode(mode)))
		}
		if query := strings.TrimSpace(c.step.Stream.QueryTemplate); query != "" {
			opts = append(opts, goalconagent.WithContextStreamQuery(query))
		}
		if c.step.Stream.MaxTokens > 0 {
			opts = append(opts, goalconagent.WithContextStreamMaxTokens(c.step.Stream.MaxTokens))
		}
	}
	return opts
}

func (c *stepCore) rewooOptions() rewooagent.RewooOptions {
	opts := rewooagent.RewooOptions{}
	if c.step.Stream != nil {
		if mode := strings.TrimSpace(c.step.Stream.Mode); mode != "" {
			opts.StreamMode = contextstream.Mode(mode)
		}
		if query := strings.TrimSpace(c.step.Stream.QueryTemplate); query != "" {
			opts.StreamQuery = query
		}
		if c.step.Stream.MaxTokens > 0 {
			opts.StreamMaxTokens = c.step.Stream.MaxTokens
		}
	}
	return opts
}

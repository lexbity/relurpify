package reflection

import (
	"context"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	reactpkg "codeburg.org/lexbit/relurpify/cognitionzoo/react"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	graph "codeburg.org/lexbit/relurpify/execution/agentgraph"
)

type Option func(*ReflectionAgent)

// ReviseBodyFunc executes the authored revise body in-process: the lowered
// body steps run sequentially and return their last result.
type ReviseBodyFunc func(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error)

// WithReviewCriteria installs the authored review criterion that drives the
// directive-mode review phase (D5). When unset, the library review prompt
// governs.
func WithReviewCriteria(text string) Option {
	return func(a *ReflectionAgent) {
		if a != nil {
			a.reviewCriteria = strings.TrimSpace(text)
		}
	}
}

// WithReviseCycle installs an authored revise cycle: after each review the
// predicate is evaluated against the envelope (scratch-aware); when it reads
// true the body runs in-process and the loop re-reviews, bounded by
// MaxRevisionCycles. A nil body disables the cycle.
func WithReviseCycle(body ReviseBodyFunc, predicate func(*contextdata.Envelope) bool) Option {
	return func(a *ReflectionAgent) {
		if a != nil {
			a.reviseBody = body
			a.revisePredicate = predicate
		}
	}
}

func New(deps *paradigm.Deps, delegate graph.WorkflowExecutor, opts ...Option) *ReflectionAgent {
	if delegate == nil {
		delegate = reactpkg.New(deps)
	}
	agent := &ReflectionAgent{Delegate: delegate}
	for _, opt := range opts {
		if opt != nil {
			opt(agent)
		}
	}
	_ = agent.InitializeDeps(deps)
	return agent
}

func (a *ReflectionAgent) InitializeDeps(deps *paradigm.Deps) error {
	if deps == nil {
		return fmt.Errorf("reflection dependencies unavailable")
	}
	a.Reviewer = deps.Model
	a.Config = deps.Config
	if envAware, ok := a.Delegate.(interface {
		InitializeDeps(*paradigm.Deps) error
	}); ok {
		if err := envAware.InitializeDeps(deps); err != nil {
			return err
		}
	}
	return a.Initialize(deps.Config)
}

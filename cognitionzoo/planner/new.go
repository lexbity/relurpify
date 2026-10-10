package planner

import (
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	pl "codeburg.org/lexbit/relurpify/cognitionzoo/plan"
	"codeburg.org/lexbit/relurpify/context/contextstream"
)

type Option func(*PlannerAgent)

func New(deps *paradigm.Deps, opts ...Option) *PlannerAgent {
	agent := &PlannerAgent{}
	for _, opt := range opts {
		if opt != nil {
			opt(agent)
		}
	}
	_ = agent.InitializeDeps(deps)
	return agent
}

func (a *PlannerAgent) InitializeDeps(deps *paradigm.Deps) error {
	if deps == nil {
		return fmt.Errorf("planner dependencies unavailable")
	}
	a.Model = deps.Model
	a.Tools = deps.Registry
	a.Memory = deps.WorkingMemory
	a.Config = deps.Config
	a.StreamTrigger = deps.StreamTrigger
	return a.Initialize(deps.Config)
}

// WithContextStreamMode sets whether planner streaming blocks or runs in the background.
func WithContextStreamMode(mode contextstream.Mode) Option {
	return func(a *PlannerAgent) {
		a.StreamMode = mode
	}
}

// WithContextStreamQuery overrides the query sent to the streaming trigger.
func WithContextStreamQuery(query string) Option {
	return func(a *PlannerAgent) {
		a.StreamQuery = query
	}
}

// WithContextStreamMaxTokens overrides the planner stream token budget.
func WithContextStreamMaxTokens(maxTokens int) Option {
	return func(a *PlannerAgent) {
		a.StreamMaxTokens = maxTokens
	}
}

// WithAuthoredPlan installs a recipe-authored plan: exactly these steps run, in
// declaration order, with zero planning model calls (D1). The objective is the
// authored `plan` guidance. Steps are copied so later mutation of the caller's
// slice cannot change the plan.
func WithAuthoredPlan(objective string, steps []pl.PlanStep) Option {
	return func(a *PlannerAgent) {
		if a == nil {
			return
		}
		a.directiveMode = true
		a.planObjective = strings.TrimSpace(objective)
		a.authoredSteps = append([]pl.PlanStep(nil), steps...)
	}
}

// WithGeneratedPlan selects generated mode: when the recipe authors no steps,
// the planner produces the structure with one bounded, validated model call.
// maxSteps <= 0 falls back to DefaultGeneratedPlanBound.
func WithGeneratedPlan(objective string, maxSteps int) Option {
	return func(a *PlannerAgent) {
		if a == nil {
			return
		}
		a.directiveMode = true
		a.planObjective = strings.TrimSpace(objective)
		a.authoredSteps = nil
		a.generatedBound = maxSteps
	}
}

// WithVerify installs the authored verification criterion. When set, the
// verify phase runs one model call and writes the verdict fields; a `fail`
// verdict is data, not an operational failure.
func WithVerify(criterion string) Option {
	return func(a *PlannerAgent) {
		if a == nil {
			return
		}
		a.directiveMode = true
		a.verifyCriterion = strings.TrimSpace(criterion)
	}
}

// WithSummarize installs the authored summarizer instruction. When set, the
// summarize phase runs one model call and replaces the result with the summary,
// preserving the pre-summary aggregate as result_raw.
func WithSummarize(instruction string) Option {
	return func(a *PlannerAgent) {
		if a == nil {
			return
		}
		a.directiveMode = true
		a.summarizeInstruction = strings.TrimSpace(instruction)
	}
}

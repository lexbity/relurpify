package rewoo

import (
	"strings"

	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	"codeburg.org/lexbit/relurpify/context/contextstream"
	"codeburg.org/lexbit/relurpify/governance/permissions"
)

// Option configures a RewooAgent. Options are the paradigm's extension idiom:
// a runner constructed with zero options behaves exactly as it did before the
// option surface existed (library-use no-op rule, FR-9).
type Option func(*RewooAgent)

// New constructs a ReWOO agent from runtime dependencies and options.
func New(deps *paradigm.Deps, opts ...Option) *RewooAgent {
	agent := &RewooAgent{}
	for _, opt := range opts {
		if opt != nil {
			opt(agent)
		}
	}
	_ = agent.InitializeDeps(deps)
	return agent
}

// WithPermissionChecker installs the mandatory governance gate that backs
// every ReWOO tool step. A repository that supplies it via deps gets it wired
// by InitializeDeps; this option exists for compositions that override it.
func WithPermissionChecker(checker permissions.CapabilityChecker) Option {
	return func(a *RewooAgent) {
		if a != nil {
			a.Options.PermissionChecker = checker
		}
	}
}

// WithContextStreamMode sets the context stream mode used before plan
// execution.
func WithContextStreamMode(mode contextstream.Mode) Option {
	return func(a *RewooAgent) {
		if a != nil {
			a.Options.StreamMode = mode
		}
	}
}

// WithContextStreamQuery sets the context stream query used before plan
// execution.
func WithContextStreamQuery(query string) Option {
	return func(a *RewooAgent) {
		if a != nil {
			a.Options.StreamQuery = strings.TrimSpace(query)
		}
	}
}

// WithContextStreamMaxTokens sets the context stream token budget used before
// plan execution.
func WithContextStreamMaxTokens(maxTokens int) Option {
	return func(a *RewooAgent) {
		if a != nil && maxTokens > 0 {
			a.Options.StreamMaxTokens = maxTokens
		}
	}
}

// WithAuthoredPlan installs a recipe-authored plan: exactly these steps run,
// in declaration order, with zero planning LLM calls. The objective is the
// authored `plan` guidance carried into the aggregate context. Steps are
// copied so later mutation of the caller's slice cannot change the plan.
func WithAuthoredPlan(objective string, steps []RewooStep) Option {
	return func(a *RewooAgent) {
		if a == nil {
			return
		}
		a.Options.PlanObjective = strings.TrimSpace(objective)
		a.Options.AuthoredSteps = append([]RewooStep(nil), steps...)
	}
}

// WithSynthesizeGuidance replaces the default synthesizer instruction for this
// run when guidance is non-empty.
func WithSynthesizeGuidance(guidance string) Option {
	return func(a *RewooAgent) {
		if a != nil {
			a.Options.SynthesizeGuidance = strings.TrimSpace(guidance)
		}
	}
}

// WithPlanObjective carries the authored `plan` guidance into the generated
// planner prompt (and into the aggregate context) without authoring the step
// structure. An authored plan (WithAuthoredPlan) also carries its objective;
// this option exists for the `plan`-only generated mode.
func WithPlanObjective(objective string) Option {
	return func(a *RewooAgent) {
		if a != nil {
			a.Options.PlanObjective = strings.TrimSpace(objective)
		}
	}
}

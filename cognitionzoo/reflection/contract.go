package reflection

import (
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
)

// contract is the declared contract for the reflection paradigm: a review
// loop over a react delegate with bounded revision iterations.
//
// The restored directives (review/revise) are honored by the runner's option
// surface (D5): `review` drives the directive-mode review phase (the verdict is
// written to the ephemeral scratch namespace), and `revise when <predicate>:`
// runs its nested run/delegate body in-process, bounded by MaxRevisionCycles.
// `review` is deliberately not Required so a goal-only reflection recipe keeps
// the library review loop (FR-9); the D1 mixing rule is expressed by `revise`
// Requires `review`.
func contract() paradigm.Contract {
	return paradigm.Contract{
		Paradigm: "reflection",
		Summary:  "Review loop over a react delegate with bounded revision iterations.",
		Shape:    paradigm.ShapeReviewLoop,
		Composes: []string{"react"},
		Directives: []paradigm.DirectiveSpec{
			{Name: "review", Form: paradigm.FormLine, Text: paradigm.ArgOne},
			{Name: "revise", Form: paradigm.FormBlock, Text: paradigm.ArgNone, Predicate: true, Body: []paradigm.BodyItem{paradigm.BodyItemRun, paradigm.BodyItemDelegate}, Requires: []string{"review"}},
		},
		Order: &paradigm.OrderRule{Sequence: []string{"review", "revise"}},
		Guarantees: []string{
			"bounded revision iterations",
			"authored review verdicts live in the ephemeral scratch namespace",
			"composes the react delegate through the scoped-registry path",
		},
		Conformance: []paradigm.ConformanceCase{
			{
				ID:           "reflection/review_writes_scratch",
				Directive:    "review",
				Assertion:    "the review verdict is written to scratch.review / scratch.review_issues and the result fields",
				FixtureShape: "agent uses reflection; run: review \"…\"",
			},
			{
				ID:           "reflection/revise_fires_on_predicate",
				Directive:    "revise",
				Assertion:    "the revise body executes exactly once when scratch.review contains issues, then re-reviews",
				FixtureShape: "run: review \"…\"; revise when scratch.review contains issues: delegate to <agent>",
			},
			{
				ID:           "reflection/revision_cap_bounded",
				Directive:    "revise",
				Assertion:    "persistent issues run at most MaxRevisionCycles bodies and emit reflection.revision_capped",
				FixtureShape: "run: review \"…\"; revise when scratch.review contains issues: delegate to <agent>",
			},
			{
				ID:           "reflection/review_pass_ends_loop",
				Directive:    "review",
				Assertion:    "a pass verdict ends the loop without executing a revise body",
				FixtureShape: "run: review \"…\"; revise when scratch.review contains issues: delegate to <agent>",
			},
		},
	}
}

func init() {
	if err := paradigm.Register(contract()); err != nil {
		panic("register reflection paradigm contract: " + err.Error())
	}
}

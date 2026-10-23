package critic

import (
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
)

// contract is the declared contract for the critic paradigm: a drafting
// phase, an independent critique phase (its own model, its own criteria —
// it never edits the draft), and a bounded conditional revision.
//
// The directives mirror the reflection paradigm's shape: `critique` names
// the authored critique criterion for the directive-mode critique phase
// (the verdict is written to the ephemeral scratch namespace), and
// `revise when <predicate>:` runs its nested run/delegate body in-process,
// bounded by MaxRevisionCycles. `critique` is deliberately not Required, so
// a goal-only critic recipe keeps the library critique loop; the D1 mixing
// rule is expressed by `revise` Requires `critique`.
//
// Shape: ShapeReviewLoop, reused by convention. The source comment on the
// closed set states that ExecutionShape is display/provenance vocabulary
// only — the shape a paradigm executes is a conformance-test obligation,
// not a runtime switch. A new shape would add a display category without
// runtime semantics; ReviewLoop already names draft→critique→revise
// truthfully.
func contract() paradigm.Contract {
	return paradigm.Contract{
		Paradigm: "critic",
		Summary:  "Independent critique loop over a react delegate: separate critique phase, bounded conditional revision.",
		Shape:    paradigm.ShapeReviewLoop,
		Composes: []string{"react"},
		Directives: []paradigm.DirectiveSpec{
			{Name: "critique", Form: paradigm.FormLine, Text: paradigm.ArgOne},
			{Name: "revise", Form: paradigm.FormBlock, Text: paradigm.ArgNone, Predicate: true, Body: []paradigm.BodyItem{paradigm.BodyItemRun, paradigm.BodyItemDelegate}, Requires: []string{"critique"}},
		},
		Order: &paradigm.OrderRule{Sequence: []string{"critique", "revise"}},
		Guarantees: []string{
			"bounded revision iterations",
			"critique is a separate phase with its own model and criteria — it never edits the draft",
			"authored critique verdicts live in the ephemeral scratch namespace",
			"composes the react delegate through the scoped-registry path",
		},
		Conformance: []paradigm.ConformanceCase{
			{
				ID:           "critic/critique_writes_scratch",
				Directive:    "critique",
				Assertion:    "the critique verdict is written to scratch.critic.verdict / scratch.critic.issues and the result fields",
				FixtureShape: "agent uses critic; run: critique \"…\"",
			},
			{
				ID:           "critic/revise_fires_on_predicate",
				Directive:    "revise",
				Assertion:    "the revise body executes exactly once when the critique verdict contains issues, then re-critiques",
				FixtureShape: "run: critique \"…\"; revise when scratch.critic.verdict contains issues: delegate to <agent>",
			},
			{
				ID:           "critic/revision_cap_bounded",
				Directive:    "revise",
				Assertion:    "persistent issues run at most MaxRevisionCycles bodies and emit critic.revision_capped",
				FixtureShape: "run: critique \"…\"; revise when scratch.critic.verdict contains issues: delegate to <agent>",
			},
			{
				ID:           "critic/critique_pass_ends_loop",
				Directive:    "critique",
				Assertion:    "a pass verdict ends the loop without executing a revise body",
				FixtureShape: "run: critique \"…\"; revise when scratch.critic.verdict contains issues: delegate to <agent>",
			},
		},
	}
}

func init() {
	if err := paradigm.Register(contract()); err != nil {
		panic("register critic paradigm contract: " + err.Error())
	}
}

package blackboard

import (
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
)

// contract is the declared contract for the blackboard paradigm: a
// specialist-controller loop over shared blackboard lanes with a bounded cycle
// cap.
//
// The restored directive (source) is honored by the runner's option surface
// (D6): authored `source` blocks replace the built-in specialist set for the
// run and execute in declaration order with per-source episode re-arming; each
// source's `write` grounds capture-equivalently through the run's grounding
// path. `source` is deliberately not Required so a goal-only blackboard recipe
// keeps the built-in specialist loop (FR-9). The per-source clause order
// (when/read/do before write) and cardinality are enforced by the option
// builder, because the contract's order rule is a per-block property.
func contract() paradigm.Contract {
	return paradigm.Contract{
		Paradigm: "blackboard",
		Summary:  "Specialist-controller loop over shared blackboard lanes with a bounded cycle cap.",
		Shape:    paradigm.ShapeBlackboard,
		Directives: []paradigm.DirectiveSpec{
			// Body items: `when`/`read`/`write` lower as named directive
			// clauses (BodyItemDirective), `do` as a capability invocation.
			{Name: "source", Form: paradigm.FormBlock, Text: paradigm.ArgOne, Repeatable: true, Body: []paradigm.BodyItem{paradigm.BodyItemDirective, paradigm.BodyItemDo}},
		},
		Guarantees: []string{
			"controller loop with cycle cap",
			"authored sources execute in declaration order with episode re-arming",
			"authored source writes ground capture-equivalently with read-context provenance",
		},
		Conformance: []paradigm.ConformanceCase{
			{
				ID:           "blackboard/source_order_and_eligibility",
				Directive:    "source",
				Assertion:    "authored sources execute in declaration order, each gated by its when predicate",
				FixtureShape: "agent uses blackboard; run: source \"a\": write state.x: do …; source \"b\": when state.gate is ready: write state.y: do …",
			},
			{
				ID:           "blackboard/source_episode_rearm",
				Directive:    "source",
				Assertion:    "a when-gated source re-arms after its predicate transitions false→true and runs again; quiescence ends the loop",
				FixtureShape: "run: source \"set\": when state.gate is ready: do <unset>; source \"react\": when state.gate is missing: do <noop>",
			},
			{
				ID:           "blackboard/write_grounds_provenance",
				Directive:    "source",
				Assertion:    "the write target grounds capture-equivalently (chunk with the write's state key and derives_from read-context inputs)",
				FixtureShape: "run: source \"a\": read state.note: write state.out: do …",
			},
			{
				ID:           "blackboard/grounding_failure_fails_cycle",
				Directive:    "source",
				Assertion:    "a grounding admission failure at the cycle barrier fails the cycle as grounding_failed",
				FixtureShape: "run: source \"a\": write state.out: do … (fault-injected grounder)",
			},
		},
	}
}

func init() {
	if err := paradigm.Register(contract()); err != nil {
		panic("register blackboard paradigm contract: " + err.Error())
	}
}

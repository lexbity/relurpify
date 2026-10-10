package planner

import (
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
)

// contract is the declared contract for the planner paradigm: an explicit
// plan-then-execute loop with verify and summarize stages.
//
// The restored directives (plan/step/verify/summarize) are honored by the
// runner's option surface (D3): authored `step` blocks execute
// deterministically, `plan` alone generates a bounded plan, `verify` adds a
// quality verdict, and `summarize` replaces the result with a synthesized
// summary. `plan` is deliberately NOT required: a recipe that binds a planner
// agent and authors only a goal keeps the library's generated-from-goal
// behavior (FR-9 library-use no-op rule). The D1 mixing rule is expressed by
// `step` Requires `plan`: authored structure without its governing objective is
// a load error, not a silent mode switch.
func contract() paradigm.Contract {
	return paradigm.Contract{
		Paradigm: "planner",
		Summary:  "Plan-then-execute agent with verify and summarize stages.",
		Shape:    paradigm.ShapePlanExecute,
		Directives: []paradigm.DirectiveSpec{
			{Name: "plan", Form: paradigm.FormLine, Text: paradigm.ArgOne},
			{Name: "step", Form: paradigm.FormBlock, Text: paradigm.ArgOne, Repeatable: true, Body: []paradigm.BodyItem{paradigm.BodyItemDo}, Requires: []string{"plan"}},
			{Name: "verify", Form: paradigm.FormLine, Text: paradigm.ArgOne},
			{Name: "summarize", Form: paradigm.FormLine, Text: paradigm.ArgOne},
		},
		Order: &paradigm.OrderRule{Sequence: []string{"plan", "step", "verify", "summarize"}},
		Guarantees: []string{
			"plan before execute",
			"authored steps execute deterministically with zero planning model calls",
			"verify produces a verification observation",
			"envelope-based completed-step resume",
		},
		Conformance: []paradigm.ConformanceCase{
			{
				ID:           "planner/authored_plan_zero_planning_calls",
				Directive:    "plan",
				Assertion:    "authored plan+steps execute with zero planning-model calls; plan_origin=authored",
				FixtureShape: "agent uses planner; run: plan \"…\"; step \"…\": do relurpic:<cap>",
			},
			{
				ID:           "planner/authored_steps_execute_in_order",
				Directive:    "step",
				Assertion:    "authored steps run in declaration order through the tool executor",
				FixtureShape: "run: plan \"…\"; step \"A\": do …; step \"B\": do …",
			},
			{
				ID:           "planner/generated_plan_bounded",
				Directive:    "plan",
				Assertion:    "a plan-only directive generates a bounded plan with one model call; plan_origin=generated",
				FixtureShape: "run: plan \"…\" (no steps)",
			},
			{
				ID:           "planner/verify_verdict_fields",
				Directive:    "verify",
				Assertion:    "verify writes verification/verification_issues and a fail verdict is not an operational failure",
				FixtureShape: "run: plan \"…\"; step \"…\": do …; verify \"…\"",
			},
			{
				ID:           "planner/summarize_replaces_result",
				Directive:    "summarize",
				Assertion:    "summarize replaces result and preserves the pre-summary aggregate as result_raw",
				FixtureShape: "run: plan \"…\"; step \"…\": do …; summarize \"…\"",
			},
		},
	}
}

func init() {
	if err := paradigm.Register(contract()); err != nil {
		// An invalid or duplicate contract is a programming error in this
		// package; no recipe-load error text can reach this point.
		panic("register planner paradigm contract: " + err.Error())
	}
}

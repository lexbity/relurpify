package rewoo

import (
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
)

// contract is the declared contract for the rewoo paradigm: an authored or
// LLM-planned, step-executed, synthesized-result loop under governance.
//
// The restored directives (plan/step/synthesize) are honored by the runner's
// option surface (D2/D7): authored `plan` text with `step` blocks executes
// deterministically with zero planning model calls, and `synthesize` guidance
// replaces the default synthesizer instruction. A recipe that carries them in
// a rewoo run block is executed, not silently dropped.
func contract() paradigm.Contract {
	return paradigm.Contract{
		Paradigm: "rewoo",
		Summary:  "Authored or LLM-planned, step-executed, synthesized-result loop under governance.",
		Shape:    paradigm.ShapeStagedPlan,
		Directives: []paradigm.DirectiveSpec{
			{Name: "plan", Form: paradigm.FormLine, Text: paradigm.ArgOne, Required: true},
			{Name: "step", Form: paradigm.FormBlock, Text: paradigm.ArgOne, Repeatable: true, Body: []paradigm.BodyItem{paradigm.BodyItemDo}},
			{Name: "synthesize", Form: paradigm.FormLine, Text: paradigm.ArgOne},
		},
		Order: &paradigm.OrderRule{Sequence: []string{"plan", "step", "synthesize"}},
		Guarantees: []string{
			"governance-gated execution",
			"authored steps execute deterministically with zero planning model calls",
		},
		Conformance: []paradigm.ConformanceCase{
			{
				ID:           "rewoo/authored_plan_skips_planner",
				Directive:    "plan",
				Assertion:    "authored plan+steps execute with zero planning-model calls; plan_origin=authored",
				FixtureShape: "agent uses rewoo; run: plan \"…\"; step \"…\": do relurpic:<cap>",
			},
			{
				ID:           "rewoo/authored_steps_execute_in_order",
				Directive:    "step",
				Assertion:    "authored steps run in declaration order through the governed executor",
				FixtureShape: "run: plan \"…\"; step \"A\": do …; step \"B\": do …",
			},
			{
				ID:           "rewoo/synthesize_guidance",
				Directive:    "synthesize",
				Assertion:    "synthesize guidance reaches the synthesizer prompt as an authoritative system message",
				FixtureShape: "run: plan \"…\"; step \"…\": do …; synthesize \"…\"",
			},
		},
	}
}

func init() {
	if err := paradigm.Register(contract()); err != nil {
		panic("register rewoo paradigm contract: " + err.Error())
	}
}

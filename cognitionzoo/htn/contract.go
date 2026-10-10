package htn

import (
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
)

// contract is the declared contract for the htn paradigm: hierarchical task
// decomposition executed in-envelope over a react primitive executor.
//
// The restored directives (method/task) are honored by the runner's option
// surface (D4): an authored `method` block decomposes to exactly its authored
// tasks in declaration order with zero LLM decomposition calls, each task's
// optional `do` clause pinning its dispatch capability. `method` is
// deliberately not Required so a goal-only htn recipe keeps the library's
// method-lookup behavior (FR-9); the D1 mixing rule is expressed by `task`
// Requires `method`.
func contract() paradigm.Contract {
	return paradigm.Contract{
		Paradigm: "htn",
		Summary:  "Hierarchical task decomposition executed in-envelope over a react primitive executor.",
		Shape:    paradigm.ShapeDecomposition,
		Composes: []string{"react"},
		Directives: []paradigm.DirectiveSpec{
			{Name: "method", Form: paradigm.FormBlock, Text: paradigm.ArgOne, Body: []paradigm.BodyItem{paradigm.BodyItemDirective}},
			{Name: "task", Form: paradigm.FormBlock, Text: paradigm.ArgOne, Repeatable: true, Body: []paradigm.BodyItem{paradigm.BodyItemDo}, Requires: []string{"method"}},
		},
		Guarantees: []string{
			"executes in-envelope",
			"authored methods decompose to exactly the authored tasks with zero LLM decomposition",
			"composes the react primitive executor through the scoped-registry path",
		},
		Conformance: []paradigm.ConformanceCase{
			{
				ID:           "htn/authored_decomposition_order",
				Directive:    "method",
				Assertion:    "authored method tasks execute in declaration order with zero decomposition model calls",
				FixtureShape: "agent uses htn; run: method \"…\": task \"A\": do …; task \"B\": do …",
			},
			{
				ID:           "htn/task_capability_pin",
				Directive:    "task",
				Assertion:    "a task's do capability is the dispatched target; the task text is the sub-goal",
				FixtureShape: "run: method \"…\": task \"…\": do relurpic:<cap>",
			},
			{
				ID:           "htn/authored_resume",
				Directive:    "method",
				Assertion:    "completed tasks are not re-executed on resume",
				FixtureShape: "run: method \"…\": task A; task B (plan.completed_steps seeded with A)",
			},
		},
	}
}

func init() {
	if err := paradigm.Register(contract()); err != nil {
		panic("register htn paradigm contract: " + err.Error())
	}
}

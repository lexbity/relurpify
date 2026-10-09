package planner

import (
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
)

// contract is the declared contract for the planner paradigm: an explicit
// plan-then-execute loop with verify and summarize stages.
//
// The grammar carries no directive clause this paradigm honors today (the
// plan/step/verify/summarize vocabularies are documented but were never
// consumed by the planner's node code); per the implement-or-delete ruling
// those directives are retired from the contract, so a recipe that carries
// them in a planner run block is a load error instead of a silent no-op. The
// planner's plan/execute/verify/summarize stages remain structural behavior of
// the paradigm shape, exercised by the shape's own execution.
func contract() paradigm.Contract {
	return paradigm.Contract{
		Paradigm: "planner",
		Summary:  "Plan-then-execute agent with verify and summarize stages.",
		Shape:    paradigm.ShapePlanExecute,
		Guarantees: []string{
			"plan before execute",
			"verify produces a verification observation",
			"envelope-based completed-step resume",
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

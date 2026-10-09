package rewoo

import (
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
)

// contract is the declared contract for the rewoo paradigm: an LLM-planned,
// step-executed, synthesized-result loop under governance.
//
// The grammar carries no directive clause this paradigm honors today (the
// plan/step/synthesize vocabularies are documented but were never consumed by
// the runner's option surface); per the implement-or-delete ruling those
// directives are retired from the contract, so a recipe that carries them in a
// rewoo run block is a load error instead of a silent no-op.
func contract() paradigm.Contract {
	return paradigm.Contract{
		Paradigm: "rewoo",
		Summary:  "LLM-planned, step-executed, synthesized-result loop under governance.",
		Shape:    paradigm.ShapeStagedPlan,
		Guarantees: []string{
			"governance-gated execution",
		},
	}
}

func init() {
	if err := paradigm.Register(contract()); err != nil {
		panic("register rewoo paradigm contract: " + err.Error())
	}
}

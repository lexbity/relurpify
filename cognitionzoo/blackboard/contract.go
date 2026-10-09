package blackboard

import (
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
)

// contract is the declared contract for the blackboard paradigm: a
// specialist-controller loop over shared blackboard lanes with a bounded cycle
// cap.
//
// The grammar carries no directive clause this paradigm honors today (the
// source vocabulary is documented but was never consumed by the runner; every
// blackboard recipe runs the built-in specialist set); per the implement-or-
// delete ruling the source directive is retired from the contract, so a recipe
// that carries it in a blackboard run block is a load error instead of a
// silent no-op.
func contract() paradigm.Contract {
	return paradigm.Contract{
		Paradigm: "blackboard",
		Summary:  "Specialist-controller loop over shared blackboard lanes with a bounded cycle cap.",
		Shape:    paradigm.ShapeBlackboard,
		Guarantees: []string{
			"controller loop with cycle cap",
		},
	}
}

func init() {
	if err := paradigm.Register(contract()); err != nil {
		panic("register blackboard paradigm contract: " + err.Error())
	}
}

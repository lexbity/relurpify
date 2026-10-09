package reflection

import (
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
)

// contract is the declared contract for the reflection paradigm: a review
// loop over a react delegate.
//
// The grammar carries no directive clause this paradigm honors today (the
// review/revise vocabularies are documented but were never consumed by the
// runner); per the implement-or-delete ruling those directives are retired
// from the contract, so a recipe that carries them in a reflection run block
// is a load error instead of a silent no-op.
func contract() paradigm.Contract {
	return paradigm.Contract{
		Paradigm: "reflection",
		Summary:  "Review loop over a react delegate with bounded revision iterations.",
		Shape:    paradigm.ShapeReviewLoop,
		Composes: []string{"react"},
		Guarantees: []string{
			"bounded revision iterations",
			"composes the react delegate through the scoped-registry path",
		},
	}
}

func init() {
	if err := paradigm.Register(contract()); err != nil {
		panic("register reflection paradigm contract: " + err.Error())
	}
}

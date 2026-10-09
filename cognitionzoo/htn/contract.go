package htn

import (
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
)

// contract is the declared contract for the htn paradigm: hierarchical task
// decomposition executed in-envelope over a react primitive executor.
//
// The grammar carries no directive clause this paradigm honors today (method
// and task vocabularies are documented but were never consumed by the runner);
// per the implement-or-delete ruling those directives are retired from the
// contract, so a recipe that carries them in an htn run block is a load error
// instead of a silent no-op. Execution-shape persistence is Wave 1 territory.
func contract() paradigm.Contract {
	return paradigm.Contract{
		Paradigm: "htn",
		Summary:  "Hierarchical task decomposition executed in-envelope over a react primitive executor.",
		Shape:    paradigm.ShapeDecomposition,
		Composes: []string{"react"},
		Guarantees: []string{
			"executes in-envelope",
			"composes the react primitive executor through the scoped-registry path",
		},
	}
}

func init() {
	if err := paradigm.Register(contract()); err != nil {
		panic("register htn paradigm contract: " + err.Error())
	}
}

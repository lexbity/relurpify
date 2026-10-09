package react

import (
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
)

// contract is the declared contract for the react paradigm: an iterative
// think-act-observe loop bounded by an iteration budget, with the `until`
// directive capping iterations.
func contract() paradigm.Contract {
	return paradigm.Contract{
		Paradigm: "react",
		Summary:  "Iterative reason-act-observe agent bounded by an iteration budget.",
		Shape:    paradigm.ShapeIterative,
		Directives: []paradigm.DirectiveSpec{
			{
				Name:        "until",
				Form:        paradigm.FormLine,
				Text:        paradigm.ArgOne,
				ArgsInteger: true,
			},
		},
		Composes: nil,
		Guarantees: []string{
			"bounded iterations",
			"writes state only via capture",
		},
		Conformance: []paradigm.ConformanceCase{
			{
				ID:           "react/until_bounds_iterations",
				Directive:    "until",
				Assertion:    "the `until` directive caps the loop iteration budget (envelope must show the capped budget, not the default)",
				FixtureShape: "a run block carrying `until <n>` whose react execution must stop at n iterations",
			},
		},
	}
}

func init() {
	if err := paradigm.Register(contract()); err != nil {
		// An invalid or duplicate contract is a programming error in this
		// package; no recipe-load error text can reach this point.
		panic("register react paradigm contract: " + err.Error())
	}
}

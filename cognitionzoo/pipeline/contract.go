package pipeline

import (
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
)

// contract is the declared contract for the pipeline paradigm: a linear,
// ordered sequence of stages built directly by the grammar (PipelineDecl /
// stage blocks), with each stage executing its run/delegate steps in order.
//
// `stage` is a grammar structure, not a directive clause, so the contract
// declares no directive vocabulary. The stubbed `Runner.resume` path in the
// underlying package has no grammar surface and stays out of scope. A shape
// conformance case pins the linear ordering this contract promises.
func contract() paradigm.Contract {
	return paradigm.Contract{
		Paradigm: "pipeline",
		Summary:  "Linear sequence of stages, each executing its run/delegate steps in order.",
		Shape:    paradigm.ShapeLinear,
		Guarantees: []string{
			"stages execute in declared order",
		},
		Conformance: []paradigm.ConformanceCase{
			{
				ID:           "pipeline/stages_execute_in_order",
				Directive:    "",
				Assertion:    "a pipeline recipe executes its stage steps in declared order and records each stage's step result on the envelope",
				FixtureShape: "a pipeline with two named stages, each a run block with a distinct capture target",
			},
		},
	}
}

func init() {
	if err := paradigm.Register(contract()); err != nil {
		panic("register pipeline paradigm contract: " + err.Error())
	}
}

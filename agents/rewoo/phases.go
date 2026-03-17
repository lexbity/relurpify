package rewoo

import "github.com/lexcodex/relurpify/framework/contextmgr"

// RewooPhase identifies a phase of ReWOO execution.
type RewooPhase string

const (
	// PhasePlan: LLM generates a tool execution plan.
	PhasePlan RewooPhase = "plan"
	// PhaseExecute: tools are invoked mechanically.
	PhaseExecute RewooPhase = "execute"
	// PhaseSynthesize: LLM produces a final answer from tool results.
	PhaseSynthesize RewooPhase = "synthesize"
	// PhaseReplan: LLM generates a new plan based on prior failures.
	PhaseReplan RewooPhase = "replan"
)

// String returns the phase name.
func (p RewooPhase) String() string {
	return string(p)
}

// toDetailLevel maps a ReWOO phase to a context manager detail level.
// Planning and synthesis use detailed content; execution uses concise to save tokens.
func toDetailLevel(phase RewooPhase) contextmgr.DetailLevel {
	switch phase {
	case PhasePlan, PhaseSynthesize, PhaseReplan:
		return contextmgr.DetailDetailed
	case PhaseExecute:
		return contextmgr.DetailConcise
	default:
		return contextmgr.DetailConcise
	}
}

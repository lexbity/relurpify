package euclo

import "github.com/lexcodex/relurpify/named/euclo/gate"

// Re-export phase gate types and functions from gate subpackage for backward compatibility.
type (
	Phase             = gate.Phase
	GateFailPolicy    = gate.GateFailPolicy
	ValidatorCheck    = gate.ValidatorCheck
	ArtifactValidator = gate.ArtifactValidator
	ArtifactGate      = gate.ArtifactGate
	PhaseGate         = gate.PhaseGate
	GateEvaluation    = gate.GateEvaluation
)

const (
	PhaseExplore    = gate.PhaseExplore
	PhasePlan       = gate.PhasePlan
	PhaseEdit       = gate.PhaseEdit
	PhaseVerify     = gate.PhaseVerify
	PhaseReport     = gate.PhaseReport
	PhaseReproduce  = gate.PhaseReproduce
	PhaseLocalize   = gate.PhaseLocalize
	PhasePatch      = gate.PhasePatch
	PhasePlanTests  = gate.PhasePlanTests
	PhaseImplement  = gate.PhaseImplement
	PhaseReview     = gate.PhaseReview
	PhaseSummarize  = gate.PhaseSummarize
	PhaseTrace      = gate.PhaseTrace
	PhaseAnalyze    = gate.PhaseAnalyze
	PhaseStage      = gate.PhaseStage

	GateFailBlock       = gate.GateFailBlock
	GateFailWarn        = gate.GateFailWarn
	GateFailSkip        = gate.GateFailSkip

	ValidatorCheckNotEmpty = gate.ValidatorCheckNotEmpty
	ValidatorCheckHasKey   = gate.ValidatorCheckHasKey
	ValidatorCheckMinCount = gate.ValidatorCheckMinCount
)

var (
	EvaluateGate         = gate.EvaluateGate
	EvaluateGateSequence = gate.EvaluateGateSequence
)

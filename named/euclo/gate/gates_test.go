package gate

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
)

func TestEvaluateGatePassesWithAllArtifacts(t *testing.T) {
	gate := PhaseGate{
		From: PhaseExplore,
		To:   PhasePlan,
		DefaultGate: ArtifactGate{
			RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindExplore},
		},
		OnFail: GateFailBlock,
	}
	artifacts := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{Kind: euclotypes.ArtifactKindExplore, Payload: map[string]any{"files": []string{"main.go"}}},
	})

	eval := EvaluateGate(gate, "code", artifacts)
	require.True(t, eval.Passed)
	require.Empty(t, eval.Missing)
	require.Empty(t, eval.FailedValidators)
	require.Equal(t, PhaseExplore, eval.From)
	require.Equal(t, PhasePlan, eval.To)
	require.Equal(t, "code", eval.ModeID)
}

func TestEvaluateGateFailsOnMissingKind(t *testing.T) {
	gate := PhaseGate{
		From: PhaseExplore,
		To:   PhasePlan,
		DefaultGate: ArtifactGate{
			RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindExplore, euclotypes.ArtifactKindClassification},
		},
		OnFail: GateFailBlock,
	}
	artifacts := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{Kind: euclotypes.ArtifactKindExplore, Payload: "some exploration"},
	})

	eval := EvaluateGate(gate, "code", artifacts)
	require.False(t, eval.Passed)
	require.Contains(t, eval.Missing, euclotypes.ArtifactKindClassification)
	require.Equal(t, GateFailBlock, eval.Policy)
}

func TestEvaluateGateUsesModeScopedGate(t *testing.T) {
	gate := PhaseGate{
		From: PhaseReproduce,
		To:   PhaseLocalize,
		DefaultGate: ArtifactGate{
			RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindExplore},
		},
		ModeGates: map[string]ArtifactGate{
			"debug": {
				RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindExplore},
				Validators: []ArtifactValidator{
					{Kind: euclotypes.ArtifactKindExplore, Field: "status", Check: ValidatorCheckNotEmpty},
				},
			},
		},
		OnFail: GateFailBlock,
	}

	// With "code" mode: uses default gate, no validators → passes
	artifacts := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{Kind: euclotypes.ArtifactKindExplore, Payload: map[string]any{"files": []string{"a.go"}}},
	})
	eval := EvaluateGate(gate, "code", artifacts)
	require.True(t, eval.Passed)

	// With "debug" mode: uses mode gate, requires status field → fails
	eval = EvaluateGate(gate, "debug", artifacts)
	require.False(t, eval.Passed)
	require.NotEmpty(t, eval.FailedValidators)
}

func TestEvaluateGateFallsBackToDefault(t *testing.T) {
	gate := PhaseGate{
		From: PhasePlan,
		To:   PhaseEdit,
		DefaultGate: ArtifactGate{
			RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindPlan},
		},
		ModeGates: map[string]ArtifactGate{
			"debug": {
				RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindPlan, euclotypes.ArtifactKindAnalyze},
			},
		},
		OnFail: GateFailBlock,
	}

	artifacts := euclotypes.NewArtifactState([]euclotypes.Artifact{{Kind: euclotypes.ArtifactKindPlan, Payload: "a plan"}})

	// "tdd" is not in ModeGates → uses default
	eval := EvaluateGate(gate, "tdd", artifacts)
	require.True(t, eval.Passed)
}

func TestEvaluateGateWithValidatorNotEmpty(t *testing.T) {
	gate := PhaseGate{
		From: PhaseTrace,
		To:   PhaseAnalyze,
		DefaultGate: ArtifactGate{
			RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindExplore},
			Validators: []ArtifactValidator{
				{Kind: euclotypes.ArtifactKindExplore, Check: ValidatorCheckNotEmpty},
			},
		},
		OnFail: GateFailBlock,
	}

	// Non-empty payload → passes
	artifacts := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{Kind: euclotypes.ArtifactKindExplore, Payload: map[string]any{"trace": "data"}},
	})
	eval := EvaluateGate(gate, "debug", artifacts)
	require.True(t, eval.Passed)

	// Empty map payload → fails
	artifacts = euclotypes.NewArtifactState([]euclotypes.Artifact{
		{Kind: euclotypes.ArtifactKindExplore, Payload: map[string]any{}},
	})
	eval = EvaluateGate(gate, "debug", artifacts)
	require.False(t, eval.Passed)
	require.NotEmpty(t, eval.FailedValidators)

	// Nil payload → fails
	artifacts = euclotypes.NewArtifactState([]euclotypes.Artifact{
		{Kind: euclotypes.ArtifactKindExplore, Payload: nil},
	})
	eval = EvaluateGate(gate, "debug", artifacts)
	require.False(t, eval.Passed)

	// Empty string payload → fails
	artifacts = euclotypes.NewArtifactState([]euclotypes.Artifact{
		{Kind: euclotypes.ArtifactKindExplore, Payload: "  "},
	})
	eval = EvaluateGate(gate, "debug", artifacts)
	require.False(t, eval.Passed)
}

func TestEvaluateGateWithValidatorHasKey(t *testing.T) {
	gate := PhaseGate{
		From: PhasePlan,
		To:   PhaseEdit,
		DefaultGate: ArtifactGate{
			RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindPlan},
			Validators: []ArtifactValidator{
				{Kind: euclotypes.ArtifactKindPlan, Check: ValidatorCheckHasKey, Value: "steps"},
			},
		},
		OnFail: GateFailBlock,
	}

	// Has key → passes
	artifacts := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{Kind: euclotypes.ArtifactKindPlan, Payload: map[string]any{"steps": []any{"s1"}, "strategy": "fix"}},
	})
	eval := EvaluateGate(gate, "code", artifacts)
	require.True(t, eval.Passed)

	// Missing key → fails
	artifacts = euclotypes.NewArtifactState([]euclotypes.Artifact{
		{Kind: euclotypes.ArtifactKindPlan, Payload: map[string]any{"strategy": "fix"}},
	})
	eval = EvaluateGate(gate, "code", artifacts)
	require.False(t, eval.Passed)
	require.Len(t, eval.FailedValidators, 1)
	require.Contains(t, eval.FailedValidators[0], "missing key")
}

func TestEvaluateGateWithValidatorMinCount(t *testing.T) {
	gate := PhaseGate{
		From: PhasePlanTests,
		To:   PhaseImplement,
		DefaultGate: ArtifactGate{
			RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindPlan},
			Validators: []ArtifactValidator{
				{Kind: euclotypes.ArtifactKindPlan, Field: "steps", Check: ValidatorCheckMinCount, Value: 2},
			},
		},
		OnFail: GateFailBlock,
	}

	// Enough items → passes
	artifacts := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{Kind: euclotypes.ArtifactKindPlan, Payload: map[string]any{"steps": []any{"s1", "s2", "s3"}}},
	})
	eval := EvaluateGate(gate, "tdd", artifacts)
	require.True(t, eval.Passed)

	// Too few → fails
	artifacts = euclotypes.NewArtifactState([]euclotypes.Artifact{
		{Kind: euclotypes.ArtifactKindPlan, Payload: map[string]any{"steps": []any{"s1"}}},
	})
	eval = EvaluateGate(gate, "tdd", artifacts)
	require.False(t, eval.Passed)
}

func TestEvaluateGateWithFieldNotEmpty(t *testing.T) {
	gate := PhaseGate{
		From: PhaseReproduce,
		To:   PhaseLocalize,
		DefaultGate: ArtifactGate{
			RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindExplore},
			Validators: []ArtifactValidator{
				{Kind: euclotypes.ArtifactKindExplore, Field: "status", Check: ValidatorCheckNotEmpty},
			},
		},
		OnFail: GateFailBlock,
	}

	// Field present and non-empty → passes
	artifacts := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{Kind: euclotypes.ArtifactKindExplore, Payload: map[string]any{"status": "failing"}},
	})
	eval := EvaluateGate(gate, "debug", artifacts)
	require.True(t, eval.Passed)

	// Field missing → fails
	artifacts = euclotypes.NewArtifactState([]euclotypes.Artifact{
		{Kind: euclotypes.ArtifactKindExplore, Payload: map[string]any{"other": "data"}},
	})
	eval = EvaluateGate(gate, "debug", artifacts)
	require.False(t, eval.Passed)

	// Field empty string → fails
	artifacts = euclotypes.NewArtifactState([]euclotypes.Artifact{
		{Kind: euclotypes.ArtifactKindExplore, Payload: map[string]any{"status": ""}},
	})
	eval = EvaluateGate(gate, "debug", artifacts)
	require.False(t, eval.Passed)
}

func TestEvaluateGateEmptyGateAlwaysPasses(t *testing.T) {
	gate := PhaseGate{
		From:        PhaseExplore,
		To:          PhasePlan,
		DefaultGate: ArtifactGate{},
		OnFail:      GateFailBlock,
	}
	eval := EvaluateGate(gate, "code", euclotypes.NewArtifactState(nil))
	require.True(t, eval.Passed)
}

func TestEvaluateGateSequenceStopsOnBlock(t *testing.T) {
	gates := []PhaseGate{
		{
			From:        PhaseExplore,
			To:          PhasePlan,
			DefaultGate: ArtifactGate{RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindExplore}},
			OnFail:      GateFailBlock,
		},
		{
			From:        PhasePlan,
			To:          PhaseEdit,
			DefaultGate: ArtifactGate{RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindPlan}},
			OnFail:      GateFailBlock,
		},
	}

	// First gate fails → sequence stops, only one evaluation returned
	artifacts := euclotypes.NewArtifactState(nil)
	evals, passed := EvaluateGateSequence(gates, "code", artifacts)
	require.False(t, passed)
	require.Len(t, evals, 1)
	require.False(t, evals[0].Passed)
}

func TestEvaluateGateSequenceAllPassingReturnsAllEvals(t *testing.T) {
	gates := []PhaseGate{
		{
			From:        PhaseExplore,
			To:          PhasePlan,
			DefaultGate: ArtifactGate{RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindExplore}},
			OnFail:      GateFailBlock,
		},
		{
			From:        PhasePlan,
			To:          PhaseEdit,
			DefaultGate: ArtifactGate{RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindPlan}},
			OnFail:      GateFailBlock,
		},
	}

	artifacts := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{Kind: euclotypes.ArtifactKindExplore, Payload: "explored"},
		{Kind: euclotypes.ArtifactKindPlan, Payload: "planned"},
	})
	evals, passed := EvaluateGateSequence(gates, "code", artifacts)
	require.True(t, passed)
	require.Len(t, evals, 2)
	require.True(t, evals[0].Passed)
	require.True(t, evals[1].Passed)
}

func TestEvaluateGateSequenceAllowsWarnContinuation(t *testing.T) {
	gates := []PhaseGate{
		{
			From:        PhaseReview,
			To:          PhaseSummarize,
			DefaultGate: ArtifactGate{RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindAnalyze}},
			OnFail:      GateFailWarn,
		},
		{
			From:        PhaseSummarize,
			To:          PhaseReport,
			DefaultGate: ArtifactGate{},
			OnFail:      GateFailBlock,
		},
	}

	// First gate fails with warn → continues to second gate
	artifacts := euclotypes.NewArtifactState(nil)
	evals, passed := EvaluateGateSequence(gates, "review", artifacts)
	require.True(t, passed)
	require.Len(t, evals, 2)
	require.False(t, evals[0].Passed)
	require.True(t, evals[1].Passed)
}

func TestEvaluateGateSequenceSkipPolicyContinues(t *testing.T) {
	gates := []PhaseGate{
		{
			From:        PhaseStage,
			To:          PhaseSummarize,
			DefaultGate: ArtifactGate{RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindEditIntent}},
			OnFail:      GateFailSkip,
		},
	}

	artifacts := euclotypes.NewArtifactState(nil)
	evals, passed := EvaluateGateSequence(gates, "planning", artifacts)
	require.True(t, passed)
	require.Len(t, evals, 1)
	require.False(t, evals[0].Passed) // gate itself didn't pass
	require.Equal(t, GateFailSkip, evals[0].Policy)
}

func TestDefaultPhaseGatesExistForAllProfiles(t *testing.T) {
	gates := DefaultPhaseGates()
	expectedProfiles := []string{
		"edit_verify_repair",
		"reproduce_localize_patch",
		"test_driven_generation",
		"review_suggest_implement",
		"plan_stage_execute",
		"trace_execute_analyze",
	}
	for _, profile := range expectedProfiles {
		profileGates, ok := gates[profile]
		require.True(t, ok, "missing gates for profile %s", profile)
		require.NotEmpty(t, profileGates, "empty gates for profile %s", profile)
	}
}

func TestDefaultPhaseGatesEditVerifyRepairHasThreeTransitions(t *testing.T) {
	gates := DefaultPhaseGates()["edit_verify_repair"]
	require.Len(t, gates, 3)
	require.Equal(t, PhaseExplore, gates[0].From)
	require.Equal(t, PhasePlan, gates[0].To)
	require.Equal(t, PhasePlan, gates[1].From)
	require.Equal(t, PhaseEdit, gates[1].To)
	require.Equal(t, PhaseEdit, gates[2].From)
	require.Equal(t, PhaseVerify, gates[2].To)
}

func TestDefaultPhaseGatesReproduceLocalizePatchDebugModeHasValidators(t *testing.T) {
	gates := DefaultPhaseGates()["reproduce_localize_patch"]
	require.Len(t, gates, 3)

	// First gate (reproduce→localize) should have debug-specific validators
	firstGate := gates[0]
	debugGate, ok := firstGate.ModeGates["debug"]
	require.True(t, ok)
	require.NotEmpty(t, debugGate.Validators)
	require.Equal(t, ValidatorCheckNotEmpty, debugGate.Validators[0].Check)
}

func TestDefaultPhaseGatesTDDModeHasMinCountValidator(t *testing.T) {
	gates := DefaultPhaseGates()["test_driven_generation"]
	firstGate := gates[0]
	tddGate, ok := firstGate.ModeGates["tdd"]
	require.True(t, ok)
	require.Len(t, tddGate.Validators, 2)

	// Second validator checks min_count on steps
	require.Equal(t, ValidatorCheckMinCount, tddGate.Validators[1].Check)
	require.Equal(t, "steps", tddGate.Validators[1].Field)
	require.Equal(t, 1, tddGate.Validators[1].Value)
}

func TestValidatorNoArtifactsOfKindFails(t *testing.T) {
	gate := PhaseGate{
		From: PhaseExplore,
		To:   PhasePlan,
		DefaultGate: ArtifactGate{
			Validators: []ArtifactValidator{
				{Kind: euclotypes.ArtifactKindExplore, Check: ValidatorCheckNotEmpty},
			},
		},
		OnFail: GateFailBlock,
	}
	eval := EvaluateGate(gate, "code", euclotypes.NewArtifactState(nil))
	require.False(t, eval.Passed)
	require.Contains(t, eval.FailedValidators[0], "no artifacts of kind")
}

func TestValidatorHasKeyNonMapPayloadFails(t *testing.T) {
	gate := PhaseGate{
		From: PhasePlan,
		To:   PhaseEdit,
		DefaultGate: ArtifactGate{
			RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindPlan},
			Validators: []ArtifactValidator{
				{Kind: euclotypes.ArtifactKindPlan, Check: ValidatorCheckHasKey, Value: "steps"},
			},
		},
		OnFail: GateFailBlock,
	}
	artifacts := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{Kind: euclotypes.ArtifactKindPlan, Payload: "just a string"},
	})
	eval := EvaluateGate(gate, "code", artifacts)
	require.False(t, eval.Passed)
	require.Contains(t, eval.FailedValidators[0], "not a map")
}

func TestArtifactGateIsEmpty(t *testing.T) {
	require.True(t, ArtifactGate{}.IsEmpty())
	require.False(t, ArtifactGate{RequiredKinds: []euclotypes.ArtifactKind{euclotypes.ArtifactKindPlan}}.IsEmpty())
	require.False(t, ArtifactGate{Validators: []ArtifactValidator{{Kind: euclotypes.ArtifactKindPlan, Check: ValidatorCheckNotEmpty}}}.IsEmpty())
}

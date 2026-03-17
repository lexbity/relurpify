package euclo

import (
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

func TestArtifactStateHasReturnsTrueForPresentKind(t *testing.T) {
	state := NewArtifactState([]Artifact{
		{Kind: ArtifactKindPlan, Summary: "a plan"},
		{Kind: ArtifactKindVerification, Summary: "verified"},
	})
	require.True(t, state.Has(ArtifactKindPlan))
	require.True(t, state.Has(ArtifactKindVerification))
	require.False(t, state.Has(ArtifactKindEditIntent))
}

func TestArtifactStateOfKindReturnsMatchingSubset(t *testing.T) {
	state := NewArtifactState([]Artifact{
		{Kind: ArtifactKindPlan, Summary: "plan-1"},
		{Kind: ArtifactKindPlan, Summary: "plan-2"},
		{Kind: ArtifactKindVerification, Summary: "verified"},
	})
	plans := state.OfKind(ArtifactKindPlan)
	require.Len(t, plans, 2)
	require.Equal(t, "plan-1", plans[0].Summary)
	require.Equal(t, "plan-2", plans[1].Summary)

	empty := state.OfKind(ArtifactKindEditExecution)
	require.Empty(t, empty)
}

func TestNewArtifactStateEmptyIsUsable(t *testing.T) {
	state := NewArtifactState(nil)
	require.False(t, state.Has(ArtifactKindPlan))
	require.Empty(t, state.OfKind(ArtifactKindPlan))
	require.Empty(t, state.All())
	require.Equal(t, 0, state.Len())
}

func TestArtifactStateAll(t *testing.T) {
	artifacts := []Artifact{
		{Kind: ArtifactKindPlan},
		{Kind: ArtifactKindVerification},
	}
	state := NewArtifactState(artifacts)
	require.Len(t, state.All(), 2)
	require.Equal(t, 2, state.Len())
}

func TestArtifactStateFromContextExtractsArtifacts(t *testing.T) {
	ctx := core.NewContext()
	ctx.Set("euclo.artifacts", []Artifact{
		{Kind: ArtifactKindPlan, Summary: "the plan"},
	})
	state := ArtifactStateFromContext(ctx)
	require.True(t, state.Has(ArtifactKindPlan))
	require.Equal(t, 1, state.Len())
}

func TestArtifactStateFromContextHandlesMissingKey(t *testing.T) {
	ctx := core.NewContext()
	state := ArtifactStateFromContext(ctx)
	require.Equal(t, 0, state.Len())
}

func TestArtifactStateFromContextHandlesNilState(t *testing.T) {
	state := ArtifactStateFromContext(nil)
	require.Equal(t, 0, state.Len())
}

func TestArtifactContractSatisfiedByWithAllPresent(t *testing.T) {
	contract := ArtifactContract{
		RequiredInputs: []ArtifactRequirement{
			{Kind: ArtifactKindIntake, Required: true},
			{Kind: ArtifactKindClassification, Required: true},
		},
		ProducedOutputs: []ArtifactKind{ArtifactKindPlan},
	}
	state := NewArtifactState([]Artifact{
		{Kind: ArtifactKindIntake},
		{Kind: ArtifactKindClassification},
	})
	require.True(t, contract.SatisfiedBy(state))
}

func TestArtifactContractSatisfiedByFailsOnMissing(t *testing.T) {
	contract := ArtifactContract{
		RequiredInputs: []ArtifactRequirement{
			{Kind: ArtifactKindIntake, Required: true},
			{Kind: ArtifactKindPlan, Required: true},
		},
	}
	state := NewArtifactState([]Artifact{
		{Kind: ArtifactKindIntake},
	})
	require.False(t, contract.SatisfiedBy(state))
}

func TestArtifactContractSatisfiedByIgnoresOptional(t *testing.T) {
	contract := ArtifactContract{
		RequiredInputs: []ArtifactRequirement{
			{Kind: ArtifactKindIntake, Required: true},
			{Kind: ArtifactKindPlan, Required: false},
		},
	}
	state := NewArtifactState([]Artifact{
		{Kind: ArtifactKindIntake},
	})
	require.True(t, contract.SatisfiedBy(state))
}

func TestArtifactContractSatisfiedByRespectsMinCount(t *testing.T) {
	contract := ArtifactContract{
		RequiredInputs: []ArtifactRequirement{
			{Kind: ArtifactKindPlan, Required: true, MinCount: 2},
		},
	}
	single := NewArtifactState([]Artifact{{Kind: ArtifactKindPlan}})
	require.False(t, contract.SatisfiedBy(single))

	double := NewArtifactState([]Artifact{{Kind: ArtifactKindPlan}, {Kind: ArtifactKindPlan}})
	require.True(t, contract.SatisfiedBy(double))
}

func TestArtifactContractMissingInputs(t *testing.T) {
	contract := ArtifactContract{
		RequiredInputs: []ArtifactRequirement{
			{Kind: ArtifactKindIntake, Required: true},
			{Kind: ArtifactKindPlan, Required: true},
			{Kind: ArtifactKindVerification, Required: false},
		},
	}
	state := NewArtifactState([]Artifact{{Kind: ArtifactKindIntake}})
	missing := contract.MissingInputs(state)
	require.Len(t, missing, 1)
	require.Equal(t, ArtifactKindPlan, missing[0])
}

func TestArtifactContractEmptyRequirementsSatisfied(t *testing.T) {
	contract := ArtifactContract{}
	state := NewArtifactState(nil)
	require.True(t, contract.SatisfiedBy(state))
	require.Empty(t, contract.MissingInputs(state))
}

func TestExecutionStatusConstants(t *testing.T) {
	require.Equal(t, ExecutionStatus("completed"), ExecutionStatusCompleted)
	require.Equal(t, ExecutionStatus("partial"), ExecutionStatusPartial)
	require.Equal(t, ExecutionStatus("failed"), ExecutionStatusFailed)
}

func TestRecoveryStrategyConstants(t *testing.T) {
	require.Equal(t, RecoveryStrategy("paradigm_switch"), RecoveryStrategyParadigmSwitch)
	require.Equal(t, RecoveryStrategy("capability_fallback"), RecoveryStrategyCapabilityFallback)
	require.Equal(t, RecoveryStrategy("profile_escalation"), RecoveryStrategyProfileEscalation)
	require.Equal(t, RecoveryStrategy("mode_escalation"), RecoveryStrategyModeEscalation)
}

func TestExecutionResultWithFailureInfo(t *testing.T) {
	result := ExecutionResult{
		Status: ExecutionStatusFailed,
		FailureInfo: &CapabilityFailure{
			Code:            "reproduction_failed",
			Message:         "could not reproduce the bug",
			Recoverable:     true,
			FailedPhase:     "reproduce",
			MissingArtifact: ArtifactKindExplore,
			ParadigmUsed:    "react",
		},
		RecoveryHint: &RecoveryHint{
			Strategy:            RecoveryStrategyCapabilityFallback,
			SuggestedCapability: "euclo:edit_verify_repair",
		},
	}
	require.Equal(t, ExecutionStatusFailed, result.Status)
	require.True(t, result.FailureInfo.Recoverable)
	require.Equal(t, RecoveryStrategyCapabilityFallback, result.RecoveryHint.Strategy)
}

func TestExecutionResultCompleted(t *testing.T) {
	result := ExecutionResult{
		Status:  ExecutionStatusCompleted,
		Summary: "edit applied and verified",
		Artifacts: []Artifact{
			{Kind: ArtifactKindEditIntent, ProducerID: "euclo:edit_verify_repair", Status: "produced"},
			{Kind: ArtifactKindVerification, ProducerID: "euclo:edit_verify_repair", Status: "produced"},
		},
	}
	require.Equal(t, ExecutionStatusCompleted, result.Status)
	require.Len(t, result.Artifacts, 2)
	require.Nil(t, result.FailureInfo)
	require.Nil(t, result.RecoveryHint)
}

func TestEligibilityResultEligible(t *testing.T) {
	result := EligibilityResult{Eligible: true, Reason: "all requirements met"}
	require.True(t, result.Eligible)
}

func TestEligibilityResultIneligible(t *testing.T) {
	result := EligibilityResult{
		Eligible:         false,
		Reason:           "missing write tools",
		MissingArtifacts: []ArtifactKind{ArtifactKindEditIntent},
	}
	require.False(t, result.Eligible)
	require.Len(t, result.MissingArtifacts, 1)
}

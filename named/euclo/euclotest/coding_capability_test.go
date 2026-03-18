package euclotest

import (
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/stretchr/testify/require"
)

func TestArtifactStateHasReturnsTrueForPresentKind(t *testing.T) {
	state := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{Kind: euclotypes.ArtifactKindPlan, Summary: "a plan"},
		{Kind: euclotypes.ArtifactKindVerification, Summary: "verified"},
	})
	require.True(t, state.Has(euclotypes.ArtifactKindPlan))
	require.True(t, state.Has(euclotypes.ArtifactKindVerification))
	require.False(t, state.Has(euclotypes.ArtifactKindEditIntent))
}

func TestArtifactStateOfKindReturnsMatchingSubset(t *testing.T) {
	state := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{Kind: euclotypes.ArtifactKindPlan, Summary: "plan-1"},
		{Kind: euclotypes.ArtifactKindPlan, Summary: "plan-2"},
		{Kind: euclotypes.ArtifactKindVerification, Summary: "verified"},
	})
	plans := state.OfKind(euclotypes.ArtifactKindPlan)
	require.Len(t, plans, 2)
	require.Equal(t, "plan-1", plans[0].Summary)
	require.Equal(t, "plan-2", plans[1].Summary)

	empty := state.OfKind(euclotypes.ArtifactKindEditExecution)
	require.Empty(t, empty)
}

func TestNewArtifactStateEmptyIsUsable(t *testing.T) {
	state := euclotypes.NewArtifactState(nil)
	require.False(t, state.Has(euclotypes.ArtifactKindPlan))
	require.Empty(t, state.OfKind(euclotypes.ArtifactKindPlan))
	require.Empty(t, state.All())
	require.Equal(t, 0, state.Len())
}

func TestArtifactStateAll(t *testing.T) {
	artifacts := []euclotypes.Artifact{
		{Kind: euclotypes.ArtifactKindPlan},
		{Kind: euclotypes.ArtifactKindVerification},
	}
	state := euclotypes.NewArtifactState(artifacts)
	require.Len(t, state.All(), 2)
	require.Equal(t, 2, state.Len())
}

func TestArtifactStateFromContextExtractsArtifacts(t *testing.T) {
	ctx := core.NewContext()
	ctx.Set("euclo.artifacts", []euclotypes.Artifact{
		{Kind: euclotypes.ArtifactKindPlan, Summary: "the plan"},
	})
	state := euclotypes.ArtifactStateFromContext(ctx)
	require.True(t, state.Has(euclotypes.ArtifactKindPlan))
	require.Equal(t, 1, state.Len())
}

func TestArtifactStateFromContextHandlesMissingKey(t *testing.T) {
	ctx := core.NewContext()
	state := euclotypes.ArtifactStateFromContext(ctx)
	require.Equal(t, 0, state.Len())
}

func TestArtifactStateFromContextHandlesNilState(t *testing.T) {
	state := euclotypes.ArtifactStateFromContext(nil)
	require.Equal(t, 0, state.Len())
}

func TestArtifactContractSatisfiedByWithAllPresent(t *testing.T) {
	contract := euclotypes.ArtifactContract{
		RequiredInputs: []euclotypes.ArtifactRequirement{
			{Kind: euclotypes.ArtifactKindIntake, Required: true},
			{Kind: euclotypes.ArtifactKindClassification, Required: true},
		},
		ProducedOutputs: []euclotypes.ArtifactKind{euclotypes.ArtifactKindPlan},
	}
	state := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{Kind: euclotypes.ArtifactKindIntake},
		{Kind: euclotypes.ArtifactKindClassification},
	})
	require.True(t, contract.SatisfiedBy(state))
}

func TestArtifactContractSatisfiedByFailsOnMissing(t *testing.T) {
	contract := euclotypes.ArtifactContract{
		RequiredInputs: []euclotypes.ArtifactRequirement{
			{Kind: euclotypes.ArtifactKindIntake, Required: true},
			{Kind: euclotypes.ArtifactKindPlan, Required: true},
		},
	}
	state := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{Kind: euclotypes.ArtifactKindIntake},
	})
	require.False(t, contract.SatisfiedBy(state))
}

func TestArtifactContractSatisfiedByIgnoresOptional(t *testing.T) {
	contract := euclotypes.ArtifactContract{
		RequiredInputs: []euclotypes.ArtifactRequirement{
			{Kind: euclotypes.ArtifactKindIntake, Required: true},
			{Kind: euclotypes.ArtifactKindPlan, Required: false},
		},
	}
	state := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{Kind: euclotypes.ArtifactKindIntake},
	})
	require.True(t, contract.SatisfiedBy(state))
}

func TestArtifactContractSatisfiedByRespectsMinCount(t *testing.T) {
	contract := euclotypes.ArtifactContract{
		RequiredInputs: []euclotypes.ArtifactRequirement{
			{Kind: euclotypes.ArtifactKindPlan, Required: true, MinCount: 2},
		},
	}
	single := euclotypes.NewArtifactState([]euclotypes.Artifact{{Kind: euclotypes.ArtifactKindPlan}})
	require.False(t, contract.SatisfiedBy(single))

	double := euclotypes.NewArtifactState([]euclotypes.Artifact{{Kind: euclotypes.ArtifactKindPlan}, {Kind: euclotypes.ArtifactKindPlan}})
	require.True(t, contract.SatisfiedBy(double))
}

func TestArtifactContractMissingInputs(t *testing.T) {
	contract := euclotypes.ArtifactContract{
		RequiredInputs: []euclotypes.ArtifactRequirement{
			{Kind: euclotypes.ArtifactKindIntake, Required: true},
			{Kind: euclotypes.ArtifactKindPlan, Required: true},
			{Kind: euclotypes.ArtifactKindVerification, Required: false},
		},
	}
	state := euclotypes.NewArtifactState([]euclotypes.Artifact{{Kind: euclotypes.ArtifactKindIntake}})
	missing := contract.MissingInputs(state)
	require.Len(t, missing, 1)
	require.Equal(t, euclotypes.ArtifactKindPlan, missing[0])
}

func TestArtifactContractEmptyRequirementsSatisfied(t *testing.T) {
	contract := euclotypes.ArtifactContract{}
	state := euclotypes.NewArtifactState(nil)
	require.True(t, contract.SatisfiedBy(state))
	require.Empty(t, contract.MissingInputs(state))
}

func TestExecutionStatusConstants(t *testing.T) {
	require.Equal(t, euclotypes.ExecutionStatus("completed"), euclotypes.ExecutionStatusCompleted)
	require.Equal(t, euclotypes.ExecutionStatus("partial"), euclotypes.ExecutionStatusPartial)
	require.Equal(t, euclotypes.ExecutionStatus("failed"), euclotypes.ExecutionStatusFailed)
}

func TestRecoveryStrategyConstants(t *testing.T) {
	require.Equal(t, euclotypes.RecoveryStrategy("paradigm_switch"), euclotypes.RecoveryStrategyParadigmSwitch)
	require.Equal(t, euclotypes.RecoveryStrategy("capability_fallback"), euclotypes.RecoveryStrategyCapabilityFallback)
	require.Equal(t, euclotypes.RecoveryStrategy("profile_escalation"), euclotypes.RecoveryStrategyProfileEscalation)
	require.Equal(t, euclotypes.RecoveryStrategy("mode_escalation"), euclotypes.RecoveryStrategyModeEscalation)
}

func TestExecutionResultWithFailureInfo(t *testing.T) {
	result := euclotypes.ExecutionResult{
		Status: euclotypes.ExecutionStatusFailed,
		FailureInfo: &euclotypes.CapabilityFailure{
			Code:            "reproduction_failed",
			Message:         "could not reproduce the bug",
			Recoverable:     true,
			FailedPhase:     "reproduce",
			MissingArtifact: euclotypes.ArtifactKindExplore,
			ParadigmUsed:    "react",
		},
		RecoveryHint: &euclotypes.RecoveryHint{
			Strategy:            euclotypes.RecoveryStrategyCapabilityFallback,
			SuggestedCapability: "euclo:edit_verify_repair",
		},
	}
	require.Equal(t, euclotypes.ExecutionStatusFailed, result.Status)
	require.True(t, result.FailureInfo.Recoverable)
	require.Equal(t, euclotypes.RecoveryStrategyCapabilityFallback, result.RecoveryHint.Strategy)
}

func TestExecutionResultCompleted(t *testing.T) {
	result := euclotypes.ExecutionResult{
		Status:  euclotypes.ExecutionStatusCompleted,
		Summary: "edit applied and verified",
		Artifacts: []euclotypes.Artifact{
			{Kind: euclotypes.ArtifactKindEditIntent, ProducerID: "euclo:edit_verify_repair", Status: "produced"},
			{Kind: euclotypes.ArtifactKindVerification, ProducerID: "euclo:edit_verify_repair", Status: "produced"},
		},
	}
	require.Equal(t, euclotypes.ExecutionStatusCompleted, result.Status)
	require.Len(t, result.Artifacts, 2)
	require.Nil(t, result.FailureInfo)
	require.Nil(t, result.RecoveryHint)
}

func TestEligibilityResultEligible(t *testing.T) {
	result := euclotypes.EligibilityResult{Eligible: true, Reason: "all requirements met"}
	require.True(t, result.Eligible)
}

func TestEligibilityResultIneligible(t *testing.T) {
	result := euclotypes.EligibilityResult{
		Eligible:         false,
		Reason:           "missing write tools",
		MissingArtifacts: []euclotypes.ArtifactKind{euclotypes.ArtifactKindEditIntent},
	}
	require.False(t, result.Eligible)
	require.Len(t, result.MissingArtifacts, 1)
}

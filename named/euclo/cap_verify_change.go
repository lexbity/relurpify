package euclo

import (
	"context"

	reactpkg "github.com/lexcodex/relurpify/agents/react"
	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/core"
)

// verifyChangeCapability is a thin verification capability that runs a
// ReActAgent to check whether applied edits satisfy the task's acceptance
// criteria. It operates read-only and produces verification artifacts.
type verifyChangeCapability struct {
	env agentenv.AgentEnvironment
}

func (c *verifyChangeCapability) Descriptor() core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:            "euclo:verify.change",
		Name:          "Verify Change",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
		Tags:          []string{"coding", "verification"},
		Annotations: map[string]any{
			"supported_profiles": []string{
				"edit_verify_repair",
				"reproduce_localize_patch",
				"test_driven_generation",
			},
		},
	}
}

func (c *verifyChangeCapability) Contract() ArtifactContract {
	return ArtifactContract{
		RequiredInputs: []ArtifactRequirement{
			{Kind: ArtifactKindEditIntent, Required: false},
			{Kind: ArtifactKindEditExecution, Required: false},
		},
		ProducedOutputs: []ArtifactKind{
			ArtifactKindVerification,
		},
	}
}

func (c *verifyChangeCapability) Eligible(artifacts ArtifactState, snapshot CapabilitySnapshot) EligibilityResult {
	if !artifacts.Has(ArtifactKindEditIntent) && !artifacts.Has(ArtifactKindEditExecution) {
		return EligibilityResult{
			Eligible:         false,
			Reason:           "no edit intent or execution artifacts to verify",
			MissingArtifacts: []ArtifactKind{ArtifactKindEditIntent},
		}
	}
	return EligibilityResult{Eligible: true, Reason: "edit artifacts present for verification"}
}

func (c *verifyChangeCapability) Execute(ctx context.Context, env ExecutionEnvelope) ExecutionResult {
	producerID := "euclo:verify.change"

	verifyState := env.State.Clone()
	agent := reactpkg.New(env.Environment)
	task := &core.Task{
		ID:          "verify-change",
		Instruction: "Verify the applied changes by running tests and checking for issues.",
		Type:        core.TaskTypeAnalysis,
		Context:     taskContextFrom(env),
	}

	result, err := agent.Execute(ctx, task, verifyState)
	if err != nil || result == nil || !result.Success {
		return ExecutionResult{
			Status:  ExecutionStatusFailed,
			Summary: "verification failed",
			FailureInfo: &CapabilityFailure{
				Code:         "verification_failed",
				Message:      errMsg(err, result),
				Recoverable:  false,
				FailedPhase:  "verify",
				ParadigmUsed: "react",
			},
		}
	}

	mergeStateArtifacts(env.State, verifyState)

	return ExecutionResult{
		Status:  ExecutionStatusCompleted,
		Summary: "verification completed",
		Artifacts: []Artifact{
			{
				ID:         "verify_change_result",
				Kind:       ArtifactKindVerification,
				Summary:    resultSummary(result),
				Payload:    result.Data,
				ProducerID: producerID,
				Status:     "produced",
			},
		},
	}
}

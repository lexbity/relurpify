package euclo

import (
	"context"
	"fmt"

	reactpkg "github.com/lexcodex/relurpify/agents/react"
	reflectionpkg "github.com/lexcodex/relurpify/agents/reflection"
	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/core"
)

// reproduceLocalizePatchCapability implements the reproduce→localize→patch→verify
// profile using multi-paradigm composition:
//   - ReActAgent for reproduction (scoped read + execute tools)
//   - ReActAgent for localization (reading reproduction artifact + sources)
//   - ReActAgent for patch generation from localization
//   - ReflectionAgent for root-cause summary
//
// On reproduction failure, it suggests falling back to edit_verify_repair.
type reproduceLocalizePatchCapability struct {
	env agentenv.AgentEnvironment
}

func (c *reproduceLocalizePatchCapability) Descriptor() core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:            "euclo:reproduce_localize_patch",
		Name:          "Reproduce-Localize-Patch",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
		Tags:          []string{"coding", "debugging"},
		Annotations: map[string]any{
			"supported_profiles": []string{"reproduce_localize_patch"},
		},
	}
}

func (c *reproduceLocalizePatchCapability) Contract() ArtifactContract {
	return ArtifactContract{
		RequiredInputs: []ArtifactRequirement{
			{Kind: ArtifactKindIntake, Required: true},
		},
		ProducedOutputs: []ArtifactKind{
			ArtifactKindExplore,
			ArtifactKindAnalyze,
			ArtifactKindEditIntent,
			ArtifactKindVerification,
		},
	}
}

func (c *reproduceLocalizePatchCapability) Eligible(artifacts ArtifactState, snapshot CapabilitySnapshot) EligibilityResult {
	if !snapshot.HasWriteTools {
		return EligibilityResult{
			Eligible: false,
			Reason:   "write tools required for patching",
		}
	}
	if !snapshot.HasExecuteTools && !snapshot.HasVerificationTools {
		return EligibilityResult{
			Eligible: false,
			Reason:   "execute or verification tools required for reproduction",
		}
	}
	return EligibilityResult{Eligible: true, Reason: "write and execute tools available"}
}

func (c *reproduceLocalizePatchCapability) Execute(ctx context.Context, env ExecutionEnvelope) ExecutionResult {
	producerID := "euclo:reproduce_localize_patch"
	var artifacts []Artifact

	// Phase 1: Reproduce — ReActAgent with read + execute tools (no write)
	reproduceState := env.State.Clone()
	reproduceAgent := reactpkg.New(env.Environment)
	reproduceTask := &core.Task{
		ID:          "rlp-reproduce",
		Instruction: fmt.Sprintf("Reproduce the issue by running tests or triggering the failure: %s", capTaskInstruction(env.Task)),
		Type:        core.TaskTypeAnalysis,
		Context:     taskContextFrom(env),
	}
	reproduceResult, err := reproduceAgent.Execute(ctx, reproduceTask, reproduceState)
	if err != nil || reproduceResult == nil || !reproduceResult.Success {
		return ExecutionResult{
			Status:  ExecutionStatusFailed,
			Summary: "reproduction phase failed",
			FailureInfo: &CapabilityFailure{
				Code:         "reproduction_failed",
				Message:      errMsg(err, reproduceResult),
				Recoverable:  true,
				FailedPhase:  "reproduce",
				ParadigmUsed: "react",
			},
			RecoveryHint: &RecoveryHint{
				Strategy:            RecoveryStrategyCapabilityFallback,
				SuggestedCapability: "euclo:edit_verify_repair",
			},
		}
	}
	artifacts = append(artifacts, Artifact{
		ID:         "rlp_reproduce",
		Kind:       ArtifactKindExplore,
		Summary:    resultSummary(reproduceResult),
		Payload:    reproduceResult.Data,
		ProducerID: producerID,
		Status:     "produced",
	})
	mergeStateArtifacts(env.State, reproduceState)

	// Phase 2: Localize — ReActAgent reading reproduction evidence
	localizeState := env.State.Clone()
	localizeAgent := reactpkg.New(env.Environment)
	localizeTask := &core.Task{
		ID:          "rlp-localize",
		Instruction: fmt.Sprintf("Localize the root cause of the issue using reproduction evidence: %s", capTaskInstruction(env.Task)),
		Type:        core.TaskTypeAnalysis,
		Context:     taskContextFrom(env),
	}
	localizeResult, err := localizeAgent.Execute(ctx, localizeTask, localizeState)
	if err != nil || localizeResult == nil || !localizeResult.Success {
		return ExecutionResult{
			Status:    ExecutionStatusPartial,
			Summary:   "localization failed after successful reproduction",
			Artifacts: artifacts,
			FailureInfo: &CapabilityFailure{
				Code:            "localization_failed",
				Message:         errMsg(err, localizeResult),
				Recoverable:     true,
				FailedPhase:     "localize",
				MissingArtifact: ArtifactKindAnalyze,
				ParadigmUsed:    "react",
			},
			RecoveryHint: &RecoveryHint{
				Strategy:            RecoveryStrategyCapabilityFallback,
				SuggestedCapability: "euclo:edit_verify_repair",
			},
		}
	}
	artifacts = append(artifacts, Artifact{
		ID:         "rlp_localize",
		Kind:       ArtifactKindAnalyze,
		Summary:    resultSummary(localizeResult),
		Payload:    localizeResult.Data,
		ProducerID: producerID,
		Status:     "produced",
	})
	mergeStateArtifacts(env.State, localizeState)

	// Phase 3: Patch — ReActAgent for edit generation from localization
	patchState := env.State.Clone()
	patchAgent := reactpkg.New(env.Environment)
	patchTask := &core.Task{
		ID:          "rlp-patch",
		Instruction: fmt.Sprintf("Generate a patch to fix the localized issue: %s", capTaskInstruction(env.Task)),
		Type:        core.TaskTypeCodeModification,
		Context:     taskContextFrom(env),
	}
	patchResult, err := patchAgent.Execute(ctx, patchTask, patchState)
	if err != nil || patchResult == nil || !patchResult.Success {
		return ExecutionResult{
			Status:    ExecutionStatusPartial,
			Summary:   "patch generation failed after localization",
			Artifacts: artifacts,
			FailureInfo: &CapabilityFailure{
				Code:            "patch_failed",
				Message:         errMsg(err, patchResult),
				Recoverable:     true,
				FailedPhase:     "patch",
				MissingArtifact: ArtifactKindEditIntent,
				ParadigmUsed:    "react",
			},
		}
	}
	artifacts = append(artifacts, Artifact{
		ID:         "rlp_edit_intent",
		Kind:       ArtifactKindEditIntent,
		Summary:    resultSummary(patchResult),
		Payload:    patchResult.Data,
		ProducerID: producerID,
		Status:     "produced",
	})
	mergeStateArtifacts(env.State, patchState)

	// Phase 4: Review — ReflectionAgent for root-cause summary
	reviewState := env.State.Clone()
	reviewAgent := reflectionpkg.New(env.Environment, reactpkg.New(env.Environment))
	reviewTask := &core.Task{
		ID:          "rlp-review",
		Instruction: "Review the patch and verify it addresses the root cause.",
		Type:        core.TaskTypeAnalysis,
		Context:     taskContextFrom(env),
	}
	reviewResult, err := reviewAgent.Execute(ctx, reviewTask, reviewState)
	if err != nil || reviewResult == nil || !reviewResult.Success {
		// Review failure is not fatal — we still have the patch.
		return ExecutionResult{
			Status:    ExecutionStatusPartial,
			Summary:   "patch applied but review incomplete",
			Artifacts: artifacts,
			FailureInfo: &CapabilityFailure{
				Code:         "review_failed",
				Message:      errMsg(err, reviewResult),
				Recoverable:  false,
				FailedPhase:  "verify",
				ParadigmUsed: "reflection",
			},
		}
	}
	artifacts = append(artifacts, Artifact{
		ID:         "rlp_verification",
		Kind:       ArtifactKindVerification,
		Summary:    resultSummary(reviewResult),
		Payload:    reviewResult.Data,
		ProducerID: producerID,
		Status:     "produced",
	})
	mergeStateArtifacts(env.State, reviewState)

	return ExecutionResult{
		Status:    ExecutionStatusCompleted,
		Summary:   "reproduce-localize-patch completed successfully",
		Artifacts: artifacts,
	}
}

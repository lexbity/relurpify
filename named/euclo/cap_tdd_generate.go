package euclo

import (
	"context"
	"fmt"

	htnpkg "github.com/lexcodex/relurpify/agents/htn"
	reactpkg "github.com/lexcodex/relurpify/agents/react"
	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/core"
)

// tddGenerateCapability implements the test-driven generation profile using
// multi-paradigm composition:
//   - HTNAgent decomposes into test-first subtasks (write test → run → implement → run)
//   - ReActAgent executes each subtask
//
// The internal gate requires the HTN to produce at least one subtask before
// implementation proceeds.
type tddGenerateCapability struct {
	env agentenv.AgentEnvironment
}

func (c *tddGenerateCapability) Descriptor() core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:            "euclo:tdd.generate",
		Name:          "TDD Generate",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
		Tags:          []string{"coding", "tdd"},
		Annotations: map[string]any{
			"supported_profiles": []string{"test_driven_generation"},
		},
	}
}

func (c *tddGenerateCapability) Contract() ArtifactContract {
	return ArtifactContract{
		RequiredInputs: []ArtifactRequirement{
			{Kind: ArtifactKindIntake, Required: true},
		},
		ProducedOutputs: []ArtifactKind{
			ArtifactKindPlan,
			ArtifactKindEditIntent,
			ArtifactKindVerification,
		},
	}
}

func (c *tddGenerateCapability) Eligible(artifacts ArtifactState, snapshot CapabilitySnapshot) EligibilityResult {
	if !snapshot.HasWriteTools {
		return EligibilityResult{
			Eligible: false,
			Reason:   "write tools required for TDD implementation",
		}
	}
	if !snapshot.HasVerificationTools {
		return EligibilityResult{
			Eligible: false,
			Reason:   "verification tools required for TDD test execution",
		}
	}
	return EligibilityResult{Eligible: true, Reason: "write and verification tools available for TDD"}
}

func (c *tddGenerateCapability) Execute(ctx context.Context, env ExecutionEnvelope) ExecutionResult {
	producerID := "euclo:tdd.generate"
	var artifacts []Artifact

	// Phase 1: HTN decomposition — plan test-first subtasks
	planState := env.State.Clone()
	methods := htnpkg.NewMethodLibrary()
	htnAgent := htnpkg.New(env.Environment, methods)
	planTask := &core.Task{
		ID:          "tdd-plan",
		Instruction: fmt.Sprintf("Decompose into test-first subtasks (write test, run test, implement, verify): %s", capTaskInstruction(env.Task)),
		Type:        core.TaskTypeAnalysis,
		Context:     taskContextFrom(env),
	}
	planResult, err := htnAgent.Execute(ctx, planTask, planState)
	if err != nil || planResult == nil || !planResult.Success {
		return ExecutionResult{
			Status:  ExecutionStatusFailed,
			Summary: "TDD planning phase failed",
			FailureInfo: &CapabilityFailure{
				Code:         "tdd_plan_failed",
				Message:      errMsg(err, planResult),
				Recoverable:  true,
				FailedPhase:  "plan_tests",
				ParadigmUsed: "htn",
			},
			RecoveryHint: &RecoveryHint{
				Strategy:            RecoveryStrategyCapabilityFallback,
				SuggestedCapability: "euclo:edit_verify_repair",
			},
		}
	}
	artifacts = append(artifacts, Artifact{
		ID:         "tdd_plan",
		Kind:       ArtifactKindPlan,
		Summary:    resultSummary(planResult),
		Payload:    planResult.Data,
		ProducerID: producerID,
		Status:     "produced",
	})
	mergeStateArtifacts(env.State, planState)

	// Phase 2: Implement — ReActAgent executes the plan (write tests, then code)
	implState := env.State.Clone()
	implAgent := reactpkg.New(env.Environment)
	implTask := &core.Task{
		ID:          "tdd-implement",
		Instruction: fmt.Sprintf("Implement using test-driven development — write tests first, then implement: %s", capTaskInstruction(env.Task)),
		Type:        core.TaskTypeCodeModification,
		Context:     taskContextFrom(env),
	}
	implResult, err := implAgent.Execute(ctx, implTask, implState)
	if err != nil || implResult == nil || !implResult.Success {
		return ExecutionResult{
			Status:    ExecutionStatusPartial,
			Summary:   "TDD implementation failed after planning",
			Artifacts: artifacts,
			FailureInfo: &CapabilityFailure{
				Code:            "tdd_implement_failed",
				Message:         errMsg(err, implResult),
				Recoverable:     true,
				FailedPhase:     "implement",
				MissingArtifact: ArtifactKindEditIntent,
				ParadigmUsed:    "react",
			},
		}
	}
	artifacts = append(artifacts, Artifact{
		ID:         "tdd_edit_intent",
		Kind:       ArtifactKindEditIntent,
		Summary:    resultSummary(implResult),
		Payload:    implResult.Data,
		ProducerID: producerID,
		Status:     "produced",
	})
	mergeStateArtifacts(env.State, implState)

	// Phase 3: Verify — ReActAgent runs tests to confirm passing
	verifyState := env.State.Clone()
	verifyAgent := reactpkg.New(env.Environment)
	verifyTask := &core.Task{
		ID:          "tdd-verify",
		Instruction: "Run all tests to verify the TDD implementation passes.",
		Type:        core.TaskTypeAnalysis,
		Context:     taskContextFrom(env),
	}
	verifyResult, err := verifyAgent.Execute(ctx, verifyTask, verifyState)
	if err != nil || verifyResult == nil || !verifyResult.Success {
		return ExecutionResult{
			Status:    ExecutionStatusPartial,
			Summary:   "TDD verification failed — tests may not pass",
			Artifacts: artifacts,
			FailureInfo: &CapabilityFailure{
				Code:         "tdd_verify_failed",
				Message:      errMsg(err, verifyResult),
				Recoverable:  true,
				FailedPhase:  "verify",
				ParadigmUsed: "react",
			},
		}
	}
	artifacts = append(artifacts, Artifact{
		ID:         "tdd_verification",
		Kind:       ArtifactKindVerification,
		Summary:    resultSummary(verifyResult),
		Payload:    verifyResult.Data,
		ProducerID: producerID,
		Status:     "produced",
	})
	mergeStateArtifacts(env.State, verifyState)

	return ExecutionResult{
		Status:    ExecutionStatusCompleted,
		Summary:   "TDD generate completed — tests written and passing",
		Artifacts: artifacts,
	}
}

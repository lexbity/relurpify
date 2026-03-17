package euclo

import (
	"context"
	"fmt"

	reactpkg "github.com/lexcodex/relurpify/agents/react"
	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/core"
)

// editVerifyRepairCapability implements the edit→verify→repair execution
// profile as a concrete EucloCodingCapability. It composes:
//   - ReActAgent for exploration (read-only tools)
//   - ReActAgent for plan + edit (full tools)
//   - ReActAgent for verification (execute + read tools)
type editVerifyRepairCapability struct {
	env agentenv.AgentEnvironment
}

func (c *editVerifyRepairCapability) Descriptor() core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:            "euclo:edit_verify_repair",
		Name:          "Edit-Verify-Repair",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
		Tags:          []string{"coding", "implementation"},
		Annotations: map[string]any{
			"supported_profiles": []string{"edit_verify_repair"},
		},
	}
}

func (c *editVerifyRepairCapability) Contract() ArtifactContract {
	return ArtifactContract{
		RequiredInputs: []ArtifactRequirement{
			{Kind: ArtifactKindIntake, Required: true},
		},
		ProducedOutputs: []ArtifactKind{
			ArtifactKindExplore,
			ArtifactKindPlan,
			ArtifactKindEditIntent,
			ArtifactKindVerification,
		},
	}
}

func (c *editVerifyRepairCapability) Eligible(artifacts ArtifactState, snapshot CapabilitySnapshot) EligibilityResult {
	if !snapshot.HasWriteTools {
		return EligibilityResult{
			Eligible:         false,
			Reason:           "write tools required for edit_verify_repair",
			MissingArtifacts: []ArtifactKind{ArtifactKindEditIntent},
		}
	}
	if !snapshot.HasVerificationTools {
		return EligibilityResult{
			Eligible: false,
			Reason:   "verification tools required for edit_verify_repair",
		}
	}
	return EligibilityResult{Eligible: true, Reason: "write and verification tools available"}
}

func (c *editVerifyRepairCapability) Execute(ctx context.Context, env ExecutionEnvelope) ExecutionResult {
	producerID := "euclo:edit_verify_repair"
	var artifacts []Artifact

	// Phase 1: Explore — use ReActAgent with cloned state
	exploreState := env.State.Clone()
	exploreAgent := reactpkg.New(env.Environment)
	exploreTask := &core.Task{
		ID:          "evr-explore",
		Instruction: fmt.Sprintf("Explore the codebase to understand the context for: %s", capTaskInstruction(env.Task)),
		Type:        core.TaskTypeAnalysis,
		Context:     taskContextFrom(env),
	}
	exploreResult, err := exploreAgent.Execute(ctx, exploreTask, exploreState)
	if err != nil || exploreResult == nil || !exploreResult.Success {
		return ExecutionResult{
			Status:  ExecutionStatusFailed,
			Summary: "exploration phase failed",
			FailureInfo: &CapabilityFailure{
				Code:        "explore_failed",
				Message:     errMsg(err, exploreResult),
				Recoverable: true,
				FailedPhase: "explore",
				ParadigmUsed: "react",
			},
			RecoveryHint: &RecoveryHint{
				Strategy:          RecoveryStrategyParadigmSwitch,
				SuggestedParadigm: "planner",
			},
		}
	}
	artifacts = append(artifacts, Artifact{
		ID:         "evr_explore",
		Kind:       ArtifactKindExplore,
		Summary:    resultSummary(exploreResult),
		Payload:    exploreResult.Data,
		ProducerID: producerID,
		Status:     "produced",
	})
	mergeStateArtifacts(env.State, exploreState)

	// Phase 2: Plan + Edit — use ReActAgent with full tools
	editState := env.State.Clone()
	editAgent := reactpkg.New(env.Environment)
	editTask := &core.Task{
		ID:          "evr-edit",
		Instruction: fmt.Sprintf("Plan and implement the changes for: %s", capTaskInstruction(env.Task)),
		Type:        core.TaskTypeCodeModification,
		Context:     taskContextFrom(env),
	}
	editResult, err := editAgent.Execute(ctx, editTask, editState)
	if err != nil || editResult == nil || !editResult.Success {
		return ExecutionResult{
			Status:    ExecutionStatusPartial,
			Summary:   "edit phase failed after successful exploration",
			Artifacts: artifacts,
			FailureInfo: &CapabilityFailure{
				Code:            "edit_failed",
				Message:         errMsg(err, editResult),
				Recoverable:     true,
				FailedPhase:     "edit",
				MissingArtifact: ArtifactKindEditIntent,
				ParadigmUsed:    "react",
			},
			RecoveryHint: &RecoveryHint{
				Strategy:          RecoveryStrategyParadigmSwitch,
				SuggestedParadigm: "pipeline",
			},
		}
	}
	artifacts = append(artifacts,
		Artifact{
			ID:         "evr_plan",
			Kind:       ArtifactKindPlan,
			Summary:    "plan generated during edit phase",
			Payload:    editResult.Data,
			ProducerID: producerID,
			Status:     "produced",
		},
		Artifact{
			ID:         "evr_edit_intent",
			Kind:       ArtifactKindEditIntent,
			Summary:    resultSummary(editResult),
			Payload:    editResult.Data,
			ProducerID: producerID,
			Status:     "produced",
		},
	)
	mergeStateArtifacts(env.State, editState)

	// Phase 3: Verify — use ReActAgent
	verifyState := env.State.Clone()
	verifyAgent := reactpkg.New(env.Environment)
	verifyTask := &core.Task{
		ID:          "evr-verify",
		Instruction: "Verify the changes by running tests and checking for issues.",
		Type:        core.TaskTypeAnalysis,
		Context:     taskContextFrom(env),
	}
	verifyResult, err := verifyAgent.Execute(ctx, verifyTask, verifyState)
	if err != nil || verifyResult == nil || !verifyResult.Success {
		return ExecutionResult{
			Status:    ExecutionStatusPartial,
			Summary:   "verification phase failed after successful edit",
			Artifacts: artifacts,
			FailureInfo: &CapabilityFailure{
				Code:         "verify_failed",
				Message:      errMsg(err, verifyResult),
				Recoverable:  true,
				FailedPhase:  "verify",
				ParadigmUsed: "react",
			},
		}
	}
	artifacts = append(artifacts, Artifact{
		ID:         "evr_verification",
		Kind:       ArtifactKindVerification,
		Summary:    resultSummary(verifyResult),
		Payload:    verifyResult.Data,
		ProducerID: producerID,
		Status:     "produced",
	})
	mergeStateArtifacts(env.State, verifyState)

	return ExecutionResult{
		Status:    ExecutionStatusCompleted,
		Summary:   "edit-verify-repair completed successfully",
		Artifacts: artifacts,
	}
}

// capTaskInstruction extracts the instruction from a task, or returns a default.
func capTaskInstruction(task *core.Task) string {
	if task != nil && task.Instruction != "" {
		return task.Instruction
	}
	return "the requested change"
}

// taskContextFrom builds a context map from the execution envelope.
func taskContextFrom(env ExecutionEnvelope) map[string]any {
	ctx := map[string]any{
		"mode":    env.Mode.ModeID,
		"profile": env.Profile.ProfileID,
	}
	if env.Task != nil && env.Task.Context != nil {
		for k, v := range env.Task.Context {
			ctx[k] = v
		}
	}
	return ctx
}

// errMsg returns an error message from an error and/or result.
func errMsg(err error, result *core.Result) string {
	if err != nil {
		return err.Error()
	}
	if result != nil && result.Error != nil {
		return result.Error.Error()
	}
	return "unknown error"
}

// resultSummary extracts a summary from a result.
func resultSummary(result *core.Result) string {
	if result == nil {
		return ""
	}
	if result.Data != nil {
		if summary, ok := result.Data["summary"].(string); ok && summary != "" {
			return summary
		}
	}
	return "completed"
}

// mergeStateArtifacts copies relevant state keys from a child state back to
// the parent state.
func mergeStateArtifacts(parent, child *core.Context) {
	if parent == nil || child == nil {
		return
	}
	for _, key := range []string{
		"pipeline.explore", "pipeline.analyze", "pipeline.plan",
		"pipeline.code", "pipeline.verify", "pipeline.final_output",
	} {
		if raw, ok := child.Get(key); ok && raw != nil {
			parent.Set(key, raw)
		}
	}
}

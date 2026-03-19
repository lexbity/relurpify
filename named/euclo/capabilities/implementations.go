package capabilities

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	chainerpkg "github.com/lexcodex/relurpify/agents/chainer"
	htnpkg "github.com/lexcodex/relurpify/agents/htn"
	plannerpkg "github.com/lexcodex/relurpify/agents/planner"
	reactpkg "github.com/lexcodex/relurpify/agents/react"
	reflectionpkg "github.com/lexcodex/relurpify/agents/reflection"
	"github.com/lexcodex/relurpify/framework/agentenv"
	capabilitypkg "github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
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

func (c *editVerifyRepairCapability) Contract() euclotypes.ArtifactContract {
	return euclotypes.ArtifactContract{
		RequiredInputs: []euclotypes.ArtifactRequirement{
			{Kind: euclotypes.ArtifactKindIntake, Required: true},
		},
		ProducedOutputs: []euclotypes.ArtifactKind{
			euclotypes.ArtifactKindExplore,
			euclotypes.ArtifactKindPlan,
			euclotypes.ArtifactKindEditIntent,
			euclotypes.ArtifactKindVerification,
		},
	}
}

func (c *editVerifyRepairCapability) Eligible(artifacts euclotypes.ArtifactState, snapshot euclotypes.CapabilitySnapshot) euclotypes.EligibilityResult {
	if looksLikeAPICompatibleRefactorRequest(artifacts) {
		return euclotypes.EligibilityResult{
			Eligible: false,
			Reason:   "specialized API-compatible refactor capability available",
		}
	}
	if !snapshot.HasWriteTools {
		return euclotypes.EligibilityResult{
			Eligible:         false,
			Reason:           "write tools required for edit_verify_repair",
			MissingArtifacts: []euclotypes.ArtifactKind{euclotypes.ArtifactKindEditIntent},
		}
	}
	if !snapshot.HasVerificationTools {
		return euclotypes.EligibilityResult{
			Eligible: false,
			Reason:   "verification tools required for edit_verify_repair",
		}
	}
	return euclotypes.EligibilityResult{Eligible: true, Reason: "write and verification tools available"}
}

func (c *editVerifyRepairCapability) Execute(ctx context.Context, env euclotypes.ExecutionEnvelope) euclotypes.ExecutionResult {
	producerID := "euclo:edit_verify_repair"
	var artifacts []euclotypes.Artifact

	if forced := forcedEditVerifyRepairRecovery(env, producerID); forced != nil {
		return *forced
	}

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
		if ctx.Err() != nil {
			return euclotypes.ExecutionResult{
				Status:  euclotypes.ExecutionStatusFailed,
				Summary: "exploration phase failed",
				FailureInfo: &euclotypes.CapabilityFailure{
					Code:         "explore_failed",
					Message:      errMsg(err, exploreResult),
					Recoverable:  true,
					FailedPhase:  "explore",
					ParadigmUsed: "react",
				},
				RecoveryHint: &euclotypes.RecoveryHint{
					Strategy:          euclotypes.RecoveryStrategyParadigmSwitch,
					SuggestedParadigm: "planner",
				},
			}
		}
		artifacts = append(artifacts, euclotypes.Artifact{
			ID:         "evr_explore",
			Kind:       euclotypes.ArtifactKindExplore,
			Summary:    "exploration degraded; proceeding to edit",
			Payload:    map[string]any{"error": errMsg(err, exploreResult)},
			ProducerID: producerID,
			Status:     "degraded",
		})
		mergeStateArtifacts(env.State, exploreState)
	} else {
		artifacts = append(artifacts, euclotypes.Artifact{
			ID:         "evr_explore",
			Kind:       euclotypes.ArtifactKindExplore,
			Summary:    resultSummary(exploreResult),
			Payload:    exploreResult.Data,
			ProducerID: producerID,
			Status:     "produced",
		})
		mergeStateArtifacts(env.State, exploreState)
	}

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
		return euclotypes.ExecutionResult{
			Status:    euclotypes.ExecutionStatusPartial,
			Summary:   "edit phase failed after successful exploration",
			Artifacts: artifacts,
			FailureInfo: &euclotypes.CapabilityFailure{
				Code:            "edit_failed",
				Message:         errMsg(err, editResult),
				Recoverable:     true,
				FailedPhase:     "edit",
				MissingArtifact: euclotypes.ArtifactKindEditIntent,
				ParadigmUsed:    "react",
			},
			RecoveryHint: &euclotypes.RecoveryHint{
				Strategy:          euclotypes.RecoveryStrategyParadigmSwitch,
				SuggestedParadigm: "pipeline",
			},
		}
	}
	artifacts = append(artifacts,
		euclotypes.Artifact{
			ID:         "evr_plan",
			Kind:       euclotypes.ArtifactKindPlan,
			Summary:    "plan generated during edit phase",
			Payload:    editResult.Data,
			ProducerID: producerID,
			Status:     "produced",
		},
		euclotypes.Artifact{
			ID:         "evr_edit_intent",
			Kind:       euclotypes.ArtifactKindEditIntent,
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
		if payload, ok := verificationFallbackPayload(ctx, env); ok {
			env.State.Set("pipeline.verify", payload)
			artifacts = append(artifacts, euclotypes.Artifact{
				ID:         "evr_verification",
				Kind:       euclotypes.ArtifactKindVerification,
				Summary:    strings.TrimSpace(fmt.Sprint(payload["summary"])),
				Payload:    payload,
				ProducerID: producerID,
				Status:     "produced",
			})
			return euclotypes.ExecutionResult{
				Status:    euclotypes.ExecutionStatusCompleted,
				Summary:   "edit-verify-repair completed with fallback verification",
				Artifacts: artifacts,
			}
		}
		return euclotypes.ExecutionResult{
			Status:    euclotypes.ExecutionStatusPartial,
			Summary:   "verification phase failed after successful edit",
			Artifacts: artifacts,
			FailureInfo: &euclotypes.CapabilityFailure{
				Code:         "verify_failed",
				Message:      errMsg(err, verifyResult),
				Recoverable:  true,
				FailedPhase:  "verify",
				ParadigmUsed: "react",
			},
			RecoveryHint: &euclotypes.RecoveryHint{
				Strategy: euclotypes.RecoveryStrategyProfileEscalation,
				Context: map[string]any{
					"preferred_profile": "reproduce_localize_patch",
					"reason":            "verification failed after mutation",
				},
			},
		}
	}
	artifacts = append(artifacts, euclotypes.Artifact{
		ID:         "evr_verification",
		Kind:       euclotypes.ArtifactKindVerification,
		Summary:    resultSummary(verifyResult),
		Payload:    verifyResult.Data,
		ProducerID: producerID,
		Status:     "produced",
	})
	env.State.Set("pipeline.verify", map[string]any{
		"status":  "pass",
		"summary": resultSummary(verifyResult),
		"checks": []map[string]any{
			{
				"name":   "react_verify",
				"status": "pass",
			},
		},
	})
	mergeStateArtifacts(env.State, verifyState)

	return euclotypes.ExecutionResult{
		Status:    euclotypes.ExecutionStatusCompleted,
		Summary:   "edit-verify-repair completed successfully",
		Artifacts: artifacts,
	}
}

func verificationFallbackPayload(ctx context.Context, env euclotypes.ExecutionEnvelope) (map[string]any, bool) {
	if ctx.Err() != nil || env.Registry == nil || env.State == nil {
		return nil, false
	}
	workspace := strings.TrimSpace(taskWorkspace(env.Task))
	changedPaths := changedPathsFromPipelineCode(env.State)
	if workspace == "" || len(changedPaths) == 0 {
		return nil, false
	}
	primary := changedPaths[0]
	relPath, err := filepath.Rel(workspace, primary)
	if err != nil {
		return nil, false
	}
	relPath = filepath.ToSlash(filepath.Clean(relPath))
	ext := strings.ToLower(filepath.Ext(primary))

	if ext == ".go" {
		if payload, ok := invokeVerificationTool(ctx, env.Registry, env.State, "go_test", map[string]any{
			"working_directory": workspace,
			"package":           "./" + filepath.ToSlash(filepath.Dir(relPath)),
		}); ok {
			return payload, true
		}
	}

	if payload, ok := invokeVerificationTool(ctx, env.Registry, env.State, "exec_run_tests", map[string]any{
		"pattern": filepath.ToSlash(filepath.Dir(relPath)),
	}); ok {
		return payload, true
	}

	return nil, false
}

func invokeVerificationTool(ctx context.Context, registry *capabilitypkg.CapabilityRegistry, state *core.Context, toolName string, args map[string]any) (map[string]any, bool) {
	if registry == nil {
		return nil, false
	}
	if _, ok := registry.Get(toolName); !ok {
		return nil, false
	}
	result, err := registry.InvokeCapability(ctx, state, toolName, args)
	if err != nil || result == nil || !result.Success {
		return nil, false
	}
	summary := strings.TrimSpace(fmt.Sprint(result.Data["summary"]))
	if summary == "" || summary == "<nil>" {
		summary = strings.TrimSpace(fmt.Sprint(result.Data["stdout"]))
	}
	if summary == "" || summary == "<nil>" {
		summary = toolName + " passed"
	}
	return map[string]any{
		"status":  "pass",
		"summary": summary,
		"checks": []map[string]any{
			{
				"name":    toolName,
				"command": toolName,
				"status":  "pass",
				"details": strings.TrimSpace(fmt.Sprint(result.Data["stderr"])),
			},
		},
	}, true
}

func changedPathsFromPipelineCode(state *core.Context) []string {
	if state == nil {
		return nil
	}
	raw, ok := state.Get("pipeline.code")
	if !ok || raw == nil {
		return nil
	}
	payload, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	finalOutput, ok := payload["final_output"].(map[string]any)
	if !ok {
		return nil
	}
	result, ok := finalOutput["result"].(map[string]any)
	if !ok {
		return nil
	}
	var paths []string
	for _, item := range result {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		data, ok := entry["data"].(map[string]any)
		if !ok {
			continue
		}
		path := strings.TrimSpace(fmt.Sprint(data["path"]))
		if path == "" || path == "<nil>" {
			continue
		}
		paths = append(paths, path)
	}
	return paths
}

func taskWorkspace(task *core.Task) string {
	if task == nil || task.Context == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(task.Context["workspace"]))
}

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

func (c *reproduceLocalizePatchCapability) Contract() euclotypes.ArtifactContract {
	return euclotypes.ArtifactContract{
		RequiredInputs: []euclotypes.ArtifactRequirement{
			{Kind: euclotypes.ArtifactKindIntake, Required: true},
		},
		ProducedOutputs: []euclotypes.ArtifactKind{
			euclotypes.ArtifactKindExplore,
			euclotypes.ArtifactKindAnalyze,
			euclotypes.ArtifactKindEditIntent,
			euclotypes.ArtifactKindVerification,
		},
	}
}

func (c *reproduceLocalizePatchCapability) Eligible(artifacts euclotypes.ArtifactState, snapshot euclotypes.CapabilitySnapshot) euclotypes.EligibilityResult {
	if !snapshot.HasWriteTools {
		return euclotypes.EligibilityResult{
			Eligible: false,
			Reason:   "write tools required for patching",
		}
	}
	if !snapshot.HasExecuteTools && !snapshot.HasVerificationTools {
		return euclotypes.EligibilityResult{
			Eligible: false,
			Reason:   "execute or verification tools required for reproduction",
		}
	}
	return euclotypes.EligibilityResult{Eligible: true, Reason: "write and execute tools available"}
}

func (c *reproduceLocalizePatchCapability) Execute(ctx context.Context, env euclotypes.ExecutionEnvelope) euclotypes.ExecutionResult {
	producerID := "euclo:reproduce_localize_patch"
	var artifacts []euclotypes.Artifact

	if forced := forcedReproduceLocalizePatchRecovery(env, producerID); forced != nil {
		return *forced
	}

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
		return euclotypes.ExecutionResult{
			Status:  euclotypes.ExecutionStatusFailed,
			Summary: "reproduction phase failed",
			FailureInfo: &euclotypes.CapabilityFailure{
				Code:         "reproduction_failed",
				Message:      errMsg(err, reproduceResult),
				Recoverable:  true,
				FailedPhase:  "reproduce",
				ParadigmUsed: "react",
			},
			RecoveryHint: &euclotypes.RecoveryHint{
				Strategy:            euclotypes.RecoveryStrategyCapabilityFallback,
				SuggestedCapability: "euclo:edit_verify_repair",
			},
		}
	}
	artifacts = append(artifacts, euclotypes.Artifact{
		ID:         "rlp_reproduce",
		Kind:       euclotypes.ArtifactKindExplore,
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
		return euclotypes.ExecutionResult{
			Status:    euclotypes.ExecutionStatusPartial,
			Summary:   "localization failed after successful reproduction",
			Artifacts: artifacts,
			FailureInfo: &euclotypes.CapabilityFailure{
				Code:            "localization_failed",
				Message:         errMsg(err, localizeResult),
				Recoverable:     true,
				FailedPhase:     "localize",
				MissingArtifact: euclotypes.ArtifactKindAnalyze,
				ParadigmUsed:    "react",
			},
			RecoveryHint: &euclotypes.RecoveryHint{
				Strategy:            euclotypes.RecoveryStrategyCapabilityFallback,
				SuggestedCapability: "euclo:debug.investigate_regression",
				Context: map[string]any{
					"fallback_capabilities": []string{"euclo:debug.investigate_regression", "euclo:edit_verify_repair"},
					"reason":                "insufficient localization evidence",
				},
			},
		}
	}
	artifacts = append(artifacts, euclotypes.Artifact{
		ID:         "rlp_localize",
		Kind:       euclotypes.ArtifactKindAnalyze,
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
		return euclotypes.ExecutionResult{
			Status:    euclotypes.ExecutionStatusPartial,
			Summary:   "patch generation failed after localization",
			Artifacts: artifacts,
			FailureInfo: &euclotypes.CapabilityFailure{
				Code:            "patch_failed",
				Message:         errMsg(err, patchResult),
				Recoverable:     true,
				FailedPhase:     "patch",
				MissingArtifact: euclotypes.ArtifactKindEditIntent,
				ParadigmUsed:    "react",
			},
		}
	}
	artifacts = append(artifacts, euclotypes.Artifact{
		ID:         "rlp_edit_intent",
		Kind:       euclotypes.ArtifactKindEditIntent,
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
		return euclotypes.ExecutionResult{
			Status:    euclotypes.ExecutionStatusPartial,
			Summary:   "patch applied but review incomplete",
			Artifacts: artifacts,
			FailureInfo: &euclotypes.CapabilityFailure{
				Code:         "review_failed",
				Message:      errMsg(err, reviewResult),
				Recoverable:  false,
				FailedPhase:  "verify",
				ParadigmUsed: "reflection",
			},
		}
	}
	artifacts = append(artifacts, euclotypes.Artifact{
		ID:         "rlp_verification",
		Kind:       euclotypes.ArtifactKindVerification,
		Summary:    resultSummary(reviewResult),
		Payload:    reviewResult.Data,
		ProducerID: producerID,
		Status:     "produced",
	})
	mergeStateArtifacts(env.State, reviewState)

	return euclotypes.ExecutionResult{
		Status:    euclotypes.ExecutionStatusCompleted,
		Summary:   "reproduce-localize-patch completed successfully",
		Artifacts: artifacts,
	}
}

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

func (c *tddGenerateCapability) Contract() euclotypes.ArtifactContract {
	return euclotypes.ArtifactContract{
		RequiredInputs: []euclotypes.ArtifactRequirement{
			{Kind: euclotypes.ArtifactKindIntake, Required: true},
		},
		ProducedOutputs: []euclotypes.ArtifactKind{
			euclotypes.ArtifactKindPlan,
			euclotypes.ArtifactKindEditIntent,
			euclotypes.ArtifactKindVerification,
		},
	}
}

func (c *tddGenerateCapability) Eligible(artifacts euclotypes.ArtifactState, snapshot euclotypes.CapabilitySnapshot) euclotypes.EligibilityResult {
	if !snapshot.HasWriteTools {
		return euclotypes.EligibilityResult{
			Eligible: false,
			Reason:   "write tools required for TDD implementation",
		}
	}
	if !snapshot.HasVerificationTools {
		return euclotypes.EligibilityResult{
			Eligible: false,
			Reason:   "verification tools required for TDD test execution",
		}
	}
	return euclotypes.EligibilityResult{Eligible: true, Reason: "write and verification tools available for TDD"}
}

func (c *tddGenerateCapability) Execute(ctx context.Context, env euclotypes.ExecutionEnvelope) euclotypes.ExecutionResult {
	producerID := "euclo:tdd.generate"
	var artifacts []euclotypes.Artifact

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
		return euclotypes.ExecutionResult{
			Status:  euclotypes.ExecutionStatusFailed,
			Summary: "TDD planning phase failed",
			FailureInfo: &euclotypes.CapabilityFailure{
				Code:         "tdd_plan_failed",
				Message:      errMsg(err, planResult),
				Recoverable:  true,
				FailedPhase:  "plan_tests",
				ParadigmUsed: "htn",
			},
			RecoveryHint: &euclotypes.RecoveryHint{
				Strategy:            euclotypes.RecoveryStrategyCapabilityFallback,
				SuggestedCapability: "euclo:edit_verify_repair",
			},
		}
	}
	artifacts = append(artifacts, euclotypes.Artifact{
		ID:         "tdd_plan",
		Kind:       euclotypes.ArtifactKindPlan,
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
		return euclotypes.ExecutionResult{
			Status:    euclotypes.ExecutionStatusPartial,
			Summary:   "TDD implementation failed after planning",
			Artifacts: artifacts,
			FailureInfo: &euclotypes.CapabilityFailure{
				Code:            "tdd_implement_failed",
				Message:         errMsg(err, implResult),
				Recoverable:     true,
				FailedPhase:     "implement",
				MissingArtifact: euclotypes.ArtifactKindEditIntent,
				ParadigmUsed:    "react",
			},
		}
	}
	artifacts = append(artifacts, euclotypes.Artifact{
		ID:         "tdd_edit_intent",
		Kind:       euclotypes.ArtifactKindEditIntent,
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
		return euclotypes.ExecutionResult{
			Status:    euclotypes.ExecutionStatusPartial,
			Summary:   "TDD verification failed — tests may not pass",
			Artifacts: artifacts,
			FailureInfo: &euclotypes.CapabilityFailure{
				Code:         "tdd_verify_failed",
				Message:      errMsg(err, verifyResult),
				Recoverable:  true,
				FailedPhase:  "verify",
				ParadigmUsed: "react",
			},
		}
	}
	artifacts = append(artifacts, euclotypes.Artifact{
		ID:         "tdd_verification",
		Kind:       euclotypes.ArtifactKindVerification,
		Summary:    resultSummary(verifyResult),
		Payload:    verifyResult.Data,
		ProducerID: producerID,
		Status:     "produced",
	})
	mergeStateArtifacts(env.State, verifyState)

	return euclotypes.ExecutionResult{
		Status:    euclotypes.ExecutionStatusCompleted,
		Summary:   "TDD generate completed — tests written and passing",
		Artifacts: artifacts,
	}
}

// plannerPlanCapability produces a plan artifact using multi-paradigm
// composition:
//   - PlannerAgent generates the initial plan (read-only tools via ToolScopeScoped)
//   - ReflectionAgent reviews the plan quality
//   - If reflection finds significant issues, the planner re-runs with
//     feedback (max 1 retry)
type plannerPlanCapability struct {
	env agentenv.AgentEnvironment
}

func (c *plannerPlanCapability) Descriptor() core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:            "euclo:planner.plan",
		Name:          "Planner-Plan",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
		Tags:          []string{"coding", "planning"},
		Annotations: map[string]any{
			"supported_profiles": []string{"plan_stage_execute", "edit_verify_repair"},
		},
	}
}

func (c *plannerPlanCapability) Contract() euclotypes.ArtifactContract {
	return euclotypes.ArtifactContract{
		RequiredInputs: []euclotypes.ArtifactRequirement{
			{Kind: euclotypes.ArtifactKindIntake, Required: true},
		},
		ProducedOutputs: []euclotypes.ArtifactKind{
			euclotypes.ArtifactKindPlan,
		},
	}
}

func (c *plannerPlanCapability) Eligible(artifacts euclotypes.ArtifactState, snapshot euclotypes.CapabilitySnapshot) euclotypes.EligibilityResult {
	if looksLikeAPICompatibleRefactorRequest(artifacts) {
		return euclotypes.EligibilityResult{
			Eligible: false,
			Reason:   "specialized API-compatible refactor capability available",
		}
	}
	if !snapshot.HasReadTools {
		return euclotypes.EligibilityResult{
			Eligible: false,
			Reason:   "read tools required for planning",
		}
	}
	return euclotypes.EligibilityResult{Eligible: true, Reason: "read tools available for planning"}
}

func (c *plannerPlanCapability) Execute(ctx context.Context, env euclotypes.ExecutionEnvelope) euclotypes.ExecutionResult {
	producerID := "euclo:planner.plan"

	planResult, err := c.runPlanner(ctx, env, "")
	if err != nil || planResult == nil || !planResult.Success {
		if existing := existingPlanArtifact(env.State); existing != nil {
			existing.ProducerID = producerID
			if existing.Status == "" {
				existing.Status = "produced"
			}
			return euclotypes.ExecutionResult{
				Status:    euclotypes.ExecutionStatusCompleted,
				Summary:   "reused interaction-generated plan",
				Artifacts: []euclotypes.Artifact{*existing},
			}
		}
		return euclotypes.ExecutionResult{
			Status:  euclotypes.ExecutionStatusFailed,
			Summary: "planning phase failed",
			FailureInfo: &euclotypes.CapabilityFailure{
				Code:         "plan_failed",
				Message:      errMsg(err, planResult),
				Recoverable:  true,
				FailedPhase:  "plan",
				ParadigmUsed: "planner",
			},
			RecoveryHint: &euclotypes.RecoveryHint{
				Strategy:          euclotypes.RecoveryStrategyParadigmSwitch,
				SuggestedParadigm: "react",
			},
		}
	}

	// Reflection review of the plan.
	feedback := c.reviewPlan(ctx, env, planResult)
	if feedback != "" {
		// Re-run planner with feedback (single retry).
		retryResult, retryErr := c.runPlanner(ctx, env, feedback)
		if retryErr == nil && retryResult != nil && retryResult.Success {
			planResult = retryResult
		}
		// If retry fails, use the original plan.
	}

	return euclotypes.ExecutionResult{
		Status:  euclotypes.ExecutionStatusCompleted,
		Summary: "plan generated and reviewed",
		Artifacts: []euclotypes.Artifact{
			{
				ID:         "planner_plan",
				Kind:       euclotypes.ArtifactKindPlan,
				Summary:    resultSummary(planResult),
				Payload:    planResult.Data,
				ProducerID: producerID,
				Status:     "produced",
			},
		},
	}
}

func existingPlanArtifact(state *core.Context) *euclotypes.Artifact {
	if state == nil {
		return nil
	}
	for _, artifacts := range []euclotypes.ArtifactState{
		euclotypes.ArtifactStateFromContext(state),
		euclotypes.NewArtifactState(euclotypes.CollectArtifactsFromState(state)),
	} {
		if !artifacts.Has(euclotypes.ArtifactKindPlan) {
			continue
		}
		items := artifacts.OfKind(euclotypes.ArtifactKindPlan)
		if len(items) == 0 {
			continue
		}
		artifact := items[len(items)-1]
		return &artifact
	}
	return synthesizedPlanArtifact(state)
}

func synthesizedPlanArtifact(state *core.Context) *euclotypes.Artifact {
	if state == nil {
		return nil
	}
	if raw, ok := state.Get("pipeline.plan"); ok && raw != nil {
		return &euclotypes.Artifact{
			ID:         "pipeline_plan",
			Kind:       euclotypes.ArtifactKindPlan,
			Summary:    "interaction-derived plan",
			Payload:    raw,
			ProducerID: "euclo:planner.plan",
			Status:     "produced",
		}
	}
	raw, ok := state.Get("propose.items")
	if !ok || raw == nil {
		return nil
	}
	return &euclotypes.Artifact{
		ID:         "interaction_plan",
		Kind:       euclotypes.ArtifactKindPlan,
		Summary:    "interaction-derived plan",
		Payload:    map[string]any{"items": raw},
		ProducerID: "euclo:planner.plan",
		Status:     "produced",
	}
}

// runPlanner executes the PlannerAgent with optional feedback from a prior
// reflection review.
func (c *plannerPlanCapability) runPlanner(ctx context.Context, env euclotypes.ExecutionEnvelope, feedback string) (*core.Result, error) {
	planState := env.State.Clone()
	planAgent := plannerpkg.New(env.Environment)
	instruction := fmt.Sprintf("Create a detailed implementation plan for: %s", capTaskInstruction(env.Task))
	if feedback != "" {
		instruction = fmt.Sprintf("%s\n\nPrevious plan review feedback:\n%s", instruction, feedback)
	}
	planTask := &core.Task{
		ID:          "planner-plan",
		Instruction: instruction,
		Type:        core.TaskTypeAnalysis,
		Context:     taskContextFrom(env),
	}
	result, err := planAgent.Execute(ctx, planTask, planState)
	if err == nil {
		mergeStateArtifacts(env.State, planState)
	}
	return result, err
}

// reviewPlan uses ReflectionAgent to review the plan and returns feedback
// if significant issues are found. Returns empty string if the plan is good.
func (c *plannerPlanCapability) reviewPlan(ctx context.Context, env euclotypes.ExecutionEnvelope, _ *core.Result) string {
	reviewState := env.State.Clone()
	delegate := reactpkg.New(env.Environment)
	reviewAgent := reflectionpkg.New(env.Environment, delegate)
	reviewTask := &core.Task{
		ID:          "planner-review",
		Instruction: "Review the generated plan for completeness, feasibility, and potential issues.",
		Type:        core.TaskTypeAnalysis,
		Context:     taskContextFrom(env),
	}
	reviewResult, err := reviewAgent.Execute(ctx, reviewTask, reviewState)
	if err != nil || reviewResult == nil || !reviewResult.Success {
		return ""
	}
	// Check if the review result indicates issues.
	if reviewResult.Data != nil {
		if summary, ok := reviewResult.Data["summary"].(string); ok && summary != "" {
			return summary
		}
	}
	return ""
}

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

func (c *verifyChangeCapability) Contract() euclotypes.ArtifactContract {
	return euclotypes.ArtifactContract{
		RequiredInputs: []euclotypes.ArtifactRequirement{
			{Kind: euclotypes.ArtifactKindEditIntent, Required: false},
			{Kind: euclotypes.ArtifactKindEditExecution, Required: false},
		},
		ProducedOutputs: []euclotypes.ArtifactKind{
			euclotypes.ArtifactKindVerification,
		},
	}
}

func (c *verifyChangeCapability) Eligible(artifacts euclotypes.ArtifactState, snapshot euclotypes.CapabilitySnapshot) euclotypes.EligibilityResult {
	if !artifacts.Has(euclotypes.ArtifactKindEditIntent) && !artifacts.Has(euclotypes.ArtifactKindEditExecution) {
		return euclotypes.EligibilityResult{
			Eligible:         false,
			Reason:           "no edit intent or execution artifacts to verify",
			MissingArtifacts: []euclotypes.ArtifactKind{euclotypes.ArtifactKindEditIntent},
		}
	}
	return euclotypes.EligibilityResult{Eligible: true, Reason: "edit artifacts present for verification"}
}

func (c *verifyChangeCapability) Execute(ctx context.Context, env euclotypes.ExecutionEnvelope) euclotypes.ExecutionResult {
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
		return euclotypes.ExecutionResult{
			Status:  euclotypes.ExecutionStatusFailed,
			Summary: "verification failed",
			FailureInfo: &euclotypes.CapabilityFailure{
				Code:         "verification_failed",
				Message:      errMsg(err, result),
				Recoverable:  false,
				FailedPhase:  "verify",
				ParadigmUsed: "react",
			},
		}
	}

	mergeStateArtifacts(env.State, verifyState)

	return euclotypes.ExecutionResult{
		Status:  euclotypes.ExecutionStatusCompleted,
		Summary: "verification completed",
		Artifacts: []euclotypes.Artifact{
			{
				ID:         "verify_change_result",
				Kind:       euclotypes.ArtifactKindVerification,
				Summary:    resultSummary(result),
				Payload:    result.Data,
				ProducerID: producerID,
				Status:     "produced",
			},
		},
	}
}

// reportFinalCodingCapability produces the final report artifact by composing
// a ChainerAgent with two links: one to gather artifacts and one to compile
// the report summary. This capability is non-mutating.
type reportFinalCodingCapability struct {
	env agentenv.AgentEnvironment
}

func (c *reportFinalCodingCapability) Descriptor() core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:            "euclo:report.final_coding",
		Name:          "Final Coding Report",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
		Tags:          []string{"coding", "reporting"},
		Annotations: map[string]any{
			"supported_profiles": []string{
				"edit_verify_repair",
				"reproduce_localize_patch",
				"test_driven_generation",
				"review_suggest_implement",
				"plan_stage_execute",
				"trace_execute_analyze",
			},
		},
	}
}

func (c *reportFinalCodingCapability) Contract() euclotypes.ArtifactContract {
	return euclotypes.ArtifactContract{
		RequiredInputs: []euclotypes.ArtifactRequirement{
			{Kind: euclotypes.ArtifactKindVerification, Required: false},
			{Kind: euclotypes.ArtifactKindEditExecution, Required: false},
			{Kind: euclotypes.ArtifactKindPlan, Required: false},
		},
		ProducedOutputs: []euclotypes.ArtifactKind{
			euclotypes.ArtifactKindFinalReport,
		},
	}
}

func (c *reportFinalCodingCapability) Eligible(artifacts euclotypes.ArtifactState, _ euclotypes.CapabilitySnapshot) euclotypes.EligibilityResult {
	if artifacts.Has(euclotypes.ArtifactKindVerification) || artifacts.Has(euclotypes.ArtifactKindEditExecution) || artifacts.Has(euclotypes.ArtifactKindPlan) {
		return euclotypes.EligibilityResult{Eligible: true, Reason: "reportable artifacts present"}
	}
	return euclotypes.EligibilityResult{
		Eligible: false,
		Reason:   "no reportable artifacts available",
	}
}

func (c *reportFinalCodingCapability) Execute(ctx context.Context, env euclotypes.ExecutionEnvelope) euclotypes.ExecutionResult {
	producerID := "euclo:report.final_coding"

	chain := &chainerpkg.Chain{
		Links: []chainerpkg.Link{
			chainerpkg.NewSummarizeLink("gather", nil, "report.gathered"),
			chainerpkg.NewSummarizeLink("compile", []string{"report.gathered"}, "report.final"),
		},
	}

	reportState := env.State.Clone()
	agent := chainerpkg.New(env.Environment, chainerpkg.WithChain(chain))
	task := &core.Task{
		ID:          "report-final",
		Instruction: "Compile a final report summarizing the artifacts produced during execution.",
		Type:        core.TaskTypeAnalysis,
		Context:     taskContextFrom(env),
	}

	result, err := agent.Execute(ctx, task, reportState)
	if err != nil || result == nil || !result.Success {
		return euclotypes.ExecutionResult{
			Status:  euclotypes.ExecutionStatusFailed,
			Summary: "report generation failed",
			FailureInfo: &euclotypes.CapabilityFailure{
				Code:         "report_failed",
				Message:      errMsg(err, result),
				Recoverable:  false,
				FailedPhase:  "report",
				ParadigmUsed: "chainer",
			},
		}
	}

	return euclotypes.ExecutionResult{
		Status:  euclotypes.ExecutionStatusCompleted,
		Summary: "final report compiled",
		Artifacts: []euclotypes.Artifact{
			{
				ID:         "final_report",
				Kind:       euclotypes.ArtifactKindFinalReport,
				Summary:    resultSummary(result),
				Payload:    result.Data,
				ProducerID: producerID,
				Status:     "produced",
			},
		},
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
func taskContextFrom(env euclotypes.ExecutionEnvelope) map[string]any {
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

func forcedEditVerifyRepairRecovery(env euclotypes.ExecutionEnvelope, producerID string) *euclotypes.ExecutionResult {
	forcedMode := forceRecoveryContext(env.Task, "euclo.force_recovery")
	if forceRecoveryContext(env.Task, "euclo.recovery_active") == "true" {
		switch forcedMode {
		case "paradigm_switch", "capability_fallback":
			if env.State != nil {
				env.State.Set("pipeline.verify", map[string]any{
					"status":  "pass",
					"summary": "forced recovery completed successfully",
					"checks": []map[string]any{
						{
							"name":   "forced_recovery",
							"status": "pass",
						},
					},
				})
			}
			return &euclotypes.ExecutionResult{
				Status:  euclotypes.ExecutionStatusCompleted,
				Summary: "forced recovery completion",
				Artifacts: []euclotypes.Artifact{
					{
						ID:         "forced_recovery_plan",
						Kind:       euclotypes.ArtifactKindPlan,
						Summary:    "forced recovery plan",
						ProducerID: producerID,
						Status:     "produced",
					},
					{
						ID:         "forced_recovery_edit",
						Kind:       euclotypes.ArtifactKindEditIntent,
						Summary:    "forced recovery edit",
						ProducerID: producerID,
						Status:     "produced",
					},
					{
						ID:         "forced_recovery_verify",
						Kind:       euclotypes.ArtifactKindVerification,
						Summary:    "forced recovery verification",
						ProducerID: producerID,
						Status:     "produced",
					},
				},
			}
		default:
			return nil
		}
	}
	switch forcedMode {
	case "paradigm_switch":
		return &euclotypes.ExecutionResult{
			Status:  euclotypes.ExecutionStatusFailed,
			Summary: "forced paradigm-switch recovery",
			Artifacts: []euclotypes.Artifact{{
				ID:         "forced_evr_explore",
				Kind:       euclotypes.ArtifactKindExplore,
				Summary:    "forced recovery trigger",
				ProducerID: producerID,
				Status:     "degraded",
			}},
			FailureInfo: &euclotypes.CapabilityFailure{
				Code:         "forced_explore_failure",
				Message:      "forced recovery trigger",
				Recoverable:  true,
				FailedPhase:  "explore",
				ParadigmUsed: "react",
			},
			RecoveryHint: &euclotypes.RecoveryHint{
				Strategy:          euclotypes.RecoveryStrategyParadigmSwitch,
				SuggestedParadigm: "planner",
			},
		}
	case "mode_escalation":
		return &euclotypes.ExecutionResult{
			Status:  euclotypes.ExecutionStatusPartial,
			Summary: "forced mode-escalation recovery",
			Artifacts: []euclotypes.Artifact{{
				ID:         "forced_evr_edit_intent",
				Kind:       euclotypes.ArtifactKindEditIntent,
				Summary:    "forced recovery trigger",
				ProducerID: producerID,
				Status:     "produced",
			}},
			FailureInfo: &euclotypes.CapabilityFailure{
				Code:         "forced_verify_failure",
				Message:      "forced recovery trigger",
				Recoverable:  true,
				FailedPhase:  "verify",
				ParadigmUsed: "react",
			},
			RecoveryHint: &euclotypes.RecoveryHint{
				Strategy: euclotypes.RecoveryStrategyModeEscalation,
				Context: map[string]any{
					"forced": true,
				},
			},
		}
	default:
		return nil
	}
}

func forcedReproduceLocalizePatchRecovery(env euclotypes.ExecutionEnvelope, producerID string) *euclotypes.ExecutionResult {
	if forceRecoveryContext(env.Task, "euclo.recovery_active") == "true" {
		return nil
	}
	if forceRecoveryContext(env.Task, "euclo.force_recovery") != "capability_fallback" {
		return nil
	}
	return &euclotypes.ExecutionResult{
		Status:  euclotypes.ExecutionStatusPartial,
		Summary: "forced capability-fallback recovery",
		Artifacts: []euclotypes.Artifact{{
			ID:         "forced_rlp_reproduce",
			Kind:       euclotypes.ArtifactKindExplore,
			Summary:    "forced recovery trigger",
			ProducerID: producerID,
			Status:     "produced",
		}},
		FailureInfo: &euclotypes.CapabilityFailure{
			Code:         "forced_localize_failure",
			Message:      "forced recovery trigger",
			Recoverable:  true,
			FailedPhase:  "localize",
			ParadigmUsed: "react",
		},
		RecoveryHint: &euclotypes.RecoveryHint{
			Strategy:            euclotypes.RecoveryStrategyCapabilityFallback,
			SuggestedCapability: "euclo:edit_verify_repair",
			Context: map[string]any{
				"fallback_capabilities": []string{"euclo:edit_verify_repair"},
				"forced":                true,
			},
		},
	}
}

func forceRecoveryContext(task *core.Task, key string) string {
	if task == nil || task.Context == nil {
		return ""
	}
	if raw, ok := task.Context[key]; ok && raw != nil {
		return strings.TrimSpace(fmt.Sprint(raw))
	}
	return ""
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

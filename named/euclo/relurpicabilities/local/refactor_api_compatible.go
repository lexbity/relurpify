package local

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	"codeburg.org/lexbit/relurpify/named/euclo/execution"
	eucloruntime "codeburg.org/lexbit/relurpify/named/euclo/runtime"
)

type refactorAPICompatibleCapability struct {
	env agentenv.AgentEnvironment
}

func NewRefactorAPICompatibleCapability(env agentenv.AgentEnvironment) euclotypes.EucloCodingCapability {
	return &refactorAPICompatibleCapability{env: env}
}

func (c *refactorAPICompatibleCapability) Descriptor() core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:            "euclo:refactor.api_compatible",
		Name:          "API-Compatible Refactor",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
		Tags:          []string{"coding", "refactor", "compatibility"},
		Annotations:   map[string]any{"supported_profiles": []string{"edit_verify_repair", "plan_stage_execute"}},
	}
}

func (c *refactorAPICompatibleCapability) Contract() euclotypes.ArtifactContract {
	return euclotypes.ArtifactContract{
		RequiredInputs: []euclotypes.ArtifactRequirement{{Kind: euclotypes.ArtifactKindIntake, Required: true}},
		ProducedOutputs: []euclotypes.ArtifactKind{
			euclotypes.ArtifactKindEditIntent,
			euclotypes.ArtifactKindVerification,
			euclotypes.ArtifactKindCompatibilityAssessment,
		},
	}
}

func (c *refactorAPICompatibleCapability) Eligible(artifacts euclotypes.ArtifactState, snapshot euclotypes.CapabilitySnapshot) euclotypes.EligibilityResult {
	if !snapshot.HasWriteTools {
		return euclotypes.EligibilityResult{Eligible: false, Reason: "write tools required for API-compatible refactor"}
	}
	if !snapshot.HasVerificationTools {
		return euclotypes.EligibilityResult{Eligible: false, Reason: "verification tools required for API-compatible refactor"}
	}
	if !looksLikeAPICompatibleRefactorRequest(artifacts) {
		return euclotypes.EligibilityResult{Eligible: false, Reason: "API-compatible refactor requires explicit refactor plus compatibility intent"}
	}
	return euclotypes.EligibilityResult{Eligible: true, Reason: "explicit API-compatible refactor request"}
}

func (c *refactorAPICompatibleCapability) Execute(ctx context.Context, env euclotypes.ExecutionEnvelope) euclotypes.ExecutionResult {
	task := env.Task
	if task == nil {
		task = &core.Task{ID: "refactor-api-compatible", Instruction: "Perform an API-compatible refactor", Type: core.TaskTypeCodeModification}
	}
	plan, methodName, err := decomposeRefactorPlan(task)
	if err != nil {
		return euclotypes.ExecutionResult{
			Status:  euclotypes.ExecutionStatusFailed,
			Summary: "failed to decompose refactor request",
			FailureInfo: &euclotypes.CapabilityFailure{
				Code:         "refactor_decomposition_failed",
				Message:      err.Error(),
				Recoverable:  true,
				FailedPhase:  "decompose",
				ParadigmUsed: "htn",
			},
		}
	}
	producerID := "euclo:refactor.api_compatible"
	var artifacts []euclotypes.Artifact
	artifacts = append(artifacts, euclotypes.Artifact{
		ID:         "refactor_plan",
		Kind:       euclotypes.ArtifactKindPlan,
		Summary:    fmt.Sprintf("decomposed with %s into %d steps", methodName, len(plan.Steps)),
		Payload:    planToPayload(plan, methodName),
		ProducerID: producerID,
		Status:     "produced",
	})
	for index, step := range plan.Steps {
		if strings.HasSuffix(step.ID, ".verify") || step.ID == "verify" {
			continue
		}
		stepArtifact := proposedRefactorEditArtifact(env, task, step, index+1, len(plan.Steps), methodName, producerID)
		gateArtifact := compatibilityArtifactForStep(env, stepArtifact, step, producerID)
		artifacts = append(artifacts, gateArtifact)
		if !compatibilityPassed(gateArtifact) {
			mergeStateArtifactsToContext(env.State, artifacts)
			return euclotypes.ExecutionResult{
				Status:    euclotypes.ExecutionStatusCompleted,
				Summary:   fmt.Sprintf("blocked refactor step %q due to API compatibility risk", step.ID),
				Artifacts: artifacts,
				RecoveryHint: &euclotypes.RecoveryHint{
					Strategy: euclotypes.RecoveryStrategyModeEscalation,
					Context:  map[string]any{"blocked_step": step.ID, "blocking_capability": "euclo:review.compatibility", "compatibility_strict": true},
				},
			}
		}
		stepTask := &core.Task{ID: fmt.Sprintf("%s-step-%d", task.ID, index+1), Instruction: step.Description, Type: core.TaskTypeCodeModification, Context: refactorStepContext(env, step, index+1, len(plan.Steps), methodName)}
		stepEnv := env
		stepEnv.Task = stepTask
		result, state, err := execution.ExecuteEnvelopeRecipe(ctx, stepEnv, execution.RecipeChatImplementEdit, stepTask.ID, step.Description)
		if err != nil || result == nil || !result.Success {
			mergeStateArtifactsToContext(env.State, artifacts)
			return euclotypes.ExecutionResult{
				Status:    euclotypes.ExecutionStatusPartial,
				Summary:   fmt.Sprintf("refactor step %q failed", step.ID),
				Artifacts: artifacts,
				FailureInfo: &euclotypes.CapabilityFailure{
					Code:         "refactor_step_failed",
					Message:      errMsg(err, result),
					Recoverable:  true,
					FailedPhase:  "execute",
					ParadigmUsed: "react",
				},
			}
		}
		execution.PropagateBehaviorTrace(env.State, state)
		editPayload := result.Data
		if editPayload == nil {
			editPayload = map[string]any{"summary": resultSummary(result), "step_id": step.ID}
		}
		artifacts = append(artifacts, euclotypes.Artifact{
			ID:         fmt.Sprintf("refactor_step_%d_edit_intent", index+1),
			Kind:       euclotypes.ArtifactKindEditIntent,
			Summary:    firstNonEmpty(resultSummary(result), step.Description),
			Payload:    editPayload,
			ProducerID: producerID,
			Status:     "produced",
		})
	}
	verifyArtifact := (&reviewCompatibilityCapability{env: c.env}).Execute(ctx, env)
	artifacts = append(artifacts, verifyArtifact.Artifacts...)
	mergeStateArtifactsToContext(env.State, artifacts)
	if verificationArtifacts, executed, execErr := ExecuteVerificationFlow(ctx, env, eucloruntime.SnapshotCapabilities(env.Registry)); execErr != nil {
		return euclotypes.ExecutionResult{
			Status:    euclotypes.ExecutionStatusFailed,
			Summary:   "API-compatible refactor failed verification",
			Artifacts: artifacts,
			FailureInfo: &euclotypes.CapabilityFailure{
				Code:         "refactor_api_compatible_verification_failed",
				Message:      execErr.Error(),
				Recoverable:  true,
				FailedPhase:  "verify",
				ParadigmUsed: "react",
			},
		}
	} else if executed {
		artifacts = append(artifacts, verificationArtifacts...)
		if verifyPayload, ok := verificationPayloadFromState(env.State); ok && verificationPayloadFailed(verifyPayload) {
			repairResult := NewFailedVerificationRepairCapability(c.env).Execute(ctx, env)
			artifacts = append(artifacts, repairResult.Artifacts...)
			if repairResult.Status == euclotypes.ExecutionStatusFailed {
				return repairResult
			}
			mergeStateArtifactsToContext(env.State, artifacts)
			return euclotypes.ExecutionResult{Status: euclotypes.ExecutionStatusCompleted, Summary: "API-compatible refactor repaired failing verification", Artifacts: artifacts}
		}
	} else if existing, ok := env.State.Get("pipeline.verify"); ok && existing != nil {
		artifacts = append(artifacts, euclotypes.Artifact{
			ID:         "refactor_verification",
			Kind:       euclotypes.ArtifactKindVerification,
			Summary:    "API-compatible refactor verification recorded",
			Payload:    existing,
			ProducerID: producerID,
			Status:     "produced",
		})
		if verifyPayload, ok := existing.(map[string]any); ok && verificationPayloadFailed(verifyPayload) {
			repairResult := NewFailedVerificationRepairCapability(c.env).Execute(ctx, env)
			artifacts = append(artifacts, repairResult.Artifacts...)
			if repairResult.Status == euclotypes.ExecutionStatusFailed {
				return repairResult
			}
			mergeStateArtifactsToContext(env.State, artifacts)
			return euclotypes.ExecutionResult{Status: euclotypes.ExecutionStatusCompleted, Summary: "API-compatible refactor repaired failing verification", Artifacts: artifacts}
		}
	}
	mergeStateArtifactsToContext(env.State, artifacts)
	return euclotypes.ExecutionResult{Status: euclotypes.ExecutionStatusCompleted, Summary: "API-compatible refactor completed", Artifacts: artifacts}
}

func looksLikeRefactorInstruction(text string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return false
	}
	for _, token := range []string{"refactor", "rename", "extract function", "extract helper", "inline", "move to file", "reorganize", "consolidate", "deduplicate", "extract interface"} {
		if strings.Contains(text, token) {
			return true
		}
	}
	return false
}

func looksLikeAPICompatibleRefactorRequest(artifacts euclotypes.ArtifactState) bool {
	text := strings.ToLower(strings.TrimSpace(instructionFromArtifacts(artifacts)))
	if !looksLikeRefactorInstruction(text) {
		return false
	}
	for _, token := range []string{"api compatible", "api-compatible", "public api", "backward compatible", "backwards compatible", "without changing api", "without changing the api", "keep api", "keep the api", "preserve api", "preserve the public api", "public surface", "do not break callers"} {
		if strings.Contains(text, token) {
			return true
		}
	}
	return false
}

func planToPayload(plan *core.Plan, methodName string) map[string]any {
	if plan == nil {
		return map[string]any{"method": methodName}
	}
	steps := make([]map[string]any, 0, len(plan.Steps))
	for _, step := range plan.Steps {
		steps = append(steps, map[string]any{"id": step.ID, "description": step.Description, "expected": step.Expected})
	}
	return map[string]any{"goal": plan.Goal, "method": methodName, "steps": steps, "dependencies": plan.Dependencies}
}

func proposedRefactorEditArtifact(env euclotypes.ExecutionEnvelope, task *core.Task, step core.PlanStep, index, total int, methodName, producerID string) euclotypes.Artifact {
	afterSurface := projectedRefactorSurface(env, firstNonEmpty(capTaskInstruction(task), step.Description))
	return euclotypes.Artifact{
		ID:      fmt.Sprintf("refactor_step_%d_intent", index),
		Kind:    euclotypes.ArtifactKindEditIntent,
		Summary: step.Description,
		Payload: map[string]any{
			"summary":                     step.Description,
			"step_id":                     step.ID,
			"step_index":                  index,
			"step_total":                  total,
			"method":                      methodName,
			"compatibility_after_surface": afterSurface,
		},
		ProducerID: producerID,
		Status:     "proposed",
	}
}

func compatibilityArtifactForStep(env euclotypes.ExecutionEnvelope, proposed euclotypes.Artifact, step core.PlanStep, producerID string) euclotypes.Artifact {
	state := env.State
	if state == nil {
		state = core.NewContext()
	}
	gateState := state.Clone()
	mergeStateArtifactsToContext(gateState, []euclotypes.Artifact{proposed})
	gateEnv := env
	gateEnv.State = gateState
	payload := buildCompatibilityAssessmentPayload(gateEnv)
	payload["step_id"] = step.ID
	payload["step_description"] = step.Description
	payload["strict_gate"] = true
	return euclotypes.Artifact{ID: strings.ReplaceAll(step.ID, ".", "_") + "_compatibility", Kind: euclotypes.ArtifactKindCompatibilityAssessment, Summary: summarizePayload(payload), Payload: payload, ProducerID: producerID, Status: "produced"}
}

func compatibilityPassed(artifact euclotypes.Artifact) bool {
	payload, ok := artifact.Payload.(map[string]any)
	if !ok {
		return false
	}
	overall, _ := payload["overall_compatible"].(bool)
	return overall
}

func projectedRefactorSurface(env euclotypes.ExecutionEnvelope, instruction string) map[string]any {
	before := extractAPISurfaceWithEnv(env.Environment, instruction, reviewScopeFiles(env))
	after := normalizeSurface(before)
	if after == nil {
		after = map[string]any{"functions": []map[string]any{}, "types": []map[string]any{}}
	}
	oldName, newName := renameTargets(instruction)
	if oldName == "" {
		return after
	}
	if renamed, ok := renameSurfaceItem(surfaceItems(after["functions"]), oldName, newName); ok {
		after["functions"] = renamed
		return after
	}
	if renamed, ok := renameSurfaceItem(surfaceItems(after["types"]), oldName, newName); ok {
		after["types"] = renamed
	}
	return after
}

func renameTargets(instruction string) (string, string) {
	re := regexp.MustCompile(`(?i)renam(?:e|ing)(?:\s+(?:function|method|symbol|type))?\s+([A-Za-z_]\w*)(?:\s+to\s+([A-Za-z_]\w*))?`)
	match := re.FindStringSubmatch(instruction)
	if len(match) < 2 {
		return "", ""
	}
	return strings.TrimSpace(match[1]), strings.TrimSpace(match[2])
}

func renameSurfaceItem(items []map[string]any, oldName, newName string) ([]map[string]any, bool) {
	if oldName == "" {
		return items, false
	}
	out := make([]map[string]any, 0, len(items))
	found := false
	for _, item := range items {
		copied := map[string]any{}
		for key, value := range item {
			copied[key] = value
		}
		if stringValue(item["name"]) == oldName {
			found = true
			if newName != "" {
				copied["name"] = newName
				if sig := stringValue(copied["signature"]); sig != "" {
					copied["signature"] = strings.Replace(sig, oldName, newName, 1)
				}
			}
		}
		out = append(out, copied)
	}
	return out, found
}

func refactorStepContext(env euclotypes.ExecutionEnvelope, step core.PlanStep, index, total int, methodName string) map[string]any {
	ctx := taskContextFromEnvelope(env)
	ctx["refactor_step_id"] = step.ID
	ctx["refactor_step_index"] = index
	ctx["refactor_step_total"] = total
	ctx["refactor_method"] = methodName
	ctx["refactor_strict_api_compatibility"] = true
	return ctx
}

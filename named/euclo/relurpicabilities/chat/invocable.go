package chat

import (
	"context"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	"codeburg.org/lexbit/relurpify/named/euclo/execution"
	localbehavior "codeburg.org/lexbit/relurpify/named/euclo/relurpicabilities/local"
	eucloruntime "codeburg.org/lexbit/relurpify/named/euclo/runtime"
	euclostate "codeburg.org/lexbit/relurpify/named/euclo/runtime/state"
)

// Invocable implementations for chat behaviors.

// AskInvocable implements the Ask capability.
type AskInvocable struct{}

// NewAskInvocable creates a new Invocable for the ask capability.
func NewAskInvocable() execution.Invocable {
	return &AskInvocable{}
}

func (a *AskInvocable) ID() string { return Ask }

func (a *AskInvocable) Invoke(ctx context.Context, in execution.InvokeInput) (*core.Result, error) {
	execInput := execution.ExecuteInput{
		Task:             in.Task,
		ExecutionTask:    in.ExecutionTask,
		State:            in.State,
		Mode:             in.Mode,
		Profile:          in.Profile,
		Work:             in.Work,
		Environment:      in.Environment,
		ServiceBundle:    in.ServiceBundle,
		WorkflowExecutor: in.WorkflowExecutor,
		Telemetry:        in.Telemetry,
		InvokeSupporting: in.InvokeSupporting,
	}

	routines := execution.SupportingIDs(execInput.Work, "euclo:chat.")
	routineArtifacts, executed, err := execution.ExecuteSupportingRoutines(ctx, execInput, routines)
	if err != nil {
		return &core.Result{Success: false, Error: err}, err
	}
	execution.SetBehaviorTraceWithoutRecipeID(execInput.State, execInput.Work, executed)
	artifacts := append([]euclotypes.Artifact{}, routineArtifacts...)

	answerResult, _, err := execution.ExecuteRecipe(ctx, execInput, execution.RecipeChatAskInquiry, "chat-ask-answer",
		"Answer the user's question with concrete codebase-aware explanation: "+execution.CapabilityTaskInstruction(execInput.Task),
	)
	if err != nil || answerResult == nil || !answerResult.Success {
		execution.MergeStateArtifactsToContext(execInput.State, artifacts)
		return &core.Result{Success: false, Error: err}, err
	}
	answerPayload := map[string]any{
		"mode":            "chat.ask",
		"focus":           chatFocusLens(execInput.Task),
		"question_type":   askQuestionShape(execInput.Task),
		"summary":         execution.ResultSummary(answerResult),
		"response":        answerResult.Data,
		"used_reflection": false,
	}
	if execInput.State != nil {
		euclostate.SetPipelineAnalyze(execInput.State, answerPayload)
	}
	artifacts = append(artifacts, euclotypes.Artifact{
		ID:         "chat_ask_answer",
		Kind:       euclotypes.ArtifactKindAnalyze,
		Summary:    execution.ResultSummary(answerResult),
		Payload:    answerPayload,
		ProducerID: execInput.Work.PrimaryRelurpicCapabilityID,
		Status:     "produced",
	})

	if askNeedsOptionsPlanning(execInput.Task) {
		planResult, _, planErr := execution.ExecuteRecipe(ctx, execInput, execution.RecipeChatAskOptions, "chat-ask-options",
			"Compare plausible implementation or design options for: "+execution.CapabilityTaskInstruction(execInput.Task))
		if planErr == nil && planResult != nil && planResult.Success {
			planPayload := map[string]any{
				"request_shape": "options",
				"summary":       execution.ResultSummary(planResult),
				"candidates":    planResult.Data,
			}
			if execInput.State != nil {
				euclostate.SetPlanCandidates(execInput.State, planPayload)
			}
			artifacts = append(artifacts, euclotypes.Artifact{
				ID:         "chat_ask_plan_candidates",
				Kind:       euclotypes.ArtifactKindPlanCandidates,
				Summary:    execution.ResultSummary(planResult),
				Payload:    planPayload,
				ProducerID: execInput.Work.PrimaryRelurpicCapabilityID,
				Status:     "produced",
			})
		} else {
			artifacts = append(artifacts, euclotypes.Artifact{
				ID:         "chat_ask_plan_candidates",
				Kind:       euclotypes.ArtifactKindPlanCandidates,
				Summary:    "option comparison degraded",
				Payload:    map[string]any{"error": execution.ErrorMessage(planErr, planResult)},
				ProducerID: execInput.Work.PrimaryRelurpicCapabilityID,
				Status:     "degraded",
			})
		}
	}

	reviewResult, _, reviewErr := execution.ExecuteRecipe(ctx, execInput, execution.RecipeChatAskReview, "chat-ask-review",
		"Review the drafted answer for correctness, completeness, and directness.")
	if reviewErr == nil && reviewResult != nil && reviewResult.Success {
		reviewPayload := behaviorReviewPayload("chat.ask", execution.ResultSummary(reviewResult), reviewResult.Data)
		if execInput.State != nil {
			euclostate.SetReviewFindings(execInput.State, reviewPayload)
			if analyze, ok := euclostate.GetPipelineAnalyze(execInput.State); ok && len(analyze) > 0 {
				analyze["used_reflection"] = true
				analyze["review_summary"] = execution.ResultSummary(reviewResult)
				euclostate.SetPipelineAnalyze(execInput.State, analyze)
			}
		}
		artifacts = append(artifacts, euclotypes.Artifact{
			ID:         "chat_ask_review",
			Kind:       euclotypes.ArtifactKindReviewFindings,
			Summary:    execution.ResultSummary(reviewResult),
			Payload:    reviewPayload,
			ProducerID: execInput.Work.PrimaryRelurpicCapabilityID,
			Status:     "produced",
		})
	}

	writeBehaviorFinalOutput(execInput.State, "chat.ask", answerResult, execution.ResultSummary(reviewResult))
	execution.MergeStateArtifactsToContext(execInput.State, artifacts)
	return execution.SuccessResult("chat ask completed successfully", artifacts)
}

func (a *AskInvocable) IsPrimary() bool { return true }

// InspectInvocable implements the Inspect capability.
type InspectInvocable struct{}

// NewInspectInvocable creates a new Invocable for the inspect capability.
func NewInspectInvocable() execution.Invocable {
	return &InspectInvocable{}
}

func (i *InspectInvocable) ID() string { return Inspect }

func (i *InspectInvocable) Invoke(ctx context.Context, in execution.InvokeInput) (*core.Result, error) {
	execInput := execution.ExecuteInput{
		Task:             in.Task,
		ExecutionTask:    in.ExecutionTask,
		State:            in.State,
		Mode:             in.Mode,
		Profile:          in.Profile,
		Work:             in.Work,
		Environment:      in.Environment,
		ServiceBundle:    in.ServiceBundle,
		WorkflowExecutor: in.WorkflowExecutor,
		Telemetry:        in.Telemetry,
		InvokeSupporting: in.InvokeSupporting,
	}

	routines := execution.SupportingIDs(execInput.Work, "euclo:chat.")
	routineArtifacts, executed, err := execution.ExecuteSupportingRoutines(ctx, execInput, routines)
	if err != nil {
		return &core.Result{Success: false, Error: err}, err
	}
	execution.AppendDiagnostic(execInput.State, "euclo.review_findings", "inspect-first relurpic behavior executed")
	execution.SetBehaviorTraceWithoutRecipeID(execInput.State, execInput.Work, executed)
	artifacts := append([]euclotypes.Artifact{}, routineArtifacts...)

	inspectResult, _, err := execution.ExecuteRecipe(ctx, execInput, execution.RecipeChatInspectCollect, "chat-inspect-collect",
		"Inspect the requested code or system behavior and collect evidence: "+execution.CapabilityTaskInstruction(execInput.Task),
	)
	if err != nil || inspectResult == nil || !inspectResult.Success {
		execution.MergeStateArtifactsToContext(execInput.State, artifacts)
		return &core.Result{Success: false, Error: err}, err
	}
	inspectPayload := map[string]any{
		"mode":          "chat.inspect",
		"focus":         chatFocusLens(execInput.Task),
		"summary":       execution.ResultSummary(inspectResult),
		"inspection":    inspectResult.Data,
		"compatibility": inspectNeedsCompatibilityAssessment(execInput.Task),
	}
	if execInput.State != nil {
		euclostate.SetPipelineAnalyze(execInput.State, inspectPayload)
	}
	artifacts = append(artifacts, euclotypes.Artifact{
		ID:         "chat_inspect_analysis",
		Kind:       euclotypes.ArtifactKindAnalyze,
		Summary:    execution.ResultSummary(inspectResult),
		Payload:    inspectPayload,
		ProducerID: execInput.Work.PrimaryRelurpicCapabilityID,
		Status:     "produced",
	})

	semanticReview := localbehavior.NewReviewSemanticCapability(execInput.Environment).Execute(ctx, localExecutionEnvelope(execInput))
	if semanticReview.Status == euclotypes.ExecutionStatusCompleted && len(semanticReview.Artifacts) > 0 {
		execution.AddSpecializedCapabilityTrace(execInput.State, "euclo:review.semantic")
		artifacts = append(artifacts, semanticReview.Artifacts...)
		execution.MergeStateArtifactsToContext(execInput.State, semanticReview.Artifacts)
	} else {
		reviewResult, _, reviewErr := execution.ExecuteRecipe(ctx, execInput, execution.RecipeChatInspectReview, "chat-inspect-review",
			"Review the inspection findings, highlight risks, and summarize the most important evidence.")
		if reviewErr == nil && reviewResult != nil && reviewResult.Success {
			reviewPayload := behaviorReviewPayload("chat.inspect", execution.ResultSummary(reviewResult), reviewResult.Data)
			if execInput.State != nil {
				euclostate.SetReviewFindings(execInput.State, reviewPayload)
			}
			artifacts = append(artifacts, euclotypes.Artifact{
				ID:         "chat_inspect_review_reflection_fallback",
				Kind:       euclotypes.ArtifactKindReviewFindings,
				Summary:    execution.ResultSummary(reviewResult),
				Payload:    reviewPayload,
				ProducerID: execInput.Work.PrimaryRelurpicCapabilityID,
				Status:     "degraded",
			})
		}
	}

	if summary := executeInspectChain(ctx, execInput); strings.TrimSpace(summary) != "" {
		if execInput.State != nil {
			if existing, ok := euclostate.GetPipelineAnalyze(execInput.State); ok && len(existing) > 0 {
				existing["inspection_summary"] = summary
				euclostate.SetPipelineAnalyze(execInput.State, existing)
			}
		}
		artifacts = append(artifacts, euclotypes.Artifact{
			ID:         "chat_inspect_summary",
			Kind:       euclotypes.ArtifactKindAnalyze,
			Summary:    summary,
			Payload:    map[string]any{"summary": summary},
			ProducerID: execInput.Work.PrimaryRelurpicCapabilityID,
			Status:     "produced",
		})
	}

	if inspectNeedsCompatibilityAssessment(execInput.Task) {
		if raw, ok := euclostate.GetCompatibilityAssessment(execInput.State); ok && len(raw) > 0 {
			artifacts = append(artifacts, euclotypes.Artifact{
				ID:         "chat_inspect_compatibility",
				Kind:       euclotypes.ArtifactKindCompatibilityAssessment,
				Summary:    firstNonEmpty(execution.StringValue(mapSummary(raw)), "semantic compatibility assessment"),
				Payload:    raw,
				ProducerID: "euclo:review.semantic",
				Status:     "produced",
			})
			execution.MergeStateArtifactsToContext(execInput.State, artifacts)
			return execution.SuccessResult("chat inspect completed successfully", artifacts)
		}
		compatPayload := buildCompatibilityAssessment(execInput.Task, inspectResult, nil)
		if execInput.State != nil {
			if chainSummary, ok := euclostate.GetInspectCompatibilitySummary(execInput.State); ok && strings.TrimSpace(chainSummary) != "" {
				compatPayload["chainer_summary"] = chainSummary
			}
		}
		if execInput.State != nil {
			euclostate.SetCompatibilityAssessment(execInput.State, compatPayload)
		}
		artifacts = append(artifacts, euclotypes.Artifact{
			ID:         "chat_inspect_compatibility",
			Kind:       euclotypes.ArtifactKindCompatibilityAssessment,
			Summary:    execution.StringValue(compatPayload["summary"]),
			Payload:    compatPayload,
			ProducerID: execInput.Work.PrimaryRelurpicCapabilityID,
			Status:     "produced",
		})
	}

	writeBehaviorFinalOutput(execInput.State, "chat.inspect", inspectResult, execution.ResultSummary(semanticReviewResult(semanticReview)))
	execution.MergeStateArtifactsToContext(execInput.State, artifacts)
	return execution.SuccessResult("chat inspect completed successfully", artifacts)
}

func (i *InspectInvocable) IsPrimary() bool { return true }

// ImplementInvocable implements the Implement capability.
type ImplementInvocable struct{}

// NewImplementInvocable creates a new Invocable for the implement capability.
func NewImplementInvocable() execution.Invocable {
	return &ImplementInvocable{}
}

func (i *ImplementInvocable) ID() string { return Implement }

func (i *ImplementInvocable) Invoke(ctx context.Context, in execution.InvokeInput) (*core.Result, error) {
	execInput := execution.ExecuteInput{
		Task:             in.Task,
		ExecutionTask:    in.ExecutionTask,
		State:            in.State,
		Mode:             in.Mode,
		Profile:          in.Profile,
		Work:             in.Work,
		Environment:      in.Environment,
		ServiceBundle:    in.ServiceBundle,
		WorkflowExecutor: in.WorkflowExecutor,
		Telemetry:        in.Telemetry,
		InvokeSupporting: in.InvokeSupporting,
	}

	routines := append(execution.SupportingIDs(execInput.Work, "euclo:chat."), execution.SupportingIDs(execInput.Work, "euclo:archaeology.")...)
	routineArtifacts, executed, err := execution.ExecuteSupportingRoutines(ctx, execInput, routines)
	if err != nil {
		return &core.Result{Success: false, Error: err}, err
	}
	if execution.ContainsString(execInput.Work.SupportingRelurpicCapabilityIDs, "euclo:archaeology.explore") {
		execution.AppendDiagnostic(execInput.State, "euclo.plan_candidates", "lazy archaeology exploration support activated for chat.implement")
	}
	execution.SetBehaviorTraceWithoutRecipeID(execInput.State, execInput.Work, executed)
	artifacts := append([]euclotypes.Artifact{}, routineArtifacts...)

	if delegated, handled, execErr := executeSpecializedImplementBehavior(ctx, execInput, artifacts); handled {
		return delegated, execErr
	}

	if execution.ContainsString(execInput.Work.SupportingRelurpicCapabilityIDs, "euclo:archaeology.explore") {
		architectResult, _, architectErr := execution.ExecuteRecipe(ctx, execInput, execution.RecipeChatImplementArchitect, "chat-implement-architect",
			"Plan and execute this cross-cutting implementation as a staged architectural change: "+execution.CapabilityTaskInstruction(execInput.Task))
		if architectErr == nil && architectResult != nil && architectResult.Success {
			execution.AddSpecializedCapabilityTrace(execInput.State, "euclo.execution.architect")
			architectPayload := map[string]any{
				"mode":    "chat.implement",
				"source":  "architect",
				"summary": execution.ResultSummary(architectResult),
				"result":  architectResult.Data,
			}
			artifacts = append(artifacts,
				euclotypes.Artifact{
					ID:         "chat_implement_architect_plan",
					Kind:       euclotypes.ArtifactKindPlan,
					Summary:    execution.ResultSummary(architectResult),
					Payload:    architectPayload,
					ProducerID: execInput.Work.PrimaryRelurpicCapabilityID,
					Status:     "produced",
				},
				euclotypes.Artifact{
					ID:         "chat_implement_architect_status",
					Kind:       euclotypes.ArtifactKindExecutionStatus,
					Summary:    execution.ResultSummary(architectResult),
					Payload:    map[string]any{"source": "architect", "summary": execution.ResultSummary(architectResult)},
					ProducerID: execInput.Work.PrimaryRelurpicCapabilityID,
					Status:     "produced",
				},
			)
			execution.MergeStateArtifactsToContext(execInput.State, artifacts)
			return execution.SuccessResult("chat implement completed via architect", artifacts)
		}
		execution.AppendDiagnostic(execInput.State, "pipeline.code", "architect implement path degraded; falling back to standard implement flow")
	}

	exploreResult, _, err := execution.ExecuteRecipe(ctx, execInput, execution.RecipeChatImplementExplore, "chat-implement-explore",
		"Explore the codebase to understand the context for: "+execution.CapabilityTaskInstruction(execInput.Task),
	)
	if err != nil || exploreResult == nil || !exploreResult.Success {
		artifacts = append(artifacts, euclotypes.Artifact{
			ID:         "chat_implement_explore",
			Kind:       euclotypes.ArtifactKindExplore,
			Summary:    "exploration degraded; proceeding to implementation",
			Payload:    map[string]any{"error": execution.ErrorMessage(err, exploreResult)},
			ProducerID: execInput.Work.PrimaryRelurpicCapabilityID,
			Status:     "degraded",
		})
	} else {
		artifacts = append(artifacts, euclotypes.Artifact{
			ID:         "chat_implement_explore",
			Kind:       euclotypes.ArtifactKindExplore,
			Summary:    execution.ResultSummary(exploreResult),
			Payload:    exploreResult.Data,
			ProducerID: execInput.Work.PrimaryRelurpicCapabilityID,
			Status:     "produced",
		})
	}

	editResult, _, err := execution.ExecuteRecipe(ctx, execInput, execution.RecipeChatImplementEdit, "chat-implement-edit",
		"Plan and implement the changes for: "+execution.CapabilityTaskInstruction(execInput.Task),
	)
	if err != nil || editResult == nil || !editResult.Success {
		return &core.Result{Success: false, Error: err}, err
	}
	editIntentPayload := editResult.Data
	if _, hasEdits := editIntentPayload["edits"]; !hasEdits && execInput.State != nil {
		if existing, ok := euclostate.GetPipelineCode(execInput.State); ok && len(existing) > 0 {
			if _, existingEdits := existing["edits"]; existingEdits {
				editIntentPayload = existing
			}
		}
	}
	artifacts = append(artifacts,
		euclotypes.Artifact{
			ID:         "chat_implement_plan",
			Kind:       euclotypes.ArtifactKindPlan,
			Summary:    "plan generated during implement behavior",
			Payload:    editResult.Data,
			ProducerID: execInput.Work.PrimaryRelurpicCapabilityID,
			Status:     "produced",
		},
		euclotypes.Artifact{
			ID:         "chat_implement_edit_intent",
			Kind:       euclotypes.ArtifactKindEditIntent,
			Summary:    execution.ResultSummary(editResult),
			Payload:    editIntentPayload,
			ProducerID: execInput.Work.PrimaryRelurpicCapabilityID,
			Status:     "produced",
		},
	)
	if execInput.State != nil {
		euclostate.SetPipelineCode(execInput.State, editIntentPayload)
	}
	if verificationArtifacts, executed, execErr := localbehavior.ExecuteVerificationFlow(ctx, localExecutionEnvelope(execInput), eucloruntime.SnapshotCapabilities(execInput.Environment.Registry)); execErr != nil {
		return &core.Result{Success: false, Error: execErr}, execErr
	} else if executed {
		artifacts = append(artifacts, verificationArtifacts...)
		if rawVerify, ok := euclostate.GetPipelineVerify(execInput.State); ok && len(rawVerify) > 0 && localbehavior.VerificationPayloadFailed(rawVerify) {
			repairResult := localbehavior.NewFailedVerificationRepairCapability(execInput.Environment).Execute(ctx, localExecutionEnvelope(execInput))
			artifacts = append(artifacts, repairResult.Artifacts...)
			execution.MergeStateArtifactsToContext(execInput.State, artifacts)
			if repairResult.Status == euclotypes.ExecutionStatusFailed {
				err := fmt.Errorf("%s", firstNonEmpty(strings.TrimSpace(repairResult.Summary), "verification repair failed"))
				return &core.Result{Success: false, Error: err, Data: map[string]any{"artifacts": artifacts}}, err
			}
			return execution.SuccessResult(firstNonEmpty(strings.TrimSpace(repairResult.Summary), "chat implement repaired verification failure"), artifacts)
		}
		execution.MergeStateArtifactsToContext(execInput.State, artifacts)
		return execution.SuccessResult("chat implement completed successfully", artifacts)
	}

	verifyResult, _, err := execution.ExecuteRecipe(ctx, execInput, execution.RecipeChatImplementVerify, "chat-implement-verify",
		"Verify the changes by running tests and checking for issues.",
	)
	if err != nil || verifyResult == nil || !verifyResult.Success {
		return &core.Result{Success: false, Error: err}, err
	}
	if execInput.State == nil {
		return &core.Result{Success: false, Error: fmt.Errorf("verification state unavailable")}, fmt.Errorf("verification state unavailable")
	}
	verifyPayload, ok := euclostate.GetPipelineVerify(execInput.State)
	if !ok || len(verifyPayload) == 0 {
		return &core.Result{Success: false, Error: fmt.Errorf("verification recipe completed without structured verification evidence")}, fmt.Errorf("verification recipe completed without structured verification evidence")
	}
	if _, ok := verifyPayload["provenance"]; !ok {
		verifyPayload["provenance"] = "executed"
	}
	if _, ok := verifyPayload["run_id"]; !ok {
		verifyPayload["run_id"] = strings.TrimSpace(execInput.Work.RunID)
	}
	euclostate.SetPipelineVerify(execInput.State, verifyPayload)
	artifacts = append(artifacts, euclotypes.Artifact{
		ID:         "chat_implement_verification",
		Kind:       euclotypes.ArtifactKindVerification,
		Summary:    strings.TrimSpace(execution.StringValue(verifyPayload["summary"])),
		Payload:    verifyPayload,
		ProducerID: execInput.Work.PrimaryRelurpicCapabilityID,
		Status:     "produced",
	})
	if localbehavior.VerificationPayloadFailed(verifyPayload) {
		repairResult := localbehavior.NewFailedVerificationRepairCapability(execInput.Environment).Execute(ctx, localExecutionEnvelope(execInput))
		artifacts = append(artifacts, repairResult.Artifacts...)
		execution.MergeStateArtifactsToContext(execInput.State, artifacts)
		if repairResult.Status == euclotypes.ExecutionStatusFailed {
			err := fmt.Errorf("%s", firstNonEmpty(strings.TrimSpace(repairResult.Summary), "verification repair failed"))
			return &core.Result{Success: false, Error: err, Data: map[string]any{"artifacts": artifacts}}, err
		}
		return execution.SuccessResult(firstNonEmpty(strings.TrimSpace(repairResult.Summary), "chat implement repaired verification failure"), artifacts)
	}
	execution.MergeStateArtifactsToContext(execInput.State, artifacts)
	return execution.SuccessResult("chat implement completed successfully", artifacts)
}

func (i *ImplementInvocable) IsPrimary() bool { return true }

// NewSupportingInvocables returns all supporting invocables for the chat package.
func NewSupportingInvocables() []execution.Invocable {
	return []execution.Invocable{
		&directEditExecutionInvocable{},
		&localReviewInvocable{},
		&targetedVerificationInvocable{},
	}
}

// directEditExecutionInvocable wraps directEditExecutionRoutine as an Invocable.
type directEditExecutionInvocable struct{}

func (r *directEditExecutionInvocable) ID() string { return DirectEditExecution }

func (r *directEditExecutionInvocable) Invoke(ctx context.Context, in execution.InvokeInput) (*core.Result, error) {
	routine := directEditExecutionRoutine{}
	return routine.Invoke(ctx, in)
}

func (r *directEditExecutionInvocable) IsPrimary() bool { return false }

// localReviewInvocable wraps localReviewRoutine as an Invocable.
type localReviewInvocable struct{}

func (r *localReviewInvocable) ID() string { return LocalReview }

func (r *localReviewInvocable) Invoke(ctx context.Context, in execution.InvokeInput) (*core.Result, error) {
	routine := localReviewRoutine{}
	return routine.Invoke(ctx, in)
}

func (r *localReviewInvocable) IsPrimary() bool { return false }

// targetedVerificationInvocable wraps targetedVerificationRoutine as an Invocable.
type targetedVerificationInvocable struct{}

func (r *targetedVerificationInvocable) ID() string { return TargetedVerification }

func (r *targetedVerificationInvocable) Invoke(ctx context.Context, in execution.InvokeInput) (*core.Result, error) {
	routine := targetedVerificationRoutine{}
	return routine.Invoke(ctx, in)
}

func (r *targetedVerificationInvocable) IsPrimary() bool { return false }

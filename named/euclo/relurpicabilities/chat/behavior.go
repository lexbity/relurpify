package chat

import (
	"context"
	"fmt"
	"strings"

	chainerpkg "github.com/lexcodex/relurpify/agents/chainer"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/execution"
	chainexec "github.com/lexcodex/relurpify/named/euclo/execution/chainer"
	localbehavior "github.com/lexcodex/relurpify/named/euclo/relurpicabilities/local"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
	euclostate "github.com/lexcodex/relurpify/named/euclo/runtime/state"
)

type askBehavior struct{}
type inspectBehavior struct{}
type implementBehavior struct{}

func NewAskBehavior() execution.Behavior       { return askBehavior{} }
func NewInspectBehavior() execution.Behavior   { return inspectBehavior{} }
func NewImplementBehavior() execution.Behavior { return implementBehavior{} }

func (askBehavior) ID() string { return Ask }

func (askBehavior) Execute(ctx context.Context, in execution.ExecuteInput) (*core.Result, error) {
	routines := execution.SupportingIDs(in.Work, "euclo:chat.")
	routineArtifacts, executed, err := execution.ExecuteSupportingRoutines(ctx, in, routines)
	if err != nil {
		return &core.Result{Success: false, Error: err}, err
	}
	execution.SetBehaviorTraceWithoutRecipeID(in.State, in.Work, executed)
	artifacts := append([]euclotypes.Artifact{}, routineArtifacts...)

	answerResult, _, err := execution.ExecuteRecipe(ctx, in, execution.RecipeChatAskInquiry, "chat-ask-answer",
		"Answer the user's question with concrete codebase-aware explanation: "+execution.CapabilityTaskInstruction(in.Task),
	)
	if err != nil || answerResult == nil || !answerResult.Success {
		execution.MergeStateArtifactsToContext(in.State, artifacts)
		return &core.Result{Success: false, Error: err}, err
	}
	answerPayload := map[string]any{
		"mode":            "chat.ask",
		"focus":           chatFocusLens(in.Task),
		"question_type":   askQuestionShape(in.Task),
		"summary":         execution.ResultSummary(answerResult),
		"response":        answerResult.Data,
		"used_reflection": false,
	}
	if in.State != nil {
		in.State.Set("pipeline.analyze", answerPayload)
	}
	artifacts = append(artifacts, euclotypes.Artifact{
		ID:         "chat_ask_answer",
		Kind:       euclotypes.ArtifactKindAnalyze,
		Summary:    execution.ResultSummary(answerResult),
		Payload:    answerPayload,
		ProducerID: in.Work.PrimaryRelurpicCapabilityID,
		Status:     "produced",
	})

	if askNeedsOptionsPlanning(in.Task) {
		planResult, _, planErr := execution.ExecuteRecipe(ctx, in, execution.RecipeChatAskOptions, "chat-ask-options",
			"Compare plausible implementation or design options for: "+execution.CapabilityTaskInstruction(in.Task))
		if planErr == nil && planResult != nil && planResult.Success {
			planPayload := map[string]any{
				"request_shape": "options",
				"summary":       execution.ResultSummary(planResult),
				"candidates":    planResult.Data,
			}
			if in.State != nil {
				in.State.Set("euclo.plan_candidates", planPayload)
			}
			artifacts = append(artifacts, euclotypes.Artifact{
				ID:         "chat_ask_plan_candidates",
				Kind:       euclotypes.ArtifactKindPlanCandidates,
				Summary:    execution.ResultSummary(planResult),
				Payload:    planPayload,
				ProducerID: in.Work.PrimaryRelurpicCapabilityID,
				Status:     "produced",
			})
		} else {
			artifacts = append(artifacts, euclotypes.Artifact{
				ID:         "chat_ask_plan_candidates",
				Kind:       euclotypes.ArtifactKindPlanCandidates,
				Summary:    "option comparison degraded",
				Payload:    map[string]any{"error": execution.ErrorMessage(planErr, planResult)},
				ProducerID: in.Work.PrimaryRelurpicCapabilityID,
				Status:     "degraded",
			})
		}
	}

	reviewResult, _, reviewErr := execution.ExecuteRecipe(ctx, in, execution.RecipeChatAskReview, "chat-ask-review",
		"Review the drafted answer for correctness, completeness, and directness.")
	if reviewErr == nil && reviewResult != nil && reviewResult.Success {
		reviewPayload := behaviorReviewPayload("chat.ask", execution.ResultSummary(reviewResult), reviewResult.Data)
		if in.State != nil {
			in.State.Set("euclo.review_findings", reviewPayload)
			if analyze, ok := in.State.Get("pipeline.analyze"); ok && analyze != nil {
				if record, ok := analyze.(map[string]any); ok {
					record["used_reflection"] = true
					record["review_summary"] = execution.ResultSummary(reviewResult)
					in.State.Set("pipeline.analyze", record)
				}
			}
		}
		artifacts = append(artifacts, euclotypes.Artifact{
			ID:         "chat_ask_review",
			Kind:       euclotypes.ArtifactKindReviewFindings,
			Summary:    execution.ResultSummary(reviewResult),
			Payload:    reviewPayload,
			ProducerID: in.Work.PrimaryRelurpicCapabilityID,
			Status:     "produced",
		})
	}

	writeBehaviorFinalOutput(in.State, "chat.ask", answerResult, execution.ResultSummary(reviewResult))
	execution.MergeStateArtifactsToContext(in.State, artifacts)
	return execution.SuccessResult("chat ask completed successfully", artifacts)
}

func (inspectBehavior) ID() string { return Inspect }

func (inspectBehavior) Execute(ctx context.Context, in execution.ExecuteInput) (*core.Result, error) {
	routines := execution.SupportingIDs(in.Work, "euclo:chat.")
	routineArtifacts, executed, err := execution.ExecuteSupportingRoutines(ctx, in, routines)
	if err != nil {
		return &core.Result{Success: false, Error: err}, err
	}
	execution.AppendDiagnostic(in.State, "euclo.review_findings", "inspect-first relurpic behavior executed")
	execution.SetBehaviorTraceWithoutRecipeID(in.State, in.Work, executed)
	artifacts := append([]euclotypes.Artifact{}, routineArtifacts...)

	inspectResult, _, err := execution.ExecuteRecipe(ctx, in, execution.RecipeChatInspectCollect, "chat-inspect-collect",
		"Inspect the requested code or system behavior and collect evidence: "+execution.CapabilityTaskInstruction(in.Task),
	)
	if err != nil || inspectResult == nil || !inspectResult.Success {
		execution.MergeStateArtifactsToContext(in.State, artifacts)
		return &core.Result{Success: false, Error: err}, err
	}
	inspectPayload := map[string]any{
		"mode":          "chat.inspect",
		"focus":         chatFocusLens(in.Task),
		"summary":       execution.ResultSummary(inspectResult),
		"inspection":    inspectResult.Data,
		"compatibility": inspectNeedsCompatibilityAssessment(in.Task),
	}
	if in.State != nil {
		in.State.Set("pipeline.analyze", inspectPayload)
	}
	artifacts = append(artifacts, euclotypes.Artifact{
		ID:         "chat_inspect_analysis",
		Kind:       euclotypes.ArtifactKindAnalyze,
		Summary:    execution.ResultSummary(inspectResult),
		Payload:    inspectPayload,
		ProducerID: in.Work.PrimaryRelurpicCapabilityID,
		Status:     "produced",
	})

	semanticReview := localbehavior.NewReviewSemanticCapability(in.Environment).Execute(ctx, localExecutionEnvelope(in))
	if semanticReview.Status == euclotypes.ExecutionStatusCompleted && len(semanticReview.Artifacts) > 0 {
		execution.AddSpecializedCapabilityTrace(in.State, "euclo:review.semantic")
		artifacts = append(artifacts, semanticReview.Artifacts...)
		execution.MergeStateArtifactsToContext(in.State, semanticReview.Artifacts)
	} else {
		reviewResult, _, reviewErr := execution.ExecuteRecipe(ctx, in, execution.RecipeChatInspectReview, "chat-inspect-review",
			"Review the inspection findings, highlight risks, and summarize the most important evidence.")
		if reviewErr == nil && reviewResult != nil && reviewResult.Success {
			reviewPayload := behaviorReviewPayload("chat.inspect", execution.ResultSummary(reviewResult), reviewResult.Data)
			if in.State != nil {
				in.State.Set("euclo.review_findings", reviewPayload)
			}
			artifacts = append(artifacts, euclotypes.Artifact{
				ID:         "chat_inspect_review_reflection_fallback",
				Kind:       euclotypes.ArtifactKindReviewFindings,
				Summary:    execution.ResultSummary(reviewResult),
				Payload:    reviewPayload,
				ProducerID: in.Work.PrimaryRelurpicCapabilityID,
				Status:     "degraded",
			})
		}
	}

	if summary := executeInspectChain(ctx, in); strings.TrimSpace(summary) != "" {
		if in.State != nil {
			if existing, ok := in.State.Get("pipeline.analyze"); ok && existing != nil {
				if record, ok := existing.(map[string]any); ok {
					record["inspection_summary"] = summary
					in.State.Set("pipeline.analyze", record)
				}
			}
		}
		artifacts = append(artifacts, euclotypes.Artifact{
			ID:         "chat_inspect_summary",
			Kind:       euclotypes.ArtifactKindAnalyze,
			Summary:    summary,
			Payload:    map[string]any{"summary": summary},
			ProducerID: in.Work.PrimaryRelurpicCapabilityID,
			Status:     "produced",
		})
	}

	if inspectNeedsCompatibilityAssessment(in.Task) {
		if raw, ok := in.State.Get("euclo.compatibility_assessment"); ok && raw != nil {
			artifacts = append(artifacts, euclotypes.Artifact{
				ID:         "chat_inspect_compatibility",
				Kind:       euclotypes.ArtifactKindCompatibilityAssessment,
				Summary:    firstNonEmpty(execution.StringValue(mapSummary(raw)), "semantic compatibility assessment"),
				Payload:    raw,
				ProducerID: "euclo:review.semantic",
				Status:     "produced",
			})
			execution.MergeStateArtifactsToContext(in.State, artifacts)
			return execution.SuccessResult("chat inspect completed successfully", artifacts)
		}
		compatPayload := buildCompatibilityAssessment(in.Task, inspectResult, nil)
		if in.State != nil {
			if chainSummary, ok := in.State.Get("euclo.inspect_compatibility_summary"); ok && chainSummary != nil {
				compatPayload["chainer_summary"] = chainSummary
			}
		}
		if in.State != nil {
			in.State.Set("euclo.compatibility_assessment", compatPayload)
		}
		artifacts = append(artifacts, euclotypes.Artifact{
			ID:         "chat_inspect_compatibility",
			Kind:       euclotypes.ArtifactKindCompatibilityAssessment,
			Summary:    execution.StringValue(compatPayload["summary"]),
			Payload:    compatPayload,
			ProducerID: in.Work.PrimaryRelurpicCapabilityID,
			Status:     "produced",
		})
	}

	writeBehaviorFinalOutput(in.State, "chat.inspect", inspectResult, execution.ResultSummary(semanticReviewResult(semanticReview)))
	execution.MergeStateArtifactsToContext(in.State, artifacts)
	return execution.SuccessResult("chat inspect completed successfully", artifacts)
}

func writeBehaviorFinalOutput(state *core.Context, mode string, result *core.Result, reviewSummary string) {
	if state == nil {
		return
	}
	text := behaviorOutputText(result)
	payload := map[string]any{
		"mode":    strings.TrimSpace(mode),
		"summary": text,
		"text":    text,
	}
	if reviewSummary = strings.TrimSpace(reviewSummary); reviewSummary != "" {
		payload["review_summary"] = reviewSummary
	}
	state.Set("pipeline.final_output", payload)
}

func behaviorOutputText(result *core.Result) string {
	if result == nil {
		return ""
	}
	if result.Data != nil {
		if raw, ok := result.Data["final_output"]; ok {
			if text := outputText(raw); text != "" {
				return text
			}
		}
		for _, key := range []string{"text", "output", "summary"} {
			if text, ok := result.Data[key].(string); ok && strings.TrimSpace(text) != "" {
				return strings.TrimSpace(text)
			}
		}
	}
	if result.Error != nil {
		return strings.TrimSpace(result.Error.Error())
	}
	return ""
}

func outputText(raw any) string {
	switch typed := raw.(type) {
	case string:
		return strings.TrimSpace(typed)
	case map[string]any:
		for _, key := range []string{"text", "output", "summary"} {
			if text, ok := typed[key].(string); ok && strings.TrimSpace(text) != "" {
				return strings.TrimSpace(text)
			}
		}
	}
	return ""
}

func semanticReviewResult(result euclotypes.ExecutionResult) *core.Result {
	if result.Status != euclotypes.ExecutionStatusCompleted {
		return nil
	}
	return &core.Result{Success: true, Data: map[string]any{"summary": strings.TrimSpace(result.Summary)}}
}

func executeInspectChain(ctx context.Context, in execution.ExecuteInput) string {
	if in.State == nil {
		return ""
	}
	links := []chainerpkg.Link{
		chainerpkg.NewLink(
			"inspect_synthesis",
			`Synthesize the inspection analysis and review findings into a concise engineering summary.
Instruction: {{.Instruction}}
Inputs:
- pipeline.analyze: {{index .Input "pipeline.analyze"}}
- euclo.review_findings: {{index .Input "euclo.review_findings"}}

Return a short plain-text summary focused on the highest-signal findings, risks, and likely next step.`,
			[]string{"pipeline.analyze", "euclo.review_findings"},
			"euclo.inspect_summary",
			nil,
		),
	}
	if inspectNeedsCompatibilityAssessment(in.Task) {
		links = append(links, chainerpkg.NewLink(
			"inspect_compatibility_synthesis",
			`Summarize the compatibility implications of the inspected change or surface.
Instruction: {{.Instruction}}
Inputs:
- euclo.inspect_summary: {{index .Input "euclo.inspect_summary"}}

Return a short plain-text compatibility summary covering likely breakage risk, caller impact, and required verification focus.`,
			[]string{"euclo.inspect_summary"},
			"euclo.inspect_compatibility_summary",
			nil,
		))
	}
	task := core.CloneTask(in.Task)
	if task == nil {
		task = &core.Task{}
	}
	if task.Type == "" {
		task.Type = core.TaskTypeAnalysis
	}
	if _, err := chainexec.ExecuteChain(ctx, in.Environment, task, in.State, &chainerpkg.Chain{Links: links}); err != nil {
		execution.AppendDiagnostic(in.State, "euclo.review_findings", "inspect chainer synthesis degraded: "+err.Error())
		return ""
	}
	if raw, ok := in.State.Get("euclo.inspect_summary"); ok && raw != nil {
		if text, ok := raw.(string); ok {
			return strings.TrimSpace(text)
		}
	}
	return ""
}

func (implementBehavior) ID() string { return Implement }

func (implementBehavior) Execute(ctx context.Context, in execution.ExecuteInput) (*core.Result, error) {
	routines := append(execution.SupportingIDs(in.Work, "euclo:chat."), execution.SupportingIDs(in.Work, "euclo:archaeology.")...)
	routineArtifacts, executed, err := execution.ExecuteSupportingRoutines(ctx, in, routines)
	if err != nil {
		return &core.Result{Success: false, Error: err}, err
	}
	if execution.ContainsString(in.Work.SupportingRelurpicCapabilityIDs, "euclo:archaeology.explore") {
		execution.AppendDiagnostic(in.State, "euclo.plan_candidates", "lazy archaeology exploration support activated for chat.implement")
	}
	execution.SetBehaviorTraceWithoutRecipeID(in.State, in.Work, executed)
	artifacts := append([]euclotypes.Artifact{}, routineArtifacts...)

	if delegated, handled, execErr := executeSpecializedImplementBehavior(ctx, in, artifacts); handled {
		return delegated, execErr
	}

	if execution.ContainsString(in.Work.SupportingRelurpicCapabilityIDs, "euclo:archaeology.explore") {
		architectResult, _, architectErr := execution.ExecuteRecipe(ctx, in, execution.RecipeChatImplementArchitect, "chat-implement-architect",
			"Plan and execute this cross-cutting implementation as a staged architectural change: "+execution.CapabilityTaskInstruction(in.Task))
		if architectErr == nil && architectResult != nil && architectResult.Success {
			execution.AddSpecializedCapabilityTrace(in.State, "euclo.execution.architect")
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
					ProducerID: in.Work.PrimaryRelurpicCapabilityID,
					Status:     "produced",
				},
				euclotypes.Artifact{
					ID:         "chat_implement_architect_status",
					Kind:       euclotypes.ArtifactKindExecutionStatus,
					Summary:    execution.ResultSummary(architectResult),
					Payload:    map[string]any{"source": "architect", "summary": execution.ResultSummary(architectResult)},
					ProducerID: in.Work.PrimaryRelurpicCapabilityID,
					Status:     "produced",
				},
			)
			execution.MergeStateArtifactsToContext(in.State, artifacts)
			return execution.SuccessResult("chat implement completed via architect", artifacts)
		}
		execution.AppendDiagnostic(in.State, "pipeline.code", "architect implement path degraded; falling back to standard implement flow")
	}

	exploreResult, _, err := execution.ExecuteRecipe(ctx, in, execution.RecipeChatImplementExplore, "chat-implement-explore",
		"Explore the codebase to understand the context for: "+execution.CapabilityTaskInstruction(in.Task),
	)
	if err != nil || exploreResult == nil || !exploreResult.Success {
		artifacts = append(artifacts, euclotypes.Artifact{
			ID:         "chat_implement_explore",
			Kind:       euclotypes.ArtifactKindExplore,
			Summary:    "exploration degraded; proceeding to implementation",
			Payload:    map[string]any{"error": execution.ErrorMessage(err, exploreResult)},
			ProducerID: in.Work.PrimaryRelurpicCapabilityID,
			Status:     "degraded",
		})
	} else {
		artifacts = append(artifacts, euclotypes.Artifact{
			ID:         "chat_implement_explore",
			Kind:       euclotypes.ArtifactKindExplore,
			Summary:    execution.ResultSummary(exploreResult),
			Payload:    exploreResult.Data,
			ProducerID: in.Work.PrimaryRelurpicCapabilityID,
			Status:     "produced",
		})
	}

	editResult, _, err := execution.ExecuteRecipe(ctx, in, execution.RecipeChatImplementEdit, "chat-implement-edit",
		"Plan and implement the changes for: "+execution.CapabilityTaskInstruction(in.Task),
	)
	if err != nil || editResult == nil || !editResult.Success {
		return &core.Result{Success: false, Error: err}, err
	}
	editIntentPayload := editResult.Data
	if _, hasEdits := editIntentPayload["edits"]; !hasEdits && in.State != nil {
		if existing, ok := in.State.Get("pipeline.code"); ok && existing != nil {
			if typed, ok := existing.(map[string]any); ok {
				if _, existingEdits := typed["edits"]; existingEdits {
					editIntentPayload = typed
				}
			}
		}
	}
	artifacts = append(artifacts,
		euclotypes.Artifact{
			ID:         "chat_implement_plan",
			Kind:       euclotypes.ArtifactKindPlan,
			Summary:    "plan generated during implement behavior",
			Payload:    editResult.Data,
			ProducerID: in.Work.PrimaryRelurpicCapabilityID,
			Status:     "produced",
		},
		euclotypes.Artifact{
			ID:         "chat_implement_edit_intent",
			Kind:       euclotypes.ArtifactKindEditIntent,
			Summary:    execution.ResultSummary(editResult),
			Payload:    editIntentPayload,
			ProducerID: in.Work.PrimaryRelurpicCapabilityID,
			Status:     "produced",
		},
	)
	if in.State != nil {
		in.State.Set("pipeline.code", editIntentPayload)
	}
	if verificationArtifacts, executed, execErr := localbehavior.ExecuteVerificationFlow(ctx, localExecutionEnvelope(in), eucloruntime.SnapshotCapabilities(in.Environment.Registry)); execErr != nil {
		return &core.Result{Success: false, Error: execErr}, execErr
	} else if executed {
		artifacts = append(artifacts, verificationArtifacts...)
		if rawVerify, ok := in.State.Get("pipeline.verify"); ok && rawVerify != nil {
			if verifyPayload, ok := rawVerify.(map[string]any); ok && localbehavior.VerificationPayloadFailed(verifyPayload) {
				repairResult := localbehavior.NewFailedVerificationRepairCapability(in.Environment).Execute(ctx, localExecutionEnvelope(in))
				artifacts = append(artifacts, repairResult.Artifacts...)
				execution.MergeStateArtifactsToContext(in.State, artifacts)
				if repairResult.Status == euclotypes.ExecutionStatusFailed {
					err := fmt.Errorf("%s", firstNonEmpty(strings.TrimSpace(repairResult.Summary), "verification repair failed"))
					return &core.Result{Success: false, Error: err, Data: map[string]any{"artifacts": artifacts}}, err
				}
				return execution.SuccessResult(firstNonEmpty(strings.TrimSpace(repairResult.Summary), "chat implement repaired verification failure"), artifacts)
			}
		}
		execution.MergeStateArtifactsToContext(in.State, artifacts)
		return execution.SuccessResult("chat implement completed successfully", artifacts)
	}

	verifyResult, _, err := execution.ExecuteRecipe(ctx, in, execution.RecipeChatImplementVerify, "chat-implement-verify",
		"Verify the changes by running tests and checking for issues.",
	)
	if err != nil || verifyResult == nil || !verifyResult.Success {
		return &core.Result{Success: false, Error: err}, err
	}
	if in.State == nil {
		return &core.Result{Success: false, Error: fmt.Errorf("verification state unavailable")}, fmt.Errorf("verification state unavailable")
	}
	rawVerify, ok := in.State.Get("pipeline.verify")
	if !ok || rawVerify == nil {
		return &core.Result{Success: false, Error: fmt.Errorf("verification recipe completed without structured verification evidence")}, fmt.Errorf("verification recipe completed without structured verification evidence")
	}
	verifyPayload, ok := rawVerify.(map[string]any)
	if !ok {
		return &core.Result{Success: false, Error: fmt.Errorf("verification evidence was not structured")}, fmt.Errorf("verification evidence was not structured")
	}
	if _, ok := verifyPayload["provenance"]; !ok {
		verifyPayload["provenance"] = "executed"
	}
	if _, ok := verifyPayload["run_id"]; !ok {
		verifyPayload["run_id"] = strings.TrimSpace(in.Work.RunID)
	}
	in.State.Set("pipeline.verify", verifyPayload)
	artifacts = append(artifacts, euclotypes.Artifact{
		ID:         "chat_implement_verification",
		Kind:       euclotypes.ArtifactKindVerification,
		Summary:    strings.TrimSpace(execution.StringValue(verifyPayload["summary"])),
		Payload:    verifyPayload,
		ProducerID: in.Work.PrimaryRelurpicCapabilityID,
		Status:     "produced",
	})
	if localbehavior.VerificationPayloadFailed(verifyPayload) {
		repairResult := localbehavior.NewFailedVerificationRepairCapability(in.Environment).Execute(ctx, localExecutionEnvelope(in))
		artifacts = append(artifacts, repairResult.Artifacts...)
		execution.MergeStateArtifactsToContext(in.State, artifacts)
		if repairResult.Status == euclotypes.ExecutionStatusFailed {
			err := fmt.Errorf("%s", firstNonEmpty(strings.TrimSpace(repairResult.Summary), "verification repair failed"))
			return &core.Result{Success: false, Error: err, Data: map[string]any{"artifacts": artifacts}}, err
		}
		return execution.SuccessResult(firstNonEmpty(strings.TrimSpace(repairResult.Summary), "chat implement repaired verification failure"), artifacts)
	}
	execution.MergeStateArtifactsToContext(in.State, artifacts)
	return execution.SuccessResult("chat implement completed successfully", artifacts)
}

func executeSpecializedImplementBehavior(ctx context.Context, in execution.ExecuteInput, artifacts []euclotypes.Artifact) (*core.Result, bool, error) {
	envelope := localExecutionEnvelope(in)
	artifactState := euclotypes.NewArtifactState(append([]euclotypes.Artifact{}, euclotypes.ArtifactStateFromContext(in.State).All()...))
	snapshot := eucloruntime.SnapshotCapabilities(in.Environment.Registry)

	specialized := specializedImplementCapabilities(in)
	for _, capability := range specialized {
		if capability == nil {
			continue
		}
		if !specializedCapabilitySelectedForProfile(in.Profile.ProfileID, capability.Descriptor().ID) {
			continue
		}
		eligibility := capability.Eligible(artifactState, snapshot)
		if !eligibility.Eligible && !specializedCapabilityForcedForProfile(in.Profile.ProfileID, capability.Descriptor().ID) {
			continue
		}
		result := capability.Execute(ctx, envelope)
		execution.AddSpecializedCapabilityTrace(in.State, capability.Descriptor().ID)
		if trace, ok := euclostate.GetBehaviorTrace(in.State); ok {
			trace.RecipeIDs = nil
			executionTrace := execution.Trace{
				PrimaryCapabilityID:      trace.PrimaryCapabilityID,
				SupportingRoutines:       append([]string(nil), trace.SupportingRoutines...),
				RecipeIDs:                append([]string(nil), trace.RecipeIDs...),
				SpecializedCapabilityIDs: append([]string(nil), trace.SpecializedCapabilityIDs...),
				ExecutorFamily:           trace.ExecutorFamily,
				Path:                     trace.Path,
			}
			in.State.Set("euclo.relurpic_behavior_trace", executionTrace)
		}
		artifacts = append(artifacts, result.Artifacts...)
		appendSpecializedArtifactSummaries(ctx, in, envelope, &artifacts)
		execution.MergeStateArtifactsToContext(in.State, artifacts)
		if result.Status == euclotypes.ExecutionStatusFailed {
			err := fmt.Errorf("%s", strings.TrimSpace(result.Summary))
			if strings.TrimSpace(result.Summary) == "" {
				err = fmt.Errorf("specialized implement behavior failed")
			}
			return &core.Result{Success: false, Error: err, Data: map[string]any{
				"summary":   result.Summary,
				"artifacts": artifacts,
			}}, true, err
		}
		return &core.Result{Success: true, Data: map[string]any{
			"summary":   successSummary(result.Summary),
			"artifacts": artifacts,
		}}, true, nil
	}
	return nil, false, nil
}

func specializedImplementCapabilities(in execution.ExecuteInput) []euclotypes.EucloCodingCapability {
	return []euclotypes.EucloCodingCapability{
		localbehavior.NewTDDRedGreenRefactorCapability(in.Environment),
		localbehavior.NewMigrationExecuteCapability(in.Environment),
		localbehavior.NewRefactorAPICompatibleCapability(in.Environment),
		localbehavior.NewReviewImplementIfSafeCapability(in.Environment),
	}
}

func specializedCapabilitySelectedForProfile(profileID, capabilityID string) bool {
	switch strings.TrimSpace(profileID) {
	case "test_driven_generation":
		return capabilityID == "euclo:tdd.red_green_refactor"
	case "review_suggest_implement":
		return capabilityID == "euclo:review.implement_if_safe"
	default:
		return true
	}
}

func specializedCapabilityForcedForProfile(profileID, capabilityID string) bool {
	return strings.TrimSpace(profileID) == "test_driven_generation" && capabilityID == "euclo:tdd.red_green_refactor"
}

func appendSpecializedArtifactSummaries(ctx context.Context, in execution.ExecuteInput, env euclotypes.ExecutionEnvelope, artifacts *[]euclotypes.Artifact) {
	specialized := []euclotypes.EucloCodingCapability{
		localbehavior.NewDiffSummaryCapability(in.Environment),
		localbehavior.NewVerificationSummaryCapability(in.Environment),
	}
	for _, capability := range specialized {
		if capability == nil {
			continue
		}
		if !capability.Eligible(euclotypes.ArtifactStateFromContext(in.State), euclotypes.CapabilitySnapshot{}).Eligible {
			continue
		}
		result := capability.Execute(ctx, env)
		*artifacts = append(*artifacts, result.Artifacts...)
	}
}

func localExecutionEnvelope(in execution.ExecuteInput) euclotypes.ExecutionEnvelope {
	return euclotypes.ExecutionEnvelope{
		Task:        in.Task,
		Mode:        in.Mode,
		Profile:     in.Profile,
		Registry:    in.Environment.Registry,
		State:       in.State,
		Memory:      in.Environment.Memory,
		Environment: in.Environment,
		PlanStore:   in.ServiceBundle.PlanStore,
		Telemetry:   in.Telemetry,
		WorkflowID:  in.Work.WorkflowID,
		RunID:       in.Work.RunID,
	}
}

func successSummary(summary string) string {
	if strings.TrimSpace(summary) == "" {
		return "chat implement completed"
	}
	return strings.TrimSpace(summary)
}

func askQuestionShape(task *core.Task) string {
	if task == nil {
		return "general"
	}
	lower := strings.ToLower(strings.TrimSpace(task.Instruction))
	switch {
	case strings.Contains(lower, "compare"), strings.Contains(lower, "option"), strings.Contains(lower, "alternative"):
		return "comparison"
	case strings.Contains(lower, "why"), strings.Contains(lower, "explain"), strings.Contains(lower, "how does"):
		return "explanation"
	case strings.Contains(lower, "what"), strings.Contains(lower, "which"):
		return "question"
	default:
		return "general"
	}
}

func askNeedsOptionsPlanning(task *core.Task) bool {
	if task == nil {
		return false
	}
	lower := strings.ToLower(strings.TrimSpace(task.Instruction))
	for _, token := range []string{"compare", "alternatives", "options", "tradeoff", "which approach"} {
		if strings.Contains(lower, token) {
			return true
		}
	}
	return false
}

func inspectNeedsCompatibilityAssessment(task *core.Task) bool {
	if task == nil {
		return false
	}
	lower := strings.ToLower(strings.TrimSpace(task.Instruction))
	for _, token := range []string{"compatibility", "breaking change", "backward compatible", "api surface"} {
		if strings.Contains(lower, token) {
			return true
		}
	}
	return false
}

func buildCompatibilityAssessment(task *core.Task, inspectResult, reviewResult *core.Result) map[string]any {
	changes := []map[string]any{{
		"classification": "review_required",
		"risk":           "medium",
		"mitigation":     "validate exported and externally consumed behavior before changing surfaces",
	}}
	summary := "compatibility assessment flagged externally visible surface review"
	if inspectResult != nil && strings.TrimSpace(execution.ResultSummary(inspectResult)) != "" {
		summary = fmt.Sprintf("compatibility assessment based on inspection evidence: %s", execution.ResultSummary(inspectResult))
	}
	return map[string]any{
		"request":            execution.CapabilityTaskInstruction(task),
		"summary":            summary,
		"overall_compatible": false,
		"changes":            changes,
		"review_summary":     execution.ResultSummary(reviewResult),
	}
}

func mapSummary(raw any) string {
	record, _ := raw.(map[string]any)
	if record == nil {
		return ""
	}
	return execution.StringValue(record["summary"])
}

func behaviorReviewPayload(mode, summary string, reviewData any) map[string]any {
	payload := map[string]any{
		"mode":          mode,
		"review_source": mode + ".reflection",
		"summary":       summary,
		"review":        reviewData,
		"findings":      []map[string]any{},
	}
	if record, ok := reviewData.(map[string]any); ok {
		switch typed := record["findings"].(type) {
		case []map[string]any:
			payload["findings"] = typed
		case []any:
			normalized := make([]map[string]any, 0, len(typed))
			for _, item := range typed {
				if entry, ok := item.(map[string]any); ok {
					normalized = append(normalized, entry)
				}
			}
			if len(normalized) > 0 {
				payload["findings"] = normalized
			}
		}
	}
	if findings, _ := payload["findings"].([]map[string]any); len(findings) == 0 {
		payload["findings"] = []map[string]any{{
			"severity":         "info",
			"description":      firstNonEmpty(summary, "review completed"),
			"rationale":        "reflection review completed and summarized the response",
			"category":         "general",
			"confidence":       0.5,
			"impacted_files":   []string{},
			"impacted_symbols": []string{},
			"review_source":    mode + ".reflection",
			"traceability": map[string]any{
				"source": "reflection_review",
			},
		}}
	}
	return payload
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

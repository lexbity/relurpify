package debug

import (
	"context"
	"fmt"
	"strings"

	"github.com/lexcodex/relurpify/framework/core"
	frameworkpipeline "github.com/lexcodex/relurpify/framework/pipeline"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/execution"
	pipeexec "github.com/lexcodex/relurpify/named/euclo/execution/pipe"
	localbehavior "github.com/lexcodex/relurpify/named/euclo/relurpicabilities/local"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
	euclostate "github.com/lexcodex/relurpify/named/euclo/runtime/state"
)

type investigateBehavior struct{}

// Deprecated: Use NewInvestigateRepairInvocable instead
func NewInvestigateRepairBehavior() investigateBehavior { return investigateBehavior{} }

func (investigateBehavior) ID() string { return InvestigateRepair }

func (investigateBehavior) Execute(ctx context.Context, in execution.ExecuteInput) (*core.Result, error) {
	routines := append(execution.SupportingIDs(in.Work, "euclo:debug."), execution.SupportingIDs(in.Work, "euclo:chat.")...)
	routineArtifacts, executed, err := execution.ExecuteSupportingRoutines(ctx, in, routines)
	if err != nil {
		return &core.Result{Success: false, Error: err}, err
	}
	execution.AppendDiagnostic(in.State, "euclo.regression_analysis", "debug investigate behavior executed with explicit tool exposition facet")
	execution.SetBehaviorTrace(in.State, in.Work, executed)
	artifacts := append([]euclotypes.Artifact{}, routineArtifacts...)

	specializedArtifacts, specializedExecuted, err := executeSpecializedDebugBehaviors(ctx, in)
	if err != nil {
		execution.MergeStateArtifactsToContext(in.State, artifacts)
		return &core.Result{Success: false, Error: err}, err
	}
	if len(specializedArtifacts) > 0 {
		artifacts = append(artifacts, specializedArtifacts...)
		execution.MergeStateArtifactsToContext(in.State, specializedArtifacts)
	}
	if len(specializedExecuted) > 0 {
		execution.AppendDiagnostic(in.State, "euclo.regression_analysis",
			"debug investigate composed specialized relurpic capabilities: "+strings.Join(specializedExecuted, ", "))
	}

	reproduceResult, _, err := execution.ExecuteRecipe(ctx, in, execution.RecipeDebugInvestigateRepairReproduce, "debug-investigate-repair-reproduce",
		"Reproduce the issue by running tests or triggering the failure: "+execution.CapabilityTaskInstruction(in.Task),
	)
	if err != nil || reproduceResult == nil || !reproduceResult.Success {
		return &core.Result{Success: false, Error: err}, err
	}
	artifacts = append(artifacts, euclotypes.Artifact{
		ID:         "debug_investigate_reproduce",
		Kind:       euclotypes.ArtifactKindExplore,
		Summary:    execution.ResultSummary(reproduceResult),
		Payload:    reproduceResult.Data,
		ProducerID: in.Work.PrimaryRelurpicCapabilityID,
		Status:     "produced",
	})

	localizeResult, _, err := execution.ExecuteRecipe(ctx, in, execution.RecipeDebugInvestigateRepairLocalize, "debug-investigate-repair-localize",
		"Localize the root cause of the issue using reproduction evidence: "+execution.CapabilityTaskInstruction(in.Task),
	)
	if err != nil || localizeResult == nil || !localizeResult.Success {
		execution.MergeStateArtifactsToContext(in.State, artifacts)
		return &core.Result{Success: false, Error: err}, err
	}
	artifacts = append(artifacts, euclotypes.Artifact{
		ID:         "debug_investigate_localize",
		Kind:       euclotypes.ArtifactKindAnalyze,
		Summary:    execution.ResultSummary(localizeResult),
		Payload:    localizeResult.Data,
		ProducerID: in.Work.PrimaryRelurpicCapabilityID,
		Status:     "produced",
	})

	if !debugHasConcreteReproduction(in.State) && debugShouldSynthesizeReproducer(in.Task) {
		synthResult := localbehavior.NewRegressionSynthesizeCapability(in.Environment).Execute(ctx, debugExecutionEnvelope(in))
		if synthResult.Status == euclotypes.ExecutionStatusCompleted && len(synthResult.Artifacts) > 0 {
			artifacts = append(artifacts, synthResult.Artifacts...)
			execution.MergeStateArtifactsToContext(in.State, synthResult.Artifacts)
			execution.AppendDiagnostic(in.State, "euclo.regression_analysis", "debug investigate synthesized a regression reproducer before patching")
		}
	}

	patchResult, _, err := execution.ExecuteRecipe(ctx, in, execution.RecipeDebugInvestigateRepairPatch, "debug-investigate-repair-patch",
		"Generate a patch to fix the localized issue: "+execution.CapabilityTaskInstruction(in.Task),
	)
	if err != nil || patchResult == nil || !patchResult.Success {
		execution.MergeStateArtifactsToContext(in.State, artifacts)
		return &core.Result{Success: false, Error: err}, err
	}
	artifacts = append(artifacts, euclotypes.Artifact{
		ID:         "debug_investigate_edit_intent",
		Kind:       euclotypes.ArtifactKindEditIntent,
		Summary:    execution.ResultSummary(patchResult),
		Payload:    patchResult.Data,
		ProducerID: in.Work.PrimaryRelurpicCapabilityID,
		Status:     "produced",
	})

	reviewResult, _, reviewErr := execution.ExecuteRecipe(ctx, in, execution.RecipeDebugInvestigateRepairReview, "debug-investigate-repair-review",
		"Review the patch and verify it addresses the root cause.")
	if reviewErr == nil && reviewResult != nil && reviewResult.Success && in.State != nil {
		reviewPayload := debugReviewPayload(execution.ResultSummary(reviewResult), reviewResult.Data)
		euclostate.SetReviewFindings(in.State, reviewPayload)
		artifacts = append(artifacts, euclotypes.Artifact{
			ID:         "debug_investigate_review",
			Kind:       euclotypes.ArtifactKindReviewFindings,
			Summary:    execution.ResultSummary(reviewResult),
			Payload:    reviewPayload,
			ProducerID: in.Work.PrimaryRelurpicCapabilityID,
			Status:     "produced",
		})
		if existing, ok := euclostate.GetPipelineVerify(in.State); ok && len(existing) > 0 {
			if _, ok := existing["provenance"]; !ok {
				existing["provenance"] = "executed"
			}
			if _, ok := existing["run_id"]; !ok {
				existing["run_id"] = strings.TrimSpace(in.Work.RunID)
			}
			euclostate.SetPipelineVerify(in.State, existing)
			artifacts = append(artifacts, euclotypes.Artifact{
				ID:         "debug_investigate_verification",
				Kind:       euclotypes.ArtifactKindVerification,
				Summary:    strings.TrimSpace(fmt.Sprint(existing["summary"])),
				Payload:    existing,
				ProducerID: in.Work.PrimaryRelurpicCapabilityID,
				Status:     "produced",
			})
		}
	}
	if verificationArtifacts, executed, execErr := localbehavior.ExecuteVerificationFlow(ctx, debugExecutionEnvelope(in), eucloruntime.SnapshotCapabilities(in.Environment.Registry)); execErr != nil {
		execution.MergeStateArtifactsToContext(in.State, artifacts)
		return &core.Result{Success: false, Error: execErr}, execErr
	} else if executed {
		artifacts = append(artifacts, verificationArtifacts...)
		if rawVerify, ok := euclostate.GetPipelineVerify(in.State); ok && len(rawVerify) > 0 && localbehavior.VerificationPayloadFailed(rawVerify) {
			repairResult := localbehavior.NewFailedVerificationRepairCapability(in.Environment).Execute(ctx, debugExecutionEnvelope(in))
			artifacts = append(artifacts, repairResult.Artifacts...)
			execution.MergeStateArtifactsToContext(in.State, artifacts)
			if repairResult.Status == euclotypes.ExecutionStatusFailed {
				err := fmt.Errorf("%s", firstNonEmptyDebug(strings.TrimSpace(repairResult.Summary), "verification repair failed"))
				return &core.Result{Success: false, Error: err, Data: map[string]any{"artifacts": artifacts}}, err
			}
		}
	}

	if summaryArtifacts := executeDebugPipelinePostpass(ctx, in); len(summaryArtifacts) > 0 {
		execution.AddSpecializedCapabilityTrace(in.State, "euclo.execution.pipeline")
		artifacts = append(artifacts, summaryArtifacts...)
	}
	execution.MergeStateArtifactsToContext(in.State, artifacts)
	return execution.SuccessResult("debug investigate completed successfully", artifacts)
}

func executeSpecializedDebugBehaviors(ctx context.Context, in execution.ExecuteInput) ([]euclotypes.Artifact, []string, error) {
	envelope := debugExecutionEnvelope(in)
	snapshot := eucloruntime.SnapshotCapabilities(in.Environment.Registry)
	artifactState := euclotypes.ArtifactStateFromContext(in.State)
	specialized := []euclotypes.EucloCodingCapability{
		NewInvestigateRegressionCapability(in.Environment),
		localbehavior.NewTraceAnalyzeCapability(in.Environment),
		localbehavior.NewTraceToRootCauseCapability(in.Environment),
	}
	var artifacts []euclotypes.Artifact
	var executed []string
	for _, capability := range specialized {
		if capability == nil {
			continue
		}
		eligibility := capability.Eligible(artifactState, snapshot)
		if !eligibility.Eligible {
			continue
		}
		result := capability.Execute(ctx, envelope)
		execution.AddSpecializedCapabilityTrace(in.State, capability.Descriptor().ID)
		if result.Status == euclotypes.ExecutionStatusFailed {
			msg := strings.TrimSpace(result.Summary)
			if msg == "" && result.FailureInfo != nil {
				msg = strings.TrimSpace(result.FailureInfo.Message)
			}
			if msg == "" {
				msg = "specialized debug behavior failed"
			}
			return artifacts, executed, fmt.Errorf("%s", msg)
		}
		if len(result.Artifacts) > 0 {
			artifacts = append(artifacts, result.Artifacts...)
			execution.MergeStateArtifactsToContext(in.State, result.Artifacts)
			artifactState = euclotypes.ArtifactStateFromContext(in.State)
		}
		executed = append(executed, strings.TrimSpace(capability.Descriptor().ID))
	}
	return artifacts, execution.UniqueStrings(executed), nil
}

func debugExecutionEnvelope(in execution.ExecuteInput) euclotypes.ExecutionEnvelope {
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

func executeDebugPipelinePostpass(ctx context.Context, in execution.ExecuteInput) []euclotypes.Artifact {
	stages := []frameworkpipeline.Stage{
		&investigationSummaryStage{task: in.Task},
		&repairReadinessStage{task: in.Task},
	}
	task := core.CloneTask(in.Task)
	if task == nil {
		task = &core.Task{}
	}
	if task.Type == "" {
		task.Type = core.TaskTypeAnalysis
	}
	if _, err := pipeexec.ExecuteStages(ctx, in.Environment, task, in.State, stages); err != nil {
		execution.AppendDiagnostic(in.State, "euclo.regression_analysis", "debug pipeline postpass degraded: "+err.Error())
		return nil
	}
	var artifacts []euclotypes.Artifact
	if raw, ok := euclostate.GetDebugInvestigationSummary(in.State); ok && strings.TrimSpace(raw) != "" {
		artifacts = append(artifacts, euclotypes.Artifact{
			ID:         "debug_investigation_summary",
			Kind:       euclotypes.ArtifactKindAnalyze,
			Summary:    strings.TrimSpace(raw),
			Payload:    map[string]any{"summary": strings.TrimSpace(raw)},
			ProducerID: in.Work.PrimaryRelurpicCapabilityID,
			Status:     "produced",
		})
	}
	if raw, ok := euclostate.GetDebugRepairReadiness(in.State); ok && strings.TrimSpace(raw) != "" {
		artifacts = append(artifacts, euclotypes.Artifact{
			ID:         "debug_repair_readiness",
			Kind:       euclotypes.ArtifactKindReviewFindings,
			Summary:    strings.TrimSpace(raw),
			Payload:    debugReviewPayload(strings.TrimSpace(raw), map[string]any{"summary": strings.TrimSpace(raw)}),
			ProducerID: in.Work.PrimaryRelurpicCapabilityID,
			Status:     "produced",
		})
	}
	return artifacts
}

func debugReviewPayload(summary string, reviewData any) map[string]any {
	payload := map[string]any{
		"mode":          "debug.investigate",
		"review_source": "debug.investigate.review",
		"summary":       summary,
		"review":        reviewData,
		"findings": []map[string]any{{
			"severity":         "info",
			"description":      firstNonEmptyDebug(summary, "debug review completed"),
			"rationale":        "debug review summarized the patch and repair readiness",
			"category":         "correctness",
			"confidence":       0.5,
			"impacted_files":   []string{},
			"impacted_symbols": []string{},
			"review_source":    "debug.investigate.review",
			"traceability": map[string]any{
				"source": "reflection_review",
			},
		}},
	}
	return payload
}

func firstNonEmptyDebug(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func debugHasConcreteReproduction(state *core.Context) bool {
	if state == nil {
		return false
	}
	raw, ok := state.Get("euclo.reproduction")
	if !ok || raw == nil {
		return false
	}
	record, ok := raw.(map[string]any)
	if !ok {
		return false
	}
	if synthesized, ok := record["synthesized"].(bool); ok && synthesized {
		return false
	}
	return len(record) > 0
}

func debugShouldSynthesizeReproducer(task *core.Task) bool {
	text := strings.ToLower(strings.TrimSpace(execution.CapabilityTaskInstruction(task)))
	for _, token := range []string{
		"bug", "bugfix", "fix", "broken", "fails", "failing", "failure", "regression",
		"stopped working", "no longer", "error", "panic", "incorrect", "wrong", "issue",
	} {
		if strings.Contains(text, token) {
			return true
		}
	}
	return false
}

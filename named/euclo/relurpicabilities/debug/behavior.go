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
)

type investigateBehavior struct{}

func NewInvestigateBehavior() execution.Behavior { return investigateBehavior{} }

func (investigateBehavior) ID() string { return Investigate }

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

	reproduceResult, _, err := execution.ExecuteRecipe(ctx, in, execution.RecipeDebugInvestigateReproduce, "debug-investigate-reproduce",
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

	localizeResult, _, err := execution.ExecuteRecipe(ctx, in, execution.RecipeDebugInvestigateLocalize, "debug-investigate-localize",
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

	patchResult, _, err := execution.ExecuteRecipe(ctx, in, execution.RecipeDebugInvestigatePatch, "debug-investigate-patch",
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

	reviewResult, _, reviewErr := execution.ExecuteRecipe(ctx, in, execution.RecipeDebugInvestigateReview, "debug-investigate-review",
		"Review the patch and verify it addresses the root cause.")
	if reviewErr == nil && reviewResult != nil && reviewResult.Success {
		verifyPayload := map[string]any{
			"status":  "pass",
			"summary": execution.ResultSummary(reviewResult),
			"checks":  []any{map[string]any{"name": "reflection_review", "status": "pass"}},
		}
		if in.State != nil {
			in.State.Set("pipeline.verify", verifyPayload)
			in.State.Set("react.verification_latched_summary", execution.ResultSummary(reviewResult))
		}
		artifacts = append(artifacts, euclotypes.Artifact{
			ID:         "debug_investigate_verification",
			Kind:       euclotypes.ArtifactKindVerification,
			Summary:    execution.ResultSummary(reviewResult),
			Payload:    verifyPayload,
			ProducerID: in.Work.PrimaryRelurpicCapabilityID,
			Status:     "produced",
		})
	} else if in.State != nil {
		if existing, ok := in.State.Get("pipeline.verify"); ok && existing != nil {
			in.State.Set("react.verification_latched_summary", "reused existing verification evidence")
			artifacts = append(artifacts, euclotypes.Artifact{
				ID:         "debug_investigate_verification",
				Kind:       euclotypes.ArtifactKindVerification,
				Summary:    "reused existing verification evidence",
				Payload:    existing,
				ProducerID: in.Work.PrimaryRelurpicCapabilityID,
				Status:     "produced",
			})
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
	if raw, ok := in.State.Get("euclo.debug_investigation_summary"); ok && raw != nil {
		artifacts = append(artifacts, euclotypes.Artifact{
			ID:         "debug_investigation_summary",
			Kind:       euclotypes.ArtifactKindAnalyze,
			Summary:    strings.TrimSpace(fmt.Sprint(raw)),
			Payload:    map[string]any{"summary": strings.TrimSpace(fmt.Sprint(raw))},
			ProducerID: in.Work.PrimaryRelurpicCapabilityID,
			Status:     "produced",
		})
	}
	if raw, ok := in.State.Get("euclo.debug_repair_readiness"); ok && raw != nil {
		artifacts = append(artifacts, euclotypes.Artifact{
			ID:         "debug_repair_readiness",
			Kind:       euclotypes.ArtifactKindReviewFindings,
			Summary:    strings.TrimSpace(fmt.Sprint(raw)),
			Payload:    map[string]any{"summary": strings.TrimSpace(fmt.Sprint(raw))},
			ProducerID: in.Work.PrimaryRelurpicCapabilityID,
			Status:     "produced",
		})
	}
	return artifacts
}

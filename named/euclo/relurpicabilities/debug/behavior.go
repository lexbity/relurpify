package debug

import (
	"context"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/execution"
)

type investigateBehavior struct{}

func NewInvestigateBehavior() execution.Behavior { return investigateBehavior{} }

func (investigateBehavior) ID() string { return Investigate }

func (investigateBehavior) Execute(ctx context.Context, in execution.ExecuteInput) (*core.Result, error) {
	routines := append(execution.SupportingIDs(in.Work, "euclo:debug."), execution.SupportingIDs(in.Work, "euclo:chat.")...)
	for _, routine := range routines {
		execution.EnsureRoutineArtifacts(in.State, routine, in.Work)
	}
	execution.AppendDiagnostic(in.State, "euclo.regression_analysis", "debug investigate behavior executed with explicit tool exposition facet")
	execution.SetBehaviorTrace(in.State, in.Work, routines)
	var artifacts []euclotypes.Artifact

	reproduceResult, _, err := execution.ExecuteReactTask(ctx, in, "debug-investigate-reproduce",
		"Reproduce the issue by running tests or triggering the failure: "+execution.CapabilityTaskInstruction(in.Task),
		core.TaskTypeAnalysis,
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

	localizeResult, _, err := execution.ExecuteReactTask(ctx, in, "debug-investigate-localize",
		"Localize the root cause of the issue using reproduction evidence: "+execution.CapabilityTaskInstruction(in.Task),
		core.TaskTypeAnalysis,
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

	patchResult, _, err := execution.ExecuteReactTask(ctx, in, "debug-investigate-patch",
		"Generate a patch to fix the localized issue: "+execution.CapabilityTaskInstruction(in.Task),
		core.TaskTypeCodeModification,
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

	reviewResult, _, reviewErr := execution.ExecuteReflectionTask(ctx, in, "debug-investigate-review",
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
	execution.MergeStateArtifactsToContext(in.State, artifacts)
	return execution.SuccessResult("debug investigate completed successfully", artifacts)
}

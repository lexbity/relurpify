package debug

import (
	"context"
	"fmt"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	"codeburg.org/lexbit/relurpify/named/euclo/execution"
	localbehavior "codeburg.org/lexbit/relurpify/named/euclo/relurpicabilities/local"
	eucloruntime "codeburg.org/lexbit/relurpify/named/euclo/runtime"
	euclostate "codeburg.org/lexbit/relurpify/named/euclo/runtime/state"
)

type simpleRepairBehavior struct{}

// Deprecated: Use NewSimpleRepairInvocable instead
func NewSimpleRepairBehavior() simpleRepairBehavior { return simpleRepairBehavior{} }

func (simpleRepairBehavior) ID() string { return SimpleRepair }

func (simpleRepairBehavior) Execute(ctx context.Context, in execution.ExecuteInput) (*core.Result, error) {
	routines := execution.SupportingIDs(in.Work, "euclo:debug.")
	routineArtifacts, executed, err := execution.ExecuteSupportingRoutines(ctx, in, routines)
	if err != nil {
		return &core.Result{Success: false, Error: err}, err
	}
	execution.SetBehaviorTrace(in.State, in.Work, executed)
	artifacts := append([]euclotypes.Artifact{}, routineArtifacts...)

	readResult, _, err := execution.ExecuteRecipe(ctx, in, execution.RecipeDebugRepairSimpleRead, "debug-repair-simple-read",
		"Read and understand the defect context: "+execution.CapabilityTaskInstruction(in.Task),
	)
	if err != nil || readResult == nil || !readResult.Success {
		execution.MergeStateArtifactsToContext(in.State, artifacts)
		return &core.Result{Success: false, Error: err}, err
	}
	artifacts = append(artifacts, euclotypes.Artifact{
		ID:         "debug_repair_simple_read",
		Kind:       euclotypes.ArtifactKindAnalyze,
		Summary:    execution.ResultSummary(readResult),
		Payload:    readResult.Data,
		ProducerID: in.Work.PrimaryRelurpicCapabilityID,
		Status:     "produced",
	})

	editResult, _, err := execution.ExecuteRecipe(ctx, in, execution.RecipeDebugRepairSimpleEdit, "debug-repair-simple-edit",
		"Apply a minimal, correct patch to fix the defect: "+execution.CapabilityTaskInstruction(in.Task),
	)
	if err != nil || editResult == nil || !editResult.Success {
		execution.MergeStateArtifactsToContext(in.State, artifacts)
		return &core.Result{Success: false, Error: err}, err
	}
	artifacts = append(artifacts, euclotypes.Artifact{
		ID:         "debug_repair_simple_edit",
		Kind:       euclotypes.ArtifactKindEditIntent,
		Summary:    execution.ResultSummary(editResult),
		Payload:    editResult.Data,
		ProducerID: in.Work.PrimaryRelurpicCapabilityID,
		Status:     "produced",
	})

	envelope := simpleRepairExecutionEnvelope(in)
	verificationArtifacts, executedVerification, err := localbehavior.ExecuteVerificationFlow(ctx, envelope, eucloruntime.SnapshotCapabilities(in.Environment.Registry))
	if err != nil {
		execution.MergeStateArtifactsToContext(in.State, artifacts)
		return &core.Result{Success: false, Error: err}, err
	}
	if executedVerification {
		artifacts = append(artifacts, verificationArtifacts...)
		execution.MergeStateArtifactsToContext(in.State, verificationArtifacts)

		if verifyPayload, ok := euclostate.GetPipelineVerify(in.State); ok && len(verifyPayload) > 0 && localbehavior.VerificationPayloadFailed(verifyPayload) {
			repairResult := localbehavior.NewFailedVerificationRepairCapability(in.Environment).Execute(ctx, envelope)
			artifacts = append(artifacts, repairResult.Artifacts...)
			execution.MergeStateArtifactsToContext(in.State, artifacts)
			if repairResult.Status == euclotypes.ExecutionStatusFailed {
				err := fmt.Errorf("%s", execution.ErrorMessage(nil, &core.Result{Error: nil, Data: map[string]any{"summary": repairResult.Summary}}))
				return &core.Result{Success: false, Error: err, Data: map[string]any{"artifacts": artifacts}}, err
			}
			artifacts = append(artifacts, euclotypes.Artifact{
				ID:         "debug_repair_simple_recovery",
				Kind:       euclotypes.ArtifactKindEditIntent,
				Summary:    "verification-repair fallback applied",
				Payload:    map[string]any{"recovery": true, "source": "failed_verification_repair"},
				ProducerID: in.Work.PrimaryRelurpicCapabilityID,
				Status:     "produced",
			})
		} else if verifyPayload, ok := euclostate.GetPipelineVerify(in.State); ok && len(verifyPayload) > 0 {
			artifacts = append(artifacts, euclotypes.Artifact{
				ID:         "debug_repair_simple_verification",
				Kind:       euclotypes.ArtifactKindVerification,
				Summary:    "verification passed",
				Payload:    verifyPayload,
				ProducerID: in.Work.PrimaryRelurpicCapabilityID,
				Status:     "produced",
			})
		}
	}

	execution.MergeStateArtifactsToContext(in.State, artifacts)
	return execution.SuccessResult("simple repair completed successfully", artifacts)
}

func simpleRepairExecutionEnvelope(in execution.ExecuteInput) euclotypes.ExecutionEnvelope {
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

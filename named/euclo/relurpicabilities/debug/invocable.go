package debug

import (
	"context"
	"fmt"
	"strings"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/execution"
	euclorelurpic "github.com/lexcodex/relurpify/named/euclo/relurpicabilities"
	localbehavior "github.com/lexcodex/relurpify/named/euclo/relurpicabilities/local"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
	euclostate "github.com/lexcodex/relurpify/named/euclo/runtime/state"
)

// Invocable implementations for debug behaviors.

// InvestigateRepairInvocable implements the investigate-repair capability.
type InvestigateRepairInvocable struct{}

// NewInvestigateRepairInvocable creates a new Invocable for the investigate-repair capability.
func NewInvestigateRepairInvocable() execution.Invocable {
	return &InvestigateRepairInvocable{}
}

func (i *InvestigateRepairInvocable) ID() string { return InvestigateRepair }

func (i *InvestigateRepairInvocable) Invoke(ctx context.Context, in execution.InvokeInput) (*core.Result, error) {
	execInput := execution.ExecuteInput{
		Task:                 in.Task,
		ExecutionTask:        in.ExecutionTask,
		State:                in.State,
		Mode:                 in.Mode,
		Profile:              in.Profile,
		Work:                 in.Work,
		Environment:          in.Environment,
		ServiceBundle:        in.ServiceBundle,
		WorkflowExecutor:     in.WorkflowExecutor,
		Telemetry:            in.Telemetry,
		InvokeSupporting:     in.InvokeSupporting,
	}

	routines := append(execution.SupportingIDs(execInput.Work, "euclo:debug."), execution.SupportingIDs(execInput.Work, "euclo:chat.")...)
	routineArtifacts, executed, err := execution.ExecuteSupportingRoutines(ctx, execInput, routines)
	if err != nil {
		return &core.Result{Success: false, Error: err}, err
	}
	execution.AppendDiagnostic(execInput.State, "euclo.regression_analysis", "debug investigate behavior executed with explicit tool exposition facet")
	execution.SetBehaviorTrace(execInput.State, execInput.Work, executed)
	artifacts := append([]euclotypes.Artifact{}, routineArtifacts...)

	specializedArtifacts, specializedExecuted, err := executeSpecializedDebugBehaviors(ctx, execInput)
	if err != nil {
		execution.MergeStateArtifactsToContext(execInput.State, artifacts)
		return &core.Result{Success: false, Error: err}, err
	}
	if len(specializedArtifacts) > 0 {
		artifacts = append(artifacts, specializedArtifacts...)
		execution.MergeStateArtifactsToContext(execInput.State, specializedArtifacts)
	}
	if len(specializedExecuted) > 0 {
		execution.AppendDiagnostic(execInput.State, "euclo.regression_analysis",
			"debug investigate composed specialized relurpic capabilities: "+strings.Join(specializedExecuted, ", "))
	}

	reproduceResult, _, err := execution.ExecuteRecipe(ctx, execInput, execution.RecipeDebugInvestigateRepairReproduce, "debug-investigate-repair-reproduce",
		"Reproduce the issue by running tests or triggering the failure: "+execution.CapabilityTaskInstruction(execInput.Task),
	)
	if err != nil || reproduceResult == nil || !reproduceResult.Success {
		return &core.Result{Success: false, Error: err}, err
	}
	artifacts = append(artifacts, euclotypes.Artifact{
		ID:         "debug_investigate_reproduce",
		Kind:       euclotypes.ArtifactKindExplore,
		Summary:    execution.ResultSummary(reproduceResult),
		Payload:    reproduceResult.Data,
		ProducerID: execInput.Work.PrimaryRelurpicCapabilityID,
		Status:     "produced",
	})

	localizeResult, _, err := execution.ExecuteRecipe(ctx, execInput, execution.RecipeDebugInvestigateRepairLocalize, "debug-investigate-repair-localize",
		"Localize the root cause of the issue using reproduction evidence: "+execution.CapabilityTaskInstruction(execInput.Task),
	)
	if err != nil || localizeResult == nil || !localizeResult.Success {
		execution.MergeStateArtifactsToContext(execInput.State, artifacts)
		return &core.Result{Success: false, Error: err}, err
	}
	artifacts = append(artifacts, euclotypes.Artifact{
		ID:         "debug_investigate_localize",
		Kind:       euclotypes.ArtifactKindAnalyze,
		Summary:    execution.ResultSummary(localizeResult),
		Payload:    localizeResult.Data,
		ProducerID: execInput.Work.PrimaryRelurpicCapabilityID,
		Status:     "produced",
	})

	if !debugHasConcreteReproduction(execInput.State) && debugShouldSynthesizeReproducer(execInput.Task) {
		synthResult := localbehavior.NewRegressionSynthesizeCapability(execInput.Environment).Execute(ctx, debugExecutionEnvelope(execInput))
		if synthResult.Status == euclotypes.ExecutionStatusCompleted && len(synthResult.Artifacts) > 0 {
			artifacts = append(artifacts, synthResult.Artifacts...)
			execution.MergeStateArtifactsToContext(execInput.State, synthResult.Artifacts)
			execution.AppendDiagnostic(execInput.State, "euclo.regression_analysis", "debug investigate synthesized a regression reproducer before patching")
		}
	}

	patchResult, _, err := execution.ExecuteRecipe(ctx, execInput, execution.RecipeDebugInvestigateRepairPatch, "debug-investigate-repair-patch",
		"Generate a patch to fix the localized issue: "+execution.CapabilityTaskInstruction(execInput.Task),
	)
	if err != nil || patchResult == nil || !patchResult.Success {
		execution.MergeStateArtifactsToContext(execInput.State, artifacts)
		return &core.Result{Success: false, Error: err}, err
	}
	artifacts = append(artifacts, euclotypes.Artifact{
		ID:         "debug_investigate_edit_intent",
		Kind:       euclotypes.ArtifactKindEditIntent,
		Summary:    execution.ResultSummary(patchResult),
		Payload:    patchResult.Data,
		ProducerID: execInput.Work.PrimaryRelurpicCapabilityID,
		Status:     "produced",
	})

	reviewResult, _, reviewErr := execution.ExecuteRecipe(ctx, execInput, execution.RecipeDebugInvestigateRepairReview, "debug-investigate-repair-review",
		"Review the patch and verify it addresses the root cause.")
	if reviewErr == nil && reviewResult != nil && reviewResult.Success && execInput.State != nil {
		reviewPayload := debugReviewPayload(execution.ResultSummary(reviewResult), reviewResult.Data)
		euclostate.SetReviewFindings(execInput.State, reviewPayload)
		artifacts = append(artifacts, euclotypes.Artifact{
			ID:         "debug_investigate_review",
			Kind:       euclotypes.ArtifactKindReviewFindings,
			Summary:    execution.ResultSummary(reviewResult),
			Payload:    reviewPayload,
			ProducerID: execInput.Work.PrimaryRelurpicCapabilityID,
			Status:     "produced",
		})
		if existing, ok := euclostate.GetPipelineVerify(execInput.State); ok && len(existing) > 0 {
			if _, ok := existing["provenance"]; !ok {
				existing["provenance"] = "executed"
			}
			if _, ok := existing["run_id"]; !ok {
				existing["run_id"] = strings.TrimSpace(execInput.Work.RunID)
			}
			euclostate.SetPipelineVerify(execInput.State, existing)
			artifacts = append(artifacts, euclotypes.Artifact{
				ID:         "debug_investigate_verification",
				Kind:       euclotypes.ArtifactKindVerification,
				Summary:    strings.TrimSpace(fmt.Sprint(existing["summary"])),
				Payload:    existing,
				ProducerID: execInput.Work.PrimaryRelurpicCapabilityID,
				Status:     "produced",
			})
		}
	}
	if verificationArtifacts, executed, execErr := localbehavior.ExecuteVerificationFlow(ctx, debugExecutionEnvelope(execInput), eucloruntime.SnapshotCapabilities(execInput.Environment.Registry)); execErr != nil {
		execution.MergeStateArtifactsToContext(execInput.State, artifacts)
		return &core.Result{Success: false, Error: execErr}, execErr
	} else if executed {
		artifacts = append(artifacts, verificationArtifacts...)
		if rawVerify, ok := euclostate.GetPipelineVerify(execInput.State); ok && len(rawVerify) > 0 && localbehavior.VerificationPayloadFailed(rawVerify) {
			repairResult := localbehavior.NewFailedVerificationRepairCapability(execInput.Environment).Execute(ctx, debugExecutionEnvelope(execInput))
			artifacts = append(artifacts, repairResult.Artifacts...)
			execution.MergeStateArtifactsToContext(execInput.State, artifacts)
			if repairResult.Status == euclotypes.ExecutionStatusFailed {
				err := fmt.Errorf("%s", firstNonEmptyDebug(strings.TrimSpace(repairResult.Summary), "verification repair failed"))
				return &core.Result{Success: false, Error: err, Data: map[string]any{"artifacts": artifacts}}, err
			}
		}
	}

	if summaryArtifacts := executeDebugPipelinePostpass(ctx, execInput); len(summaryArtifacts) > 0 {
		execution.AddSpecializedCapabilityTrace(execInput.State, "euclo.execution.pipeline")
		artifacts = append(artifacts, summaryArtifacts...)
	}
	execution.MergeStateArtifactsToContext(execInput.State, artifacts)
	return execution.SuccessResult("debug investigate completed successfully", artifacts)
}

func (i *InvestigateRepairInvocable) IsPrimary() bool { return true }

// SimpleRepairInvocable implements the simple-repair capability.
type SimpleRepairInvocable struct{}

// NewSimpleRepairInvocable creates a new Invocable for the simple-repair capability.
func NewSimpleRepairInvocable() execution.Invocable {
	return &SimpleRepairInvocable{}
}

func (s *SimpleRepairInvocable) ID() string { return SimpleRepair }

func (s *SimpleRepairInvocable) Invoke(ctx context.Context, in execution.InvokeInput) (*core.Result, error) {
	execInput := execution.ExecuteInput{
		Task:                 in.Task,
		ExecutionTask:        in.ExecutionTask,
		State:                in.State,
		Mode:                 in.Mode,
		Profile:              in.Profile,
		Work:                 in.Work,
		Environment:          in.Environment,
		ServiceBundle:        in.ServiceBundle,
		WorkflowExecutor:     in.WorkflowExecutor,
		Telemetry:            in.Telemetry,
		InvokeSupporting:     in.InvokeSupporting,
	}

	routines := execution.SupportingIDs(execInput.Work, "euclo:debug.")
	routineArtifacts, executed, err := execution.ExecuteSupportingRoutines(ctx, execInput, routines)
	if err != nil {
		return &core.Result{Success: false, Error: err}, err
	}
	execution.SetBehaviorTrace(execInput.State, execInput.Work, executed)
	artifacts := append([]euclotypes.Artifact{}, routineArtifacts...)

	readResult, _, err := execution.ExecuteRecipe(ctx, execInput, execution.RecipeDebugRepairSimpleRead, "debug-repair-simple-read",
		"Read and understand the defect context: "+execution.CapabilityTaskInstruction(execInput.Task),
	)
	if err != nil || readResult == nil || !readResult.Success {
		execution.MergeStateArtifactsToContext(execInput.State, artifacts)
		return &core.Result{Success: false, Error: err}, err
	}
	artifacts = append(artifacts, euclotypes.Artifact{
		ID:         "debug_repair_simple_read",
		Kind:       euclotypes.ArtifactKindAnalyze,
		Summary:    execution.ResultSummary(readResult),
		Payload:    readResult.Data,
		ProducerID: execInput.Work.PrimaryRelurpicCapabilityID,
		Status:     "produced",
	})

	editResult, _, err := execution.ExecuteRecipe(ctx, execInput, execution.RecipeDebugRepairSimpleEdit, "debug-repair-simple-edit",
		"Apply a minimal, correct patch to fix the defect: "+execution.CapabilityTaskInstruction(execInput.Task),
	)
	if err != nil || editResult == nil || !editResult.Success {
		execution.MergeStateArtifactsToContext(execInput.State, artifacts)
		return &core.Result{Success: false, Error: err}, err
	}
	artifacts = append(artifacts, euclotypes.Artifact{
		ID:         "debug_repair_simple_edit",
		Kind:       euclotypes.ArtifactKindEditIntent,
		Summary:    execution.ResultSummary(editResult),
		Payload:    editResult.Data,
		ProducerID: execInput.Work.PrimaryRelurpicCapabilityID,
		Status:     "produced",
	})

	envelope := simpleRepairExecutionEnvelope(execInput)
	verificationArtifacts, executedVerification, err := localbehavior.ExecuteVerificationFlow(ctx, envelope, eucloruntime.SnapshotCapabilities(execInput.Environment.Registry))
	if err != nil {
		execution.MergeStateArtifactsToContext(execInput.State, artifacts)
		return &core.Result{Success: false, Error: err}, err
	}
	if executedVerification {
		artifacts = append(artifacts, verificationArtifacts...)
		execution.MergeStateArtifactsToContext(execInput.State, verificationArtifacts)

		if verifyPayload, ok := euclostate.GetPipelineVerify(execInput.State); ok && len(verifyPayload) > 0 && localbehavior.VerificationPayloadFailed(verifyPayload) {
			repairResult := localbehavior.NewFailedVerificationRepairCapability(execInput.Environment).Execute(ctx, envelope)
			artifacts = append(artifacts, repairResult.Artifacts...)
			execution.MergeStateArtifactsToContext(execInput.State, artifacts)
			if repairResult.Status == euclotypes.ExecutionStatusFailed {
				err := fmt.Errorf("%s", execution.ErrorMessage(nil, &core.Result{Error: nil, Data: map[string]any{"summary": repairResult.Summary}}))
				return &core.Result{Success: false, Error: err, Data: map[string]any{"artifacts": artifacts}}, err
			}
			artifacts = append(artifacts, euclotypes.Artifact{
				ID:         "debug_repair_simple_recovery",
				Kind:       euclotypes.ArtifactKindEditIntent,
				Summary:    "verification-repair fallback applied",
				Payload:    map[string]any{"recovery": true, "source": "failed_verification_repair"},
				ProducerID: execInput.Work.PrimaryRelurpicCapabilityID,
				Status:     "produced",
			})
		} else if verifyPayload, ok := euclostate.GetPipelineVerify(execInput.State); ok && len(verifyPayload) > 0 {
			artifacts = append(artifacts, euclotypes.Artifact{
				ID:         "debug_repair_simple_verification",
				Kind:       euclotypes.ArtifactKindVerification,
				Summary:    "verification passed",
				Payload:    verifyPayload,
				ProducerID: execInput.Work.PrimaryRelurpicCapabilityID,
				Status:     "produced",
			})
		}
	}

	execution.MergeStateArtifactsToContext(execInput.State, artifacts)
	return execution.SuccessResult("simple repair completed successfully", artifacts)
}

func (s *SimpleRepairInvocable) IsPrimary() bool { return true }

// NewSupportingInvocables returns all supporting invocables for the debug package.
func NewSupportingInvocables() []execution.Invocable {
	return []execution.Invocable{
		&rootCauseInvocable{},
		&hypothesisRefineInvocable{},
		&localizationInvocable{},
		&flawSurfaceInvocable{},
		&verificationRepairInvocable{},
	}
}

// rootCauseInvocable wraps rootCauseRoutine as an Invocable.
type rootCauseInvocable struct{}

func (r *rootCauseInvocable) ID() string { return RootCause }

func (r *rootCauseInvocable) Invoke(ctx context.Context, in execution.InvokeInput) (*core.Result, error) {
	routine := rootCauseRoutine{}
	artifacts, err := routine.Execute(ctx, convertInvokeInputToRoutineInput(in))
	if err != nil {
		return nil, err
	}
	return &core.Result{
		Success: true,
		Data:    map[string]any{"artifacts": artifacts},
	}, nil
}

func (r *rootCauseInvocable) IsPrimary() bool { return false }

// hypothesisRefineInvocable wraps hypothesisRefineRoutine as an Invocable.
type hypothesisRefineInvocable struct{}

func (h *hypothesisRefineInvocable) ID() string { return HypothesisRefine }

func (h *hypothesisRefineInvocable) Invoke(ctx context.Context, in execution.InvokeInput) (*core.Result, error) {
	routine := hypothesisRefineRoutine{}
	artifacts, err := routine.Execute(ctx, convertInvokeInputToRoutineInput(in))
	if err != nil {
		return nil, err
	}
	return &core.Result{
		Success: true,
		Data:    map[string]any{"artifacts": artifacts},
	}, nil
}

func (h *hypothesisRefineInvocable) IsPrimary() bool { return false }

// localizationInvocable wraps localizationRoutine as an Invocable.
type localizationInvocable struct{}

func (l *localizationInvocable) ID() string { return Localization }

func (l *localizationInvocable) Invoke(ctx context.Context, in execution.InvokeInput) (*core.Result, error) {
	routine := localizationRoutine{}
	artifacts, err := routine.Execute(ctx, convertInvokeInputToRoutineInput(in))
	if err != nil {
		return nil, err
	}
	return &core.Result{
		Success: true,
		Data:    map[string]any{"artifacts": artifacts},
	}, nil
}

func (l *localizationInvocable) IsPrimary() bool { return false }

// flawSurfaceInvocable wraps flawSurfaceRoutine as an Invocable.
type flawSurfaceInvocable struct{}

func (f *flawSurfaceInvocable) ID() string { return FlawSurface }

func (f *flawSurfaceInvocable) Invoke(ctx context.Context, in execution.InvokeInput) (*core.Result, error) {
	routine := flawSurfaceRoutine{}
	artifacts, err := routine.Execute(ctx, convertInvokeInputToRoutineInput(in))
	if err != nil {
		return nil, err
	}
	return &core.Result{
		Success: true,
		Data:    map[string]any{"artifacts": artifacts},
	}, nil
}

func (f *flawSurfaceInvocable) IsPrimary() bool { return false }

// verificationRepairInvocable wraps verificationRepairRoutine as an Invocable.
type verificationRepairInvocable struct{}

func (v *verificationRepairInvocable) ID() string { return VerificationRepair }

func (v *verificationRepairInvocable) Invoke(ctx context.Context, in execution.InvokeInput) (*core.Result, error) {
	routine := verificationRepairRoutine{}
	artifacts, err := routine.Execute(ctx, convertInvokeInputToRoutineInput(in))
	if err != nil {
		return nil, err
	}
	return &core.Result{
		Success: true,
		Data:    map[string]any{"artifacts": artifacts},
	}, nil
}

func (v *verificationRepairInvocable) IsPrimary() bool { return false }

func convertInvokeInputToRoutineInput(in execution.InvokeInput) euclorelurpic.RoutineInput {
	return euclorelurpic.RoutineInput{
		Task:  in.Task,
		State: in.State,
		Work: euclorelurpic.WorkContext{
			PrimaryCapabilityID:             in.Work.PrimaryRelurpicCapabilityID,
			SupportingRelurpicCapabilityIDs: append([]string(nil), in.Work.SupportingRelurpicCapabilityIDs...),
			PatternRefs:                     append([]string(nil), in.Work.SemanticInputs.PatternRefs...),
			TensionRefs:                     append([]string(nil), in.Work.SemanticInputs.TensionRefs...),
			ProspectiveRefs:                 append([]string(nil), in.Work.SemanticInputs.ProspectiveRefs...),
			ConvergenceRefs:                 append([]string(nil), in.Work.SemanticInputs.ConvergenceRefs...),
			RequestProvenanceRefs:           append([]string(nil), in.Work.SemanticInputs.RequestProvenanceRefs...),
		},
		Environment:   in.Environment,
		ServiceBundle: in.ServiceBundle,
	}
}

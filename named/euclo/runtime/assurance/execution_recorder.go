package assurance

import (
	"context"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeoexec "github.com/lexcodex/relurpify/archaeo/execution"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
	eucloreporting "github.com/lexcodex/relurpify/named/euclo/runtime/reporting"
	euclostate "github.com/lexcodex/relurpify/named/euclo/runtime/state"
)

// ExecutionRecorder handles artifact collection, action log assembly,
// proof surface assembly, artifact persistence, final report assembly,
// and telemetry emission.
type ExecutionRecorder struct {
	PersistArtifacts ArtifactPersister
	Telemetry        core.Telemetry
}

// RecordResult is the result of execution recording.
type RecordResult struct {
	Artifacts           []euclotypes.Artifact
	ActionLog           []eucloruntime.ActionLogEntry
	ProofSurface        eucloruntime.ProofSurface
	FinalReport         map[string]any
	MutationCheckpoints []archaeodomain.MutationCheckpointSummary
	Err                 error
}

// Record collects artifacts, builds action log and proof surface,
// persists artifacts, assembles final report, and emits telemetry.
// This corresponds to the artifact/recording half of the old applyVerificationAndArtifacts
// and the identical tail of ShortCircuit.
func (r ExecutionRecorder) Record(ctx context.Context, task *core.Task, state *core.Context,
	gateResult GateResult, result *core.Result) RecordResult {

	// Collect artifacts from state
	artifacts := euclotypes.CollectArtifactsFromState(state)

	// Build action log
	actionLog := eucloreporting.BuildActionLog(state, artifacts)
	euclostate.SetActionLog(state, actionLog)

	// Build proof surface
	proofSurface := eucloreporting.BuildProofSurface(state, artifacts)
	euclostate.SetProofSurface(state, proofSurface)

	// Re-collect artifacts after state updates
	artifacts = euclotypes.CollectArtifactsFromState(state)
	euclostate.SetArtifacts(state, artifacts)

	// Persist artifacts if persister is configured
	var err error
	if r.PersistArtifacts != nil {
		if persistErr := r.PersistArtifacts(ctx, task, state, artifacts); persistErr != nil {
			err = persistErr
		}
	}

	// Assemble final report
	finalReport := euclotypes.AssembleFinalReport(artifacts)

	// Add deferred next actions if any
	if nextActions := assembleDeferredNextActions(state, artifacts); len(nextActions) > 0 {
		finalReport["deferred_next_actions"] = nextActions
	}

	// Add assurance class to final report
	if assuranceClass, ok := euclostate.GetAssuranceClass(state); ok {
		finalReport["assurance_class"] = assuranceClass
	}

	// Add waiver info if present
	if raw, ok := euclostate.GetWaiver(state); ok && raw != nil {
		finalReport["waiver"] = raw
	}

	// Add degradation info if present
	if gateResult.SuccessGate.DegradationMode != "" {
		finalReport["degradation_mode"] = gateResult.SuccessGate.DegradationMode
	}
	if gateResult.SuccessGate.DegradationReason != "" {
		finalReport["degradation_reason"] = gateResult.SuccessGate.DegradationReason
	}

	// Add provider restore if present
	if raw, ok := euclostate.GetProviderRestore(state); ok && raw != nil {
		finalReport["provider_restore"] = raw
	}

	// Add runtime info
	if raw, ok := euclostate.GetContextRuntime(state); ok {
		finalReport["context_runtime"] = raw
	}
	if runtime, ok := euclostate.GetSecurityRuntime(state); ok {
		finalReport["security_runtime"] = runtime
	}
	if runtime, ok := euclostate.GetSharedContextRuntime(state); ok {
		finalReport["shared_context_runtime"] = runtime
	}

	// Persist final report to state
	euclostate.SetFinalReport(state, finalReport)

	// Emit telemetry
	eucloreporting.EmitObservabilityTelemetry(r.Telemetry, task, actionLog, proofSurface)

	// Collect mutation checkpoints
	mutationCheckpoints := archaeoexec.MutationCheckpointSummaries(state)

	return RecordResult{
		Artifacts:           artifacts,
		ActionLog:           actionLog,
		ProofSurface:        proofSurface,
		FinalReport:         finalReport,
		MutationCheckpoints: mutationCheckpoints,
		Err:                 err,
	}
}

// assembleDeferredNextActions assembles deferred next actions from state or artifacts.
func assembleDeferredNextActions(state *core.Context, artifacts []euclotypes.Artifact) []eucloruntime.DeferralNextAction {
	issues := deferredIssuesFromState(state)
	if len(issues) == 0 {
		issues = deferredIssuesFromArtifacts(artifacts)
	}
	if len(issues) == 0 {
		return nil
	}
	return eucloruntime.AssembleDeferralNextActions(issues)
}

// deferredIssuesFromState extracts deferred issues from state.
func deferredIssuesFromState(state *core.Context) []eucloruntime.DeferredExecutionIssue {
	if state == nil {
		return nil
	}
	issues, ok := euclostate.GetDeferredIssues(state)
	if !ok {
		return nil
	}
	return append([]eucloruntime.DeferredExecutionIssue(nil), issues...)
}

// deferredIssuesFromArtifacts extracts deferred issues from artifacts.
func deferredIssuesFromArtifacts(artifacts []euclotypes.Artifact) []eucloruntime.DeferredExecutionIssue {
	for _, artifact := range artifacts {
		if artifact.Kind != euclotypes.ArtifactKindDeferredExecutionIssues {
			continue
		}
		switch typed := artifact.Payload.(type) {
		case []eucloruntime.DeferredExecutionIssue:
			return append([]eucloruntime.DeferredExecutionIssue(nil), typed...)
		case []any:
			issues := make([]eucloruntime.DeferredExecutionIssue, 0, len(typed))
			for _, item := range typed {
				if issue, ok := item.(eucloruntime.DeferredExecutionIssue); ok {
					issues = append(issues, issue)
				}
			}
			return issues
		}
	}
	return nil
}

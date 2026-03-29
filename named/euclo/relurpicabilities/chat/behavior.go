package chat

import (
	"context"
	"strings"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/execution"
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
	for _, routine := range routines {
		execution.EnsureRoutineArtifacts(in.State, routine, in.Work)
	}
	execution.SetBehaviorTrace(in.State, in.Work, routines)
	return execution.ExecuteWorkflow(ctx, in)
}

func (inspectBehavior) ID() string { return Inspect }

func (inspectBehavior) Execute(ctx context.Context, in execution.ExecuteInput) (*core.Result, error) {
	routines := execution.SupportingIDs(in.Work, "euclo:chat.")
	for _, routine := range routines {
		execution.EnsureRoutineArtifacts(in.State, routine, in.Work)
	}
	execution.AppendDiagnostic(in.State, "euclo.review_findings", "inspect-first relurpic behavior executed")
	execution.SetBehaviorTrace(in.State, in.Work, routines)
	return execution.ExecuteWorkflow(ctx, in)
}

func (implementBehavior) ID() string { return Implement }

func (implementBehavior) Execute(ctx context.Context, in execution.ExecuteInput) (*core.Result, error) {
	routines := append(execution.SupportingIDs(in.Work, "euclo:chat."), execution.SupportingIDs(in.Work, "euclo:archaeology.")...)
	for _, routine := range routines {
		execution.EnsureRoutineArtifacts(in.State, routine, in.Work)
	}
	if execution.ContainsString(in.Work.SupportingRelurpicCapabilityIDs, "euclo:archaeology.explore") {
		execution.AppendDiagnostic(in.State, "euclo.plan_candidates", "lazy archaeology exploration support activated for chat.implement")
	}
	execution.SetBehaviorTrace(in.State, in.Work, routines)
	var artifacts []euclotypes.Artifact

	exploreResult, _, err := execution.ExecuteReactTask(ctx, in, "chat-implement-explore",
		"Explore the codebase to understand the context for: "+execution.CapabilityTaskInstruction(in.Task),
		core.TaskTypeAnalysis,
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

	editResult, _, err := execution.ExecuteReactTask(ctx, in, "chat-implement-edit",
		"Plan and implement the changes for: "+execution.CapabilityTaskInstruction(in.Task),
		core.TaskTypeCodeModification,
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

	verifyResult, _, err := execution.ExecuteReactTask(ctx, in, "chat-implement-verify",
		"Verify the changes by running tests and checking for issues.",
		core.TaskTypeAnalysis,
	)
	if err == nil && verifyResult != nil && verifyResult.Success && in.State != nil {
		if existing, ok := in.State.Get("pipeline.verify"); ok && existing != nil {
			in.State.Set("react.verification_latched_summary", "reused existing verification evidence")
			artifacts = append(artifacts, euclotypes.Artifact{
				ID:         "chat_implement_verification",
				Kind:       euclotypes.ArtifactKindVerification,
				Summary:    "reused existing verification evidence",
				Payload:    existing,
				ProducerID: in.Work.PrimaryRelurpicCapabilityID,
				Status:     "produced",
			})
			execution.MergeStateArtifactsToContext(in.State, artifacts)
			return execution.SuccessResult("chat implement completed with existing verification evidence", artifacts)
		}
	}
	if err != nil || verifyResult == nil || !verifyResult.Success {
		if in.State != nil {
			if existing, ok := in.State.Get("pipeline.verify"); ok && existing != nil {
				in.State.Set("react.verification_latched_summary", "reused existing verification evidence")
				artifacts = append(artifacts, euclotypes.Artifact{
					ID:         "chat_implement_verification",
					Kind:       euclotypes.ArtifactKindVerification,
					Summary:    "reused existing verification evidence",
					Payload:    existing,
					ProducerID: in.Work.PrimaryRelurpicCapabilityID,
					Status:     "produced",
				})
				execution.MergeStateArtifactsToContext(in.State, artifacts)
				return execution.SuccessResult("chat implement completed with existing verification evidence", artifacts)
			}
		}
		if payload, ok := execution.VerificationFallbackPayload(ctx, in); ok {
			if in.State != nil {
				in.State.Set("pipeline.verify", payload)
				in.State.Set("react.verification_latched_summary", strings.TrimSpace(execution.StringValue(payload["summary"])))
			}
			artifacts = append(artifacts, euclotypes.Artifact{
				ID:         "chat_implement_verification",
				Kind:       euclotypes.ArtifactKindVerification,
				Summary:    strings.TrimSpace(execution.StringValue(payload["summary"])),
				Payload:    payload,
				ProducerID: in.Work.PrimaryRelurpicCapabilityID,
				Status:     "produced",
			})
			execution.MergeStateArtifactsToContext(in.State, artifacts)
			return execution.SuccessResult("chat implement completed with fallback verification", artifacts)
		}
		return &core.Result{Success: false, Error: err}, err
	}
	if !execution.VerificationToolAllowed(in.Work) {
		execution.MergeStateArtifactsToContext(in.State, artifacts)
		return execution.SuccessResult("chat implement completed without admitted verification tooling", artifacts)
	}
	verifyPayload := map[string]any{
		"status":  "pass",
		"summary": execution.ResultSummary(verifyResult),
		"checks":  []any{map[string]any{"name": "react_verify", "status": "pass"}},
	}
	if in.State != nil {
		in.State.Set("pipeline.verify", verifyPayload)
		in.State.Set("react.verification_latched_summary", execution.ResultSummary(verifyResult))
	}
	artifacts = append(artifacts, euclotypes.Artifact{
		ID:         "chat_implement_verification",
		Kind:       euclotypes.ArtifactKindVerification,
		Summary:    execution.ResultSummary(verifyResult),
		Payload:    verifyPayload,
		ProducerID: in.Work.PrimaryRelurpicCapabilityID,
		Status:     "produced",
	})
	execution.MergeStateArtifactsToContext(in.State, artifacts)
	return execution.SuccessResult("chat implement completed successfully", artifacts)
}

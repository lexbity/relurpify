package planning

import (
	"context"
	"errors"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/execution"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
)

// PlanningInvocable wraps an EucloCodingCapability as an execution.Invocable.
//
// BKC capabilities are SupportingOnly in the capability registry — users do not
// name them directly — but they can be the primary work in a planning session.
// PlanningInvocable is the correct invocable path for this "orchestrated capability"
// category: selected by system context, not by user intent.
type PlanningInvocable struct {
	capabilityID string
	capability   euclotypes.EucloCodingCapability
}

// NewInvocable creates a PlanningInvocable for the given capability.
func NewInvocable(id string, cap euclotypes.EucloCodingCapability) *PlanningInvocable {
	return &PlanningInvocable{capabilityID: id, capability: cap}
}

func (i *PlanningInvocable) ID() string { return i.capabilityID }

func (i *PlanningInvocable) Invoke(ctx context.Context, in execution.InvokeInput) (*core.Result, error) {
	// Build CapabilitySnapshot from the registry tool permissions — not from
	// WorkflowStore presence, which was the bug in the previous adapter.
	snapshot := eucloruntime.SnapshotCapabilities(in.Environment.Registry)

	artifactState := euclotypes.ArtifactStateFromContext(in.State)
	eligibility := i.capability.Eligible(artifactState, snapshot)
	if !eligibility.Eligible {
		return &core.Result{
			Success: false,
			Error:   errors.New(eligibility.Reason),
			Data:    map[string]any{"eligibility": eligibility},
		}, nil
	}

	envelope := euclotypes.ExecutionEnvelope{
		Task:          in.Task,
		Mode:          in.Mode,
		Profile:       in.Profile,
		Registry:      in.Environment.Registry,
		State:         in.State,
		Memory:        in.Environment.Memory,
		Environment:   in.Environment,
		PlanStore:     in.ServiceBundle.PlanStore,
		WorkflowStore: in.ServiceBundle.WorkflowStore,
		Telemetry:     in.Telemetry,
	}

	result := i.capability.Execute(ctx, envelope)

	var err error
	if result.FailureInfo != nil {
		err = errors.New(result.FailureInfo.Message)
	}
	return &core.Result{
		Success: result.Status == euclotypes.ExecutionStatusCompleted,
		Data: map[string]any{
			"summary":   result.Summary,
			"artifacts": result.Artifacts,
			"status":    result.Status,
		},
		Error: err,
	}, nil
}

func (i *PlanningInvocable) IsPrimary() bool { return true }

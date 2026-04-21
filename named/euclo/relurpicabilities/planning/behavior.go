package planning

import (
	"context"
	"errors"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	"codeburg.org/lexbit/relurpify/named/euclo/execution"
	eucloruntime "codeburg.org/lexbit/relurpify/named/euclo/runtime"
)

// PlanningBehavior wraps an EucloCodingCapability as an execution.Behavior.
//
// BKC capabilities are SupportingOnly in the capability registry — users do not
// name them directly — but they can be the primary work in a planning session.
// PlanningBehavior is the correct dispatch path for this "orchestrated capability"
// category: selected by system context, not by user intent.
type PlanningBehavior struct {
	capabilityID string
	capability   euclotypes.EucloCodingCapability
}

// Deprecated: Use planning.NewInvocable instead
func New(id string, cap euclotypes.EucloCodingCapability) *PlanningBehavior {
	return &PlanningBehavior{capabilityID: id, capability: cap}
}

func (b *PlanningBehavior) ID() string { return b.capabilityID }

func (b *PlanningBehavior) Execute(ctx context.Context, in execution.ExecuteInput) (*core.Result, error) {
	// Build CapabilitySnapshot from the registry tool permissions — not from
	// WorkflowStore presence, which was the bug in the previous adapter.
	snapshot := eucloruntime.SnapshotCapabilities(in.Environment.Registry)

	artifactState := euclotypes.ArtifactStateFromContext(in.State)
	eligibility := b.capability.Eligible(artifactState, snapshot)
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

	result := b.capability.Execute(ctx, envelope)

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

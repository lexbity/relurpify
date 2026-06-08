package runtime

import (
	"context"
	"fmt"
	"strings"
	"time"

	capability "codeburg.org/lexbit/relurpify/capability"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/execution/agentlifecycle"
	fauthorization "codeburg.org/lexbit/relurpify/governance/authorization"
	policy "codeburg.org/lexbit/relurpify/governance/policy"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

func (r *Runtime) StartDelegation(ctx context.Context, request policy.DelegationRequest, opts fauthorization.DelegationStartOptions) (*policy.DelegationSnapshot, error) {
	if r == nil || r.Delegations == nil {
		return nil, fmt.Errorf("runtime delegations unavailable")
	}
	return r.Delegations.StartDelegation(ctx, request, opts)
}

func (r *Runtime) ExecuteDelegation(ctx context.Context, request policy.DelegationRequest, opts fauthorization.DelegationExecutionOptions) (*policy.DelegationSnapshot, error) {
	if r == nil || r.Delegations == nil || r.Tools == nil {
		return nil, fmt.Errorf("runtime delegations unavailable")
	}
	opts.Registry = capability.NewDelegationRegistry(r.Tools)
	opts.AgentSpec = r.AgentWorkspace().AgentSpec
	opts.State = firstDelegationContext(opts.State)
	if shouldUseBackgroundDelegation(request) {
		runner, err := r.ensureBackgroundDelegationProvider(ctx)
		if err != nil {
			return nil, err
		}
		opts.Background = true
		opts.BackgroundRunner = runner
	}
	return r.Delegations.ExecuteDelegation(ctx, request, opts)
}

func (r *Runtime) CompleteDelegation(id string, result *policy.DelegationResult) (*policy.DelegationSnapshot, error) {
	if r == nil || r.Delegations == nil {
		return nil, fmt.Errorf("runtime delegations unavailable")
	}
	return r.Delegations.CompleteDelegation(id, result)
}

func (r *Runtime) CancelDelegation(ctx context.Context, id, reason string) (*policy.DelegationSnapshot, error) {
	if r == nil || r.Delegations == nil {
		return nil, fmt.Errorf("runtime delegations unavailable")
	}
	return r.Delegations.CancelDelegation(ctx, id, reason)
}

func (r *Runtime) ListDelegations(filter policy.DelegationFilter) []policy.DelegationSnapshot {
	if r == nil || r.Delegations == nil {
		return nil
	}
	return r.Delegations.ListDelegations(filter)
}

func (r *Runtime) SnapshotDelegations() []policy.DelegationSnapshot {
	if r == nil || r.Delegations == nil {
		return nil
	}
	return r.Delegations.SnapshotDelegations()
}

func (r *Runtime) PersistDelegations(ctx context.Context, repo agentlifecycle.Repository, workflowID, runID string) error {
	if r == nil || r.Delegations == nil {
		return fmt.Errorf("runtime delegations unavailable")
	}
	return r.Delegations.PersistDelegations(ctx, repo, workflowID, runID)
}

func firstDelegationContext(values ...*contextdata.Envelope) *contextdata.Envelope {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func (r *Runtime) ensureBackgroundDelegationProvider(ctx context.Context) (*backgroundDelegationProvider, error) {
	if r == nil {
		return nil, fmt.Errorf("runtime unavailable")
	}
	r.delegationMu.Lock()
	defer r.delegationMu.Unlock()
	if r.delegationBG != nil {
		return r.delegationBG, nil
	}
	provider := newBackgroundDelegationProvider()
	if err := r.RegisterProvider(ctx, provider); err != nil {
		return nil, err
	}
	r.delegationBG = provider
	return provider, nil
}

func shouldUseBackgroundDelegation(request policy.DelegationRequest) bool {
	if request.Metadata == nil {
		return false
	}
	value, ok := request.Metadata["background"]
	if !ok || value == nil {
		return false
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
	}
}

func (r *Runtime) observeDelegationSnapshot(snapshot policy.DelegationSnapshot) {
	if r == nil {
		return
	}
	r.emitDelegationTelemetry(snapshot)
	r.logDelegationAudit(snapshot)
}

func (r *Runtime) emitDelegationTelemetry(snapshot policy.DelegationSnapshot) {
	if r == nil || r.AgentWorkspace().Telemetry == nil {
		return
	}
	eventType := telemetry.EventDelegationFinish
	switch snapshot.State {
	case policy.DelegationStateRunning:
		eventType = telemetry.EventDelegationStart
	case policy.DelegationStateCancelled:
		eventType = telemetry.EventDelegationCancel
	}
	metadata := map[string]interface{}{
		"delegation_id":        snapshot.Request.ID,
		"workflow_id":          snapshot.Request.WorkflowID,
		"task_id":              snapshot.Request.TaskID,
		"task_type":            snapshot.Request.TaskType,
		"target_capability_id": snapshot.Request.TargetCapabilityID,
		"target_provider_id":   snapshot.Request.TargetProviderID,
		"target_session_id":    snapshot.Request.TargetSessionID,
		"state":                snapshot.State,
		"background":           snapshot.Background,
		"recoverability":       snapshot.Recoverability,
		"trust_class":          snapshot.TrustClass,
	}
	if snapshot.Result != nil {
		metadata["result_success"] = snapshot.Result.Success
		if ins, ok := snapshot.Result.Insertion.(capability.InsertionDecision); ok {
			metadata["insertion_action"] = ins.Action
		}
		if prov, ok := snapshot.Result.Provenance.(capability.ContentProvenance); ok {
			metadata["result_trust_class"] = prov.TrustClass
		}
	}
	r.AgentWorkspace().Telemetry.Emit(telemetry.Event{
		Type:      eventType,
		TaskID:    firstDelegationTaskID(snapshot),
		Message:   delegationTelemetryMessage(snapshot),
		Timestamp: time.Now().UTC(),
		Metadata:  metadata,
	})
}

func (r *Runtime) logDelegationAudit(snapshot policy.DelegationSnapshot) {
	if r == nil || r.AgentWorkspace().Registration == nil || r.AgentWorkspace().Registration.Audit == nil {
		return
	}
	result := string(snapshot.State)
	if snapshot.Result != nil && snapshot.Result.Success {
		result = "success"
	}
	metadata := map[string]interface{}{
		"delegation_id":        snapshot.Request.ID,
		"workflow_id":          snapshot.Request.WorkflowID,
		"task_id":              snapshot.Request.TaskID,
		"task_type":            snapshot.Request.TaskType,
		"target_capability_id": snapshot.Request.TargetCapabilityID,
		"target_provider_id":   snapshot.Request.TargetProviderID,
		"target_session_id":    snapshot.Request.TargetSessionID,
		"background":           snapshot.Background,
		"recoverability":       snapshot.Recoverability,
		"trust_class":          snapshot.TrustClass,
	}
	if snapshot.Result != nil {
		metadata["result_success"] = snapshot.Result.Success
		if ins, ok := snapshot.Result.Insertion.(capability.InsertionDecision); ok {
			metadata["insertion_action"] = ins.Action
		}
	}
	_ = r.AgentWorkspace().Registration.Audit.Log(context.Background(), policy.AuditRecord{
		Timestamp: time.Now().UTC(),
		AgentID:   r.AgentWorkspace().Registration.ID,
		Action:    "delegation",
		Type:      string(snapshot.State),
		Result:    result,
		Metadata:  metadata,
	})
}

func delegationTelemetryMessage(snapshot policy.DelegationSnapshot) string {
	target := snapshot.Request.TargetCapabilityID
	if target == "" {
		target = snapshot.Request.TargetProviderID
	}
	switch snapshot.State {
	case policy.DelegationStateRunning:
		return fmt.Sprintf("delegation %s started for %s", snapshot.Request.ID, target)
	case policy.DelegationStateCancelled:
		return fmt.Sprintf("delegation %s cancelled for %s", snapshot.Request.ID, target)
	case policy.DelegationStateSucceeded:
		return fmt.Sprintf("delegation %s succeeded for %s", snapshot.Request.ID, target)
	case policy.DelegationStateFailed:
		return fmt.Sprintf("delegation %s failed for %s", snapshot.Request.ID, target)
	default:
		return fmt.Sprintf("delegation %s updated for %s", snapshot.Request.ID, target)
	}
}

func firstDelegationTaskID(snapshot policy.DelegationSnapshot) string {
	if strings.TrimSpace(snapshot.Request.TaskID) != "" {
		return snapshot.Request.TaskID
	}
	return snapshot.Request.WorkflowID
}

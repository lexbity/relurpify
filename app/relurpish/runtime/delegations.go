package runtime

import (
	"context"
	"fmt"
	"strings"
	"time"

	fauthorization "codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/config"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
	"codeburg.org/lexbit/relurpify/framework/memory/db"
)

func (r *Runtime) StartDelegation(ctx context.Context, request core.DelegationRequest, opts fauthorization.DelegationStartOptions) (*core.DelegationSnapshot, error) {
	if r == nil || r.Delegations == nil {
		return nil, fmt.Errorf("runtime delegations unavailable")
	}
	return r.Delegations.StartDelegation(ctx, request, opts)
}

func (r *Runtime) ExecuteDelegation(ctx context.Context, request core.DelegationRequest, opts fauthorization.DelegationExecutionOptions) (*core.DelegationSnapshot, error) {
	if r == nil || r.Delegations == nil || r.Tools == nil {
		return nil, fmt.Errorf("runtime delegations unavailable")
	}
	opts.Registry = r.Tools
	opts.AgentSpec = r.AgentSpec
	opts.State = firstDelegationContext(opts.State, r.Context)
	if shouldUseBackgroundDelegation(request) {
		runner, err := r.ensureBackgroundDelegationProvider(ctx)
		if err != nil {
			return nil, err
		}
		opts.Background = true
		opts.BackgroundRunner = runner
	}
	if opts.WorkflowStore == nil && strings.TrimSpace(request.WorkflowID) != "" {
		store, err := db.NewSQLiteWorkflowStateStore(config.New(r.Config.Workspace).WorkflowStateFile())
		if err != nil {
			return nil, err
		}
		defer store.Close()
		opts.WorkflowStore = store
		return r.Delegations.ExecuteDelegation(ctx, request, opts)
	}
	return r.Delegations.ExecuteDelegation(ctx, request, opts)
}

func (r *Runtime) CompleteDelegation(id string, result *core.DelegationResult) (*core.DelegationSnapshot, error) {
	if r == nil || r.Delegations == nil {
		return nil, fmt.Errorf("runtime delegations unavailable")
	}
	return r.Delegations.CompleteDelegation(id, result)
}

func (r *Runtime) CancelDelegation(ctx context.Context, id, reason string) (*core.DelegationSnapshot, error) {
	if r == nil || r.Delegations == nil {
		return nil, fmt.Errorf("runtime delegations unavailable")
	}
	return r.Delegations.CancelDelegation(ctx, id, reason)
}

func (r *Runtime) ListDelegations(filter core.DelegationFilter) []core.DelegationSnapshot {
	if r == nil || r.Delegations == nil {
		return nil
	}
	return r.Delegations.ListDelegations(filter)
}

func (r *Runtime) SnapshotDelegations() []core.DelegationSnapshot {
	if r == nil || r.Delegations == nil {
		return nil
	}
	return r.Delegations.SnapshotDelegations()
}

func (r *Runtime) PersistDelegations(ctx context.Context, store memory.WorkflowStateStore, workflowID, runID string) error {
	if r == nil || r.Delegations == nil {
		return fmt.Errorf("runtime delegations unavailable")
	}
	return r.Delegations.PersistDelegations(ctx, store, workflowID, runID)
}

func firstDelegationContext(values ...*core.Context) *core.Context {
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

func shouldUseBackgroundDelegation(request core.DelegationRequest) bool {
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

func (r *Runtime) observeDelegationSnapshot(snapshot core.DelegationSnapshot) {
	if r == nil {
		return
	}
	r.emitDelegationTelemetry(snapshot)
	r.logDelegationAudit(snapshot)
}

func (r *Runtime) emitDelegationTelemetry(snapshot core.DelegationSnapshot) {
	if r == nil || r.Telemetry == nil {
		return
	}
	eventType := core.EventDelegationFinish
	switch snapshot.State {
	case core.DelegationStateRunning:
		eventType = core.EventDelegationStart
	case core.DelegationStateCancelled:
		eventType = core.EventDelegationCancel
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
		metadata["insertion_action"] = snapshot.Result.Insertion.Action
		metadata["result_trust_class"] = snapshot.Result.Provenance.TrustClass
	}
	r.Telemetry.Emit(core.Event{
		Type:      eventType,
		TaskID:    firstDelegationTaskID(snapshot),
		Message:   delegationTelemetryMessage(snapshot),
		Timestamp: time.Now().UTC(),
		Metadata:  metadata,
	})
}

func (r *Runtime) logDelegationAudit(snapshot core.DelegationSnapshot) {
	if r == nil || r.Registration == nil || r.Registration.Audit == nil {
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
		metadata["insertion_action"] = snapshot.Result.Insertion.Action
		metadata["result_success"] = snapshot.Result.Success
	}
	_ = r.Registration.Audit.Log(context.Background(), core.AuditRecord{
		Timestamp: time.Now().UTC(),
		AgentID:   r.Registration.ID,
		Action:    "delegation",
		Type:      string(snapshot.State),
		Result:    result,
		Metadata:  metadata,
	})
}

func delegationTelemetryMessage(snapshot core.DelegationSnapshot) string {
	target := snapshot.Request.TargetCapabilityID
	if target == "" {
		target = snapshot.Request.TargetProviderID
	}
	switch snapshot.State {
	case core.DelegationStateRunning:
		return fmt.Sprintf("delegation %s started for %s", snapshot.Request.ID, target)
	case core.DelegationStateCancelled:
		return fmt.Sprintf("delegation %s cancelled for %s", snapshot.Request.ID, target)
	case core.DelegationStateSucceeded:
		return fmt.Sprintf("delegation %s succeeded for %s", snapshot.Request.ID, target)
	case core.DelegationStateFailed:
		return fmt.Sprintf("delegation %s failed for %s", snapshot.Request.ID, target)
	default:
		return fmt.Sprintf("delegation %s updated for %s", snapshot.Request.ID, target)
	}
}

func firstDelegationTaskID(snapshot core.DelegationSnapshot) string {
	if strings.TrimSpace(snapshot.Request.TaskID) != "" {
		return snapshot.Request.TaskID
	}
	return snapshot.Request.WorkflowID
}

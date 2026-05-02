package nexus

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/agentlifecycle"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/rex/reconcile"
	rexctx "codeburg.org/lexbit/relurpify/named/rex/rexctx"
	"codeburg.org/lexbit/relurpify/named/rex/rexkeys"
	rexstate "codeburg.org/lexbit/relurpify/named/rex/state"
	fwfmp "codeburg.org/lexbit/relurpify/relurpnet/fmp"
	"codeburg.org/lexbit/relurpify/relurpnet/identity"
)

var _ rexstate.ExecutionObserver = (*LineageBridge)(nil)

type LineageBinding struct {
	LineageID string    `json:"lineage_id"`
	AttemptID string    `json:"attempt_id"`
	RuntimeID string    `json:"runtime_id"`
	SessionID string    `json:"session_id,omitempty"`
	State     string    `json:"state,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

// LineageBridge projects rex workflow execution into FMP lineage and attempt state.
type LineageBridge struct {
	Service        *fwfmp.Service
	LifecycleRepo  agentlifecycle.Repository
	RuntimeID      string
	Now            func() time.Time
	PolicyResolver rexctx.TrustedContextResolver
}

func (b *LineageBridge) HandleFrameworkEvent(ctx context.Context, frameworkEvent core.FrameworkEvent) error {
	if b == nil || b.LifecycleRepo == nil {
		return nil
	}
	state, ok, err := bridgeStateForFrameworkEvent(frameworkEvent)
	if err != nil || !ok {
		return err
	}
	payload, err := decodeFrameworkPayload(frameworkEvent)
	if err != nil {
		return err
	}
	bindings, err := b.findBindings(ctx, payload)
	if err != nil {
		return err
	}
	for _, match := range bindings {
		if nextState, changed := applyBridgeState(match.Binding, payload, state); changed {
			match.Binding.State = nextState
			match.Binding.UpdatedAt = frameworkEvent.Timestamp.UTC()
			if err := b.persistBinding(ctx, match.WorkflowID, match.RunID, match.Binding); err != nil {
				return err
			}
		}
		if err := b.LifecycleRepo.AppendEvent(ctx, agentlifecycle.WorkflowEventRecord{
			EventID:    fmt.Sprintf("%s:%d", match.RunID, frameworkEvent.Seq),
			WorkflowID: match.WorkflowID,
			RunID:      match.RunID,
			EventType:  frameworkEvent.Type,
			Payload:    payload,
			Sequence:   frameworkEvent.Seq,
			CreatedAt:  frameworkEvent.Timestamp.UTC(),
		}); err != nil {
			return err
		}
	}
	return nil
}

type matchedBinding struct {
	WorkflowID string
	RunID      string
	Binding    LineageBinding
}

func (b *LineageBridge) BeforeExecute(ctx context.Context, workflowID, runID string, task *core.Task, env *contextdata.Envelope) error {
	if b == nil || b.LifecycleRepo == nil {
		return nil
	}
	now := b.nowUTC()
	if err := b.persistTaskRequest(ctx, workflowID, runID, task, env, now); err != nil {
		return err
	}
	trusted := b.resolveTrustedExecutionContext(ctx, task, env)
	if env != nil {
		if strings.TrimSpace(trusted.SessionID) != "" {
			env.SetWorkingValue(rexkeys.GatewaySessionID, trusted.SessionID, contextdata.MemoryClassTask)
		}
		if strings.TrimSpace(string(trusted.SensitivityClass)) != "" {
			env.SetWorkingValue("fmp.sensitivity_class", string(trusted.SensitivityClass), contextdata.MemoryClassTask)
		}
		if len(trusted.FederationTargets) > 0 {
			env.SetWorkingValue("fmp.allowed_federation_targets", append([]string(nil), trusted.FederationTargets...), contextdata.MemoryClassTask)
		}
	}
	binding, err := b.ensureBinding(ctx, workflowID, runID, task, env, now)
	if err != nil || binding == nil {
		return err
	}
	attempt, ok, err := b.Service.Ownership.GetAttempt(ctx, binding.AttemptID)
	if err != nil {
		return err
	}
	startTime := now
	if ok && !attempt.StartTime.IsZero() {
		startTime = attempt.StartTime
	}
	record := fwfmp.AttemptRecord{
		AttemptID:        binding.AttemptID,
		LineageID:        binding.LineageID,
		RuntimeID:        binding.RuntimeID,
		State:            fwfmp.AttemptStateRunning,
		StartTime:        startTime,
		LastProgressTime: now,
	}
	if ok {
		record.LeaseID = attempt.LeaseID
		record.LeaseExpiry = attempt.LeaseExpiry
		record.Fenced = attempt.Fenced
		record.FencingEpoch = attempt.FencingEpoch
		record.PreviousAttemptID = attempt.PreviousAttemptID
	}
	if err := b.Service.Ownership.UpsertAttempt(ctx, record); err != nil {
		return err
	}
	binding.State = string(record.State)
	binding.UpdatedAt = now
	env.SetWorkingValue(rexkeys.FMPLineageID, binding.LineageID, contextdata.MemoryClassTask)
	env.SetWorkingValue(rexkeys.FMPAttemptID, binding.AttemptID, contextdata.MemoryClassTask)
	env.SetWorkingValue(rexkeys.RexFMPLineageID, binding.LineageID, contextdata.MemoryClassTask)
	env.SetWorkingValue(rexkeys.RexFMPAttemptID, binding.AttemptID, contextdata.MemoryClassTask)
	return b.persistBinding(ctx, workflowID, runID, *binding)
}

func (b *LineageBridge) AfterExecute(ctx context.Context, workflowID, runID string, _ *core.Task, env *contextdata.Envelope, _ *core.Result, execErr error) error {
	if b == nil || b.LifecycleRepo == nil || b.Service == nil || b.Service.Ownership == nil {
		return nil
	}
	binding, err := b.bindingFromEnvelope(ctx, workflowID, runID, env)
	if err != nil || binding == nil {
		return err
	}
	attempt, ok, err := b.Service.Ownership.GetAttempt(ctx, binding.AttemptID)
	if err != nil || !ok {
		return err
	}
	attempt.LastProgressTime = b.nowUTC()
	if execErr != nil {
		attempt.State = fwfmp.AttemptStateFailed
		binding.State = string(fwfmp.AttemptStateFailed)
	} else {
		attempt.State = fwfmp.AttemptStateCompleted
		binding.State = string(fwfmp.AttemptStateCompleted)
	}
	if err := b.Service.Ownership.UpsertAttempt(ctx, *attempt); err != nil {
		return err
	}
	binding.UpdatedAt = attempt.LastProgressTime
	return b.persistBinding(ctx, workflowID, runID, *binding)
}

func (b *LineageBridge) ensureBinding(ctx context.Context, workflowID, runID string, task *core.Task, env *contextdata.Envelope, now time.Time) (*LineageBinding, error) {
	if env != nil {
		lineageID := ""
		if val, ok := env.GetWorkingValue(rexkeys.FMPLineageID); ok {
			lineageID = strings.TrimSpace(fmt.Sprint(val))
		}
		if lineageID == "" {
			if val, ok := env.GetWorkingValue(rexkeys.RexFMPLineageID); ok {
				lineageID = strings.TrimSpace(fmt.Sprint(val))
			}
		}
		if lineageID != "" {
			attemptID := ""
			if val, ok := env.GetWorkingValue(rexkeys.FMPAttemptID); ok {
				attemptID = strings.TrimSpace(fmt.Sprint(val))
			}
			if attemptID == "" {
				if val, ok := env.GetWorkingValue(rexkeys.RexFMPAttemptID); ok {
					attemptID = strings.TrimSpace(fmt.Sprint(val))
				}
			}
			if attemptID == "" {
				attemptID = runID
			}
			sessionID := ""
			if val, ok := env.GetWorkingValue(rexkeys.GatewaySessionID); ok {
				sessionID = strings.TrimSpace(fmt.Sprint(val))
			}
			return &LineageBinding{
				LineageID: lineageID,
				AttemptID: attemptID,
				RuntimeID: b.runtimeID(),
				SessionID: sessionID,
				UpdatedAt: now,
			}, nil
		}
	}
	binding, err := b.readBinding(ctx, workflowID, runID)
	if err != nil || binding != nil {
		return binding, err
	}
	if b.Service == nil || b.Service.Ownership == nil {
		return nil, nil
	}
	trusted := b.resolveTrustedExecutionContext(ctx, task, env)
	sessionID := sessionIDFromEnvelope(env, task)
	if sessionID == "" {
		sessionID = trusted.SessionID
	}
	if sessionID == "" {
		return nil, nil
	}
	lineageID := "rex-lineage:" + workflowID
	lineage, err := b.Service.CreateLineageFromSession(ctx, fwfmp.SessionLineageRequest{
		LineageID:                lineageID,
		SessionID:                sessionID,
		TaskClass:                "agent.run",
		ContextClass:             "workflow-runtime",
		CapabilityEnvelope:       defaultCapabilityEnvelope(),
		SensitivityClass:         fwfmp.SensitivityClass(trusted.SensitivityClass),
		AllowedFederationTargets: append([]string(nil), trusted.FederationTargets...),
	})
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "already exists") {
		return nil, err
	}
	if lineage == nil {
		lineage, _, err = b.Service.Ownership.GetLineage(ctx, lineageID)
		if err != nil {
			return nil, err
		}
	}
	if lineage == nil {
		return nil, fmt.Errorf("lineage %s unavailable", lineageID)
	}
	return &LineageBinding{
		LineageID: lineage.LineageID,
		AttemptID: runID,
		RuntimeID: b.runtimeID(),
		SessionID: sessionID,
		UpdatedAt: now,
	}, nil
}

func (b *LineageBridge) readBinding(ctx context.Context, workflowID, runID string) (*LineageBinding, error) {
	if b.LifecycleRepo == nil {
		return nil, nil
	}
	// Try to find lineage binding by workflow/run
	bindings, err := b.LifecycleRepo.FindLineageBindingByWorkflow(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	for _, binding := range bindings {
		if binding.RunID == runID {
			return &LineageBinding{
				LineageID: binding.LineageID,
				AttemptID: binding.AttemptID,
				RuntimeID: b.runtimeID(),
				UpdatedAt: binding.UpdatedAt,
			}, nil
		}
	}
	// Fallback to artifact-based lookup
	artifacts, err := b.LifecycleRepo.ListArtifacts(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	for _, artifact := range artifacts {
		if artifact.RunID != runID {
			continue
		}
		if artifact.Kind != "rex.fmp_lineage" || strings.TrimSpace(artifact.InlineRawText) == "" {
			continue
		}
		var binding LineageBinding
		if err := json.Unmarshal([]byte(artifact.InlineRawText), &binding); err != nil {
			return nil, err
		}
		return &binding, nil
	}
	return nil, nil
}

func (b *LineageBridge) bindingFromEnvelope(ctx context.Context, workflowID, runID string, env *contextdata.Envelope) (*LineageBinding, error) {
	if env != nil {
		lineageID := ""
		if val, ok := env.GetWorkingValue(rexkeys.FMPLineageID); ok {
			lineageID = strings.TrimSpace(fmt.Sprint(val))
		}
		attemptID := ""
		if val, ok := env.GetWorkingValue(rexkeys.FMPAttemptID); ok {
			attemptID = strings.TrimSpace(fmt.Sprint(val))
		}
		if lineageID != "" && attemptID != "" {
			return &LineageBinding{
				LineageID: lineageID,
				AttemptID: attemptID,
				RuntimeID: b.runtimeID(),
				UpdatedAt: b.nowUTC(),
			}, nil
		}
	}
	return b.readBinding(ctx, workflowID, runID)
}

func (b *LineageBridge) persistTaskRequest(ctx context.Context, workflowID, runID string, task *core.Task, env *contextdata.Envelope, now time.Time) error {
	if task == nil {
		return nil
	}
	payload := map[string]any{
		"task": map[string]any{
			"id":          task.ID,
			"type":        task.Type,
			"instruction": task.Instruction,
			"context":     task.Context,
			"metadata":    task.Metadata,
		},
		"env": env,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return b.LifecycleRepo.UpsertArtifact(ctx, agentlifecycle.WorkflowArtifactRecord{
		ArtifactID:        runID + ":task-request",
		WorkflowID:        workflowID,
		RunID:             runID,
		Kind:              "rex.task_request",
		ContentType:       "application/json",
		StorageKind:       agentlifecycle.ArtifactStorageInline,
		SummaryText:       "rex task request",
		InlineRawText:     string(raw),
		SummaryMetadata:   map[string]any{"session_id": b.executionSessionID(ctx, task, env)},
		RawSizeBytes:      int64(len(raw)),
		CompressionMethod: "none",
		CreatedAt:         now,
	})
}

func (b *LineageBridge) persistBinding(ctx context.Context, workflowID, runID string, binding LineageBinding) error {
	if b.LifecycleRepo == nil {
		return nil
	}
	return b.LifecycleRepo.UpsertLineageBinding(ctx, agentlifecycle.LineageBindingRecord{
		BindingID:   workflowID + ":" + runID,
		WorkflowID:  workflowID,
		RunID:       runID,
		LineageID:   binding.LineageID,
		AttemptID:   binding.AttemptID,
		BindingType: "fmp",
		Metadata: map[string]any{
			"runtime_id": binding.RuntimeID,
			"session_id": binding.SessionID,
			"state":      binding.State,
		},
		UpdatedAt: binding.UpdatedAt,
	})
}

// ApplyReconciliationOutcome updates FMP attempt state based on a reconciliation outcome.
// This is called when Rex reconciliation completes for a workflow with an FMP lineage.
func (b *LineageBridge) ApplyReconciliationOutcome(ctx context.Context, workflowID, runID string, outcome *reconcile.Record) error {
	if b == nil || b.Service == nil || b.Service.Ownership == nil || outcome == nil {
		return nil
	}

	binding, err := b.bindingFromEnvelope(ctx, workflowID, runID, nil)
	if err != nil || binding == nil {
		return err
	}

	// Update the FMP attempt state via the service
	if _, err := b.Service.ReconcileAttemptFromOutcome(ctx, binding.LineageID, outcome); err != nil {
		return err
	}

	// Update the binding state to reflect the new FMP attempt state
	attempt, ok, err := b.Service.Ownership.GetAttempt(ctx, binding.AttemptID)
	if err != nil || !ok {
		return err
	}

	binding.State = string(attempt.State)
	binding.UpdatedAt = b.nowUTC()
	return b.persistBinding(ctx, workflowID, runID, *binding)
}

func (b *LineageBridge) ResolveReconciliationBinding(ctx context.Context, workflowID, runID string) (*reconcile.Binding, error) {
	if b == nil || b.Service == nil || b.Service.Ownership == nil {
		return nil, nil
	}
	binding, err := b.bindingFromEnvelope(ctx, workflowID, runID, nil)
	if err != nil || binding == nil {
		return nil, err
	}
	attempt, ok, err := b.Service.Ownership.GetAttempt(ctx, binding.AttemptID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return &reconcile.Binding{
			LineageID: binding.LineageID,
			AttemptID: binding.AttemptID,
		}, nil
	}
	return &reconcile.Binding{
		LineageID:    binding.LineageID,
		AttemptID:    binding.AttemptID,
		FencingEpoch: attempt.FencingEpoch,
	}, nil
}

func (b *LineageBridge) findBindings(ctx context.Context, payload map[string]any) ([]matchedBinding, error) {
	if b.LifecycleRepo == nil {
		return nil, nil
	}
	candidates := bindingSearchCandidates(payload)
	var out []matchedBinding
	seen := map[string]struct{}{}
	for _, candidate := range candidates {
		var records []agentlifecycle.LineageBindingRecord
		if candidate.kind == "lineage" {
			record, err := b.LifecycleRepo.FindLineageBindingByLineageID(ctx, candidate.value)
			if err != nil {
				return nil, err
			}
			if record != nil {
				records = []agentlifecycle.LineageBindingRecord{*record}
			}
		} else {
			record, err := b.LifecycleRepo.FindLineageBindingByAttemptID(ctx, candidate.value)
			if err != nil {
				return nil, err
			}
			if record != nil {
				records = []agentlifecycle.LineageBindingRecord{*record}
			}
		}
		for _, record := range records {
			key := record.WorkflowID + "\x00" + record.RunID
			if _, ok := seen[key]; ok {
				continue
			}
			binding := bindingFromRecord(record)
			if !matchesFrameworkBinding(*binding, payload) {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, matchedBinding{
				WorkflowID: record.WorkflowID,
				RunID:      record.RunID,
				Binding:    *binding,
			})
		}
	}
	return out, nil
}

func (b *LineageBridge) runtimeID() string {
	if b != nil && strings.TrimSpace(b.RuntimeID) != "" {
		return strings.TrimSpace(b.RuntimeID)
	}
	return "rex"
}

func (b *LineageBridge) nowUTC() time.Time {
	if b != nil && b.Now != nil {
		return b.Now().UTC()
	}
	return time.Now().UTC()
}

type bindingSearchCandidate struct {
	kind  string
	value string
}

func bindingSearchCandidates(payload map[string]any) []bindingSearchCandidate {
	seen := map[string]struct{}{}
	out := make([]bindingSearchCandidate, 0, 4)
	add := func(kind, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		key := kind + ":" + strings.ToLower(value)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, bindingSearchCandidate{kind: kind, value: value})
	}
	add("lineage", stringValue(payload["lineage_id"]))
	add("attempt", stringValue(payload["attempt_id"]))
	add("attempt", stringValue(payload["old_attempt"]))
	add("attempt", stringValue(payload["new_attempt"]))
	return out
}

func bindingFromRecord(record agentlifecycle.LineageBindingRecord) *LineageBinding {
	binding := &LineageBinding{
		LineageID: record.LineageID,
		AttemptID: record.AttemptID,
		RuntimeID: "",
		SessionID: "",
		State:     "",
		UpdatedAt: record.UpdatedAt,
	}
	if record.Metadata != nil {
		if val, ok := record.Metadata["runtime_id"].(string); ok {
			binding.RuntimeID = val
		}
		if val, ok := record.Metadata["session_id"].(string); ok {
			binding.SessionID = val
		}
		if val, ok := record.Metadata["state"].(string); ok {
			binding.State = val
		}
	}
	return binding
}

func (b *LineageBridge) resolveTrustedExecutionContext(ctx context.Context, task *core.Task, env *contextdata.Envelope) rexctx.TrustedExecutionContext {
	if trusted, ok := rexctx.TrustedExecutionContextFromContext(ctx); ok {
		return trusted
	}
	if b != nil && b.PolicyResolver != nil {
		actor := identity.EventActor{
			TenantID: firstNonEmpty(
				envelopeString(env, rexkeys.RexAdmissionTenantID),
				envelopeString(env, rexkeys.GatewayTenantID),
			),
			ID: firstNonEmpty(
				envelopeString(env, rexkeys.GatewaySessionID),
				sessionIDFromEnvelope(env, task),
			),
			Kind: firstNonEmpty(
				envelopeString(env, rexkeys.RexWorkloadClass),
				envelopeString(env, "gateway.role"),
			),
		}
		if resolved, err := b.PolicyResolver.Resolve(ctx, actor); err == nil {
			return resolved
		}
	}
	resolved, _ := rexctx.DefaultTrustedContextResolver{}.Resolve(ctx, identity.EventActor{})
	return resolved
}

func (b *LineageBridge) executionSessionID(ctx context.Context, task *core.Task, env *contextdata.Envelope) string {
	if trusted, ok := rexctx.TrustedExecutionContextFromContext(ctx); ok && strings.TrimSpace(trusted.SessionID) != "" {
		return strings.TrimSpace(trusted.SessionID)
	}
	if sessionID := sessionIDFromEnvelope(env, task); strings.TrimSpace(sessionID) != "" {
		return strings.TrimSpace(sessionID)
	}
	return ""
}

func envelopeString(env *contextdata.Envelope, key string) string {
	if env == nil {
		return ""
	}
	if val, ok := env.GetWorkingValue(key); ok {
		return strings.TrimSpace(fmt.Sprint(val))
	}
	return ""
}

func sessionIDFromEnvelope(env *contextdata.Envelope, task *core.Task) string {
	if env != nil {
		for _, key := range []string{rexkeys.GatewaySessionID, rexkeys.SessionID} {
			if val, ok := env.GetWorkingValue(key); ok {
				if value := strings.TrimSpace(fmt.Sprint(val)); value != "" {
					return value
				}
			}
		}
	}
	if task != nil {
		for _, key := range []string{rexkeys.SessionID, rexkeys.GatewaySessionID} {
			if value := strings.TrimSpace(stringValue(task.Context[key])); value != "" {
				return value
			}
		}
	}
	return ""
}

func defaultCapabilityEnvelope() fwfmp.CapabilityEnvelope {
	return fwfmp.CapabilityEnvelope{
		AllowedCapabilityIDs: []string{
			"plan",
			"execute",
			"code",
			"explain",
		},
		AllowedTaskClasses: []string{"agent.run"},
	}
}

func decodeFrameworkPayload(frameworkEvent core.FrameworkEvent) (map[string]any, error) {
	if len(frameworkEvent.Payload) == 0 {
		return map[string]any{}, nil
	}
	var payload map[string]any
	if err := json.Unmarshal(frameworkEvent.Payload, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func bridgeStateForFrameworkEvent(frameworkEvent core.FrameworkEvent) (string, bool, error) {
	switch frameworkEvent.Type {
	case fwfmp.FrameworkEventFMPHandoffOffered:
		return string(fwfmp.AttemptStateHandoffOffered), true, nil
	case fwfmp.FrameworkEventFMPHandoffAccepted:
		return string(fwfmp.AttemptStateHandoffAccepted), true, nil
	case fwfmp.FrameworkEventFMPResumeCommitted:
		return string(fwfmp.AttemptStateCommittedRemote), true, nil
	case fwfmp.FrameworkEventFMPFenceIssued:
		return string(fwfmp.AttemptStateFenced), true, nil
	default:
		return "", false, nil
	}
}

func matchesFrameworkBinding(binding LineageBinding, payload map[string]any) bool {
	lineageID := strings.TrimSpace(stringValue(payload["lineage_id"]))
	attemptID := strings.TrimSpace(stringValue(payload["attempt_id"]))
	oldAttemptID := strings.TrimSpace(stringValue(payload["old_attempt"]))
	newAttemptID := strings.TrimSpace(stringValue(payload["new_attempt"]))
	if lineageID != "" && strings.EqualFold(binding.LineageID, lineageID) {
		return true
	}
	for _, candidate := range []string{attemptID, oldAttemptID, newAttemptID} {
		if candidate != "" && strings.EqualFold(binding.AttemptID, candidate) {
			return true
		}
	}
	return false
}

func applyBridgeState(binding LineageBinding, payload map[string]any, state string) (string, bool) {
	switch state {
	case string(fwfmp.AttemptStateCommittedRemote):
		oldAttemptID := strings.TrimSpace(stringValue(payload["old_attempt"]))
		if oldAttemptID != "" && strings.EqualFold(binding.AttemptID, oldAttemptID) {
			return state, true
		}
		newAttemptID := strings.TrimSpace(stringValue(payload["new_attempt"]))
		if newAttemptID != "" && strings.EqualFold(binding.AttemptID, newAttemptID) {
			return string(fwfmp.AttemptStateRunning), true
		}
		return binding.State, false
	case string(fwfmp.AttemptStateFenced):
		attemptID := firstNonEmpty(stringValue(payload["attempt_id"]), stringValue(payload["old_attempt"]))
		if attemptID != "" && strings.EqualFold(binding.AttemptID, attemptID) {
			return state, true
		}
		return binding.State, false
	default:
		if lineageID := strings.TrimSpace(stringValue(payload["lineage_id"])); lineageID != "" && strings.EqualFold(binding.LineageID, lineageID) {
			return state, true
		}
		return binding.State, false
	}
}

func bridgeMessageForEvent(eventType string) string {
	switch eventType {
	case fwfmp.FrameworkEventFMPHandoffOffered:
		return "fmp handoff offered"
	case fwfmp.FrameworkEventFMPHandoffAccepted:
		return "fmp handoff accepted"
	case fwfmp.FrameworkEventFMPResumeCommitted:
		return "fmp resume committed"
	case fwfmp.FrameworkEventFMPFenceIssued:
		return "fmp fence issued"
	default:
		return "fmp lifecycle event"
	}
}

func stringValue(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

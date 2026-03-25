package nexus

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	memdb "github.com/lexcodex/relurpify/framework/memory/db"
	fwfmp "github.com/lexcodex/relurpify/framework/middleware/fmp"
	"github.com/lexcodex/relurpify/named/rex/reconcile"
	rexstate "github.com/lexcodex/relurpify/named/rex/state"
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
	Service       *fwfmp.Service
	WorkflowStore *memdb.SQLiteWorkflowStateStore
	RuntimeID     string
	Now           func() time.Time
}

func (b *LineageBridge) HandleFrameworkEvent(ctx context.Context, frameworkEvent core.FrameworkEvent) error {
	if b == nil || b.WorkflowStore == nil {
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
		if err := b.WorkflowStore.AppendEvent(ctx, memory.WorkflowEventRecord{
			EventID:    fmt.Sprintf("%s:%d", match.RunID, frameworkEvent.Seq),
			WorkflowID: match.WorkflowID,
			RunID:      match.RunID,
			EventType:  frameworkEvent.Type,
			Message:    bridgeMessageForEvent(frameworkEvent.Type),
			Metadata:   payload,
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

func (b *LineageBridge) BeforeExecute(ctx context.Context, workflowID, runID string, task *core.Task, state *core.Context) error {
	if b == nil || b.WorkflowStore == nil {
		return nil
	}
	now := b.nowUTC()
	if err := b.persistTaskRequest(ctx, workflowID, runID, task, state, now); err != nil {
		return err
	}
	binding, err := b.ensureBinding(ctx, workflowID, runID, task, state, now)
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
	record := core.AttemptRecord{
		AttemptID:        binding.AttemptID,
		LineageID:        binding.LineageID,
		RuntimeID:        binding.RuntimeID,
		State:            core.AttemptStateRunning,
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
	state.Set("fmp.lineage_id", binding.LineageID)
	state.Set("fmp.attempt_id", binding.AttemptID)
	state.Set("rex.fmp_lineage_id", binding.LineageID)
	state.Set("rex.fmp_attempt_id", binding.AttemptID)
	return b.persistBinding(ctx, workflowID, runID, *binding)
}

func (b *LineageBridge) AfterExecute(ctx context.Context, workflowID, runID string, _ *core.Task, state *core.Context, _ *core.Result, execErr error) error {
	if b == nil || b.WorkflowStore == nil || b.Service == nil || b.Service.Ownership == nil {
		return nil
	}
	binding, err := b.bindingFromState(ctx, workflowID, runID, state)
	if err != nil || binding == nil {
		return err
	}
	attempt, ok, err := b.Service.Ownership.GetAttempt(ctx, binding.AttemptID)
	if err != nil || !ok {
		return err
	}
	attempt.LastProgressTime = b.nowUTC()
	if execErr != nil {
		attempt.State = core.AttemptStateFailed
		binding.State = string(core.AttemptStateFailed)
	} else {
		attempt.State = core.AttemptStateCompleted
		binding.State = string(core.AttemptStateCompleted)
	}
	if err := b.Service.Ownership.UpsertAttempt(ctx, *attempt); err != nil {
		return err
	}
	binding.UpdatedAt = attempt.LastProgressTime
	return b.persistBinding(ctx, workflowID, runID, *binding)
}

func (b *LineageBridge) ensureBinding(ctx context.Context, workflowID, runID string, task *core.Task, state *core.Context, now time.Time) (*LineageBinding, error) {
	if state != nil {
		lineageID := strings.TrimSpace(state.GetString("fmp.lineage_id"))
		if lineageID == "" {
			lineageID = strings.TrimSpace(state.GetString("rex.fmp_lineage_id"))
		}
		if lineageID != "" {
			attemptID := strings.TrimSpace(state.GetString("fmp.attempt_id"))
			if attemptID == "" {
				attemptID = strings.TrimSpace(state.GetString("rex.fmp_attempt_id"))
			}
			if attemptID == "" {
				attemptID = runID
			}
			return &LineageBinding{
				LineageID: lineageID,
				AttemptID: attemptID,
				RuntimeID: b.runtimeID(),
				SessionID: strings.TrimSpace(state.GetString("gateway.session_id")),
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
	sessionID := sessionIDFromState(state, task)
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
		SensitivityClass:         sensitivityFromState(state),
		AllowedFederationTargets: federationTargetsFromState(state),
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
	artifacts, err := b.WorkflowStore.ListWorkflowArtifacts(ctx, workflowID, runID)
	if err != nil {
		return nil, err
	}
	for _, artifact := range artifacts {
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

func (b *LineageBridge) bindingFromState(ctx context.Context, workflowID, runID string, state *core.Context) (*LineageBinding, error) {
	if state != nil {
		lineageID := strings.TrimSpace(state.GetString("fmp.lineage_id"))
		attemptID := strings.TrimSpace(state.GetString("fmp.attempt_id"))
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

func (b *LineageBridge) persistTaskRequest(ctx context.Context, workflowID, runID string, task *core.Task, state *core.Context, now time.Time) error {
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
		"state": state.StateSnapshot(),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return b.WorkflowStore.UpsertWorkflowArtifact(ctx, memory.WorkflowArtifactRecord{
		ArtifactID:      runID + ":task-request",
		WorkflowID:      workflowID,
		RunID:           runID,
		Kind:            "rex.task_request",
		ContentType:     "application/json",
		StorageKind:     memory.ArtifactStorageInline,
		SummaryText:     "rex task request",
		InlineRawText:   string(raw),
		SummaryMetadata: map[string]any{"session_id": sessionIDFromState(state, task)},
		CreatedAt:       now,
	})
}

func (b *LineageBridge) persistBinding(ctx context.Context, workflowID, runID string, binding LineageBinding) error {
	raw, err := json.Marshal(binding)
	if err != nil {
		return err
	}
	return b.WorkflowStore.UpsertWorkflowArtifact(ctx, memory.WorkflowArtifactRecord{
		ArtifactID:      runID + ":fmp-lineage",
		WorkflowID:      workflowID,
		RunID:           runID,
		Kind:            "rex.fmp_lineage",
		ContentType:     "application/json",
		StorageKind:     memory.ArtifactStorageInline,
		SummaryText:     "rex fmp lineage binding",
		InlineRawText:   string(raw),
		SummaryMetadata: map[string]any{"lineage_id": binding.LineageID, "attempt_id": binding.AttemptID, "state": binding.State},
		CreatedAt:       binding.UpdatedAt,
	})
}

// ApplyReconciliationOutcome updates FMP attempt state based on a reconciliation outcome.
// This is called when Rex reconciliation completes for a workflow with an FMP lineage.
func (b *LineageBridge) ApplyReconciliationOutcome(ctx context.Context, workflowID, runID string, outcome *reconcile.Record) error {
	if b == nil || b.Service == nil || b.Service.Ownership == nil || outcome == nil {
		return nil
	}

	binding, err := b.bindingFromState(ctx, workflowID, runID, nil)
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

func (b *LineageBridge) findBindings(ctx context.Context, payload map[string]any) ([]matchedBinding, error) {
	workflows, err := b.WorkflowStore.ListWorkflows(ctx, 512)
	if err != nil {
		return nil, err
	}
	var out []matchedBinding
	for _, workflow := range workflows {
		artifacts, err := b.WorkflowStore.ListWorkflowArtifacts(ctx, workflow.WorkflowID, "")
		if err != nil {
			return nil, err
		}
		for _, artifact := range artifacts {
			if artifact.Kind != "rex.fmp_lineage" || strings.TrimSpace(artifact.InlineRawText) == "" {
				continue
			}
			var binding LineageBinding
			if err := json.Unmarshal([]byte(artifact.InlineRawText), &binding); err != nil {
				return nil, err
			}
			if matchesFrameworkBinding(binding, payload) {
				out = append(out, matchedBinding{
					WorkflowID: workflow.WorkflowID,
					RunID:      firstNonEmpty(artifact.RunID, binding.AttemptID),
					Binding:    binding,
				})
			}
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

func sessionIDFromState(state *core.Context, task *core.Task) string {
	if state != nil {
		for _, key := range []string{"gateway.session_id", "session_id"} {
			if value := strings.TrimSpace(state.GetString(key)); value != "" {
				return value
			}
		}
	}
	if task != nil {
		for _, key := range []string{"session_id", "gateway.session_id"} {
			if value := strings.TrimSpace(stringValue(task.Context[key])); value != "" {
				return value
			}
		}
	}
	return ""
}

func sensitivityFromState(state *core.Context) core.SensitivityClass {
	if state == nil {
		return core.SensitivityClassModerate
	}
	value := core.SensitivityClass(strings.TrimSpace(state.GetString("fmp.sensitivity_class")))
	if value == "" {
		return core.SensitivityClassModerate
	}
	return value
}

func federationTargetsFromState(state *core.Context) []string {
	if state == nil {
		return nil
	}
	raw, ok := state.Get("fmp.allowed_federation_targets")
	if !ok {
		return nil
	}
	values, ok := raw.([]string)
	if !ok {
		return nil
	}
	return append([]string(nil), values...)
}

func defaultCapabilityEnvelope() core.CapabilityEnvelope {
	return core.CapabilityEnvelope{
		AllowedCapabilityIDs: []string{
			string(core.CapabilityPlan),
			string(core.CapabilityExecute),
			string(core.CapabilityCode),
			string(core.CapabilityExplain),
			string(core.CapabilityHumanInLoop),
		},
		AllowedTaskClasses: []string{"agent.run"},
		AllowChildTasks:    true,
		AllowOnwardExport:  true,
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
	case core.FrameworkEventFMPHandoffOffered:
		return string(core.AttemptStateHandoffOffered), true, nil
	case core.FrameworkEventFMPHandoffAccepted:
		return string(core.AttemptStateHandoffAccepted), true, nil
	case core.FrameworkEventFMPResumeCommitted:
		return string(core.AttemptStateCommittedRemote), true, nil
	case core.FrameworkEventFMPFenceIssued:
		return string(core.AttemptStateFenced), true, nil
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
	case string(core.AttemptStateCommittedRemote):
		oldAttemptID := strings.TrimSpace(stringValue(payload["old_attempt"]))
		if oldAttemptID != "" && strings.EqualFold(binding.AttemptID, oldAttemptID) {
			return state, true
		}
		newAttemptID := strings.TrimSpace(stringValue(payload["new_attempt"]))
		if newAttemptID != "" && strings.EqualFold(binding.AttemptID, newAttemptID) {
			return string(core.AttemptStateRunning), true
		}
		return binding.State, false
	case string(core.AttemptStateFenced):
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
	case core.FrameworkEventFMPHandoffOffered:
		return "fmp handoff offered"
	case core.FrameworkEventFMPHandoffAccepted:
		return "fmp handoff accepted"
	case core.FrameworkEventFMPResumeCommitted:
		return "fmp resume committed"
	case core.FrameworkEventFMPFenceIssued:
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

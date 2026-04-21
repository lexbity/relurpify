package architect

import (
	"context"
	"fmt"
	"strings"
	"time"

	reactpkg "codeburg.org/lexbit/relurpify/agents/react"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
	"codeburg.org/lexbit/relurpify/framework/memory/db"
)

func (a *ArchitectAgent) persistStepSecurityEvents(ctx context.Context, store *db.SQLiteWorkflowStateStore, workflowID, runID, stepID string, stepState *core.Context, result *core.Result, createdAt time.Time) {
	if store == nil {
		return
	}
	var rawEnvelope any
	var ok bool
	if stepState != nil {
		rawEnvelope, ok = stepState.Get("react.last_tool_result_envelope")
	}
	if (!ok || rawEnvelope == nil) && stepState != nil {
		if rawLast, found := stepState.Get("react.last_result"); found && rawLast != nil {
			if lastResult, typed := rawLast.(*core.Result); typed && lastResult != nil && lastResult.Metadata != nil {
				rawEnvelope, ok = lastResult.Metadata["capability_result"]
			}
		}
	}
	if (!ok || rawEnvelope == nil) && result != nil && result.Metadata != nil {
		rawEnvelope, ok = result.Metadata["capability_result"]
	}
	if !ok || rawEnvelope == nil {
		if stepState == nil {
			return
		}
		rawObs, found := stepState.Get("react.tool_observations")
		if !found || rawObs == nil {
			return
		}
		var metadata map[string]any
		var toolName string
		var message string
		switch observations := rawObs.(type) {
		case []reactpkg.ToolObservation:
			if len(observations) == 0 {
				return
			}
			last := observations[len(observations)-1]
			toolName = strings.TrimSpace(last.Tool)
			if toolName == "" {
				return
			}
			message = fmt.Sprintf("Capability %s invoked during workflow step execution.", last.Tool)
			metadata = map[string]any{
				"capability_id": "tool:" + last.Tool,
				"capability":    last.Tool,
				"phase":         last.Phase,
				"success":       last.Success,
			}
		case map[string]any:
			toolName = strings.TrimSpace(fmt.Sprint(observations["last_tool"]))
			if toolName == "" || toolName == "<nil>" {
				return
			}
			message = fmt.Sprintf("Capability %s invoked during workflow step execution.", toolName)
			metadata = map[string]any{
				"capability_id": "tool:" + toolName,
				"capability":    toolName,
			}
			if success, exists := observations["last_success"]; exists {
				metadata["success"] = success
			}
		default:
			return
		}
		_ = store.AppendEvent(ctx, memory.WorkflowEventRecord{
			EventID:    architectRecordID("security_event"),
			WorkflowID: workflowID,
			RunID:      runID,
			StepID:     stepID,
			EventType:  "security.capability_invoked",
			Message:    message,
			Metadata:   metadata,
			CreatedAt:  createdAt,
		})
		return
	}
	envelope, ok := rawEnvelope.(*core.CapabilityResultEnvelope)
	if !ok || envelope == nil {
		return
	}
	metadata := map[string]any{
		"capability_id": envelope.Descriptor.ID,
		"capability":    envelope.Descriptor.Name,
		"kind":          string(envelope.Descriptor.Kind),
		"trust_class":   string(envelope.Descriptor.TrustClass),
		"insertion":     string(envelope.Insertion.Action),
	}
	if envelope.Policy != nil {
		metadata["policy_snapshot_id"] = envelope.Policy.ID
	}
	if envelope.Descriptor.Source.ProviderID != "" {
		metadata["provider_id"] = envelope.Descriptor.Source.ProviderID
	}
	if envelope.Descriptor.Source.SessionID != "" {
		metadata["session_id"] = envelope.Descriptor.Source.SessionID
	}
	if envelope.Approval != nil && envelope.Approval.TargetResource != "" {
		metadata["target_resource"] = envelope.Approval.TargetResource
	}
	_ = store.AppendEvent(ctx, memory.WorkflowEventRecord{
		EventID:    architectRecordID("security_event"),
		WorkflowID: workflowID,
		RunID:      runID,
		StepID:     stepID,
		EventType:  "security.insertion_decision",
		Message:    fmt.Sprintf("Capability %s insertion resolved as %s.", envelope.Descriptor.Name, envelope.Insertion.Action),
		Metadata:   metadata,
		CreatedAt:  createdAt,
	})
}

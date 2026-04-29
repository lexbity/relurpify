package nexus

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"codeburg.org/lexbit/relurpify/named/rex/store"
	fwfmp "codeburg.org/lexbit/relurpify/relurpnet/fmp"
)

var _ fwfmp.RuntimeSnapshotStore = (*SnapshotStore)(nil)

// SnapshotStore exposes rex workflow state through the FMP runtime snapshot seam.
type SnapshotStore struct {
	WorkflowStore *store.SQLiteWorkflowStore
}

func (s SnapshotStore) QueryWorkflowRuntime(ctx context.Context, workflowID, runID string) (map[string]any, error) {
	if s.WorkflowStore == nil {
		return nil, fmt.Errorf("workflow store unavailable")
	}
	payload := map[string]any{
		"workflow_id": workflowID,
		"run_id":      runID,
	}
	workflow, ok, err := s.WorkflowStore.GetWorkflow(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	if ok {
		payload["workflow"] = workflow
		payload["task"] = map[string]any{
			"id":          workflow.TaskID,
			"type":        workflow.TaskType,
			"instruction": workflow.Instruction,
			"context": map[string]any{
				"workflow_id": workflowID,
				"run_id":      runID,
			},
			"metadata": workflow.Metadata,
		}
	}
	if strings.TrimSpace(runID) != "" {
		run, ok, err := s.WorkflowStore.GetRun(ctx, runID)
		if err != nil {
			return nil, err
		}
		if ok {
			payload["run"] = run
		}
	}
	if binding, ok, err := s.WorkflowStore.GetLineageBinding(ctx, workflowID, runID); err != nil {
		return nil, err
	} else if ok {
		payload["lineage_binding"] = binding
	}
	artifacts, err := s.WorkflowStore.ListWorkflowArtifacts(ctx, workflowID, runID)
	if err != nil {
		return nil, err
	}
	payload["artifact_summaries"] = summarizeArtifacts(artifacts)
	for _, artifact := range artifacts {
		switch artifact.Kind {
		case "rex.task_request":
			if value := decodeArtifactJSON(artifact); value != nil {
				payload["task_request"] = value
				if task, ok := value["task"].(map[string]any); ok {
					payload["task"] = task
				}
				if state, ok := value["state"].(map[string]any); ok {
					payload["state"] = state
				}
			}
		case "rex.proof_surface":
			if value := decodeArtifactJSON(artifact); value != nil {
				payload["proof_surface"] = value
			}
		case "rex.action_log":
			if value := decodeArtifactJSON(artifact); value != nil {
				payload["action_log"] = value
			}
		case "rex.completion":
			if value := decodeArtifactJSON(artifact); value != nil {
				payload["completion"] = value
			}
		case "rex.verification":
			if value := decodeArtifactJSON(artifact); value != nil {
				payload["verification_evidence"] = value
			}
		case "rex.fmp_lineage":
			if _, ok := payload["lineage_binding"]; !ok {
				if value := decodeArtifactJSON(artifact); value != nil {
					payload["lineage_binding"] = value
				}
			}
		}
	}
	if _, ok := payload["state"]; !ok {
		payload["state"] = map[string]any{
			"workflow_id": workflowID,
			"run_id":      runID,
		}
	}
	events, err := s.WorkflowStore.ListEvents(ctx, workflowID, 64)
	if err != nil {
		return nil, err
	}
	payload["events"] = events
	payload["completed_step_ids"] = completedStepIDs(events)
	return payload, nil
}

func summarizeArtifacts(records []store.WorkflowArtifactRecord) []map[string]any {
	out := make([]map[string]any, 0, len(records))
	for _, record := range records {
		out = append(out, map[string]any{
			"artifact_id":        record.ArtifactID,
			"kind":               record.Kind,
			"content_type":       record.ContentType,
			"storage_kind":       record.StorageKind,
			"summary_text":       record.SummaryText,
			"summary_metadata":   record.SummaryMetadata,
			"raw_size_bytes":     record.RawSizeBytes,
			"compression_method": record.CompressionMethod,
			"created_at":         record.CreatedAt,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return fmt.Sprint(out[i]["artifact_id"]) < fmt.Sprint(out[j]["artifact_id"])
	})
	return out
}

func decodeArtifactJSON(record store.WorkflowArtifactRecord) map[string]any {
	if strings.TrimSpace(record.InlineRawText) == "" {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(record.InlineRawText), &out); err != nil {
		return nil
	}
	return out
}

func completedStepIDs(events []store.WorkflowEventRecord) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0)
	for _, event := range events {
		if strings.TrimSpace(event.EventID) == "" {
			continue
		}
		if !strings.Contains(strings.ToLower(event.EventType), "completed") && !strings.Contains(strings.ToLower(event.Message), "completed") {
			continue
		}
		if _, ok := seen[event.EventID]; ok {
			continue
		}
		seen[event.EventID] = struct{}{}
		out = append(out, event.EventID)
	}
	sort.Strings(out)
	return out
}

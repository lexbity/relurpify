package memory

import (
	"context"
	"fmt"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/graph"
)

type workflowArtifactSink struct {
	store      WorkflowStateStore
	workflowID string
	runID      string
}

// AdaptWorkflowStateStoreArtifactSink writes graph artifacts into workflow artifacts.
func AdaptWorkflowStateStoreArtifactSink(store WorkflowStateStore, workflowID, runID string) graph.ArtifactSink {
	if store == nil || strings.TrimSpace(workflowID) == "" {
		return nil
	}
	return workflowArtifactSink{
		store:      store,
		workflowID: strings.TrimSpace(workflowID),
		runID:      strings.TrimSpace(runID),
	}
}

func (s workflowArtifactSink) SaveArtifact(ctx context.Context, artifact graph.ArtifactRecord) error {
	return s.store.UpsertWorkflowArtifact(ctx, WorkflowArtifactRecord{
		ArtifactID:      artifact.ArtifactID,
		WorkflowID:      s.workflowID,
		RunID:           s.runID,
		Kind:            artifact.Kind,
		ContentType:     artifact.ContentType,
		StorageKind:     ArtifactStorageInline,
		SummaryText:     artifact.Summary,
		SummaryMetadata: cloneMapAny(artifact.Metadata),
		InlineRawText:   artifact.RawText,
		RawRef:          "",
		RawSizeBytes:    artifact.RawSizeBytes,
		CreatedAt:       chooseCreatedAt(artifact.CreatedAt),
	})
}

type workflowAuditSink struct {
	store      WorkflowStateStore
	workflowID string
	runID      string
}

// AdaptWorkflowStateStoreAuditSink records persistence decisions as workflow events.
func AdaptWorkflowStateStoreAuditSink(store WorkflowStateStore, workflowID, runID string) graph.PersistenceAuditSink {
	if store == nil || strings.TrimSpace(workflowID) == "" {
		return nil
	}
	return workflowAuditSink{
		store:      store,
		workflowID: strings.TrimSpace(workflowID),
		runID:      strings.TrimSpace(runID),
	}
}

func (s workflowAuditSink) RecordPersistence(ctx context.Context, record graph.PersistenceAuditRecord) error {
	return s.store.AppendEvent(ctx, WorkflowEventRecord{
		EventID:    fmt.Sprintf("persist_%s_%d", strings.TrimSpace(record.SubjectID), time.Now().UTC().UnixNano()),
		WorkflowID: s.workflowID,
		RunID:      s.runID,
		EventType:  "graph.persistence",
		Message:    string(record.Action),
		Metadata: map[string]any{
			"reason":         record.Reason,
			"summary":        record.Summary,
			"memory_class":   string(record.MemoryClass),
			"scope":          record.Scope,
			"subject_id":     record.SubjectID,
			"subject_type":   record.SubjectType,
			"origin_node_id": record.OriginNodeID,
			"policy_name":    record.PolicyName,
			"metadata":       cloneMapAny(record.Metadata),
		},
		CreatedAt: chooseCreatedAt(record.CreatedAt),
	})
}

func chooseCreatedAt(value time.Time) time.Time {
	if value.IsZero() {
		return time.Now().UTC()
	}
	return value
}

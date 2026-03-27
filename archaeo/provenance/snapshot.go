package provenance

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeoevents "github.com/lexcodex/relurpify/archaeo/events"
	"github.com/lexcodex/relurpify/archaeo/internal/storeutil"
	"github.com/lexcodex/relurpify/framework/memory"
)

const (
	provenanceSnapshotArtifactKind = "archaeo_provenance_snapshot"
	requestArtifactKind            = "archaeo_request"
	planVersionArtifactKind        = "archaeo_living_plan_version"
	explorationSessionArtifactKind = "archaeo_exploration_session"
	deferredArtifactKind           = "archaeo_deferred_draft"
	convergenceArtifactKind        = "archaeo_convergence_record"
	decisionArtifactKind           = "archaeo_decision_record"
)

type snapshotEnvelope struct {
	Record *archaeodomain.ProvenanceRecord `json:"record"`

	WorkspaceID          string     `json:"workspace_id,omitempty"`
	LatestMutationAt     *time.Time `json:"latest_mutation_at,omitempty"`
	LatestRequestAt      *time.Time `json:"latest_request_at,omitempty"`
	LatestPlanArtifactAt *time.Time `json:"latest_plan_artifact_at,omitempty"`
	LatestDeferredAt     *time.Time `json:"latest_deferred_at,omitempty"`
	LatestConvergenceAt  *time.Time `json:"latest_convergence_at,omitempty"`
	LatestDecisionAt     *time.Time `json:"latest_decision_at,omitempty"`
	ActivePlanVersion    *int       `json:"active_plan_version,omitempty"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

func loadSnapshot(ctx context.Context, store memory.WorkflowStateStore, workflowID string) (*snapshotEnvelope, error) {
	if artifact, ok, err := storeutil.WorkflowArtifactByID(ctx, store, provenanceSnapshotArtifactID(workflowID)); err != nil {
		return nil, err
	} else if ok && artifact != nil && artifact.Kind == provenanceSnapshotArtifactKind {
		var envelope snapshotEnvelope
		if err := json.Unmarshal([]byte(artifact.InlineRawText), &envelope); err != nil {
			return nil, err
		}
		return &envelope, nil
	}
	return nil, nil
}

func saveSnapshot(ctx context.Context, store memory.WorkflowStateStore, workflowID string, envelope *snapshotEnvelope) error {
	if store == nil || strings.TrimSpace(workflowID) == "" || envelope == nil {
		return nil
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	return store.UpsertWorkflowArtifact(ctx, memory.WorkflowArtifactRecord{
		ArtifactID:      provenanceSnapshotArtifactID(workflowID),
		WorkflowID:      strings.TrimSpace(workflowID),
		Kind:            provenanceSnapshotArtifactKind,
		ContentType:     "application/json",
		StorageKind:     memory.ArtifactStorageInline,
		SummaryText:     "provenance snapshot",
		SummaryMetadata: map[string]any{"workspace_id": envelope.WorkspaceID},
		InlineRawText:   string(raw),
		CreatedAt:       ensureSnapshotTime(envelope.UpdatedAt),
	})
}

func snapshotFresh(ctx context.Context, store memory.WorkflowStateStore, workflowID string, envelope *snapshotEnvelope) (bool, error) {
	if store == nil || envelope == nil {
		return false, nil
	}
	log := archaeoevents.WorkflowLog{Store: store}
	if latestMutation, ok, err := log.LatestRecordByTypes(ctx, workflowID, archaeoevents.EventMutationRecorded); err != nil {
		return false, err
	} else if !sameTimePointer(envelope.LatestMutationAt, eventTime(latestMutation, ok)) {
		return false, nil
	}
	if latestRequest, ok, err := storeutil.LatestWorkflowArtifactByKind(ctx, store, workflowID, "", requestArtifactKind); err != nil {
		return false, err
	} else if !sameTimePointer(envelope.LatestRequestAt, artifactTime(latestRequest, ok)) {
		return false, nil
	}
	if latestPlan, ok, err := storeutil.LatestWorkflowArtifactByKind(ctx, store, workflowID, "", planVersionArtifactKind); err != nil {
		return false, err
	} else if !sameTimePointer(envelope.LatestPlanArtifactAt, artifactTime(latestPlan, ok)) {
		return false, nil
	}
	workspaceID := strings.TrimSpace(envelope.WorkspaceID)
	if workspaceID == "" {
		return true, nil
	}
	if latestDeferred, ok, err := storeutil.LatestWorkflowArtifactByKindAndWorkspace(ctx, store, "", "", deferredArtifactKind, workspaceID); err != nil {
		return false, err
	} else if !sameTimePointer(envelope.LatestDeferredAt, artifactTime(latestDeferred, ok)) {
		return false, nil
	}
	if latestConvergence, ok, err := storeutil.LatestWorkflowArtifactByKindAndWorkspace(ctx, store, "", "", convergenceArtifactKind, workspaceID); err != nil {
		return false, err
	} else if !sameTimePointer(envelope.LatestConvergenceAt, artifactTime(latestConvergence, ok)) {
		return false, nil
	}
	if latestDecision, ok, err := storeutil.LatestWorkflowArtifactByKindAndWorkspace(ctx, store, "", "", decisionArtifactKind, workspaceID); err != nil {
		return false, err
	} else if !sameTimePointer(envelope.LatestDecisionAt, artifactTime(latestDecision, ok)) {
		return false, nil
	}
	return true, nil
}

func buildSnapshotEnvelope(ctx context.Context, store memory.WorkflowStateStore, workflowID, workspaceID string, activePlanVersion *int, record *archaeodomain.ProvenanceRecord) (*snapshotEnvelope, error) {
	log := archaeoevents.WorkflowLog{Store: store}
	envelope := &snapshotEnvelope{
		Record:            record,
		WorkspaceID:       strings.TrimSpace(workspaceID),
		ActivePlanVersion: cloneInt(activePlanVersion),
		UpdatedAt:         time.Now().UTC(),
	}
	if latestMutation, ok, err := log.LatestRecordByTypes(ctx, workflowID, archaeoevents.EventMutationRecorded); err != nil {
		return nil, err
	} else {
		envelope.LatestMutationAt = eventTime(latestMutation, ok)
	}
	if latestRequest, ok, err := storeutil.LatestWorkflowArtifactByKind(ctx, store, workflowID, "", requestArtifactKind); err != nil {
		return nil, err
	} else {
		envelope.LatestRequestAt = artifactTime(latestRequest, ok)
	}
	if latestPlan, ok, err := storeutil.LatestWorkflowArtifactByKind(ctx, store, workflowID, "", planVersionArtifactKind); err != nil {
		return nil, err
	} else {
		envelope.LatestPlanArtifactAt = artifactTime(latestPlan, ok)
	}
	if strings.TrimSpace(workspaceID) != "" {
		if latestDeferred, ok, err := storeutil.LatestWorkflowArtifactByKindAndWorkspace(ctx, store, "", "", deferredArtifactKind, workspaceID); err != nil {
			return nil, err
		} else {
			envelope.LatestDeferredAt = artifactTime(latestDeferred, ok)
		}
		if latestConvergence, ok, err := storeutil.LatestWorkflowArtifactByKindAndWorkspace(ctx, store, "", "", convergenceArtifactKind, workspaceID); err != nil {
			return nil, err
		} else {
			envelope.LatestConvergenceAt = artifactTime(latestConvergence, ok)
		}
		if latestDecision, ok, err := storeutil.LatestWorkflowArtifactByKindAndWorkspace(ctx, store, "", "", decisionArtifactKind, workspaceID); err != nil {
			return nil, err
		} else {
			envelope.LatestDecisionAt = artifactTime(latestDecision, ok)
		}
	}
	return envelope, nil
}

func provenanceSnapshotArtifactID(workflowID string) string {
	return "provenance-snapshot:" + strings.TrimSpace(workflowID)
}

func artifactTime(record *memory.WorkflowArtifactRecord, ok bool) *time.Time {
	if !ok || record == nil || record.CreatedAt.IsZero() {
		return nil
	}
	value := record.CreatedAt
	return &value
}

func eventTime(record *memory.WorkflowEventRecord, ok bool) *time.Time {
	if !ok || record == nil || record.CreatedAt.IsZero() {
		return nil
	}
	value := record.CreatedAt
	return &value
}

func sameTimePointer(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.UTC().Equal(right.UTC())
}

func ensureSnapshotTime(value time.Time) time.Time {
	if value.IsZero() {
		return time.Now().UTC()
	}
	return value.UTC()
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

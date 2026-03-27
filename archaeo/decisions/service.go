package decisions

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeoevents "github.com/lexcodex/relurpify/archaeo/events"
	"github.com/lexcodex/relurpify/archaeo/internal/keylock"
	"github.com/lexcodex/relurpify/archaeo/internal/storeutil"
	"github.com/lexcodex/relurpify/framework/memory"
)

const artifactKind = "archaeo_decision_record"

type CreateInput struct {
	WorkspaceID            string
	WorkflowID             string
	Kind                   archaeodomain.DecisionKind
	RelatedRequestID       string
	RelatedConvergenceID   string
	RelatedDeferredDraftID string
	RelatedPlanID          string
	RelatedPlanVersion     *int
	Validity               archaeodomain.RequestValidity
	Title                  string
	Summary                string
	CommentRefs            []string
	Metadata               map[string]any
}

type ResolveInput struct {
	WorkflowID  string
	RecordID    string
	Status      archaeodomain.DecisionStatus
	CommentRefs []string
	Metadata    map[string]any
}

type Service struct {
	Store memory.WorkflowStateStore
	Now   func() time.Time
	NewID func(string) string
}

var decisionMutationLocks keylock.Locker

func (s Service) Create(ctx context.Context, input CreateInput) (*archaeodomain.DecisionRecord, error) {
	var (
		record *archaeodomain.DecisionRecord
		err    error
	)
	err = decisionMutationLocks.With("workspace:"+strings.TrimSpace(input.WorkspaceID), func() error {
		if s.Store == nil || strings.TrimSpace(input.WorkflowID) == "" || strings.TrimSpace(input.WorkspaceID) == "" || input.Kind == "" {
			return nil
		}
		record = &archaeodomain.DecisionRecord{
			ID:                     s.newID("decision"),
			WorkspaceID:            strings.TrimSpace(input.WorkspaceID),
			WorkflowID:             strings.TrimSpace(input.WorkflowID),
			Kind:                   input.Kind,
			Status:                 archaeodomain.DecisionStatusOpen,
			RelatedRequestID:       strings.TrimSpace(input.RelatedRequestID),
			RelatedConvergenceID:   strings.TrimSpace(input.RelatedConvergenceID),
			RelatedDeferredDraftID: strings.TrimSpace(input.RelatedDeferredDraftID),
			RelatedPlanID:          strings.TrimSpace(input.RelatedPlanID),
			RelatedPlanVersion:     cloneInt(input.RelatedPlanVersion),
			Validity:               input.Validity,
			Title:                  strings.TrimSpace(input.Title),
			Summary:                strings.TrimSpace(input.Summary),
			CommentRefs:            mergeStrings(nil, input.CommentRefs),
			Metadata:               cloneMap(input.Metadata),
			CreatedAt:              s.now(),
			UpdatedAt:              s.now(),
		}
		if err := s.save(ctx, record); err != nil {
			return err
		}
		_ = archaeoevents.AppendWorkflowEvent(ctx, s.Store, record.WorkflowID, archaeoevents.EventDecisionRecordUpserted, firstNonEmpty(record.Title, record.Summary), decisionMetadata(*record), record.CreatedAt)
		return nil
	})
	return record, err
}

func (s Service) Resolve(ctx context.Context, input ResolveInput) (*archaeodomain.DecisionRecord, error) {
	var (
		record *archaeodomain.DecisionRecord
		err    error
	)
	err = decisionMutationLocks.With("workflow:"+strings.TrimSpace(input.WorkflowID), func() error {
		record, err = s.Load(ctx, input.WorkflowID, input.RecordID)
		if err != nil || record == nil {
			return err
		}
		now := s.now()
		record.Status = input.Status
		record.CommentRefs = mergeStrings(record.CommentRefs, input.CommentRefs)
		record.Metadata = mergeMap(record.Metadata, input.Metadata)
		record.ResolvedAt = &now
		record.UpdatedAt = now
		if err := s.save(ctx, record); err != nil {
			return err
		}
		_ = archaeoevents.AppendWorkflowEvent(ctx, s.Store, record.WorkflowID, archaeoevents.EventDecisionRecordUpserted, firstNonEmpty(record.Title, record.Summary), decisionMetadata(*record), now)
		return nil
	})
	return record, err
}

func (s Service) Load(ctx context.Context, workflowID, recordID string) (*archaeodomain.DecisionRecord, error) {
	if s.Store == nil || strings.TrimSpace(workflowID) == "" || strings.TrimSpace(recordID) == "" {
		return nil, nil
	}
	if artifact, ok, err := storeutil.WorkflowArtifactByID(ctx, s.Store, recordID); err != nil {
		return nil, err
	} else if ok && artifact != nil && artifact.Kind == artifactKind && strings.TrimSpace(artifact.WorkflowID) == strings.TrimSpace(workflowID) {
		var record archaeodomain.DecisionRecord
		if err := json.Unmarshal([]byte(artifact.InlineRawText), &record); err != nil {
			return nil, err
		}
		return &record, nil
	}
	return nil, nil
}

func (s Service) ListByWorkspace(ctx context.Context, workspaceID string) ([]archaeodomain.DecisionRecord, error) {
	if s.Store == nil || strings.TrimSpace(workspaceID) == "" {
		return nil, nil
	}
	artifacts, err := storeutil.ListWorkflowArtifactsByKindAndWorkspace(ctx, s.Store, "", "", artifactKind, workspaceID)
	if err != nil {
		return nil, err
	}
	out := make([]archaeodomain.DecisionRecord, 0, len(artifacts))
	for _, artifact := range artifacts {
		var record archaeodomain.DecisionRecord
		if err := json.Unmarshal([]byte(artifact.InlineRawText), &record); err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	return out, nil
}

func (s Service) IDsByWorkspace(ctx context.Context, workspaceID string) ([]string, error) {
	if s.Store == nil || strings.TrimSpace(workspaceID) == "" {
		return nil, nil
	}
	artifacts, err := storeutil.ListWorkflowArtifactsByKindAndWorkspace(ctx, s.Store, "", "", artifactKind, workspaceID)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		if id := strings.TrimSpace(artifact.ArtifactID); id != "" {
			out = append(out, id)
		}
	}
	return out, nil
}

func (s Service) SummaryByWorkspace(ctx context.Context, workspaceID string) (map[archaeodomain.DecisionStatus]int, error) {
	if s.Store == nil || strings.TrimSpace(workspaceID) == "" {
		return nil, nil
	}
	artifacts, err := storeutil.ListWorkflowArtifactsByKindAndWorkspace(ctx, s.Store, "", "", artifactKind, workspaceID)
	if err != nil {
		return nil, err
	}
	out := make(map[archaeodomain.DecisionStatus]int)
	for _, artifact := range artifacts {
		status := archaeodomain.DecisionStatus(strings.TrimSpace(metadataString(artifact.SummaryMetadata, "status")))
		out[status]++
	}
	return out, nil
}

func (s Service) save(ctx context.Context, record *archaeodomain.DecisionRecord) error {
	raw, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return s.Store.UpsertWorkflowArtifact(ctx, memory.WorkflowArtifactRecord{
		ArtifactID:      record.ID,
		WorkflowID:      record.WorkflowID,
		Kind:            artifactKind,
		ContentType:     "application/json",
		StorageKind:     memory.ArtifactStorageInline,
		SummaryText:     firstNonEmpty(record.Title, record.Summary),
		SummaryMetadata: map[string]any{"workspace_id": record.WorkspaceID, "kind": string(record.Kind), "status": string(record.Status)},
		InlineRawText:   string(raw),
		CreatedAt:       record.UpdatedAt,
	})
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s Service) newID(prefix string) string {
	if s.NewID != nil {
		return s.NewID(prefix)
	}
	return fmt.Sprintf("%s-%d", prefix, s.now().UnixNano())
}

func decisionMetadata(record archaeodomain.DecisionRecord) map[string]any {
	return map[string]any{
		"workspace_id":           record.WorkspaceID,
		"kind":                   string(record.Kind),
		"status":                 string(record.Status),
		"related_request_id":     record.RelatedRequestID,
		"related_convergence_id": record.RelatedConvergenceID,
		"related_deferred_id":    record.RelatedDeferredDraftID,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func mergeMap(existing, update map[string]any) map[string]any {
	if len(existing) == 0 && len(update) == 0 {
		return nil
	}
	out := cloneMap(existing)
	if out == nil {
		out = map[string]any{}
	}
	for key, value := range update {
		out[key] = value
	}
	return out
}

func mergeStrings(existing, update []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(existing)+len(update))
	for _, values := range [][]string{existing, update} {
		for _, value := range values {
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				if _, ok := seen[trimmed]; ok {
					continue
				}
				seen[trimmed] = struct{}{}
				out = append(out, trimmed)
			}
		}
	}
	return out
}

func metadataString(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	if value, ok := metadata[key].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

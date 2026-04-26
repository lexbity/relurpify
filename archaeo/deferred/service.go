package deferred

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	archaeodomain "codeburg.org/lexbit/relurpify/archaeo/domain"
	archaeoevents "codeburg.org/lexbit/relurpify/archaeo/events"
	"codeburg.org/lexbit/relurpify/archaeo/internal/storeutil"
	frameworkkeylock "codeburg.org/lexbit/relurpify/framework/keylock"
	"codeburg.org/lexbit/relurpify/framework/memory"
)

const artifactKind = "archaeo_deferred_draft"

type CreateInput struct {
	WorkspaceID        string
	WorkflowID         string
	ExplorationID      string
	PlanID             string
	PlanVersion        *int
	RequestID          string
	AmbiguityKey       string
	Title              string
	Description        string
	LinkedDraftVersion *int
	LinkedDraftPlanID  string
	CommentRefs        []string
	Metadata           map[string]any
}

type FinalizeInput struct {
	WorkflowID  string
	RecordID    string
	CommentRefs []string
	Metadata    map[string]any
}

type Service struct {
	Store memory.WorkflowStateStore
	Now   func() time.Time
	NewID func(string) string
}

var deferredMutationLocks frameworkkeylock.Locker

func (s Service) CreateOrUpdate(ctx context.Context, input CreateInput) (*archaeodomain.DeferredDraftRecord, error) {
	var (
		record *archaeodomain.DeferredDraftRecord
		err    error
	)
	err = deferredMutationLocks.With("workspace:"+strings.TrimSpace(input.WorkspaceID), func() error {
		if s.Store == nil || strings.TrimSpace(input.WorkflowID) == "" || strings.TrimSpace(input.WorkspaceID) == "" || strings.TrimSpace(input.AmbiguityKey) == "" {
			return nil
		}
		if existing, err := s.findOpenByKey(ctx, input.WorkflowID, input.WorkspaceID, input.AmbiguityKey); err != nil {
			return err
		} else if existing != nil {
			existing.Description = firstNonEmpty(input.Description, existing.Description)
			existing.Title = firstNonEmpty(input.Title, existing.Title)
			existing.PlanID = firstNonEmpty(input.PlanID, existing.PlanID)
			existing.RequestID = firstNonEmpty(input.RequestID, existing.RequestID)
			existing.ExplorationID = firstNonEmpty(input.ExplorationID, existing.ExplorationID)
			if input.PlanVersion != nil {
				existing.PlanVersion = cloneInt(input.PlanVersion)
			}
			if input.LinkedDraftVersion != nil {
				existing.LinkedDraftVersion = cloneInt(input.LinkedDraftVersion)
				existing.Status = archaeodomain.DeferredDraftFormed
			}
			existing.LinkedDraftPlanID = firstNonEmpty(input.LinkedDraftPlanID, existing.LinkedDraftPlanID)
			existing.CommentRefs = mergeStrings(existing.CommentRefs, input.CommentRefs)
			existing.Metadata = mergeMap(existing.Metadata, input.Metadata)
			existing.UpdatedAt = s.now()
			if err := s.save(ctx, existing); err != nil {
				return err
			}
			_ = archaeoevents.AppendWorkflowEvent(ctx, s.Store, existing.WorkflowID, archaeoevents.EventDeferredDraftUpserted, firstNonEmpty(existing.Title, existing.AmbiguityKey), deferredMetadata(*existing), existing.UpdatedAt)
			record = existing
			return nil
		}
		record = &archaeodomain.DeferredDraftRecord{
			ID:                 s.newID("deferred"),
			WorkspaceID:        strings.TrimSpace(input.WorkspaceID),
			WorkflowID:         strings.TrimSpace(input.WorkflowID),
			ExplorationID:      strings.TrimSpace(input.ExplorationID),
			PlanID:             strings.TrimSpace(input.PlanID),
			PlanVersion:        cloneInt(input.PlanVersion),
			RequestID:          strings.TrimSpace(input.RequestID),
			AmbiguityKey:       strings.TrimSpace(input.AmbiguityKey),
			Title:              strings.TrimSpace(input.Title),
			Description:        strings.TrimSpace(input.Description),
			Status:             archaeodomain.DeferredDraftPending,
			LinkedDraftVersion: cloneInt(input.LinkedDraftVersion),
			LinkedDraftPlanID:  strings.TrimSpace(input.LinkedDraftPlanID),
			CommentRefs:        append([]string(nil), input.CommentRefs...),
			Metadata:           cloneMap(input.Metadata),
			CreatedAt:          s.now(),
			UpdatedAt:          s.now(),
		}
		if record.LinkedDraftVersion != nil {
			record.Status = archaeodomain.DeferredDraftFormed
		}
		if err := s.save(ctx, record); err != nil {
			return err
		}
		_ = archaeoevents.AppendWorkflowEvent(ctx, s.Store, record.WorkflowID, archaeoevents.EventDeferredDraftUpserted, firstNonEmpty(record.Title, record.AmbiguityKey), deferredMetadata(*record), record.CreatedAt)
		return nil
	})
	return record, err
}

func (s Service) Finalize(ctx context.Context, input FinalizeInput) (*archaeodomain.DeferredDraftRecord, error) {
	var (
		record *archaeodomain.DeferredDraftRecord
		err    error
	)
	err = deferredMutationLocks.With("workflow:"+strings.TrimSpace(input.WorkflowID), func() error {
		record, err = s.Load(ctx, input.WorkflowID, input.RecordID)
		if err != nil || record == nil {
			return err
		}
		now := s.now()
		record.Status = archaeodomain.DeferredDraftFinalized
		record.CommentRefs = mergeStrings(record.CommentRefs, input.CommentRefs)
		record.Metadata = mergeMap(record.Metadata, input.Metadata)
		record.FinalizedAt = &now
		record.UpdatedAt = now
		if err := s.save(ctx, record); err != nil {
			return err
		}
		_ = archaeoevents.AppendWorkflowEvent(ctx, s.Store, record.WorkflowID, archaeoevents.EventDeferredDraftUpserted, firstNonEmpty(record.Title, record.AmbiguityKey), deferredMetadata(*record), now)
		return nil
	})
	return record, err
}

func (s Service) Load(ctx context.Context, workflowID, recordID string) (*archaeodomain.DeferredDraftRecord, error) {
	if s.Store == nil || strings.TrimSpace(workflowID) == "" || strings.TrimSpace(recordID) == "" {
		return nil, nil
	}
	if artifact, ok, err := storeutil.WorkflowArtifactByID(ctx, s.Store, recordID); err != nil {
		return nil, err
	} else if ok && artifact != nil && artifact.Kind == artifactKind && strings.TrimSpace(artifact.WorkflowID) == strings.TrimSpace(workflowID) {
		var record archaeodomain.DeferredDraftRecord
		if err := json.Unmarshal([]byte(artifact.InlineRawText), &record); err != nil {
			return nil, err
		}
		return &record, nil
	}
	return nil, nil
}

func (s Service) ListByWorkspace(ctx context.Context, workspaceID string) ([]archaeodomain.DeferredDraftRecord, error) {
	if s.Store == nil || strings.TrimSpace(workspaceID) == "" {
		return nil, nil
	}
	artifacts, err := storeutil.ListWorkflowArtifactsByKindAndWorkspace(ctx, s.Store, "", "", artifactKind, workspaceID)
	if err != nil {
		return nil, err
	}
	out := make([]archaeodomain.DeferredDraftRecord, 0, len(artifacts))
	for _, artifact := range artifacts {
		var record archaeodomain.DeferredDraftRecord
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

func (s Service) SummaryByWorkspace(ctx context.Context, workspaceID string) (map[archaeodomain.DeferredDraftStatus]int, error) {
	if s.Store == nil || strings.TrimSpace(workspaceID) == "" {
		return nil, nil
	}
	artifacts, err := storeutil.ListWorkflowArtifactsByKindAndWorkspace(ctx, s.Store, "", "", artifactKind, workspaceID)
	if err != nil {
		return nil, err
	}
	out := make(map[archaeodomain.DeferredDraftStatus]int)
	for _, artifact := range artifacts {
		status := archaeodomain.DeferredDraftStatus(strings.TrimSpace(metadataString(artifact.SummaryMetadata, "status")))
		out[status]++
	}
	return out, nil
}

func (s Service) findOpenByKey(ctx context.Context, workflowID, workspaceID, ambiguityKey string) (*archaeodomain.DeferredDraftRecord, error) {
	records, err := s.ListByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	for i := range records {
		record := records[i]
		if strings.TrimSpace(record.WorkflowID) != strings.TrimSpace(workflowID) || strings.TrimSpace(record.AmbiguityKey) != strings.TrimSpace(ambiguityKey) {
			continue
		}
		switch record.Status {
		case archaeodomain.DeferredDraftPending, archaeodomain.DeferredDraftFormed:
			return &record, nil
		}
	}
	return nil, nil
}

func (s Service) save(ctx context.Context, record *archaeodomain.DeferredDraftRecord) error {
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
		SummaryText:     firstNonEmpty(record.Title, record.AmbiguityKey),
		SummaryMetadata: map[string]any{"workspace_id": record.WorkspaceID, "ambiguity_key": record.AmbiguityKey, "status": string(record.Status)},
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

func deferredMetadata(record archaeodomain.DeferredDraftRecord) map[string]any {
	return map[string]any{
		"workspace_id":         record.WorkspaceID,
		"ambiguity_key":        record.AmbiguityKey,
		"status":               string(record.Status),
		"request_id":           record.RequestID,
		"linked_draft_version": record.LinkedDraftVersion,
		"linked_draft_plan_id": record.LinkedDraftPlanID,
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

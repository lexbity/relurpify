package convergence

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	archaeodomain "codeburg.org/lexbit/relurpify/archaeo/domain"
	archaeoevents "codeburg.org/lexbit/relurpify/archaeo/events"
	"codeburg.org/lexbit/relurpify/archaeo/internal/keylock"
	"codeburg.org/lexbit/relurpify/archaeo/internal/storeutil"
	"codeburg.org/lexbit/relurpify/framework/memory"
)

const artifactKind = "archaeo_convergence_record"
const currentArtifactKind = "archaeo_convergence_current"

type CreateInput struct {
	WorkspaceID        string
	WorkflowID         string
	ExplorationID      string
	PlanID             string
	PlanVersion        *int
	Question           string
	Title              string
	RelevantTensionIDs []string
	PendingLearningIDs []string
	AcceptedDebt       []string
	DeferredDraftIDs   []string
	ProvenanceRefs     []string
	CommentRefs        []string
	Metadata           map[string]any
}

type ResolveInput struct {
	WorkflowID string
	RecordID   string
	Resolution archaeodomain.ConvergenceResolution
}

type Service struct {
	Store memory.WorkflowStateStore
	Now   func() time.Time
	NewID func(string) string
}

var convergenceMutationLocks keylock.Locker

func (s Service) Create(ctx context.Context, input CreateInput) (*archaeodomain.ConvergenceRecord, error) {
	var (
		record *archaeodomain.ConvergenceRecord
		err    error
	)
	err = convergenceMutationLocks.With("workspace:"+strings.TrimSpace(input.WorkspaceID), func() error {
		if s.Store == nil || strings.TrimSpace(input.WorkflowID) == "" || strings.TrimSpace(input.WorkspaceID) == "" {
			return nil
		}
		record = &archaeodomain.ConvergenceRecord{
			ID:                 s.newID("convergence"),
			WorkspaceID:        strings.TrimSpace(input.WorkspaceID),
			WorkflowID:         strings.TrimSpace(input.WorkflowID),
			ExplorationID:      strings.TrimSpace(input.ExplorationID),
			PlanID:             strings.TrimSpace(input.PlanID),
			PlanVersion:        cloneInt(input.PlanVersion),
			Status:             archaeodomain.ConvergenceResolutionOpen,
			Question:           strings.TrimSpace(input.Question),
			Title:              strings.TrimSpace(input.Title),
			RelevantTensionIDs: mergeStrings(nil, input.RelevantTensionIDs),
			PendingLearningIDs: mergeStrings(nil, input.PendingLearningIDs),
			AcceptedDebt:       mergeStrings(nil, input.AcceptedDebt),
			DeferredDraftIDs:   mergeStrings(nil, input.DeferredDraftIDs),
			ProvenanceRefs:     mergeStrings(nil, input.ProvenanceRefs),
			CommentRefs:        mergeStrings(nil, input.CommentRefs),
			Metadata:           cloneMap(input.Metadata),
			CreatedAt:          s.now(),
			UpdatedAt:          s.now(),
		}
		if err := s.save(ctx, record); err != nil {
			return err
		}
		_ = s.saveCurrent(ctx, record.WorkflowID, record.WorkspaceID)
		_ = archaeoevents.AppendWorkflowEvent(ctx, s.Store, record.WorkflowID, archaeoevents.EventConvergenceRecordUpserted, firstNonEmpty(record.Title, record.Question), convergenceMetadata(*record), record.CreatedAt)
		return nil
	})
	return record, err
}

func (s Service) Resolve(ctx context.Context, input ResolveInput) (*archaeodomain.ConvergenceRecord, error) {
	var (
		record *archaeodomain.ConvergenceRecord
		err    error
	)
	err = convergenceMutationLocks.With("workflow:"+strings.TrimSpace(input.WorkflowID), func() error {
		record, err = s.Load(ctx, input.WorkflowID, input.RecordID)
		if err != nil || record == nil {
			return err
		}
		now := s.now()
		resolution := input.Resolution
		resolution.CommentRefs = mergeStrings(record.CommentRefs, resolution.CommentRefs)
		resolution.Metadata = mergeMap(record.Metadata, resolution.Metadata)
		resolution.ResolvedAt = &now
		record.Status = resolution.Status
		record.Resolution = &resolution
		record.CommentRefs = mergeStrings(record.CommentRefs, resolution.CommentRefs)
		record.UpdatedAt = now
		if err := s.save(ctx, record); err != nil {
			return err
		}
		_ = s.saveCurrent(ctx, record.WorkflowID, record.WorkspaceID)
		_ = archaeoevents.AppendWorkflowEvent(ctx, s.Store, record.WorkflowID, archaeoevents.EventConvergenceRecordUpserted, firstNonEmpty(record.Title, record.Question), convergenceMetadata(*record), now)
		return nil
	})
	return record, err
}

func (s Service) Load(ctx context.Context, workflowID, recordID string) (*archaeodomain.ConvergenceRecord, error) {
	if s.Store == nil || strings.TrimSpace(workflowID) == "" || strings.TrimSpace(recordID) == "" {
		return nil, nil
	}
	if artifact, ok, err := storeutil.WorkflowArtifactByID(ctx, s.Store, recordID); err != nil {
		return nil, err
	} else if ok && artifact != nil && artifact.Kind == artifactKind && strings.TrimSpace(artifact.WorkflowID) == strings.TrimSpace(workflowID) {
		var record archaeodomain.ConvergenceRecord
		if err := json.Unmarshal([]byte(artifact.InlineRawText), &record); err != nil {
			return nil, err
		}
		return &record, nil
	}
	return nil, nil
}

func (s Service) ListByWorkspace(ctx context.Context, workspaceID string) ([]archaeodomain.ConvergenceRecord, error) {
	if s.Store == nil || strings.TrimSpace(workspaceID) == "" {
		return nil, nil
	}
	artifacts, err := storeutil.ListWorkflowArtifactsByKindAndWorkspace(ctx, s.Store, "", "", artifactKind, workspaceID)
	if err != nil {
		return nil, err
	}
	out := make([]archaeodomain.ConvergenceRecord, 0, len(artifacts))
	for _, artifact := range artifacts {
		var record archaeodomain.ConvergenceRecord
		if err := json.Unmarshal([]byte(artifact.InlineRawText), &record); err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (s Service) CurrentByWorkspace(ctx context.Context, workspaceID string) (*archaeodomain.WorkspaceConvergenceProjection, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if s.Store == nil || workspaceID == "" {
		return nil, nil
	}
	if artifact, ok, err := storeutil.LatestWorkflowArtifactByKindAndWorkspace(ctx, s.Store, "", "", currentArtifactKind, workspaceID); err != nil {
		return nil, err
	} else if ok && artifact != nil {
		var proj archaeodomain.WorkspaceConvergenceProjection
		if err := json.Unmarshal([]byte(artifact.InlineRawText), &proj); err != nil {
			return nil, err
		}
		return &proj, nil
	}
	records, err := s.ListByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	proj := buildCurrentProjection(records, workspaceID)
	return proj, nil
}

func (s Service) RebuildCurrent(ctx context.Context, workspaceID string) (*archaeodomain.WorkspaceConvergenceProjection, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if s.Store == nil || workspaceID == "" {
		return nil, nil
	}
	records, err := s.ListByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	proj := buildCurrentProjection(records, workspaceID)
	if err := s.saveCurrent(ctx, workflowIDForProjection(records), workspaceID); err != nil {
		return nil, err
	}
	return proj, nil
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

func (s Service) save(ctx context.Context, record *archaeodomain.ConvergenceRecord) error {
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
		SummaryText:     firstNonEmpty(record.Title, record.Question),
		SummaryMetadata: map[string]any{"workspace_id": record.WorkspaceID, "status": string(record.Status), "plan_version": record.PlanVersion},
		InlineRawText:   string(raw),
		CreatedAt:       record.UpdatedAt,
	})
}

func (s Service) saveCurrent(ctx context.Context, workflowID, workspaceID string) error {
	records, err := s.ListByWorkspace(ctx, workspaceID)
	if err != nil {
		return err
	}
	proj := buildCurrentProjection(records, workspaceID)
	raw, err := json.Marshal(proj)
	if err != nil {
		return err
	}
	return s.Store.UpsertWorkflowArtifact(ctx, memory.WorkflowArtifactRecord{
		ArtifactID:      currentArtifactID(workspaceID),
		WorkflowID:      strings.TrimSpace(workflowID),
		Kind:            currentArtifactKind,
		ContentType:     "application/json",
		StorageKind:     memory.ArtifactStorageInline,
		SummaryText:     firstNonEmpty(proj.WorkspaceID, workspaceID),
		SummaryMetadata: map[string]any{"workspace_id": strings.TrimSpace(workspaceID)},
		InlineRawText:   string(raw),
		CreatedAt:       s.now(),
	})
}

func workflowIDForProjection(records []archaeodomain.ConvergenceRecord) string {
	for _, record := range records {
		if trimmed := strings.TrimSpace(record.WorkflowID); trimmed != "" {
			return trimmed
		}
	}
	return ""
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

func convergenceMetadata(record archaeodomain.ConvergenceRecord) map[string]any {
	return map[string]any{
		"workspace_id":       record.WorkspaceID,
		"status":             string(record.Status),
		"plan_id":            record.PlanID,
		"plan_version":       record.PlanVersion,
		"deferred_draft_ids": append([]string(nil), record.DeferredDraftIDs...),
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

func buildCurrentProjection(records []archaeodomain.ConvergenceRecord, workspaceID string) *archaeodomain.WorkspaceConvergenceProjection {
	proj := &archaeodomain.WorkspaceConvergenceProjection{
		WorkspaceID: strings.TrimSpace(workspaceID),
		History:     append([]archaeodomain.ConvergenceRecord(nil), records...),
	}
	for i := range records {
		record := records[i]
		switch record.Status {
		case archaeodomain.ConvergenceResolutionOpen:
			proj.OpenCount++
			copy := record
			proj.Current = &copy
		case archaeodomain.ConvergenceResolutionResolved:
			proj.ResolvedCount++
		case archaeodomain.ConvergenceResolutionDeferred:
			proj.DeferredCount++
			copy := record
			proj.Current = &copy
		}
	}
	if proj.Current == nil && len(records) > 0 {
		copy := records[len(records)-1]
		proj.Current = &copy
	}
	return proj
}

func currentArtifactID(workspaceID string) string {
	return "convergence-current:" + strings.TrimSpace(workspaceID)
}

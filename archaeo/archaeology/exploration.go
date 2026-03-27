package archaeology

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeoevents "github.com/lexcodex/relurpify/archaeo/events"
	"github.com/lexcodex/relurpify/archaeo/internal/keylock"
	archaeotensions "github.com/lexcodex/relurpify/archaeo/tensions"
	"github.com/lexcodex/relurpify/framework/memory"
)

const (
	explorationSessionArtifactKind  = "archaeo_exploration_session"
	explorationSnapshotArtifactKind = "archaeo_exploration_snapshot"
)

type SnapshotInput struct {
	WorkflowID           string
	WorkspaceID          string
	BasedOnRevision      string
	SemanticSnapshotRef  string
	CandidatePatternRefs []string
	CandidateAnchorRefs  []string
	TensionIDs           []string
	OpenLearningIDs      []string
	Summary              string
}

type SessionView struct {
	Session        *archaeodomain.ExplorationSession
	Snapshots      []archaeodomain.ExplorationSnapshot
	Tensions       []archaeodomain.Tension
	TensionSummary *archaeodomain.TensionSummary
}

var explorationMutationLocks keylock.Locker

func (s Service) EnsureExplorationSession(ctx context.Context, workflowID, workspaceID, basedOnRevision string) (*archaeodomain.ExplorationSession, error) {
	var (
		session *archaeodomain.ExplorationSession
		err     error
	)
	err = explorationMutationLocks.With("workspace:"+strings.TrimSpace(workspaceID), func() error {
		store := s.workflowStore()
		workflowID = strings.TrimSpace(workflowID)
		workspaceID = strings.TrimSpace(workspaceID)
		if store == nil || workflowID == "" || workspaceID == "" {
			return nil
		}
		var persistedWorkflowID string
		session, persistedWorkflowID, err = s.findSessionByWorkspace(ctx, workspaceID)
		if err != nil {
			return err
		}
		now := s.now()
		if session != nil {
			if revisionChanged(session.BasedOnRevision, basedOnRevision) {
				session.Status = archaeodomain.ExplorationStatusStale
				session.RecomputeRequired = true
				session.StaleReason = fmt.Sprintf("revision changed: %s -> %s", strings.TrimSpace(session.BasedOnRevision), strings.TrimSpace(basedOnRevision))
			} else {
				session.Status = archaeodomain.ExplorationStatusActive
				session.RecomputeRequired = false
				session.StaleReason = ""
			}
			if strings.TrimSpace(basedOnRevision) != "" {
				session.BasedOnRevision = strings.TrimSpace(basedOnRevision)
			}
			session.UpdatedAt = now
			return s.saveSession(ctx, persistedWorkflowID, workflowID, session)
		}
		session = &archaeodomain.ExplorationSession{
			ID:              s.newID("explore"),
			WorkspaceID:     workspaceID,
			Status:          archaeodomain.ExplorationStatusActive,
			BasedOnRevision: strings.TrimSpace(basedOnRevision),
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		return s.saveSession(ctx, workflowID, workflowID, session)
	})
	return session, err
}

func (s Service) CreateExplorationSnapshot(ctx context.Context, session *archaeodomain.ExplorationSession, input SnapshotInput) (*archaeodomain.ExplorationSnapshot, error) {
	var (
		snapshot *archaeodomain.ExplorationSnapshot
		err      error
	)
	lockKey := "workflow:" + strings.TrimSpace(input.WorkflowID)
	if session != nil && strings.TrimSpace(session.WorkspaceID) != "" {
		lockKey = "workspace:" + strings.TrimSpace(session.WorkspaceID)
	}
	err = explorationMutationLocks.With(lockKey, func() error {
		store := s.workflowStore()
		if store == nil || session == nil {
			return nil
		}
		workflowID := strings.TrimSpace(input.WorkflowID)
		if workflowID == "" {
			return fmt.Errorf("workflow id required")
		}
		persistedWorkflowID := workflowID
		if _, ownerWorkflowID, err := s.findSessionByID(ctx, session.ID); err != nil {
			return err
		} else if strings.TrimSpace(ownerWorkflowID) != "" {
			persistedWorkflowID = ownerWorkflowID
		}
		now := s.now()
		snapshot = &archaeodomain.ExplorationSnapshot{
			ID:                   s.newID("snapshot"),
			ExplorationID:        session.ID,
			WorkspaceID:          firstNonEmpty(strings.TrimSpace(input.WorkspaceID), session.WorkspaceID),
			WorkflowID:           workflowID,
			SnapshotKey:          snapshotKey(now, session.ID),
			BasedOnRevision:      strings.TrimSpace(input.BasedOnRevision),
			SemanticSnapshotRef:  strings.TrimSpace(input.SemanticSnapshotRef),
			CandidatePatternRefs: append([]string(nil), input.CandidatePatternRefs...),
			CandidateAnchorRefs:  append([]string(nil), input.CandidateAnchorRefs...),
			TensionIDs:           append([]string(nil), input.TensionIDs...),
			OpenLearningIDs:      append([]string(nil), input.OpenLearningIDs...),
			Summary:              strings.TrimSpace(input.Summary),
			CreatedAt:            now,
			UpdatedAt:            now,
		}
		if err := s.saveSnapshot(ctx, snapshot); err != nil {
			return err
		}
		session.Status = archaeodomain.ExplorationStatusActive
		session.LatestSnapshotID = snapshot.ID
		session.SnapshotIDs = appendUnique(session.SnapshotIDs, snapshot.ID)
		session.BasedOnRevision = firstNonEmpty(snapshot.BasedOnRevision, session.BasedOnRevision)
		session.RecomputeRequired = false
		session.StaleReason = ""
		session.UpdatedAt = now
		if err := s.saveSession(ctx, persistedWorkflowID, workflowID, session); err != nil {
			return err
		}
		return nil
	})
	return snapshot, err
}

func (s Service) UpdateExplorationSnapshot(ctx context.Context, snapshot *archaeodomain.ExplorationSnapshot, input SnapshotInput) (*archaeodomain.ExplorationSnapshot, error) {
	var err error
	if snapshot == nil {
		return nil, nil
	}
	err = explorationMutationLocks.With("workflow:"+strings.TrimSpace(snapshot.WorkflowID), func() error {
		snapshot.BasedOnRevision = firstNonEmpty(strings.TrimSpace(input.BasedOnRevision), snapshot.BasedOnRevision)
		snapshot.SemanticSnapshotRef = firstNonEmpty(strings.TrimSpace(input.SemanticSnapshotRef), snapshot.SemanticSnapshotRef)
		snapshot.CandidatePatternRefs = append([]string(nil), input.CandidatePatternRefs...)
		snapshot.CandidateAnchorRefs = append([]string(nil), input.CandidateAnchorRefs...)
		snapshot.TensionIDs = append([]string(nil), input.TensionIDs...)
		snapshot.OpenLearningIDs = append([]string(nil), input.OpenLearningIDs...)
		if summary := strings.TrimSpace(input.Summary); summary != "" {
			snapshot.Summary = summary
		}
		snapshot.UpdatedAt = s.now()
		return s.saveSnapshot(ctx, snapshot)
	})
	return snapshot, err
}

func (s Service) MarkExplorationStale(ctx context.Context, explorationID, reason string) (*archaeodomain.ExplorationSession, error) {
	store := s.workflowStore()
	explorationID = strings.TrimSpace(explorationID)
	if store == nil || explorationID == "" {
		return nil, nil
	}
	session, persistedWorkflowID, err := s.findSessionByID(ctx, explorationID)
	if err != nil || session == nil {
		return session, err
	}
	session.Status = archaeodomain.ExplorationStatusStale
	session.RecomputeRequired = true
	session.StaleReason = strings.TrimSpace(reason)
	session.UpdatedAt = s.now()
	return session, s.saveSession(ctx, persistedWorkflowID, persistedWorkflowID, session)
}

func (s Service) LoadExplorationSession(ctx context.Context, explorationID string) (*archaeodomain.ExplorationSession, error) {
	session, _, err := s.findSessionByID(ctx, strings.TrimSpace(explorationID))
	return session, err
}

func (s Service) LoadActiveExplorationByWorkspace(ctx context.Context, workspaceID string) (*archaeodomain.ExplorationSession, error) {
	session, _, err := s.findSessionByWorkspace(ctx, strings.TrimSpace(workspaceID))
	return session, err
}

func (s Service) ListExplorationSnapshots(ctx context.Context, explorationID string) ([]archaeodomain.ExplorationSnapshot, error) {
	store := s.workflowStore()
	explorationID = strings.TrimSpace(explorationID)
	if store == nil || explorationID == "" {
		return nil, nil
	}
	workflows, err := store.ListWorkflows(ctx, 1024)
	if err != nil {
		return nil, err
	}
	out := make([]archaeodomain.ExplorationSnapshot, 0)
	for _, workflow := range workflows {
		artifacts, err := store.ListWorkflowArtifacts(ctx, workflow.WorkflowID, "")
		if err != nil {
			return nil, err
		}
		for _, artifact := range artifacts {
			if artifact.Kind != explorationSnapshotArtifactKind {
				continue
			}
			var snapshot archaeodomain.ExplorationSnapshot
			if err := json.Unmarshal([]byte(artifact.InlineRawText), &snapshot); err != nil {
				return nil, err
			}
			if snapshot.ExplorationID != explorationID {
				continue
			}
			out = append(out, snapshot)
		}
	}
	sortSnapshots(out)
	return out, nil
}

func (s Service) LoadExplorationView(ctx context.Context, explorationID string) (*SessionView, error) {
	session, err := s.LoadExplorationSession(ctx, explorationID)
	if err != nil || session == nil {
		return nil, err
	}
	snapshots, err := s.ListExplorationSnapshots(ctx, explorationID)
	if err != nil {
		return nil, err
	}
	tensionSvc := archaeotensions.Service{Store: s.workflowStore()}
	tensions, err := tensionSvc.ListByExploration(ctx, explorationID)
	if err != nil {
		return nil, err
	}
	summary, err := tensionSvc.SummaryByExploration(ctx, explorationID)
	if err != nil {
		return nil, err
	}
	return &SessionView{
		Session:        session,
		Snapshots:      snapshots,
		Tensions:       tensions,
		TensionSummary: summary,
	}, nil
}

func (s Service) LoadExplorationByWorkflow(ctx context.Context, workflowID string) (*archaeodomain.ExplorationSession, error) {
	view, err := s.LoadExplorationViewByWorkflow(ctx, workflowID)
	if err != nil || view == nil {
		return nil, err
	}
	return view.Session, nil
}

func (s Service) LoadExplorationSnapshotByWorkflow(ctx context.Context, workflowID, snapshotID string) (*archaeodomain.ExplorationSnapshot, error) {
	store := s.workflowStore()
	workflowID = strings.TrimSpace(workflowID)
	snapshotID = strings.TrimSpace(snapshotID)
	if store == nil || workflowID == "" || snapshotID == "" {
		return nil, nil
	}
	artifacts, err := store.ListWorkflowArtifacts(ctx, workflowID, "")
	if err != nil {
		return nil, err
	}
	for _, artifact := range artifacts {
		if artifact.Kind != explorationSnapshotArtifactKind {
			continue
		}
		var snapshot archaeodomain.ExplorationSnapshot
		if err := json.Unmarshal([]byte(artifact.InlineRawText), &snapshot); err != nil {
			return nil, err
		}
		if snapshot.ID == snapshotID {
			return &snapshot, nil
		}
	}
	return nil, nil
}

func (s Service) LoadExplorationViewByWorkflow(ctx context.Context, workflowID string) (*SessionView, error) {
	latestSnapshot, err := s.latestSnapshotByWorkflow(ctx, workflowID)
	if err != nil || latestSnapshot == nil {
		return nil, err
	}
	return s.LoadExplorationView(ctx, latestSnapshot.ExplorationID)
}

func (s Service) workflowStore() memory.WorkflowStateStore {
	if !isNilWorkflowStore(s.Store) {
		return s.Store
	}
	if !isNilWorkflowStore(s.Learning.Store) {
		return s.Learning.Store
	}
	return nil
}

func (s Service) findSessionByWorkspace(ctx context.Context, workspaceID string) (*archaeodomain.ExplorationSession, string, error) {
	return s.findSession(ctx, func(candidate archaeodomain.ExplorationSession) bool {
		return candidate.WorkspaceID == workspaceID && candidate.Status != archaeodomain.ExplorationStatusArchived
	})
}

func (s Service) findSessionByID(ctx context.Context, explorationID string) (*archaeodomain.ExplorationSession, string, error) {
	return s.findSession(ctx, func(candidate archaeodomain.ExplorationSession) bool {
		return candidate.ID == explorationID
	})
}

func (s Service) findSession(ctx context.Context, match func(archaeodomain.ExplorationSession) bool) (*archaeodomain.ExplorationSession, string, error) {
	store := s.workflowStore()
	if store == nil {
		return nil, "", nil
	}
	workflows, err := store.ListWorkflows(ctx, 1024)
	if err != nil {
		return nil, "", err
	}
	var latest *archaeodomain.ExplorationSession
	var owner string
	for _, workflow := range workflows {
		artifacts, err := store.ListWorkflowArtifacts(ctx, workflow.WorkflowID, "")
		if err != nil {
			return nil, "", err
		}
		for _, artifact := range artifacts {
			if artifact.Kind != explorationSessionArtifactKind {
				continue
			}
			var candidate archaeodomain.ExplorationSession
			if err := json.Unmarshal([]byte(artifact.InlineRawText), &candidate); err != nil {
				return nil, "", err
			}
			if !match(candidate) {
				continue
			}
			if latest == nil || latest.UpdatedAt.Before(candidate.UpdatedAt) {
				copy := candidate
				latest = &copy
				owner = workflow.WorkflowID
			}
		}
	}
	return latest, owner, nil
}

func (s Service) saveSession(ctx context.Context, persistedWorkflowID, actorWorkflowID string, session *archaeodomain.ExplorationSession) error {
	store := s.workflowStore()
	persistedWorkflowID = strings.TrimSpace(persistedWorkflowID)
	actorWorkflowID = strings.TrimSpace(actorWorkflowID)
	if store == nil || session == nil || persistedWorkflowID == "" {
		return nil
	}
	raw, err := json.Marshal(session)
	if err != nil {
		return err
	}
	if err := store.UpsertWorkflowArtifact(ctx, memory.WorkflowArtifactRecord{
		ArtifactID:  fmt.Sprintf("archaeo-exploration-session:%s", session.ID),
		WorkflowID:  persistedWorkflowID,
		Kind:        explorationSessionArtifactKind,
		ContentType: "application/json",
		StorageKind: memory.ArtifactStorageInline,
		SummaryText: fmt.Sprintf("exploration session %s", session.WorkspaceID),
		SummaryMetadata: map[string]any{
			"exploration_id":              session.ID,
			"workspace_id":                session.WorkspaceID,
			"status":                      session.Status,
			"persisted_workflow_id":       persistedWorkflowID,
			"last_touched_by_workflow_id": firstNonEmpty(actorWorkflowID, persistedWorkflowID),
		},
		InlineRawText: string(raw),
		CreatedAt:     s.now(),
	}); err != nil {
		return err
	}
	return archaeoevents.AppendWorkflowEvent(ctx, store, persistedWorkflowID, archaeoevents.EventExplorationSessionUpserted, session.WorkspaceID, map[string]any{
		"exploration_id":              session.ID,
		"workspace_id":                session.WorkspaceID,
		"status":                      session.Status,
		"latest_snapshot_id":          session.LatestSnapshotID,
		"based_on_revision":           session.BasedOnRevision,
		"recompute_required":          session.RecomputeRequired,
		"persisted_workflow_id":       persistedWorkflowID,
		"last_touched_by_workflow_id": firstNonEmpty(actorWorkflowID, persistedWorkflowID),
	}, s.now())
}

func (s Service) latestSnapshotByWorkflow(ctx context.Context, workflowID string) (*archaeodomain.ExplorationSnapshot, error) {
	workflowID = strings.TrimSpace(workflowID)
	if workflowID == "" {
		return nil, nil
	}
	store := s.workflowStore()
	if store == nil {
		return nil, nil
	}
	artifacts, err := store.ListWorkflowArtifacts(ctx, workflowID, "")
	if err != nil {
		return nil, err
	}
	var latest *archaeodomain.ExplorationSnapshot
	for _, artifact := range artifacts {
		if artifact.Kind != explorationSnapshotArtifactKind {
			continue
		}
		var snapshot archaeodomain.ExplorationSnapshot
		if err := json.Unmarshal([]byte(artifact.InlineRawText), &snapshot); err != nil {
			return nil, err
		}
		if latest == nil || latest.UpdatedAt.Before(snapshot.UpdatedAt) {
			copy := snapshot
			latest = &copy
		}
	}
	return latest, nil
}

func (s Service) saveSnapshot(ctx context.Context, snapshot *archaeodomain.ExplorationSnapshot) error {
	store := s.workflowStore()
	if store == nil || snapshot == nil {
		return nil
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	if err := store.UpsertWorkflowArtifact(ctx, memory.WorkflowArtifactRecord{
		ArtifactID:      fmt.Sprintf("archaeo-exploration-snapshot:%s", snapshot.ID),
		WorkflowID:      snapshot.WorkflowID,
		Kind:            explorationSnapshotArtifactKind,
		ContentType:     "application/json",
		StorageKind:     memory.ArtifactStorageInline,
		SummaryText:     fmt.Sprintf("exploration snapshot %s", snapshot.SnapshotKey),
		SummaryMetadata: map[string]any{"exploration_id": snapshot.ExplorationID, "snapshot_key": snapshot.SnapshotKey},
		InlineRawText:   string(raw),
		CreatedAt:       s.now(),
	}); err != nil {
		return err
	}
	return archaeoevents.AppendWorkflowEvent(ctx, store, snapshot.WorkflowID, archaeoevents.EventExplorationSnapshotUpserted, snapshot.SnapshotKey, map[string]any{
		"exploration_id":         snapshot.ExplorationID,
		"snapshot_id":            snapshot.ID,
		"snapshot_key":           snapshot.SnapshotKey,
		"candidate_pattern_refs": snapshot.CandidatePatternRefs,
		"candidate_anchor_refs":  snapshot.CandidateAnchorRefs,
		"tension_ids":            snapshot.TensionIDs,
		"open_learning_ids":      snapshot.OpenLearningIDs,
		"based_on_revision":      snapshot.BasedOnRevision,
		"semantic_snapshot_ref":  snapshot.SemanticSnapshotRef,
		"recompute_required":     snapshot.RecomputeRequired,
	}, s.now())
}

func snapshotKey(now time.Time, explorationID string) string {
	return fmt.Sprintf("%s-%s", now.UTC().Format("20060102T150405.000000000Z"), strings.TrimSpace(explorationID))
}

func appendUnique(values []string, extra string) []string {
	extra = strings.TrimSpace(extra)
	if extra == "" {
		return values
	}
	for _, value := range values {
		if value == extra {
			return values
		}
	}
	return append(values, extra)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func isNilWorkflowStore(store memory.WorkflowStateStore) bool {
	if store == nil {
		return true
	}
	value := reflect.ValueOf(store)
	switch value.Kind() {
	case reflect.Ptr, reflect.Map, reflect.Slice, reflect.Func, reflect.Interface:
		return value.IsNil()
	default:
		return false
	}
}

func revisionChanged(existing, updated string) bool {
	existing = strings.TrimSpace(existing)
	updated = strings.TrimSpace(updated)
	return existing != "" && updated != "" && existing != updated
}

func sortSnapshots(values []archaeodomain.ExplorationSnapshot) {
	for i := 0; i < len(values); i++ {
		for j := i + 1; j < len(values); j++ {
			if values[j].CreatedAt.Before(values[i].CreatedAt) {
				values[i], values[j] = values[j], values[i]
			}
		}
	}
}

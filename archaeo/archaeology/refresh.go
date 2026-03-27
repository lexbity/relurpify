package archaeology

import (
	"context"
	"strings"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeolearning "github.com/lexcodex/relurpify/archaeo/learning"
	archaeoplans "github.com/lexcodex/relurpify/archaeo/plans"
	"github.com/lexcodex/relurpify/archaeo/providers"
	archaeotensions "github.com/lexcodex/relurpify/archaeo/tensions"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/patterns"
	frameworkretrieval "github.com/lexcodex/relurpify/framework/retrieval"
)

type learningPreload struct {
	pending    []archaeolearning.Interaction
	blocking   []archaeolearning.Interaction
	patterns   []patterns.PatternRecord
	anchors    []frameworkretrieval.AnchorRecord
	driftByID  map[string]frameworkretrieval.AnchorEventRecord
	tensions   []archaeodomain.Tension
	patternIDs []string
	anchorIDs  []string
	tensionIDs []string
}

type refreshBundle struct {
	learning        *learningPreload
	pendingRequests []archaeodomain.RequestRecord
	snapshot        *archaeodomain.ExplorationSnapshot
}

func (s Service) preloadLearning(ctx context.Context, task *core.Task, state *core.Context, workflowID string) (*learningPreload, error) {
	if s.Learning.Store == nil {
		return &learningPreload{}, nil
	}
	corpusScope := corpusScopeFromTaskState(task, state)
	preload := &learningPreload{
		driftByID: make(map[string]frameworkretrieval.AnchorEventRecord),
	}
	if state != nil {
		if raw, ok := state.Get("euclo.preloaded_pending_learning"); ok && raw != nil {
			if pending, ok := raw.([]archaeolearning.Interaction); ok {
				preload.pending = append([]archaeolearning.Interaction(nil), pending...)
			}
		}
		if raw, ok := state.Get("euclo.preloaded_blocking_learning_ids"); ok && raw != nil {
			if ids, ok := raw.([]string); ok && len(ids) > 0 && len(preload.pending) > 0 {
				blocking := make([]archaeolearning.Interaction, 0, len(ids))
				lookup := make(map[string]struct{}, len(ids))
				for _, id := range ids {
					lookup[id] = struct{}{}
				}
				for _, interaction := range preload.pending {
					if _, ok := lookup[interaction.ID]; ok {
						blocking = append(blocking, interaction)
					}
				}
				preload.blocking = blocking
			}
		}
	}
	if s.Learning.PatternStore != nil && strings.TrimSpace(corpusScope) != "" {
		records, err := s.Learning.PatternStore.ListByStatus(ctx, patterns.PatternStatusProposed, corpusScope)
		if err != nil {
			return nil, err
		}
		preload.patterns = records
		preload.patternIDs = make([]string, 0, len(records))
		for _, record := range records {
			preload.patternIDs = append(preload.patternIDs, record.ID)
		}
	}
	if s.Learning.Retrieval != nil && strings.TrimSpace(corpusScope) != "" {
		anchors, err := s.Learning.Retrieval.DriftedAnchors(ctx, corpusScope)
		if err != nil {
			return nil, err
		}
		preload.anchors = anchors
		preload.anchorIDs = make([]string, 0, len(anchors))
		for _, anchor := range anchors {
			preload.anchorIDs = append(preload.anchorIDs, anchor.AnchorID)
		}
		drifts, err := s.Learning.Retrieval.UnresolvedDrifts(ctx, corpusScope)
		if err != nil {
			return nil, err
		}
		for _, drift := range drifts {
			current, ok := preload.driftByID[drift.AnchorID]
			if !ok || current.CreatedAt.Before(drift.CreatedAt) {
				preload.driftByID[drift.AnchorID] = drift
			}
		}
	}
	if s.Store != nil && strings.TrimSpace(workflowID) != "" {
		tensions, err := (archaeotensions.Service{Store: s.Store}).ActiveByWorkflow(ctx, workflowID)
		if err != nil {
			return nil, err
		}
		preload.tensions = tensions
		preload.tensionIDs = make([]string, 0, len(tensions))
		for _, tension := range tensions {
			preload.tensionIDs = append(preload.tensionIDs, tension.ID)
		}
	}
	return preload, nil
}

func (s Service) preloadRefresh(ctx context.Context, task *core.Task, state *core.Context, workflowID string) (*refreshBundle, error) {
	learning, err := s.preloadLearning(ctx, task, state, workflowID)
	if err != nil {
		return nil, err
	}
	bundle := &refreshBundle{learning: learning}
	if s.Requests.Store != nil && strings.TrimSpace(workflowID) != "" {
		requests, err := s.Requests.Pending(ctx, workflowID)
		if err != nil {
			return nil, err
		}
		bundle.pendingRequests = append([]archaeodomain.RequestRecord(nil), requests...)
	}
	snapshotID := ""
	if state != nil {
		snapshotID = strings.TrimSpace(state.GetString("euclo.active_exploration_snapshot_id"))
	}
	if s.Store != nil && strings.TrimSpace(workflowID) != "" && snapshotID != "" {
		snapshot, err := s.LoadExplorationSnapshotByWorkflow(ctx, workflowID, snapshotID)
		if err != nil {
			return nil, err
		}
		bundle.snapshot = snapshot
	}
	return bundle, nil
}

func (s Service) syncLearning(ctx context.Context, task *core.Task, state *core.Context, workflowID string, refresh *refreshBundle) (int, int, error) {
	if s.Learning.Store == nil {
		return 0, 0, nil
	}
	explorationID := explorationIDFromTaskState(task, state, workflowID)
	corpusScope := corpusScopeFromTaskState(task, state)
	revision := basedOnRevisionFromTask(task, state)
	preload := refreshLearningPreload(refresh)
	if preload == nil {
		var err error
		refresh, err = s.preloadRefresh(ctx, task, state, workflowID)
		if err != nil {
			return 0, 0, err
		}
		preload = refreshLearningPreload(refresh)
	}
	if len(preload.patterns) == 0 && len(preload.anchors) == 0 && len(preload.tensions) == 0 && preload.pending != nil {
		state.Set("euclo.learning_queue", append([]archaeolearning.Interaction(nil), preload.pending...))
		ids := make([]string, 0, len(preload.pending))
		for _, interaction := range preload.pending {
			ids = append(ids, interaction.ID)
		}
		state.Set("euclo.pending_learning_ids", ids)
		blockingIDs := make([]string, 0, len(preload.blocking))
		for _, interaction := range preload.blocking {
			blockingIDs = append(blockingIDs, interaction.ID)
		}
		state.Set("euclo.blocking_learning_ids", blockingIDs)
		_ = s.refreshExplorationSnapshot(ctx, workflowID, state, corpusScope, refresh, preload.pending)
		return len(preload.blocking), len(preload.pending), nil
	}
	if err := s.runProviderLifecycle(ctx, task, state, workflowID, explorationID, refresh); err != nil {
		return 0, 0, err
	}
	if _, err := s.Learning.SyncPatternProposals(ctx, workflowID, explorationID, corpusScope, revision); err != nil {
		return 0, 0, err
	}
	if _, err := s.Learning.SyncAnchorDrifts(ctx, workflowID, explorationID, corpusScope, revision); err != nil {
		return 0, 0, err
	}
	if _, err := s.Learning.SyncTensions(ctx, workflowID, explorationID, state.GetString("euclo.active_exploration_snapshot_id"), revision); err != nil {
		return 0, 0, err
	}
	pending, err := s.Learning.Pending(ctx, workflowID)
	if err != nil {
		return 0, 0, err
	}
	blocking, err := s.Learning.BlockingPending(ctx, workflowID)
	if err != nil {
		return 0, 0, err
	}
	state.Set("euclo.learning_queue", pending)
	ids := make([]string, 0, len(pending))
	for _, interaction := range pending {
		ids = append(ids, interaction.ID)
	}
	state.Set("euclo.pending_learning_ids", ids)
	blockingIDs := make([]string, 0, len(blocking))
	for _, interaction := range blocking {
		blockingIDs = append(blockingIDs, interaction.ID)
	}
	state.Set("euclo.blocking_learning_ids", blockingIDs)
	if refresh != nil && refresh.learning != nil {
		refresh.learning.pending = append([]archaeolearning.Interaction(nil), pending...)
		refresh.learning.blocking = append([]archaeolearning.Interaction(nil), blocking...)
	}
	_ = s.refreshExplorationSnapshot(ctx, workflowID, state, corpusScope, refresh, pending)
	return len(blocking), len(pending), nil
}

func (s Service) refreshExplorationSnapshot(ctx context.Context, workflowID string, state *core.Context, corpusScope string, refresh *refreshBundle, pending []archaeolearning.Interaction) error {
	snapshotID := state.GetString("euclo.active_exploration_snapshot_id")
	if workflowID == "" || snapshotID == "" {
		return nil
	}
	snapshot := refreshSnapshot(refresh, snapshotID)
	var err error
	if snapshot == nil {
		snapshot, err = s.LoadExplorationSnapshotByWorkflow(ctx, workflowID, snapshotID)
	}
	if err != nil || snapshot == nil {
		return err
	}
	patternRefs, anchorRefs, tensionRefs := s.collectSnapshotCandidates(ctx, snapshot.WorkflowID, corpusScope, refreshLearningPreload(refresh))
	openLearningIDs := make([]string, 0, len(pending))
	for _, interaction := range pending {
		openLearningIDs = append(openLearningIDs, interaction.ID)
	}
	updated, err := s.UpdateExplorationSnapshot(ctx, snapshot, SnapshotInput{
		BasedOnRevision:      state.GetString("euclo.based_on_revision"),
		SemanticSnapshotRef:  state.GetString("euclo.semantic_snapshot_ref"),
		CandidatePatternRefs: patternRefs,
		CandidateAnchorRefs:  anchorRefs,
		TensionIDs:           tensionRefs,
		OpenLearningIDs:      openLearningIDs,
		Summary:              snapshot.Summary,
	})
	if err != nil || updated == nil {
		return err
	}
	if refresh != nil {
		refresh.snapshot = updated
	}
	state.Set("euclo.exploration_candidate_pattern_refs", updated.CandidatePatternRefs)
	state.Set("euclo.exploration_candidate_anchor_refs", updated.CandidateAnchorRefs)
	state.Set("euclo.exploration_tension_refs", updated.TensionIDs)
	if draft, err := s.Plans.EnsureDraftFromExploration(ctx, archaeoplans.FormationInput{
		WorkflowID:       snapshot.WorkflowID,
		ExplorationID:    updated.ExplorationID,
		SnapshotID:       updated.ID,
		BasedOnRevision:  updated.BasedOnRevision,
		SemanticSnapshot: firstNonEmpty(updated.SemanticSnapshotRef, updated.ID),
		PatternRefs:      updated.CandidatePatternRefs,
		AnchorRefs:       updated.CandidateAnchorRefs,
		TensionRefs:      updated.TensionIDs,
		PendingLearning:  openLearningIDs,
	}); err == nil && draft != nil {
		state.Set("euclo.draft_plan_version", draft.Version)
		state.Set("euclo.plan_recompute_required", true)
		if s.Providers.ConvergenceReviewer != nil {
			failure, reviewErr := s.runConvergenceReviewRequest(ctx, refresh, providers.ConvergenceReviewRequest{
				WorkflowID:      workflowID,
				ExplorationID:   updated.ExplorationID,
				Plan:            &draft.Plan,
				BasedOnRevision: updated.BasedOnRevision,
			})
			if reviewErr != nil {
				return reviewErr
			}
			if failure != nil {
				state.Set("euclo.plan_formation_convergence_failure", *failure)
			}
		}
	} else if err != nil {
		return err
	}
	return nil
}

func preloadAnchorIDs(preload *learningPreload) []string {
	if preload == nil {
		return nil
	}
	return uniqueStrings(preload.anchorIDs)
}

func refreshLearningPreload(refresh *refreshBundle) *learningPreload {
	if refresh == nil {
		return nil
	}
	return refresh.learning
}

func refreshPendingRequests(refresh *refreshBundle) []archaeodomain.RequestRecord {
	if refresh == nil {
		return nil
	}
	return refresh.pendingRequests
}

func refreshSnapshot(refresh *refreshBundle, snapshotID string) *archaeodomain.ExplorationSnapshot {
	if refresh == nil || refresh.snapshot == nil {
		return nil
	}
	if strings.TrimSpace(snapshotID) == "" || strings.TrimSpace(refresh.snapshot.ID) != strings.TrimSpace(snapshotID) {
		return nil
	}
	return refresh.snapshot
}

func refreshRequest(refresh *refreshBundle, request *archaeodomain.RequestRecord) {
	if refresh == nil || request == nil {
		return
	}
	for i := range refresh.pendingRequests {
		if strings.TrimSpace(refresh.pendingRequests[i].ID) == strings.TrimSpace(request.ID) {
			refresh.pendingRequests[i] = *request
			return
		}
	}
	refresh.pendingRequests = append(refresh.pendingRequests, *request)
}

func (s Service) collectSnapshotCandidates(ctx context.Context, workflowID, corpusScope string, preload *learningPreload) ([]string, []string, []string) {
	if preload != nil {
		return append([]string(nil), preload.patternIDs...), append([]string(nil), preload.anchorIDs...), append([]string(nil), preload.tensionIDs...)
	}
	var patternRefs []string
	if s.Learning.PatternStore != nil && strings.TrimSpace(corpusScope) != "" {
		if records, err := s.Learning.PatternStore.ListByStatus(ctx, patterns.PatternStatusProposed, corpusScope); err == nil {
			patternRefs = make([]string, 0, len(records))
			for _, record := range records {
				patternRefs = append(patternRefs, record.ID)
			}
		}
	}
	var anchorRefs []string
	if s.Learning.Retrieval != nil && strings.TrimSpace(corpusScope) != "" {
		if anchors, err := s.Learning.Retrieval.DriftedAnchors(ctx, corpusScope); err == nil {
			anchorRefs = make([]string, 0, len(anchors))
			for _, anchor := range anchors {
				anchorRefs = append(anchorRefs, anchor.AnchorID)
			}
		}
	}
	var tensionRefs []string
	if s.Store != nil && strings.TrimSpace(workflowID) != "" {
		if tensions, err := (archaeotensions.Service{Store: s.Store}).ListByWorkflow(ctx, workflowID); err == nil {
			for _, tension := range tensions {
				if tension.Status == archaeodomain.TensionAccepted || tension.Status == archaeodomain.TensionResolved {
					continue
				}
				tensionRefs = append(tensionRefs, tension.ID)
			}
		}
	}
	return patternRefs, anchorRefs, tensionRefs
}

package archaeology

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	"github.com/lexcodex/relurpify/archaeo/providers"
	archaeorequests "github.com/lexcodex/relurpify/archaeo/requests"
	"github.com/lexcodex/relurpify/framework/core"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
)

func (s Service) runProviderLifecycle(ctx context.Context, task *core.Task, state *core.Context, workflowID, explorationID string, refresh *refreshBundle) error {
	preload := refreshLearningPreload(refresh)
	workspaceID := workspaceIDFromTaskState(task, state)
	corpusScope := corpusScopeFromTaskState(task, state)
	basedOnRevision := basedOnRevisionFromTask(task, state)
	symbolScope := symbolScopeFromTaskState(task, state)
	snapshotID := state.GetString("euclo.active_exploration_snapshot_id")
	var tasks []func() error
	if strings.TrimSpace(symbolScope) != "" {
		patternReq := providers.PatternSurfacingRequest{
			WorkflowID:      workflowID,
			ExplorationID:   explorationID,
			WorkspaceID:     workspaceID,
			SymbolScope:     symbolScope,
			CorpusScope:     corpusScope,
			MaxProposals:    8,
			BasedOnRevision: basedOnRevision,
		}
		tasks = append(tasks, func() error { return s.requestPatternSurfacing(ctx, refresh, patternReq, preload) })
		tensionReq := providers.TensionAnalysisRequest{
			WorkflowID:      workflowID,
			ExplorationID:   explorationID,
			SnapshotID:      snapshotID,
			WorkspaceID:     workspaceID,
			FilePath:        symbolScope,
			AnchorIDs:       preloadAnchorIDs(preload),
			BasedOnRevision: basedOnRevision,
		}
		tasks = append(tasks, func() error { return s.requestTensionAnalysis(ctx, refresh, tensionReq, preload) })
	}

	if strings.TrimSpace(taskInstruction(task)) != "" {
		prospectiveReq := providers.ProspectiveAnalysisRequest{
			WorkflowID:      workflowID,
			ExplorationID:   explorationID,
			WorkspaceID:     workspaceID,
			CorpusScope:     corpusScope,
			Description:     taskInstruction(task),
			Limit:           8,
			MinScore:        0.5,
			BasedOnRevision: basedOnRevision,
		}
		tasks = append(tasks, func() error { return s.requestProspectiveAnalysis(ctx, refresh, prospectiveReq, preload) })
	}
	return runConcurrentProviderTasks(tasks)
}

func runConcurrentProviderTasks(tasks []func() error) error {
	var (
		wg   sync.WaitGroup
		errs []error
		mu   sync.Mutex
	)
	for _, task := range tasks {
		wg.Add(1)
		go func(task func() error) {
			defer wg.Done()
			if err := task(); err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		}(task)
	}
	wg.Wait()
	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

func (s Service) requestPatternSurfacing(ctx context.Context, refresh *refreshBundle, req providers.PatternSurfacingRequest, preload *learningPreload) error {
	request, err := s.ensurePendingRequest(ctx, refresh, archaeodomain.RequestPatternSurfacing, req.WorkflowID, req.ExplorationID, "", nil, "Surface candidate patterns", "Provider-backed pattern surfacing during archaeology refresh.", []string{req.SymbolScope}, map[string]any{
		"workspace_id":  req.WorkspaceID,
		"symbol_scope":  req.SymbolScope,
		"corpus_scope":  req.CorpusScope,
		"max_proposals": req.MaxProposals,
	}, req.BasedOnRevision)
	if err != nil || request == nil || s.Providers.PatternSurfacer == nil {
		return err
	}
	refreshRequest(refresh, request)
	if preload != nil {
		preload.patternIDs = uniqueStrings(preload.patternIDs)
	}
	return nil
}

func (s Service) requestTensionAnalysis(ctx context.Context, refresh *refreshBundle, req providers.TensionAnalysisRequest, preload *learningPreload) error {
	request, err := s.ensurePendingRequest(ctx, refresh, archaeodomain.RequestTensionAnalysis, req.WorkflowID, req.ExplorationID, req.SnapshotID, nil, "Analyze tensions", "Provider-backed tension analysis during archaeology refresh.", append([]string{req.FilePath}, req.AnchorIDs...), map[string]any{
		"workspace_id": req.WorkspaceID,
		"file_path":    req.FilePath,
		"anchor_ids":   append([]string(nil), req.AnchorIDs...),
	}, req.BasedOnRevision)
	if err != nil || request == nil || s.Providers.TensionAnalyzer == nil {
		return err
	}
	refreshRequest(refresh, request)
	if preload != nil {
		preload.tensionIDs = uniqueStrings(preload.tensionIDs)
	}
	return nil
}

func (s Service) requestProspectiveAnalysis(ctx context.Context, refresh *refreshBundle, req providers.ProspectiveAnalysisRequest, preload *learningPreload) error {
	request, err := s.ensurePendingRequest(ctx, refresh, archaeodomain.RequestProspectiveAnalysis, req.WorkflowID, req.ExplorationID, "", nil, "Analyze prospective structure", "Provider-backed prospective analysis during archaeology refresh.", nil, map[string]any{
		"workspace_id": req.WorkspaceID,
		"corpus_scope": req.CorpusScope,
		"description":  req.Description,
		"limit":        req.Limit,
		"min_score":    req.MinScore,
	}, req.BasedOnRevision)
	if err != nil || request == nil || s.Providers.ProspectiveAnalyzer == nil {
		return err
	}
	refreshRequest(refresh, request)
	if preload != nil {
		preload.patternIDs = uniqueStrings(preload.patternIDs)
	}
	return nil
}

func (s Service) runConvergenceReviewRequest(ctx context.Context, refresh *refreshBundle, req providers.ConvergenceReviewRequest) (*frameworkplan.ConvergenceFailure, error) {
	request, err := s.ensurePendingRequest(ctx, refresh, archaeodomain.RequestConvergenceReview, req.WorkflowID, req.ExplorationID, "", planVersionRef(req.Plan), "Review plan convergence", "Provider-backed convergence review for newly formed draft.", planStepRefs(req.Plan), map[string]any{
		"plan_id": planID(req.Plan),
	}, req.BasedOnRevision)
	if err != nil || request == nil || s.Providers.ConvergenceReviewer == nil {
		return nil, err
	}
	refreshRequest(refresh, request)
	return nil, nil
}

func (s Service) ensurePendingRequest(ctx context.Context, refresh *refreshBundle, kind archaeodomain.RequestKind, workflowID, explorationID, snapshotID string, planVersion *int, title, description string, subjectRefs []string, input map[string]any, basedOnRevision string) (*archaeodomain.RequestRecord, error) {
	if s.Requests.Store == nil || strings.TrimSpace(workflowID) == "" {
		return nil, nil
	}
	correlationID := providerCorrelationID(kind, workflowID, explorationID, snapshotID, planVersion)
	idempotencyKey := providerIdempotencyKey(kind, workflowID, explorationID, snapshotID, subjectRefs, input, basedOnRevision)
	if existing := refreshPendingRequestByIdentity(refresh, kind, explorationID, snapshotID, basedOnRevision, correlationID, idempotencyKey, subjectRefs, input); existing != nil {
		return existing, nil
	}
	record, err := s.Requests.Create(ctx, archaeorequests.CreateInput{
		WorkflowID:      workflowID,
		ExplorationID:   explorationID,
		SnapshotID:      snapshotID,
		PlanVersion:     cloneInt(planVersion),
		Kind:            kind,
		Title:           title,
		Description:     description,
		RequestedBy:     "archaeo.lifecycle",
		CorrelationID:   correlationID,
		IdempotencyKey:  idempotencyKey,
		SubjectRefs:     uniqueStrings(subjectRefs),
		Input:           cloneAnyMap(input),
		BasedOnRevision: basedOnRevision,
	})
	if err != nil || record == nil {
		return record, err
	}
	refreshRequest(refresh, record)
	return record, nil
}

func refreshPendingRequestByIdentity(refresh *refreshBundle, kind archaeodomain.RequestKind, explorationID, snapshotID, basedOnRevision, correlationID, idempotencyKey string, subjectRefs []string, input map[string]any) *archaeodomain.RequestRecord {
	if refresh == nil {
		return nil
	}
	for i := range refresh.pendingRequests {
		record := &refresh.pendingRequests[i]
		if record.Status == archaeodomain.RequestStatusSuperseded || record.Status == archaeodomain.RequestStatusInvalidated {
			continue
		}
		if idempotencyKey != "" && strings.TrimSpace(record.IdempotencyKey) == strings.TrimSpace(idempotencyKey) {
			return record
		}
		if correlationID != "" && strings.TrimSpace(record.CorrelationID) == strings.TrimSpace(correlationID) && record.Kind == kind {
			return record
		}
		if record.Kind == kind &&
			strings.TrimSpace(record.ExplorationID) == strings.TrimSpace(explorationID) &&
			strings.TrimSpace(record.SnapshotID) == strings.TrimSpace(snapshotID) &&
			strings.TrimSpace(record.BasedOnRevision) == strings.TrimSpace(basedOnRevision) &&
			sameStringSet(record.SubjectRefs, subjectRefs) &&
			sameAnyMap(record.Input, input) {
			return record
		}
	}
	return nil
}

func providerCorrelationID(kind archaeodomain.RequestKind, workflowID, explorationID, snapshotID string, planVersion *int) string {
	parts := []string{string(kind), strings.TrimSpace(workflowID), strings.TrimSpace(explorationID), strings.TrimSpace(snapshotID)}
	if planVersion != nil {
		parts = append(parts, fmt.Sprintf("v%d", *planVersion))
	}
	return strings.Join(parts, ":")
}

func providerIdempotencyKey(kind archaeodomain.RequestKind, workflowID, explorationID, snapshotID string, subjectRefs []string, input map[string]any, basedOnRevision string) string {
	return strings.Join([]string{
		string(kind),
		strings.TrimSpace(workflowID),
		strings.TrimSpace(explorationID),
		strings.TrimSpace(snapshotID),
		strings.TrimSpace(basedOnRevision),
		joinNormalized(subjectRefs),
		normalizeAnyMap(input),
	}, "|")
}

func joinNormalized(values []string) string {
	normalized := uniqueStrings(values)
	sort.Strings(normalized)
	return strings.Join(normalized, ",")
}

func normalizeAnyMap(values map[string]any) string {
	if len(values) == 0 {
		return ""
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		switch typed := values[key].(type) {
		case []string:
			parts = append(parts, key+"="+joinNormalized(typed))
		default:
			parts = append(parts, key+"="+fmt.Sprint(values[key]))
		}
	}
	return strings.Join(parts, ";")
}

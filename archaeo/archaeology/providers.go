package archaeology

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	"github.com/lexcodex/relurpify/archaeo/providers"
	archaeorequests "github.com/lexcodex/relurpify/archaeo/requests"
	archaeotensions "github.com/lexcodex/relurpify/archaeo/tensions"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/patterns"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
)

func (s Service) runProviderLifecycle(ctx context.Context, task *core.Task, state *core.Context, workflowID, explorationID string, refresh *refreshBundle) error {
	preload := refreshLearningPreload(refresh)
	workspaceID := workspaceIDFromTaskState(task, state)
	corpusScope := corpusScopeFromTaskState(task, state)
	basedOnRevision := basedOnRevisionFromTask(task, state)
	symbolScope := symbolScopeFromTaskState(task, state)
	snapshotID := state.GetString("euclo.active_exploration_snapshot_id")
	inline := inlineProviderFulfillmentEnabled(state)

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
		if err := s.requestPatternSurfacing(ctx, refresh, patternReq, inline, preload, state); err != nil {
			return err
		}

		tensionReq := providers.TensionAnalysisRequest{
			WorkflowID:      workflowID,
			ExplorationID:   explorationID,
			SnapshotID:      snapshotID,
			WorkspaceID:     workspaceID,
			FilePath:        symbolScope,
			AnchorIDs:       preloadAnchorIDs(preload),
			BasedOnRevision: basedOnRevision,
		}
		if err := s.requestTensionAnalysis(ctx, refresh, tensionReq, inline, preload); err != nil {
			return err
		}
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
		if err := s.requestProspectiveAnalysis(ctx, refresh, prospectiveReq, inline, preload, state); err != nil {
			return err
		}
	}

	return nil
}

func (s Service) recordPendingProviderRequests(ctx context.Context, task *core.Task, state *core.Context, workflowID, explorationID string, refresh *refreshBundle) error {
	if s.Requests.Store == nil {
		return nil
	}
	preload := refreshLearningPreload(refresh)
	workspaceID := workspaceIDFromTaskState(task, state)
	corpusScope := corpusScopeFromTaskState(task, state)
	basedOnRevision := basedOnRevisionFromTask(task, state)
	symbolScope := symbolScopeFromTaskState(task, state)
	snapshotID := state.GetString("euclo.active_exploration_snapshot_id")
	if symbolScope != "" {
		if _, err := s.ensurePendingRequest(ctx, refresh, archaeodomain.RequestPatternSurfacing, workflowID, explorationID, snapshotID, nil, "Surface candidate patterns", "Pattern surfacing requested for archaeology refresh.", []string{symbolScope}, map[string]any{
			"workspace_id": workspaceID,
			"symbol_scope": symbolScope,
			"corpus_scope": corpusScope,
		}, basedOnRevision); err != nil {
			return err
		}
		if _, err := s.ensurePendingRequest(ctx, refresh, archaeodomain.RequestTensionAnalysis, workflowID, explorationID, snapshotID, nil, "Analyze tensions", "Tension analysis requested for archaeology refresh.", append([]string(nil), preloadAnchorIDs(preload)...), map[string]any{
			"workspace_id": workspaceID,
			"file_path":    symbolScope,
			"anchor_ids":   preloadAnchorIDs(preload),
		}, basedOnRevision); err != nil {
			return err
		}
	}
	if strings.TrimSpace(taskInstruction(task)) != "" {
		if _, err := s.ensurePendingRequest(ctx, refresh, archaeodomain.RequestProspectiveAnalysis, workflowID, explorationID, snapshotID, nil, "Analyze prospective structure", "Prospective analysis requested for archaeology refresh.", nil, map[string]any{
			"workspace_id": workspaceID,
			"corpus_scope": corpusScope,
			"description":  taskInstruction(task),
		}, basedOnRevision); err != nil {
			return err
		}
	}
	return nil
}

func (s Service) requestPatternSurfacing(ctx context.Context, refresh *refreshBundle, req providers.PatternSurfacingRequest, inline bool, preload *learningPreload, state *core.Context) error {
	request, err := s.ensurePendingRequest(ctx, refresh, archaeodomain.RequestPatternSurfacing, req.WorkflowID, req.ExplorationID, "", nil, "Surface candidate patterns", "Provider-backed pattern surfacing during archaeology refresh.", []string{req.SymbolScope}, map[string]any{
		"workspace_id":  req.WorkspaceID,
		"symbol_scope":  req.SymbolScope,
		"corpus_scope":  req.CorpusScope,
		"max_proposals": req.MaxProposals,
	}, req.BasedOnRevision)
	if err != nil || request == nil || s.Providers.PatternSurfacer == nil || !inline || !requestNeedsFulfillment(request) {
		return err
	}
	request, err = s.claimProviderRequest(ctx, refresh, request)
	if err != nil {
		return err
	}
	records, err := s.Providers.PatternSurfacer.SurfacePatterns(ctx, req)
	if err != nil {
		s.failProviderRequest(ctx, refresh, request, err)
		return err
	}
	patternIDs, err := s.applyPatternSurfacingFulfillment(ctx, records, preload)
	if err != nil {
		s.failProviderRequest(ctx, refresh, request, err)
		return err
	}
	if state != nil && len(patternIDs) > 0 {
		state.Set("euclo.prospective_pattern_refs", uniqueStrings(append(stringSliceFromState(state, "euclo.prospective_pattern_refs"), patternIDs...)))
	}
	return s.applyProviderFulfillment(ctx, refresh, request, archaeodomain.RequestFulfillment{
		Kind:    "pattern_records",
		RefID:   firstNonEmpty(patternIDs...),
		Summary: fmt.Sprintf("surfaced %d candidate patterns", len(patternIDs)),
		Metadata: map[string]any{
			"count":       len(patternIDs),
			"pattern_ids": append([]string(nil), patternIDs...),
		},
	})
}

func (s Service) requestTensionAnalysis(ctx context.Context, refresh *refreshBundle, req providers.TensionAnalysisRequest, inline bool, preload *learningPreload) error {
	request, err := s.ensurePendingRequest(ctx, refresh, archaeodomain.RequestTensionAnalysis, req.WorkflowID, req.ExplorationID, req.SnapshotID, nil, "Analyze tensions", "Provider-backed tension analysis during archaeology refresh.", append([]string{req.FilePath}, req.AnchorIDs...), map[string]any{
		"workspace_id": req.WorkspaceID,
		"file_path":    req.FilePath,
		"anchor_ids":   append([]string(nil), req.AnchorIDs...),
	}, req.BasedOnRevision)
	if err != nil || request == nil || s.Providers.TensionAnalyzer == nil || !inline || !requestNeedsFulfillment(request) {
		return err
	}
	request, err = s.claimProviderRequest(ctx, refresh, request)
	if err != nil {
		return err
	}
	records, err := s.Providers.TensionAnalyzer.AnalyzeTensions(ctx, req)
	if err != nil {
		s.failProviderRequest(ctx, refresh, request, err)
		return err
	}
	tensionIDs, err := s.applyTensionAnalysisFulfillment(ctx, req, records, preload)
	if err != nil {
		s.failProviderRequest(ctx, refresh, request, err)
		return err
	}
	return s.applyProviderFulfillment(ctx, refresh, request, archaeodomain.RequestFulfillment{
		Kind:    "tension_records",
		RefID:   firstNonEmpty(tensionIDs...),
		Summary: fmt.Sprintf("analyzed %d tensions", len(tensionIDs)),
		Metadata: map[string]any{
			"count":       len(tensionIDs),
			"tension_ids": append([]string(nil), tensionIDs...),
		},
	})
}

func (s Service) requestProspectiveAnalysis(ctx context.Context, refresh *refreshBundle, req providers.ProspectiveAnalysisRequest, inline bool, preload *learningPreload, state *core.Context) error {
	request, err := s.ensurePendingRequest(ctx, refresh, archaeodomain.RequestProspectiveAnalysis, req.WorkflowID, req.ExplorationID, "", nil, "Analyze prospective structure", "Provider-backed prospective analysis during archaeology refresh.", nil, map[string]any{
		"workspace_id": req.WorkspaceID,
		"corpus_scope": req.CorpusScope,
		"description":  req.Description,
		"limit":        req.Limit,
		"min_score":    req.MinScore,
	}, req.BasedOnRevision)
	if err != nil || request == nil || s.Providers.ProspectiveAnalyzer == nil || !inline || !requestNeedsFulfillment(request) {
		return err
	}
	request, err = s.claimProviderRequest(ctx, refresh, request)
	if err != nil {
		return err
	}
	records, err := s.Providers.ProspectiveAnalyzer.AnalyzeProspective(ctx, req)
	if err != nil {
		s.failProviderRequest(ctx, refresh, request, err)
		return err
	}
	patternIDs, err := s.applyProspectiveAnalysisFulfillment(ctx, records, preload, state)
	if err != nil {
		s.failProviderRequest(ctx, refresh, request, err)
		return err
	}
	return s.applyProviderFulfillment(ctx, refresh, request, archaeodomain.RequestFulfillment{
		Kind:    "prospective_patterns",
		RefID:   firstNonEmpty(patternIDs...),
		Summary: fmt.Sprintf("matched %d prospective patterns", len(patternIDs)),
		Metadata: map[string]any{
			"count":       len(patternIDs),
			"pattern_ids": append([]string(nil), patternIDs...),
		},
	})
}

func (s Service) runConvergenceReviewRequest(ctx context.Context, refresh *refreshBundle, req providers.ConvergenceReviewRequest) (*frameworkplan.ConvergenceFailure, error) {
	request, err := s.ensurePendingRequest(ctx, refresh, archaeodomain.RequestConvergenceReview, req.WorkflowID, req.ExplorationID, "", planVersionRef(req.Plan), "Review plan convergence", "Provider-backed convergence review for newly formed draft.", planStepRefs(req.Plan), map[string]any{
		"plan_id": planID(req.Plan),
	}, req.BasedOnRevision)
	if err != nil || request == nil || s.Providers.ConvergenceReviewer == nil {
		return nil, err
	}
	if !requestNeedsFulfillment(request) {
		return nil, nil
	}
	request, err = s.claimProviderRequest(ctx, refresh, request)
	if err != nil {
		return nil, err
	}
	failure, err := s.Providers.ConvergenceReviewer.ReviewConvergence(ctx, req)
	if err != nil {
		s.failProviderRequest(ctx, refresh, request, err)
		return nil, err
	}
	fulfillment := archaeodomain.RequestFulfillment{
		Kind:    "convergence_review",
		RefID:   planID(req.Plan),
		Summary: "plan convergence verified",
		Metadata: map[string]any{
			"failed": false,
		},
	}
	if failure != nil {
		fulfillment.Summary = "plan convergence failed"
		fulfillment.Metadata["failed"] = true
		fulfillment.Metadata["description"] = failure.Description
		fulfillment.Metadata["unresolved_tension_ids"] = append([]string(nil), failure.UnresolvedTensions...)
	}
	if err := s.applyProviderFulfillment(ctx, refresh, request, fulfillment); err != nil {
		return nil, err
	}
	return failure, nil
}

func (s Service) ensurePendingRequest(ctx context.Context, refresh *refreshBundle, kind archaeodomain.RequestKind, workflowID, explorationID, snapshotID string, planVersion *int, title, description string, subjectRefs []string, input map[string]any, basedOnRevision string) (*archaeodomain.RequestRecord, error) {
	if s.Requests.Store == nil || strings.TrimSpace(workflowID) == "" {
		return nil, nil
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
		CorrelationID:   providerCorrelationID(kind, workflowID, explorationID, snapshotID, planVersion),
		IdempotencyKey:  providerIdempotencyKey(kind, workflowID, explorationID, snapshotID, subjectRefs, input, basedOnRevision),
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

func (s Service) claimProviderRequest(ctx context.Context, refresh *refreshBundle, request *archaeodomain.RequestRecord) (*archaeodomain.RequestRecord, error) {
	if request == nil || s.Requests.Store == nil {
		return request, nil
	}
	var err error
	if request.Status == archaeodomain.RequestStatusPending {
		request, err = s.Requests.Dispatch(ctx, request.WorkflowID, request.ID, map[string]any{"mode": "inline_provider"})
		if err != nil {
			return nil, err
		}
		refreshRequest(refresh, request)
	}
	updated, err := s.Requests.Claim(ctx, archaeorequests.ClaimInput{
		WorkflowID: request.WorkflowID,
		RequestID:  request.ID,
		ClaimedBy:  "archaeo.inline_provider",
		LeaseTTL:   5 * time.Minute,
		Metadata:   map[string]any{"mode": "inline_provider"},
	})
	if err != nil {
		return nil, err
	}
	refreshRequest(refresh, updated)
	return updated, nil
}

func (s Service) failProviderRequest(ctx context.Context, refresh *refreshBundle, request *archaeodomain.RequestRecord, err error) {
	if request == nil || s.Requests.Store == nil || err == nil {
		return
	}
	updated, _ := s.Requests.Fail(ctx, request.WorkflowID, request.ID, err.Error(), false)
	refreshRequest(refresh, updated)
}

func (s Service) applyProviderFulfillment(ctx context.Context, refresh *refreshBundle, request *archaeodomain.RequestRecord, fulfillment archaeodomain.RequestFulfillment) error {
	if request == nil || s.Requests.Store == nil {
		return nil
	}
	updated, _, err := s.Requests.ApplyFulfillment(ctx, archaeorequests.ApplyFulfillmentInput{
		WorkflowID:        request.WorkflowID,
		RequestID:         request.ID,
		Fulfillment:       fulfillment,
		CurrentRevision:   request.BasedOnRevision,
		CurrentSnapshotID: request.SnapshotID,
	})
	refreshRequest(refresh, updated)
	return err
}

func (s Service) applyPatternSurfacingFulfillment(ctx context.Context, records []patterns.PatternRecord, preload *learningPreload) ([]string, error) {
	ids := make([]string, 0, len(records))
	for _, record := range records {
		if s.Learning.PatternStore != nil {
			if err := s.Learning.PatternStore.Save(ctx, record); err != nil {
				return nil, err
			}
		}
		ids = append(ids, record.ID)
	}
	if preload != nil {
		preload.patternIDs = uniqueStrings(append(preload.patternIDs, ids...))
	}
	return uniqueStrings(ids), nil
}

func (s Service) applyTensionAnalysisFulfillment(ctx context.Context, req providers.TensionAnalysisRequest, records []archaeodomain.Tension, preload *learningPreload) ([]string, error) {
	ids := make([]string, 0, len(records))
	for _, record := range records {
		created, err := (archaeotensions.Service{Store: s.Store}).CreateOrUpdate(ctx, archaeotensions.CreateInput{
			WorkflowID:         req.WorkflowID,
			ExplorationID:      req.ExplorationID,
			SnapshotID:         req.SnapshotID,
			SourceRef:          firstNonEmpty(strings.TrimSpace(record.SourceRef), strings.TrimSpace(record.ID)),
			PatternIDs:         append([]string(nil), record.PatternIDs...),
			AnchorRefs:         append([]string(nil), record.AnchorRefs...),
			SymbolScope:        append([]string(nil), record.SymbolScope...),
			Kind:               firstNonEmpty(strings.TrimSpace(record.Kind), "analysis_tension"),
			Description:        strings.TrimSpace(record.Description),
			Severity:           strings.TrimSpace(record.Severity),
			Status:             firstNonEmptyTensionStatus(record.Status, archaeodomain.TensionUnresolved),
			BlastRadiusNodeIDs: append([]string(nil), record.BlastRadiusNodeIDs...),
			RelatedPlanStepIDs: append([]string(nil), record.RelatedPlanStepIDs...),
			CommentRefs:        append([]string(nil), record.CommentRefs...),
			BasedOnRevision:    firstNonEmpty(strings.TrimSpace(record.BasedOnRevision), req.BasedOnRevision),
		})
		if err != nil {
			return nil, err
		}
		if created != nil {
			ids = append(ids, created.ID)
		}
	}
	if preload != nil {
		preload.tensionIDs = uniqueStrings(append(preload.tensionIDs, ids...))
	}
	return uniqueStrings(ids), nil
}

func (s Service) applyProspectiveAnalysisFulfillment(ctx context.Context, records []patterns.PatternRecord, preload *learningPreload, state *core.Context) ([]string, error) {
	ids := make([]string, 0, len(records))
	for _, record := range records {
		if s.Learning.PatternStore != nil {
			if err := s.Learning.PatternStore.Save(ctx, record); err != nil {
				return nil, err
			}
		}
		ids = append(ids, record.ID)
	}
	ids = uniqueStrings(ids)
	if preload != nil {
		preload.patternIDs = uniqueStrings(append(preload.patternIDs, ids...))
	}
	if state != nil && len(ids) > 0 {
		state.Set("euclo.prospective_pattern_refs", ids)
	}
	return ids, nil
}

func inlineProviderFulfillmentEnabled(state *core.Context) bool {
	if state == nil {
		return true
	}
	raw, ok := state.Get("euclo.archaeo.inline_provider_fulfillment")
	if !ok {
		return true
	}
	switch typed := raw.(type) {
	case bool:
		return typed
	case string:
		return !strings.EqualFold(strings.TrimSpace(typed), "false")
	default:
		return true
	}
}

func requestNeedsDispatch(record *archaeodomain.RequestRecord) bool {
	return record != nil && record.Status == archaeodomain.RequestStatusPending
}

func requestNeedsFulfillment(record *archaeodomain.RequestRecord) bool {
	if record == nil {
		return false
	}
	switch record.Status {
	case archaeodomain.RequestStatusPending, archaeodomain.RequestStatusDispatched:
		return true
	default:
		return false
	}
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

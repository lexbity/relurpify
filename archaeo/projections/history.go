package projections

import (
	"context"
	"strings"

	archaeoarch "codeburg.org/lexbit/relurpify/archaeo/archaeology"
	archaeoconvergence "codeburg.org/lexbit/relurpify/archaeo/convergence"
	archaeodecisions "codeburg.org/lexbit/relurpify/archaeo/decisions"
	archaeodeferred "codeburg.org/lexbit/relurpify/archaeo/deferred"
	archaeodomain "codeburg.org/lexbit/relurpify/archaeo/domain"
	archaeoevents "codeburg.org/lexbit/relurpify/archaeo/events"
	archaeolearning "codeburg.org/lexbit/relurpify/archaeo/learning"
	archaeoplans "codeburg.org/lexbit/relurpify/archaeo/plans"
	archaeoprovenance "codeburg.org/lexbit/relurpify/archaeo/provenance"
	archaeorequests "codeburg.org/lexbit/relurpify/archaeo/requests"
	"codeburg.org/lexbit/relurpify/framework/memory"
)

func (s *Service) MutationHistory(ctx context.Context, workflowID string) (*MutationHistoryProjection, error) {
	workflowID = strings.TrimSpace(workflowID)
	if s == nil || s.Store == nil || workflowID == "" {
		return nil, nil
	}
	return buildMutationHistoryProjection(ctx, s.Store, workflowID)
}

func (s *Service) RequestHistory(ctx context.Context, workflowID string) (*RequestHistoryProjection, error) {
	workflowID = strings.TrimSpace(workflowID)
	if s == nil || s.Store == nil || workflowID == "" {
		return nil, nil
	}
	return buildRequestHistoryProjection(ctx, s.Store, workflowID)
}

func (s *Service) PlanLineage(ctx context.Context, workflowID string) (*PlanLineageProjection, error) {
	workflowID = strings.TrimSpace(workflowID)
	if s == nil || s.Store == nil || workflowID == "" {
		return nil, nil
	}
	return buildPlanLineageProjection(ctx, s.Store, workflowID)
}

func (s *Service) ExplorationActivity(ctx context.Context, workflowID string) (*ExplorationActivityProjection, error) {
	workflowID = strings.TrimSpace(workflowID)
	if s == nil || s.Store == nil || workflowID == "" {
		return nil, nil
	}
	return buildExplorationActivityProjection(ctx, s.Store, workflowID)
}

func (s *Service) Provenance(ctx context.Context, workflowID string) (*ProvenanceProjection, error) {
	workflowID = strings.TrimSpace(workflowID)
	if s == nil || s.Store == nil || workflowID == "" {
		return nil, nil
	}
	return buildProvenanceProjection(ctx, s.Store, workflowID)
}

func (s *Service) DeferredDrafts(ctx context.Context, workspaceID string) (*DeferredDraftProjection, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if s == nil || s.Store == nil || workspaceID == "" {
		return nil, nil
	}
	return buildDeferredDraftProjection(ctx, s.Store, workspaceID)
}

func (s *Service) ConvergenceHistory(ctx context.Context, workspaceID string) (*archaeodomain.WorkspaceConvergenceProjection, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if s == nil || s.Store == nil || workspaceID == "" {
		return nil, nil
	}
	return (archaeoconvergence.Service{Store: s.Store}).CurrentByWorkspace(ctx, workspaceID)
}

func (s *Service) DecisionTrail(ctx context.Context, workspaceID string) (*DecisionTrailProjection, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if s == nil || s.Store == nil || workspaceID == "" {
		return nil, nil
	}
	return buildDecisionTrailProjection(ctx, s.Store, workspaceID)
}

func buildMutationHistoryProjection(ctx context.Context, store memory.WorkflowStateStore, workflowID string) (*MutationHistoryProjection, error) {
	workflowID = strings.TrimSpace(workflowID)
	if store == nil || workflowID == "" {
		return nil, nil
	}
	mutations, err := archaeoevents.ReadMutationEvents(ctx, store, workflowID)
	if err != nil {
		return nil, err
	}
	proj := &MutationHistoryProjection{
		WorkflowID:      workflowID,
		Mutations:       append([]archaeodomain.MutationEvent(nil), mutations...),
		DispositionByID: make(map[string]string, len(mutations)),
	}
	for _, mutation := range mutations {
		proj.DispositionByID[mutation.ID] = string(mutation.Disposition)
		if mutation.Blocking {
			proj.BlockingCount++
		}
		if mutation.CreatedAt.IsZero() {
			continue
		}
		if proj.LastMutationAt == nil || proj.LastMutationAt.Before(mutation.CreatedAt) {
			value := mutation.CreatedAt
			proj.LastMutationAt = &value
		}
	}
	return proj, nil
}

func buildRequestHistoryProjection(ctx context.Context, store memory.WorkflowStateStore, workflowID string) (*RequestHistoryProjection, error) {
	workflowID = strings.TrimSpace(workflowID)
	if store == nil || workflowID == "" {
		return nil, nil
	}
	requests, err := (archaeorequests.Service{Store: store}).ListByWorkflow(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	proj := &RequestHistoryProjection{
		WorkflowID: workflowID,
		Requests:   append([]archaeodomain.RequestRecord(nil), requests...),
	}
	for _, request := range requests {
		switch request.Status {
		case archaeodomain.RequestStatusPending, archaeodomain.RequestStatusDispatched:
			proj.Pending++
		case archaeodomain.RequestStatusRunning:
			proj.Running++
		case archaeodomain.RequestStatusCompleted:
			proj.Completed++
		case archaeodomain.RequestStatusFailed:
			proj.Failed++
		case archaeodomain.RequestStatusCanceled:
			proj.Canceled++
		case archaeodomain.RequestStatusInvalidated:
			proj.Canceled++
		case archaeodomain.RequestStatusSuperseded:
			proj.Canceled++
		}
	}
	return proj, nil
}

func buildPlanLineageProjection(ctx context.Context, store memory.WorkflowStateStore, workflowID string) (*PlanLineageProjection, error) {
	workflowID = strings.TrimSpace(workflowID)
	if store == nil || workflowID == "" {
		return nil, nil
	}
	lineage, err := (archaeoplans.Service{WorkflowStore: store}).LoadLineage(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	proj := &PlanLineageProjection{
		WorkflowID: workflowID,
	}
	if lineage == nil {
		return proj, nil
	}
	proj.Versions = append([]archaeodomain.VersionedLivingPlan(nil), lineage.Versions...)
	proj.DraftVersions = append([]archaeodomain.VersionedLivingPlan(nil), lineage.DraftVersions...)
	proj.RecomputePending = lineage.RecomputePending
	if lineage.ActiveVersion != nil {
		copy := *lineage.ActiveVersion
		proj.ActiveVersion = &copy
	}
	if lineage.LatestDraft != nil {
		copy := *lineage.LatestDraft
		proj.LatestDraft = &copy
	}
	return proj, nil
}

func buildExplorationActivityProjection(ctx context.Context, store memory.WorkflowStateStore, workflowID string) (*ExplorationActivityProjection, error) {
	workflowID = strings.TrimSpace(workflowID)
	if store == nil || workflowID == "" {
		return nil, nil
	}
	view, err := (archaeoarch.Service{Store: store, Learning: archaeolearning.Service{Store: store}}).LoadExplorationViewByWorkflow(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	timeline, _, err := (&TimelineMaterializer{Store: store, WorkflowID: workflowID}).build(ctx)
	if err != nil {
		return nil, err
	}
	proj := &ExplorationActivityProjection{WorkflowID: workflowID}
	if view != nil && view.Session != nil {
		proj.ExplorationID = view.Session.ID
		proj.LatestSnapshotID = view.Session.LatestSnapshotID
	}
	for _, entry := range timeline {
		if !explorationActivityRelevant(entry, proj.ExplorationID) {
			continue
		}
		proj.ActivityTimeline = append(proj.ActivityTimeline, entry)
		switch entry.EventType {
		case archaeoevents.EventMutationRecorded:
			proj.MutationCount++
		case archaeoevents.EventRequestCreated, archaeoevents.EventRequestDispatched, archaeoevents.EventRequestStarted, archaeoevents.EventRequestCompleted, archaeoevents.EventRequestFailed, archaeoevents.EventRequestCanceled:
			proj.RequestCount++
		case archaeoevents.EventLearningInteractionRequested, archaeoevents.EventLearningInteractionResolved, archaeoevents.EventLearningInteractionExpired:
			proj.LearningEventCount++
		}
	}
	return proj, nil
}

func buildProvenanceProjection(ctx context.Context, store memory.WorkflowStateStore, workflowID string) (*ProvenanceProjection, error) {
	workflowID = strings.TrimSpace(workflowID)
	if store == nil || workflowID == "" {
		return nil, nil
	}
	record, err := (archaeoprovenance.Service{Store: store}).Build(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	if record == nil {
		return &ProvenanceProjection{WorkflowID: workflowID}, nil
	}
	learning := make([]LearningOutcomeProvenance, 0, len(record.Learning))
	for _, item := range record.Learning {
		evidence := make([]archaeolearning.EvidenceRef, 0, len(item.EvidenceRefs))
		for _, ref := range item.EvidenceRefs {
			evidence = append(evidence, archaeolearning.EvidenceRef{RefID: ref})
		}
		learning = append(learning, LearningOutcomeProvenance{
			InteractionID:   item.InteractionID,
			SubjectType:     item.SubjectType,
			SubjectID:       item.SubjectID,
			Status:          item.Status,
			Blocking:        item.Blocking,
			BasedOnRevision: item.BasedOnRevision,
			CommentRef:      item.CommentRef,
			Evidence:        evidence,
			MutationIDs:     append([]string(nil), item.MutationIDs...),
		})
	}
	tensions := make([]TensionProvenance, 0, len(record.Tensions))
	for _, item := range record.Tensions {
		tensions = append(tensions, TensionProvenance(item))
	}
	planVersions := make([]PlanVersionProvenance, 0, len(record.PlanVersions))
	for _, item := range record.PlanVersions {
		planVersions = append(planVersions, PlanVersionProvenance(item))
	}
	return &ProvenanceProjection{
		WorkflowID:        record.WorkflowID,
		Learning:          learning,
		Tensions:          tensions,
		PlanVersions:      planVersions,
		Requests:          append([]archaeodomain.RequestProvenance(nil), record.Requests...),
		Mutations:         append([]archaeodomain.MutationProvenance(nil), record.Mutations...),
		DeferredDraftRefs: append([]string(nil), record.DeferredDraftRefs...),
		ConvergenceRefs:   append([]string(nil), record.ConvergenceRefs...),
		DecisionRefs:      append([]string(nil), record.DecisionRefs...),
		LastMutationAt:    record.LastMutationAt,
		LastRequestAt:     record.LastRequestAt,
	}, nil
}

func buildDeferredDraftProjection(ctx context.Context, store memory.WorkflowStateStore, workspaceID string) (*DeferredDraftProjection, error) {
	records, err := (archaeodeferred.Service{Store: store}).ListByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	proj := &DeferredDraftProjection{WorkspaceID: workspaceID, Records: append([]archaeodomain.DeferredDraftRecord(nil), records...)}
	for _, record := range records {
		switch record.Status {
		case archaeodomain.DeferredDraftPending:
			proj.OpenCount++
		case archaeodomain.DeferredDraftFormed:
			proj.FormedCount++
		}
	}
	return proj, nil
}

func buildDecisionTrailProjection(ctx context.Context, store memory.WorkflowStateStore, workspaceID string) (*DecisionTrailProjection, error) {
	records, err := (archaeodecisions.Service{Store: store}).ListByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	proj := &DecisionTrailProjection{WorkspaceID: workspaceID, Records: append([]archaeodomain.DecisionRecord(nil), records...)}
	for _, record := range records {
		switch record.Status {
		case archaeodomain.DecisionStatusOpen:
			proj.OpenCount++
		case archaeodomain.DecisionStatusResolved:
			proj.Resolved++
		}
	}
	return proj, nil
}

func explorationActivityRelevant(entry archaeodomain.TimelineEvent, explorationID string) bool {
	switch entry.EventType {
	case archaeoevents.EventExplorationSessionUpserted,
		archaeoevents.EventExplorationSnapshotUpserted,
		archaeoevents.EventLearningInteractionRequested,
		archaeoevents.EventLearningInteractionResolved,
		archaeoevents.EventLearningInteractionExpired,
		archaeoevents.EventTensionUpserted,
		archaeoevents.EventMutationRecorded,
		archaeoevents.EventRequestCreated,
		archaeoevents.EventRequestDispatched,
		archaeoevents.EventRequestStarted,
		archaeoevents.EventRequestCompleted,
		archaeoevents.EventRequestFailed,
		archaeoevents.EventRequestCanceled:
	default:
		return false
	}
	if strings.TrimSpace(explorationID) == "" {
		return true
	}
	if stringValue(entry.Metadata["exploration_id"]) == strings.TrimSpace(explorationID) {
		return true
	}
	return entry.EventType == archaeoevents.EventExplorationSessionUpserted || entry.EventType == archaeoevents.EventExplorationSnapshotUpserted
}

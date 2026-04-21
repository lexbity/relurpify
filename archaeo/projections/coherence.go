package projections

import (
	"context"
	"strings"

	archaeodomain "codeburg.org/lexbit/relurpify/archaeo/domain"
	archaeolearning "codeburg.org/lexbit/relurpify/archaeo/learning"
	archaeoprovenance "codeburg.org/lexbit/relurpify/archaeo/provenance"
	archaeotensions "codeburg.org/lexbit/relurpify/archaeo/tensions"
	"codeburg.org/lexbit/relurpify/framework/memory"
)

func (s *Service) Coherence(ctx context.Context, workflowID string) (*CoherenceProjection, error) {
	workflowID = strings.TrimSpace(workflowID)
	if s == nil || s.Store == nil || workflowID == "" {
		return nil, nil
	}
	return buildCoherenceProjection(ctx, s.Store, workflowID)
}

func buildCoherenceProjection(ctx context.Context, store memory.WorkflowStateStore, workflowID string) (*CoherenceProjection, error) {
	workflowID = strings.TrimSpace(workflowID)
	if store == nil || workflowID == "" {
		return nil, nil
	}
	tensionSvc := archaeotensions.Service{Store: store}
	activeTensions, err := tensionSvc.ActiveByWorkflow(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	tensionSummary, err := tensionSvc.SummaryByWorkflow(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	learningSvc := archaeolearning.Service{Store: store}
	pendingLearning, err := learningSvc.Pending(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	provenance, err := (archaeoprovenance.Service{Store: store}).Build(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	proj := &CoherenceProjection{
		WorkflowID:      workflowID,
		TensionSummary:  tensionSummary,
		ActiveTensions:  append([]archaeodomain.Tension(nil), activeTensions...),
		PendingLearning: append([]archaeolearning.Interaction(nil), pendingLearning...),
	}
	if lineage, err := buildPlanLineageProjection(ctx, store, workflowID); err != nil {
		return nil, err
	} else if lineage != nil {
		proj.ActivePlanVersion = lineage.ActiveVersion
		proj.DraftPlanVersions = append([]archaeodomain.VersionedLivingPlan(nil), lineage.DraftVersions...)
	}
	proj.ConvergenceState = latestConvergenceState(ctx, store, workflowID)
	for _, interaction := range pendingLearning {
		if interaction.Blocking {
			proj.BlockingLearningCount++
		}
	}
	if tensionSummary != nil {
		proj.AcceptedDebt = tensionSummary.AcceptedDebt
	}
	if provenance != nil {
		for _, mutation := range provenance.Mutations {
			if confidenceAffectingMutation(mutation) {
				proj.ConfidenceAffectingMutations = append(proj.ConfidenceAffectingMutations, archaeodomain.MutationEvent{
					ID:                  mutation.MutationID,
					Category:            mutation.Category,
					Impact:              mutation.Impact,
					Disposition:         mutation.Disposition,
					Blocking:            mutation.Blocking,
					SourceKind:          mutation.SourceKind,
					SourceRef:           mutation.SourceRef,
					BasedOnRevision:     mutation.BasedOnRevision,
					SemanticSnapshotRef: mutation.SemanticSnapshotRef,
					Description:         mutation.Description,
					CreatedAt:           mutation.CreatedAt,
				})
			}
			if mutation.Blocking || (mutation.Disposition != archaeodomain.DispositionContinue && mutation.Disposition != archaeodomain.DispositionContinueOnStalePlan) {
				proj.BlockingMutationCount++
			}
		}
	}
	return proj, nil
}

func confidenceAffectingMutation(mutation archaeodomain.MutationProvenance) bool {
	if mutation.Category == archaeodomain.MutationObservation {
		return false
	}
	return mutation.Impact != archaeodomain.ImpactInformational
}

package euclo

import (
	"context"
	"strings"

	archaeodomain "codeburg.org/lexbit/relurpify/archaeo/domain"
	archaeoplans "codeburg.org/lexbit/relurpify/archaeo/plans"
	archaeoprojections "codeburg.org/lexbit/relurpify/archaeo/projections"
	frameworkplan "codeburg.org/lexbit/relurpify/framework/plan"
	eucloexec "codeburg.org/lexbit/relurpify/named/euclo/execution"
)

func (a *Agent) serviceBundle() eucloexec.ServiceBundle {
	return eucloexec.ServiceBundle{
		Archaeo:        agentArchaeoAccess{agent: a},
		GraphDB:        a.GraphDB,
		RetrievalDB:    a.RetrievalDB,
		PlanStore:      a.PlanStore,
		PatternStore:   a.PatternStore,
		CommentStore:   a.CommentStore,
		WorkflowStore:  a.WorkflowStore,
		GuidanceBroker: a.GuidanceBroker,
		LearningBroker: a.LearningBroker,
		DeferralPlan:   a.DeferralPlan,
	}
}

type agentArchaeoAccess struct {
	agent *Agent
}

func (a agentArchaeoAccess) RequestHistory(ctx context.Context, workflowID string) (*eucloexec.RequestHistoryView, error) {
	if a.agent == nil {
		return nil, nil
	}
	history, err := a.agent.projectionService().RequestHistory(ctx, workflowID)
	if err != nil || history == nil {
		return nil, err
	}
	return requestHistoryView(history), nil
}

func (a agentArchaeoAccess) ActivePlan(ctx context.Context, workflowID string) (*eucloexec.ActivePlanView, error) {
	if a.agent == nil {
		return nil, nil
	}
	proj, err := a.agent.ActivePlanProjection(ctx, workflowID)
	if err != nil || proj == nil {
		return nil, err
	}
	return activePlanView(proj), nil
}

func (a agentArchaeoAccess) LearningQueue(ctx context.Context, workflowID string) (*eucloexec.LearningQueueView, error) {
	if a.agent == nil {
		return nil, nil
	}
	queue, err := a.agent.LearningQueueProjection(ctx, workflowID)
	if err != nil || queue == nil {
		return nil, err
	}
	return learningQueueView(queue), nil
}

func (a agentArchaeoAccess) TensionsByWorkflow(ctx context.Context, workflowID string) ([]eucloexec.TensionView, error) {
	if a.agent == nil {
		return nil, nil
	}
	tensions, err := a.agent.TensionsByWorkflow(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	return tensionViews(tensions), nil
}

func (a agentArchaeoAccess) TensionSummaryByWorkflow(ctx context.Context, workflowID string) (*eucloexec.TensionSummaryView, error) {
	if a.agent == nil {
		return nil, nil
	}
	summary, err := a.agent.TensionSummaryByWorkflow(ctx, workflowID)
	if err != nil || summary == nil {
		return nil, err
	}
	return tensionSummaryView(summary), nil
}

func (a agentArchaeoAccess) PlanVersions(ctx context.Context, workflowID string) ([]eucloexec.VersionedPlanView, error) {
	if a.agent == nil {
		return nil, nil
	}
	versions, err := a.agent.PlanVersions(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	return versionedPlanViews(versions), nil
}

func (a agentArchaeoAccess) ActivePlanVersion(ctx context.Context, workflowID string) (*eucloexec.VersionedPlanView, error) {
	if a.agent == nil {
		return nil, nil
	}
	version, err := a.agent.ActivePlanVersion(ctx, workflowID)
	if err != nil || version == nil {
		return nil, err
	}
	return versionedPlanView(*version), nil
}

func (a agentArchaeoAccess) DraftPlanVersion(ctx context.Context, plan *frameworkplan.LivingPlan, input eucloexec.DraftPlanInput) (*eucloexec.VersionedPlanView, error) {
	if a.agent == nil {
		return nil, nil
	}
	version, err := a.agent.planService().DraftVersion(ctx, plan, archaeoplans.DraftVersionInput{
		WorkflowID:              input.WorkflowID,
		DerivedFromExploration:  input.DerivedFromExploration,
		BasedOnRevision:         input.BasedOnRevision,
		SemanticSnapshotRef:     input.SemanticSnapshotRef,
		CommentRefs:             append([]string(nil), input.CommentRefs...),
		TensionRefs:             append([]string(nil), input.TensionRefs...),
		PatternRefs:             append([]string(nil), input.PatternRefs...),
		AnchorRefs:              append([]string(nil), input.AnchorRefs...),
		FormationResultRef:      input.FormationResultRef,
		FormationProvenanceRefs: append([]string(nil), input.FormationProvenanceRefs...),
	})
	if err != nil || version == nil {
		return nil, err
	}
	return versionedPlanView(*version), nil
}

func (a agentArchaeoAccess) ActivatePlanVersion(ctx context.Context, workflowID string, version int) (*eucloexec.VersionedPlanView, error) {
	if a.agent == nil {
		return nil, nil
	}
	versioned, err := a.agent.planService().ActivateVersion(ctx, workflowID, version)
	if err != nil || versioned == nil {
		return nil, err
	}
	return versionedPlanView(*versioned), nil
}

func versionedPlanView(version archaeodomain.VersionedLivingPlan) *eucloexec.VersionedPlanView {
	return &eucloexec.VersionedPlanView{
		ID:                     version.ID,
		WorkflowID:             version.WorkflowID,
		PlanID:                 version.Plan.ID,
		Version:                version.Version,
		Status:                 string(version.Status),
		DerivedFromExploration: version.DerivedFromExploration,
		BasedOnRevision:        version.BasedOnRevision,
		SemanticSnapshotRef:    version.SemanticSnapshotRef,
		PatternRefs:            append([]string(nil), version.PatternRefs...),
		AnchorRefs:             append([]string(nil), version.AnchorRefs...),
		TensionRefs:            append([]string(nil), version.TensionRefs...),
		Plan:                   version.Plan,
	}
}

func requestHistoryView(history *archaeoprojections.RequestHistoryProjection) *eucloexec.RequestHistoryView {
	view := &eucloexec.RequestHistoryView{
		WorkflowID: history.WorkflowID,
		Pending:    history.Pending,
		Running:    history.Running,
		Completed:  history.Completed,
		Failed:     history.Failed,
		Canceled:   history.Canceled,
		Requests:   make([]eucloexec.RequestRecordView, 0, len(history.Requests)),
	}
	for _, request := range history.Requests {
		view.Requests = append(view.Requests, eucloexec.RequestRecordView{
			ID:        request.ID,
			Kind:      string(request.Kind),
			Scope:     strings.Join(request.SubjectRefs, ","),
			Status:    string(request.Status),
			Summary:   firstNonEmptyStringValue(strings.TrimSpace(request.Title), strings.TrimSpace(request.Description)),
			CreatedAt: request.RequestedAt,
			UpdatedAt: request.UpdatedAt,
		})
	}
	return view
}

func activePlanView(proj *archaeoprojections.ActivePlanProjection) *eucloexec.ActivePlanView {
	view := &eucloexec.ActivePlanView{WorkflowID: proj.WorkflowID}
	if proj.PhaseState != nil {
		view.Phase = string(proj.PhaseState.CurrentPhase)
	}
	if proj.ActivePlanVersion != nil {
		view.ActivePlan = versionedPlanView(*proj.ActivePlanVersion)
		for _, stepID := range proj.ActivePlanVersion.Plan.StepOrder {
			if step := proj.ActivePlanVersion.Plan.Steps[stepID]; step != nil && step.Status == frameworkplan.PlanStepInProgress {
				view.ActiveStepID = stepID
				break
			}
		}
	}
	return view
}

func learningQueueView(queue *archaeoprojections.LearningQueueProjection) *eucloexec.LearningQueueView {
	view := &eucloexec.LearningQueueView{
		WorkflowID:         queue.WorkflowID,
		PendingGuidanceIDs: append([]string(nil), queue.PendingGuidanceIDs...),
		BlockingLearning:   append([]string(nil), queue.BlockingLearning...),
		PendingLearning:    make([]eucloexec.LearningInteractionView, 0, len(queue.PendingLearning)),
	}
	for _, item := range queue.PendingLearning {
		evidence := make([]string, 0, len(item.Evidence))
		for _, ref := range item.Evidence {
			evidence = append(evidence, strings.TrimSpace(ref.RefID))
		}
		view.PendingLearning = append(view.PendingLearning, eucloexec.LearningInteractionView{
			ID:        item.ID,
			Status:    string(item.Status),
			Blocking:  item.Blocking,
			Prompt:    firstNonEmptyStringValue(strings.TrimSpace(item.Title), strings.TrimSpace(item.Description)),
			SubjectID: item.SubjectID,
			Evidence:  evidence,
		})
	}
	return view
}

func tensionViews(tensions []archaeodomain.Tension) []eucloexec.TensionView {
	out := make([]eucloexec.TensionView, 0, len(tensions))
	for _, tension := range tensions {
		out = append(out, eucloexec.TensionView{
			ID:                 tension.ID,
			Kind:               tension.Kind,
			Description:        tension.Description,
			Severity:           tension.Severity,
			Status:             string(tension.Status),
			PatternIDs:         append([]string(nil), tension.PatternIDs...),
			AnchorRefs:         append([]string(nil), tension.AnchorRefs...),
			SymbolScope:        append([]string(nil), tension.SymbolScope...),
			RelatedPlanStepIDs: append([]string(nil), tension.RelatedPlanStepIDs...),
			BasedOnRevision:    tension.BasedOnRevision,
		})
	}
	return out
}

func tensionSummaryView(summary *archaeodomain.TensionSummary) *eucloexec.TensionSummaryView {
	return &eucloexec.TensionSummaryView{
		WorkflowID: summary.WorkflowID,
		Total:      summary.Total,
		Active:     summary.Active,
		Accepted:   summary.Accepted,
		Resolved:   summary.Resolved,
		Unresolved: summary.Unresolved,
	}
}

func versionedPlanViews(versions []archaeodomain.VersionedLivingPlan) []eucloexec.VersionedPlanView {
	out := make([]eucloexec.VersionedPlanView, 0, len(versions))
	for _, version := range versions {
		out = append(out, *versionedPlanView(version))
	}
	return out
}

func firstNonEmptyStringValue(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

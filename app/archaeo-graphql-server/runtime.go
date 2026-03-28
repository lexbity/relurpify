package archaeographqlserver

import (
	"context"
	"strings"
	"time"

	archaeoarch "github.com/lexcodex/relurpify/archaeo/archaeology"
	relurpishbindings "github.com/lexcodex/relurpify/archaeo/bindings/relurpish"
	archaeoconvergence "github.com/lexcodex/relurpify/archaeo/convergence"
	archaeodecisions "github.com/lexcodex/relurpify/archaeo/decisions"
	archaeodeferred "github.com/lexcodex/relurpify/archaeo/deferred"
	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeoexec "github.com/lexcodex/relurpify/archaeo/execution"
	archaeolearning "github.com/lexcodex/relurpify/archaeo/learning"
	archaeoplans "github.com/lexcodex/relurpify/archaeo/plans"
	archaeoprojections "github.com/lexcodex/relurpify/archaeo/projections"
	archaeorequests "github.com/lexcodex/relurpify/archaeo/requests"
	archaeotensions "github.com/lexcodex/relurpify/archaeo/tensions"
	"github.com/lexcodex/relurpify/framework/core"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
)

// Runtime is the app-level GraphQL runtime. It delegates operations to
// archaeology bindings and runtime services without reimplementing domain rules.
type Runtime struct {
	Bindings     relurpishbindings.Runtime
	PollInterval time.Duration
}

func (r Runtime) archaeologyService() archaeoarch.Service {
	return archaeoarch.Service{
		Store:    r.Bindings.WorkflowStore,
		Plans:    r.planService(),
		Learning: r.learningService(),
		Requests: r.requestService(),
		EvaluateGate: func(context.Context, *core.Task, *core.Context, *frameworkplan.LivingPlan, *frameworkplan.PlanStep) (archaeoexec.PreflightOutcome, error) {
			return archaeoexec.PreflightOutcome{}, nil
		},
	}
}

func (r Runtime) planService() archaeoplans.Service {
	return archaeoplans.Service{Store: r.Bindings.PlanStore, WorkflowStore: r.Bindings.WorkflowStore}
}

func (r Runtime) learningService() archaeolearning.Service {
	return r.Bindings.LearningService()
}

func (r Runtime) tensionService() archaeotensions.Service {
	return archaeotensions.Service{Store: r.Bindings.WorkflowStore}
}

func (r Runtime) projectionService() *archaeoprojections.Service {
	return &archaeoprojections.Service{Store: r.Bindings.WorkflowStore}
}

func (r Runtime) requestService() archaeorequests.Service {
	return archaeorequests.Service{Store: r.Bindings.WorkflowStore}
}

func (r Runtime) deferredService() archaeodeferred.Service {
	return archaeodeferred.Service{Store: r.Bindings.WorkflowStore}
}

func (r Runtime) convergenceService() archaeoconvergence.Service {
	return archaeoconvergence.Service{Store: r.Bindings.WorkflowStore}
}

func (r Runtime) decisionService() archaeodecisions.Service {
	return archaeodecisions.Service{Store: r.Bindings.WorkflowStore}
}

func (r Runtime) ActiveExploration(ctx context.Context, workspaceID string) (*archaeoarch.SessionView, error) {
	return r.Bindings.ActiveExploration(ctx, workspaceID)
}

func (r Runtime) ExplorationView(ctx context.Context, explorationID string) (*archaeoarch.SessionView, error) {
	return r.Bindings.ExplorationView(ctx, explorationID)
}

func (r Runtime) ExplorationByWorkflow(ctx context.Context, workflowID string) (*archaeodomain.ExplorationSession, error) {
	return r.archaeologyService().LoadExplorationByWorkflow(ctx, workflowID)
}

func (r Runtime) LearningQueue(ctx context.Context, workflowID string) ([]archaeolearning.Interaction, error) {
	return r.learningService().Pending(ctx, workflowID)
}

func (r Runtime) WorkflowProjection(ctx context.Context, workflowID string) (*archaeoprojections.WorkflowReadModel, error) {
	return r.projectionService().Workflow(ctx, workflowID)
}

func (r Runtime) TimelineProjection(ctx context.Context, workflowID string) (*archaeoprojections.TimelineProjection, error) {
	return r.projectionService().TimelineProjection(ctx, workflowID)
}

func (r Runtime) MutationHistory(ctx context.Context, workflowID string) (*archaeoprojections.MutationHistoryProjection, error) {
	return r.projectionService().MutationHistory(ctx, workflowID)
}

func (r Runtime) RequestHistory(ctx context.Context, workflowID string) (*archaeoprojections.RequestHistoryProjection, error) {
	return r.projectionService().RequestHistory(ctx, workflowID)
}

func (r Runtime) Provenance(ctx context.Context, workflowID string) (*archaeoprojections.ProvenanceProjection, error) {
	return r.projectionService().Provenance(ctx, workflowID)
}

func (r Runtime) Coherence(ctx context.Context, workflowID string) (*archaeoprojections.CoherenceProjection, error) {
	return r.projectionService().Coherence(ctx, workflowID)
}

func (r Runtime) Tensions(ctx context.Context, workflowID string) ([]archaeodomain.Tension, error) {
	return r.tensionService().ListByWorkflow(ctx, workflowID)
}

func (r Runtime) TensionSummary(ctx context.Context, workflowID string) (*archaeodomain.TensionSummary, error) {
	return r.tensionService().SummaryByWorkflow(ctx, workflowID)
}

func (r Runtime) ActivePlanVersion(ctx context.Context, workflowID string) (*archaeodomain.VersionedLivingPlan, error) {
	return r.planService().LoadActiveVersion(ctx, workflowID)
}

func (r Runtime) PlanLineage(ctx context.Context, workflowID string) (*archaeoprojections.PlanLineageProjection, error) {
	return r.projectionService().PlanLineage(ctx, workflowID)
}

func (r Runtime) ComparePlanVersions(ctx context.Context, workflowID string, left, right int) (Map, error) {
	diff, err := r.planService().CompareVersions(ctx, workflowID, left, right)
	if err != nil {
		return nil, err
	}
	return Map(diff), nil
}

func (r Runtime) DeferredDrafts(ctx context.Context, workspaceID string, limit int) (*archaeoprojections.DeferredDraftProjection, error) {
	proj, err := r.projectionService().DeferredDrafts(ctx, workspaceID)
	if err != nil || proj == nil {
		return proj, err
	}
	proj.Records = limitDeferred(proj.Records, limit)
	return proj, nil
}

func (r Runtime) CurrentConvergence(ctx context.Context, workspaceID string) (*archaeodomain.ConvergenceRecord, error) {
	proj, err := r.convergenceService().CurrentByWorkspace(ctx, workspaceID)
	if err != nil || proj == nil {
		return nil, err
	}
	return proj.Current, nil
}

func (r Runtime) ConvergenceHistory(ctx context.Context, workspaceID string, limit int) (*archaeodomain.WorkspaceConvergenceProjection, error) {
	proj, err := r.projectionService().ConvergenceHistory(ctx, workspaceID)
	if err != nil || proj == nil {
		return proj, err
	}
	proj.History = limitConvergence(proj.History, limit)
	return proj, nil
}

func (r Runtime) DecisionTrail(ctx context.Context, workspaceID string, limit int) (*archaeoprojections.DecisionTrailProjection, error) {
	proj, err := r.projectionService().DecisionTrail(ctx, workspaceID)
	if err != nil || proj == nil {
		return proj, err
	}
	proj.Records = limitDecisions(proj.Records, limit)
	return proj, nil
}

func (r Runtime) WorkspaceSummary(ctx context.Context, workspaceID string) (*WorkspaceSummary, error) {
	deferredSummary, err := r.deferredService().SummaryByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	decisionSummary, err := r.decisionService().SummaryByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	convergenceProj, err := r.convergenceService().CurrentByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	summary := &WorkspaceSummary{
		WorkspaceID:              strings.TrimSpace(workspaceID),
		DeferredDraftOpenCount:   deferredSummary[archaeodomain.DeferredDraftPending],
		DeferredDraftFormedCount: deferredSummary[archaeodomain.DeferredDraftFormed],
		DecisionOpenCount:        decisionSummary[archaeodomain.DecisionStatusOpen],
		DecisionResolvedCount:    decisionSummary[archaeodomain.DecisionStatusResolved],
	}
	if convergenceProj != nil {
		summary.CurrentConvergence = convergenceProj.Current
		summary.ConvergenceOpenCount = convergenceProj.OpenCount
		summary.ConvergenceResolvedCount = convergenceProj.ResolvedCount
		summary.ConvergenceDeferredCount = convergenceProj.DeferredCount
	}
	return summary, nil
}

func (r Runtime) PendingRequests(ctx context.Context, workflowID string) ([]archaeodomain.RequestRecord, error) {
	return r.requestService().Pending(ctx, workflowID)
}

func (r Runtime) Request(ctx context.Context, workflowID, requestID string) (*archaeodomain.RequestRecord, error) {
	record, _, err := r.requestService().Load(ctx, workflowID, requestID)
	return record, err
}

func (r Runtime) ResolveLearningInteraction(ctx context.Context, input archaeolearning.ResolveInput) (*archaeolearning.Interaction, error) {
	return r.learningService().Resolve(ctx, input)
}

func (r Runtime) UpdateTensionStatus(ctx context.Context, workflowID, tensionID string, status archaeodomain.TensionStatus, commentRefs []string) (*archaeodomain.Tension, error) {
	return r.tensionService().UpdateStatus(ctx, workflowID, tensionID, status, commentRefs)
}

func (r Runtime) ActivatePlanVersion(ctx context.Context, workflowID string, version int) (*archaeodomain.VersionedLivingPlan, error) {
	return r.planService().ActivateVersion(ctx, workflowID, version)
}

func (r Runtime) ArchivePlanVersion(ctx context.Context, workflowID string, version int, reason string) (*archaeodomain.VersionedLivingPlan, error) {
	return r.planService().ArchiveVersion(ctx, workflowID, version, reason)
}

func (r Runtime) MarkPlanVersionStale(ctx context.Context, workflowID string, version int, reason string) (*archaeodomain.VersionedLivingPlan, error) {
	return r.planService().MarkVersionStale(ctx, workflowID, version, reason)
}

func (r Runtime) MarkExplorationStale(ctx context.Context, explorationID, reason string) (*archaeodomain.ExplorationSession, error) {
	return r.archaeologyService().MarkExplorationStale(ctx, explorationID, reason)
}

func (r Runtime) PrepareLivingPlan(ctx context.Context, input PrepareLivingPlanInput) (*PrepareLivingPlanPayload, error) {
	state := core.NewContext()
	if strings.TrimSpace(input.SemanticSnapshotRef) != "" {
		state.Set("euclo.semantic_snapshot_ref", strings.TrimSpace(input.SemanticSnapshotRef))
	}
	task := &core.Task{
		Instruction: strings.TrimSpace(input.Instruction),
		Context: map[string]any{
			"workflow_id":       strings.TrimSpace(input.WorkflowID),
			"workspace":         strings.TrimSpace(input.WorkspaceID),
			"corpus_scope":      strings.TrimSpace(input.CorpusScope),
			"symbol_scope":      strings.TrimSpace(input.SymbolScope),
			"based_on_revision": strings.TrimSpace(input.BasedOnRevision),
		},
	}
	out := r.archaeologyService().PrepareLivingPlan(ctx, task, state, strings.TrimSpace(input.WorkflowID))
	payload := &PrepareLivingPlanPayload{
		WorkflowID:                  strings.TrimSpace(input.WorkflowID),
		ActiveExplorationID:         strings.TrimSpace(state.GetString("euclo.active_exploration_id")),
		ActiveExplorationSnapshotID: strings.TrimSpace(state.GetString("euclo.active_exploration_snapshot_id")),
		Success:                     out.Err == nil,
	}
	if out.Step != nil {
		payload.StepID = strings.TrimSpace(out.Step.ID)
	}
	if out.Result != nil && len(out.Result.Data) > 0 {
		payload.Result = Map(out.Result.Data)
	}
	if out.Err != nil {
		payload.Error = out.Err.Error()
	}
	if version, err := r.planService().LoadActiveVersion(ctx, input.WorkflowID); err == nil {
		payload.Plan = version
	}
	return payload, out.Err
}

func (r Runtime) RefreshExplorationSnapshot(ctx context.Context, input RefreshExplorationSnapshotInput) (*archaeodomain.ExplorationSnapshot, error) {
	svc := r.archaeologyService()
	snapshot, err := svc.LoadExplorationSnapshotByWorkflow(ctx, input.WorkflowID, input.SnapshotID)
	if err != nil || snapshot == nil {
		return snapshot, err
	}
	return svc.UpdateExplorationSnapshot(ctx, snapshot, archaeoarch.SnapshotInput{
		BasedOnRevision:      strings.TrimSpace(input.BasedOnRevision),
		SemanticSnapshotRef:  strings.TrimSpace(input.SemanticSnapshotRef),
		CandidatePatternRefs: cloneStrings(input.CandidatePatternRefs),
		CandidateAnchorRefs:  cloneStrings(input.CandidateAnchorRefs),
		TensionIDs:           cloneStrings(input.TensionIDs),
		OpenLearningIDs:      cloneStrings(input.OpenLearningIDs),
		Summary:              strings.TrimSpace(input.Summary),
	})
}

func (r Runtime) CreateOrUpdateDeferredDraft(ctx context.Context, input archaeodeferred.CreateInput) (*archaeodomain.DeferredDraftRecord, error) {
	return r.deferredService().CreateOrUpdate(ctx, input)
}

func (r Runtime) FinalizeDeferredDraft(ctx context.Context, input archaeodeferred.FinalizeInput) (*archaeodomain.DeferredDraftRecord, error) {
	return r.deferredService().Finalize(ctx, input)
}

func (r Runtime) CreateConvergenceRecord(ctx context.Context, input archaeoconvergence.CreateInput) (*archaeodomain.ConvergenceRecord, error) {
	return r.convergenceService().Create(ctx, input)
}

func (r Runtime) ResolveConvergenceRecord(ctx context.Context, input archaeoconvergence.ResolveInput) (*archaeodomain.ConvergenceRecord, error) {
	return r.convergenceService().Resolve(ctx, input)
}

func (r Runtime) CreateDecisionRecord(ctx context.Context, input archaeodecisions.CreateInput) (*archaeodomain.DecisionRecord, error) {
	return r.decisionService().Create(ctx, input)
}

func (r Runtime) ResolveDecisionRecord(ctx context.Context, input archaeodecisions.ResolveInput) (*archaeodomain.DecisionRecord, error) {
	return r.decisionService().Resolve(ctx, input)
}

func (r Runtime) DispatchRequest(ctx context.Context, workflowID, requestID string, metadata map[string]any) (*archaeodomain.RequestRecord, error) {
	return r.requestService().Dispatch(ctx, workflowID, requestID, metadata)
}

func (r Runtime) ClaimRequest(ctx context.Context, input archaeorequests.ClaimInput) (*archaeodomain.RequestRecord, error) {
	return r.requestService().Claim(ctx, input)
}

func (r Runtime) RenewRequestClaim(ctx context.Context, input archaeorequests.RenewInput) (*archaeodomain.RequestRecord, error) {
	return r.requestService().Renew(ctx, input)
}

func (r Runtime) ReleaseRequestClaim(ctx context.Context, workflowID, requestID string) (*archaeodomain.RequestRecord, error) {
	return r.requestService().Release(ctx, workflowID, requestID)
}

func (r Runtime) ApplyRequestFulfillment(ctx context.Context, input archaeorequests.ApplyFulfillmentInput) (*ApplyRequestFulfillmentPayload, error) {
	record, validity, err := r.requestService().ApplyFulfillment(ctx, input)
	return &ApplyRequestFulfillmentPayload{Request: record, Validity: validity}, err
}

func (r Runtime) FailRequest(ctx context.Context, workflowID, requestID, errorText string, retry bool) (*archaeodomain.RequestRecord, error) {
	return r.requestService().Fail(ctx, workflowID, requestID, errorText, retry)
}

func (r Runtime) InvalidateRequest(ctx context.Context, workflowID, requestID, reason string, conflictingRefs []string) (*archaeodomain.RequestRecord, error) {
	return r.requestService().Invalidate(ctx, workflowID, requestID, reason, conflictingRefs)
}

func (r Runtime) SupersedeRequest(ctx context.Context, workflowID, requestID, successorID, reason string) (*archaeodomain.RequestRecord, error) {
	return r.requestService().Supersede(ctx, workflowID, requestID, successorID, reason)
}

func (r Runtime) pollInterval() time.Duration {
	if r.PollInterval > 0 {
		return r.PollInterval
	}
	return 100 * time.Millisecond
}

func limitDeferred(records []archaeodomain.DeferredDraftRecord, limit int) []archaeodomain.DeferredDraftRecord {
	if limit <= 0 || len(records) <= limit {
		return records
	}
	return append([]archaeodomain.DeferredDraftRecord(nil), records[:limit]...)
}

func limitConvergence(records []archaeodomain.ConvergenceRecord, limit int) []archaeodomain.ConvergenceRecord {
	if limit <= 0 || len(records) <= limit {
		return records
	}
	return append([]archaeodomain.ConvergenceRecord(nil), records[:limit]...)
}

func limitDecisions(records []archaeodomain.DecisionRecord, limit int) []archaeodomain.DecisionRecord {
	if limit <= 0 || len(records) <= limit {
		return records
	}
	return append([]archaeodomain.DecisionRecord(nil), records[:limit]...)
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return append([]string(nil), values...)
}

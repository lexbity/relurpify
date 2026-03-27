package relurpishbindings

import (
	"context"

	archaeoarch "github.com/lexcodex/relurpify/archaeo/archaeology"
	archaeoconvergence "github.com/lexcodex/relurpify/archaeo/convergence"
	archaeodecisions "github.com/lexcodex/relurpify/archaeo/decisions"
	archaeodeferred "github.com/lexcodex/relurpify/archaeo/deferred"
	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeolearning "github.com/lexcodex/relurpify/archaeo/learning"
	archaeophases "github.com/lexcodex/relurpify/archaeo/phases"
	archaeoplans "github.com/lexcodex/relurpify/archaeo/plans"
	archaeoprojections "github.com/lexcodex/relurpify/archaeo/projections"
	archaeoretrieval "github.com/lexcodex/relurpify/archaeo/retrieval"
	archaeotensions "github.com/lexcodex/relurpify/archaeo/tensions"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/patterns"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
)

// Runtime exposes direct archaeology runtime surfaces for relurpish and other
// in-process consumers that should not be forced through transport.
type Runtime struct {
	WorkflowStore  memory.WorkflowStateStore
	PlanStore      frameworkplan.PlanStore
	PatternStore   patterns.PatternStore
	CommentStore   patterns.CommentStore
	Retrieval      archaeoretrieval.Store
	LearningBroker *archaeolearning.Broker
}

func (r Runtime) ArchaeologyService() archaeoarch.Service {
	return archaeoarch.Service{
		Store:    r.WorkflowStore,
		Plans:    r.PlanService(),
		Learning: r.LearningService(),
	}
}

func (r Runtime) PhaseService() archaeophases.Service {
	return archaeophases.Service{Store: r.WorkflowStore}
}

func (r Runtime) PlanService() archaeoplans.Service {
	return archaeoplans.Service{Store: r.PlanStore, WorkflowStore: r.WorkflowStore}
}

func (r Runtime) LearningService() archaeolearning.Service {
	service := archaeolearning.Service{
		Store:        r.WorkflowStore,
		PatternStore: r.PatternStore,
		CommentStore: r.CommentStore,
		PlanStore:    r.PlanStore,
		Retrieval:    r.Retrieval,
		Broker:       r.LearningBroker,
	}
	phaseService := r.PhaseService()
	if phaseService.Store != nil {
		service.Phases = &phaseService
	}
	return service
}

func (r Runtime) TensionService() archaeotensions.Service {
	return archaeotensions.Service{Store: r.WorkflowStore}
}

func (r Runtime) ProjectionService() *archaeoprojections.Service {
	return &archaeoprojections.Service{Store: r.WorkflowStore}
}

func (r Runtime) DeferredDraftService() archaeodeferred.Service {
	return archaeodeferred.Service{Store: r.WorkflowStore}
}

func (r Runtime) ConvergenceService() archaeoconvergence.Service {
	return archaeoconvergence.Service{Store: r.WorkflowStore}
}

func (r Runtime) DecisionService() archaeodecisions.Service {
	return archaeodecisions.Service{Store: r.WorkflowStore}
}

func (r Runtime) ActiveExploration(ctx context.Context, workspaceID string) (*archaeoarch.SessionView, error) {
	session, err := r.ArchaeologyService().LoadActiveExplorationByWorkspace(ctx, workspaceID)
	if err != nil || session == nil {
		return nil, err
	}
	return r.ArchaeologyService().LoadExplorationView(ctx, session.ID)
}

func (r Runtime) ExplorationView(ctx context.Context, explorationID string) (*archaeoarch.SessionView, error) {
	return r.ArchaeologyService().LoadExplorationView(ctx, explorationID)
}

func (r Runtime) PendingLearning(ctx context.Context, workflowID string) ([]archaeolearning.Interaction, error) {
	return r.LearningService().Pending(ctx, workflowID)
}

func (r Runtime) ResolveLearning(ctx context.Context, input archaeolearning.ResolveInput) (*archaeolearning.Interaction, error) {
	return r.LearningService().Resolve(ctx, input)
}

func (r Runtime) TensionsByWorkflow(ctx context.Context, workflowID string) ([]archaeodomain.Tension, error) {
	return r.TensionService().ListByWorkflow(ctx, workflowID)
}

func (r Runtime) TensionsByExploration(ctx context.Context, explorationID string) ([]archaeodomain.Tension, error) {
	return r.TensionService().ListByExploration(ctx, explorationID)
}

func (r Runtime) UpdateTensionStatus(ctx context.Context, workflowID, tensionID string, status archaeodomain.TensionStatus, commentRefs []string) (*archaeodomain.Tension, error) {
	return r.TensionService().UpdateStatus(ctx, workflowID, tensionID, status, commentRefs)
}

func (r Runtime) TensionSummaryByWorkflow(ctx context.Context, workflowID string) (*archaeodomain.TensionSummary, error) {
	return r.TensionService().SummaryByWorkflow(ctx, workflowID)
}

func (r Runtime) TensionSummaryByExploration(ctx context.Context, explorationID string) (*archaeodomain.TensionSummary, error) {
	return r.TensionService().SummaryByExploration(ctx, explorationID)
}

func (r Runtime) WorkflowProjection(ctx context.Context, workflowID string) (*archaeoprojections.WorkflowReadModel, error) {
	return r.ProjectionService().Workflow(ctx, workflowID)
}

func (r Runtime) ExplorationProjection(ctx context.Context, workflowID string) (*archaeoprojections.ExplorationProjection, error) {
	return r.ProjectionService().Exploration(ctx, workflowID)
}

func (r Runtime) LearningQueueProjection(ctx context.Context, workflowID string) (*archaeoprojections.LearningQueueProjection, error) {
	return r.ProjectionService().LearningQueue(ctx, workflowID)
}

func (r Runtime) ActivePlanProjection(ctx context.Context, workflowID string) (*archaeoprojections.ActivePlanProjection, error) {
	return r.ProjectionService().ActivePlan(ctx, workflowID)
}

func (r Runtime) WorkflowTimeline(ctx context.Context, workflowID string) ([]archaeodomain.TimelineEvent, error) {
	return r.ProjectionService().Timeline(ctx, workflowID)
}

func (r Runtime) SubscribeWorkflowProjection(workflowID string, buffer int) (<-chan archaeoprojections.ProjectionEvent, func()) {
	return r.ProjectionService().SubscribeWorkflow(workflowID, buffer)
}

func (r Runtime) PlanVersions(ctx context.Context, workflowID string) ([]archaeodomain.VersionedLivingPlan, error) {
	return r.PlanService().ListVersions(ctx, workflowID)
}

func (r Runtime) ActivePlanVersion(ctx context.Context, workflowID string) (*archaeodomain.VersionedLivingPlan, error) {
	return r.PlanService().LoadActiveVersion(ctx, workflowID)
}

func (r Runtime) ComparePlanVersions(ctx context.Context, workflowID string, fromVersion, toVersion int) (map[string]any, error) {
	return r.PlanService().CompareVersions(ctx, workflowID, fromVersion, toVersion)
}

func (r Runtime) DeferredDrafts(ctx context.Context, workspaceID string) (*archaeoprojections.DeferredDraftProjection, error) {
	return r.ProjectionService().DeferredDrafts(ctx, workspaceID)
}

func (r Runtime) ConvergenceHistory(ctx context.Context, workspaceID string) (*archaeodomain.WorkspaceConvergenceProjection, error) {
	return r.ProjectionService().ConvergenceHistory(ctx, workspaceID)
}

func (r Runtime) DecisionTrail(ctx context.Context, workspaceID string) (*archaeoprojections.DecisionTrailProjection, error) {
	return r.ProjectionService().DecisionTrail(ctx, workspaceID)
}

func (r Runtime) CreateConvergenceRecord(ctx context.Context, input archaeoconvergence.CreateInput) (*archaeodomain.ConvergenceRecord, error) {
	return r.ConvergenceService().Create(ctx, input)
}

func (r Runtime) ResolveConvergenceRecord(ctx context.Context, input archaeoconvergence.ResolveInput) (*archaeodomain.ConvergenceRecord, error) {
	return r.ConvergenceService().Resolve(ctx, input)
}

func (r Runtime) CreateDecisionRecord(ctx context.Context, input archaeodecisions.CreateInput) (*archaeodomain.DecisionRecord, error) {
	return r.DecisionService().Create(ctx, input)
}

func (r Runtime) ResolveDecisionRecord(ctx context.Context, input archaeodecisions.ResolveInput) (*archaeodomain.DecisionRecord, error) {
	return r.DecisionService().Resolve(ctx, input)
}

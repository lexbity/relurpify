package euclobindings

import (
	"context"
	"time"

	archaeoarch "github.com/lexcodex/relurpify/archaeo/archaeology"
	archaeoconvergence "github.com/lexcodex/relurpify/archaeo/convergence"
	archaeodecisions "github.com/lexcodex/relurpify/archaeo/decisions"
	archaeodeferred "github.com/lexcodex/relurpify/archaeo/deferred"
	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeoexec "github.com/lexcodex/relurpify/archaeo/execution"
	archaeolearning "github.com/lexcodex/relurpify/archaeo/learning"
	archaeophases "github.com/lexcodex/relurpify/archaeo/phases"
	archaeoplans "github.com/lexcodex/relurpify/archaeo/plans"
	archaeoprojections "github.com/lexcodex/relurpify/archaeo/projections"
	archaeoproviders "github.com/lexcodex/relurpify/archaeo/providers"
	archaeorequests "github.com/lexcodex/relurpify/archaeo/requests"
	archaeoretrieval "github.com/lexcodex/relurpify/archaeo/retrieval"
	archaeotensions "github.com/lexcodex/relurpify/archaeo/tensions"
	archaeoverification "github.com/lexcodex/relurpify/archaeo/verification"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/guidance"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/patterns"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
)

// Runtime wires euclo-facing archaeology services from shared runtime
// dependencies without importing named/euclo back into archaeo.
type Runtime struct {
	WorkflowStore  memory.WorkflowStateStore
	PlanStore      frameworkplan.PlanStore
	PatternStore   patterns.PatternStore
	CommentStore   patterns.CommentStore
	Retrieval      archaeoretrieval.Store
	ConvVerifier   frameworkplan.ConvergenceVerifier
	GuidanceBroker *guidance.GuidanceBroker
	LearningBroker *archaeolearning.Broker
	DeferralPolicy guidance.DeferralPolicy
	MutationPolicy archaeoexec.MutationPolicy
	Providers      archaeoproviders.Bundle
	Now            func() time.Time
	NewID          func(string) string
}

type ArchaeologyConfig struct {
	PersistPhase archaeoarch.PhasePersister
	EvaluateGate archaeoarch.GateEvaluator
	ResetDoom    archaeoarch.DoomLoopReset
}

type DriverConfig struct {
	Handoff func(context.Context, *core.Task, *core.Context, *frameworkplan.PlanStep) error
}

type FinalizerConfig struct {
	GitCheckpoint func(context.Context, *core.Task) string
}

type PreflightConfig struct {
	RequestGuidance func(context.Context, guidance.GuidanceRequest, string) guidance.GuidanceDecision
}

func (r Runtime) PhaseService() archaeophases.Service {
	return archaeophases.Service{Store: r.WorkflowStore}
}

func (r Runtime) ArchaeologyService(cfg ArchaeologyConfig) archaeoarch.Service {
	return archaeoarch.Service{
		Store:        r.WorkflowStore,
		Plans:        r.PlanService(),
		PersistPhase: cfg.PersistPhase,
		EvaluateGate: cfg.EvaluateGate,
		ResetDoom:    cfg.ResetDoom,
		Learning:     r.LearningService(),
		Providers:    r.Providers,
		Requests:     r.RequestService(),
		Now:          r.Now,
		NewID:        r.NewID,
	}
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

func (r Runtime) PlanService() archaeoplans.Service {
	return archaeoplans.Service{Store: r.PlanStore, WorkflowStore: r.WorkflowStore, Now: r.Now}
}

func (r Runtime) TensionService() archaeotensions.Service {
	return archaeotensions.Service{Store: r.WorkflowStore, Now: r.Now, NewID: r.NewID}
}

func (r Runtime) RequestService() archaeorequests.Service {
	return archaeorequests.Service{Store: r.WorkflowStore, Now: r.Now, NewID: r.NewID}
}

func (r Runtime) VerificationService() archaeoverification.Service {
	return archaeoverification.Service{
		Store:    r.PlanStore,
		Workflow: r.WorkflowStore,
		Verifier: r.ConvVerifier,
		Tensions: r.TensionService(),
	}
}

func (r Runtime) ProjectionService() *archaeoprojections.Service {
	return &archaeoprojections.Service{Store: r.WorkflowStore}
}

func (r Runtime) DeferredDraftService() archaeodeferred.Service {
	return archaeodeferred.Service{Store: r.WorkflowStore, Now: r.Now, NewID: r.NewID}
}

func (r Runtime) ConvergenceService() archaeoconvergence.Service {
	return archaeoconvergence.Service{Store: r.WorkflowStore, Now: r.Now, NewID: r.NewID}
}

func (r Runtime) DecisionService() archaeodecisions.Service {
	return archaeodecisions.Service{Store: r.WorkflowStore, Now: r.Now, NewID: r.NewID}
}

func (r Runtime) ExecutionService() archaeoexec.Service {
	return archaeoexec.Service{
		GuidanceBroker: r.GuidanceBroker,
		DeferralPolicy: r.DeferralPolicy,
		WorkflowStore:  r.WorkflowStore,
		Retrieval:      r.Retrieval,
		MutationPolicy: r.MutationPolicy,
		Now:            r.Now,
	}
}

func (r Runtime) ExecutionHandoffRecorder() archaeoexec.HandoffRecorder {
	return archaeoexec.HandoffRecorder{Store: r.WorkflowStore, Now: r.Now, NewID: r.NewID}
}

func (r Runtime) PreflightCoordinator(cfg PreflightConfig) archaeoexec.PreflightCoordinator {
	return archaeoexec.PreflightCoordinator{
		Service:         r.ExecutionService(),
		Plans:           r.PlanService(),
		RequestGuidance: cfg.RequestGuidance,
	}
}

func (r Runtime) ExecutionFinalizer(cfg FinalizerConfig) archaeoexec.Finalizer {
	return archaeoexec.Finalizer{
		Plans:         r.PlanService(),
		Verification:  r.VerificationService(),
		GitCheckpoint: cfg.GitCheckpoint,
	}
}

func (r Runtime) PhaseDriver(cfg DriverConfig) archaeophases.Driver {
	return archaeophases.Driver{
		Service: r.PhaseService(),
		Broker:  r.GuidanceBroker,
		Handoff: cfg.Handoff,
	}
}

func (r Runtime) EvaluateExecutionMutations(ctx context.Context, workflowID string, handoff *archaeodomain.ExecutionHandoff, plan *frameworkplan.LivingPlan, step *frameworkplan.PlanStep) (*archaeoexec.MutationEvaluation, error) {
	return r.ExecutionService().EvaluateMutations(ctx, workflowID, handoff, plan, step)
}

func (r Runtime) ActiveExploration(ctx context.Context, workspaceID string) (*archaeoarch.SessionView, error) {
	session, err := r.ArchaeologyService(ArchaeologyConfig{}).LoadActiveExplorationByWorkspace(ctx, workspaceID)
	if err != nil || session == nil {
		return nil, err
	}
	return r.ArchaeologyService(ArchaeologyConfig{}).LoadExplorationView(ctx, session.ID)
}

func (r Runtime) ExplorationView(ctx context.Context, explorationID string) (*archaeoarch.SessionView, error) {
	return r.ArchaeologyService(ArchaeologyConfig{}).LoadExplorationView(ctx, explorationID)
}

func (r Runtime) PendingLearning(ctx context.Context, workflowID string) ([]archaeolearning.Interaction, error) {
	return r.LearningService().Pending(ctx, workflowID)
}

func (r Runtime) ResolveLearning(ctx context.Context, input archaeolearning.ResolveInput) (*archaeolearning.Interaction, error) {
	return r.LearningService().Resolve(ctx, input)
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

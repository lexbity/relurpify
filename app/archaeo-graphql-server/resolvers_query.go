package archaeographqlserver

import "context"

type rootResolver struct {
	runtime Runtime
}

type queryResolver struct {
	runtime Runtime
}

type mutationResolver struct {
	runtime Runtime
}

type subscriptionResolver struct {
	runtime Runtime
}

func (r *rootResolver) Query() *queryResolver {
	return &queryResolver{runtime: r.runtime}
}

func (r *rootResolver) Mutation() *mutationResolver {
	return &mutationResolver{runtime: r.runtime}
}

func (r *rootResolver) Subscription() *subscriptionResolver {
	return &subscriptionResolver{runtime: r.runtime}
}

func (r *queryResolver) ActiveExploration(ctx context.Context, args struct{ WorkspaceID string }) (*Map, error) {
	value, err := r.runtime.ActiveExploration(ctx, args.WorkspaceID)
	if err != nil {
		return nil, err
	}
	return toMap(value)
}

func (r *queryResolver) ExplorationView(ctx context.Context, args struct{ ExplorationID string }) (*Map, error) {
	value, err := r.runtime.ExplorationView(ctx, args.ExplorationID)
	if err != nil {
		return nil, err
	}
	return toMap(value)
}

func (r *queryResolver) ExplorationByWorkflow(ctx context.Context, args struct{ WorkflowID string }) (*Map, error) {
	value, err := r.runtime.ExplorationByWorkflow(ctx, args.WorkflowID)
	if err != nil {
		return nil, err
	}
	return toMap(value)
}

func (r *queryResolver) WorkflowProjection(ctx context.Context, args struct{ WorkflowID string }) (*Map, error) {
	value, err := r.runtime.WorkflowProjection(ctx, args.WorkflowID)
	if err != nil {
		return nil, err
	}
	return toMap(value)
}

func (r *queryResolver) Timeline(ctx context.Context, args struct{ WorkflowID string }) (*Map, error) {
	value, err := r.runtime.TimelineProjection(ctx, args.WorkflowID)
	if err != nil {
		return nil, err
	}
	return toMap(value)
}

func (r *queryResolver) MutationHistory(ctx context.Context, args struct{ WorkflowID string }) (*Map, error) {
	value, err := r.runtime.MutationHistory(ctx, args.WorkflowID)
	if err != nil {
		return nil, err
	}
	return toMap(value)
}

func (r *queryResolver) RequestHistory(ctx context.Context, args struct{ WorkflowID string }) (*Map, error) {
	value, err := r.runtime.RequestHistory(ctx, args.WorkflowID)
	if err != nil {
		return nil, err
	}
	return toMap(value)
}

func (r *queryResolver) Provenance(ctx context.Context, args struct{ WorkflowID string }) (*Map, error) {
	value, err := r.runtime.Provenance(ctx, args.WorkflowID)
	if err != nil {
		return nil, err
	}
	return toMap(value)
}

func (r *queryResolver) Coherence(ctx context.Context, args struct{ WorkflowID string }) (*Map, error) {
	value, err := r.runtime.Coherence(ctx, args.WorkflowID)
	if err != nil {
		return nil, err
	}
	return toMap(value)
}

func (r *queryResolver) LearningQueue(ctx context.Context, args struct{ WorkflowID string }) ([]Map, error) {
	value, err := r.runtime.LearningQueue(ctx, args.WorkflowID)
	if err != nil {
		return nil, err
	}
	return toMaps(value)
}

func (r *queryResolver) Tensions(ctx context.Context, args struct{ WorkflowID string }) ([]Map, error) {
	value, err := r.runtime.Tensions(ctx, args.WorkflowID)
	if err != nil {
		return nil, err
	}
	return toMaps(value)
}

func (r *queryResolver) TensionSummary(ctx context.Context, args struct{ WorkflowID string }) (*Map, error) {
	value, err := r.runtime.TensionSummary(ctx, args.WorkflowID)
	if err != nil {
		return nil, err
	}
	return toMap(value)
}

func (r *queryResolver) ActivePlanVersion(ctx context.Context, args struct{ WorkflowID string }) (*Map, error) {
	value, err := r.runtime.ActivePlanVersion(ctx, args.WorkflowID)
	if err != nil {
		return nil, err
	}
	return toMap(value)
}

func (r *queryResolver) PlanLineage(ctx context.Context, args struct{ WorkflowID string }) (*Map, error) {
	value, err := r.runtime.PlanLineage(ctx, args.WorkflowID)
	if err != nil {
		return nil, err
	}
	return toMap(value)
}

func (r *queryResolver) ComparePlanVersions(ctx context.Context, args struct {
	WorkflowID string
	Left       int32
	Right      int32
}) (*Map, error) {
	value, err := r.runtime.ComparePlanVersions(ctx, args.WorkflowID, int(args.Left), int(args.Right))
	if err != nil {
		return nil, err
	}
	mapped := value
	return &mapped, nil
}

func (r *queryResolver) DeferredDrafts(ctx context.Context, args struct {
	WorkspaceID string
	Limit       *int32
}) (*Map, error) {
	value, err := r.runtime.DeferredDrafts(ctx, args.WorkspaceID, argLimit(args.Limit))
	if err != nil {
		return nil, err
	}
	return toMap(value)
}

func (r *queryResolver) CurrentConvergence(ctx context.Context, args struct{ WorkspaceID string }) (*Map, error) {
	value, err := r.runtime.CurrentConvergence(ctx, args.WorkspaceID)
	if err != nil {
		return nil, err
	}
	return toMap(value)
}

func (r *queryResolver) ConvergenceHistory(ctx context.Context, args struct {
	WorkspaceID string
	Limit       *int32
}) (*Map, error) {
	value, err := r.runtime.ConvergenceHistory(ctx, args.WorkspaceID, argLimit(args.Limit))
	if err != nil {
		return nil, err
	}
	return toMap(value)
}

func (r *queryResolver) DecisionTrail(ctx context.Context, args struct {
	WorkspaceID string
	Limit       *int32
}) (*Map, error) {
	value, err := r.runtime.DecisionTrail(ctx, args.WorkspaceID, argLimit(args.Limit))
	if err != nil {
		return nil, err
	}
	return toMap(value)
}

func (r *queryResolver) WorkspaceSummary(ctx context.Context, args struct{ WorkspaceID string }) (*Map, error) {
	value, err := r.runtime.WorkspaceSummary(ctx, args.WorkspaceID)
	if err != nil {
		return nil, err
	}
	return toMap(value)
}

func (r *queryResolver) PendingRequests(ctx context.Context, args struct{ WorkflowID string }) ([]Map, error) {
	value, err := r.runtime.PendingRequests(ctx, args.WorkflowID)
	if err != nil {
		return nil, err
	}
	return toMaps(value)
}

func (r *queryResolver) Request(ctx context.Context, args struct {
	WorkflowID string
	RequestID  string
}) (*Map, error) {
	value, err := r.runtime.Request(ctx, args.WorkflowID, args.RequestID)
	if err != nil {
		return nil, err
	}
	return toMap(value)
}

func argLimit(limit *int32) int {
	if limit == nil {
		return 0
	}
	if *limit <= 0 {
		return 0
	}
	return int(*limit)
}

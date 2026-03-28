package archaeographqlserver

import (
	"bytes"
	"context"
	"encoding/json"
	"time"
)

func (r *subscriptionResolver) WorkflowProjectionUpdated(ctx context.Context, args struct{ WorkflowID string }) <-chan *Map {
	return subscribePolling(ctx, r.runtime.pollInterval(), func(ctx context.Context) (*Map, error) {
		value, err := r.runtime.WorkflowProjection(ctx, args.WorkflowID)
		if err != nil {
			return nil, err
		}
		return toMap(value)
	})
}

func (r *subscriptionResolver) TimelineUpdated(ctx context.Context, args struct{ WorkflowID string }) <-chan *Map {
	return subscribePolling(ctx, r.runtime.pollInterval(), func(ctx context.Context) (*Map, error) {
		value, err := r.runtime.TimelineProjection(ctx, args.WorkflowID)
		if err != nil {
			return nil, err
		}
		return toMap(value)
	})
}

func (r *subscriptionResolver) RequestHistoryUpdated(ctx context.Context, args struct{ WorkflowID string }) <-chan *Map {
	return subscribePolling(ctx, r.runtime.pollInterval(), func(ctx context.Context) (*Map, error) {
		value, err := r.runtime.RequestHistory(ctx, args.WorkflowID)
		if err != nil {
			return nil, err
		}
		return toMap(value)
	})
}

func (r *subscriptionResolver) LearningQueueUpdated(ctx context.Context, args struct{ WorkflowID string }) <-chan []Map {
	return subscribePolling(ctx, r.runtime.pollInterval(), func(ctx context.Context) ([]Map, error) {
		value, err := r.runtime.LearningQueue(ctx, args.WorkflowID)
		if err != nil {
			return nil, err
		}
		return toMaps(value)
	})
}

func (r *subscriptionResolver) TensionsUpdated(ctx context.Context, args struct{ WorkflowID string }) <-chan []Map {
	return subscribePolling(ctx, r.runtime.pollInterval(), func(ctx context.Context) ([]Map, error) {
		value, err := r.runtime.Tensions(ctx, args.WorkflowID)
		if err != nil {
			return nil, err
		}
		return toMaps(value)
	})
}

func (r *subscriptionResolver) TensionSummaryUpdated(ctx context.Context, args struct{ WorkflowID string }) <-chan *Map {
	return subscribePolling(ctx, r.runtime.pollInterval(), func(ctx context.Context) (*Map, error) {
		value, err := r.runtime.TensionSummary(ctx, args.WorkflowID)
		if err != nil {
			return nil, err
		}
		return toMap(value)
	})
}

func (r *subscriptionResolver) ActivePlanVersionUpdated(ctx context.Context, args struct{ WorkflowID string }) <-chan *Map {
	return subscribePolling(ctx, r.runtime.pollInterval(), func(ctx context.Context) (*Map, error) {
		value, err := r.runtime.ActivePlanVersion(ctx, args.WorkflowID)
		if err != nil {
			return nil, err
		}
		return toMap(value)
	})
}

func (r *subscriptionResolver) PlanLineageUpdated(ctx context.Context, args struct{ WorkflowID string }) <-chan *Map {
	return subscribePolling(ctx, r.runtime.pollInterval(), func(ctx context.Context) (*Map, error) {
		value, err := r.runtime.PlanLineage(ctx, args.WorkflowID)
		if err != nil {
			return nil, err
		}
		return toMap(value)
	})
}

func (r *subscriptionResolver) ProvenanceUpdated(ctx context.Context, args struct{ WorkflowID string }) <-chan *Map {
	return subscribePolling(ctx, r.runtime.pollInterval(), func(ctx context.Context) (*Map, error) {
		value, err := r.runtime.Provenance(ctx, args.WorkflowID)
		if err != nil {
			return nil, err
		}
		return toMap(value)
	})
}

func (r *subscriptionResolver) CoherenceUpdated(ctx context.Context, args struct{ WorkflowID string }) <-chan *Map {
	return subscribePolling(ctx, r.runtime.pollInterval(), func(ctx context.Context) (*Map, error) {
		value, err := r.runtime.Coherence(ctx, args.WorkflowID)
		if err != nil {
			return nil, err
		}
		return toMap(value)
	})
}

func (r *subscriptionResolver) DeferredDraftsUpdated(ctx context.Context, args struct {
	WorkspaceID string
	Limit       *int32
}) <-chan *Map {
	return subscribePolling(ctx, r.runtime.pollInterval(), func(ctx context.Context) (*Map, error) {
		value, err := r.runtime.DeferredDrafts(ctx, args.WorkspaceID, argLimit(args.Limit))
		if err != nil {
			return nil, err
		}
		return toMap(value)
	})
}

func (r *subscriptionResolver) CurrentConvergenceUpdated(ctx context.Context, args struct{ WorkspaceID string }) <-chan *Map {
	return subscribePolling(ctx, r.runtime.pollInterval(), func(ctx context.Context) (*Map, error) {
		value, err := r.runtime.CurrentConvergence(ctx, args.WorkspaceID)
		if err != nil {
			return nil, err
		}
		return toMap(value)
	})
}

func (r *subscriptionResolver) ConvergenceHistoryUpdated(ctx context.Context, args struct {
	WorkspaceID string
	Limit       *int32
}) <-chan *Map {
	return subscribePolling(ctx, r.runtime.pollInterval(), func(ctx context.Context) (*Map, error) {
		value, err := r.runtime.ConvergenceHistory(ctx, args.WorkspaceID, argLimit(args.Limit))
		if err != nil {
			return nil, err
		}
		return toMap(value)
	})
}

func (r *subscriptionResolver) DecisionTrailUpdated(ctx context.Context, args struct {
	WorkspaceID string
	Limit       *int32
}) <-chan *Map {
	return subscribePolling(ctx, r.runtime.pollInterval(), func(ctx context.Context) (*Map, error) {
		value, err := r.runtime.DecisionTrail(ctx, args.WorkspaceID, argLimit(args.Limit))
		if err != nil {
			return nil, err
		}
		return toMap(value)
	})
}

func subscribePolling[T any](ctx context.Context, interval time.Duration, load func(context.Context) (T, error)) <-chan T {
	ch := make(chan T, 1)
	go func() {
		defer close(ch)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		var (
			last    []byte
			hasLast bool
		)
		for {
			value, err := load(ctx)
			if err == nil {
				if raw, marshalErr := json.Marshal(value); marshalErr == nil {
					if !hasLast || !bytes.Equal(last, raw) {
						select {
						case ch <- value:
							last = raw
							hasLast = true
						case <-ctx.Done():
							return
						}
					}
				}
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return ch
}

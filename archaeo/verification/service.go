package verification

import (
	"context"
	"time"

	archaeoevents "codeburg.org/lexbit/relurpify/archaeo/events"
	archaeotensions "codeburg.org/lexbit/relurpify/archaeo/tensions"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
	frameworkplan "codeburg.org/lexbit/relurpify/framework/plan"
)

type Service struct {
	Store    frameworkplan.PlanStore
	Workflow memory.WorkflowStateStore
	Verifier frameworkplan.ConvergenceVerifier
	Tensions archaeotensions.Service
	Now      func() time.Time
}

func (s Service) FinalizeConvergence(ctx context.Context, plan *frameworkplan.LivingPlan, result *core.Result) (*frameworkplan.ConvergenceFailure, error) {
	if plan == nil || s.Verifier == nil || plan.ConvergenceTarget == nil {
		return nil, nil
	}
	failure, err := s.Verifier.Verify(ctx, *plan.ConvergenceTarget)
	if err != nil {
		return nil, err
	}
	if failure == nil {
		now := s.now()
		plan.ConvergenceTarget.VerifiedAt = &now
		plan.UpdatedAt = now
		if s.Store != nil {
			if err := s.Store.SavePlan(ctx, plan); err != nil {
				return nil, err
			}
		}
		_ = archaeoevents.AppendWorkflowEvent(ctx, s.Workflow, plan.WorkflowID, archaeoevents.EventConvergenceVerified, "convergence verified", map[string]any{
			"plan_id":      plan.ID,
			"plan_version": plan.Version,
		}, now)
		return nil, nil
	}
	if result != nil {
		if result.Data == nil {
			result.Data = map[string]any{}
		}
		result.Data["convergence_failure"] = failure
		if len(failure.UnresolvedTensions) > 0 && s.Tensions.Store != nil {
			var unresolved []any
			for _, tensionID := range failure.UnresolvedTensions {
				record, err := s.Tensions.Load(ctx, plan.WorkflowID, tensionID)
				if err != nil {
					return nil, err
				}
				if record != nil {
					unresolved = append(unresolved, *record)
				}
			}
			if len(unresolved) > 0 {
				result.Data["unresolved_tension_records"] = unresolved
			}
		}
	}
	_ = archaeoevents.AppendWorkflowEvent(ctx, s.Workflow, plan.WorkflowID, archaeoevents.EventConvergenceFailed, failure.Description, map[string]any{
		"plan_id":                plan.ID,
		"plan_version":           plan.Version,
		"description":            failure.Description,
		"unresolved_tension_ids": failure.UnresolvedTensions,
	}, s.now())
	return failure, nil
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

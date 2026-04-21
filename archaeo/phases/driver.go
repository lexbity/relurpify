package phases

import (
	"context"

	archaeodomain "codeburg.org/lexbit/relurpify/archaeo/domain"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/guidance"
	frameworkplan "codeburg.org/lexbit/relurpify/framework/plan"
)

type Driver struct {
	Service Service
	Broker  *guidance.GuidanceBroker
	Handoff func(context.Context, *core.Task, *core.Context, *frameworkplan.PlanStep) error
}

func (d Driver) HandlePreparationOutcome(ctx context.Context, task *core.Task, state *core.Context, result *core.Result, err error, deferralPlan *guidance.DeferralPlan) (*core.Result, error, bool) {
	if err == nil && result == nil {
		return nil, nil, false
	}
	if result != nil && deferralPlan != nil && !deferralPlan.IsEmpty() && state != nil {
		state.Set("euclo.deferral_plan", deferralPlan)
	}
	blockedReason := errorString(err)
	if _, recordErr := d.Service.RecordState(ctx, task, state, d.Broker, archaeodomain.PhaseBlocked, blockedReason, nil); recordErr != nil && err == nil {
		err = recordErr
		if result == nil {
			result = &core.Result{Success: false, Error: err}
		} else {
			result.Success = false
			result.Error = err
		}
	}
	if result == nil && err != nil {
		result = &core.Result{Success: false, Error: err}
	}
	return result, err, true
}

func (d Driver) EnterExecution(ctx context.Context, task *core.Task, state *core.Context, step *frameworkplan.PlanStep) {
	if step == nil {
		return
	}
	if d.Handoff != nil {
		_ = d.Handoff(ctx, task, state, step)
	}
	_, _ = d.Service.RecordState(ctx, task, state, d.Broker, archaeodomain.PhaseExecution, "", step)
}

func (d Driver) EnterVerification(ctx context.Context, task *core.Task, state *core.Context, step *frameworkplan.PlanStep, err error) {
	_, _ = d.Service.RecordState(ctx, task, state, d.Broker, archaeodomain.PhaseVerification, errorString(err), step)
}

func (d Driver) EnterSurfacing(ctx context.Context, task *core.Task, state *core.Context, step *frameworkplan.PlanStep, err error) {
	_, _ = d.Service.RecordState(ctx, task, state, d.Broker, archaeodomain.PhaseSurfacing, errorString(err), step)
}

func (d Driver) Complete(ctx context.Context, task *core.Task, state *core.Context, step *frameworkplan.PlanStep, err error) {
	phase := archaeodomain.PhaseCompleted
	reason := ""
	if err != nil {
		phase = archaeodomain.PhaseBlocked
		reason = err.Error()
	}
	_, _ = d.Service.RecordState(ctx, task, state, d.Broker, phase, reason, step)
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

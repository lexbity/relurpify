package execution

import (
	"context"
	"log"

	archaeoplans "github.com/lexcodex/relurpify/archaeo/plans"
	archaeoverification "github.com/lexcodex/relurpify/archaeo/verification"
	"github.com/lexcodex/relurpify/framework/core"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
)

type Finalizer struct {
	Plans         archaeoplans.Service
	Verification  archaeoverification.Service
	GitCheckpoint func(context.Context, *core.Task) string
}

func (f Finalizer) FinalizeLivingPlan(ctx context.Context, task *core.Task, state *core.Context, plan *frameworkplan.LivingPlan, step *frameworkplan.PlanStep, result *core.Result, execErr error) {
	if plan == nil || step == nil {
		return
	}
	outcome := "completed"
	failureReason := ""
	if execErr != nil {
		outcome = "failed"
		failureReason = execErr.Error()
	} else if result != nil && result.Error != nil {
		outcome = "failed"
		failureReason = result.Error.Error()
	}
	checkpoint := ""
	if f.GitCheckpoint != nil {
		checkpoint = f.GitCheckpoint(ctx, task)
	}
	f.Plans.RecordStepOutcome(plan, step, outcome, failureReason, checkpoint)
	if execErr == nil {
		if changed := f.Plans.ApplyScopeInvalidations(plan, step); len(changed) > 0 {
			_ = f.Plans.PersistAllSteps(ctx, plan)
		}
	}
	if state != nil {
		state.Set("euclo.living_plan", plan)
	}
	_ = f.Plans.PersistStep(ctx, plan, step.ID)
	if execErr != nil || f.Verification.Verifier == nil || plan.ConvergenceTarget == nil {
		return
	}
	failure, err := f.Verification.FinalizeConvergence(ctx, plan, result)
	if err != nil {
		log.Printf("euclo: convergence verifier failed: %v", err)
		return
	}
	if state == nil {
		return
	}
	if failure == nil {
		state.Set("euclo.living_plan", plan)
		return
	}
	log.Printf("euclo: convergence target unmet: %s", failure.Description)
	state.Set("euclo.convergence_failure", *failure)
}

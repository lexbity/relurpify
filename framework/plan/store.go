package plan

import (
	"context"
	"time"
)

type PlanSummary struct {
	ID         string
	WorkflowID string
	Title      string
	StepCount  int
	UpdatedAt  time.Time
}

type PlanStore interface {
	SavePlan(ctx context.Context, plan *LivingPlan) error
	LoadPlan(ctx context.Context, planID string) (*LivingPlan, error)
	LoadPlanByWorkflow(ctx context.Context, workflowID string) (*LivingPlan, error)
	UpdateStep(ctx context.Context, planID, stepID string, step *PlanStep) error
	InvalidateStep(ctx context.Context, planID, stepID string, rule InvalidationRule) error
	DeletePlan(ctx context.Context, planID string) error
	ListPlans(ctx context.Context) ([]PlanSummary, error)
}

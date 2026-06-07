package plan

import (
	"context"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
)

type Result = execution.Result

type WorkflowExecutor interface {
	Execute(ctx context.Context, task *execution.Task, env *contextdata.Envelope) (*Result, error)
}

// Plan represents a collection of steps with dependencies.
type Plan struct {
	ID           string
	Goal         string
	Steps        []PlanStep
	Dependencies map[string][]string
	Files        []string
}

// PlanStep represents a single step in a plan.
type PlanStep struct {
	ID           string
	Description  string
	DependsOn    []string
	Expected     string
	Verification string
	Files        []string
	Tool         string
	Params       map[string]interface{}
}

// PlanExecutionOptions configures how plan steps are executed.
type PlanExecutionOptions struct {
	MaxRecoveryAttempts int
	BuildStepTask       func(parentTask *execution.Task, plan *Plan, step PlanStep, state *contextdata.Envelope) *execution.Task
	CompletedStepIDs    func(state *contextdata.Envelope) []string
	Diagnose            func(ctx context.Context, step PlanStep, err error) (string, error)
	Recover             func(ctx context.Context, step PlanStep, stepTask *execution.Task, state *contextdata.Envelope, err error) (*StepRecovery, error)
	BeforeStep          func(step PlanStep, stepTask *execution.Task, state *contextdata.Envelope)
	AfterStep           func(step PlanStep, state *contextdata.Envelope, result *Result)
	MergeBranches       func(parent *contextdata.Envelope, branches []BranchExecutionResult) error
}

// StepRecovery captures structured retry guidance after a failed step attempt.
type StepRecovery struct {
	Diagnosis string
	Notes     []string
	Context   map[string]any
}

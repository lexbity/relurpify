package archaeology

import (
	"context"

	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/execution"
	euclorelurpic "github.com/lexcodex/relurpify/named/euclo/relurpicabilities"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
)

// Invocable implementations for archaeology behaviors.

// exploreInvocable wraps exploreBehavior as an Invocable.
type exploreInvocable struct {
	behavior execution.Behavior
}

// NewExploreInvocable creates a new Invocable for the explore capability.
func NewExploreInvocable() execution.Invocable {
	return &exploreInvocable{behavior: NewExploreBehavior()}
}

func (e *exploreInvocable) ID() string { return e.behavior.ID() }

func (e *exploreInvocable) Invoke(ctx context.Context, in execution.InvokeInput) (*core.Result, error) {
	execInput := convertInvokeInputToExecuteInput(in)
	return e.behavior.Execute(ctx, execInput)
}

func (e *exploreInvocable) IsPrimary() bool { return true }

// compilePlanInvocable wraps compilePlanBehavior as an Invocable.
type compilePlanInvocable struct {
	behavior execution.Behavior
}

// NewCompilePlanInvocable creates a new Invocable for the compile-plan capability.
func NewCompilePlanInvocable() execution.Invocable {
	return &compilePlanInvocable{behavior: NewCompilePlanBehavior()}
}

func (c *compilePlanInvocable) ID() string { return c.behavior.ID() }

func (c *compilePlanInvocable) Invoke(ctx context.Context, in execution.InvokeInput) (*core.Result, error) {
	execInput := convertInvokeInputToExecuteInput(in)
	return c.behavior.Execute(ctx, execInput)
}

func (c *compilePlanInvocable) IsPrimary() bool { return true }

// implementPlanInvocable wraps implementPlanBehavior as an Invocable.
type implementPlanInvocable struct {
	behavior execution.Behavior
}

// NewImplementPlanInvocable creates a new Invocable for the implement-plan capability.
func NewImplementPlanInvocable() execution.Invocable {
	return &implementPlanInvocable{behavior: NewImplementPlanBehavior()}
}

func (i *implementPlanInvocable) ID() string { return i.behavior.ID() }

func (i *implementPlanInvocable) Invoke(ctx context.Context, in execution.InvokeInput) (*core.Result, error) {
	execInput := convertInvokeInputToExecuteInput(in)
	return i.behavior.Execute(ctx, execInput)
}

func (i *implementPlanInvocable) IsPrimary() bool { return true }

// NewSupportingInvocables returns all supporting invocables for the archaeology package.
func NewSupportingInvocables() []execution.Invocable {
	return []execution.Invocable{
		&patternSurfaceInvocable{},
		&prospectiveAssessInvocable{},
		&convergenceGuardInvocable{},
		&coherenceAssessInvocable{},
		&scopeExpandInvocable{},
	}
}

// patternSurfaceInvocable wraps patternSurfaceRoutine as an Invocable.
type patternSurfaceInvocable struct{}

func (p *patternSurfaceInvocable) ID() string { return PatternSurface }

func (p *patternSurfaceInvocable) Invoke(ctx context.Context, in execution.InvokeInput) (*core.Result, error) {
	routine := patternSurfaceRoutine{}
	artifacts, err := routine.Execute(ctx, convertInvokeInputToRoutineInput(in))
	if err != nil {
		return nil, err
	}
	return &core.Result{
		Success: true,
		Data:    map[string]any{"artifacts": artifacts},
	}, nil
}

func (p *patternSurfaceInvocable) IsPrimary() bool { return false }

// prospectiveAssessInvocable wraps prospectiveAssessRoutine as an Invocable.
type prospectiveAssessInvocable struct{}

func (p *prospectiveAssessInvocable) ID() string { return ProspectiveAssess }

func (p *prospectiveAssessInvocable) Invoke(ctx context.Context, in execution.InvokeInput) (*core.Result, error) {
	routine := prospectiveAssessRoutine{}
	artifacts, err := routine.Execute(ctx, convertInvokeInputToRoutineInput(in))
	if err != nil {
		return nil, err
	}
	return &core.Result{
		Success: true,
		Data:    map[string]any{"artifacts": artifacts},
	}, nil
}

func (p *prospectiveAssessInvocable) IsPrimary() bool { return false }

// convergenceGuardInvocable wraps convergenceGuardRoutine as an Invocable.
type convergenceGuardInvocable struct{}

func (c *convergenceGuardInvocable) ID() string { return ConvergenceGuard }

func (c *convergenceGuardInvocable) Invoke(ctx context.Context, in execution.InvokeInput) (*core.Result, error) {
	routine := convergenceGuardRoutine{}
	artifacts, err := routine.Execute(ctx, convertInvokeInputToRoutineInput(in))
	if err != nil {
		return nil, err
	}
	return &core.Result{
		Success: true,
		Data:    map[string]any{"artifacts": artifacts},
	}, nil
}

func (c *convergenceGuardInvocable) IsPrimary() bool { return false }

// coherenceAssessInvocable wraps coherenceAssessRoutine as an Invocable.
type coherenceAssessInvocable struct{}

func (c *coherenceAssessInvocable) ID() string { return CoherenceAssess }

func (c *coherenceAssessInvocable) Invoke(ctx context.Context, in execution.InvokeInput) (*core.Result, error) {
	routine := coherenceAssessRoutine{}
	artifacts, err := routine.Execute(ctx, convertInvokeInputToRoutineInput(in))
	if err != nil {
		return nil, err
	}
	return &core.Result{
		Success: true,
		Data:    map[string]any{"artifacts": artifacts},
	}, nil
}

func (c *coherenceAssessInvocable) IsPrimary() bool { return false }

// scopeExpandInvocable wraps scopeExpandRoutine as an Invocable.
type scopeExpandInvocable struct{}

func (s *scopeExpandInvocable) ID() string { return ScopeExpansionAssess }

func (s *scopeExpandInvocable) Invoke(ctx context.Context, in execution.InvokeInput) (*core.Result, error) {
	routine := scopeExpandRoutine{}
	artifacts, err := routine.Execute(ctx, convertInvokeInputToRoutineInput(in))
	if err != nil {
		return nil, err
	}
	return &core.Result{
		Success: true,
		Data:    map[string]any{"artifacts": artifacts},
	}, nil
}

func (s *scopeExpandInvocable) IsPrimary() bool { return false }

// convertInvokeInputToExecuteInput converts the new InvokeInput to the legacy ExecuteInput.
func convertInvokeInputToExecuteInput(in execution.InvokeInput) execution.ExecuteInput {
	return execution.ExecuteInput{
		Task:                 in.Task,
		ExecutionTask:        in.ExecutionTask,
		State:                in.State,
		Mode:                 in.Mode,
		Profile:              in.Profile,
		Work:                 in.Work,
		Environment:          in.Environment,
		ServiceBundle:        in.ServiceBundle,
		WorkflowExecutor:     in.WorkflowExecutor,
		Telemetry:            in.Telemetry,
		RunSupportingRoutine: convertInvokeSupportingToRunSupporting(in.InvokeSupporting),
	}
}

// convertInvokeSupportingToRunSupporting adapts the new InvokeSupporting signature
// to the legacy RunSupportingRoutine signature.
func convertInvokeSupportingToRunSupporting(
	invokeSupporting func(context.Context, string, execution.InvokeInput) ([]euclotypes.Artifact, error),
) func(context.Context, string, *core.Task, *core.Context, eucloruntime.UnitOfWork, agentenv.AgentEnvironment, execution.ServiceBundle) ([]euclotypes.Artifact, error) {
	if invokeSupporting == nil {
		return nil
	}
	return func(ctx context.Context, routineID string, task *core.Task, state *core.Context, work eucloruntime.UnitOfWork, env agentenv.AgentEnvironment, bundle execution.ServiceBundle) ([]euclotypes.Artifact, error) {
		in := execution.InvokeInput{
			Task:          task,
			State:         state,
			Work:          work,
			Environment:   env,
			ServiceBundle: bundle,
		}
		return invokeSupporting(ctx, routineID, in)
	}
}

// convertInvokeInputToRoutineInput converts InvokeInput to RoutineInput for supporting routine compatibility.
func convertInvokeInputToRoutineInput(in execution.InvokeInput) euclorelurpic.RoutineInput {
	return euclorelurpic.RoutineInput{
		Task:  in.Task,
		State: in.State,
		Work: euclorelurpic.WorkContext{
			PrimaryCapabilityID:             in.Work.PrimaryRelurpicCapabilityID,
			SupportingRelurpicCapabilityIDs: append([]string(nil), in.Work.SupportingRelurpicCapabilityIDs...),
			PatternRefs:                     append([]string(nil), in.Work.SemanticInputs.PatternRefs...),
			TensionRefs:                     append([]string(nil), in.Work.SemanticInputs.TensionRefs...),
			ProspectiveRefs:                 append([]string(nil), in.Work.SemanticInputs.ProspectiveRefs...),
			ConvergenceRefs:                 append([]string(nil), in.Work.SemanticInputs.ConvergenceRefs...),
			RequestProvenanceRefs:           append([]string(nil), in.Work.SemanticInputs.RequestProvenanceRefs...),
		},
		Environment:   in.Environment,
		ServiceBundle: in.ServiceBundle,
	}
}

package archaeology

import (
	"context"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/euclo/execution"
	euclorelurpic "codeburg.org/lexbit/relurpify/named/euclo/relurpicabilities"
)

// Invocable implementations for archaeology behaviors.

// ExploreInvocable implements the explore capability.
type ExploreInvocable struct{}

// NewExploreInvocable creates a new Invocable for the explore capability.
func NewExploreInvocable() execution.Invocable {
	return &ExploreInvocable{}
}

func (e *ExploreInvocable) ID() string { return Explore }

func (e *ExploreInvocable) Invoke(ctx context.Context, in execution.InvokeInput) (*core.Result, error) {
	execInput := execution.ExecuteInput{
		Task:             in.Task,
		ExecutionTask:    in.ExecutionTask,
		State:            in.State,
		Mode:             in.Mode,
		Profile:          in.Profile,
		Work:             in.Work,
		Environment:      in.Environment,
		ServiceBundle:    in.ServiceBundle,
		WorkflowExecutor: in.WorkflowExecutor,
		Telemetry:        in.Telemetry,
		InvokeSupporting: in.InvokeSupporting,
	}
	// Call the underlying behavior's Execute method
	exploreBehavior := exploreBehavior{}
	return exploreBehavior.Execute(ctx, execInput)
}

func (e *ExploreInvocable) IsPrimary() bool { return true }

// CompilePlanInvocable implements the compile-plan capability.
type CompilePlanInvocable struct{}

// NewCompilePlanInvocable creates a new Invocable for the compile-plan capability.
func NewCompilePlanInvocable() execution.Invocable {
	return &CompilePlanInvocable{}
}

func (c *CompilePlanInvocable) ID() string { return CompilePlan }

func (c *CompilePlanInvocable) Invoke(ctx context.Context, in execution.InvokeInput) (*core.Result, error) {
	execInput := execution.ExecuteInput{
		Task:             in.Task,
		ExecutionTask:    in.ExecutionTask,
		State:            in.State,
		Mode:             in.Mode,
		Profile:          in.Profile,
		Work:             in.Work,
		Environment:      in.Environment,
		ServiceBundle:    in.ServiceBundle,
		WorkflowExecutor: in.WorkflowExecutor,
		Telemetry:        in.Telemetry,
		InvokeSupporting: in.InvokeSupporting,
	}
	// Call the underlying behavior's Execute method
	compilePlanBehavior := compilePlanBehavior{}
	return compilePlanBehavior.Execute(ctx, execInput)
}

func (c *CompilePlanInvocable) IsPrimary() bool { return true }

// ImplementPlanInvocable implements the implement-plan capability.
type ImplementPlanInvocable struct{}

// NewImplementPlanInvocable creates a new Invocable for the implement-plan capability.
func NewImplementPlanInvocable() execution.Invocable {
	return &ImplementPlanInvocable{}
}

func (i *ImplementPlanInvocable) ID() string { return ImplementPlan }

func (i *ImplementPlanInvocable) Invoke(ctx context.Context, in execution.InvokeInput) (*core.Result, error) {
	execInput := execution.ExecuteInput{
		Task:             in.Task,
		ExecutionTask:    in.ExecutionTask,
		State:            in.State,
		Mode:             in.Mode,
		Profile:          in.Profile,
		Work:             in.Work,
		Environment:      in.Environment,
		ServiceBundle:    in.ServiceBundle,
		WorkflowExecutor: in.WorkflowExecutor,
		Telemetry:        in.Telemetry,
		InvokeSupporting: in.InvokeSupporting,
	}
	// Call the underlying behavior's Execute method
	implementPlanBehavior := implementPlanBehavior{}
	return implementPlanBehavior.Execute(ctx, execInput)
}

func (i *ImplementPlanInvocable) IsPrimary() bool { return true }

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

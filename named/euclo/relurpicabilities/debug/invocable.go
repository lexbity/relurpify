package debug

import (
	"context"

	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/execution"
	euclorelurpic "github.com/lexcodex/relurpify/named/euclo/relurpicabilities"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
)

// Invocable implementations for debug behaviors.

// investigateRepairInvocable wraps investigateRepairBehavior as an Invocable.
type investigateRepairInvocable struct {
	behavior execution.Behavior
}

// NewInvestigateRepairInvocable creates a new Invocable for the investigate-repair capability.
func NewInvestigateRepairInvocable() execution.Invocable {
	return &investigateRepairInvocable{behavior: NewInvestigateRepairBehavior()}
}

func (i *investigateRepairInvocable) ID() string { return i.behavior.ID() }

func (i *investigateRepairInvocable) Invoke(ctx context.Context, in execution.InvokeInput) (*core.Result, error) {
	execInput := convertInvokeInputToExecuteInput(in)
	return i.behavior.Execute(ctx, execInput)
}

func (i *investigateRepairInvocable) IsPrimary() bool { return true }

// simpleRepairInvocable wraps simpleRepairBehavior as an Invocable.
type simpleRepairInvocable struct {
	behavior execution.Behavior
}

// NewSimpleRepairInvocable creates a new Invocable for the simple-repair capability.
func NewSimpleRepairInvocable() execution.Invocable {
	return &simpleRepairInvocable{behavior: NewSimpleRepairBehavior()}
}

func (s *simpleRepairInvocable) ID() string { return s.behavior.ID() }

func (s *simpleRepairInvocable) Invoke(ctx context.Context, in execution.InvokeInput) (*core.Result, error) {
	execInput := convertInvokeInputToExecuteInput(in)
	return s.behavior.Execute(ctx, execInput)
}

func (s *simpleRepairInvocable) IsPrimary() bool { return true }

// NewSupportingInvocables returns all supporting invocables for the debug package.
func NewSupportingInvocables() []execution.Invocable {
	return []execution.Invocable{
		&rootCauseInvocable{},
		&hypothesisRefineInvocable{},
		&localizationInvocable{},
		&flawSurfaceInvocable{},
		&verificationRepairInvocable{},
	}
}

// rootCauseInvocable wraps rootCauseRoutine as an Invocable.
type rootCauseInvocable struct{}

func (r *rootCauseInvocable) ID() string { return RootCause }

func (r *rootCauseInvocable) Invoke(ctx context.Context, in execution.InvokeInput) (*core.Result, error) {
	routine := rootCauseRoutine{}
	artifacts, err := routine.Execute(ctx, convertInvokeInputToRoutineInput(in))
	if err != nil {
		return nil, err
	}
	return &core.Result{
		Success: true,
		Data:    map[string]any{"artifacts": artifacts},
	}, nil
}

func (r *rootCauseInvocable) IsPrimary() bool { return false }

// hypothesisRefineInvocable wraps hypothesisRefineRoutine as an Invocable.
type hypothesisRefineInvocable struct{}

func (h *hypothesisRefineInvocable) ID() string { return HypothesisRefine }

func (h *hypothesisRefineInvocable) Invoke(ctx context.Context, in execution.InvokeInput) (*core.Result, error) {
	routine := hypothesisRefineRoutine{}
	artifacts, err := routine.Execute(ctx, convertInvokeInputToRoutineInput(in))
	if err != nil {
		return nil, err
	}
	return &core.Result{
		Success: true,
		Data:    map[string]any{"artifacts": artifacts},
	}, nil
}

func (h *hypothesisRefineInvocable) IsPrimary() bool { return false }

// localizationInvocable wraps localizationRoutine as an Invocable.
type localizationInvocable struct{}

func (l *localizationInvocable) ID() string { return Localization }

func (l *localizationInvocable) Invoke(ctx context.Context, in execution.InvokeInput) (*core.Result, error) {
	routine := localizationRoutine{}
	artifacts, err := routine.Execute(ctx, convertInvokeInputToRoutineInput(in))
	if err != nil {
		return nil, err
	}
	return &core.Result{
		Success: true,
		Data:    map[string]any{"artifacts": artifacts},
	}, nil
}

func (l *localizationInvocable) IsPrimary() bool { return false }

// flawSurfaceInvocable wraps flawSurfaceRoutine as an Invocable.
type flawSurfaceInvocable struct{}

func (f *flawSurfaceInvocable) ID() string { return FlawSurface }

func (f *flawSurfaceInvocable) Invoke(ctx context.Context, in execution.InvokeInput) (*core.Result, error) {
	routine := flawSurfaceRoutine{}
	artifacts, err := routine.Execute(ctx, convertInvokeInputToRoutineInput(in))
	if err != nil {
		return nil, err
	}
	return &core.Result{
		Success: true,
		Data:    map[string]any{"artifacts": artifacts},
	}, nil
}

func (f *flawSurfaceInvocable) IsPrimary() bool { return false }

// verificationRepairInvocable wraps verificationRepairRoutine as an Invocable.
type verificationRepairInvocable struct{}

func (v *verificationRepairInvocable) ID() string { return VerificationRepair }

func (v *verificationRepairInvocable) Invoke(ctx context.Context, in execution.InvokeInput) (*core.Result, error) {
	routine := verificationRepairRoutine{}
	artifacts, err := routine.Execute(ctx, convertInvokeInputToRoutineInput(in))
	if err != nil {
		return nil, err
	}
	return &core.Result{
		Success: true,
		Data:    map[string]any{"artifacts": artifacts},
	}, nil
}

func (v *verificationRepairInvocable) IsPrimary() bool { return false }

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

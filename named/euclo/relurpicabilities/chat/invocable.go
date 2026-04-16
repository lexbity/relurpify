package chat

import (
	"context"

	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/execution"
	euclorelurpic "github.com/lexcodex/relurpify/named/euclo/relurpicabilities"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
)

// Invocable implementations for chat behaviors.
// These wrap the existing Behavior implementations to satisfy the Invocable interface.

// askInvocable wraps askBehavior as an Invocable.
type askInvocable struct {
	behavior execution.Behavior
}

// NewAskInvocable creates a new Invocable for the ask capability.
func NewAskInvocable() execution.Invocable {
	return &askInvocable{behavior: NewAskBehavior()}
}

func (a *askInvocable) ID() string { return a.behavior.ID() }

func (a *askInvocable) Invoke(ctx context.Context, in execution.InvokeInput) (*core.Result, error) {
	execInput := convertInvokeInputToExecuteInput(in)
	return a.behavior.Execute(ctx, execInput)
}

func (a *askInvocable) IsPrimary() bool { return true }

// inspectInvocable wraps inspectBehavior as an Invocable.
type inspectInvocable struct {
	behavior execution.Behavior
}

// NewInspectInvocable creates a new Invocable for the inspect capability.
func NewInspectInvocable() execution.Invocable {
	return &inspectInvocable{behavior: NewInspectBehavior()}
}

func (i *inspectInvocable) ID() string { return i.behavior.ID() }

func (i *inspectInvocable) Invoke(ctx context.Context, in execution.InvokeInput) (*core.Result, error) {
	execInput := convertInvokeInputToExecuteInput(in)
	return i.behavior.Execute(ctx, execInput)
}

func (i *inspectInvocable) IsPrimary() bool { return true }

// implementInvocable wraps implementBehavior as an Invocable.
type implementInvocable struct {
	behavior execution.Behavior
}

// NewImplementInvocable creates a new Invocable for the implement capability.
func NewImplementInvocable() execution.Invocable {
	return &implementInvocable{behavior: NewImplementBehavior()}
}

func (i *implementInvocable) ID() string { return i.behavior.ID() }

func (i *implementInvocable) Invoke(ctx context.Context, in execution.InvokeInput) (*core.Result, error) {
	execInput := convertInvokeInputToExecuteInput(in)
	return i.behavior.Execute(ctx, execInput)
}

func (i *implementInvocable) IsPrimary() bool { return true }

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

// NewSupportingInvocables returns all supporting invocables for the chat package.
func NewSupportingInvocables() []execution.Invocable {
	return []execution.Invocable{
		&directEditExecutionInvocable{},
		&localReviewInvocable{},
		&targetedVerificationInvocable{},
	}
}

// directEditExecutionInvocable wraps directEditExecutionRoutine as an Invocable.
type directEditExecutionInvocable struct{}

func (r *directEditExecutionInvocable) ID() string { return DirectEditExecution }

func (r *directEditExecutionInvocable) Invoke(ctx context.Context, in execution.InvokeInput) (*core.Result, error) {
	routine := directEditExecutionRoutine{}
	artifacts, err := routine.Execute(ctx, convertInvokeInputToRoutineInput(in))
	if err != nil {
		return nil, err
	}
	return &core.Result{
		Success: true,
		Data:    map[string]any{"artifacts": artifacts},
	}, nil
}

func (r *directEditExecutionInvocable) IsPrimary() bool { return false }

// localReviewInvocable wraps localReviewRoutine as an Invocable.
type localReviewInvocable struct{}

func (r *localReviewInvocable) ID() string { return LocalReview }

func (r *localReviewInvocable) Invoke(ctx context.Context, in execution.InvokeInput) (*core.Result, error) {
	routine := localReviewRoutine{}
	artifacts, err := routine.Execute(ctx, convertInvokeInputToRoutineInput(in))
	if err != nil {
		return nil, err
	}
	return &core.Result{
		Success: true,
		Data:    map[string]any{"artifacts": artifacts},
	}, nil
}

func (r *localReviewInvocable) IsPrimary() bool { return false }

// targetedVerificationInvocable wraps targetedVerificationRoutine as an Invocable.
type targetedVerificationInvocable struct{}

func (r *targetedVerificationInvocable) ID() string { return TargetedVerification }

func (r *targetedVerificationInvocable) Invoke(ctx context.Context, in execution.InvokeInput) (*core.Result, error) {
	routine := targetedVerificationRoutine{}
	artifacts, err := routine.Execute(ctx, convertInvokeInputToRoutineInput(in))
	if err != nil {
		return nil, err
	}
	return &core.Result{
		Success: true,
		Data:    map[string]any{"artifacts": artifacts},
	}, nil
}

func (r *targetedVerificationInvocable) IsPrimary() bool { return false }

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

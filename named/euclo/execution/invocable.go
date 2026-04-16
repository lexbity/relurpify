package execution

import (
	"context"

	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
)

// Invocable is the unified interface for all euclo capabilities.
// It replaces the separate Behavior and SupportingRoutine interfaces.
// Primary behaviors return a *core.Result with full execution context.
// Supporting invocables return only artifacts.
type Invocable interface {
	ID() string
	// Invoke executes the capability. Primary behaviors return a full result;
	// supporting invocables embed artifacts in result.Data["artifacts"].
	Invoke(context.Context, InvokeInput) (*core.Result, error)
	// IsPrimary returns true if this invocable can be a primary dispatch target.
	IsPrimary() bool
}

// InvokeInput is the unified input type for all invocables.
type InvokeInput struct {
	Task             *core.Task
	ExecutionTask    *core.Task
	State            *core.Context
	Mode             euclotypes.ModeResolution
	Profile          euclotypes.ExecutionProfileSelection
	Work             eucloruntime.UnitOfWork
	Environment      agentenv.AgentEnvironment
	ServiceBundle    ServiceBundle
	WorkflowExecutor graph.WorkflowExecutor
	Telemetry        core.Telemetry
	InvokeSupporting func(context.Context, string, InvokeInput) ([]euclotypes.Artifact, error)
}

// BehaviorAsInvocable wraps an execution.Behavior as an Invocable.
// This adapter is used during migration and will be removed once all
// behaviors implement Invocable directly.
type BehaviorAsInvocable struct {
	behavior Behavior
	primary  bool
}

// NewBehaviorAsInvocable creates a new Invocable wrapper for a Behavior.
func NewBehaviorAsInvocable(b Behavior, primary bool) *BehaviorAsInvocable {
	return &BehaviorAsInvocable{behavior: b, primary: primary}
}

func (a *BehaviorAsInvocable) ID() string { return a.behavior.ID() }

func (a *BehaviorAsInvocable) Invoke(ctx context.Context, in InvokeInput) (*core.Result, error) {
	execInput := ExecuteInput{
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
	return a.behavior.Execute(ctx, execInput)
}

func (a *BehaviorAsInvocable) IsPrimary() bool { return a.primary }

// convertInvokeSupportingToRunSupporting adapts the new InvokeSupporting signature
// to the legacy RunSupportingRoutine signature.
func convertInvokeSupportingToRunSupporting(
	invokeSupporting func(context.Context, string, InvokeInput) ([]euclotypes.Artifact, error),
) func(context.Context, string, *core.Task, *core.Context, eucloruntime.UnitOfWork, agentenv.AgentEnvironment, ServiceBundle) ([]euclotypes.Artifact, error) {
	if invokeSupporting == nil {
		return nil
	}
	return func(ctx context.Context, routineID string, task *core.Task, state *core.Context, work eucloruntime.UnitOfWork, env agentenv.AgentEnvironment, bundle ServiceBundle) ([]euclotypes.Artifact, error) {
		// Create a minimal InvokeInput with the available fields
		in := InvokeInput{
			Task:          task,
			State:         state,
			Work:          work,
			Environment:   env,
			ServiceBundle: bundle,
		}
		return invokeSupporting(ctx, routineID, in)
	}
}

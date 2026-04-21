package execution

import (
	"context"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/graph"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	eucloruntime "codeburg.org/lexbit/relurpify/named/euclo/runtime"
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

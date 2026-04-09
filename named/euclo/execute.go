package euclo

import (
	"context"
	"fmt"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
)

func (a *Agent) BuildGraph(task *core.Task) (*graph.Graph, error) {
	_, _, _, _, work := a.runtimeState(task, nil)
	selection, err := a.selectExecutor(work)
	if err != nil {
		return nil, err
	}
	return selection.Workflow.BuildGraph(task)
}

func (a *Agent) Execute(ctx context.Context, task *core.Task, state *core.Context) (*core.Result, error) {
	if state == nil {
		state = core.NewContext()
	}
	envelope, classification, mode, profile, work := a.runtimeState(task, state)
	a.seedRuntimeState(state, envelope, classification, mode, profile, work)
	selection, err := a.selectExecutor(work)
	if err != nil {
		return &core.Result{Success: false, Error: err}, err
	}
	if selection.Workflow == nil {
		err := fmt.Errorf("workflow executor unavailable")
		return &core.Result{Success: false, Error: err}, err
	}
	state.Set("euclo.executor_runtime", selection.Runtime)
	return a.executeManagedFlow(ctx, task, state, selection.Workflow)
}

func (a *Agent) executeManagedFlow(ctx context.Context, task *core.Task, state *core.Context, workflowExecutor graph.WorkflowExecutor) (*core.Result, error) {
	flow, result, err := a.initializeManagedExecution(ctx, task, state, workflowExecutor)
	if err != nil {
		return result, err
	}
	return a.executeManagedExecution(ctx, flow)
}

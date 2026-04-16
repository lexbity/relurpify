package euclo

import (
	"context"
	"fmt"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
	euclointake "github.com/lexcodex/relurpify/named/euclo/runtime/intake"
	euclostate "github.com/lexcodex/relurpify/named/euclo/runtime/state"
)

func (a *Agent) BuildGraph(task *core.Task) (*graph.Graph, error) {
	// Use single-pass enrichment for graph building
	semanticInputs := a.semanticInputBundle(task, nil, euclotypes.ModeResolution{})
	skillPolicy := eucloruntime.BuildResolvedExecutionPolicy(task, a.Config, a.CapabilityRegistry(), euclotypes.ModeResolution{}, euclotypes.ExecutionProfileSelection{})
	executorDescriptor := eucloruntime.WorkUnitExecutorDescriptor{}

	classifier := a.newCapabilityClassifier()

	classified, err := euclointake.RunEnrichment(context.Background(), task, nil, a.Environment, a.ModeRegistry, a.ProfileRegistry, classifier, semanticInputs, skillPolicy, executorDescriptor)
	if err != nil {
		return nil, err
	}

	selection, err := a.selectExecutor(classified.Work)
	if err != nil {
		return nil, err
	}
	return selection.Workflow.BuildGraph(task)
}

func (a *Agent) Execute(ctx context.Context, task *core.Task, state *core.Context) (*core.Result, error) {
	if state == nil {
		state = core.NewContext()
	}

	// Single-pass enrichment: replaces the double runtimeState() call pattern
	semanticInputs := a.semanticInputBundle(task, state, euclotypes.ModeResolution{})
	skillPolicy := eucloruntime.BuildResolvedExecutionPolicy(task, a.Config, a.CapabilityRegistry(), euclotypes.ModeResolution{}, euclotypes.ExecutionProfileSelection{})
	executorDescriptor := eucloruntime.WorkUnitExecutorDescriptor{}

	classifier := a.newCapabilityClassifier()

	classified, err := euclointake.RunEnrichment(ctx, task, state, a.Environment, a.ModeRegistry, a.ProfileRegistry, classifier, semanticInputs, skillPolicy, executorDescriptor)
	if err != nil {
		return &core.Result{Success: false, Error: err}, err
	}

	// Persist classified envelope to state
	euclointake.SeedClassifiedEnvelope(state, classified)

	selection, err := a.selectExecutor(classified.Work)
	if err != nil {
		return &core.Result{Success: false, Error: err}, err
	}
	if selection.Workflow == nil {
		err := fmt.Errorf("workflow executor unavailable")
		return &core.Result{Success: false, Error: err}, err
	}
	euclostate.SetExecutorRuntime(state, selection.Runtime)
	return a.executeManagedFlow(ctx, task, state, selection.Workflow)
}

func (a *Agent) executeManagedFlow(ctx context.Context, task *core.Task, state *core.Context, workflowExecutor graph.WorkflowExecutor) (*core.Result, error) {
	flow, result, err := a.initializeManagedExecution(ctx, task, state, workflowExecutor)
	if err != nil {
		return result, err
	}
	return a.executeManagedExecution(ctx, flow)
}

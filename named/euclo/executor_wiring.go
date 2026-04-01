package euclo

import (
	eucloexec "github.com/lexcodex/relurpify/named/euclo/execution"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
)

func (a *Agent) selectExecutor(work eucloruntime.UnitOfWork) eucloexec.WorkUnitExecutor {
	// Ensure the react delegate is initialised before capturing a.Delegate in the
	// factory value. SelectExecutor's default case captures React at factory-construction
	// time; if Delegate is nil here the executeF closure passes a typed-nil interface
	// to RunWithWorkflow, which bypasses the executor==nil guard and panics.
	_ = a.ensureReactDelegate()
	return eucloexec.SelectExecutor(eucloexec.ExecutorFactory{
		Model:          a.Environment.Model,
		Registry:       a.CapabilityRegistry(),
		Memory:         a.Memory,
		Config:         a.Config,
		CheckpointPath: a.CheckpointPath,
		IndexManager:   a.Environment.IndexManager,
		SearchEngine:   a.Environment.SearchEngine,
		Telemetry:      a.ConfigTelemetry(),
		React:          a.Delegate,
		EnsureReact:    a.ensureReactDelegate,
		RunWithWorkflow: a.executeWithWorkflowExecutor,
	}, work)
}

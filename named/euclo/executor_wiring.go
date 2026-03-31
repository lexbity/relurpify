package euclo

import (
	eucloexec "github.com/lexcodex/relurpify/named/euclo/execution"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
)

func (a *Agent) selectExecutor(work eucloruntime.UnitOfWork) eucloexec.WorkUnitExecutor {
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

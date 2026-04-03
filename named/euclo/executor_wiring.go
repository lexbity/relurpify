package euclo

import (
	eucloexec "github.com/lexcodex/relurpify/named/euclo/execution"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
)

func (a *Agent) selectExecutor(work eucloruntime.UnitOfWork) (eucloexec.Selection, error) {
	// Ensure the react delegate is initialised before capturing a.Delegate in the
	// factory value. The default case returns React directly as a WorkflowExecutor,
	// so selecting before Delegate is ready would hand back a typed-nil executor.
	if err := a.ensureReactDelegate(); err != nil {
		return eucloexec.Selection{}, err
	}
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
	}, work)
}

package agentgraph

import (
	"context"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
)

// WorkflowExecutor is the runtime execution contract consumed by graph-level
// orchestration helpers. Concrete agents may implement this interface, but the
// contract itself is framework-owned and runtime-oriented rather than specific
// to any single agent paradigm.
type WorkflowExecutor interface {
	Initialize(config *execution.Config) error
	Execute(ctx context.Context, task *execution.Task, env *contextdata.Envelope) (*execution.Result, error)
	Capabilities() []string
	BuildGraph(task *execution.Task) (*Graph, error)
}

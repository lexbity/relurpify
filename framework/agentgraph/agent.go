package agentgraph

import (
	"context"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
)

// WorkflowExecutor is the runtime execution contract consumed by graph-level
// orchestration helpers such as PlanExecutor. Concrete agents may implement
// this interface, but the contract itself is framework-owned and runtime-
// oriented rather than specific to any single agent paradigm.
type WorkflowExecutor interface {
	Initialize(config *core.Config) error
	Execute(ctx context.Context, task *core.Task, env *contextdata.Envelope) (*Result, error)
	Capabilities() []string
	BuildGraph(task *core.Task) (*Graph, error)
}

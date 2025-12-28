package graph

import (
	"context"

	"github.com/lexcodex/relurpify/framework/core"
)

// Agent defines the contract for all specialized agents. BuildGraph is exposed
// so orchestrators can inspect the workflow ahead of time (for visualization or
// validation) before calling Execute.
type Agent interface {
	Initialize(config *core.Config) error
	Execute(ctx context.Context, task *core.Task, state *core.Context) (*core.Result, error)
	Capabilities() []core.Capability
	BuildGraph(task *core.Task) (*Graph, error)
}

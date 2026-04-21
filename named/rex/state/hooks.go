package state

import (
	"context"

	"codeburg.org/lexbit/relurpify/framework/core"
)

// ExecutionObserver tracks rex workflow lifecycle transitions that need to
// project into external control-plane state such as FMP lineage records.
type ExecutionObserver interface {
	BeforeExecute(ctx context.Context, workflowID, runID string, task *core.Task, state *core.Context) error
	AfterExecute(ctx context.Context, workflowID, runID string, task *core.Task, state *core.Context, result *core.Result, execErr error) error
}

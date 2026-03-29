package restore

import (
	"context"

	frameworkcore "github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	runtimepkg "github.com/lexcodex/relurpify/named/euclo/runtime"
)

type ProviderSnapshot = frameworkcore.ProviderSnapshot
type ProviderSessionSnapshot = frameworkcore.ProviderSessionSnapshot

var CompiledExecutionFromState = runtimepkg.CompiledExecutionFromState
var RestoreRequested = runtimepkg.RestoreRequested

func Persist(ctx context.Context, store memory.WorkflowStateStore, workflowID, runID string, state *frameworkcore.Context, taskID string) (ProviderRestoreState, error) {
	return PersistProviderSnapshotState(ctx, store, workflowID, runID, state, taskID)
}

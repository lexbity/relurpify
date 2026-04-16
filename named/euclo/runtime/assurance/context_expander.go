package assurance

import (
	"context"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
	euclopolicy "github.com/lexcodex/relurpify/named/euclo/runtime/policy"
	euclorestore "github.com/lexcodex/relurpify/named/euclo/runtime/restore"
	euclostate "github.com/lexcodex/relurpify/named/euclo/runtime/state"
)

// ContextExpander handles context expansion for execution tasks.
// It loads workflow artifacts and applies retrieval policy to produce
// the final execution task.
type ContextExpander struct {
	Memory memory.MemoryStore
}

// ContextExpansionResult is the result of context expansion.
type ContextExpansionResult struct {
	ExecutionTask *core.Task
	Err           error
}

// Expand loads workflow artifacts, applies retrieval policy, and produces
// the final execution task. Currently this is a thin wrapper over the
// legacy expandContext logic, but extracted into a testable service.
func (e ContextExpander) Expand(ctx context.Context, in Input) ContextExpansionResult {
	executionTask := in.ExecutionTask
	if executionTask == nil {
		executionTask = in.Task
	}

	surfaces := euclorestore.ResolveRuntimeSurfaces(e.Memory)
	if surfaces.Workflow == nil {
		return ContextExpansionResult{ExecutionTask: executionTask}
	}

	workflowID, _ := euclostate.GetWorkflowID(in.State)
	if workflowID == "" && in.Task != nil && in.Task.Context != nil {
		if value, ok := in.Task.Context["workflow_id"]; ok {
			if id, ok := value.(string); ok {
				workflowID = id
			}
		}
	}

	policy := euclopolicy.ResolveRetrievalPolicy(in.Mode, in.Profile)
	euclostate.SetRetrievalPolicy(in.State, policy)

	expansion, err := eucloruntime.ExpandContext(ctx, surfaces.Workflow, workflowID, executionTask, in.State, policy)
	if err != nil {
		return ContextExpansionResult{
			ExecutionTask: executionTask,
			Err:           err,
		}
	}

	expandedTask := eucloruntime.ApplyContextExpansion(in.State, executionTask, expansion)
	return ContextExpansionResult{ExecutionTask: expandedTask}
}

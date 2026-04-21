package restore

import (
	"context"
	"fmt"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
	"codeburg.org/lexbit/relurpify/framework/memory/db"
	runtimepkg "codeburg.org/lexbit/relurpify/named/euclo/runtime"
	euclostate "codeburg.org/lexbit/relurpify/named/euclo/runtime/state"
)

type RuntimeSurfaces = runtimepkg.RuntimeSurfaces

func ResolveRuntimeSurfaces(store memory.MemoryStore) RuntimeSurfaces {
	switch typed := store.(type) {
	case *memory.CompositeRuntimeStore:
		surfaces := RuntimeSurfaces{Runtime: typed.RuntimeMemoryStore}
		if workflow, ok := typed.WorkflowStateStore.(*db.SQLiteWorkflowStateStore); ok {
			surfaces.Workflow = workflow
		}
		return surfaces
	case memory.RuntimeMemoryStore:
		return RuntimeSurfaces{Runtime: typed}
	default:
		return RuntimeSurfaces{}
	}
}

func EnsureWorkflowRun(ctx context.Context, store *db.SQLiteWorkflowStateStore, task *core.Task, state *core.Context) (string, string, error) {
	if store == nil {
		return "", "", nil
	}
	workflowID := contextString(state, "euclo.workflow_id")
	if workflowID == "" {
		workflowID = taskContextString(task, "workflow_id")
	}
	if workflowID == "" {
		workflowID = fmt.Sprintf("euclo-%s", fallbackTaskID(task))
	}
	runID := contextString(state, "euclo.run_id")
	if runID == "" {
		runID = taskContextString(task, "run_id")
	}
	if runID == "" {
		runID = fmt.Sprintf("%s-run-%d", fallbackTaskID(task), time.Now().UnixNano())
	}
	if _, ok, err := store.GetWorkflow(ctx, workflowID); err != nil {
		return "", "", err
	} else if !ok {
		if err := store.CreateWorkflow(ctx, memory.WorkflowRecord{
			WorkflowID:  workflowID,
			TaskID:      fallbackTaskID(task),
			TaskType:    fallbackTaskType(task),
			Instruction: fallbackInstruction(task),
			Status:      memory.WorkflowRunStatusRunning,
			Metadata:    map[string]any{"agent": "euclo"},
		}); err != nil {
			return "", "", err
		}
	}
	if _, ok, err := store.GetRun(ctx, runID); err != nil {
		return "", "", err
	} else if !ok {
		if err := store.CreateRun(ctx, memory.WorkflowRunRecord{
			RunID:      runID,
			WorkflowID: workflowID,
			Status:     memory.WorkflowRunStatusRunning,
			AgentName:  "euclo",
			AgentMode:  contextString(state, "euclo.mode"),
			StartedAt:  time.Now().UTC(),
		}); err != nil {
			return "", "", err
		}
	}
	if state != nil {
		euclostate.SetWorkflowID(state, workflowID)
		euclostate.SetRunID(state, runID)
	}

	// Update workflow metadata with workspace and mode for session resume support
	workspace := workspaceFromTask(task)
	mode := contextString(state, "euclo.mode")
	if workspace != "" || mode != "" {
		metadata := make(map[string]any)
		if workspace != "" {
			metadata["workspace"] = workspace
		}
		if mode != "" {
			metadata["mode"] = mode
		}
		_ = store.UpdateWorkflowMetadata(ctx, workflowID, metadata)
	}

	return workflowID, runID, nil
}

func workspaceFromTask(task *core.Task) string {
	if task == nil || task.Context == nil {
		return ""
	}
	if value, ok := task.Context["workspace"].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

func contextString(state *core.Context, key string) string {
	if state == nil {
		return ""
	}
	switch key {
	case "euclo.workflow_id":
		if value, ok := euclostate.GetWorkflowID(state); ok {
			return value
		}
	case "euclo.run_id":
		if value, ok := euclostate.GetRunID(state); ok {
			return value
		}
	case "euclo.mode":
		if value, ok := euclostate.GetMode(state); ok {
			return value
		}
	}
	return strings.TrimSpace(state.GetString(key))
}

func taskContextString(task *core.Task, key string) string {
	if task == nil || task.Context == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(task.Context[key]))
}

func fallbackTaskID(task *core.Task) string {
	if task != nil && strings.TrimSpace(task.ID) != "" {
		return strings.TrimSpace(task.ID)
	}
	return "task"
}

func fallbackTaskType(task *core.Task) core.TaskType {
	if task == nil || task.Type == "" {
		return core.TaskTypeCodeGeneration
	}
	return task.Type
}

func fallbackInstruction(task *core.Task) string {
	if task == nil {
		return ""
	}
	return task.Instruction
}

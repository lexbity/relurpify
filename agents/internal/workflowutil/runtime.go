package workflowutil

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/memory/db"
)

// RuntimeSurfaces exposes durable workflow and runtime-memory handles when the
// shared agent memory store is backed by the unified runtime stack.
type RuntimeSurfaces struct {
	Workflow *db.SQLiteWorkflowStateStore
	Runtime  memory.RuntimeMemoryStore
}

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

func EnsureWorkflowRun(ctx context.Context, store *db.SQLiteWorkflowStateStore, task *core.Task, state *core.Context, agentKey string) (string, string, error) {
	if store == nil {
		return "", "", nil
	}
	workflowID := strings.TrimSpace(contextString(state, agentKey+".workflow_id"))
	if workflowID == "" {
		workflowID = strings.TrimSpace(taskContextString(task, "workflow_id"))
	}
	if workflowID == "" {
		workflowID = fmt.Sprintf("%s-%s", agentKey, fallbackTaskID(task))
	}
	runID := strings.TrimSpace(contextString(state, agentKey+".run_id"))
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
			Metadata:    map[string]any{"agent": agentKey},
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
			AgentName:  agentKey,
			StartedAt:  time.Now().UTC(),
		}); err != nil {
			return "", "", err
		}
	}
	if state != nil {
		state.Set(agentKey+".workflow_id", workflowID)
		state.Set(agentKey+".run_id", runID)
	}
	return workflowID, runID, nil
}

func contextString(state *core.Context, key string) string {
	if state == nil {
		return ""
	}
	return state.GetString(key)
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

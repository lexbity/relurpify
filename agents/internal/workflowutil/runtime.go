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

func WorkflowArtifactReference(record memory.WorkflowArtifactRecord) core.ArtifactReference {
	return core.ArtifactReference{
		ArtifactID:   strings.TrimSpace(record.ArtifactID),
		WorkflowID:   strings.TrimSpace(record.WorkflowID),
		RunID:        strings.TrimSpace(record.RunID),
		Kind:         strings.TrimSpace(record.Kind),
		ContentType:  strings.TrimSpace(record.ContentType),
		StorageKind:  string(record.StorageKind),
		URI:          fmt.Sprintf("workflow://artifact/%s/%s/%s", strings.TrimSpace(record.WorkflowID), strings.TrimSpace(record.RunID), strings.TrimSpace(record.ArtifactID)),
		Summary:      strings.TrimSpace(record.SummaryText),
		Metadata:     artifactReferenceMetadata(record.SummaryMetadata),
		RawSizeBytes: record.RawSizeBytes,
	}
}

func StepArtifactReference(record memory.StepArtifactRecord) core.ArtifactReference {
	return core.ArtifactReference{
		ArtifactID:   strings.TrimSpace(record.ArtifactID),
		WorkflowID:   strings.TrimSpace(record.WorkflowID),
		StepRunID:    strings.TrimSpace(record.StepRunID),
		Kind:         strings.TrimSpace(record.Kind),
		ContentType:  strings.TrimSpace(record.ContentType),
		StorageKind:  string(record.StorageKind),
		URI:          fmt.Sprintf("workflow://step-artifact/%s/%s/%s", strings.TrimSpace(record.WorkflowID), strings.TrimSpace(record.StepRunID), strings.TrimSpace(record.ArtifactID)),
		Summary:      strings.TrimSpace(record.SummaryText),
		Metadata:     artifactReferenceMetadata(record.SummaryMetadata),
		RawSizeBytes: record.RawSizeBytes,
	}
}

func artifactReferenceMetadata(src map[string]any) map[string]string {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]string, len(src))
	for key, value := range src {
		text := strings.TrimSpace(fmt.Sprint(value))
		if text == "" || text == "<nil>" {
			continue
		}
		out[key] = text
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

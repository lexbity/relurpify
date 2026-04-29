package state

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
	"codeburg.org/lexbit/relurpify/named/rex/envelope"
	"codeburg.org/lexbit/relurpify/named/rex/rexkeys"
)

// Identity captures the durable workflow identity used by rex.
type Identity struct {
	WorkflowID string
	RunID      string
}

// RecoveryCandidate is resumable work discovered during recovery boot.
type RecoveryCandidate struct {
	WorkflowID string
	RunID      string
	Status     string
}

// ComputeIdentity returns deterministic workflow/run identity.
func ComputeIdentity(env envelope.Envelope) Identity {
	workflowID := strings.TrimSpace(env.WorkflowID)
	if workflowID == "" {
		sum := sha256.Sum256([]byte(strings.Join([]string{env.TaskID, env.Source, env.Instruction}, "::")))
		workflowID = "rex:" + hex.EncodeToString(sum[:])
	}
	runID := strings.TrimSpace(env.RunID)
	if runID == "" {
		runID = workflowID + ":run"
	}
	return Identity{WorkflowID: workflowID, RunID: runID}
}

type RuntimeSurfaces struct {
	Workflow *db.SQLiteWorkflowStateStore `json:"-"`
}

// ResolveRuntimeSurfaces exposes the workflow store when available.
func ResolveRuntimeSurfaces(mem memory.MemoryStore) RuntimeSurfaces {
	if typed, ok := mem.(*memory.CompositeRuntimeStore); ok {
		if workflow, ok := typed.WorkflowStateStore.(*db.SQLiteWorkflowStateStore); ok {
			return RuntimeSurfaces{Workflow: workflow}
		}
	}
	return RuntimeSurfaces{}
}

// RecoveryBoot scans workflow state for resumable work.
func RecoveryBoot(ctx context.Context, mem memory.MemoryStore) ([]RecoveryCandidate, error) {
	surfaces := ResolveRuntimeSurfaces(mem)
	if surfaces.Workflow == nil {
		return nil, nil
	}
	if lister, ok := any(surfaces.Workflow).(interface {
		ListRunsByStatus(context.Context, []memory.WorkflowRunStatus, int) ([]memory.WorkflowRunRecord, error)
	}); ok {
		runs, err := lister.ListRunsByStatus(ctx, []memory.WorkflowRunStatus{memory.WorkflowRunStatusRunning, memory.WorkflowRunStatusNeedsReplan}, 128)
		if err != nil {
			return nil, err
		}
		out := make([]RecoveryCandidate, 0, len(runs))
		for _, run := range runs {
			out = append(out, RecoveryCandidate{WorkflowID: run.WorkflowID, RunID: run.RunID, Status: string(run.Status)})
		}
		return out, nil
	}
	rows, err := surfaces.Workflow.ListWorkflows(ctx, 128)
	if err != nil {
		return nil, err
	}
	out := make([]RecoveryCandidate, 0, len(rows))
	for _, row := range rows {
		if row.Status != memory.WorkflowRunStatusRunning && row.Status != memory.WorkflowRunStatusNeedsReplan {
			continue
		}
		out = append(out, RecoveryCandidate{WorkflowID: row.WorkflowID, RunID: row.WorkflowID + ":run", Status: string(row.Status)})
	}
	return out, nil
}

// PersistIdentity seeds rex identifiers into state.
func PersistIdentity(ctx map[string]any, identity Identity) {
	if ctx == nil {
		return
	}
	ctx[rexkeys.RexWorkflowID] = identity.WorkflowID
	ctx[rexkeys.RexRunID] = identity.RunID
	ctx[rexkeys.WorkflowID] = identity.WorkflowID
	ctx[rexkeys.RunID] = identity.RunID
}

// PersistenceRequired reports whether workflow state is required.
func PersistenceRequired(requirePersistence bool) error {
	if !requirePersistence {
		return nil
	}
	return nil
}

func DescribeCandidate(candidate RecoveryCandidate) string {
	return fmt.Sprintf("%s:%s:%s", candidate.WorkflowID, candidate.RunID, candidate.Status)
}

func NewRunRecord(identity Identity, agentName, agentMode string) memory.WorkflowRunRecord {
	return memory.WorkflowRunRecord{
		RunID:          identity.RunID,
		WorkflowID:     identity.WorkflowID,
		Status:         memory.WorkflowRunStatusRunning,
		AgentName:      agentName,
		AgentMode:      agentMode,
		RuntimeVersion: "rex.v1",
		StartedAt:      time.Now().UTC(),
	}
}

// EnsureWorkflowRun ensures rex workflow and run state exist in the workflow store.
func EnsureWorkflowRun(ctx context.Context, store interface {
	CreateWorkflow(context.Context, memory.WorkflowRecord) error
	GetWorkflow(context.Context, string) (*memory.WorkflowRecord, bool, error)
	CreateRun(context.Context, memory.WorkflowRunRecord) error
	GetRun(context.Context, string) (*memory.WorkflowRunRecord, bool, error)
}, identity Identity, task *core.Task, agentMode string) error {
	if store == nil {
		return nil
	}
	if _, ok, err := store.GetWorkflow(ctx, identity.WorkflowID); err != nil {
		return err
	} else if !ok {
		if err := store.CreateWorkflow(ctx, memory.WorkflowRecord{
			WorkflowID:  identity.WorkflowID,
			TaskID:      fallbackTaskID(task),
			TaskType:    fallbackTaskType(task),
			Instruction: fallbackInstruction(task),
			Status:      memory.WorkflowRunStatusRunning,
			Metadata:    map[string]any{"agent": "rex"},
			CreatedAt:   time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
		}); err != nil {
			return err
		}
	}
	if _, ok, err := store.GetRun(ctx, identity.RunID); err != nil {
		return err
	} else if !ok {
		if err := store.CreateRun(ctx, NewRunRecord(identity, "rex", agentMode)); err != nil {
			return err
		}
	}
	return nil
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

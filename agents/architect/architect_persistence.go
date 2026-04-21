package architect

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
	"codeburg.org/lexbit/relurpify/framework/memory/db"
)

func (a *ArchitectAgent) resolveWorkflowIdentity(ctx context.Context, task *core.Task, store *db.SQLiteWorkflowStateStore) (string, bool, error) {
	if task == nil {
		return "", false, fmt.Errorf("task required")
	}
	if task.Context != nil {
		if raw, ok := task.Context["workflow_id"]; ok && strings.TrimSpace(fmt.Sprint(raw)) != "" {
			return strings.TrimSpace(fmt.Sprint(raw)), true, nil
		}
		if enabled, ok := task.Context["resume_latest_workflow"]; ok {
			if flag, ok := enabled.(bool); ok && flag {
				workflows, err := store.ListWorkflows(ctx, 1)
				if err != nil {
					return "", true, err
				}
				if len(workflows) > 0 {
					return workflows[0].WorkflowID, true, nil
				}
			}
		}
	}
	if strings.TrimSpace(task.ID) != "" {
		return strings.TrimSpace(task.ID), false, nil
	}
	return fmt.Sprintf("workflow_%d", time.Now().UnixNano()), false, nil
}

func (a *ArchitectAgent) workflowStateDBPath() string {
	if strings.TrimSpace(a.WorkflowStatePath) != "" {
		return filepath.Clean(a.WorkflowStatePath)
	}
	return ""
}

func (a *ArchitectAgent) openWorkflowStateStore() (*db.SQLiteWorkflowStateStore, error) {
	path := a.workflowStateDBPath()
	if path == "" {
		return nil, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	return db.NewSQLiteWorkflowStateStore(path)
}

func (a *ArchitectAgent) applyReplayDirectives(ctx context.Context, task *core.Task, store *db.SQLiteWorkflowStateStore, workflowID string) error {
	if task == nil || task.Context == nil {
		return nil
	}
	rerunFrom := optionalContextString(task.Context, "rerun_from_step_id")
	rerunStep := optionalContextString(task.Context, "rerun_step_id")
	rerunInvalidated, _ := task.Context["rerun_invalidated"].(bool)

	switch {
	case rerunFrom != "":
		return a.resetReplayFromStep(ctx, store, workflowID, rerunFrom, true)
	case rerunStep != "":
		return a.resetReplayFromStep(ctx, store, workflowID, rerunStep, true)
	case rerunInvalidated:
		return a.resetInvalidatedSteps(ctx, store, workflowID)
	default:
		return nil
	}
}

func (a *ArchitectAgent) resetReplayFromStep(ctx context.Context, store *db.SQLiteWorkflowStateStore, workflowID, stepID string, includeDependents bool) error {
	if err := store.UpdateStepStatus(ctx, workflowID, stepID, memory.StepStatusPending, "queued for replay"); err != nil {
		return err
	}
	if includeDependents {
		invalidations, err := store.InvalidateDependents(ctx, workflowID, stepID, "rerun requested")
		if err != nil {
			return err
		}
		for _, record := range invalidations {
			if err := store.UpdateStepStatus(ctx, workflowID, record.InvalidatedStepID, memory.StepStatusPending, "queued for replay"); err != nil {
				return err
			}
		}
	}
	_ = store.AppendEvent(ctx, memory.WorkflowEventRecord{
		EventID:    architectRecordID("rerun_requested"),
		WorkflowID: workflowID,
		StepID:     stepID,
		EventType:  "rerun_requested",
		Message:    fmt.Sprintf("Replay requested from step %s.", stepID),
		CreatedAt:  time.Now().UTC(),
	})
	return nil
}

func (a *ArchitectAgent) resetInvalidatedSteps(ctx context.Context, store *db.SQLiteWorkflowStateStore, workflowID string) error {
	steps, err := store.ListSteps(ctx, workflowID)
	if err != nil {
		return err
	}
	for _, step := range steps {
		if step.Status != memory.StepStatusInvalidated {
			continue
		}
		if err := store.UpdateStepStatus(ctx, workflowID, step.StepID, memory.StepStatusPending, "queued for replay"); err != nil {
			return err
		}
	}
	return nil
}

func (a *ArchitectAgent) persistStepKnowledge(ctx context.Context, store *db.SQLiteWorkflowStateStore, workflowID, stepRunID, stepID, summary string, status memory.StepStatus, stepTask *core.Task, result *core.Result, errorText string, createdAt time.Time) {
	if status == memory.StepStatusCompleted {
		_ = store.PutKnowledge(ctx, memory.KnowledgeRecord{
			RecordID:   architectRecordID("fact"),
			WorkflowID: workflowID,
			StepRunID:  stepRunID,
			StepID:     stepID,
			Kind:       memory.KnowledgeKindFact,
			Title:      "Completed step result",
			Content:    summary,
			Status:     "accepted",
			CreatedAt:  createdAt,
		})
		if stepTask != nil && stepTask.Context != nil {
			if previous := strings.TrimSpace(fmt.Sprint(stepTask.Context["previous_step_result"])); previous != "" {
				_ = store.PutKnowledge(ctx, memory.KnowledgeRecord{
					RecordID:   architectRecordID("decision"),
					WorkflowID: workflowID,
					StepRunID:  stepRunID,
					StepID:     stepID,
					Kind:       memory.KnowledgeKindDecision,
					Title:      "Step constraint",
					Content:    previous,
					Status:     "accepted",
					Metadata:   map[string]any{"source": "dependency_summary"},
					CreatedAt:  createdAt,
				})
			}
		}
		if result != nil && result.Data != nil {
			if text := strings.TrimSpace(fmt.Sprint(result.Data["text"])); text != "" && text != "<nil>" {
				_ = store.PutKnowledge(ctx, memory.KnowledgeRecord{
					RecordID:   architectRecordID("fact"),
					WorkflowID: workflowID,
					StepRunID:  stepRunID,
					StepID:     stepID,
					Kind:       memory.KnowledgeKindFact,
					Title:      "Execution text",
					Content:    text,
					Status:     "accepted",
					CreatedAt:  createdAt,
				})
			}
		}
		return
	}
	content := summary
	if strings.TrimSpace(errorText) != "" {
		content = errorText
	}
	_ = store.PutKnowledge(ctx, memory.KnowledgeRecord{
		RecordID:   architectRecordID("issue"),
		WorkflowID: workflowID,
		StepRunID:  stepRunID,
		StepID:     stepID,
		Kind:       memory.KnowledgeKindIssue,
		Title:      "Step failure",
		Content:    content,
		Status:     "open",
		CreatedAt:  createdAt,
	})
}

func (a *ArchitectAgent) stepNeedsReplanThreshold() int {
	if a != nil && a.Config != nil && a.Config.MaxIterations > 0 {
		return 3
	}
	return 3
}

func completedStepsFromRecords(steps []memory.WorkflowStepRecord) []string {
	out := make([]string, 0, len(steps))
	for _, step := range steps {
		if step.Status == memory.StepStatusCompleted {
			out = append(out, step.StepID)
		}
	}
	return out
}

func allStepsTerminal(steps []memory.WorkflowStepRecord) bool {
	if len(steps) == 0 {
		return true
	}
	for _, step := range steps {
		if step.Status == memory.StepStatusPending || step.Status == memory.StepStatusRunning {
			return false
		}
	}
	return true
}

func dependencySummary(runs []memory.StepRunRecord) string {
	if len(runs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(runs))
	for _, run := range runs {
		if strings.TrimSpace(run.Summary) == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s: %s", run.StepID, run.Summary))
	}
	return strings.Join(parts, "\n")
}

func nextStepAttempt(ctx context.Context, store *db.SQLiteWorkflowStateStore, workflowID, stepID string) int {
	runs, err := store.ListStepRuns(ctx, workflowID, stepID)
	if err != nil || len(runs) == 0 {
		return 1
	}
	return runs[len(runs)-1].Attempt + 1
}

func latestStepRunID(ctx context.Context, store *db.SQLiteWorkflowStateStore, workflowID, stepID string) string {
	runs, err := store.ListStepRuns(ctx, workflowID, stepID)
	if err != nil || len(runs) == 0 {
		return ""
	}
	return runs[len(runs)-1].StepRunID
}

func resultData(result *core.Result, errorText string) map[string]any {
	if result == nil {
		return map[string]any{"error": errorText}
	}
	if result.Data == nil {
		if errorText == "" {
			return map[string]any{}
		}
		return map[string]any{"error": errorText}
	}
	out := make(map[string]any, len(result.Data)+1)
	for k, v := range result.Data {
		out[k] = v
	}
	if errorText != "" {
		out["error"] = errorText
	}
	return out
}

func architectRecordID(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
}

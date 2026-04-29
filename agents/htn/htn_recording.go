package htn

import (
	"context"
	"fmt"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/agents/plan"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
)

// recordingPrimitiveAgent wraps a primitive executor and persists step outcomes
// to the runtime and workflow memory stores after each execution.
type recordingPrimitiveAgent struct {
	delegate graph.WorkflowExecutor
	workflow interface {
		PutKnowledge(context.Context, memory.KnowledgeRecord) error
		AppendEvent(context.Context, memory.WorkflowEventRecord) error
	}
	workflowID string
	runID      string
}

func (a *recordingPrimitiveAgent) BranchExecutor() (plan.WorkflowExecutor, error) {
	if a == nil {
		return &recordingPrimitiveAgent{}, nil
	}
	branch := &recordingPrimitiveAgent{
		workflow:   a.workflow,
		workflowID: a.workflowID,
		runID:      a.runID,
	}
	if provider, ok := a.delegate.(plan.BranchExecutorProvider); ok {
		exec, err := provider.BranchExecutor()
		if err != nil {
			return nil, err
		}
		branch.delegate = exec
		return branch, nil
	}
	branch.delegate = a.delegate
	return branch, nil
}

func (a *recordingPrimitiveAgent) Initialize(cfg *core.Config) error {
	if a == nil || a.delegate == nil {
		return nil
	}
	return a.delegate.Initialize(cfg)
}

func (a *recordingPrimitiveAgent) Capabilities() []core.Capability {
	if a == nil || a.delegate == nil {
		return nil
	}
	return a.delegate.Capabilities()
}

func (a *recordingPrimitiveAgent) BuildGraph(task *core.Task) (*graph.Graph, error) {
	if a == nil || a.delegate == nil {
		return nil, nil
	}
	return a.delegate.BuildGraph(task)
}

func (a *recordingPrimitiveAgent) Execute(ctx context.Context, task *core.Task, state *contextdata.Envelope) (*core.Result, error) {
	if a == nil || a.delegate == nil {
		return &core.Result{Success: true}, nil
	}
	result, err := a.delegate.Execute(ctx, task, state)
	a.persistStep(ctx, task, result, err)
	return result, err
}

func (a *recordingPrimitiveAgent) persistStep(ctx context.Context, task *core.Task, result *core.Result, execErr error) {
	stepID, stepTitle := htnStepMetadata(task)
	if stepID == "" {
		return
	}
	summary := htnResultSummary(result, execErr)
	now := time.Now().UTC()
	if a.workflow != nil && strings.TrimSpace(a.workflowID) != "" {
		kind := memory.KnowledgeKindFact
		title := "Primitive step result"
		status := "accepted"
		eventType := "step_completed"
		if execErr != nil {
			kind = memory.KnowledgeKindIssue
			title = "Primitive step failure"
			status = "open"
			eventType = "step_failed"
		}
		_ = a.workflow.PutKnowledge(ctx, memory.KnowledgeRecord{
			RecordID:   fmt.Sprintf("htn_knowledge_%d", now.UnixNano()),
			WorkflowID: a.workflowID,
			StepID:     stepID,
			Kind:       kind,
			Title:      title,
			Content:    summary,
			Status:     status,
			Metadata:   map[string]any{"agent": "htn", "run_id": a.runID},
			CreatedAt:  now,
		})
		_ = a.workflow.AppendEvent(ctx, memory.WorkflowEventRecord{
			EventID:    fmt.Sprintf("htn_event_%d", now.UnixNano()),
			WorkflowID: a.workflowID,
			RunID:      a.runID,
			StepID:     stepID,
			EventType:  eventType,
			Message:    summary,
			CreatedAt:  now,
		})
	}
}

// htnStepMetadata extracts the step ID and trimmed description from the task context.
func htnStepMetadata(task *core.Task) (string, string) {
	if task == nil || task.Context == nil {
		return "", ""
	}
	raw, ok := task.Context["current_step"]
	if !ok {
		return "", ""
	}
	switch step := raw.(type) {
	case core.PlanStep:
		return step.ID, strings.TrimSpace(step.Description)
	case *core.PlanStep:
		if step == nil {
			return "", ""
		}
		return step.ID, strings.TrimSpace(step.Description)
	default:
		return "", ""
	}
}

// htnResultSummary produces a human-readable summary from a step result or error.
func htnResultSummary(result *core.Result, execErr error) string {
	if execErr != nil {
		return execErr.Error()
	}
	if result == nil {
		return "step completed"
	}
	if text := strings.TrimSpace(fmt.Sprint(result.Data["text"])); text != "" && text != "<nil>" {
		return text
	}
	if len(result.Data) == 0 {
		return "step completed"
	}
	return fmt.Sprint(result.Data)
}

// htnStatus returns a status string for metadata based on whether execution failed.
func htnStatus(execErr error) string {
	if execErr != nil {
		return "failed"
	}
	return "completed"
}

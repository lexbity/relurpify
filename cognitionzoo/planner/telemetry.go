package planner

import (
	"context"
	"time"

	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/telemetry"
)

// emit delivers one planner lifecycle event to the agent's configured
// telemetry sink (spec §1.7). The planner already flows graph/node events
// through the agentgraph runtime; these paradigm-level events frame the
// plan-execute boundary that the graph model does not expose.
func (a *PlannerAgent) emit(ctx context.Context, eventType telemetry.EventType, message string, metadata map[string]any) {
	if a == nil || a.Config == nil || a.Config.Telemetry == nil {
		return
	}
	ev := telemetry.Event{
		Type:      eventType,
		Message:   message,
		Timestamp: time.Now().UTC(),
		Metadata:  metadata,
	}
	telemetry.StampCorrelation(ctx, &ev)
	a.Config.Telemetry.Emit(ev)
}

// planStarted opens the plan-execute lifecycle.
func (a *PlannerAgent) planStarted(ctx context.Context, task *execution.Task) {
	a.emit(ctx, telemetry.EventPlannerPlanStarted, "planner plan started", map[string]any{
		"task_id":  taskIDOf(task),
		"paradigm": "planner",
	})
}

// planCompleted closes the plan-execute lifecycle with the run outcome.
func (a *PlannerAgent) planCompleted(ctx context.Context, task *execution.Task, success bool) {
	a.emit(ctx, telemetry.EventPlannerPlanCompleted, "planner plan completed", map[string]any{
		"task_id":  taskIDOf(task),
		"success":  success,
		"paradigm": "planner",
	})
}

// planFailed closes the plan-execute lifecycle on a construction or execution
// error.
func (a *PlannerAgent) planFailed(ctx context.Context, task *execution.Task, err error) {
	metadata := map[string]any{"task_id": taskIDOf(task), "paradigm": "planner"}
	if err != nil {
		metadata["error"] = clipText(err.Error())
	}
	a.emit(ctx, telemetry.EventPlannerPlanFailed, "planner plan failed", metadata)
}

func taskIDOf(task *execution.Task) string {
	if task == nil {
		return ""
	}
	return task.ID
}

func clipText(text string) string {
	const max = 512
	if len(text) <= max {
		return text
	}
	return text[:max] + "...(truncated)"
}

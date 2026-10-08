package htn

import (
	"context"
	"time"

	pl "codeburg.org/lexbit/relurpify/cognitionzoo/plan"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/telemetry"
)

// emit delivers one HTN lifecycle event to the agent's configured telemetry
// sink (spec §1.7: silent paradigms must follow the standard
// telemetry.Telemetry + StampCorrelation path — no stubs, absence is nil).
// Correlation identifiers are stamped from ctx so the JSONL retains the
// session/run/trace join key.
func (a *HTNAgent) emit(ctx context.Context, eventType telemetry.EventType, message string, metadata map[string]any) {
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

// planStarted opens the HTN plan lifecycle.
func (a *HTNAgent) planStarted(ctx context.Context, task *execution.Task, plan *pl.Plan) {
	metadata := map[string]any{
		"task_id":    taskIDOf(task),
		"paradigm":   "htn",
		"step_count": 0,
	}
	if plan != nil {
		metadata["step_count"] = len(plan.Steps)
	}
	a.emit(ctx, telemetry.EventHTNPlanStarted, "htn plan started", metadata)
}

// stepStarted opens one plan step execution.
func (a *HTNAgent) stepStarted(ctx context.Context, step pl.PlanStep) {
	a.emit(ctx, telemetry.EventHTNStepStarted, "htn step started", map[string]any{
		"step_id":  step.ID,
		"operator": step.Tool,
		"paradigm": "htn",
	})
}

// stepCompleted closes one plan step execution. The plan executor invokes the
// AfterStep hook only for successful steps, so failed steps surface at the
// aggregate execution event rather than a per-step event.
func (a *HTNAgent) stepCompleted(ctx context.Context, step pl.PlanStep) {
	a.emit(ctx, telemetry.EventHTNStepCompleted, "htn step completed", map[string]any{
		"step_id":  step.ID,
		"operator": step.Tool,
		"paradigm": "htn",
	})
}

// executionCompleted closes the HTN run.
func (a *HTNAgent) executionCompleted(ctx context.Context, task *execution.Task, success bool, completedSteps, plannedSteps int) {
	a.emit(ctx, telemetry.EventHTNExecutionCompleted, "htn execution completed", map[string]any{
		"task_id":         taskIDOf(task),
		"success":         success,
		"completed_steps": completedSteps,
		"planned_steps":   plannedSteps,
		"paradigm":        "htn",
	})
}

// planFailed closes the HTN plan lifecycle on decomposition/preflight errors.
func (a *HTNAgent) planFailed(ctx context.Context, task *execution.Task, err error) {
	metadata := map[string]any{"task_id": taskIDOf(task), "paradigm": "htn"}
	if err != nil {
		metadata["error"] = clipText(err.Error())
	}
	a.emit(ctx, telemetry.EventHTNPlanFailed, "htn plan failed", metadata)
}

func taskIDOf(task *execution.Task) string {
	if task == nil {
		return ""
	}
	return task.ID
}

// clipText bounds free-form text payloads (task instructions, error strings)
// so the durable record cannot balloon and secret-bearing values are limited
// (NFR-7).
func clipText(text string) string {
	const max = 512
	if len(text) <= max {
		return text
	}
	return text[:max] + "...(truncated)"
}

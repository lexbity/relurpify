package goalcon

import (
	"context"
	"time"

	"codeburg.org/lexbit/relurpify/cognitionzoo/goalcon/types"
	"codeburg.org/lexbit/relurpify/cognitionzoo/plan"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/telemetry"
)

// emit delivers one goalcon lifecycle event to the agent's configured
// telemetry sink (spec §1.7: silent paradigms must follow the standard
// telemetry.Telemetry + StampCorrelation path — no stubs, absence is nil).
func (a *GoalConAgent) emit(ctx context.Context, eventType telemetry.EventType, message string, metadata map[string]any) {
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

// planStarted opens the backward-chaining plan lifecycle.
func (a *GoalConAgent) planStarted(ctx context.Context, goal types.GoalCondition, maxDepth int) {
	metadata := map[string]any{
		"paradigm":        "goalcon",
		"goal_predicates": len(goal.Predicates),
		"max_depth":       maxDepth,
	}
	if clip := clipText(goal.Description); clip != "" {
		metadata["description"] = clip
	}
	a.emit(ctx, telemetry.EventGoalConPlanStarted, "goalcon plan started", metadata)
}

// planCompleted reports the synthesized plan and its solver statistics.
func (a *GoalConAgent) planCompleted(ctx context.Context, plan *plan.Plan, depth, unsatisfied int) {
	metadata := map[string]any{
		"paradigm":     "goalcon",
		"search_depth": depth,
		"unsatisfied":  unsatisfied,
		"step_count":   0,
	}
	if plan != nil {
		metadata["step_count"] = len(plan.Steps)
	}
	a.emit(ctx, telemetry.EventGoalConPlanCompleted, "goalcon plan completed", metadata)
}

// planFailed closes the plan lifecycle on solver or execution errors before
// step completion.
func (a *GoalConAgent) planFailed(ctx context.Context, err error) {
	metadata := map[string]any{"paradigm": "goalcon"}
	if err != nil {
		metadata["error"] = clipText(err.Error())
	}
	a.emit(ctx, telemetry.EventGoalConPlanFailed, "goalcon plan failed", metadata)
}

// stepStarted opens one plan step execution.
func (a *GoalConAgent) stepStarted(ctx context.Context, step plan.PlanStep) {
	a.emit(ctx, telemetry.EventGoalConStepStarted, "goalcon step started", map[string]any{
		"step_id":  step.ID,
		"operator": step.Tool,
		"paradigm": "goalcon",
	})
}

// stepCompleted closes one plan step execution. The plan executor invokes its
// AfterStep hook only for successful steps, so failed steps surface at the
// aggregate completion event rather than a per-step event.
func (a *GoalConAgent) stepCompleted(ctx context.Context, step plan.PlanStep) {
	a.emit(ctx, telemetry.EventGoalConStepCompleted, "goalcon step completed", map[string]any{
		"step_id":  step.ID,
		"operator": step.Tool,
		"paradigm": "goalcon",
	})
}

// executionDone closes the goalcon run.
func (a *GoalConAgent) executionDone(ctx context.Context, taskID string, success bool, depth, unsatisfied int) {
	a.emit(ctx, telemetry.EventGoalConExecutionDone, "goalcon execution completed", map[string]any{
		"task_id":      taskID,
		"success":      success,
		"search_depth": depth,
		"unsatisfied":  unsatisfied,
		"paradigm":     "goalcon",
	})
}

func clipText(text string) string {
	const max = 512
	if len(text) <= max {
		return text
	}
	return text[:max] + "...(truncated)"
}

func taskIDOf(task *execution.Task) string {
	if task == nil {
		return ""
	}
	return task.ID
}

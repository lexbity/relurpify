package reflection

import (
	"context"
	"time"

	"codeburg.org/lexbit/relurpify/telemetry"
)

// emit delivers one reflection lifecycle event to the agent's configured
// telemetry sink (spec §1.7). Reflection runs through the agentgraph runtime
// (which already emits graph/node events); these paradigm-level events carry
// the review-iteration decisions that the graph model does not.
func (a *ReflectionAgent) emit(ctx context.Context, eventType telemetry.EventType, message string, metadata map[string]any) {
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

// emitIteration records one review/decide pass: whether the reviewer approved,
// whether the agent revises, and the assessment numbers behind the decision.
func (a *ReflectionAgent) emitIteration(ctx context.Context, iteration int, approve, revise bool, issueScore float64, blockingIssues int) {
	a.emit(ctx, telemetry.EventReflectionIteration, "reflection iteration", map[string]any{
		"iteration":       iteration,
		"approve":         approve,
		"revise":          revise,
		"issue_score":     issueScore,
		"blocking_issues": blockingIssues,
		"paradigm":        "reflection",
	})
}

// emitCompleted closes the reflection loop with the run outcome.
func (a *ReflectionAgent) emitCompleted(ctx context.Context, success bool, iterations int) {
	a.emit(ctx, telemetry.EventReflectionCompleted, "reflection completed", map[string]any{
		"success":    success,
		"iterations": iterations,
		"paradigm":   "reflection",
	})
}

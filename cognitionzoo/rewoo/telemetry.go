package rewoo

import (
	"context"
	"time"
)

// RewooTelemetry wraps telemetry client and emits ReWOO-specific events.
// Phase 8: Observability integration for all workflow phases.
// Emits structured telemetry events for monitoring and debugging.
type RewooTelemetry struct {
	client interface{} // core.Telemetry (avoiding import cycle)
	debugf func(string, ...interface{})
}

// NewRewooTelemetry creates a telemetry wrapper for ReWOO events.
func NewRewooTelemetry(client interface{}, debugf func(string, ...interface{})) *RewooTelemetry {
	if debugf == nil {
		debugf = func(string, ...interface{}) {}
	}
	return &RewooTelemetry{client: client, debugf: debugf}
}

// EmitPlanStart emits event when planning begins.
func (t *RewooTelemetry) EmitPlanStart(ctx context.Context, taskID string) {
	t.debugf("telemetry: plan_start task_id=%s", taskID)
	// Future: wire to actual telemetry client when methods are available
}

// EmitPlanComplete emits event when planning completes.
func (t *RewooTelemetry) EmitPlanComplete(ctx context.Context, taskID string, stepCount int, tokenCount int, duration time.Duration) {
	t.debugf("telemetry: plan_complete task_id=%s steps=%d tokens=%d duration=%dms", taskID, stepCount, tokenCount, duration.Milliseconds())
}

// EmitStepStart emits event when a step begins execution.
func (t *RewooTelemetry) EmitStepStart(ctx context.Context, taskID string, stepID string, tool string) {
	t.debugf("telemetry: step_start task_id=%s step_id=%s tool=%s", taskID, stepID, tool)
}

// EmitStepComplete emits event when a step completes successfully.
func (t *RewooTelemetry) EmitStepComplete(ctx context.Context, taskID string, stepID string, tool string, duration time.Duration) {
	t.debugf("telemetry: step_complete task_id=%s step_id=%s tool=%s duration=%dms", taskID, stepID, tool, duration.Milliseconds())
}

// EmitStepFailed emits event when a step fails.
func (t *RewooTelemetry) EmitStepFailed(ctx context.Context, taskID string, stepID string, tool string, err string, duration time.Duration) {
	t.debugf("telemetry: step_failed task_id=%s step_id=%s tool=%s error=%s duration=%dms", taskID, stepID, tool, err, duration.Milliseconds())
}

// EmitReplan emits event when a replan is triggered.
func (t *RewooTelemetry) EmitReplan(ctx context.Context, taskID string, attempt int, failureRatio float64, reason string) {
	t.debugf("telemetry: replan task_id=%s attempt=%d failure_ratio=%.2f reason=%s", taskID, attempt, failureRatio, reason)
}

// EmitSynthesisStart emits event when synthesis begins.
func (t *RewooTelemetry) EmitSynthesisStart(ctx context.Context, taskID string, stepsCompleted int) {
	t.debugf("telemetry: synthesis_start task_id=%s steps_completed=%d", taskID, stepsCompleted)
}

// EmitSynthesisComplete emits event when synthesis completes.
func (t *RewooTelemetry) EmitSynthesisComplete(ctx context.Context, taskID string, tokenCount int, duration time.Duration) {
	t.debugf("telemetry: synthesis_complete task_id=%s tokens=%d duration=%dms", taskID, tokenCount, duration.Milliseconds())
}

// EmitExecutionComplete emits event when entire workflow completes.
func (t *RewooTelemetry) EmitExecutionComplete(ctx context.Context, taskID string, stepsRun int, stepsOK int, totalDuration time.Duration) {
	successRatio := float64(stepsOK) / float64(stepsRun)
	t.debugf("telemetry: execution_complete task_id=%s steps=%d ok=%d success_ratio=%.2f duration=%dms", taskID, stepsRun, stepsOK, successRatio, totalDuration.Milliseconds())
}

// EmitCheckpoint emits event when a checkpoint is saved.
func (t *RewooTelemetry) EmitCheckpoint(ctx context.Context, taskID string, checkpointID string, phase string) {
	t.debugf("telemetry: checkpoint_saved task_id=%s checkpoint_id=%s phase=%s", taskID, checkpointID, phase)
}

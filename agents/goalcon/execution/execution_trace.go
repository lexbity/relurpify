package execution

import (
	"fmt"
	"time"
)

// ExecutionEvent represents a single event during plan execution.
type ExecutionEvent struct {
	Timestamp    time.Time
	EventType    string // "step_start", "step_complete", "step_error", "plan_start", "plan_complete"
	StepID       string
	ToolName     string
	Data         map[string]any
	ErrorMessage string
}

// ExecutionTrace tracks all events during a plan execution.
type ExecutionTrace struct {
	PlanGoal      string
	StartTime     time.Time
	EndTime       time.Time
	Events        []ExecutionEvent
	StepResults   map[string]*StepExecutionResult
}

// NewExecutionTrace creates a new execution trace.
func NewExecutionTrace(planGoal string) *ExecutionTrace {
	return &ExecutionTrace{
		PlanGoal:    planGoal,
		StartTime:   time.Now(),
		Events:      make([]ExecutionEvent, 0),
		StepResults: make(map[string]*StepExecutionResult),
	}
}

// RecordPlanStart records the start of plan execution.
func (t *ExecutionTrace) RecordPlanStart() {
	if t == nil {
		return
	}
	t.StartTime = time.Now()
	t.recordEvent(ExecutionEvent{
		EventType: "plan_start",
		Data: map[string]any{
			"goal": t.PlanGoal,
		},
	})
}

// RecordPlanComplete records the completion of plan execution.
func (t *ExecutionTrace) RecordPlanComplete(success bool, stepCount int) {
	if t == nil {
		return
	}
	t.EndTime = time.Now()
	t.recordEvent(ExecutionEvent{
		EventType: "plan_complete",
		Data: map[string]any{
			"success":        success,
			"step_count":     stepCount,
			"duration":       t.EndTime.Sub(t.StartTime).String(),
		},
	})
}

// RecordStepStart records the start of step execution.
func (t *ExecutionTrace) RecordStepStart(stepID, toolName string) {
	if t == nil {
		return
	}
	t.recordEvent(ExecutionEvent{
		EventType: "step_start",
		StepID:    stepID,
		ToolName:  toolName,
		Data: map[string]any{
			"step_id": stepID,
			"tool":    toolName,
		},
	})
}

// RecordStepComplete records the completion of step execution.
func (t *ExecutionTrace) RecordStepComplete(result *StepExecutionResult) {
	if t == nil || result == nil {
		return
	}

	// Store result
	t.StepResults[result.StepID] = result

	event := ExecutionEvent{
		EventType: "step_complete",
		StepID:    result.StepID,
		ToolName:  result.ToolName,
		Data: map[string]any{
			"step_id":   result.StepID,
			"tool":      result.ToolName,
			"success":   result.Success,
			"duration":  result.Duration.String(),
			"output":    result.Output,
			"retries":   result.Retries,
		},
	}

	if result.Error != nil {
		event.ErrorMessage = result.Error.Error()
	}

	t.recordEvent(event)
}

// RecordStepError records an error during step execution.
func (t *ExecutionTrace) RecordStepError(stepID, toolName string, err error) {
	if t == nil {
		return
	}

	t.recordEvent(ExecutionEvent{
		EventType:    "step_error",
		StepID:       stepID,
		ToolName:     toolName,
		ErrorMessage: err.Error(),
		Data: map[string]any{
			"step_id": stepID,
			"tool":    toolName,
			"error":   err.Error(),
		},
	})
}

// recordEvent appends an event to the trace.
func (t *ExecutionTrace) recordEvent(event ExecutionEvent) {
	if t == nil {
		return
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	t.Events = append(t.Events, event)
}

// Duration returns the total execution time.
func (t *ExecutionTrace) Duration() time.Duration {
	if t == nil || t.EndTime.IsZero() {
		return 0
	}
	return t.EndTime.Sub(t.StartTime)
}

// StepCount returns the number of steps executed.
func (t *ExecutionTrace) StepCount() int {
	if t == nil {
		return 0
	}
	return len(t.StepResults)
}

// SuccessCount returns the number of successful steps.
func (t *ExecutionTrace) SuccessCount() int {
	if t == nil {
		return 0
	}
	count := 0
	for _, result := range t.StepResults {
		if result != nil && result.Success {
			count++
		}
	}
	return count
}

// FailureCount returns the number of failed steps.
func (t *ExecutionTrace) FailureCount() int {
	if t == nil {
		return 0
	}
	return t.StepCount() - t.SuccessCount()
}

// Summary returns a human-readable execution summary.
func (t *ExecutionTrace) Summary() string {
	if t == nil {
		return "No trace"
	}

	total := t.StepCount()
	success := t.SuccessCount()
	fail := t.FailureCount()
	duration := t.Duration()

	return fmt.Sprintf(
		"Plan '%s': %d steps executed (%d succeeded, %d failed) in %v",
		t.PlanGoal, total, success, fail, duration,
	)
}

// EventsByType returns all events of a specific type.
func (t *ExecutionTrace) EventsByType(eventType string) []ExecutionEvent {
	if t == nil {
		return nil
	}

	var filtered []ExecutionEvent
	for _, event := range t.Events {
		if event.EventType == eventType {
			filtered = append(filtered, event)
		}
	}
	return filtered
}

// FailedSteps returns all failed step results.
func (t *ExecutionTrace) FailedSteps() []*StepExecutionResult {
	if t == nil {
		return nil
	}

	var failed []*StepExecutionResult
	for _, result := range t.StepResults {
		if result != nil && !result.Success {
			failed = append(failed, result)
		}
	}
	return failed
}

// CriticalPath analyzes the execution timeline and returns a critical path analysis.
// This identifies which steps took the longest.
func (t *ExecutionTrace) CriticalPath() []*StepExecutionResult {
	if t == nil || len(t.StepResults) == 0 {
		return nil
	}

	// Convert to slice
	results := make([]*StepExecutionResult, 0, len(t.StepResults))
	for _, result := range t.StepResults {
		if result != nil {
			results = append(results, result)
		}
	}

	// Sort by duration (bubble sort for simplicity)
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Duration > results[i].Duration {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	return results
}

// ToDebugString returns a detailed trace for debugging.
func (t *ExecutionTrace) ToDebugString() string {
	if t == nil {
		return "No trace"
	}

	output := fmt.Sprintf("=== Execution Trace ===\n")
	output += fmt.Sprintf("Plan Goal: %s\n", t.PlanGoal)
	output += fmt.Sprintf("Duration: %v\n", t.Duration())
	output += fmt.Sprintf("Steps: %d executed (%d success, %d failures)\n\n",
		t.StepCount(), t.SuccessCount(), t.FailureCount())

	output += "=== Events ===\n"
	for _, event := range t.Events {
		output += fmt.Sprintf("[%s] %s", event.Timestamp.Format("15:04:05"), event.EventType)
		if event.StepID != "" {
			output += fmt.Sprintf(" - %s", event.StepID)
		}
		if event.ToolName != "" {
			output += fmt.Sprintf(" (%s)", event.ToolName)
		}
		if event.ErrorMessage != "" {
			output += fmt.Sprintf(" ERROR: %s", event.ErrorMessage)
		}
		output += "\n"
	}

	if failedSteps := t.FailedSteps(); len(failedSteps) > 0 {
		output += "\n=== Failed Steps ===\n"
		for _, result := range failedSteps {
			output += fmt.Sprintf("- %s (%s): %v\n", result.StepID, result.ToolName, result.Error)
		}
	}

	if criticalPath := t.CriticalPath(); len(criticalPath) > 0 && len(criticalPath) > 3 {
		output += "\n=== Top 3 Slowest Steps ===\n"
		for i := 0; i < 3 && i < len(criticalPath); i++ {
			result := criticalPath[i]
			output += fmt.Sprintf("- %s (%s): %v\n", result.StepID, result.ToolName, result.Duration)
		}
	}

	return output
}

package runtime

import (
	"fmt"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
)

// publishTaskState records the active task in context under htn.* namespace.
func publishTaskState(state *core.Context, task *core.Task) {
	if state == nil || task == nil {
		return
	}
	summary := TaskState{
		ID:          task.ID,
		Type:        task.Type,
		Instruction: task.Instruction,
	}
	if len(task.Metadata) > 0 {
		summary.Metadata = mapsClone(task.Metadata)
	}
	state.Set(contextKeyTask, summary)
	state.Set(contextKeyTaskType, task.Type)
	state.SetKnowledge(contextKnowledgeTaskType, task.Type)
	mustPublishHTNState(state)
}

// publishMethodState records the selected method in context.
func publishMethodState(state *core.Context, method *Method) {
	if state == nil {
		return
	}
	summary := MethodState{}
	if method != nil {
		resolved := ResolveMethod(*method)
		summary = methodStateFromResolved(resolved)
	}
	state.Set(contextKeySelectedMethod, summary)
	state.SetKnowledge(contextKnowledgeMethod, summary.Name)
	mustPublishHTNState(state)
}

// publishResolvedMethodState records a resolved method in context.
func publishResolvedMethodState(state *core.Context, method *ResolvedMethod) {
	if state == nil {
		return
	}
	summary := MethodState{}
	if method != nil {
		summary = methodStateFromResolved(*method)
	}
	state.Set(contextKeySelectedMethod, summary)
	state.SetKnowledge(contextKnowledgeMethod, summary.Name)
	mustPublishHTNState(state)
}

// publishPlanState records the decomposed plan in context.
func publishPlanState(state *core.Context, plan *core.Plan) {
	if state == nil || plan == nil {
		return
	}
	cloned := *plan
	cloned.Steps = append([]core.PlanStep(nil), plan.Steps...)
	if plan.Dependencies != nil {
		cloned.Dependencies = make(map[string][]string, len(plan.Dependencies))
		for key, deps := range plan.Dependencies {
			cloned.Dependencies[key] = append([]string(nil), deps...)
		}
	}
	cloned.Files = append([]string(nil), plan.Files...)
	state.Set(contextKeyPlan, &cloned)
	execution := loadExecutionState(state)
	execution.PlannedStepCount = len(cloned.Steps)
	publishExecutionState(state, execution)
}

// publishWorkflowRetrieval records workflow retrieval results in context.
func publishWorkflowRetrieval(state *core.Context, payload any, applied bool) {
	if state == nil {
		return
	}
	if payload != nil {
		state.Set(contextKeyWorkflowRetrieval, payload)
		state.Set(contextKeyWorkflowRetrievalPayload, payload)
	}
	state.Set(contextKeyRetrievalApplied, applied)
	mustPublishHTNState(state)
}

// publishPreflightState records graph preflight results in context.
func publishPreflightState(state *core.Context, report *graph.PreflightReport, err error) {
	if state == nil {
		return
	}
	if report != nil {
		state.Set(contextKeyPreflightReport, report)
	} else {
		state.Set(contextKeyPreflightReport, nil)
	}
	if err != nil {
		state.Set(contextKeyPreflightError, err.Error())
	} else {
		state.Set(contextKeyPreflightError, "")
	}
	mustPublishHTNState(state)
}

// publishCheckpointState records a checkpoint snapshot in context.
func publishCheckpointState(state *core.Context, checkpointID, stageName string, stageIndex int, workflowID, runID string) {
	if state == nil {
		return
	}
	snapshot, _, err := LoadStateFromContext(state)
	if err != nil {
		state.Set(contextKeyStateError, err.Error())
		return
	}
	checkpoint := CheckpointState{
		SchemaVersion:  htnSchemaVersion,
		CheckpointID:   checkpointID,
		StageName:      stageName,
		StageIndex:     stageIndex,
		WorkflowID:     workflowID,
		RunID:          runID,
		CompletedSteps: append([]string(nil), completedStepsFromContext(state)...),
		Snapshot:       snapshot,
	}
	state.Set(contextKeyCheckpoint, checkpoint)
}

// publishResumeState records checkpoint resume information in context.
func publishResumeState(state *core.Context, checkpointID string) {
	if state == nil || checkpointID == "" {
		return
	}
	execution := loadExecutionState(state)
	execution.Resumed = true
	execution.ResumeCheckpointID = checkpointID
	publishExecutionState(state, execution)
	state.Set(contextKeyResumeCheckpointID, checkpointID)
}

// completedStepsFromContext extracts the list of completed step IDs from context.
func completedStepsFromContext(state *core.Context) []string {
	if state == nil {
		return nil
	}
	if raw, ok := state.Get(contextKeyCompletedSteps); ok {
		var typed []string
		if decodeContextValue(raw, &typed) {
			return append([]string(nil), typed...)
		}
	}
	return core.StringSliceFromContext(state, legacyPlanCompletedStepsKey)
}

// appendCompletedStep adds a step ID to the completed steps list.
func appendCompletedStep(state *core.Context, stepID string) {
	if state == nil || stepID == "" {
		return
	}
	completed := completedStepsFromContext(state)
	completed = append(completed, stepID)
	state.Set(legacyPlanCompletedStepsKey, completed)
	state.Set(contextKeyCompletedSteps, completed)
	execution := loadExecutionState(state)
	execution.CompletedSteps = append([]string(nil), completed...)
	execution.CompletedStepCount = len(completed)
	execution.LastCompletedStep = stepID
	publishExecutionState(state, execution)
}

// publishExecutionState records execution progress in context.
func publishExecutionState(state *core.Context, execution ExecutionState) {
	if state == nil {
		return
	}
	execution.CompletedSteps = append([]string(nil), execution.CompletedSteps...)
	execution.CompletedStepCount = len(execution.CompletedSteps)
	if execution.CompletedStepCount > 0 && execution.LastCompletedStep == "" {
		execution.LastCompletedStep = execution.CompletedSteps[execution.CompletedStepCount-1]
	}
	state.Set(contextKeyExecution, execution)
	state.Set(contextKeyCompletedSteps, append([]string(nil), execution.CompletedSteps...))
	state.Set(legacyPlanCompletedStepsKey, append([]string(nil), execution.CompletedSteps...))
	publishMetricsAndSummary(state, execution, state.GetString(contextKeyTermination))
	mustPublishHTNState(state)
}

// publishTerminationState records the execution termination reason in context.
func publishTerminationState(state *core.Context, termination string) {
	if state == nil {
		return
	}
	state.Set(contextKeyTermination, termination)
	state.SetKnowledge(contextKnowledgeTermination, termination)
	execution := loadExecutionState(state)
	publishMetricsAndSummary(state, execution, termination)
	mustPublishHTNState(state)
}

// publishMetricsAndSummary records metrics and knowledge summary in context.
func publishMetricsAndSummary(state *core.Context, execution ExecutionState, termination string) {
	if state == nil {
		return
	}
	metrics := Metrics{
		PlannedStepCount:   execution.PlannedStepCount,
		CompletedStepCount: execution.CompletedStepCount,
	}
	state.Set(contextKeyMetrics, metrics)
	taskType := fmt.Sprint(executionTaskType(state))
	methodName := ""
	if raw, ok := state.Get(contextKeySelectedMethod); ok {
		var method MethodState
		if decodeContextValue(raw, &method) {
			methodName = method.Name
		}
	}
	state.SetKnowledge(contextKnowledgeSummary, fmt.Sprintf(
		"task_type=%s method=%s planned=%d completed=%d termination=%s",
		taskType,
		methodName,
		metrics.PlannedStepCount,
		metrics.CompletedStepCount,
		termination,
	))
	state.Set(contextKeyMetrics, metrics)
}

// loadExecutionState extracts ExecutionState from context.
func loadExecutionState(state *core.Context) ExecutionState {
	if state == nil {
		return ExecutionState{}
	}
	var execution ExecutionState
	if raw, ok := state.Get(contextKeyExecution); ok && decodeContextValue(raw, &execution) {
		execution.CompletedSteps = completedStepsFromContext(state)
		execution.CompletedStepCount = len(execution.CompletedSteps)
		return execution
	}
	execution.CompletedSteps = completedStepsFromContext(state)
	execution.CompletedStepCount = len(execution.CompletedSteps)
	execution.ResumeCheckpointID = state.GetString(contextKeyResumeCheckpointID)
	execution.Resumed = execution.ResumeCheckpointID != ""
	return execution
}

// loadCheckpointState extracts CheckpointState from context.
func loadCheckpointState(state *core.Context) (*CheckpointState, bool) {
	if state == nil {
		return nil, false
	}
	raw, ok := state.Get(contextKeyCheckpoint)
	if !ok || raw == nil {
		return nil, false
	}
	var checkpoint CheckpointState
	if !decodeContextValue(raw, &checkpoint) {
		return nil, false
	}
	if checkpoint.SchemaVersion == 0 {
		checkpoint.SchemaVersion = htnSchemaVersion
	}
	checkpoint.CompletedSteps = append([]string(nil), checkpoint.CompletedSteps...)
	if checkpoint.Snapshot != nil {
		normalizeHTNState(checkpoint.Snapshot)
	}
	return &checkpoint, true
}

// executionTaskType extracts the current task type from context.
func executionTaskType(state *core.Context) core.TaskType {
	if state == nil {
		return ""
	}
	if raw, ok := state.Get(contextKeyTaskType); ok {
		var taskType core.TaskType
		if decodeContextValue(raw, &taskType) {
			return taskType
		}
	}
	return ""
}

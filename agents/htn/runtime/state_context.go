package runtime

import (
	"fmt"

	"codeburg.org/lexbit/relurpify/agents/plan"
	graph "codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
)

// publishTaskState records the active task in envelope working memory under htn.* namespace.
func publishTaskState(env *contextdata.Envelope, task *core.Task) {
	if env == nil || task == nil {
		return
	}
	summary := TaskState{
		ID:          task.ID,
		Type:        core.TaskType(task.Type),
		Instruction: task.Instruction,
	}
	if len(task.Metadata) > 0 {
		summary.Metadata = make(map[string]string, len(task.Metadata))
		for key, value := range task.Metadata {
			if s, ok := value.(string); ok {
				summary.Metadata[key] = s
			}
		}
	}
	env.SetWorkingValue(contextKeyTask, summary, contextdata.MemoryClassTask)
	env.SetWorkingValue(contextKeyTaskType, task.Type, contextdata.MemoryClassTask)
	env.SetWorkingValue(contextKnowledgeTaskType, task.Type, contextdata.MemoryClassTask)
	mustPublishHTNState(env)
}

// publishMethodState records the selected method in envelope working memory.
func publishMethodState(env *contextdata.Envelope, method *Method) {
	if env == nil {
		return
	}
	summary := MethodState{}
	if method != nil {
		resolved := ResolveMethod(*method)
		summary = methodStateFromResolved(resolved)
	}
	env.SetWorkingValue(contextKeySelectedMethod, summary, contextdata.MemoryClassTask)
	env.SetWorkingValue(contextKnowledgeMethod, summary.Name, contextdata.MemoryClassTask)
	mustPublishHTNState(env)
}

// publishResolvedMethodState records a resolved method in envelope working memory.
func publishResolvedMethodState(env *contextdata.Envelope, method *ResolvedMethod) {
	if env == nil {
		return
	}
	summary := MethodState{}
	if method != nil {
		summary = methodStateFromResolved(*method)
	}
	env.SetWorkingValue(contextKeySelectedMethod, summary, contextdata.MemoryClassTask)
	env.SetWorkingValue(contextKnowledgeMethod, summary.Name, contextdata.MemoryClassTask)
	mustPublishHTNState(env)
}

// publishPlanState records the decomposed plan in envelope working memory.
func publishPlanState(env *contextdata.Envelope, p *plan.Plan) {
	if env == nil || p == nil {
		return
	}
	cloned := *p
	cloned.Steps = append([]plan.PlanStep(nil), p.Steps...)
	if p.Dependencies != nil {
		cloned.Dependencies = make(map[string][]string, len(p.Dependencies))
		for key, deps := range p.Dependencies {
			cloned.Dependencies[key] = append([]string(nil), deps...)
		}
	}
	cloned.Files = append([]string(nil), p.Files...)
	env.SetWorkingValue(contextKeyPlan, &cloned, contextdata.MemoryClassTask)
	execution := loadExecutionState(env)
	execution.PlannedStepCount = len(cloned.Steps)
	publishExecutionState(env, execution)
}

// publishWorkflowRetrieval records workflow retrieval results in envelope working memory.
func publishWorkflowRetrieval(env *contextdata.Envelope, payload any, applied bool) {
	if env == nil {
		return
	}
	if payload != nil {
		env.SetWorkingValue(contextKeyWorkflowRetrieval, payload, contextdata.MemoryClassTask)
		env.SetWorkingValue(contextKeyWorkflowRetrievalPayload, payload, contextdata.MemoryClassTask)
	}
	env.SetWorkingValue(contextKeyRetrievalApplied, applied, contextdata.MemoryClassTask)
	mustPublishHTNState(env)
}

// publishPreflightState records graph preflight results in envelope working memory.
func publishPreflightState(env *contextdata.Envelope, report *graph.PreflightReport, err error) {
	if env == nil {
		return
	}
	if report != nil {
		env.SetWorkingValue(contextKeyPreflightReport, report, contextdata.MemoryClassTask)
	} else {
		env.SetWorkingValue(contextKeyPreflightReport, nil, contextdata.MemoryClassTask)
	}
	if err != nil {
		env.SetWorkingValue(contextKeyPreflightError, err.Error(), contextdata.MemoryClassTask)
	} else {
		env.SetWorkingValue(contextKeyPreflightError, "", contextdata.MemoryClassTask)
	}
	mustPublishHTNState(env)
}

// publishCheckpointState records a checkpoint snapshot in envelope working memory.
func publishCheckpointState(env *contextdata.Envelope, checkpointID, stageName string, stageIndex int, workflowID, runID string) {
	if env == nil {
		return
	}
	snapshot, _, err := LoadStateFromEnvelope(env)
	if err != nil {
		env.SetWorkingValue(contextKeyStateError, err.Error(), contextdata.MemoryClassTask)
		return
	}
	checkpoint := CheckpointState{
		SchemaVersion:  htnSchemaVersion,
		CheckpointID:   checkpointID,
		StageName:      stageName,
		StageIndex:     stageIndex,
		WorkflowID:     workflowID,
		RunID:          runID,
		CompletedSteps: append([]string(nil), completedStepsFromEnvelope(env)...),
		Snapshot:       snapshot,
	}
	env.SetWorkingValue(contextKeyCheckpoint, checkpoint, contextdata.MemoryClassTask)
}

// publishResumeState records checkpoint resume information in envelope working memory.
func publishResumeState(env *contextdata.Envelope, checkpointID string) {
	if env == nil || checkpointID == "" {
		return
	}
	execution := loadExecutionState(env)
	execution.Resumed = true
	execution.ResumeCheckpointID = checkpointID
	publishExecutionState(env, execution)
	env.SetWorkingValue(contextKeyResumeCheckpointID, checkpointID, contextdata.MemoryClassTask)
}

// completedStepsFromEnvelope extracts the list of completed step IDs from envelope working memory.
func completedStepsFromEnvelope(env *contextdata.Envelope) []string {
	if env == nil {
		return nil
	}
	if raw, ok := env.GetWorkingValue(contextKeyCompletedSteps); ok {
		var typed []string
		if decodeContextValue(raw, &typed) {
			return append([]string(nil), typed...)
		}
	}
	// Legacy fallback - try working memory with legacy key
	if raw, ok := env.GetWorkingValue(legacyPlanCompletedStepsKey); ok {
		var typed []string
		if decodeContextValue(raw, &typed) {
			return append([]string(nil), typed...)
		}
	}
	return nil
}

// appendCompletedStep adds a step ID to the completed steps list in envelope.
func appendCompletedStep(env *contextdata.Envelope, stepID string) {
	if env == nil || stepID == "" {
		return
	}
	completed := completedStepsFromEnvelope(env)
	completed = append(completed, stepID)
	env.SetWorkingValue(legacyPlanCompletedStepsKey, completed, contextdata.MemoryClassTask)
	env.SetWorkingValue(contextKeyCompletedSteps, completed, contextdata.MemoryClassTask)
	execution := loadExecutionState(env)
	execution.CompletedSteps = append([]string(nil), completed...)
	execution.CompletedStepCount = len(completed)
	execution.LastCompletedStep = stepID
	publishExecutionState(env, execution)
}

// publishExecutionState records execution progress in envelope working memory.
func publishExecutionState(env *contextdata.Envelope, execution ExecutionState) {
	if env == nil {
		return
	}
	execution.CompletedSteps = append([]string(nil), execution.CompletedSteps...)
	execution.CompletedStepCount = len(execution.CompletedSteps)
	if execution.CompletedStepCount > 0 && execution.LastCompletedStep == "" {
		execution.LastCompletedStep = execution.CompletedSteps[execution.CompletedStepCount-1]
	}
	env.SetWorkingValue(contextKeyExecution, execution, contextdata.MemoryClassTask)
	env.SetWorkingValue(contextKeyCompletedSteps, append([]string(nil), execution.CompletedSteps...), contextdata.MemoryClassTask)
	env.SetWorkingValue(legacyPlanCompletedStepsKey, append([]string(nil), execution.CompletedSteps...), contextdata.MemoryClassTask)
	termination := ""
	if raw, ok := env.GetWorkingValue(contextKeyTermination); ok {
		if s, ok := raw.(string); ok {
			termination = s
		}
	}
	publishMetricsAndSummary(env, execution, termination)
	mustPublishHTNState(env)
}

// publishTerminationState records the execution termination reason in envelope working memory.
func publishTerminationState(env *contextdata.Envelope, termination string) {
	if env == nil {
		return
	}
	env.SetWorkingValue(contextKeyTermination, termination, contextdata.MemoryClassTask)
	env.SetWorkingValue(contextKnowledgeTermination, termination, contextdata.MemoryClassTask)
	execution := loadExecutionState(env)
	publishMetricsAndSummary(env, execution, termination)
	mustPublishHTNState(env)
}

// publishMetricsAndSummary records metrics and knowledge summary in envelope.
func publishMetricsAndSummary(env *contextdata.Envelope, execution ExecutionState, termination string) {
	if env == nil {
		return
	}
	metrics := Metrics{
		PlannedStepCount:   execution.PlannedStepCount,
		CompletedStepCount: execution.CompletedStepCount,
	}
	env.SetWorkingValue(contextKeyMetrics, metrics, contextdata.MemoryClassTask)
	taskType := fmt.Sprint(executionTaskType(env))
	methodName := ""
	if raw, ok := env.GetWorkingValue(contextKeySelectedMethod); ok {
		var method MethodState
		if decodeContextValue(raw, &method) {
			methodName = method.Name
		}
	}
	env.SetWorkingValue(contextKnowledgeSummary, fmt.Sprintf(
		"task_type=%s method=%s planned=%d completed=%d termination=%s",
		taskType,
		methodName,
		metrics.PlannedStepCount,
		metrics.CompletedStepCount,
		termination,
	), contextdata.MemoryClassTask)
	env.SetWorkingValue(contextKeyMetrics, metrics, contextdata.MemoryClassTask)
}

// loadExecutionState extracts ExecutionState from envelope working memory.
func loadExecutionState(env *contextdata.Envelope) ExecutionState {
	if env == nil {
		return ExecutionState{}
	}
	var execution ExecutionState
	if raw, ok := env.GetWorkingValue(contextKeyExecution); ok && decodeContextValue(raw, &execution) {
		execution.CompletedSteps = completedStepsFromEnvelope(env)
		execution.CompletedStepCount = len(execution.CompletedSteps)
		return execution
	}
	execution.CompletedSteps = completedStepsFromEnvelope(env)
	execution.CompletedStepCount = len(execution.CompletedSteps)
	if raw, ok := env.GetWorkingValue(contextKeyResumeCheckpointID); ok {
		if checkpointID, ok := raw.(string); ok {
			execution.ResumeCheckpointID = checkpointID
		}
	}
	execution.Resumed = execution.ResumeCheckpointID != ""
	return execution
}

// loadCheckpointState extracts CheckpointState from envelope working memory.
func loadCheckpointState(env *contextdata.Envelope) (*CheckpointState, bool) {
	if env == nil {
		return nil, false
	}
	raw, ok := env.GetWorkingValue(contextKeyCheckpoint)
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

// executionTaskType extracts the current task type from envelope working memory.
func executionTaskType(env *contextdata.Envelope) core.TaskType {
	if env == nil {
		return ""
	}
	if raw, ok := env.GetWorkingValue(contextKeyTaskType); ok {
		var taskType core.TaskType
		if decodeContextValue(raw, &taskType) {
			return taskType
		}
	}
	return ""
}

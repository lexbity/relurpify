package workflowutil

import (
	"fmt"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/jobs"
)

// TaskJobEnvelope captures the task-shaped inputs that are migrated into the
// jobs boundary while preserving orchestration metadata.
type TaskJobEnvelope struct {
	Job          jobs.Job
	Execution    jobs.JobExecutionEnvelope
	Instruction  string
	TaskContext  map[string]any
	TaskMetadata map[string]string
}

// TaskToJob translates a task into a job envelope without collapsing routing
// metadata or task-specific execution controls.
func TaskToJob(task *core.Task) (TaskJobEnvelope, error) {
	if task == nil {
		return TaskJobEnvelope{}, fmt.Errorf("task required")
	}
	now := time.Now().UTC()
	jobID := fallbackTaskID(task)
	taskType := fallbackTaskType(task)
	instruction := fallbackInstruction(task)
	ctx := cloneTaskAnyMap(taskContext(task))
	metadata := cloneTaskStringMap(taskMetadata(task))

	job := jobs.Job{
		ID:             jobID,
		ParentJobID:    firstNonEmpty(stringValue(ctx["parent_job_id"]), stringValue(ctx["parent_job"])),
		RootWorkflowID: firstNonEmpty(stringValue(ctx["workflow_id"]), stringValue(ctx["root_workflow_id"])),
		TraceID:        firstNonEmpty(stringValue(ctx["run_id"]), stringValue(ctx["trace_id"])),
		IdempotencyKey: firstNonEmpty(stringValue(ctx["idempotency_key"]), stringValue(ctx["dedupe_key"])),
		Source:         "task",
		Spec: jobs.JobSpec{
			Kind:     string(taskType),
			Payload:  taskPayload(task, instruction, jobID, taskType, ctx, metadata),
			Queue:    firstNonEmpty(stringValue(ctx["queue"]), "default"),
			Priority: intValue(ctx["priority"], 0),
			WorkerSelector: jobs.WorkerSelector{
				WorkerIDs:           stringSlice(ctx["worker_ids"]),
				WorkerKinds:         stringSlice(ctx["worker_kinds"]),
				Labels:              selectorLabels(taskType, ctx, metadata),
				Capabilities:        stringSlice(ctx["required_capabilities"]),
				QueueAffinities:     stringSlice(ctx["queue_affinities"]),
				Localities:          stringSlice(ctx["localities"]),
				ResourceClass:       stringValue(ctx["resource_class"]),
				TrustDomain:         stringValue(ctx["trust_domain"]),
				RequireExclusiveUse: boolValue(ctx["exclusive_worker"]),
			},
			RetryPolicy: jobs.RetryPolicy{
				MaxAttempts: intValue(ctx["max_attempts"], 0),
				Backoff: jobs.BackoffPolicy{
					Strategy:     backoffStrategy(ctx),
					InitialDelay: durationValue(ctx["retry_initial_delay"], durationValue(ctx["retry_delay"], time.Second)),
					FixedDelay:   durationValue(ctx["retry_delay"], time.Second),
					MaxDelay:     durationValue(ctx["retry_max_delay"], 0),
					Multiplier:   floatValue(ctx["retry_multiplier"], 2),
					Jitter:       floatValue(ctx["retry_jitter"], 0),
				},
				RetryAfterLeaseExpiration: boolValue(ctx["retry_after_lease_expiration"]),
				RetryBudgetCap:            intValue(ctx["retry_budget_cap"], 0),
			},
			CancelPolicy: jobs.CancelPolicy{
				Mode:           cancelMode(ctx, jobs.CancelModeBestEffort),
				GracePeriod:    durationValue(ctx["cancel_grace_period"], 0),
				ReasonRequired: boolValue(ctx["cancel_reason_required"]),
			},
			ResumePolicy: jobs.ResumePolicy{
				Mode:                 translatedResumeMode(ctx, jobID),
				RequiresCheckpoint:   boolValue(ctx["requires_checkpoint"]),
				CheckpointKey:        translatedCheckpointKey(ctx, jobID),
				AllowAfterFailure:    boolValue(ctx["resume_after_failure"]),
				AllowAfterExpiration: boolValue(ctx["resume_after_expiration"]),
			},
			TimeoutPolicy: jobs.TimeoutPolicy{
				Execution:   durationValue(ctx["timeout"], 0),
				QueueWait:   durationValue(ctx["queue_wait"], 0),
				GracePeriod: durationValue(ctx["cancel_grace_period"], 0),
			},
			LeasePolicy: jobs.LeasePolicy{
				Duration:          durationValue(ctx["lease_duration"], time.Minute),
				HeartbeatInterval: durationValue(ctx["heartbeat_interval"], 0),
				MaxRenewals:       intValue(ctx["max_renewals"], 0),
			},
			CorrelationID:        stringValue(ctx["correlation_id"]),
			PreferredWorkers:     stringSlice(ctx["preferred_workers"]),
			RequiredCapabilities: append([]string{}, stringSlice(ctx["required_capabilities"])...),
			Notes:                stringValue(ctx["notes"]),
		},
		State:              jobs.JobStateQueued,
		CreatedAt:          now,
		UpdatedAt:          now,
		ResumeCheckpointID: firstNonEmpty(stringValue(ctx["resume_checkpoint_id"]), stringValue(ctx["checkpoint_id"])),
		Labels:             taskLabels(taskType, ctx, metadata),
		Metadata:           taskMetadataEnvelope(ctx, metadata, jobID, taskType, instruction),
	}
	if err := ensureSelectorFallbacks(&job, taskType); err != nil {
		return TaskJobEnvelope{}, err
	}
	if err := job.Validate(); err != nil {
		return TaskJobEnvelope{}, fmt.Errorf("translated job invalid: %w", err)
	}
	execution := jobs.NewJobExecutionEnvelope(job)
	execution.RouteMetadata["task_id"] = job.ID
	execution.RouteMetadata["task_type"] = string(taskType)
	execution.RouteMetadata["instruction"] = instruction
	execution.RouteMetadata["workflow_id"] = job.RootWorkflowID
	execution.RouteMetadata["run_id"] = job.TraceID
	execution.RouteMetadata["resume_checkpoint_id"] = job.ResumeCheckpointID
	execution.RouteMetadata["task_context"] = cloneTaskAnyMap(ctx)
	execution.RouteMetadata["task_metadata"] = cloneTaskStringMap(metadata)
	return TaskJobEnvelope{
		Job:          job,
		Execution:    execution,
		Instruction:  instruction,
		TaskContext:  ctx,
		TaskMetadata: metadata,
	}, nil
}

func taskContext(task *core.Task) map[string]any {
	if task == nil || task.Context == nil {
		return nil
	}
	return task.Context
}

func taskMetadata(task *core.Task) map[string]string {
	if task == nil || task.Metadata == nil {
		return nil
	}
	return task.Metadata
}

func cloneTaskStringMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func cloneTaskAnyMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func taskPayload(task *core.Task, instruction, jobID string, taskType core.TaskType, ctx map[string]any, metadata map[string]string) map[string]any {
	payload := map[string]any{
		"instruction": instruction,
		"task_id":     jobID,
		"task_type":   string(taskType),
	}
	if task != nil && task.Context != nil {
		payload["task_context"] = cloneTaskAnyMap(ctx)
	}
	if task != nil && task.Metadata != nil {
		payload["task_metadata"] = cloneTaskStringMap(metadata)
	}
	return payload
}

func taskLabels(taskType core.TaskType, ctx map[string]any, metadata map[string]string) map[string]string {
	labels := sanitizeTaskStringMap(metadata)
	if labels == nil {
		labels = map[string]string{}
	}
	for _, key := range []string{"workflow_id", "run_id", "resume_checkpoint_id", "checkpoint_id"} {
		if value := strings.TrimSpace(stringValue(ctx[key])); value != "" {
			labels[key] = value
		}
	}
	if _, ok := labels["task_type"]; !ok && strings.TrimSpace(string(taskType)) != "" {
		labels["task_type"] = string(taskType)
	}
	return labels
}

func selectorLabels(taskType core.TaskType, ctx map[string]any, metadata map[string]string) map[string]string {
	labels := sanitizeTaskStringMap(metadata)
	if labels == nil {
		labels = map[string]string{}
	}
	for _, key := range []string{"workflow_id", "run_id", "resume_checkpoint_id", "queue"} {
		if value := strings.TrimSpace(stringValue(ctx[key])); value != "" {
			labels[key] = value
		}
	}
	if len(labels) == 0 {
		labels["task_type"] = string(taskType)
	}
	return labels
}

func ensureSelectorFallbacks(job *jobs.Job, taskType core.TaskType) error {
	if job == nil {
		return fmt.Errorf("job required")
	}
	if len(job.Spec.WorkerSelector.WorkerIDs) == 0 &&
		len(job.Spec.WorkerSelector.WorkerKinds) == 0 &&
		len(job.Spec.WorkerSelector.Capabilities) == 0 &&
		len(job.Spec.WorkerSelector.QueueAffinities) == 0 &&
		len(job.Spec.WorkerSelector.Localities) == 0 &&
		strings.TrimSpace(job.Spec.WorkerSelector.ResourceClass) == "" &&
		strings.TrimSpace(job.Spec.WorkerSelector.TrustDomain) == "" &&
		len(job.Spec.WorkerSelector.Labels) == 0 {
		job.Spec.WorkerSelector.Labels = map[string]string{
			"task_type": string(taskType),
		}
	}
	return nil
}

func sanitizeTaskStringMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func taskMetadataEnvelope(ctx map[string]any, metadata map[string]string, jobID string, taskType core.TaskType, instruction string) map[string]any {
	out := cloneTaskAnyMap(ctx)
	if out == nil {
		out = map[string]any{}
	}
	out["task_id"] = jobID
	out["task_type"] = string(taskType)
	out["instruction"] = instruction
	if len(metadata) > 0 {
		out["task_metadata"] = cloneTaskStringMap(metadata)
	}
	return out
}

func stringValue(raw any) string {
	return strings.TrimSpace(fmt.Sprint(raw))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func stringSlice(raw any) []string {
	switch typed := raw.(type) {
	case []string:
		out := make([]string, 0, len(typed))
		for _, value := range typed {
			value = strings.TrimSpace(value)
			if value != "" {
				out = append(out, value)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(typed))
		for _, value := range typed {
			text := strings.TrimSpace(fmt.Sprint(value))
			if text != "" && text != "<nil>" {
				out = append(out, text)
			}
		}
		return out
	default:
		if text := strings.TrimSpace(fmt.Sprint(raw)); text != "" && text != "<nil>" {
			return []string{text}
		}
		return nil
	}
}

func boolValue(raw any) bool {
	switch typed := raw.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
	}
}

func intValue(raw any, fallback int) int {
	switch typed := raw.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case float32:
		return int(typed)
	default:
		return fallback
	}
}

func floatValue(raw any, fallback float64) float64 {
	switch typed := raw.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	default:
		return fallback
	}
}

func durationValue(raw any, fallback time.Duration) time.Duration {
	switch typed := raw.(type) {
	case time.Duration:
		return typed
	case int:
		return time.Duration(typed)
	case int64:
		return time.Duration(typed)
	case float64:
		return time.Duration(typed)
	case string:
		if parsed, err := time.ParseDuration(strings.TrimSpace(typed)); err == nil {
			return parsed
		}
	}
	return fallback
}

func backoffStrategy(ctx map[string]any) jobs.BackoffStrategy {
	if value := strings.TrimSpace(stringValue(ctx["retry_strategy"])); value != "" {
		return jobs.BackoffStrategy(value)
	}
	if durationValue(ctx["retry_delay"], 0) > 0 {
		return jobs.BackoffStrategyFixed
	}
	if durationValue(ctx["retry_initial_delay"], 0) > 0 {
		return jobs.BackoffStrategyExponential
	}
	return jobs.BackoffStrategyFixed
}

func cancelMode(ctx map[string]any, fallback jobs.CancelMode) jobs.CancelMode {
	if text := strings.TrimSpace(stringValue(ctx["cancel_mode"])); text != "" && text != "<nil>" {
		return jobs.CancelMode(text)
	}
	return fallback
}

func translatedResumeMode(ctx map[string]any, jobID string) jobs.ResumeMode {
	key := translatedCheckpointKey(ctx, jobID)
	if text := strings.TrimSpace(stringValue(ctx["resume_mode"])); text != "" && text != "<nil>" {
		mode := jobs.ResumeMode(text)
		if mode == jobs.ResumeModeDisabled && key != "" {
			return jobs.ResumeModeCheckpoint
		}
		return mode
	}
	if key != "" {
		return jobs.ResumeModeCheckpoint
	}
	return jobs.ResumeModeDisabled
}

func translatedCheckpointKey(ctx map[string]any, jobID string) string {
	key := firstNonEmpty(stringValue(ctx["resume_checkpoint_id"]), stringValue(ctx["checkpoint_id"]))
	if key != "" {
		return key
	}
	if mode := strings.TrimSpace(stringValue(ctx["resume_mode"])); mode == string(jobs.ResumeModeCheckpoint) {
		return jobID
	}
	return ""
}

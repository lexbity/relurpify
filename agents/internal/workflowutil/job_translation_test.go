package workflowutil

import (
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/jobs"
)

func TestTaskToJobTranslationPreservesExecutionControls(t *testing.T) {
	task := &core.Task{
		ID:          "task-123",
		Type:        core.TaskTypeAnalysis,
		Instruction: "continue the workflow",
		Context: map[string]any{
			"workflow_id":                  "wf-9",
			"run_id":                       "run-7",
			"resume_checkpoint_id":         "checkpoint-42",
			"required_capabilities":        []any{"graph.read", "memory.write"},
			"worker_kinds":                 []string{"pipeline"},
			"queue":                        "deferred",
			"priority":                     17,
			"retry_strategy":               string(jobs.BackoffStrategyFixed),
			"retry_delay":                  "2s",
			"max_attempts":                 3,
			"cancel_mode":                  string(jobs.CancelModeCooperative),
			"resume_mode":                  string(jobs.ResumeModeCheckpoint),
			"lease_duration":               "45s",
			"heartbeat_interval":           "15s",
			"queue_wait":                   "5s",
			"cancel_grace_period":          "10s",
			"exclusive_worker":             true,
			"preferred_workers":            []any{"worker-a", "worker-b"},
			"queue_affinities":             []string{"deferred"},
			"resource_class":               "cpu-heavy",
			"trust_domain":                 "workspace",
			"correlation_id":               "corr-1",
			"idempotency_key":              "idem-1",
			"notes":                        "preserve controls",
			"retry_after_lease_expiration": true,
		},
		Metadata: map[string]string{
			"source": "pipeline",
			"team":   "ops",
		},
	}

	envelope, err := TaskToJob(task)
	if err != nil {
		t.Fatalf("unexpected translation error: %v", err)
	}

	if envelope.Job.ID != "task-123" {
		t.Fatalf("expected job id to mirror task id, got %q", envelope.Job.ID)
	}
	if envelope.Job.RootWorkflowID != "wf-9" {
		t.Fatalf("expected root workflow id preserved, got %q", envelope.Job.RootWorkflowID)
	}
	if envelope.Job.TraceID != "run-7" {
		t.Fatalf("expected run id preserved, got %q", envelope.Job.TraceID)
	}
	if envelope.Job.ResumeCheckpointID != "checkpoint-42" {
		t.Fatalf("expected resume checkpoint preserved, got %q", envelope.Job.ResumeCheckpointID)
	}
	if envelope.Job.Spec.Queue != "deferred" {
		t.Fatalf("expected queue preserved, got %q", envelope.Job.Spec.Queue)
	}
	if envelope.Job.Spec.Priority != 17 {
		t.Fatalf("expected priority preserved, got %d", envelope.Job.Spec.Priority)
	}
	if got := envelope.Job.Spec.WorkerSelector.ResourceClass; got != "cpu-heavy" {
		t.Fatalf("expected resource class preserved, got %q", got)
	}
	if got := envelope.Job.Spec.WorkerSelector.TrustDomain; got != "workspace" {
		t.Fatalf("expected trust domain preserved, got %q", got)
	}
	if got := envelope.Job.Spec.WorkerSelector.WorkerKinds; len(got) != 1 || got[0] != "pipeline" {
		t.Fatalf("expected worker kind preserved, got %#v", got)
	}
	if got := envelope.Job.Spec.WorkerSelector.Capabilities; len(got) != 2 || got[0] != "graph.read" || got[1] != "memory.write" {
		t.Fatalf("expected required capabilities preserved, got %#v", got)
	}
	if got := envelope.Job.Spec.WorkerSelector.Labels["workflow_id"]; got != "wf-9" {
		t.Fatalf("expected workflow label preserved, got %q", got)
	}
	if got := envelope.Job.Spec.WorkerSelector.Labels["resume_checkpoint_id"]; got != "checkpoint-42" {
		t.Fatalf("expected checkpoint label preserved, got %q", got)
	}
	if envelope.Job.Spec.RetryPolicy.MaxAttempts != 3 {
		t.Fatalf("expected retry attempts preserved, got %d", envelope.Job.Spec.RetryPolicy.MaxAttempts)
	}
	if envelope.Job.Spec.ResumePolicy.Mode != jobs.ResumeModeCheckpoint {
		t.Fatalf("expected checkpoint resume mode, got %q", envelope.Job.Spec.ResumePolicy.Mode)
	}
	if envelope.Job.Spec.ResumePolicy.CheckpointKey != "checkpoint-42" {
		t.Fatalf("expected checkpoint key preserved, got %q", envelope.Job.Spec.ResumePolicy.CheckpointKey)
	}
	if envelope.Execution.RouteMetadata["workflow_id"] != "wf-9" {
		t.Fatalf("expected execution workflow metadata preserved, got %#v", envelope.Execution.RouteMetadata["workflow_id"])
	}
	if envelope.Execution.RouteMetadata["run_id"] != "run-7" {
		t.Fatalf("expected execution run metadata preserved, got %#v", envelope.Execution.RouteMetadata["run_id"])
	}
	if envelope.Execution.RouteMetadata["resume_checkpoint_id"] != "checkpoint-42" {
		t.Fatalf("expected execution checkpoint metadata preserved, got %#v", envelope.Execution.RouteMetadata["resume_checkpoint_id"])
	}
	if envelope.Execution.RouteMetadata["task_id"] != "task-123" {
		t.Fatalf("expected task id routed, got %#v", envelope.Execution.RouteMetadata["task_id"])
	}
	if envelope.Execution.RouteMetadata["task_type"] != string(core.TaskTypeAnalysis) {
		t.Fatalf("expected task type routed, got %#v", envelope.Execution.RouteMetadata["task_type"])
	}
	if envelope.TaskContext["workflow_id"] != "wf-9" {
		t.Fatalf("expected task context preserved, got %#v", envelope.TaskContext["workflow_id"])
	}
	if envelope.TaskMetadata["team"] != "ops" {
		t.Fatalf("expected task metadata preserved, got %#v", envelope.TaskMetadata["team"])
	}
}

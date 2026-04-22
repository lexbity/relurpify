package jobs

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type recordingJobExecutor struct {
	envelope JobExecutionEnvelope
}

func (r *recordingJobExecutor) ExecuteJob(_ context.Context, envelope JobExecutionEnvelope) (map[string]any, error) {
	r.envelope = envelope
	return envelope.RoutingMetadata(), nil
}

func TestAgentWorkerConsumesJobWithoutLosingRoutingMetadata(t *testing.T) {
	job := Job{
		ID:             "job-1",
		ParentJobID:    "job-parent",
		RootWorkflowID: "wf-1",
		TraceID:        "trace-1",
		IdempotencyKey: "idem-1",
		Source:         "task-1",
		Spec: JobSpec{
			Kind:     "workflow.step",
			Payload:  map[string]any{"step": "inspect"},
			Queue:    "default",
			Priority: 7,
			WorkerSelector: WorkerSelector{
				WorkerIDs:    []string{"worker-a"},
				Labels:       map[string]string{"tier": "ops"},
				Capabilities: []string{"search"},
			},
			RetryPolicy: RetryPolicy{
				MaxAttempts: 2,
				Backoff:     BackoffPolicy{Strategy: BackoffStrategyFixed, FixedDelay: 5 * time.Second, MaxDelay: 5 * time.Second},
			},
			CancelPolicy:         CancelPolicy{Mode: CancelModeBestEffort},
			ResumePolicy:         ResumePolicy{Mode: ResumeModeRestartable},
			TimeoutPolicy:        TimeoutPolicy{Execution: time.Minute},
			LeasePolicy:          LeasePolicy{Duration: time.Minute},
			RequiredCapabilities: []string{"tool.read"},
		},
		State:          JobStateLeased,
		AttemptCount:   1,
		CreatedAt:      time.Date(2026, 4, 21, 20, 0, 0, 0, time.UTC),
		AvailableAt:    time.Date(2026, 4, 21, 20, 0, 0, 0, time.UTC),
		NextRetryAt:    time.Date(2026, 4, 21, 20, 30, 0, 0, time.UTC),
		CurrentAttempt: &JobAttempt{ID: "attempt-1", JobID: "job-1", AttemptNumber: 1, State: JobStateLeased},
		CurrentLease:   &JobLease{ID: "lease-1", JobID: "job-1", AttemptID: "attempt-1", WorkerID: "worker-a", AcquiredAt: time.Date(2026, 4, 21, 20, 0, 0, 0, time.UTC), ExpiresAt: time.Date(2026, 4, 21, 20, 5, 0, 0, time.UTC)},
	}

	executor := &recordingJobExecutor{}
	adapter := WorkerAdapter{Executor: executor}

	metadata, err := adapter.Execute(context.Background(), job)
	require.NoError(t, err)
	require.Equal(t, "job-1", executor.envelope.JobID)
	require.Equal(t, "worker-a", executor.envelope.WorkerID)
	require.Equal(t, "lease-1", executor.envelope.LeaseID)
	require.Equal(t, "attempt-1", executor.envelope.AttemptID)
	require.Equal(t, "worker-a", executor.envelope.WorkerSelector.WorkerIDs[0])
	require.Equal(t, "ops", executor.envelope.WorkerSelector.Labels["tier"])
	require.Equal(t, []string{"tool.read"}, executor.envelope.RequiredCapabilities)
	require.Equal(t, "job-parent", metadata["job_parent_id"])
	require.Equal(t, "wf-1", metadata["root_workflow_id"])
	require.Equal(t, "trace-1", metadata["trace_id"])
	require.Equal(t, "idem-1", metadata["idempotency_key"])
	require.Equal(t, "task-1", metadata["source"])
	require.Equal(t, "lease-1", metadata["lease_id"])
	require.Equal(t, "worker-a", metadata["worker_id"])
	require.Equal(t, 7, metadata["priority"])
}

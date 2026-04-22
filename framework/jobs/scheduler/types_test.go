package scheduler

import (
	"context"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/jobs"
	"codeburg.org/lexbit/relurpify/framework/jobs/store"
	"github.com/stretchr/testify/require"
)

func TestSchedulerSelectsEligibleWorkerBySelectorAndQueuePriority(t *testing.T) {
	jobStore := store.NewMemoryJobStore()
	now := time.Date(2026, 4, 21, 18, 45, 0, 0, time.UTC)

	jobsToCreate := []jobs.Job{
		{
			ID: "job-low",
			Spec: jobs.JobSpec{
				Kind:     "workflow.step",
				Payload:  map[string]any{"step": "low"},
				Queue:    "default",
				Priority: 1,
				WorkerSelector: jobs.WorkerSelector{
					WorkerIDs:    []string{"worker-a"},
					Labels:       map[string]string{"tier": "ops"},
					Capabilities: []string{"search"},
				},
				RetryPolicy: jobs.RetryPolicy{
					MaxAttempts: 1,
					Backoff: jobs.BackoffPolicy{
						Strategy:   jobs.BackoffStrategyFixed,
						FixedDelay: 5 * time.Second,
						MaxDelay:   5 * time.Second,
					},
				},
				CancelPolicy:  jobs.CancelPolicy{Mode: jobs.CancelModeBestEffort},
				ResumePolicy:  jobs.ResumePolicy{Mode: jobs.ResumeModeDisabled},
				TimeoutPolicy: jobs.TimeoutPolicy{Execution: time.Minute},
				LeasePolicy:   jobs.LeasePolicy{Duration: time.Minute},
			},
			State:     jobs.JobStateQueued,
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID: "job-high",
			Spec: jobs.JobSpec{
				Kind:     "workflow.step",
				Payload:  map[string]any{"step": "high"},
				Queue:    "default",
				Priority: 10,
				WorkerSelector: jobs.WorkerSelector{
					WorkerIDs:    []string{"worker-b"},
					Labels:       map[string]string{"tier": "ops"},
					Capabilities: []string{"search"},
				},
				RetryPolicy: jobs.RetryPolicy{
					MaxAttempts: 1,
					Backoff: jobs.BackoffPolicy{
						Strategy:   jobs.BackoffStrategyFixed,
						FixedDelay: 5 * time.Second,
						MaxDelay:   5 * time.Second,
					},
				},
				CancelPolicy:  jobs.CancelPolicy{Mode: jobs.CancelModeBestEffort},
				ResumePolicy:  jobs.ResumePolicy{Mode: jobs.ResumeModeDisabled},
				TimeoutPolicy: jobs.TimeoutPolicy{Execution: time.Minute},
				LeasePolicy:   jobs.LeasePolicy{Duration: time.Minute},
			},
			State:     jobs.JobStateQueued,
			CreatedAt: now.Add(time.Second),
			UpdatedAt: now.Add(time.Second),
		},
	}

	for _, job := range jobsToCreate {
		require.NoError(t, jobStore.CreateJob(context.Background(), job))
	}

	sched := NewMemoryScheduler(jobStore, []WorkerDescriptor{
		{
			ID:              "worker-a",
			Kind:            "agent",
			Labels:          map[string]string{"tier": "ops"},
			Capabilities:    []string{"search", "write"},
			QueueAffinities: []string{"default"},
			Load:            3,
		},
		{
			ID:              "worker-b",
			Kind:            "agent",
			Labels:          map[string]string{"tier": "ops"},
			Capabilities:    []string{"search"},
			QueueAffinities: []string{"default"},
			Load:            1,
		},
	})
	sched.clock = func() time.Time { return now.Add(5 * time.Second) }

	result, err := sched.Dispatch(context.Background())
	require.NoError(t, err)
	require.Equal(t, "job-high", result.Job.ID)
	require.Equal(t, "worker-b", result.Worker.ID)
	require.Equal(t, "explicit worker id preference", result.Selection.Reason)
	require.Contains(t, result.Selection.EligibleIDs, "worker-b")

	loaded, err := jobStore.LoadJob(context.Background(), "job-high")
	require.NoError(t, err)
	require.Equal(t, jobs.JobStateLeased, loaded.State)
	require.NotNil(t, loaded.CurrentLease)
	require.Equal(t, "worker-b", loaded.CurrentLease.WorkerID)

	events, err := jobStore.LoadEvents(context.Background(), "job-high")
	require.NoError(t, err)
	require.NotEmpty(t, events)
	require.Equal(t, jobs.JobEventTypeLeased, events[len(events)-1].Type)
}

func TestSchedulerAdmissionAndCancelAndResume(t *testing.T) {
	jobStore := store.NewMemoryJobStore()
	now := time.Date(2026, 4, 21, 19, 0, 0, 0, time.UTC)
	job := jobs.Job{
		ID: "job-1",
		Spec: jobs.JobSpec{
			Kind:           "workflow.step",
			Payload:        map[string]any{"step": "inspect"},
			Queue:          "default",
			WorkerSelector: jobs.WorkerSelector{WorkerIDs: []string{"worker-a"}},
			RetryPolicy: jobs.RetryPolicy{
				MaxAttempts: 1,
				Backoff:     jobs.BackoffPolicy{Strategy: jobs.BackoffStrategyFixed, FixedDelay: 5 * time.Second, MaxDelay: 5 * time.Second},
			},
			CancelPolicy:  jobs.CancelPolicy{Mode: jobs.CancelModeBestEffort},
			ResumePolicy:  jobs.ResumePolicy{Mode: jobs.ResumeModeRestartable},
			TimeoutPolicy: jobs.TimeoutPolicy{Execution: time.Minute},
			LeasePolicy:   jobs.LeasePolicy{Duration: time.Minute},
		},
		State:     jobs.JobStateQueued,
		CreatedAt: now,
		UpdatedAt: now,
	}
	require.NoError(t, jobStore.CreateJob(context.Background(), job))

	sched := NewMemoryScheduler(jobStore, []WorkerDescriptor{{
		ID:              "worker-a",
		Capabilities:    []string{"search"},
		QueueAffinities: []string{"default"},
	}})

	admitted, err := sched.Admit(context.Background(), "job-1")
	require.NoError(t, err)
	require.Equal(t, jobs.JobStateAdmitted, admitted.State)

	cancelled, err := sched.Cancel(context.Background(), "job-1", "user request")
	require.NoError(t, err)
	require.Equal(t, jobs.JobStateCancelled, cancelled.State)

	resumed, err := sched.Resume(context.Background(), "job-1")
	require.NoError(t, err)
	require.Equal(t, jobs.JobStateResumed, resumed.State)
}

func TestLeaseExpirationMakesJobSchedulableAgain(t *testing.T) {
	jobStore := store.NewMemoryJobStore()
	now := time.Date(2026, 4, 21, 19, 15, 0, 0, time.UTC)
	job := jobs.Job{
		ID: "job-expire",
		Spec: jobs.JobSpec{
			Kind:           "workflow.step",
			Payload:        map[string]any{"step": "expire"},
			Queue:          "default",
			WorkerSelector: jobs.WorkerSelector{WorkerIDs: []string{"worker-a"}},
			RetryPolicy: jobs.RetryPolicy{
				MaxAttempts: 2,
				Backoff:     jobs.BackoffPolicy{Strategy: jobs.BackoffStrategyFixed, FixedDelay: 5 * time.Second, MaxDelay: 5 * time.Second},
			},
			CancelPolicy:  jobs.CancelPolicy{Mode: jobs.CancelModeBestEffort},
			ResumePolicy:  jobs.ResumePolicy{Mode: jobs.ResumeModeRestartable},
			TimeoutPolicy: jobs.TimeoutPolicy{Execution: time.Minute},
			LeasePolicy:   jobs.LeasePolicy{Duration: time.Minute},
		},
		State:     jobs.JobStateQueued,
		CreatedAt: now,
		UpdatedAt: now,
	}
	require.NoError(t, jobStore.CreateJob(context.Background(), job))

	sched := NewMemoryScheduler(jobStore, []WorkerDescriptor{{
		ID:              "worker-a",
		Capabilities:    []string{"search"},
		QueueAffinities: []string{"default"},
		Load:            1,
	}})
	sched.clock = func() time.Time { return now }

	first, err := sched.Dispatch(context.Background())
	require.NoError(t, err)
	require.Equal(t, jobs.JobStateLeased, first.Job.State)

	expiredLease, err := sched.ExpireLease(context.Background(), first.Job.ID, first.Lease.ID)
	require.NoError(t, err)
	require.Equal(t, first.Lease.ID, expiredLease.ID)

	loaded, err := jobStore.LoadJob(context.Background(), first.Job.ID)
	require.NoError(t, err)
	require.Equal(t, jobs.JobStateExpired, loaded.State)

	second, err := sched.Dispatch(context.Background())
	require.NoError(t, err)
	require.Equal(t, first.Job.ID, second.Job.ID)
	require.Equal(t, 2, second.Job.AttemptCount)
}

func TestRetrySchedulesDelayedReDispatch(t *testing.T) {
	jobStore := store.NewMemoryJobStore()
	base := time.Date(2026, 4, 21, 19, 30, 0, 0, time.UTC)
	job := jobs.Job{
		ID: "job-retry",
		Spec: jobs.JobSpec{
			Kind:           "workflow.step",
			Payload:        map[string]any{"step": "retry"},
			Queue:          "default",
			WorkerSelector: jobs.WorkerSelector{WorkerIDs: []string{"worker-a"}},
			RetryPolicy: jobs.RetryPolicy{
				MaxAttempts: 2,
				Backoff:     jobs.BackoffPolicy{Strategy: jobs.BackoffStrategyFixed, FixedDelay: 10 * time.Second, MaxDelay: 10 * time.Second},
			},
			CancelPolicy:  jobs.CancelPolicy{Mode: jobs.CancelModeBestEffort},
			ResumePolicy:  jobs.ResumePolicy{Mode: jobs.ResumeModeDisabled},
			TimeoutPolicy: jobs.TimeoutPolicy{Execution: time.Minute},
			LeasePolicy:   jobs.LeasePolicy{Duration: time.Minute},
		},
		State:     jobs.JobStateQueued,
		CreatedAt: base,
		UpdatedAt: base,
	}
	require.NoError(t, jobStore.CreateJob(context.Background(), job))

	sched := NewMemoryScheduler(jobStore, []WorkerDescriptor{{
		ID:              "worker-a",
		Capabilities:    []string{"search"},
		QueueAffinities: []string{"default"},
		Load:            1,
	}})
	current := base
	sched.clock = func() time.Time { return current }

	first, err := sched.Dispatch(context.Background())
	require.NoError(t, err)
	require.Equal(t, jobs.JobStateLeased, first.Job.State)

	retried, err := sched.Fail(context.Background(), first.Job.ID, first.Lease.ID, "transient", "temporary issue")
	require.NoError(t, err)
	require.Equal(t, jobs.JobStateRetrying, retried.State)
	require.True(t, retried.NextRetryAt.After(current))

	_, err = sched.Dispatch(context.Background())
	require.Error(t, err)

	current = current.Add(11 * time.Second)
	second, err := sched.Dispatch(context.Background())
	require.NoError(t, err)
	require.Equal(t, 2, second.Job.AttemptCount)
}

func TestResumeUsesCheckpoint(t *testing.T) {
	jobStore := store.NewMemoryJobStore()
	now := time.Date(2026, 4, 21, 19, 45, 0, 0, time.UTC)
	job := jobs.Job{
		ID: "job-resume",
		Spec: jobs.JobSpec{
			Kind:           "workflow.step",
			Payload:        map[string]any{"step": "resume"},
			Queue:          "default",
			WorkerSelector: jobs.WorkerSelector{WorkerIDs: []string{"worker-a"}},
			RetryPolicy: jobs.RetryPolicy{
				MaxAttempts: 2,
				Backoff:     jobs.BackoffPolicy{Strategy: jobs.BackoffStrategyFixed, FixedDelay: 5 * time.Second, MaxDelay: 5 * time.Second},
			},
			CancelPolicy: jobs.CancelPolicy{Mode: jobs.CancelModeBestEffort},
			ResumePolicy: jobs.ResumePolicy{
				Mode:               jobs.ResumeModeCheckpoint,
				RequiresCheckpoint: true,
				CheckpointKey:      "checkpoint-1",
			},
			TimeoutPolicy: jobs.TimeoutPolicy{Execution: time.Minute},
			LeasePolicy:   jobs.LeasePolicy{Duration: time.Minute},
		},
		State:     jobs.JobStatePaused,
		CreatedAt: now,
		UpdatedAt: now,
	}
	require.NoError(t, jobStore.CreateJob(context.Background(), job))
	require.NoError(t, jobStore.StoreCheckpoint(context.Background(), jobs.JobCheckpoint{
		ID:            "checkpoint-1",
		JobID:         job.ID,
		AttemptNumber: 1,
		Sequence:      1,
		ResumeToken:   "checkpoint-1",
		State:         map[string]any{"cursor": 42},
		CreatedAt:     now.Add(time.Second),
	}))

	sched := NewMemoryScheduler(jobStore, []WorkerDescriptor{{
		ID:              "worker-a",
		Capabilities:    []string{"search"},
		QueueAffinities: []string{"default"},
		Load:            1,
	}})
	sched.clock = func() time.Time { return now.Add(2 * time.Second) }

	resumed, err := sched.Resume(context.Background(), job.ID)
	require.NoError(t, err)
	require.Equal(t, jobs.JobStateResumed, resumed.State)
	require.Equal(t, "checkpoint-1", resumed.ResumeCheckpointID)

	loaded, err := jobStore.LoadJob(context.Background(), job.ID)
	require.NoError(t, err)
	require.Equal(t, "checkpoint-1", loaded.ResumeCheckpointID)
	require.Equal(t, jobs.JobStateResumed, loaded.State)
}

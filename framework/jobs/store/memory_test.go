package store

import (
	"context"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/jobs"
	"github.com/stretchr/testify/require"
)

func TestMemoryJobStoreRoundTripPersistsStateEventsAndCheckpoint(t *testing.T) {
	store := NewMemoryJobStore()
	now := time.Date(2026, 4, 21, 18, 30, 0, 0, time.UTC)

	job := jobs.Job{
		ID:     "job-1",
		Source: "task-1",
		Spec: jobs.JobSpec{
			Kind:     "workflow.step",
			Payload:  map[string]any{"step": "inspect"},
			Queue:    "default",
			Priority: 5,
			WorkerSelector: jobs.WorkerSelector{
				WorkerIDs: []string{"worker-a"},
			},
			RetryPolicy: jobs.RetryPolicy{
				MaxAttempts: 2,
				Backoff: jobs.BackoffPolicy{
					Strategy:   jobs.BackoffStrategyFixed,
					FixedDelay: 5 * time.Second,
					MaxDelay:   5 * time.Second,
				},
			},
			CancelPolicy:  jobs.CancelPolicy{Mode: jobs.CancelModeBestEffort},
			ResumePolicy:  jobs.ResumePolicy{Mode: jobs.ResumeModeCheckpoint, CheckpointKey: "resume.token"},
			TimeoutPolicy: jobs.TimeoutPolicy{Execution: time.Minute},
			LeasePolicy:   jobs.LeasePolicy{Duration: time.Minute},
		},
		State:     jobs.JobStateQueued,
		CreatedAt: now,
		UpdatedAt: now,
		Labels:    map[string]string{"tier": "ops"},
		Metadata:  map[string]any{"trace": "trace-1"},
	}

	require.NoError(t, store.CreateJob(context.Background(), job))

	job.State = jobs.JobStateRunning
	job.AttemptCount = 1
	job.CurrentAttempt = &jobs.JobAttempt{
		ID:            "attempt-1",
		JobID:         job.ID,
		AttemptNumber: 1,
		State:         jobs.JobStateRunning,
		StartedAt:     now.Add(time.Second),
	}
	job.CurrentLease = &jobs.JobLease{
		ID:          "lease-1",
		JobID:       job.ID,
		WorkerID:    "worker-a",
		AcquiredAt:  now.Add(time.Second),
		ExpiresAt:   now.Add(time.Minute),
		HeartbeatAt: now.Add(10 * time.Second),
	}
	job.UpdatedAt = now.Add(2 * time.Second)
	require.NoError(t, store.UpdateJob(context.Background(), job))
	require.NoError(t, store.StoreLease(context.Background(), *job.CurrentLease))

	require.NoError(t, store.AppendEvent(context.Background(), jobs.JobEvent{
		ID:         "event-1",
		JobID:      job.ID,
		Type:       jobs.JobEventTypeCreated,
		State:      jobs.JobStateQueued,
		OccurredAt: now,
	}))
	require.NoError(t, store.AppendEvent(context.Background(), jobs.JobEvent{
		ID:         "event-2",
		JobID:      job.ID,
		Type:       jobs.JobEventTypeStarted,
		State:      jobs.JobStateRunning,
		OccurredAt: now.Add(time.Second),
		LeaseID:    "lease-1",
		WorkerID:   "worker-a",
	}))

	require.NoError(t, store.StoreCheckpoint(context.Background(), jobs.JobCheckpoint{
		ID:            "checkpoint-1",
		JobID:         job.ID,
		AttemptNumber: 1,
		Sequence:      1,
		ResumeToken:   "resume.token",
		State:         map[string]any{"progress": "half"},
		CreatedAt:     now.Add(2 * time.Second),
		Metadata:      map[string]any{"kind": "intermediate"},
	}))

	loadedJob, err := store.LoadJob(context.Background(), job.ID)
	require.NoError(t, err)
	require.Equal(t, jobs.JobStateRunning, loadedJob.State)
	require.Equal(t, 1, loadedJob.AttemptCount)
	require.NotNil(t, loadedJob.CurrentAttempt)
	require.NotNil(t, loadedJob.CurrentLease)
	require.Equal(t, "ops", loadedJob.Labels["tier"])

	events, err := store.LoadEvents(context.Background(), job.ID)
	require.NoError(t, err)
	require.Len(t, events, 2)
	require.Equal(t, jobs.JobEventTypeCreated, events[0].Type)
	require.Equal(t, jobs.JobEventTypeStarted, events[1].Type)

	checkpoint, err := store.LoadCheckpoint(context.Background(), job.ID, "checkpoint-1")
	require.NoError(t, err)
	require.Equal(t, "resume.token", checkpoint.ResumeToken)
	require.Equal(t, "half", checkpoint.State.(map[string]any)["progress"])

	checkpoints, err := store.ListCheckpoints(context.Background(), job.ID)
	require.NoError(t, err)
	require.Len(t, checkpoints, 1)

	lease, err := store.LoadLease(context.Background(), job.ID, "lease-1")
	require.NoError(t, err)
	require.Equal(t, "worker-a", lease.WorkerID)

	leases, err := store.ListLeases(context.Background(), job.ID)
	require.NoError(t, err)
	require.Len(t, leases, 1)

	jobsByQueue, err := store.ListJobs(context.Background(), jobs.JobQuery{Queue: "default"})
	require.NoError(t, err)
	require.Len(t, jobsByQueue, 1)

	jobsByState, err := store.ListJobs(context.Background(), jobs.JobQuery{State: jobs.JobStateRunning})
	require.NoError(t, err)
	require.Len(t, jobsByState, 1)
}

func TestMemoryJobStoreRejectsUnknownJobReferences(t *testing.T) {
	store := NewMemoryJobStore()

	err := store.AppendEvent(context.Background(), jobs.JobEvent{
		ID:         "event-1",
		JobID:      "missing",
		Type:       jobs.JobEventTypeCreated,
		OccurredAt: time.Now().UTC(),
	})
	require.ErrorIs(t, err, jobs.ErrJobNotFound)

	err = store.StoreCheckpoint(context.Background(), jobs.JobCheckpoint{
		ID:            "checkpoint-1",
		JobID:         "missing",
		AttemptNumber: 1,
		State:         map[string]any{"x": 1},
		CreatedAt:     time.Now().UTC(),
	})
	require.ErrorIs(t, err, jobs.ErrJobNotFound)

	err = store.StoreLease(context.Background(), jobs.JobLease{
		ID:         "lease-1",
		JobID:      "missing",
		WorkerID:   "worker-a",
		AcquiredAt: time.Now().UTC(),
		ExpiresAt:  time.Now().Add(time.Minute).UTC(),
	})
	require.ErrorIs(t, err, jobs.ErrJobNotFound)
}

package jobs

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestJobSpecValidateRejectsEmptyQueueAndInvalidRetryPolicy(t *testing.T) {
	spec := JobSpec{
		Kind:    "workflow.step",
		Payload: map[string]any{"step": "inspect"},
		Queue:   "",
		WorkerSelector: WorkerSelector{
			WorkerIDs: []string{"worker-a"},
		},
		RetryPolicy: RetryPolicy{
			MaxAttempts: 1,
			Backoff: BackoffPolicy{
				Strategy: BackoffStrategyExponential,
			},
		},
		CancelPolicy: CancelPolicy{Mode: CancelModeBestEffort},
		ResumePolicy: ResumePolicy{Mode: ResumeModeDisabled},
		TimeoutPolicy: TimeoutPolicy{
			Execution: time.Minute,
		},
		LeasePolicy: LeasePolicy{
			Duration: time.Minute,
		},
	}

	require.ErrorContains(t, spec.Validate(), "queue required")

	spec.Queue = "default"
	err := spec.Validate()
	require.ErrorContains(t, err, "retry policy invalid")
	require.ErrorContains(t, err, "initial_delay")
}

func TestJobSpecValidateAcceptsMinimalContract(t *testing.T) {
	spec := JobSpec{
		Kind:    "workflow.step",
		Payload: map[string]any{"step": "inspect"},
		Queue:   "default",
		WorkerSelector: WorkerSelector{
			WorkerIDs: []string{"worker-a"},
		},
		RetryPolicy: RetryPolicy{
			MaxAttempts: 1,
			Backoff: BackoffPolicy{
				Strategy:   BackoffStrategyFixed,
				FixedDelay: 5 * time.Second,
				MaxDelay:   5 * time.Second,
				Jitter:     0.1,
			},
		},
		CancelPolicy: CancelPolicy{Mode: CancelModeBestEffort},
		ResumePolicy: ResumePolicy{Mode: ResumeModeDisabled},
		TimeoutPolicy: TimeoutPolicy{
			Execution: time.Minute,
		},
		LeasePolicy: LeasePolicy{
			Duration:          time.Minute,
			HeartbeatInterval: 10 * time.Second,
		},
		Tags: []string{"background"},
	}

	require.NoError(t, spec.Validate())
}

package jobs

import (
	"context"
	"errors"
)

var (
	ErrJobNotFound        = errors.New("job not found")
	ErrJobAlreadyExists   = errors.New("job already exists")
	ErrEventAlreadyExists = errors.New("job event already exists")
	ErrCheckpointNotFound = errors.New("job checkpoint not found")
	ErrCheckpointExists   = errors.New("job checkpoint already exists")
	ErrLeaseNotFound      = errors.New("job lease not found")
	ErrLeaseAlreadyExists = errors.New("job lease already exists")
)

type JobQuery struct {
	Queue string   `json:"queue,omitempty" yaml:"queue,omitempty"`
	State JobState `json:"state,omitempty" yaml:"state,omitempty"`
	Limit int      `json:"limit,omitempty" yaml:"limit,omitempty"`
}

func (q JobQuery) Validate() error {
	if q.State != "" {
		if err := q.State.Validate(); err != nil {
			return err
		}
	}
	if q.Limit < 0 {
		return errors.New("limit must be >= 0")
	}
	return nil
}

type JobStore interface {
	CreateJob(ctx context.Context, job Job) error
	UpdateJob(ctx context.Context, job Job) error
	LoadJob(ctx context.Context, jobID string) (*Job, error)
	ListJobs(ctx context.Context, query JobQuery) ([]Job, error)

	AppendEvent(ctx context.Context, event JobEvent) error
	LoadEvents(ctx context.Context, jobID string) ([]JobEvent, error)

	StoreCheckpoint(ctx context.Context, checkpoint JobCheckpoint) error
	LoadCheckpoint(ctx context.Context, jobID, checkpointID string) (*JobCheckpoint, error)
	ListCheckpoints(ctx context.Context, jobID string) ([]JobCheckpoint, error)

	StoreLease(ctx context.Context, lease JobLease) error
	UpdateLease(ctx context.Context, lease JobLease) error
	LoadLease(ctx context.Context, jobID, leaseID string) (*JobLease, error)
	ListLeases(ctx context.Context, jobID string) ([]JobLease, error)
}
